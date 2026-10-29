// Retro API surface — the old retrogaming service's REST routes, now served
// by the sessions service. ROMs/BIOS/saves come straight from the shared
// storage mount through the retro_files package (pure filesystem, no
// session dependency); the session routes talk to the spawned containers.
package api

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"gameflex/storage"
	"gameflex/storage/listing"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gameflex/kernel/internal/auth"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/installs"
	"gameflex/storage/saves"
)

// retroRomsRoot/savesRoot/biosRoot/coresRoot — the sections of the storage
// folder, as this service mounts it.
func (server *Server) retroRomsRoot() string  { return filepath.Join(server.storageRoot, "roms") }
func (server *Server) retroSavesRoot() string { return filepath.Join(server.storageRoot, "saves") }
func (server *Server) retroBiosRoot() string  { return filepath.Join(server.storageRoot, "bios") }
func (server *Server) retroCoresRoot() string { return filepath.Join(server.storageRoot, "cores") }

// handleConsolesRegistry — the STATIC console registry
// (id, name, publisher, method, cores, maxPlayers) the frontend's menu,
// emulator picker and player-slot logic depend on (gameflex-consoles).
func (server *Server) handleConsolesRegistry(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, consoles.List())
}

// handleRetroConsoles — {id, count} per console, cross-referenced with the
// registry by the frontend (same shape as the old service's ListConsoles).
//
// LE COMPTE VIENT DU CATALOGUE, PAS D'UN NOUVEAU PARCOURS DU DISQUE (#107) :
// c'est le même que celui de la liste (`handleRetroRoms`), donc le nombre
// annoncé est celui des jeux affichés. `ScanConsoles` relisait le disque à
// chaque affichage et comptait aussi ce qu'une console refuse — 521 jeux gbc
// annoncés pour 494 listés.
func (server *Server) handleRetroConsoles(w http.ResponseWriter, r *http.Request) {
	// LA BIBLIOTHÈQUE D'ICI, ET ELLE SEULE (chantier 14) : ce compte répond à
	// « pour quelles consoles ai-je des jeux ? », et un jeu de chez un ami ou
	// d'un partage monté n'est pas ici tant qu'on ne l'a pas ajouté. Lu dans
	// le listing de ce serveur (chantier 15), comme la liste elle-même.
	server.ensureSelf()
	header, err := listing.LoadHeader(listingCacheDir, listing.SelfKey)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "the listing of this server is not built yet")

		return
	}
	// LES DISQUES INTERNES COMPTENT AUSSI (chantier 23) : ils sont à cette
	// machine, et une console dont les jeux ne sont plus que sur l'un d'eux
	// doit rester dans le menu.
	counts := server.mergedConsoleCounts(r.Context(), header.Sections[listing.Roms].Consoles)
	summaries := make([]retro_files.ConsoleSummary, 0, len(counts))
	for id, count := range counts {
		summaries = append(summaries, retro_files.ConsoleSummary{ID: id, Count: count})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	writeJSON(w, http.StatusOK, summaries)
}

// handleRetroRoms — GET /api/v1/retrogaming/{consoleId}/roms?q=[&server=<id>]
//
// LES JEUX D'ICI, ET EUX SEULS (chantier 14, 2026-10-12). Ceux des partages
// montés et des serveurs fédérés s'y mêlaient, et se jouaient ici après une
// copie dans un cache — une seconde bibliothèque : « mettre en cache dans
// gameflex/cache/roms c'est exactement comme mettre dans gameflex/roms, ça n'a
// juste pas de sens d'avoir les jeux dans différents dossiers » (le porteur).
//
// `server` — le sélecteur de source de la page désigne UN serveur fédéré : ce
// sont alors ses jeux, et seulement les siens (`servePeerRoms`). Un partage
// monté n'a pas de liste ici : il ne sert qu'à l'import, dans Réglages › Jeux.
func (server *Server) handleRetroRoms(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	query := r.URL.Query().Get("q")

	if peer := r.URL.Query().Get("server"); peer != "" {
		server.servePeerRoms(w, peer, consoleID, query)

		return
	}
	// `disk` — le sélecteur désigne un disque interne (chantier 23) : ses
	// jeux, qui se jouent ici comme ceux de la bibliothèque.
	if disk := r.URL.Query().Get("disk"); disk != "" {
		server.serveDiskRoms(w, r, disk, consoleID, query)

		return
	}

	// LE LISTING DE CE SERVEUR (chantier 15), plus le disque : ce que GameFlex
	// a écrit y est déjà, ce qu'on a déposé à la main attend un parcours.
	needle := strings.ToLower(strings.TrimSpace(query))
	roms := []retro_files.RomEntry{}
	// Les statuts se lisent dans leur fichier, pas dans le listing : c'est
	// lui qui fait foi, et un signalement s'y voit sans attendre.
	statuses := server.consoleStatus(consoleID)
	for _, item := range server.selfItems(listing.Roms, consoleID) {
		if needle != "" && !strings.Contains(strings.ToLower(item.Name), needle) {
			continue
		}
		files := item.Files
		if files == nil {
			files = []string{}
		}
		roms = append(roms, retro_files.RomEntry{
			Reference: item.ID, Title: item.Name, Files: files, SizeBytes: item.Size, Folder: item.Dir,
			Status: statusView(consoleID, statuses, item.ID), Engine: item.Engine,
		})
	}

	writeJSON(w, http.StatusOK, roms)
}

// handleRetroBios — GET /api/v1/retrogaming/{consoleId}/bios
func (server *Server) handleRetroBios(w http.ResponseWriter, r *http.Request) {
	// Avec les tailles depuis le 2026-09-01 : l'écran d'administration les
	// affiche, et le sélecteur de BIOS d'une fiche de jeu n'a qu'à lire le
	// champ `name`. Une seule route, une seule forme.
	// Lus dans le listing de ce serveur (chantier 15).
	biosFiles := []retro_files.BiosEntry{}
	for _, item := range server.selfItems(listing.Bios, chi.URLParam(r, "consoleId")) {
		biosFiles = append(biosFiles, retro_files.BiosEntry{Name: item.Name, SizeBytes: item.Size, IsFolder: item.Dir})
	}
	writeJSON(w, http.StatusOK, biosFiles)
}

// handleRetroCores — GET /api/v1/retrogaming/{consoleId}/cores.
//
// Les cores de cette console, installés ou non. Le pendant de /roms et de
// /bios : trois ressources, trois routes de la même forme, chacune tenue par
// sa console. L'écran d'administration n'a ainsi rien à savoir du rangement
// sur le disque.
func (server *Server) handleRetroCores(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	writeJSON(w, http.StatusOK, append(
		coreEntriesFrom(consoleID, server.selfItems(listing.Cores, consoleID)),
		server.streamexCoreEntries(r.Context(), consoleID)...))
}

// streamexCoreEntries — LES ÉMULATEURS EN CONTENEUR de cette console, pour la
// fiche d'un jeu : présents ceux qu'une image de ce serveur fait tourner
// (docker.StreamexCores — Citron et Eden pour la Switch, chantier 26), absents
// les autres — Eden, du 2026-10-14 au 2026-10-16, que seul le téléphone
// faisait tourner. La fiche ne recevait aucun émulateur streamex, donc n'en
// grisait aucun : elle aurait proposé un émulateur qu'aucune image n'a, et le
// serveur lancé le sien à sa place. Hors de coreEntriesFrom : l'écran du
// stockage, qui la lit aussi, n'a rien à télécharger pour eux.
//
// Et depuis que l'image ne porte plus son émulateur (chantier 29), présent
// veut dire aussi : une de ses versions est dans cores/<console>/. `File`
// nomme celle qu'une partie prendrait, ou la version testée quand il n'y en a
// aucune.
func (server *Server) streamexCoreEntries(ctx context.Context, consoleID string) []retro_files.CoreEntry {
	running := docker.StreamexCores(consoleID)
	if len(running) == 0 {
		return nil
	}
	entries := []retro_files.CoreEntry{}
	for _, core := range consoles.CoresFor(consoleID) {
		if !strings.Contains(core, " ") {
			continue
		}
		entry := retro_files.CoreEntry{Name: core, Installed: slices.Contains(running, core)}
		if need, ok := server.requiredEmulatorFor(ctx, consoleID, core); ok {
			name, _, present := server.emulatorInUse(need)
			entry.File, entry.Installed = need.File, entry.Installed && present
			if present {
				entry.File = name
			}
		}
		entries = append(entries, entry)
	}

	return entries
}

// coreEntriesFrom — les cœurs que le registre déclare pour une console, et
// ceux qu'un listing dit présents : la forme de `retro_files.ListCoreFiles`,
// que l'écran et la fiche d'un jeu lisent.
func coreEntriesFrom(consoleID string, items []listing.Item) []retro_files.CoreEntry {
	present := map[string]listing.Item{}
	for _, item := range items {
		present[item.Name] = item
	}
	entries := []retro_files.CoreEntry{}
	for _, core := range consoles.CoresFor(consoleID) {
		if strings.Contains(core, " ") {
			continue // un émulateur en conteneur, pas un core libretro
		}
		entry := retro_files.CoreEntry{Name: core, File: retro_files.CoreFileName(core)}
		if item, ok := present[core]; ok {
			if size, linux := item.Variants["linux"]; linux {
				entry.Installed, entry.Scoped, entry.SizeBytes = true, true, size
			}
			if size, android := item.Variants["android"]; android {
				entry.AndroidInstalled, entry.AndroidSizeBytes = true, size
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	return entries
}

// handleRetroLibrary — GET /api/v1/retrogaming/library/{section}.
//
// L'inventaire d'une section du stockage (bios ou cores), lue comme un
// dossier et non comme une console : c'est ce que « Ma bibliothèque »
// montre. Les jeux ont déjà leur route par console — un jeu EST un dossier
// ou un fichier de ce dossier-là, la question ne se pose pas pour eux.
func (server *Server) handleRetroLibrary(w http.ResponseWriter, r *http.Request) {
	var (
		entries []retro_files.LibraryEntry
		err     error
	)
	switch chi.URLParam(r, "section") {
	case "bios":
		entries, err = retro_files.ListBiosTree(server.retroBiosRoot())
	case "cores":
		entries, err = retro_files.ListCoreTree(server.retroCoresRoot())
	default:
		writeError(w, http.StatusBadRequest, "unknown section")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read section: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleRetroBiosDirs — GET /api/v1/retrogaming/library/bios-dirs.
//
// La table « console → dossier de BIOS », telle quelle. L'interface en a
// besoin pour l'INVERSE : elle voit un dossier « psx » et doit savoir qu'il
// désigne la PlayStation, sinon un jeu de BIOS parfaitement rangé passe
// tout entier pour inconnu. Recopier cette table côté page l'aurait fait
// diverger au premier ajout.
func (server *Server) handleRetroBiosDirs(w http.ResponseWriter, r *http.Request) {
	dirs := map[string]string{}
	for _, console := range consoles.All() {
		if folder := consoles.BiosDir(console.ID); folder != "" {
			dirs[console.ID] = folder
		}
	}
	writeJSON(w, http.StatusOK, dirs)
}

// handleRetroServeRom — GET/HEAD /api/v1/retrogaming/{consoleId}/roms/{ref}.
// Serves the single file directly, or zips a multi-file game folder on the
// fly (the folder name IS the title, so the zip keeps a readable name).
func (server *Server) handleRetroServeRom(w http.ResponseWriter, r *http.Request) {
	server.serveRomFile(w, r, chi.URLParam(r, "consoleId"), chi.URLParam(r, "ref"))
}

// serveRomFile — LE MÊME CODE POUR LES DEUX CHEMINS.
//
// Deux routes mènent au même fichier : celle qui exige un en-tête (la fiche,
// via `fetch`) et celle qui consomme un ticket (le navigateur, sans en-tête —
// voir retro_download_ticket.go). Elles servent donc par CE corps commun, et
// non par deux copies : deux chemins qui ne résolvent pas pareil, c'est un
// téléchargement qui marche d'un côté et 404 de l'autre, sans que rien ne le
// dise. La console et la référence arrivent en argument parce qu'elles viennent
// de l'URL dans un cas et du ticket dans l'autre.
func (server *Server) serveRomFile(w http.ResponseWriter, r *http.Request, consoleID, ref string) {
	// La bibliothèque, puis les disques internes (chantier 23) : un ami voit
	// les deux comme une seule, et télécharge un jeu là où il est.
	path, isDir, _, err := server.findRom(r.Context(), consoleID, ref)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "rom not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rom reference: "+err.Error())
		return
	}

	if !isDir {
		// LE NOM DU FICHIER, AVEC SON EXTENSION.
		//
		// `ServeFile` ne pose pas d'en-tête de disposition : le navigateur
		// n'avait donc aucun nom à proposer, et la page retombait sur le
		// TITRE du jeu — que le scanner rend justement sans extension. Le
		// fichier téléchargé arrivait sans « .nes » et ne servait à rien
		// (signalé le 2026-09-02).
		//
		// La réponse fait autorité : c'est ici qu'on sait quel fichier est
		// réellement sur le disque.
		w.Header().Set("Content-Disposition",
			`attachment; filename="`+strings.ReplaceAll(filepath.Base(path), `"`, "")+`"`)
		http.ServeFile(w, r, path)
		return
	}

	serveDirectoryAsZip(w, r, path, filepath.Base(path))
}

// handleRetroDeleteRom — DELETE /api/v1/retrogaming/{consoleId}/roms/{ref}.
//
// SYMÉTRIQUE DU GET, volontairement : même résolveur, même identifiant, mêmes
// formes d'erreur. Un jeu se désigne d'une seule façon, qu'on le lise ou qu'on
// l'efface — c'est ce qui évite qu'une des deux routes accepte une référence
// que l'autre refuse.
//
// `RemoveAll` et non `Remove` : un jeu peut ÊTRE un dossier (un .cue et ses
// pistes, un dump en plusieurs fichiers). `ResolveCatalogRom` le dit déjà par
// son `isDir`, dont le GET se sert pour zipper à la volée ; ici les deux cas
// s'effacent pareil, donc il n'y a rien de plus à décider.
//
// Aucune invalidation de catalogue à appeler : il se rafraîchit sur l'âge des
// dossiers (`catalogIsStale`), et supprimer change justement la date du dossier
// parent. Le jeu aura disparu au prochain listage.
func (server *Server) handleRetroDeleteRom(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	reference := chi.URLParam(r, "ref")

	// `?disk=<nom>` — la copie de CE disque interne (chantier 23), celle que
	// l'administrateur regarde ; sans lui, celle de la bibliothèque.
	romsRoot := server.retroRomsRoot()
	if name := r.URL.Query().Get("disk"); name != "" {
		disk, ok := server.diskNamed(w, r, name)
		if !ok {
			return
		}
		romsRoot = disk.RomsDir()
	}

	path, _, err := retro_files.ResolveCatalogRom(romsRoot, consoleID, reference)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "rom not found")

		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rom reference: "+err.Error())

		return
	}

	// Ce que streamex en a déballé (un jeu PS4), lu AVANT de supprimer.
	var stems []string
	if consoleID == "ps4" {
		stems = pkgStems(path)
	}
	if err := os.RemoveAll(path); err != nil {
		writeError(w, http.StatusInternalServerError, "suppression impossible : "+err.Error())

		return
	}
	server.forgetUnpacked(r.Context(), stems)

	writeJSON(w, http.StatusOK, map[string]any{
		"consoleId": consoleID, "reference": reference, "removed": true,
	})
}

// pkgStems — les .pkg d'un jeu PS4 (un fichier, ou un dossier qui en porte
// plusieurs : le jeu et sa mise à jour), sous le nom où streamex les déballe :
// `cache/installs/ps4/<nom sans .pkg>` (adapter/generic.go).
func pkgStems(path string) []string {
	stems := []string{}
	_ = filepath.WalkDir(path, func(file string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.EqualFold(filepath.Ext(file), ".pkg") {
			stems = append(stems, strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
		}

		return nil
	})

	return stems
}

// forgetUnpacked — LE DÉBALLÉ D'UN JEU PS4 PART AVEC LUI (chantier 22) :
// partagé par tous les comptes, il restait sinon orphelin pour toujours (FF7
// Remake : 145 Go). Sauf si un autre .pkg du même nom reste, dans la
// bibliothèque ou sur un disque interne : streamex le déballerait au même
// endroit, et il servirait encore.
func (server *Server) forgetUnpacked(ctx context.Context, stems []string) {
	if len(stems) == 0 {
		return
	}
	remaining := map[string]bool{}
	roots := []string{server.retroRomsRoot()}
	for _, disk := range server.presentDisks(ctx) {
		roots = append(roots, disk.RomsDir())
	}
	for _, root := range roots {
		for _, stem := range pkgStems(filepath.Join(root, "ps4")) {
			remaining[stem] = true
		}
	}
	for _, stem := range stems {
		if remaining[stem] {
			continue
		}
		unpacked := installs.Dir(server.storageRoot, "ps4", stem)
		for _, leftover := range []string{unpacked, unpacked + ".partial", unpacked + ".lock"} {
			if err := os.RemoveAll(leftover); err != nil {
				slog.Warn("déballé non retiré", "path", leftover, "error", err)
			}
		}
		slog.Info("déballé retiré avec son jeu", "game", stem)
	}
}

// serveDirectoryAsZip streams a folder as a zip — the shape a multi-file
// game takes on the wire (a .cue and its .bin tracks), and the shape a
// folder-shaped BIOS dump takes too (chantier 6). Extracted from
// handleRetroServeRom when the federated surface needed the same behaviour:
// two writers of the same zip would have drifted, and the caller that
// drifted would have been the one nobody tests.
//
// IL DESCEND DANS LES SOUS-DOSSIERS, et le zip garde les chemins relatifs.
// Il ne le faisait pas : les entrées `IsDir()` étaient simplement ignorées,
// ce qui convenait aux seuls cas connus (tous plats) et donnait, au premier
// jeu arborescent (un jeu PS5, 2026-09-10), une archive de 31 Mo pour un jeu
// de 3,2 Go — sans erreur, sans avertissement. Pour un dossier plat le
// résultat est identique à avant : le chemin relatif y EST le nom du
// fichier.
func serveDirectoryAsZip(w http.ResponseWriter, r *http.Request, path, name string) {
	if _, err := os.ReadDir(path); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read folder: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.zip"`)
	if r.Method == http.MethodHead {
		return
	}
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()
	filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // best effort : un dossier illisible n'annule pas le reste
		}
		if retro_files.IsHidden(entry.Name()) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(path, current)
		if err != nil {
			return nil
		}
		if err := retro_files.AddFileToZip(zipWriter, current, filepath.ToSlash(relative)); err != nil {
			return err // en-têtes déjà envoyés — on s'arrête là
		}
		return nil
	})
}

// handleRetroScreenshot — GET /api/v1/retrogaming/savestate/screenshot
func (server *Server) handleRetroScreenshot(w http.ResponseWriter, r *http.Request) {
	consoleID := r.URL.Query().Get("consoleId")
	reference := r.URL.Query().Get("reference")
	// From the token, never the query string — see retroRouter's own doc.
	username := auth.Username(r.Context())
	if consoleID == "" || reference == "" || username == "" {
		writeError(w, http.StatusBadRequest, "consoleId and reference are required")
		return
	}
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	slot, err := strconv.Atoi(r.URL.Query().Get("slot"))
	if err != nil {
		slot = retro_files.AutoSaveSlot()
	}
	path, err := retro_files.StateFilePath(root, owner, consoleID, reference, slot, ".png")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "no screenshot found")
		return
	}
	http.ServeFile(w, r, path)
}

// handleRetroSaveFile — GET /api/v1/retrogaming/savestate/file
//
// L'ÉTAT LUI-MÊME, et c'est ce qui manquait pour synchroniser (#181). On savait
// lister les emplacements, lire la vignette d'un créneau et en supprimer un,
// mais jamais obtenir l'état ni le déposer : écrire ne passait que par le
// WebSocket d'une session EN COURS, donc supposait un émulateur qui tourne —
// inutilisable pour copier une sauvegarde d'un serveur à l'autre.
//
// C'EST LE NAVIGATEUR QUI TRANSFÈRE, et cette route est donc appelée aussi
// depuis une AUTRE origine. Lui seul tient les deux clés — notre jeton et le
// compte chez le pair —, ce dernier ne vivant que dans son `localStorage` : il
// n'existe aucune session de pair côté serveur. Rien à ouvrir pour autant, le
// kernel répond déjà en cross-origin, pré-vol compris (`withCORS`, api.go).
func (server *Server) handleRetroSaveFile(w http.ResponseWriter, r *http.Request) {
	consoleID := r.URL.Query().Get("consoleId")
	reference := r.URL.Query().Get("reference")
	// Du jeton, jamais de la requête — voir la doc de retroRouter.
	username := auth.Username(r.Context())
	if consoleID == "" || reference == "" || username == "" {
		writeError(w, http.StatusBadRequest, "consoleId and reference are required")
		return
	}
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	slot, err := strconv.Atoi(r.URL.Query().Get("slot"))
	if err != nil || slot < 0 || slot >= retro_files.MaxSaveSlots() {
		writeError(w, http.StatusBadRequest, "slot is required")
		return
	}
	path, err := retro_files.StateFilePath(root, owner, consoleID, reference, slot, ".state")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "no save in this slot")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path)
}

// maxSaveStateBytes — le plus gros état reçu. Le plus lourd mesuré pèse
// 104 Mo (une Wii, 2026-10-28) ; sans plafond, un compte remplissait le
// disque d'un seul envoi (revue de sécurité du kernel, #601).
const maxSaveStateBytes = 512 << 20

// handleRetroSaveFileUpload — POST /api/v1/retrogaming/savestate/file
//
// LE CORPS EST LES OCTETS, PAS DU JSON, et cette convention est CHOISIE faute
// de précédent : aucune route de ce kernel ne recevait de fichier jusqu'ici,
// tout `r.Body` y étant du JSON de commande. Un état de PS2 pèse des dizaines
// de mégaoctets, que du base64 gonflerait d'un tiers en les gardant tous en
// mémoire ; `io.Copy` les écrit au fil de l'eau, et la lecture ci-dessus rend
// déjà des octets bruts — les deux sens se ressemblent.
//
// DANS UNE COPIE, JAMAIS DANS LE PRINCIPAL (chantier 38, le porteur,
// 2026-10-29 : « chaque upload qu'on fera sur des serveurs fédérés, ça ne va
// pas remplacer la sauvegarde principale, ça va juste aller dans le
// sous-dossier sources/{nom serveur} »). `source` est le nom que la fédération
// donne à l'expéditeur (saves.SourceName) ; `title`, le titre du jeu, gardé
// avec la copie pour un jeu qu'ici n'a pas ; `screenshot=1` dépose la vignette
// de l'emplacement au lieu de son état. Seule la restauration
// (`POST /api/v1/saves/states/restore`), un geste d'ici, écrit le principal.
func (server *Server) handleRetroSaveFileUpload(w http.ResponseWriter, r *http.Request) {
	consoleID := r.URL.Query().Get("consoleId")
	reference := r.URL.Query().Get("reference")
	username := auth.Username(r.Context())
	if consoleID == "" || reference == "" || username == "" {
		writeError(w, http.StatusBadRequest, "consoleId and reference are required")
		return
	}
	slot, err := strconv.Atoi(r.URL.Query().Get("slot"))
	if err != nil || slot < 0 || slot >= retro_files.MaxSaveSlots() {
		writeError(w, http.StatusBadRequest, "slot is required")
		return
	}
	// LA DATE DE L'ORIGINAL, quand on copie (storage.ParseSaveTimestamp, #395).
	saved, err := storage.ParseSaveTimestamp(r.URL.Query().Get("timestamp"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source := saves.SourceName(r.URL.Query().Get("source"))
	if source == "" {
		writeError(w, http.StatusBadRequest, "source is required: an upload only writes into a copy")
		return
	}
	root, owner, err := saves.TreeParts(server.retroSavesRoot(), username, source)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid save source")
		return
	}
	ext := ".state"
	if r.URL.Query().Get("screenshot") == "1" {
		ext = ".png"
	}
	path, err := retro_files.StateFilePath(root, owner, consoleID, reference, slot, ext)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "cannot create the save directory")
		return
	}

	// ÉCRIRE À CÔTÉ PUIS RENOMMER : un transfert coupé au milieu laisserait
	// sinon un état tronqué SOUS LE NOM D'UN ÉTAT VALIDE, que le prochain
	// chargement accepterait avant de planter l'émulateur. Le renommage est
	// atomique sur le même système de fichiers.
	temporary := path + ".part"
	file, err := os.Create(temporary)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot write the save")
		return
	}
	written, copyErr := io.Copy(file, http.MaxBytesReader(w, r.Body, maxSaveStateBytes))
	closeErr := file.Close()
	var tooLarge *http.MaxBytesError
	if errors.As(copyErr, &tooLarge) {
		_ = os.Remove(temporary)
		writeError(w, http.StatusRequestEntityTooLarge, "the save is too large")
		return
	}
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(temporary)
		writeError(w, http.StatusInternalServerError, "cannot write the save")
		return
	}
	if written == 0 {
		_ = os.Remove(temporary)
		writeError(w, http.StatusBadRequest, "the save is empty")
		return
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		writeError(w, http.StatusInternalServerError, "cannot write the save")
		return
	}

	if err := storage.KeepSaveTimestamp(path, saved); err != nil {
		slog.Warn("save timestamp not kept", "path", path, "error", err)
	}
	if err := saves.RememberTitle(filepath.Join(root, owner), consoleID, reference, r.URL.Query().Get("title")); err != nil {
		slog.Warn("save title not kept", "source", source, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sizeBytes": written})
}

// handleRetroSaveSlots — GET /api/v1/retrogaming/savestate/slots
func (server *Server) handleRetroSaveSlots(w http.ResponseWriter, r *http.Request) {
	consoleID := r.URL.Query().Get("consoleId")
	reference := r.URL.Query().Get("reference")
	username := auth.Username(r.Context())
	if consoleID == "" || reference == "" || username == "" {
		writeError(w, http.StatusBadRequest, "consoleId and reference are required")
		return
	}
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}

	slots := make([]retroSaveSlotInfo, retro_files.MaxSaveSlots())
	for i := range slots {
		slots[i] = retroSaveSlotInfo{Slot: i}
		path, err := retro_files.StateFilePath(root, owner, consoleID, reference, i, ".state")
		if err != nil {
			continue
		}
		if info, err := os.Stat(path); err == nil {
			slots[i].Exists = true
			modTime := info.ModTime()
			slots[i].Timestamp = &modTime
			slots[i].SizeBytes = info.Size()
		}
	}
	writeJSON(w, http.StatusOK, slots)
}

// retroSaveSlotInfo — one entry in the slots list; Timestamp is a pointer
// (null, not Go's zero date) when the slot is empty.
type retroSaveSlotInfo struct {
	Slot      int        `json:"slot"`
	Exists    bool       `json:"exists"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
	// SizeBytes et Timestamp reconnaissent une sauvegarde déjà copiée : une
	// copie garde la date de l'original (`timestamp` à l'envoi, #395).
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// handleRetroUserSaves — GET /api/v1/retrogaming/savestate/user?username=
func (server *Server) handleRetroUserSaves(w http.ResponseWriter, r *http.Request) {
	username := auth.Username(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	diskRoots := []string{}
	for _, disk := range server.presentDisks(r.Context()) {
		diskRoots = append(diskRoots, disk.RomsDir())
	}
	entries, err := retro_files.ListUserSaveStates(server.retroRomsRoot(), root, owner, diskRoots...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list saves: %v", err))
		return
	}
	// UNE COPIE VENUE D'AILLEURS porte parfois un jeu qu'ici n'a pas : son
	// titre a voyagé avec l'envoi (saves.RememberTitle).
	if source := r.URL.Query().Get("source"); source != "" {
		tree := filepath.Join(root, owner)
		for index, entry := range entries {
			if entry.Title == entry.Reference {
				if title := saves.Title(tree, entry.ConsoleID, entry.Reference); title != "" {
					entries[index].Title = title
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleRetroDeleteSave — DELETE /api/v1/retrogaming/savestate
func (server *Server) handleRetroDeleteSave(w http.ResponseWriter, r *http.Request) {
	consoleID := r.URL.Query().Get("consoleId")
	reference := r.URL.Query().Get("reference")
	username := auth.Username(r.Context())
	if consoleID == "" || reference == "" || username == "" {
		writeError(w, http.StatusBadRequest, "consoleId and reference are required")
		return
	}
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	slot, err := strconv.Atoi(r.URL.Query().Get("slot"))
	if err != nil || slot < 0 || slot >= retro_files.MaxSaveSlots() {
		writeError(w, http.StatusBadRequest, "invalid slot")
		return
	}
	if err := retro_files.RemoveSaveFiles(root, owner, consoleID, reference, slot); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRetroDeleteGameSaves — DELETE /api/v1/retrogaming/savestate/game
func (server *Server) handleRetroDeleteGameSaves(w http.ResponseWriter, r *http.Request) {
	consoleID := r.URL.Query().Get("consoleId")
	reference := r.URL.Query().Get("reference")
	username := auth.Username(r.Context())
	if consoleID == "" || reference == "" || username == "" {
		writeError(w, http.StatusBadRequest, "consoleId and reference are required")
		return
	}
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	for i := 0; i < retro_files.MaxSaveSlots(); i++ {
		if err := retro_files.RemoveSaveFiles(root, owner, consoleID, reference, i); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete: %v", err))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// retroRouter — chi subrouter for the /api/v1/retrogaming/* surface (mounted
// by Routes). chi resolves the overlapping wildcard patterns by static
// segments; the stdlib ServeMux rejects them outright.
// retroRouter — the whole /api/v1/retrogaming surface, behind the SAME JWT
// middleware as /api/v1. It used to be mounted with no authentication at all
// while the /api/v1 routes right beside it were guarded, which is the kind of
// asymmetry nobody re-reads: launching a container, downloading a ROM and
// deleting someone's save states were all reachable from the LAN without a
// token (the port is published on 0.0.0.0).
//
// Two consequences worth knowing:
//
//   - every handler here takes the username from auth.Username(ctx), never
//     from a query parameter. A caller-supplied username was both an
//     impersonation vector (launch a session as someone else) and the source
//     of the path traversal in retro_files.stateFilePath;
//   - /stream/ws is registered OUTSIDE this middleware and does its own check
//     on ?token=. A browser cannot attach an Authorization header to a
//     WebSocket handshake — same reason /api/v1/facade/ws authenticates that
//     way.
func (server *Server) retroRouter() http.Handler {
	router := chi.NewRouter()

	// WebSocket first, on its own auth path (see the doc above).
	router.Get("/stream/ws", server.handleRetroStreamWS)

	// LE TÉLÉCHARGEMENT PAR TICKET, hors middleware pour la MÊME raison que le
	// WebSocket juste au-dessus : un navigateur qui navigue vers un lien ne
	// peut pas y attacher d'en-tête Authorization. Ce n'est pas une brèche mais
	// l'inverse — le ticket ne porte aucune identité, ne vaut que pour UN jeu
	// nommé à l'émission, expire en une minute et se consomme au premier usage
	// (retro_download_ticket.go). Son ÉMISSION, elle, est authentifiée.
	//
	// Un segment statique l'emporte sur un paramètre chez chi : `/downloads/x`
	// ne peut donc pas être capté par `/{id}/status`, exactement comme
	// `/stream/ws`.
	router.Get("/downloads/{ticket}", server.handleRetroDownloadByTicket)
	router.Head("/downloads/{ticket}", server.handleRetroDownloadByTicket)

	// Group, not Use: chi panics if middleware is declared after a route on
	// the same mux, and /stream/ws above has to stay outside it.
	router.Group(func(guarded chi.Router) {
		guarded.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(auth.Middleware(server.config.JWTSecret, next.ServeHTTP))
		})

		guarded.Get("/consoles", server.handleRetroConsoles)
		// LES RESSOURCES INTERNES (chantier 23) : les autres disques de jeux
		// de cette machine. Les lister est ouvert à tous (le sélecteur de la
		// page des jeux) ; les relire est un geste d'administration.
		guarded.Get("/disks", server.handleInternalDisks)
		guarded.Post("/disks/{name}/rescan", adminOnly(server.handleInternalDiskRescan))
		// Ce qu'un disque apporte d'autre que des jeux (chantier 24) — ses
		// cœurs, BIOS, paramètres et correctifs —, et l'importer dans le
		// dossier : les mêmes gestes que pour un ami (`/servers/{id}/storage`).
		guarded.Get("/disks/{name}/storage/{section}", adminOnly(server.handleDiskStorage))
		guarded.Post("/disks/{name}/storage/{section}", adminOnly(server.refreshing(
			func(r *http.Request) string { return chi.URLParam(r, "section") }, server.handleDiskStorageImport)))
		// LE LANCEMENT EST PARTI SUR LA PORTE UNIQUE (`POST /api/v1/launch`,
		// #195). `handleRetroLaunch` vit toujours : c'est l'aiguillage qui
		// l'appelle désormais. Le GET ci-dessous RESTE — même chemin, autre
		// verbe, et les confondre coûterait le listage des sessions.
		guarded.Get("/sessions", server.handleRetroList)
		guarded.Get("/{consoleId}/roms", server.handleRetroRoms)
		// LE STATUT DES JEUX (chantier 30, game_status.go) : un jeu, ou toute
		// sa console d'un geste. Administration seulement ; le lire est ouvert
		// à tous par la liste ci-dessus.
		guarded.Get("/{consoleId}/roms/{ref}/status", adminOnly(server.handleGameStatus))
		guarded.Post("/{consoleId}/roms/{ref}/status", adminOnly(server.handleReportGame))
		guarded.Get("/{consoleId}/status", adminOnly(server.handleGetConsoleStatus))
		guarded.Post("/{consoleId}/status", adminOnly(server.handleConsoleStatus))
		guarded.Get("/{consoleId}/bios", server.handleRetroBios)
		guarded.Get("/{consoleId}/cores", server.handleRetroCores)
		guarded.Get("/library/bios-dirs", server.handleRetroBiosDirs)
		// Gestion du stockage (storage_admin_api.go) — ce que la machine
		// contient et ce qui lui manque.
		guarded.Get("/storage/cores", adminOnly(server.handleStorageCores))
		guarded.Post("/storage/cores", adminOnly(server.refreshing(sectionNamed(listing.Cores), server.handleStorageCoreDownload)))
		// Supprimer un cœur ou un BIOS (#146) : sans ces deux routes, revenir
		// à une version qui marche demandait un accès shell — et le refus
		// d'écraser (#143) donnait une consigne que rien ne savait suivre.
		guarded.Delete("/storage/cores", adminOnly(server.refreshing(sectionNamed(listing.Cores), server.handleStorageCoreDelete)))
		guarded.Get("/storage/bios", adminOnly(server.handleStorageBios))
		guarded.Delete("/storage/bios", adminOnly(server.refreshing(sectionNamed(listing.Bios), server.handleStorageBiosDelete)))
		guarded.Get("/storage/logs", adminOnly(server.handleStorageLogs))
		guarded.Get("/storage/logs/tail", adminOnly(server.handleStorageLogTail))
		guarded.Get("/storage/logs/file", adminOnly(server.handleStorageLogFile))
		guarded.Delete("/storage/logs", adminOnly(server.handleStorageLogDelete))
		// SIGNALER, ce n'est pas LIRE (#191). Les quatre routes ci-dessus sont
		// réservées aux administrateurs parce qu'elles exposent les journaux de
		// tout le monde ; celle-ci n'ajoute qu'une ligne sur ce qui vient
		// d'échouer à celui qui la poste. La réserver aux administrateurs ne
		// laisserait de signalements que de celui qui n'en a pas besoin.
		guarded.Post("/storage/logs/report", server.handleStorageLogReport)
		guarded.Get("/storage/saves", adminOnly(server.handleStorageSaves))
		guarded.Get("/storage/saves/archive", adminOnly(server.handleStorageSavesArchive))
		guarded.Delete("/storage/saves", adminOnly(server.handleStorageSavesDelete))
		// Le cache du dossier (storage_cache_api.go, #448) : ce que le premier
		// lancement d'un jeu installe, montré et vidé entrée par entrée.
		guarded.Get("/storage/cache", adminOnly(server.handleStorageCache))
		guarded.Delete("/storage/cache", adminOnly(server.handleStorageCacheDelete))
		// Les paramètres des émulateurs (storage_params_api.go, #347) : les
		// mêmes routes que sur le téléphone.
		guarded.Get("/storage/params", adminOnly(server.handleStorageParams))
		// LE LISTING DE CE SERVEUR (chantier 15) : son en-tête — les dates —
		// pour les textes d'aide ; et le refaire, en entier (« Rescan all »)
		// ou une section (l'icône d'un sélecteur de source).
		guarded.Get("/listing", server.handleSelfListing)
		guarded.Post("/listing/rescan", adminOnly(server.handleSelfRescan))
		guarded.Put("/storage/params/{consoleId}/{emulator}", adminOnly(server.refreshing(sectionNamed(listing.Params), server.handleStorageParamsWrite)))
		guarded.Delete("/storage/params/{consoleId}/{emulator}/presets/{preset}", adminOnly(server.refreshing(sectionNamed(listing.Params), server.handleStorageParamsPresetDelete)))
		// Les correctifs (storage_patches_api.go, chantier 15) : ceux d'ici,
		// et en supprimer un.
		guarded.Get("/storage/patches", adminOnly(server.handleStoragePatches))
		guarded.Delete("/storage/patches", adminOnly(server.refreshing(sectionNamed(listing.Patches), server.handleStoragePatchDelete)))
		guarded.Get("/library/{section}", server.handleRetroLibrary)
		guarded.Get("/{consoleId}/roms/{ref}", server.handleRetroServeRom)
		guarded.Head("/{consoleId}/roms/{ref}", server.handleRetroServeRom)
		// L'ÉMISSION d'un ticket de téléchargement : ici, donc authentifiée.
		// C'est la seule moitié du mécanisme qui exige un compte — la
		// consommation se fait sans en-tête, par nécessité de plateforme.
		guarded.Post("/{consoleId}/roms/{ref}/ticket", server.handleRetroMintDownloadTicket)
		guarded.Get("/savestate/screenshot", server.handleRetroScreenshot)
		guarded.Get("/savestate/slots", server.handleRetroSaveSlots)
		guarded.Get("/savestate/user", server.handleRetroUserSaves)
		guarded.Delete("/savestate", server.handleRetroDeleteSave)
		guarded.Delete("/savestate/game", server.handleRetroDeleteGameSaves)
		// LE FICHIER, dans les deux sens — ce que la synchronisation d'une
		// sauvegarde vers un pair demandait (#181) ; l'envoi n'écrit que dans
		// une copie (chantier 38). Un segment statique l'emporte sur un
		// paramètre chez chi, donc `/savestate/file` ne peut pas être capté
		// ailleurs. Toutes ces routes prennent `?source=` : sans lui le
		// principal, avec lui cette copie (saves_api.go).
		guarded.Get("/savestate/file", server.handleRetroSaveFile)
		guarded.Post("/savestate/file", server.handleRetroSaveFileUpload)
		// SUPPRIMER UN JEU EST OUVERT AUX ADMINISTRATEURS depuis le
		// 2026-09-17, et la raison compte plus que la règle : c'est le
		// RAPATRIEMENT qui l'a rendu réversible.
		//
		// La règle inverse était écrite ici, et elle était juste tant
		// qu'elle a duré. La racine froide était montée en lecture seule,
		// puis, passée en écriture le 2026-09-09, ce commentaire est resté
		// le seul gardien. Son argument : une suppression n'a pas de retour
		// en arrière. Ce n'est plus vrai — on sait reprendre un jeu chez un
		// serveur fédéré ou une ressource distante, et le rapatriement le
		// range tout seul dans le bon dossier. Effacer pour faire de la
		// place redevient donc un geste normal d'administration.
		//
		// CE QUI N'A PAS CHANGÉ : un rôle `user` n'écrit ni n'efface jamais
		// dans la bibliothèque. Le disque est celui de tout le monde, d'où
		// `adminOnly` et non la simple authentification de ses voisines.
		guarded.Delete("/{consoleId}/roms/{ref}", adminOnly(server.refreshing(sectionNamed(listing.Roms), server.handleRetroDeleteRom)))
	})

	return router
}
