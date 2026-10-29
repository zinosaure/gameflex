package api

// LE LISTING DE CE SERVEUR (chantier 15, décidé par le porteur le 2026-10-13).
//
// Ce qu'il possède — jeux, BIOS, cœurs, correctifs, paramètres — rangé dans
// `cache/listing/self/`, au format de tous les listings (gameflex/storage/
// listing). Ses propres écrans le lisent, au lieu de relire le disque : « on
// utilise ce fichier pour lister les éléments » (le porteur). Et c'est lui que
// la fédération sert à un ami.
//
// IL SUIT CE QUE GAMEFLEX ÉCRIT : un jeu importé ou supprimé, un cœur
// téléchargé, des paramètres enregistrés refont leur section (refreshSelf).
// Ce qu'on dépose à la main sur le disque attend un parcours : « Rescan all »,
// ou l'icône à côté d'un sélecteur de source, qui ne refait que sa section.

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gameflex/consoles"
	"gameflex/emuparams"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/gamestatus"
	"gameflex/storage/listing"
)

// selfMu — un parcours de ce serveur à la fois : deux « Rescan all » croisés
// réécriraient le même dossier.
var selfMu sync.Mutex

// patchesRoot — les correctifs que l'administrateur dépose : `patches/` du
// dossier, à côté de `params/` (voir l'entrypoint Wii U). La même
// variable que les paramètres : un test la remplace, et rien n'écrit alors
// dans la vraie bibliothèque.
func patchesRoot() string {
	return filepath.Join(paramsRoot, "patches")
}

// selfServer — qui sert ce listing : son nom, et sa version.
func (server *Server) selfServer() *listing.Server {
	return &listing.Server{
		Hostname: server.federationHostname(),
		Version:  os.Getenv("GAMEFLEX_VERSION"),
		Commit:   os.Getenv("GITHUB_COMMIT_SHA"),
	}
}

// libraryRoots — où une bibliothèque range ses sections : le dossier de ce
// serveur, ou un disque interne (chantier 24), rangés de la même façon.
type libraryRoots struct {
	Roms, Cores, Bios, Patches, Params string
	// Status — le dossier des statuts des jeux (chantier 30), toujours celui
	// de ce serveur ; vide : les jeux partent sans leurs signalements.
	Status string
}

// selfRoots — les sections du dossier de ce serveur. `params/` et `patches/`
// suivent `paramsRoot`, qu'un test déplace.
func (server *Server) selfRoots() libraryRoots {
	return libraryRoots{
		Roms: server.retroRomsRoot(), Cores: server.retroCoresRoot(), Bios: server.retroBiosRoot(),
		Patches: patchesRoot(), Params: paramsRoot, Status: server.statusStore().Dir,
	}
}

// selfSection — une section de ce serveur, lue sur le disque.
func (server *Server) selfSection(section string) map[string][]listing.Item {
	return librarySection(server.selfRoots(), section)
}

// romItems — les jeux d'UNE console d'une bibliothèque, avec leurs
// signalements (chantier 30), sans les versions : ce que le listing publie.
// Nil quand la console est illisible ou vide.
func romItems(roots libraryRoots, consoleID string) []listing.Item {
	roms, err := retro_files.ListCatalog(roots.Roms, consoleID, "")
	if err != nil {
		return nil // une console illisible n'emporte pas les autres
	}
	var reports map[string][]gamestatus.Report
	if roots.Status != "" {
		console, err := gamestatus.Store{Dir: roots.Status}.Load(consoleID)
		if err != nil {
			slog.Warn("game status unreadable", "console", consoleID, "error", err)
		}
		reports = console.Games
	}
	var items []listing.Item
	for _, rom := range roms {
		items = append(items, listing.Item{
			ID: rom.Reference, Name: rom.Title, Files: rom.Files, Dir: rom.Folder, Size: rom.SizeBytes,
			Reports: gamestatus.Public(reports[rom.Reference]), Engine: rom.Engine,
		})
	}

	return items
}

// librarySection — une section d'une bibliothèque, lue sur le disque, à la
// forme de tous les listings. Rien n'y est écrit : un disque interne se lit
// de la même façon que le dossier de ce serveur.
func librarySection(roots libraryRoots, section string) map[string][]listing.Item {
	items := map[string][]listing.Item{}
	switch section {
	case listing.Roms:
		for _, summary := range retro_files.CountCatalog(roots.Roms) {
			if summary.Count == 0 {
				continue
			}
			if found := romItems(roots, summary.ID); len(found) > 0 {
				items[summary.ID] = found
			}
		}
	case listing.Cores:
		for _, console := range consoles.All() {
			entries, err := retro_files.ListCoreFiles(roots.Cores, console.ID)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				// Rangé sous sa console, comme GameFlex les dépose : un cœur à
				// plat (`cores/<fichier>`) est l'ancien rangement, qui n'a plus
				// cours (aucun sur les deux piles, 2026-10-13).
				if !entry.Scoped && !entry.AndroidInstalled {
					continue
				}
				variants := map[string]int64{}
				if entry.Scoped {
					variants["linux"] = entry.SizeBytes
				}
				if entry.AndroidInstalled {
					variants["android"] = entry.AndroidSizeBytes
				}
				items[console.ID] = append(items[console.ID], listing.Item{
					Name: entry.Name, Size: entry.SizeBytes, Variants: variants,
				})
			}
			// LES FICHIERS DES ÉMULATEURS DE BUREAU : un élément par fichier, sous
			// son nom. Les paquets du téléphone (chantier 28) sous la variante
			// « bundle » — un téléphone y cherche le nom exact que son APK
			// attend —, ceux du serveur sous « emulator » (chantier 29) : c'est
			// par eux qu'un ami prend la dernière version chez nous.
			for _, file := range retro_files.ListEmulatorFiles(roots.Cores, console.ID) {
				variant := "emulator"
				if file.Phone {
					variant = "bundle"
				}
				items[console.ID] = append(items[console.ID], listing.Item{
					Name: file.File, Size: file.SizeBytes,
					Variants: map[string]int64{variant: file.SizeBytes},
				})
			}
		}
	case listing.Bios:
		for _, console := range consoles.All() {
			entries, err := retro_files.ListBiosEntries(roots.Bios, console.ID)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				items[console.ID] = append(items[console.ID], listing.Item{
					Name: entry.Name, Dir: entry.IsFolder, Size: entry.SizeBytes,
				})
			}
		}
	case listing.Patches:
		items = listing.LocalTree(roots.Patches)
	case listing.Params:
		items = paramsItems(emuparams.Store{Root: roots.Params}.Inventory())
	}

	return items
}

// paramsItems — ce que les paramètres d'ici ont de propre, à la forme du
// listing : un élément par émulateur. Le même chez le téléphone.
func paramsItems(inventory []emuparams.Shared) map[string][]listing.Item {
	items := map[string][]listing.Item{}
	for _, shared := range inventory {
		item := listing.Item{Name: shared.Ref.Emulator, Values: shared.Values}
		for _, preset := range shared.Presets {
			item.Presets = append(item.Presets, listing.Preset{ID: preset.ID, Name: preset.Name})
		}
		items[shared.Ref.ConsoleID] = append(items[shared.Ref.ConsoleID], item)
	}

	return items
}

// rebuildSelf refait le listing de ce serveur : ces sections, ou tout — et
// tout quand il n'existe pas encore, puisqu'une section n'a rien où se poser.
func (server *Server) rebuildSelf(sections ...string) error {
	selfMu.Lock()
	defer selfMu.Unlock()

	return server.rebuildSelfLocked(sections...)
}

func (server *Server) rebuildSelfLocked(sections ...string) error {
	now := time.Now()
	if len(sections) == 0 || !listing.Scanned(listingCacheDir, listing.SelfKey) {
		built := listing.New(listing.SelfSource, server.federationHostname())
		built.Header.Server = server.selfServer()
		machine := listing.ReadMachine()
		built.Header.Machine = &machine
		for _, section := range listing.Sections {
			built.SetSection(section, server.selfSection(section), now)
		}
		built.Header.Statuses = server.statusStore().Statuses()

		return listing.Save(listingCacheDir, listing.SelfKey, built)
	}
	for _, section := range sections {
		if err := listing.SaveSection(listingCacheDir, listing.SelfKey, section, server.selfSection(section), now); err != nil {
			return err
		}
	}

	return nil
}

// refreshSelf — après ce que GameFlex vient d'écrire : la section touchée.
// L'écriture elle-même a réussi ; un listing qui ne suit pas se dit au
// journal, et le bouton le rattrapera.
func (server *Server) refreshSelf(sections ...string) {
	if err := server.rebuildSelf(sections...); err != nil {
		slog.Warn("self listing not refreshed", "sections", sections, "error", err)
	}
}

// ensureSelf — le listing existe avant qu'on le lise : la toute première
// lecture le construit, et deux premières lectures croisées ne le
// construisent qu'une fois.
func (server *Server) ensureSelf() {
	if listing.Scanned(listingCacheDir, listing.SelfKey) {
		return
	}
	selfMu.Lock()
	defer selfMu.Unlock()
	if !listing.Scanned(listingCacheDir, listing.SelfKey) {
		if err := server.rebuildSelfLocked(); err != nil {
			slog.Warn("self listing not built", "error", err)
		}
	}
}

// selfItems — une section d'une console, lue dans le listing de ce serveur.
func (server *Server) selfItems(section, consoleID string) []listing.Item {
	server.ensureSelf()
	items, err := listing.LoadSection(listingCacheDir, listing.SelfKey, consoleID, section)
	if err != nil {
		slog.Warn("self listing unreadable", "section", section, "console", consoleID, "error", err)

		return []listing.Item{}
	}

	return items
}

// selfSectionAll — une section entière, toutes consoles.
func (server *Server) selfSectionAll(section string) map[string][]listing.Item {
	server.ensureSelf()
	items, err := listing.LoadSectionAll(listingCacheDir, listing.SelfKey, section)
	if err != nil {
		slog.Warn("self listing unreadable", "section", section, "error", err)

		return map[string][]listing.Item{}
	}

	return items
}

// startSelfListing — au démarrage : le listing s'il manque, sinon seulement
// son en-tête (la version a pu changer, la machine aussi — et sa date avance
// alors, pour qu'un ami le voie). En fond : un premier parcours d'une grande
// bibliothèque ne retient pas le démarrage.
func (server *Server) startSelfListing() {
	go func() {
		if !listing.Scanned(listingCacheDir, listing.SelfKey) {
			started := time.Now()
			server.ensureSelf()
			slog.Info("self listing built", "duration", time.Since(started))

			return
		}
		machine := listing.ReadMachine()
		if err := listing.SaveServer(listingCacheDir, listing.SelfKey, server.federationHostname(),
			server.selfServer(), &machine, time.Now()); err != nil {
			slog.Warn("self listing header not refreshed", "error", err)
		}
	}()
}

// handleSelfListing — GET /api/v1/retrogaming/listing : l'en-tête du listing
// de ce serveur — sa date, celle de chaque section —, pour les textes d'aide
// sous « Rescan all » et sous chaque sélecteur de source.
func (server *Server) handleSelfListing(w http.ResponseWriter, r *http.Request) {
	server.ensureSelf()
	header, err := listing.LoadHeader(listingCacheDir, listing.SelfKey)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "the listing of this server is not built yet")

		return
	}

	writeJSON(w, http.StatusOK, header)
}

// handleSelfRescan — POST /api/v1/retrogaming/listing/rescan[?section=…] :
// « Rescan all » sans section, l'icône d'un sélecteur avec la sienne.
// Administrateurs seulement : c'est la bibliothèque de tout le monde.
func (server *Server) handleSelfRescan(w http.ResponseWriter, r *http.Request) {
	var sections []string
	if section := r.URL.Query().Get("section"); section != "" {
		if !listing.IsSection(section) {
			writeError(w, http.StatusBadRequest, "unknown section: "+section)

			return
		}
		sections = append(sections, section)
	}
	started := time.Now()
	if err := server.rebuildSelf(sections...); err != nil {
		writeError(w, http.StatusInternalServerError, "the listing could not be rebuilt: "+err.Error())

		return
	}
	header, _ := listing.LoadHeader(listingCacheDir, listing.SelfKey)
	slog.Info("self listing rescanned", "sections", sections, "duration", time.Since(started))

	writeJSON(w, http.StatusOK, header)
}

// refreshing — cette route écrit dans la bibliothèque : après une réponse
// réussie, la section qu'elle touche se refait dans le listing, AVANT que la
// réponse ne parte — la page qui relit aussitôt lit déjà le nouveau.
func (server *Server) refreshing(section func(*http.Request) string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(recorder, r)
		if recorder.status < http.StatusMultipleChoices {
			if name := section(r); listing.IsSection(name) {
				server.refreshSelf(name)
			}
		}
	}
}

// sectionNamed — une route qui touche toujours la même section.
func sectionNamed(name string) func(*http.Request) string {
	return func(*http.Request) string { return name }
}

// statusRecorder retient le statut que le gestionnaire a écrit.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

// Flush — un gestionnaire qui pousse en continu le peut encore.
func (recorder *statusRecorder) Flush() {
	if flusher, ok := recorder.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
