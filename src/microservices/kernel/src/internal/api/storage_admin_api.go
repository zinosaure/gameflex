package api

// GESTION DU STOCKAGE — ce que la machine contient, vu par un administrateur.
//
// Cinq sections, et elles ne se ressemblent pas :
//
//	Jeux (roms)         ce qu'on a mis là — on regarde, on ne touche pas
//	Émulateurs (cores)  ce qu'il FAUT avoir, et ce qui manque se télécharge
//	Firmwares (bios)    ce qu'il faut avoir aussi, mais sans fournisseur
//	Sauvegardes (saves)  par joueur — l'admin prend le dossier ou le supprime
//	Journaux (logs)      lire les cent dernières lignes, garder, jeter
//
// La « mémoire temporaire » (le cache des jeux joués à distance) n'en est plus
// une depuis le chantier 14 (2026-10-12) : un jeu n'entre plus ici que par la
// bibliothèque.
//
// Le point commun des deux du milieu : la liste ne vient PAS du disque, elle
// vient du REGISTRE partagé (consoles.Cores, consoles.Bios). Un dossier vide
// doit se lire « rien d'installé » et pas « cette console n'existe pas » —
// c'est justement quand il manque quelque chose que cet écran sert.
//
// Réservé aux ADMINISTRATEURS, en lecture comme en écriture : ces listes
// disent ce que la machine n'a pas, et les journaux portent des noms de
// comptes et des chemins.

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gameflex/storage/listing"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"gameflex/consoles"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/retro_files"
	"gameflex/logfile"
)

// maxReportBytes — un signalement décrit un écran d'étapes, pas un fichier.
// Borné parce que ce corps arrive d'un navigateur et finit sur le disque.
const maxReportBytes = 64 << 10

// reportsPerHour — ce qu'un compte signale en une heure. Un écran d'échec en
// envoie un par échec : soixante, c'est une partie qui échoue chaque minute.
// Sans plafond, un compte remplissait le disque de signalements (#601).
const reportsPerHour = 60

// reportBudget — les signalements de chaque compte dans la dernière heure.
// Zéro-valeur utilisable, comme launchProgress.
type reportBudget struct {
	mu   sync.Mutex
	sent map[string][]time.Time
}

// allow — ce compte peut-il signaler encore, maintenant ? Compte l'envoi s'il
// passe.
func (budget *reportBudget) allow(username string, now time.Time) bool {
	budget.mu.Lock()
	defer budget.mu.Unlock()

	if budget.sent == nil {
		budget.sent = map[string][]time.Time{}
	}
	recent := []time.Time{}
	for _, at := range budget.sent[username] {
		if now.Sub(at) < time.Hour {
			recent = append(recent, at)
		}
	}
	if len(recent) >= reportsPerHour {
		budget.sent[username] = recent

		return false
	}
	budget.sent[username] = append(recent, now)

	return true
}

// savesDeletionTracker — quelles suppressions de sauvegardes tournent, et
// comment la dernière s'est terminée.
//
// POURQUOI EN FOND. `os.RemoveAll` sur le dossier d'un joueur, c'est ici
// 29 Go et 76 000 fichiers : des MINUTES. La requête restait pendue tout ce
// temps, le navigateur du téléphone abandonnait, et la relecture qui suit
// affichait « impossible de lire cette section » sur une liste vide — signalé
// en vrai le 2026-09-18, avec la capture d'écran. Le travail n'avait pourtant
// rien échoué : il n'avait pas fini.
//
// Même forme que `listing.Rescans` (les parcours de ressource), qui répond à
// exactement le même besoin : un travail long, lancé en fond, dont l'état se
// lit. Clé par nom de compte au lieu d'un identifiant numérique, c'est la
// seule différence.
type savesDeletionTracker struct {
	mu      sync.Mutex
	running map[string]bool
	failed  map[string]string
}

// start réserve la suppression d'un compte. Rend faux si elle tourne déjà —
// deux clics ne lancent pas deux effacements du même dossier.
func (tracker *savesDeletionTracker) start(user string) bool {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	if tracker.running == nil {
		tracker.running, tracker.failed = map[string]bool{}, map[string]string{}
	}
	if tracker.running[user] {
		return false
	}
	tracker.running[user] = true
	delete(tracker.failed, user)

	return true
}

func (tracker *savesDeletionTracker) finish(user string, err error) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	delete(tracker.running, user)
	if err != nil {
		tracker.failed[user] = err.Error()
	}
}

// state dit si une suppression tourne pour ce compte, et ce que la précédente
// a laissé comme erreur.
func (tracker *savesDeletionTracker) state(user string) (running bool, failure string) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	return tracker.running[user], tracker.failed[user]
}

// coreDownloadTimeout — un core pèse quelques mégaoctets et le buildbot est
// parfois lent. Assez large pour ne pas couper une descente qui avance,
// assez court pour qu'un écran ne reste pas suspendu à un serveur muet.
const coreDownloadTimeout = 3 * time.Minute

// logTailLines — ce qu'on montre d'un journal quand on l'ouvre.
//
// La FIN, jamais le début : on ouvre cet écran après qu'une chose vient de
// mal se passer. Le fichier entier se télécharge, pour qui veut le reste.
const logTailLines = 100

// =========================================================================
// Émulateurs (cores)
// =========================================================================

// coreBuildbotURL — d'où vient un core qui manque.
//
// Le même fournisseur que le téléphone (Cores.java), à l'architecture près :
// android/<abi> là-bas, linux/x86_64 ici. Les deux plateformes descendent le
// même .zip d'une seule bibliothèque, et c'est voulu — un serveur qui
// hébergerait sa propre copie des cores serait une deuxième chose à tenir à
// jour pour rendre ce que le buildbot rend déjà.
func coreBuildbotURL(core string) string {
	return coreBuildbotBase + retro_files.CoreFileName(core) + ".zip"
}

// coreBuildbotBase — une variable et non une constante, pour qu'un test vise
// un faux buildbot.
var coreBuildbotBase = "https://buildbot.libretro.com/nightly/linux/x86_64/latest/"

// coreAndroidBuildbotURL — la variante Android du même cœur, pour les
// téléphones de ce serveur (chantier 13, phase 5). arm64 seulement : c'est
// l'architecture des téléphones ; l'émulateur x86_64 prend les siens à la
// source officielle.
func coreAndroidBuildbotURL(core string) string {
	return coreAndroidBuildbotBase + retro_files.AndroidCoreFileName(core) + ".zip"
}

var coreAndroidBuildbotBase = "https://buildbot.libretro.com/nightly/android/latest/arm64-v8a/"

// consoleSection — une console et ce qu'elle réclame.
type consoleSection struct {
	ConsoleID string      `json:"consoleId"`
	Console   string      `json:"console"`
	Items     []storeItem `json:"items"`
}

// storeItem — une ligne de l'écran : ce qu'il faut, et si on l'a.
type storeItem struct {
	Name      string `json:"name"`
	File      string `json:"file"`
	SizeBytes int64  `json:"sizeBytes"`
	Installed bool   `json:"installed"`
	// Scoped — le fichier est-il rangé sous sa console (cores/<id>/…) ou
	// posé à plat avec tous les autres ? Les deux se chargent ; seul le
	// premier se lit. L'écran le montre pour que la pile à plat, qui ne dit
	// plus à quoi elle sert, se voie et se résorbe.
	Scoped bool `json:"scoped"`
	// Source — où aller le chercher quand il manque. Vide pour un BIOS :
	// personne ne les distribue, et proposer un bouton qui ne mène nulle
	// part serait pire que de ne rien proposer.
	Source string `json:"source,omitempty"`
	// La variante Android d'un cœur, pour les téléphones (chantier 13,
	// phase 5). Un pointeur : vrai OU faux pour un cœur — l'écran propose de
	// prendre celle qui manque —, absent pour un BIOS et sur le téléphone,
	// dont les cœurs SONT la version Android.
	AndroidInstalled *bool `json:"androidInstalled,omitempty"`
	AndroidSizeBytes int64 `json:"androidSizeBytes,omitempty"`
	// Package — un FICHIER d'émulateur de bureau (chantier 29), et non un cœur
	// du registre : « server » (l'archive de l'éditeur, notre construction) ou
	// « phone » (un paquet du téléphone). Son nom est celui du fichier, version
	// comprise ; il n'existe que là où il est rangé, jamais « absent ».
	Package string `json:"package,omitempty"`
	// Tested — pour le fichier d'un émulateur de console récente : est-ce une
	// version éprouvée avec l'image installée (docker.RequiredEmulator) ? La
	// liste informe, elle ne bloque pas. Absent pour le reste.
	Tested *bool `json:"tested,omitempty"`
	// InUse — c'est lui qu'une partie prendrait : le dernier déposé parmi les
	// versions que l'image reconnaît.
	InUse bool `json:"inUse,omitempty"`
}

// coresSections — les cores de CETTE machine, console par console.
//
// Extrait du handler pour que la surface fédérée serve exactement la même
// forme : un pair et nous-mêmes ne doivent pas construire deux listes
// séparément, sinon l'écran d'administration afficherait deux dialectes
// selon la source choisie. Seuls les serveurs fédérés peuvent fournir des
// cores et des BIOS — un montage Samba/WebDAV/kDrive ne porte que des ROMs.
func (server *Server) coresSections(ctx context.Context) []consoleSection {
	// LE FICHIER QU'ATTEND CHAQUE IMAGE (chantier 29), manquant ou copié à la
	// main depuis le dernier parcours : le listing ne le connaît pas encore.
	sections := coresSectionsWith(server.selfSectionAll(listing.Cores), func(consoleID string, listed map[string]bool) []storeItem {
		return server.requiredEmulatorItems(ctx, consoleID, listed)
	})
	server.markEmulatorFiles(ctx, sections)

	return sections
}

// coresSectionsFrom — les cœurs du registre, console par console, et ce
// qu'un listing en a : celui de ce serveur, ou la copie de celui d'un ami.
func coresSectionsFrom(present map[string][]listing.Item) []consoleSection {
	return coresSectionsWith(present, nil)
}

// coresSectionsWith — la même chose, plus les lignes que `more` ajoute à une
// console, sachant celles qu'elle a déjà. Avant de juger une console vide : la
// PS5 n'a aucun cœur libretro, et seule cette ligne-là la fait apparaître.
func coresSectionsWith(present map[string][]listing.Item, more func(consoleID string, listed map[string]bool) []storeItem) []consoleSection {
	sections := []consoleSection{}
	for _, console := range consoles.All() {
		entries := coreEntriesFrom(console.ID, present[console.ID])
		items := make([]storeItem, 0, len(entries))
		for _, entry := range entries {
			android := entry.AndroidInstalled
			items = append(items, storeItem{
				Name:             entry.Name,
				File:             entry.File,
				SizeBytes:        entry.SizeBytes,
				Installed:        entry.Installed,
				Scoped:           entry.Scoped,
				Source:           coreBuildbotURL(entry.Name),
				AndroidInstalled: &android,
				AndroidSizeBytes: entry.AndroidSizeBytes,
			})
		}
		// Les fichiers des émulateurs de bureau, tels que le listing les a.
		for _, item := range present[console.ID] {
			kind := ""
			if _, phone := item.Variants["bundle"]; phone {
				kind = "phone"
			} else if _, server := item.Variants["emulator"]; server {
				kind = "server"
			}
			if kind == "" {
				continue
			}
			items = append(items, storeItem{
				Name: item.Name, File: item.Name, SizeBytes: item.Size, Installed: true, Scoped: true, Package: kind,
			})
		}
		if more != nil {
			listed := map[string]bool{}
			for _, item := range items {
				listed[item.Name] = true
			}
			items = append(items, more(console.ID, listed)...)
		}
		if len(items) == 0 {
			continue
		}
		sections = append(sections, consoleSection{
			ConsoleID: console.ID, Console: console.Name, Items: items,
		})
	}

	return sections
}

// handleStorageCores — GET /api/v1/retrogaming/storage/cores.
func (server *Server) handleStorageCores(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"consoles": server.coresSections(r.Context())})
}

// handleStorageCoreDownload — POST /api/v1/retrogaming/storage/cores {consoleId, core}.
//
// Le fichier atterrit dans cores/<consoleId>/, jamais à plat : deux cents
// fichiers côte à côte ne disent plus à quoi ils servent. Un core qui sert
// plusieurs consoles est déposé sous chacune — c'est quelques mégaoctets
// contre un rangement qui se lit.
func (server *Server) handleStorageCoreDownload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConsoleID string `json:"consoleId"`
		Core      string `json:"core"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corps illisible")

		return
	}
	body.ConsoleID = strings.ToLower(strings.TrimSpace(body.ConsoleID))
	body.Core = strings.TrimSpace(body.Core)

	// LE FICHIER QU'ATTEND L'IMAGE D'UNE CONSOLE RÉCENTE (chantier 29) : il se
	// désigne par son nom, et descend chez son éditeur.
	if server.downloadRequiredEmulator(w, r, body.ConsoleID, body.Core) {
		return
	}

	// Le core est vérifié CONTRE LE REGISTRE, pas contre son allure : c'est
	// ce nom qui devient un chemin sur le disque et un segment d'URL.
	if !coreBelongsTo(body.ConsoleID, body.Core) {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("le core « %s » n'est pas déclaré pour %s", body.Core, body.ConsoleID))

		return
	}

	target := filepath.Join(server.retroCoresRoot(), body.ConsoleID, retro_files.CoreFileName(body.Core))
	androidTarget := filepath.Join(server.retroCoresRoot(), body.ConsoleID, retro_files.AndroidCoreFileName(body.Core))
	linuxInfo, linuxErr := os.Stat(target)
	_, androidErr := os.Stat(androidTarget)

	// UN CŒUR PRÉSENT NE S'ÉCRASE PAS (#143).
	//
	// Ce bouton visait `latest/` du buildbot et remplaçait le fichier sans
	// rien demander. Or « le dernier » n'est pas « celui qui marche » : le
	// dolphin de nightly exige `GLIBCXX_3.4.32` quand les deux images de
	// session fournissent `3.4.30`, si bien qu'un remplacement rend la console
	// injouable — et l'ancien fichier, lui, n'existe plus.
	//
	// Épingler l'adresse était le geste évident (c'est ce que #137 a fait pour
	// xemu), mais il n'est pas disponible ici : libretro ne publie **que**
	// `latest/` — les répertoires datés répondent 404, et son index est un
	// navigateur JavaScript sans listage à lire. La seule protection qui ne
	// dépend pas d'eux est donc celle-ci : refuser, et dire quoi faire.
	//
	// Même règle et même mot que pour un rapatriement depuis un pair, qui
	// refuse déjà par un 409 : un seul comportement à apprendre.
	//
	// Le cœur, c'est maintenant SES DEUX VARIANTES (chantier 13, phase 5) :
	// un serveur qui a la version Linux sans la version Android descend
	// celle qui manque, et rien d'autre.
	if linuxErr == nil && androidErr == nil {
		writeError(w, http.StatusConflict,
			"ce cœur existe déjà : le supprimer d'abord pour en reprendre une version — "+
				"« le dernier » du buildbot n'est pas toujours celui qui marche")

		return
	}

	var written int64
	if linuxErr == nil {
		written = linuxInfo.Size()
	} else {
		var err error
		written, err = downloadCore(r.Context(), coreBuildbotURL(body.Core), target)
		if err != nil {
			// UNE INCOMPATIBILITÉ N'EST PAS UNE PANNE. Le fichier est arrivé
			// entier ; il est seulement trop récent pour l'image de session.
			// 412 plutôt que 502, et le message part tel quel à l'écran —
			// l'écran affiche `errorMessage`, donc l'administrateur lit quelle
			// version le cœur exige et laquelle la machine fournit, au lieu
			// d'un « échec ».
			if errors.Is(err, errCoreIncompatible) {
				writeError(w, http.StatusPreconditionFailed, err.Error())

				return
			}
			writeError(w, http.StatusBadGateway, err.Error())

			return
		}
	}
	answer := map[string]any{"core": body.Core, "consoleId": body.ConsoleID, "sizeBytes": written}

	// LA VARIANTE ANDROID, EN PLUS : son absence chez libretro (un cœur qui
	// n'existe pas pour Android) ne défait pas la version Linux qu'on vient
	// d'installer — elle se dit dans la réponse. Seule, elle est l'échec.
	if androidErr != nil {
		androidWritten, err := downloadAndroidCore(r.Context(), coreAndroidBuildbotURL(body.Core), androidTarget)
		switch {
		case err != nil && linuxErr == nil && errors.Is(err, errCoreIncompatible):
			writeError(w, http.StatusPreconditionFailed, "version Android : "+err.Error())

			return
		case err != nil && linuxErr == nil:
			writeError(w, http.StatusBadGateway, "version Android : "+err.Error())

			return
		case err != nil:
			answer["androidError"] = err.Error()
		default:
			answer["androidSizeBytes"] = androidWritten
		}
	}

	writeJSON(w, http.StatusOK, answer)
}

// handleStorageCoreDelete — DELETE /api/v1/retrogaming/storage/cores?consoleId=&core=
//
// POURQUOI CETTE ROUTE EXISTE (#146). Sans elle, remplacer un cœur demandait un
// accès shell : le noyau tourne en root, et les dossiers qu'il avait créés
// n'appartenaient plus à personne d'autre. La feuille de route notait qu'une
// action d'administration est « la seule qui serve aussi l'utilisateur qui n'a
// pas la main sur la machine » — c'est celle-ci.
//
// Et elle rend exécutable ce que le téléchargement dit déjà : il refuse
// d'écraser un cœur en place (#143) en demandant de « le supprimer d'abord ».
// Une consigne que le produit ne savait pas suivre.
//
// Réservée aux administrateurs, comme ses voisines : le disque est celui de
// tout le monde.
func (server *Server) handleStorageCoreDelete(w http.ResponseWriter, r *http.Request) {
	consoleID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("consoleId")))
	core := strings.TrimSpace(r.URL.Query().Get("core"))

	// UN FICHIER D'ÉMULATEUR DE BUREAU (chantier 29) se désigne par son nom :
	// c'est ainsi qu'une ancienne version quitte la liste, et la fédération.
	if retro_files.IsEmulatorFile(consoleID, core) {
		server.removeLibraryFile(w, filepath.Join(server.retroCoresRoot(), consoleID, core),
			map[string]any{"consoleId": consoleID, "core": core})

		return
	}

	if !coreBelongsTo(consoleID, core) {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("le core « %s » n'est pas déclaré pour %s", core, consoleID))

		return
	}

	// LES DEUX VARIANTES PARTENT ENSEMBLE (chantier 13, phase 5) : un cœur se
	// supprime pour en reprendre une version, et la variante Android doit
	// suivre — sinon les téléphones garderaient l'ancienne.
	removed := false
	for _, name := range []string{retro_files.CoreFileName(core), retro_files.AndroidCoreFileName(core)} {
		target := filepath.Join(server.retroCoresRoot(), consoleID, name)
		if _, err := os.Stat(target); err != nil {
			continue
		}
		if err := os.Remove(target); err != nil {
			writeError(w, http.StatusInternalServerError, "suppression impossible : "+err.Error())

			return
		}
		removed = true
	}
	if !removed {
		writeError(w, http.StatusNotFound, "ce fichier n'est pas installé ici")

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"consoleId": consoleID, "core": core, "removed": true})
}

// handleStorageBiosDelete — DELETE /api/v1/retrogaming/storage/bios?consoleId=&name=
//
// Le pendant pour un firmware. Un BIOS ne se télécharge pas — personne ne les
// distribue — mais il peut être MAUVAIS : une région qui ne correspond pas, un
// fichier tronqué. Le retirer était le seul geste qui manquait pour que l'écran
// serve à quelque chose au-delà du constat.
func (server *Server) handleStorageBiosDelete(w http.ResponseWriter, r *http.Request) {
	consoleID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("consoleId")))
	name := strings.TrimSpace(r.URL.Query().Get("name"))

	if !biosBelongsTo(consoleID, name) {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("le BIOS « %s » n'est pas attendu par %s", name, consoleID))

		return
	}

	server.removeLibraryFile(w,
		filepath.Join(server.retroBiosRoot(), consoleID, name),
		map[string]any{"consoleId": consoleID, "name": name})
}

// removeLibraryFile — retirer UN fichier de la bibliothèque, et le dire.
//
// Le chemin est déjà passé par le registre chez l'appelant : c'est ce qui
// garantit qu'aucun nom venu d'une requête ne devient un chemin arbitraire.
// Ici il ne reste qu'à distinguer « il n'était pas là » de « je n'ai pas pu ».
func (server *Server) removeLibraryFile(w http.ResponseWriter, target string, answer map[string]any) {
	if _, err := os.Stat(target); err != nil {
		writeError(w, http.StatusNotFound, "ce fichier n'est pas installé ici")

		return
	}
	if err := os.Remove(target); err != nil {
		writeError(w, http.StatusInternalServerError, "suppression impossible : "+err.Error())

		return
	}

	answer["removed"] = true
	writeJSON(w, http.StatusOK, answer)
}

// coreBelongsTo — ce core est-il déclaré pour cette console ?
func coreBelongsTo(consoleID, core string) bool {
	if consoleID == "" || core == "" {
		return false
	}
	for _, declared := range consoles.CoresFor(consoleID) {
		if declared == core {
			return true
		}
	}

	return false
}

// downloadCore descend l'archive et en extrait la bibliothèque.
//
// Fichier temporaire puis renommage, comme sur le téléphone : une coupure
// réseau laisserait sinon un .so tronqué que le prochain lancement croirait
// valide, et qui échouerait au chargement sans dire pourquoi.
func downloadCore(ctx context.Context, url, target string) (int64, error) {
	return downloadCoreFor(ctx, url, target, "linux/x86_64", verifyCoreABI)
}

// downloadAndroidCore — la variante Android, contrôlée comme une bibliothèque
// arm64 (verifyAndroidCoreABI).
func downloadAndroidCore(ctx context.Context, url, target string) (int64, error) {
	return downloadCoreFor(ctx, url, target, "android/arm64-v8a", verifyAndroidCoreABI)
}

func downloadCoreFor(ctx context.Context, url, target, platform string, verify func(string) error) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, fmt.Errorf("dossier impossible à créer : %w", err)
	}
	// Le dossier de console aussi : c'est lui qui devenait `root:root` et que
	// la personne ne pouvait plus gérer (#146).
	adoptParentOwner(filepath.Dir(target))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: coreDownloadTimeout}
	response, err := client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("libretro injoignable : %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return 0, fmt.Errorf("ce core n'existe pas chez libretro pour %s", platform)
	}
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("libretro a répondu %d", response.StatusCode)
	}

	// zip.NewReader veut pouvoir revenir en arrière — l'archive passe donc
	// par un fichier, pas par le flux.
	archive, err := os.CreateTemp(filepath.Dir(target), ".core-*.zip")
	if err != nil {
		return 0, err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()

	size, err := io.Copy(archive, response.Body)
	if err != nil {
		return 0, fmt.Errorf("téléchargement interrompu : %w", err)
	}

	reader, err := zip.NewReader(archive, size)
	if err != nil {
		return 0, fmt.Errorf("archive illisible : %w", err)
	}

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || !strings.HasSuffix(entry.Name, ".so") {
			continue
		}
		if err := fitsOnDisk(filepath.Dir(target), entry.UncompressedSize64); err != nil {
			return 0, err
		}
		// LE CONTRÔLE D'ABI EST ICI, et nulle part ailleurs : c'est le seul
		// chemin par lequel un cœur du buildbot entre sur ce disque.
		return extractCore(entry, target, verify)
	}

	return 0, errors.New("archive sans bibliothèque")
}

// diskMargin — ce qu'une archive dépliée laisse libre sur le disque, au moins.
const diskMargin = 1 << 30

// fitsOnDisk — `size` octets tiennent-ils encore dans le dossier `dir`, en y
// laissant `diskMargin` ? La taille est celle qu'ANNONCE l'archive : une
// archive piégée (quelques Ko qui se déplient en téraoctets) remplissait le
// disque, rien ne bornait la décompression (revue de sécurité du kernel,
// #601). L'annonce suffit : archive/zip refuse de rendre plus d'octets
// qu'une entrée n'en déclare.
func fitsOnDisk(dir string, size uint64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return fmt.Errorf("place libre illisible : %w", err)
	}
	free := stat.Bavail * uint64(stat.Bsize)
	if size > free || free-size < diskMargin {
		return fmt.Errorf("l'archive se déplie en %d Mo, et le disque n'en a que %d de libres",
			size>>20, free>>20)
	}

	return nil
}

// extractCore écrit UNE entrée d'archive, sans jamais laisser de demi-fichier.
//
// `verify` est appelé sur le fichier TEMPORAIRE, avant le renommage : c'est le
// seul moment où l'on peut refuser sans avoir rien installé. `nil` quand il n'y
// a rien à vérifier — les jeux et les BIOS passent par ici aussi (voir
// `expandZipInto`), et un contrôle d'ABI n'aurait aucun sens sur une ROM.
func extractCore(entry *zip.File, target string, verify func(string) error) (int64, error) {
	source, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer source.Close()

	partial := target + ".part"
	out, err := os.Create(partial)
	if err != nil {
		return 0, fmt.Errorf("écriture impossible : %w", err)
	}
	written, err := io.Copy(out, source)
	out.Close()
	if err != nil {
		os.Remove(partial)

		return 0, err
	}
	if verify != nil {
		if err := verify(partial); err != nil {
			os.Remove(partial)

			return 0, err
		}
	}
	if err := os.Rename(partial, target); err != nil {
		os.Remove(partial)

		return 0, err
	}
	// Le fichier est à la personne, pas au noyau qui l'a descendu (#146).
	adoptParentOwner(target)

	return written, nil
}

// =========================================================================
// Firmwares (bios)
// =========================================================================

// handleStorageBios — GET /api/v1/retrogaming/storage/bios.
//
// Listing seul : aucun fournisseur légitime ne distribue ces fichiers, donc
// aucun bouton de téléchargement. L'écran dit ce qui manque et où le poser,
// et c'est déjà ce qui manquait — un BIOS absent se manifestait jusqu'ici par
// un émulateur qui refuse de démarrer, sans dire lequel il cherchait.
func (server *Server) handleStorageBios(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"consoles": server.biosSections()})
}

// biosSections — même extraction que coresSections, et pour la même raison :
// la surface fédérée doit rendre cette liste-ci, pas une variante.
func (server *Server) biosSections() []consoleSection {
	return biosSectionsFrom(server.selfSectionAll(listing.Bios))
}

// biosSectionsFrom — les BIOS que le registre réclame, console par console, et
// ceux qu'un listing dit présents.
func biosSectionsFrom(present map[string][]listing.Item) []consoleSection {
	sections := []consoleSection{}
	for _, console := range consoles.All() {
		wanted := consoles.BiosFor(console.ID)
		if len(wanted) == 0 {
			continue
		}
		have := map[string]listing.Item{}
		for _, item := range present[console.ID] {
			have[item.Name] = item
		}
		items := make([]storeItem, 0, len(wanted))
		for _, name := range wanted {
			item := storeItem{Name: name, File: name}
			if found, ok := have[name]; ok && !found.Dir {
				item.Installed, item.Scoped, item.SizeBytes = true, true, found.Size
			}
			items = append(items, item)
		}
		sections = append(sections, consoleSection{
			ConsoleID: console.ID, Console: console.Name, Items: items,
		})
	}

	return sections
}

// =========================================================================
// Journaux (logs)
// =========================================================================

func (server *Server) logsRoot() string {
	// Les journaux vivent dans la racine CHAUDE, à côté des sauvegardes —
	// ce sont des choses que la machine PRODUIT, pas des choses qu'on y a
	// déposées.
	return filepath.Dir(server.retroSavesRoot())
}

// handleStorageLogs — GET /api/v1/retrogaming/storage/logs.
func (server *Server) handleStorageLogs(w http.ResponseWriter, r *http.Request) {
	entries, err := logfile.List(server.logsRoot())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"logs": entries})
}

// handleStorageLogTail — GET /api/v1/retrogaming/storage/logs/tail?path=…
func (server *Server) handleStorageLogTail(w http.ResponseWriter, r *http.Request) {
	content, err := logfile.Tail(server.logsRoot(), r.URL.Query().Get("path"), logTailLines)
	if err != nil {
		writeLogError(w, err)

		return
	}

	// Découpé, pas brut : le message en titre, l'heure et le niveau
	// dessous. Un bloc de JSON rendu tel quel redonne à l'écran ce qu'un
	// terminal donne déjà, et se replie sur quatre lignes par entrée sur un
	// téléphone. `content` reste rendu pour ce qui voudrait le texte.
	writeJSON(w, http.StatusOK, map[string]any{
		"lines":   logTailLines,
		"content": content,
		"entries": logfile.ParseTail(content),
	})
}

// handleStorageLogFile — GET /api/v1/retrogaming/storage/logs/file?path=…
func (server *Server) handleStorageLogFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(path, "/", "-")))

	// `WriteRedacted` et NON `http.ServeContent` : servir le fichier tel
	// qu'il est sur le disque rendrait les jetons que la lecture masque.
	// Un téléchargement est la porte la plus large des deux — il emporte le
	// journal entier, pas ses cent dernières lignes.
	if err := logfile.WriteRedacted(w, server.logsRoot(), path); err != nil {
		// L'en-tête est déjà parti : on ne peut plus changer le code de
		// statut, seulement dire dans le corps que le fichier est tronqué.
		fmt.Fprintf(w, "\n[lecture interrompue : %v]\n", err)
	}
}

// handleStorageLogDelete — DELETE /api/v1/retrogaming/storage/logs?path=…
func (server *Server) handleStorageLogDelete(w http.ResponseWriter, r *http.Request) {
	if err := logfile.Remove(server.logsRoot(), r.URL.Query().Get("path")); err != nil {
		writeLogError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// handleStorageLogReport — POST /api/v1/retrogaming/storage/logs/report (#191).
//
// LE SEUL POINT D'ÉCRITURE DE JOURNAL DEPUIS LA PAGE, et le seul de cette
// famille qui ne soit PAS réservé aux administrateurs. La distinction se tient :
// LIRE les journaux, c'est lire qui joue à quoi et quand ; SIGNALER un échec
// qu'on vient de subir, non. Réserver ce geste aux administrateurs ne laisserait
// de signalements que de celui qui n'en a pas besoin.
//
// Le nom vient du JETON et jamais du corps (doctrine de retro_api.go) : un
// signalement désigne quelqu'un, et se laisser dicter qui serait offrir une
// signature.
func (server *Server) handleStorageLogReport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Context string           `json:"context"`
		Steps   []map[string]any `json:"steps"`
		Detail  map[string]any   `json:"detail"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxReportBytes)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "unreadable report: "+err.Error())

		return
	}
	if !server.reports.allow(auth.Username(r.Context()), time.Now()) {
		writeError(w, http.StatusTooManyRequests, "too many reports: try again later")

		return
	}

	file, err := logfile.Reports(server.logsRoot())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}
	// NE PAS RÉPONDRE « ENREGISTRÉ » QUAND RIEN NE L'EST. `logfile.open` rend
	// (nil, nil) quand l'écriture est éteinte — un succès muet ici ferait
	// croire à un signalement envoyé, et c'est précisément le genre de mensonge
	// qu'un écran d'erreur ne doit pas produire.
	if file == nil {
		writeError(w, http.StatusServiceUnavailable, "file logs are disabled on this machine")

		return
	}

	defer file.Close()

	encoded, err := json.Marshal(map[string]any{
		"at":       time.Now().UTC().Format(time.RFC3339),
		"username": auth.Username(r.Context()),
		"context":  body.Context,
		"steps":    body.Steps,
		"detail":   body.Detail,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	if _, err := file.Write(append(encoded, '\n')); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"reported": true})
}

// writeLogError distingue « tu n'as rien à faire là » de « ce fichier n'existe
// pas » — deux causes, deux corrections, et les confondre ferait chercher un
// bug de permission là où un journal a simplement été supprimé.
func writeLogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, logfile.ErrOutsideRoot):
		writeError(w, http.StatusBadRequest, "chemin hors du dossier des journaux")
	case os.IsNotExist(err):
		writeError(w, http.StatusNotFound, "journal introuvable")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// =========================================================================
// Sauvegardes (saves)
// =========================================================================

// userSaves — le dossier d'un joueur, vu de loin.
//
// De LOIN, volontairement : le joueur gère ses sauvegardes une par une dans
// « Mes sauvegardes ». Ici, l'administrateur voit de la place occupée par un
// compte, et n'a que deux gestes — l'emporter, ou la reprendre.
type userSaves struct {
	Username   string    `json:"username"`
	Files      int       `json:"files"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
	// Deleting — un effacement tourne en fond pour ce compte. Sans ce mot,
	// l'écran montrerait une taille intacte pendant les minutes que dure la
	// suppression, et on cliquerait une seconde fois en croyant que rien ne
	// s'est passé.
	Deleting bool `json:"deleting,omitempty"`
}

// handleStorageSaves — GET /api/v1/retrogaming/storage/saves.
func (server *Server) handleStorageSaves(w http.ResponseWriter, r *http.Request) {
	root := server.retroSavesRoot()
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	users := []userSaves{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		user := userSaves{Username: entry.Name()}
		_ = filepath.Walk(filepath.Join(root, entry.Name()),
			func(_ string, info os.FileInfo, walkErr error) error {
				if walkErr != nil || info == nil || info.IsDir() {
					return nil //nolint:nilerr
				}
				user.Files++
				user.SizeBytes += info.Size()
				if info.ModTime().After(user.ModifiedAt) {
					user.ModifiedAt = info.ModTime().UTC()
				}

				return nil
			})
		user.Deleting, _ = server.savesDeletions.state(entry.Name())
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].SizeBytes > users[j].SizeBytes })

	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// saveUserDir résout le dossier d'un joueur, en refusant tout ce qui n'est
// pas un simple nom.
//
// Le nom vient d'une requête HTTP, et il sert ici à choisir un dossier à
// EMPORTER puis à SUPPRIMER. « ../databases » y passerait sans cette garde.
func (server *Server) saveUserDir(username string) (string, error) {
	if username == "" || username != filepath.Base(username) ||
		username == "." || username == ".." || strings.ContainsAny(username, `/\`) {
		return "", errors.New("nom de compte invalide")
	}

	return filepath.Join(server.retroSavesRoot(), username), nil
}

// handleStorageSavesArchive — GET /api/v1/retrogaming/storage/saves/archive?user=…
//
// Une archive, et pas un fichier à la fois : le geste offert ici est « tout
// le dossier », et le rendre en cent requêtes serait rendre autre chose.
func (server *Server) handleStorageSavesArchive(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("user")
	dir, err := server.saveUserDir(username)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		writeError(w, http.StatusNotFound, "ce compte n'a pas de sauvegardes")

		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "saves-"+username+".zip"))

	archive := zip.NewWriter(w)
	defer archive.Close()

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			return nil //nolint:nilerr
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return nil //nolint:nilerr
		}
		source, err := os.Open(path)
		if err != nil {
			return nil //nolint:nilerr
		}
		defer source.Close()

		entry, err := archive.Create(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		_, err = io.Copy(entry, source)

		return err
	})
}

// handleStorageSavesDelete — DELETE /api/v1/retrogaming/storage/saves?user=…
func (server *Server) handleStorageSavesDelete(w http.ResponseWriter, r *http.Request) {
	user := r.URL.Query().Get("user")
	dir, err := server.saveUserDir(user)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}

	// ON RÉPOND TOUT DE SUITE, ON EFFACE EN FOND — voir savesDeletionTracker
	// pour la raison : des minutes d'attente sur un gros dossier, et un
	// navigateur qui abandonne avant la fin.
	//
	// Déjà en cours : on le dit sans relancer. Un second clic sur une
	// suppression qui travaille ne doit pas lancer un second effacement du
	// même dossier.
	if !server.savesDeletions.start(user) {
		writeJSON(w, http.StatusOK, map[string]any{"started": true, "alreadyRunning": true})

		return
	}

	go func() {
		removeErr := os.RemoveAll(dir)
		server.savesDeletions.finish(user, removeErr)

		// JOURNALISÉ DES DEUX CÔTÉS. La requête est déjà partie quand ce
		// travail se termine : sans ces deux lignes, une suppression qui
		// échoue en fond ne laisserait aucune trace nulle part.
		if removeErr != nil {
			slog.Error("saves deletion failed", "user", user, "error", removeErr)

			return
		}
		slog.Info("saves deleted", "user", user)
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{"started": true})
}

// adminOnly — ce qui touche à la MACHINE ne se fait pas depuis n'importe quel
// compte : ses listes disent ce qu'elle n'a pas, ses écritures la changent pour
// tous ceux qui y jouent.
// localOnly — UNE SESSION OUVERTE PAR LE JETON D'UN COMPTE, DEPUIS UN AUTRE
// SERVEUR (chantier 20), JOUE ICI, ET RIEN D'AUTRE. auth.Federated le
// promettait, et une seule route le vérifiait (PUT servers/{id}/account) :
// la session d'un ami pouvait lister nos serveurs fédérés, demander une
// session chez un troisième avec les jetons rangés ici, lire nos partages ou
// faire sonder une adresse (revue de sécurité du kernel, 2026-10-28, #598).
// Posée sur tout ce qui n'est pas « jouer » : /api/v1/servers/*, le QR des
// invitations, les partages montés.
func localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth.Federated(r.Context()) {
			writeError(w, http.StatusForbidden, "a session opened from another server only plays here")

			return
		}
		next(w, r)
	}
}

func adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.IsAdmin(r.Context()) {
			writeError(w, http.StatusForbidden, "admin access required")

			return
		}
		next(w, r)
	}
}
