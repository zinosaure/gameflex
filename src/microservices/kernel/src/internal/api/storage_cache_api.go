package api

// LE CACHE DU DOSSIER (#448) : ce que le premier lancement d'un jeu installe,
// une fois pour tous les comptes (`cache/installs/<console>/`, chantier 22),
// montré et vidé entrée par entrée depuis Réglages › Stockage › Cache. Le
// porteur, 2026-10-24 : « ça évitera de grossir le stockage sans savoir d'où ça
// vient, étant donné qu'on ne liste pas les installs dans l'admin ».
//
// Mêmes routes et mêmes réponses sur le téléphone (mobilekernel).

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"gameflex/consoles"
	"gameflex/storage/installs"
)

// cacheEntry — une entrée telle que l'écran la montre : ce que le paquet
// partagé en sait, plus ce que seul le kernel sait.
type cacheEntry struct {
	installs.Entry
	// Source — « present » quand ce qui la refait est encore là (le .pkg d'un
	// jeu PS4 déballé), « missing » quand il n'y est plus ; vide quand on ne
	// sait pas le dire.
	Source string `json:"source,omitempty"`
	// InUse — une partie de cette console tourne : rien ne s'en efface.
	InUse bool `json:"inUse"`
}

type cacheConsole struct {
	ConsoleID string       `json:"consoleId"`
	Console   string       `json:"console"`
	SizeBytes int64        `json:"sizeBytes"`
	Entries   []cacheEntry `json:"entries"`
}

// handleStorageCache — GET /api/v1/retrogaming/storage/cache.
func (server *Server) handleStorageCache(w http.ResponseWriter, r *http.Request) {
	listed, err := installs.List(server.storageRoot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}
	running := server.runningConsoles()
	stems := server.presentPkgStems(r.Context())

	out := []cacheConsole{}
	var total int64
	for _, console := range listed {
		item := cacheConsole{ConsoleID: console.ConsoleID, Console: consoleDisplayName(console.ConsoleID), SizeBytes: console.SizeBytes}
		for _, entry := range console.Entries {
			shown := cacheEntry{Entry: entry, InUse: running[console.ConsoleID]}
			if entry.Kind == installs.KindUnpacked {
				shown.Source = "missing"
				if stems[entry.Name] {
					shown.Source = "present"
				}
			}
			item.Entries = append(item.Entries, shown)
		}
		total += console.SizeBytes
		out = append(out, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{"consoles": out, "sizeBytes": total})
}

// handleStorageCacheDelete — DELETE /api/v1/retrogaming/storage/cache?consoleId=…&name=….
//
// Refusé pendant une partie de cette console (409) : elle lit ce cache — le
// firmware et les données installées de la PS3 sont partagés par toutes ses
// parties.
func (server *Server) handleStorageCacheDelete(w http.ResponseWriter, r *http.Request) {
	consoleID := strings.TrimSpace(r.URL.Query().Get("consoleId"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if consoleID == "" || name == "" {
		writeError(w, http.StatusBadRequest, "consoleId et name sont requis")

		return
	}
	if server.runningConsoles()[consoleID] {
		writeError(w, http.StatusConflict, "une partie de cette console est en cours : elle lit ce cache")

		return
	}
	if err := installs.Remove(server.storageRoot, consoleID, name); err != nil {
		if errors.Is(err, installs.ErrUnknown) {
			writeError(w, http.StatusNotFound, "entrée inconnue dans le cache")

			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"consoleId": consoleID, "name": name, "removed": true, "finished": true})
}

// runningConsoles — les consoles dont une partie tourne sur ce serveur.
func (server *Server) runningConsoles() map[string]bool {
	running := map[string]bool{}
	if server.manager == nil {
		return running
	}
	for _, session := range server.manager.List() {
		running[session.Console] = true
	}

	return running
}

// presentPkgStems — les .pkg PS4 de la bibliothèque et des disques internes,
// sous le nom de leur déballé.
func (server *Server) presentPkgStems(ctx context.Context) map[string]bool {
	present := map[string]bool{}
	roots := []string{server.retroRomsRoot()}
	for _, disk := range server.presentDisks(ctx) {
		roots = append(roots, disk.RomsDir())
	}
	for _, root := range roots {
		for _, stem := range pkgStems(filepath.Join(root, "ps4")) {
			present[stem] = true
		}
	}

	return present
}

// consoleDisplayName — le nom d'une console, son identifiant à défaut.
func consoleDisplayName(consoleID string) string {
	for _, console := range consoles.All() {
		if console.ID == consoleID {
			return console.Name
		}
	}

	return consoleID
}
