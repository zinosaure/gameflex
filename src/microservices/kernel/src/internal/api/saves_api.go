package api

// LES SAUVEGARDES PAR SOURCE (chantier 38, le porteur, 2026-10-28 et 29) :
// un principal, que les émulateurs jouent, et des copies — `local` pour les
// backups, une par serveur fédéré (shared/storage/saves).
//
//	GET    /api/v1/saves/sources                 le nom de ce serveur, et les copies du compte
//	GET    /api/v1/saves/streamex?source=        les jeux d'un rangement, console par console
//	GET    /api/v1/saves/streamex/game?source=&console=&id=   l'archive d'un jeu
//	POST   /api/v1/saves/streamex/game?source=&console=&id=   une archive, dans une copie
//	DELETE /api/v1/saves/streamex/game?source=&console=&id=   un jeu, effacé d'une copie ou du principal
//	POST   /api/v1/saves/streamex/backup         des jeux du principal, copiés dans local
//	POST   /api/v1/saves/streamex/restore        des jeux d'une copie, remis dans le principal
//	POST   /api/v1/saves/states/backup           des emplacements du rétro, dans local
//	POST   /api/v1/saves/states/restore          des emplacements d'une copie, dans le principal
//	GET    /api/v1/saves/notes                   les notes des backups (saves/notes.go)
//	PUT    /api/v1/saves/notes                   la note d'un backup : écrite, corrigée, ou vidée
//
// Un backup emporte sa note (`note`, facultative, dans le corps des deux
// routes `backup`) : pourquoi on l'a fait, et quand.
//
// Le rétro se lit et s'envoie par ses routes de toujours, `?source=` en plus
// (`/api/v1/retrogaming/savestate/*`).
//
// CE QU'UNE SESSION VENUE D'UN AUTRE SERVEUR PEUT FAIRE ICI : lire, et
// déposer dans une copie — c'est tout l'objet de la fédération des
// sauvegardes. Effacer, faire un backup, le noter et restaurer restent des gestes d'ici
// (`localOnly`) : « ce serait à l'utilisateur ensuite de choisir de faire la
// synchronisation entre le dossier principal et les sources » (le porteur).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/manager"
	"gameflex/kernel/internal/retro_files"
	"gameflex/metadata"
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
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid save source")

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
	router.Get("/notes", server.handleSaveNotes)
	router.Put("/notes", localOnly(server.handleSaveNote))

	return router
}

// handleSaveSources — GET /api/v1/saves/sources. `self` : le nom que ce
// serveur annonce à la fédération — celui sous lequel un envoi d'ici arrive
// chez un autre (saves.SourceName).
func (server *Server) handleSaveSources(w http.ResponseWriter, r *http.Request) {
	names, err := saves.Sources(server.retroSavesRoot(), auth.Username(r.Context()))
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "saves_failed", "cannot list the save copies")

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"self": server.federationHostname(), "sources": names})
}

// handleStreamexSaves — GET /api/v1/saves/streamex?source=. `consoles` : celles
// dont les sauvegardes se copient (saves.Consoles — la Xbox 360 n'y est pas
// encore). Chaque jeu porte le titre de son jeu d'ici quand on le trouve, sinon
// celui que la copie a gardé (metadata.NameSaves, chantier 76).
func (server *Server) handleStreamexSaves(w http.ResponseWriter, r *http.Request) {
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	tree := filepath.Join(root, owner)
	writeJSON(w, http.StatusOK, map[string]any{
		"consoles": saves.Consoles(),
		"games":    metadata.NameSaves(tree, saves.Games(tree), server.saveLibrary(r.Context())),
	})
}

// saveLibrary — les jeux d'une console qui peuvent nommer une sauvegarde : la
// bibliothèque, puis chaque disque interne présent, tels que leurs catalogues
// les gardent (retro_files.ListCatalog ne reparcourt un dossier que s'il a
// changé). Les disques ne se lisent qu'à la première console demandée.
func (server *Server) saveLibrary(ctx context.Context) func(string) []metadata.LibraryGame {
	var disks []docker.InternalDisk
	disksRead := false

	return func(consoleID string) []metadata.LibraryGame {
		if !disksRead {
			disks, disksRead = server.presentDisks(ctx), true
		}
		games := []metadata.LibraryGame{}
		add := func(root, disk string) {
			roms, err := retro_files.ListCatalog(root, consoleID, "")
			if err != nil {
				return
			}
			paths, _ := retro_files.CatalogPaths(root, consoleID)
			for _, rom := range roms {
				if path, found := paths[rom.Reference]; found {
					games = append(games, metadata.LibraryGame{Path: path, RelPath: relativeTo(root, path), Reference: rom.Reference, Title: rom.Title, Disk: disk})
				}
			}
		}
		add(server.retroRomsRoot(), "")
		for _, disk := range disks {
			add(disk.RomsDir(), disk.Name)
		}

		return games
	}
}

// rememberLaunched — LE NOM DE LA SAUVEGARDE, RETENU DÈS LE LANCEMENT (chantier 76) : le jeu lancé tel que la liste des
// sauvegardes le nommerait (saveLibrary : son titre, sa référence, son chemin), sous son identifiant de titre (GameKey)
// — la règle de metadata.NameSaves, et celle du téléphone (rememberLaunched). Elle garde ainsi son nom même si le jeu
// quitte la bibliothèque avant qu'une liste l'ait nommée. Le catalogue du lancement, lui, nomme un jeu par son fichier
// (« FINAL FANTASY II [01006B7014156000][USA][v0] ») : vu à l'épreuve du 2026-11-23.
func (server *Server) rememberLaunched(ctx context.Context, username, console, reference string) {
	if reference == "" || !saves.Supported(console) {
		return
	}
	tree, err := saves.Tree(server.retroSavesRoot(), username, "")
	if err != nil {
		return
	}
	for _, game := range server.saveLibrary(ctx)(console) {
		if game.Reference != reference {
			continue
		}
		if err := saves.Remember(tree, saves.Named{Console: console, Save: metadata.GameKey(game.Path, game.Reference),
			Game: saves.KnownGame{Title: game.Title, ID: game.Reference, Path: game.RelPath}}); err != nil {
			slog.Warn("save title not kept", "console", console, "error", err)
		}

		return
	}
}

// relativeTo — un chemin de jeu depuis son dossier des jeux (« ps5/Astro Bot ») :
// ce que garde `titles.json` (chantier 76), lisible d'une machine à l'autre.
func relativeTo(root, path string) string {
	if relative, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(relative, "..") {
		return filepath.ToSlash(relative)
	}

	return ""
}

// streamexGame — la console et le jeu d'une requête, vérifiés. Faux : l'erreur
// est écrite.
func streamexGame(w http.ResponseWriter, r *http.Request) (console, id string, ok bool) {
	console, id = r.URL.Query().Get("console"), r.URL.Query().Get("id")
	if !saves.Supported(console) || id == "" {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "a supported console and a game id are required")

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
		writeErrorCode(w, http.StatusNotFound, "save_not_found", "no save for this game")

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
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "source is required: an upload only writes into a copy")

		return
	}
	console, id, ok := streamexGame(w, r)
	if !ok {
		return
	}
	tree, err := saves.Tree(server.retroSavesRoot(), auth.Username(r.Context()), source)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid save source")

		return
	}
	if err := os.MkdirAll(tree, 0o755); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "saves_failed", "cannot create the copy")

		return
	}
	if r.ContentLength > 0 {
		if err := fitsOnDisk(tree, uint64(r.ContentLength)); err != nil {
			writeErrorCode(w, http.StatusInsufficientStorage, "saves_failed", err.Error())

			return
		}
	}

	upload, err := os.CreateTemp(tree, ".upload-*.zip")
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "saves_failed", "cannot receive the archive")

		return
	}
	defer os.Remove(upload.Name())
	_, copyErr := io.Copy(upload, http.MaxBytesReader(w, r.Body, maxStreamexSaveBytes))
	closeErr := upload.Close()
	var tooLarge *http.MaxBytesError
	if errors.As(copyErr, &tooLarge) {
		writeErrorCode(w, http.StatusRequestEntityTooLarge, "save_too_large", "the save is too large")

		return
	}
	if copyErr != nil || closeErr != nil {
		writeErrorCode(w, http.StatusInternalServerError, "saves_failed", "cannot receive the archive")

		return
	}

	err = saves.Receive(upload.Name(), tree, console, id, func(size uint64) error { return fitsOnDisk(tree, size) })
	if errors.Is(err, saves.ErrForeignEntry) {
		writeErrorCode(w, http.StatusBadRequest, "saves_failed", err.Error())

		return
	}
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "saves_failed", fmt.Sprintf("cannot store the save: %v", err))

		return
	}
	// LE NOM DU JEU VOYAGE AVEC LA SAUVEGARDE (chantier 76), comme pour le rétro.
	if err := saves.Remember(tree, saves.Named{Console: console, Save: id, Game: saves.Announced(r.URL.Query(), "")}); err != nil {
		slog.Warn("save title not kept", "source", source, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "source": source})
}

// handleStreamexSaveDelete — DELETE /api/v1/saves/streamex/game : un jeu, effacé
// d'une copie ou du principal (le porteur, 2026-11-23 : « rajouter dans la fiche
// des sauvegardes la section danger pour pouvoir supprimer les sauvegardes » ; la
// page Stockage de l'administration, qui effaçait le principal, est partie avec la
// webapp). Le principal, pas pendant une partie de cette console : elle écrit dans
// ce dossier — le même refus que la restauration.
func (server *Server) handleStreamexSaveDelete(w http.ResponseWriter, r *http.Request) {
	root, owner, ok := server.saveTree(w, r)
	if !ok {
		return
	}
	console, id, ok := streamexGame(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("source") == "" {
		username := auth.Username(r.Context())
		for _, session := range server.streamexSessions() {
			if session.Owner == username && session.Console == console {
				writeErrorCode(w, http.StatusConflict, "game_running", "a game of this console is running: stop it first")

				return
			}
		}
	}
	if err := saves.Remove(filepath.Join(root, owner), console, id); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "delete_failed", fmt.Sprintf("cannot delete: %v", err))

		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// streamexCopyRequest — le corps d'un backup ou d'une restauration. `Note` :
// celle d'un backup, posée à côté de chaque jeu copié (saves.WriteNote).
type streamexCopyRequest struct {
	Source  string   `json:"source"`
	Console string   `json:"console"`
	IDs     []string `json:"ids"`
	Note    string   `json:"note"`
}

func decodeStreamexCopy(w http.ResponseWriter, r *http.Request) (streamexCopyRequest, bool) {
	var body streamexCopyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid body")

		return body, false
	}
	if !saves.Supported(body.Console) || len(body.IDs) == 0 {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "a supported console and game ids are required")

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
// `{console, ids, note?}` : chaque jeu du principal, copié dans `local`, avec
// la note — jugée avant le premier jeu (saves.CheckNote).
func (server *Server) handleStreamexSaveBackup(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeStreamexCopy(w, r)
	if !ok {
		return
	}
	if err := saves.CheckNote(body.Note); err != nil {
		writeErrorCode(w, http.StatusUnprocessableEntity, "saves_failed", err.Error())

		return
	}
	username := auth.Username(r.Context())
	results := []copyResult{}
	for _, id := range body.IDs {
		err := saves.Backup(server.retroSavesRoot(), username, body.Console, id)
		if err == nil {
			_, err = saves.WriteNote(server.retroSavesRoot(), username, saves.NoteItem{Kind: saves.KindStreamex, Console: body.Console, ID: id}, body.Note, time.Now())
		}
		results = append(results, resultOf(id, err))
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
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "a copy to restore from is required")

		return
	}
	username := auth.Username(r.Context())
	for _, session := range server.streamexSessions() {
		if session.Owner == username && session.Console == body.Console {
			writeErrorCode(w, http.StatusConflict, "game_running", "a game of this console is running: stop it first")

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
	Note   string      `json:"note"`
}

type stateResult struct {
	stateItem
	Error string `json:"error,omitempty"`
}

func decodeStateCopy(w http.ResponseWriter, r *http.Request) (stateCopyRequest, bool) {
	var body stateCopyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || len(body.Items) == 0 {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "items are required")

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

// handleStateBackup — POST /api/v1/saves/states/backup `{items, note?}` :
// chaque emplacement du principal, copié dans `local`, avec la note.
func (server *Server) handleStateBackup(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeStateCopy(w, r)
	if !ok {
		return
	}
	if err := saves.CheckNote(body.Note); err != nil {
		writeErrorCode(w, http.StatusUnprocessableEntity, "saves_failed", err.Error())

		return
	}
	username := auth.Username(r.Context())
	results := []stateResult{}
	for _, item := range body.Items {
		err := stateItemError(item)
		if err == nil {
			err = saves.BackupSlot(server.retroSavesRoot(), username, item.ConsoleID, item.Reference, item.Slot)
		}
		if err == nil {
			_, err = saves.WriteNote(server.retroSavesRoot(), username, saves.NoteItem{Kind: saves.KindStates, Console: item.ConsoleID, Reference: item.Reference, Slot: item.Slot}, body.Note, time.Now())
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
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "a copy to restore from is required")

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

// handleSaveNotes — GET /api/v1/saves/notes : les notes des backups du compte
// (saves.Notes), chacune avec la sauvegarde qu'elle accompagne.
func (server *Server) handleSaveNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := saves.Notes(server.retroSavesRoot(), auth.Username(r.Context()))
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "saves_failed", "cannot read the notes")

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}

// noteRequest — une note, et la sauvegarde des backups qu'elle accompagne.
type noteRequest struct {
	saves.NoteItem
	Note string `json:"note"`
}

// handleSaveNote — PUT /api/v1/saves/notes `{kind, console, reference + slot
// | id, note}` : la note d'une sauvegarde des backups, écrite ou corrigée ;
// vide, elle part. `item` : la note telle qu'elle est gardée, nulle quand il
// n'y en a plus.
func (server *Server) handleSaveNote(w http.ResponseWriter, r *http.Request) {
	var body noteRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid body")

		return
	}
	if body.Kind == saves.KindStates && stateItemError(stateItem{ConsoleID: body.Console, Reference: body.Reference, Slot: body.Slot}) != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid slot")

		return
	}
	item, err := saves.WriteNote(server.retroSavesRoot(), auth.Username(r.Context()), body.NoteItem, body.Note, time.Now())
	switch {
	case errors.Is(err, saves.ErrNoteTooLong):
		writeErrorCode(w, http.StatusUnprocessableEntity, "saves_failed", err.Error())
	case errors.Is(err, saves.ErrNotASave):
		writeErrorCode(w, http.StatusBadRequest, "saves_failed", err.Error())
	case errors.Is(err, os.ErrNotExist):
		writeErrorCode(w, http.StatusNotFound, "save_not_found", "the backups have no such save")
	case err != nil:
		writeErrorCode(w, http.StatusInternalServerError, "saves_failed", fmt.Sprintf("cannot write the note: %v", err))
	default:
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
	}
}

// streamexSessions — les parties streamex en cours ; aucune sans gestionnaire
// (un test, comme storage_cache_api.go).
func (server *Server) streamexSessions() []manager.Session {
	if server.manager == nil {
		return nil
	}

	return server.manager.List()
}
