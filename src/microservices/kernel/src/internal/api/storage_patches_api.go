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

	"gameflex/storage"
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
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid patch")

		return
	}
	if _, err := os.Stat(path); err != nil {
		writeErrorCode(w, http.StatusNotFound, "not_found", "patch not found")

		return
	}
	if err := os.RemoveAll(path); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "storage_failed", err.Error())

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
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid patch")

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
		writeErrorCode(w, http.StatusNotFound, "not_found", "patch not found")

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
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "corps illisible")

		return
	}
	target, ok := patchPath(body.ConsoleID, body.Name)
	if !ok {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid patch")

		return
	}
	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}

	items, err := server.copies().Items(peerOf(entry), listing.Patches, body.ConsoleID)
	if err != nil {
		refused := remote.Refusal(err)
		writeErrorCode(w, refused.Status, codeOr(refused.Code, "storage_failed"), refused.Message)

		return
	}
	var announced *listing.Item
	for index := range items {
		if items[index].Name == body.Name {
			announced = &items[index]
		}
	}
	if announced == nil {
		writeErrorCode(w, http.StatusNotFound, "not_lent", "this server does not announce this patch")

		return
	}
	if _, err := os.Stat(target); err == nil {
		writeErrorCode(w, http.StatusConflict, "already_installed", "ce correctif existe déjà : le supprimer d'abord pour en reprendre une version")

		return
	}

	client, err := remote.New(entry.Link, entry.Token)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "peer_unreachable", "this address did not answer")

		return
	}
	policy := zipKeep
	if announced.Dir {
		policy = zipExpand
	}
	written, err := importIntoLibrary(r.Context(), func() (io.ReadCloser, error) {
		reader, err := client.OpenPatch(body.ConsoleID, body.Name)
		if errors.Is(err, remote.ErrNotLent) {
			return nil, errors.New("ce serveur ne prête plus ce correctif")
		}

		return reader, err
	}, target, policy)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "storage_failed", err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"section": "patches", "consoleId": body.ConsoleID, "name": body.Name,
		"sizeBytes": written, "source": entry.Hostname,
	})
}

// LE CORRECTIF D'UN JEU (chantier 53, P6, #675) : la page d'un jeu montre le
// sien comme un émulateur — installé ici, sinon « Le récupérer » chez chaque
// source qui l'a. Il n'y avait qu'une liste globale, que la page filtrait
// elle-même par l'identifiant du jeu.
//
//	GET /api/v1/retrogaming/{consoleId}/roms/{ref}/patch
//
// Le correctif d'un jeu porte le nom de son identifiant (`patches/<console>/<id>/`,
// le même que ses réglages par jeu). Ses sources : chaque disque interne, puis
// chaque serveur fédéré, lu dans la copie de son listing — un pair qui la
// refuse ou ne répond pas le dit, au lieu d'une liste vide. Et d'abord la
// source officielle (chantier 63, P6) : le paquet `streamex-patches` de l'index
// des paquets streamex, quand il porte ce correctif.
type gamePatchSource struct {
	Source    string `json:"source"` // "official" | "disk" | "server"
	Disk      string `json:"disk,omitempty"`
	ServerID  int64  `json:"serverId,omitempty"`
	Name      string `json:"name,omitempty"`
	Available bool   `json:"available"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type gamePatchView struct {
	GameID    string            `json:"gameId"`
	Folder    string            `json:"folder,omitempty"`
	Installed *remote.PatchItem `json:"installed"`
	Sources   []gamePatchSource `json:"sources"`
}

func (server *Server) handleGamePatch(w http.ResponseWriter, r *http.Request) {
	consoleID, _, gamePath, _, ok := server.metadataGame(w, r)
	if !ok {
		return
	}
	view := gamePatchView{GameID: storage.GameTitleID(gamePath), Sources: []gamePatchSource{}}
	if view.GameID == "" || !remote.PatchName(view.GameID) {
		view.GameID = ""
		writeJSON(w, http.StatusOK, view)

		return
	}
	view.Folder = "patches/" + consoleID + "/" + view.GameID + "/"

	for _, item := range server.selfSectionAll(listing.Patches)[consoleID] {
		if item.Name == view.GameID {
			files := item.Files
			if files == nil {
				files = []string{}
			}
			view.Installed = &remote.PatchItem{Name: item.Name, SizeBytes: item.Size, Dir: item.Dir, Files: files, Installed: true}
		}
	}

	official := gamePatchSource{Source: "official"}
	if index, err := server.cachedStreamexIndex(r.Context()); err != nil {
		official.Reason = err.Error()
	} else if pkg, ok := index.patchesFor(consoleID, view.GameID); ok {
		official.Available, official.SizeBytes = true, pkg.SizeBytes
	}
	view.Sources = append(view.Sources, official)

	for _, disk := range server.presentDisks(r.Context()) {
		source := gamePatchSource{Source: "disk", Disk: disk.Name}
		if info, err := os.Stat(filepath.Join(disk.Section(listing.Patches), consoleID, view.GameID)); err == nil {
			source.Available = true
			if !info.IsDir() {
				source.SizeBytes = info.Size()
			}
		}
		view.Sources = append(view.Sources, source)
	}

	peers, _ := server.registeredServers()
	for _, entry := range peers {
		source := gamePatchSource{Source: "server", ServerID: entry.ID, Name: entry.Hostname}
		items, err := server.copies().Items(peerOf(entry), listing.Patches, consoleID)
		if err != nil {
			source.Reason = remote.Refusal(err).Message
		}
		for _, item := range items {
			if item.Name == view.GameID {
				source.Available, source.SizeBytes = true, item.Size
			}
		}
		view.Sources = append(view.Sources, source)
	}

	writeJSON(w, http.StatusOK, view)
}
