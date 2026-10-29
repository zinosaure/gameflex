package api

// LES SAUVEGARDES PAR SOURCE (chantier 38, le porteur, 2026-10-28 et 29) :
// un principal, que les émulateurs jouent, et des copies — `local` pour les
// backups, une par serveur fédéré (shared/storage/saves).
//
//	GET    /api/v1/saves/sources                 le nom de ce serveur, et les copies du compte
//	GET    /api/v1/saves/streamex?source=        les jeux d'un rangement, console par console
//	GET    /api/v1/saves/streamex/game?source=&console=&id=   l'archive d'un jeu
//	POST   /api/v1/saves/streamex/game?source=&console=&id=   une archive, dans une copie
//	DELETE /api/v1/saves/streamex/game?source=&console=&id=   un jeu, effacé d'une copie
//	POST   /api/v1/saves/streamex/backup         des jeux du principal, copiés dans local
//	POST   /api/v1/saves/streamex/restore        des jeux d'une copie, remis dans le principal
//	POST   /api/v1/saves/states/backup           des emplacements du rétro, dans local
//	POST   /api/v1/saves/states/restore          des emplacements d'une copie, dans le principal
//
// Le rétro se lit et s'envoie par ses routes de toujours, `?source=` en plus
// (`/api/v1/retrogaming/savestate/*`).
//
// CE QU'UNE SESSION VENUE D'UN AUTRE SERVEUR PEUT FAIRE ICI : lire, et
// déposer dans une copie — c'est tout l'objet de la fédération des
// sauvegardes. Effacer, faire un backup et restaurer restent des gestes d'ici
// (`localOnly`) : « ce serait à l'utilisateur ensuite de choisir de faire la
// synchronisation entre le dossier principal et les sources » (le porteur).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/manager"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/saves"
)

// maxStreamexSaveBytes — la plus grosse archive reçue. Le disque d'une Xbox
// (xemu) grandit avec ses jeux jusqu'à 8 Go ; au-delà, c'est autre chose
// qu'une sauvegarde. La place du disque est jugée en plus (fitsOnDisk).
const maxStreamexSaveBytes = 16 << 30

// saveTree — le rangement que vise une requête : le principal sans `source`,
// sinon cette copie. Le compte vient du jeton, jamais de la requête. Faux :
// l'erreur est écrite.
func (server *Server) saveTree(w http.ResponseWriter, r *http.Request) (root, owner string, ok bool) {
	root, owner, err := saves.TreeParts(server.retroSavesRoot(), auth.Username(r.Context()), r.URL.Query().Get("source"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid save source")

		return "", "", false
	}

	return root, owner, true
}

func (server *Server) savesRouter() http.Handler {
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(auth.Middleware(server.config.JWTSecret, next.ServeHTTP))
	})
	router.Get("/sources", server.handleSaveSources)
	router.Get("/streamex", server.handleStreamexSaves)
	router.Get("/streamex/game", server.handleStreamexSaveArchive)
	router.Post("/streamex/game", server.handleStreamexSaveUpload)
	router.Delete("/streamex/game", localOnly(server.handleStreamexSaveDelete))
	router.Post("/streamex/backup", localOnly(server.handleStreamexSaveBackup))
	router.Post("/streamex/restore", localOnly(server.handleStreamexSaveRestore))
	router.Post("/states/backup", localOnly(server.handleStateBackup))
	router.Post("/states/restore", localOnly(server.handleStateRestore))

	return router
}

// handleSaveSources — GET /api/v1/saves/sources. `self` : le nom que ce
// serveur annonce à la fédération — celui sous lequel un envoi d'ici arrive
// chez un autre (saves.SourceName).
func (server *Server) handleSaveSources(w http.ResponseWriter, r *http.Request) {
	names, err := saves.Sources(server.retroSavesRoot(), auth.Username(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot list the save copies")

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"self": server.federationHostname(), "sources": names})
}

// handleStreamexSaves — GET /api/v1/saves/streamex?source=. `consoles` : celles
// dont les sauvegardes se copient (saves.Consoles — la Xbox 360 n'y est pas
// encore).
func (server *Server) handleStreamexSaves(w http.ResponseWriter, r *http.Request) {
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"consoles": saves.Consoles(),
		"games":    saves.Games(filepath.Join(root, owner)),
	})
}

// streamexGame — la console et le jeu d'une requête, vérifiés. Faux : l'erreur
// est écrite.
func streamexGame(w http.ResponseWriter, r *http.Request) (console, id string, ok bool) {
	console, id = r.URL.Query().Get("console"), r.URL.Query().Get("id")
	if !saves.Supported(console) || id == "" {
		writeError(w, http.StatusBadRequest, "a supported console and a game id are required")

		return "", "", false
	}

	return console, id, true
}

// handleStreamexSaveArchive — GET /api/v1/saves/streamex/game : la sauvegarde
// d'un jeu, en zip (le disque entier pour la Xbox).
func (server *Server) handleStreamexSaveArchive(w http.ResponseWriter, r *http.Request) {
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	console, id, ok := streamexGame(w, r)
	if !ok {
		return
	}
	tree := filepath.Join(root, owner)
	if !saves.Has(tree, console, id) {
		writeError(w, http.StatusNotFound, "no save for this game")

		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, console, id))
	if err := saves.WriteArchive(w, tree, console, id); err != nil {
		// En-têtes déjà partis : on s'arrête là, et la page voit une archive
		// illisible.
		slog.Warn("save archive interrupted", "console", console, "id", id, "error", err)
	}
}

// handleStreamexSaveUpload — POST /api/v1/saves/streamex/game : une archive
// (handleStreamexSaveArchive d'un autre serveur), déposée dans la copie qui
// porte le nom de l'expéditeur — JAMAIS dans le principal.
func (server *Server) handleStreamexSaveUpload(w http.ResponseWriter, r *http.Request) {
	source := saves.SourceName(r.URL.Query().Get("source"))
	if source == "" {
		writeError(w, http.StatusBadRequest, "source is required: an upload only writes into a copy")

		return
	}
	console, id, ok := streamexGame(w, r)
	if !ok {
		return
	}
	tree, err := saves.Tree(server.retroSavesRoot(), auth.Username(r.Context()), source)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid save source")

		return
	}
	if err := os.MkdirAll(tree, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "cannot create the copy")

		return
	}
	if r.ContentLength > 0 {
		if err := fitsOnDisk(tree, uint64(r.ContentLength)); err != nil {
			writeError(w, http.StatusInsufficientStorage, err.Error())

			return
		}
	}

	upload, err := os.CreateTemp(tree, ".upload-*.zip")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot receive the archive")

		return
	}
	defer os.Remove(upload.Name())
	_, copyErr := io.Copy(upload, http.MaxBytesReader(w, r.Body, maxStreamexSaveBytes))
	closeErr := upload.Close()
	var tooLarge *http.MaxBytesError
	if errors.As(copyErr, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "the save is too large")

		return
	}
	if copyErr != nil || closeErr != nil {
		writeError(w, http.StatusInternalServerError, "cannot receive the archive")

		return
	}

	err = saves.Receive(upload.Name(), tree, console, id, func(size uint64) error { return fitsOnDisk(tree, size) })
	if errors.Is(err, saves.ErrForeignEntry) {
		writeError(w, http.StatusBadRequest, err.Error())

		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("cannot store the save: %v", err))

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": source})
}

// handleStreamexSaveDelete — DELETE /api/v1/saves/streamex/game : un jeu,
// effacé d'une COPIE. Le principal ne s'efface pas d'ici : c'est la page
// Stockage de l'administration qui le fait, compte par compte.
func (server *Server) handleStreamexSaveDelete(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("source") == "" {
		writeError(w, http.StatusBadRequest, "source is required: only a copy is deleted here")

		return
	}
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	console, id, ok := streamexGame(w, r)
	if !ok {
		return
	}
	if err := saves.Remove(filepath.Join(root, owner), console, id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("cannot delete: %v", err))

		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// streamexCopyRequest — le corps d'un backup ou d'une restauration.
type streamexCopyRequest struct {
	Source  string   `json:"source"`
	Console string   `json:"console"`
	IDs     []string `json:"ids"`
}

func decodeStreamexCopy(w http.ResponseWriter, r *http.Request) (streamexCopyRequest, bool) {
	var body streamexCopyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")

		return body, false
	}
	if !saves.Supported(body.Console) || len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "a supported console and game ids are required")

		return body, false
	}

	return body, true
}

// copyResult — ce que chaque jeu a donné, dans l'ordre de la demande.
type copyResult struct {
	ID    string `json:"id"`
	Error string `json:"error,omitempty"`
}

func resultOf(id string, err error) copyResult {
	switch {
	case err == nil:
		return copyResult{ID: id}
	case errors.Is(err, os.ErrNotExist):
		return copyResult{ID: id, Error: "not found"}
	default:
		return copyResult{ID: id, Error: err.Error()}
	}
}

// handleStreamexSaveBackup — POST /api/v1/saves/streamex/backup
// `{console, ids}` : chaque jeu du principal, copié dans `local`.
func (server *Server) handleStreamexSaveBackup(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeStreamexCopy(w, r)
	if !ok {
		return
	}
	username := auth.Username(r.Context())
	results := []copyResult{}
	for _, id := range body.IDs {
		results = append(results, resultOf(id, saves.Backup(server.retroSavesRoot(), username, body.Console, id)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// handleStreamexSaveRestore — POST /api/v1/saves/streamex/restore
// `{source, console, ids}` : chaque jeu de la copie, remis dans le principal,
// et ce que le principal avait, dans `local`.
//
// PAS PENDANT UNE PARTIE de cette console pour ce compte : l'émulateur tient
// ses fichiers ouverts, et réécrit les siens en quittant.
func (server *Server) handleStreamexSaveRestore(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeStreamexCopy(w, r)
	if !ok {
		return
	}
	if body.Source == "" || !saves.ValidSource(body.Source) {
		writeError(w, http.StatusBadRequest, "a copy to restore from is required")

		return
	}
	username := auth.Username(r.Context())
	for _, session := range server.streamexSessions() {
		if session.Owner == username && session.Console == body.Console {
			writeError(w, http.StatusConflict, "a game of this console is running: stop it first")

			return
		}
	}

	results := []copyResult{}
	for _, id := range body.IDs {
		results = append(results, resultOf(id, saves.Restore(server.retroSavesRoot(), username, body.Source, body.Console, id)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// stateItem — un emplacement du rétro, dans un backup ou une restauration.
// ToReference — sous quelle référence le remettre ici, quand la copie vient
// d'un serveur qui range ce jeu sous une autre (vide : la même).
type stateItem struct {
	ConsoleID   string `json:"consoleId"`
	Reference   string `json:"reference"`
	ToReference string `json:"toReference,omitempty"`
	Slot        int    `json:"slot"`
}

type stateCopyRequest struct {
	Source string      `json:"source"`
	Items  []stateItem `json:"items"`
}

type stateResult struct {
	stateItem
	Error string `json:"error,omitempty"`
}

func decodeStateCopy(w http.ResponseWriter, r *http.Request) (stateCopyRequest, bool) {
	var body stateCopyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items are required")

		return body, false
	}

	return body, true
}

// stateItemError — un emplacement qui ne se désigne pas : une console
// inconnue, un numéro hors bornes, une référence vide.
func stateItemError(item stateItem) error {
	if !consoles.Exists(item.ConsoleID) || item.Reference == "" || item.Slot < 0 || item.Slot >= retro_files.MaxSaveSlots() {
		return errors.New("invalid slot")
	}

	return nil
}

// handleStateBackup — POST /api/v1/saves/states/backup `{items}` : chaque
// emplacement du principal, copié dans `local`.
func (server *Server) handleStateBackup(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeStateCopy(w, r)
	if !ok {
		return
	}
	username := auth.Username(r.Context())
	results := []stateResult{}
	for _, item := range body.Items {
		err := stateItemError(item)
		if err == nil {
			err = saves.BackupSlot(server.retroSavesRoot(), username, item.ConsoleID, item.Reference, item.Slot)
		}
		results = append(results, stateResult{stateItem: item, Error: resultOf("", err).Error})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// handleStateRestore — POST /api/v1/saves/states/restore `{source, items}` :
// chaque emplacement de la copie, remis dans le principal ; ce que le
// principal avait à cet emplacement, dans `local`.
func (server *Server) handleStateRestore(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeStateCopy(w, r)
	if !ok {
		return
	}
	if body.Source == "" || !saves.ValidSource(body.Source) {
		writeError(w, http.StatusBadRequest, "a copy to restore from is required")

		return
	}
	username := auth.Username(r.Context())
	results := []stateResult{}
	for _, item := range body.Items {
		target := item.ToReference
		if target == "" {
			target = item.Reference
		}
		err := stateItemError(item)
		if err == nil {
			err = saves.RestoreSlot(server.retroSavesRoot(), username, body.Source, item.ConsoleID, item.Reference, target, item.Slot)
		}
		results = append(results, stateResult{stateItem: item, Error: resultOf("", err).Error})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// streamexSessions — les parties streamex en cours ; aucune sans gestionnaire
// (un test, comme storage_cache_api.go).
func (server *Server) streamexSessions() []manager.Session {
	if server.manager == nil {
		return nil
	}

	return server.manager.List()
}
