package api

// LE FICHIER D'ÉMULATEUR QU'ATTEND CHAQUE CONSOLE RÉCENTE (chantier 29).
//
// Les images des consoles ne portent plus leur émulateur, pour pouvoir se
// publier : chacune le déballe au lancement depuis `cores/<console>/`, et dit
// lequel par une étiquette (docker.RequiredEmulator). Trois endroits en
// tiennent compte :
//
//   - le lancement le refuse quand le fichier manque, en le nommant, plutôt
//     qu'un conteneur qui s'arrête au démarrage ;
//   - la fiche d'un jeu grise l'émulateur, comme un cœur libretro absent ;
//   - l'écran Émulateurs le montre manquant, et le télécharge chez son
//     éditeur, comme un cœur chez libretro.
//
// LA PRÉSENCE SE LIT SUR LE DISQUE, pas dans le listing : c'est le disque que
// le conteneur lira, et un fichier copié à la main doit compter tout de suite.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"gameflex/facade"
	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage"
)

// requiredEmulators — les fichiers qu'attendent les images installées ici. Un
// test les donne par `server.emulatorsRequired` ; sinon c'est le manager qui
// lit les étiquettes des images.
//
// GARDÉS QUELQUES SECONDES (chantier 53) : lire les étiquettes inspecte chaque
// image Streamex par Docker, et la liste des émulateurs le faisait pour CHACUNE
// des quelque quarante-cinq consoles du registre — 4,7 s par liste sur la dev.
// Une image reconstruite se voit au plus `requiredEmulatorsFresh` après.
func (server *Server) requiredEmulators(ctx context.Context) []docker.RequiredEmulator {
	if server.emulatorsRequired != nil {
		return server.emulatorsRequired(ctx)
	}
	if server.manager == nil {
		return nil
	}
	server.requiredCache.mu.Lock()
	defer server.requiredCache.mu.Unlock()
	if server.requiredCache.items == nil || time.Since(server.requiredCache.at) > requiredEmulatorsFresh {
		server.requiredCache.items = server.manager.RequiredEmulators(ctx)
		server.requiredCache.at = time.Now()
	}

	return slices.Clone(server.requiredCache.items)
}

// requiredEmulatorsFresh — combien de temps les étiquettes lues restent bonnes.
const requiredEmulatorsFresh = 15 * time.Second

// requiredEmulatorsCache — la dernière lecture des étiquettes. Zéro-valeur
// utilisable.
type requiredEmulatorsCache struct {
	mu    sync.Mutex
	at    time.Time
	items []docker.RequiredEmulator
}

// requiredEmulatorFor — celui d'un lancement : l'émulateur choisi, ou le
// premier de la console, le même que le lancement prend.
func (server *Server) requiredEmulatorFor(ctx context.Context, consoleID, core string) (docker.RequiredEmulator, bool) {
	if core == "" {
		cores := docker.StreamexCores(consoleID)
		if len(cores) == 0 {
			return docker.RequiredEmulator{}, false
		}
		core = cores[0]
	}
	for _, need := range server.requiredEmulators(ctx) {
		if need.Console == consoleID && need.Core == core {
			return need, true
		}
	}

	return docker.RequiredEmulator{}, false
}

// emulatorInUse — le fichier qu'une partie prendrait : la version en service
// que l'administrateur a choisie (console_defaults.go) quand elle est de cet
// émulateur et toujours là ; sinon, parmi ceux qui répondent au motif de
// l'image, le dernier déposé. Le lancement passe ce nom au conteneur
// (GAMEFLEX_EMULATOR_CHOSEN), que consoles/unpack-emulator.sh déballe.
func (server *Server) emulatorInUse(need docker.RequiredEmulator) (string, os.FileInfo, bool) {
	dir := filepath.Join(server.retroCoresRoot(), need.Console)
	if chosen := server.consoleDefaultsOf(need.Console).EmulatorFile; chosen != "" {
		if matched, _ := filepath.Match(need.Pattern, chosen); matched {
			if info, err := os.Stat(filepath.Join(dir, chosen)); err == nil && !info.IsDir() {
				return chosen, info, true
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, false
	}
	var (
		chosen string
		newest os.FileInfo
	)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if matched, _ := filepath.Match(need.Pattern, entry.Name()); !matched {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if newest == nil || info.ModTime().After(newest.ModTime()) {
			chosen, newest = entry.Name(), info
		}
	}

	return chosen, newest, newest != nil
}

// requireEmulator — avant une partie d'une console récente. Rien à exiger
// quand l'image manque : son lancement dit déjà qu'elle n'est pas installée.
func (server *Server) requireEmulator(ctx context.Context, consoleID, core string) error {
	need, ok := server.requiredEmulatorFor(ctx, consoleID, core)
	if !ok {
		return nil
	}
	if _, _, present := server.emulatorInUse(need); present {
		return nil
	}

	return fmt.Errorf("this console's emulator is missing from cores/%s/ — "+
		"download %s from Settings › Emulators", need.Console, need.File)
}

// requiredEmulatorItems — les lignes de l'écran Émulateurs pour ce
// qu'attendent les images, quand le listing ne les a pas déjà : la version
// testée quand le dossier n'en a aucune, à télécharger ; ou celle qu'une partie
// prendrait, copiée à la main depuis le dernier parcours.
func (server *Server) requiredEmulatorItems(ctx context.Context, consoleID string, listed map[string]bool) []storeItem {
	items := []storeItem{}
	for _, need := range server.requiredEmulators(ctx) {
		if need.Console != consoleID {
			continue
		}
		item := storeItem{Name: need.File, File: need.File, Scoped: true, Package: "server", Source: need.URL}
		if name, info, present := server.emulatorInUse(need); present {
			item.Name, item.File, item.Installed, item.SizeBytes = name, name, true, info.Size()
		}
		if listed[item.Name] {
			continue
		}
		listed[item.Name] = true
		items = append(items, item)
	}

	return items
}

// markEmulatorFiles — sur chaque fichier d'émulateur de console récente :
// éprouvé ou non avec l'image installée, et celui qu'une partie prendrait.
func (server *Server) markEmulatorFiles(ctx context.Context, sections []consoleSection) {
	for _, need := range server.requiredEmulators(ctx) {
		inUse, _, _ := server.emulatorInUse(need)
		for index := range sections {
			if sections[index].ConsoleID != need.Console {
				continue
			}
			for item := range sections[index].Items {
				row := &sections[index].Items[item]
				if row.Package != "server" {
					continue
				}
				if matched, _ := filepath.Match(need.Pattern, row.Name); !matched {
					continue
				}
				tested := slices.Contains(need.Tested, row.Name)
				row.Tested = &tested
				row.InUse = row.Installed && row.Name == inUse
				if row.Installed {
					row.Version = wildcardPart(need.Pattern, row.Name)
				}
			}
		}
	}
}

// downloadRequiredEmulator — le bouton de l'écran Émulateurs, pour la version
// testée avec une image : elle descend chez l'éditeur, à l'adresse que dit
// l'image, et se range sous son nom. Une autre version déjà là n'empêche rien :
// la télécharger, c'est y revenir, puisqu'elle devient la dernière déposée.
// Faux pour un autre fichier : seule celle-là a une adresse connue.
func (server *Server) downloadRequiredEmulator(w http.ResponseWriter, r *http.Request, consoleID, name string) bool {
	var need docker.RequiredEmulator
	found := false
	for _, candidate := range server.requiredEmulators(r.Context()) {
		if candidate.Console == consoleID && candidate.File == name {
			need, found = candidate, true

			break
		}
	}
	if !found {
		return false
	}
	if _, err := os.Stat(filepath.Join(server.retroCoresRoot(), need.Console, need.File)); err == nil {
		writeErrorCode(w, http.StatusConflict, "already_installed", "this emulator is already here: delete it first to download it again")

		return true
	}
	if need.URL == "" {
		writeErrorCode(w, http.StatusUnprocessableEntity, "emulator_no_source", "this console's image does not say where its emulator is published")

		return true
	}
	written, err := downloadEmulatorFile(r.Context(), need.URL,
		filepath.Join(server.retroCoresRoot(), need.Console, need.File))
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "download_failed", err.Error())

		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"core": name, "consoleId": consoleID, "sizeBytes": written})

	return true
}

// emulatorDownloadTimeout — les plus gros font une centaine de mégaoctets.
const emulatorDownloadTimeout = 15 * time.Minute

// downloadEmulatorFile — le fichier tel que l'éditeur le publie, sans le
// déballer : c'est l'image qui le fera, à chaque partie. Un fichier partiel
// d'abord, pour qu'une coupure ne laisse pas un fichier tronqué qui passerait
// pour présent.
func downloadEmulatorFile(ctx context.Context, url, target string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, fmt.Errorf("folder cannot be created: %w", err)
	}
	storage.AdoptParentOwner(filepath.Dir(target))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	response, err := (&http.Client{Timeout: emulatorDownloadTimeout}).Do(request)
	if err != nil {
		return 0, fmt.Errorf("the emulator's publisher did not answer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("the emulator's publisher answered %d for %s", response.StatusCode, url)
	}

	partial := target + ".part"
	out, err := os.Create(partial)
	if err != nil {
		return 0, fmt.Errorf("cannot write: %w", err)
	}
	written, err := io.Copy(facade.ProgressWriter(ctx, out, response.ContentLength), response.Body)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err == nil && written == 0 {
		err = errors.New("the emulator's publisher sent an empty file")
	}
	if err != nil {
		os.Remove(partial)

		return 0, fmt.Errorf("download interrupted: %w", err)
	}
	if err := os.Rename(partial, target); err != nil {
		os.Remove(partial)

		return 0, err
	}
	storage.AdoptParentOwner(target)

	return written, nil
}

// markCoreVersions — la version d'un cœur libretro installé : la date de son
// fichier, sous sa console d'abord, puis à plat (emulatorVersion).
func (server *Server) markCoreVersions(sections []consoleSection) {
	for index := range sections {
		for item := range sections[index].Items {
			row := &sections[index].Items[item]
			if row.Package != "" || !row.Installed || row.Version != "" {
				continue
			}
			for _, path := range []string{
				filepath.Join(server.retroCoresRoot(), sections[index].ConsoleID, retro_files.CoreFileName(row.Name)),
				filepath.Join(server.retroCoresRoot(), retro_files.CoreFileName(row.Name)),
			} {
				if info, err := os.Stat(path); err == nil {
					row.Version = info.ModTime().UTC().Format("2006-01-02")

					break
				}
			}
		}
	}
}
