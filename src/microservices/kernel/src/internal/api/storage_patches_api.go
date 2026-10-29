package api

// LES CORRECTIFS (chantier 15, décidé par le porteur le 2026-10-13) —
// `patches/<console>/<nom>` du dossier, à côté de `params/` :
// d'ordinaire le dossier d'un jeu, qui porte ses packs
// (`wiiu/NewSuperMarioBrosU/CrashFix/`).
//
// L'écran Correctifs les liste — lus dans le listing de ce serveur —, en
// supprime un, et en rapatrie un d'un ami ; la fédération sert les nôtres, un
// dossier zippé à la volée comme un jeu-dossier. RIEN N'ÉCRASE RIEN : pour
// reprendre une version, on supprime d'abord.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

// patchPath — où vit ce correctif ici, ou faux si sa console ou son nom ne
// peuvent pas être un chemin.
func patchPath(consoleID, name string) (string, bool) {
	if !remote.PatchName(consoleID) || !remote.PatchName(name) {
		return "", false
	}

	return filepath.Join(patchesRoot(), consoleID, name), true
}

// patchHere — ce correctif est-il ici ?
func patchHere(consoleID, name string) bool {
	path, ok := patchPath(consoleID, name)
	if !ok {
		return false
	}
	_, err := os.Stat(path)

	return err == nil
}

// handleStoragePatches — GET /api/v1/retrogaming/storage/patches : les
// correctifs d'ici, console par console, lus dans le listing de ce serveur.
func (server *Server) handleStoragePatches(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"consoles": remote.PatchSections(server.selfSectionAll(listing.Patches), func(string, string) bool { return true }),
	})
}

// handleStoragePatchDelete — DELETE /api/v1/retrogaming/storage/patches?consoleId=&name=
func (server *Server) handleStoragePatchDelete(w http.ResponseWriter, r *http.Request) {
	consoleID, name := r.URL.Query().Get("consoleId"), r.URL.Query().Get("name")
	path, ok := patchPath(consoleID, name)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid patch")

		return
	}
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "patch not found")

		return
	}
	if err := os.RemoveAll(path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"consoleId": consoleID, "name": name, "removed": true})
}

// handleFederationPatch — GET/HEAD /api/v1/federation/patches/{consoleId}/{name}
// : un correctif d'ici, servi à un ami — un dossier zippé à la volée. Pris
// dans la bibliothèque, puis sur les disques internes (chantier 24).
func (server *Server) handleFederationPatch(w http.ResponseWriter, r *http.Request) {
	consoleID, name := chi.URLParam(r, "consoleId"), chi.URLParam(r, "name")
	if _, ok := patchPath(consoleID, name); !ok {
		writeError(w, http.StatusBadRequest, "invalid patch")

		return
	}
	var path string
	var info os.FileInfo
	err := os.ErrNotExist
	for _, roots := range server.libraryRootsInOrder(r.Context()) {
		path = filepath.Join(roots.Patches, consoleID, name)
		if info, err = os.Stat(path); err == nil {
			break
		}
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "patch not found")

		return
	}
	if info.IsDir() {
		serveDirectoryAsZip(w, r, path, name)

		return
	}
	http.ServeFile(w, r, path)
}

// handleRemoteStoragePatches — GET /api/v1/servers/{id}/storage/patches : les
// correctifs d'un ami, lus dans la copie de son listing, « aussi ici » quand
// on les a.
func (server *Server) handleRemoteStoragePatches(w http.ResponseWriter, r *http.Request) {
	server.serveRemoteStorage(w, r, listing.Patches, func(_ registeredServer, items map[string][]listing.Item) any {
		return remote.PatchSections(items, patchHere)
	})
}

// importPeerPatch — POST /api/v1/servers/{id}/storage/patches {consoleId, name}
// : rapatrier le correctif d'un ami. Ce que la copie de son listing en dit
// décide de la forme : un dossier arrive zippé et se déplie, un fichier se
// pose tel quel.
func (server *Server) importPeerPatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConsoleID string `json:"consoleId"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corps illisible")

		return
	}
	target, ok := patchPath(body.ConsoleID, body.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid patch")

		return
	}
	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}

	items, err := server.copies().Items(peerOf(entry), listing.Patches, body.ConsoleID)
	if err != nil {
		refused := remote.Refusal(err)
		writeError(w, refused.Status, refused.Message)

		return
	}
	var announced *listing.Item
	for index := range items {
		if items[index].Name == body.Name {
			announced = &items[index]
		}
	}
	if announced == nil {
		writeError(w, http.StatusNotFound, "this server does not announce this patch")

		return
	}
	if _, err := os.Stat(target); err == nil {
		writeError(w, http.StatusConflict,
			"ce correctif existe déjà : le supprimer d'abord pour en reprendre une version")

		return
	}

	client, err := remote.New(entry.Link, entry.Token)
	if err != nil {
		writeError(w, http.StatusBadGateway, "this address did not answer")

		return
	}
	policy := zipKeep
	if announced.Dir {
		policy = zipExpand
	}
	written, err := importIntoLibrary(func() (io.ReadCloser, error) {
		reader, err := client.OpenPatch(body.ConsoleID, body.Name)
		if errors.Is(err, remote.ErrNotLent) {
			return nil, errors.New("ce serveur ne prête plus ce correctif")
		}

		return reader, err
	}, target, policy)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"section": "patches", "consoleId": body.ConsoleID, "name": body.Name,
		"sizeBytes": written, "source": entry.Hostname,
	})
}
