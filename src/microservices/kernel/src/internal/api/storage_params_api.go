package api

// LES PARAMÈTRES DES ÉMULATEURS SUR LE SERVEUR (#347) —
// `params/<console>/<émulateur>.json` du dossier, à côté de
// `patches/`.
//
// Le format, les fichiers livrés et l'écriture dans le fichier de
// l'émulateur sont dans gameflex/emuparams, communs avec le téléphone : « c'est
// une feature en commun » (le porteur, 2026-10-12). Mêmes routes, même
// réponse : le même écran sert les deux.
//
// Le serveur ne pose AUCUN fichier : il n'imposait rien à RPCS3 (à part ce
// que son entrypoint règle lui-même), et sans fichier rien n'est appliqué —
// le comportement d'avant. Le formulaire part alors de la liste livrée, et
// l'enregistrer crée le fichier. Les préréglages, eux, se posent dès que
// l'écran s'ouvre.
//
// Ce que le kernel crée dans le dossier, il le rend au propriétaire de
// la bibliothèque (adoptParentOwner) : il tourne en root.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/emuparams"
	"gameflex/kernel/internal/config"
	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

// paramsRoot — le dossier, où vivent `params/` et `patches/` ; une
// variable pour qu'un test pose ses fichiers ailleurs que dans la
// bibliothèque.
var paramsRoot = config.StorageDir

func paramsStore() emuparams.Store {
	return emuparams.Store{Root: paramsRoot, AfterWrite: adoptParentOwner}
}

func consoleNames() map[string]string {
	names := map[string]string{}
	for _, console := range consoles.All() {
		names[console.ID] = console.Name
	}

	return names
}

// handleStorageParams — GET /api/v1/retrogaming/storage/params : chaque
// émulateur réglable, son fichier et ses préréglages.
func (server *Server) handleStorageParams(w http.ResponseWriter, r *http.Request) {
	store := paramsStore()
	if err := store.Ensure(); err != nil {
		slog.Warn("paramètres : préréglages incomplets", "error", err)
	}
	names := consoleNames()
	emulators := []emuparams.Emulator{}
	for _, ref := range emuparams.Shipped() {
		emulators = append(emulators, store.Describe(ref, names[ref.ConsoleID]))
	}

	writeJSON(w, http.StatusOK, map[string]any{"emulators": emulators})
}

// handleStorageParamsWrite — PUT /api/v1/retrogaming/storage/params/{consoleId}/{emulator}
// {enabled, values, preset, engines} : l'interrupteur et les valeurs du
// formulaire, et, avec `preset`, les mêmes valeurs gardées en préréglage sous ce
// nom ; avec `engines`, le préréglage que chaque moteur de jeu pose (chantier 36).
func (server *Server) handleStorageParamsWrite(w http.ResponseWriter, r *http.Request) {
	ref := emuparams.Ref{ConsoleID: chi.URLParam(r, "consoleId"), Emulator: chi.URLParam(r, "emulator")}
	if !emuparams.IsShipped(ref) {
		writeError(w, http.StatusNotFound, "pas de paramètres pour "+ref.ConsoleID+"/"+ref.Emulator)

		return
	}
	var body struct {
		Enabled *bool                     `json:"enabled"`
		Values  map[string]any            `json:"values"`
		Preset  string                    `json:"preset"`
		Engines []emuparams.EngineSetting `json:"engines"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corps invalide")

		return
	}
	saved, err := paramsStore().Save(ref, consoleNames()[ref.ConsoleID], body.Enabled, body.Values, body.Preset, body.Engines)
	var refused *emuparams.RefusedError
	switch {
	case errors.As(err, &refused):
		writeError(w, http.StatusBadRequest, refused.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		slog.Info("paramètres enregistrés", "console", ref.ConsoleID, "émulateur", ref.Emulator,
			"activés", saved.Enabled, "préréglage", saved.SavedPreset)
		writeJSON(w, http.StatusOK, saved)
	}
}

// handleStorageParamsPresetDelete — DELETE
// /api/v1/retrogaming/storage/params/{consoleId}/{emulator}/presets/{preset} :
// supprimer un préréglage de l'admin. Ceux que GameFlex livre sont refusés.
func (server *Server) handleStorageParamsPresetDelete(w http.ResponseWriter, r *http.Request) {
	ref := emuparams.Ref{ConsoleID: chi.URLParam(r, "consoleId"), Emulator: chi.URLParam(r, "emulator")}
	if !emuparams.IsShipped(ref) {
		writeError(w, http.StatusNotFound, "pas de paramètres pour "+ref.ConsoleID+"/"+ref.Emulator)

		return
	}
	preset := chi.URLParam(r, "preset")
	described, err := paramsStore().DeletePreset(ref, consoleNames()[ref.ConsoleID], preset)
	var refused *emuparams.RefusedError
	switch {
	case errors.As(err, &refused):
		writeError(w, http.StatusBadRequest, refused.Error())
	case errors.Is(err, emuparams.ErrNoPreset):
		writeError(w, http.StatusNotFound, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		slog.Info("préréglage supprimé", "console", ref.ConsoleID, "émulateur", ref.Emulator, "préréglage", preset)
		writeJSON(w, http.StatusOK, described)
	}
}

// handleFederationParams — GET /api/v1/federation/params/{consoleId}/{emulator}
// [/presets/{preset}] : ce que cet émulateur prête à un ami (chantier 15) —
// ses valeurs choisies, ou un de ses préréglages perso. Rien d'autre : un
// préréglage de GameFlex est le même chez lui.
func (server *Server) handleFederationParams(w http.ResponseWriter, r *http.Request) {
	ref := emuparams.Ref{ConsoleID: chi.URLParam(r, "consoleId"), Emulator: chi.URLParam(r, "emulator")}
	if !emuparams.IsShipped(ref) {
		writeError(w, http.StatusNotFound, "pas de paramètres pour "+ref.ConsoleID+"/"+ref.Emulator)

		return
	}
	// La bibliothèque, puis les disques internes (chantier 24).
	var lent emuparams.Lent
	err := emuparams.ErrNoPreset
	for _, roots := range server.libraryRootsInOrder(r.Context()) {
		if lent, err = (emuparams.Store{Root: roots.Params}).Lend(ref, chi.URLParam(r, "preset")); err == nil {
			break
		}
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "rien de prêté ici")

		return
	}

	writeJSON(w, http.StatusOK, lent)
}

// handleRemoteStorageParams — GET /api/v1/servers/{id}/storage/params : ce
// qu'un ami prête, lu dans la copie de son listing — chaque prêt dit le
// préréglage qu'il deviendrait ici (`<serveur>-<préréglage>`), et s'il y est
// déjà.
func (server *Server) handleRemoteStorageParams(w http.ResponseWriter, r *http.Request) {
	server.serveRemoteStorage(w, r, listing.Params, func(entry registeredServer, items map[string][]listing.Item) any {
		store := paramsStore()

		return remote.ParamsSections(items, func(consoleID, emulator, presetID, presetName string) any {
			return store.OfferOf(entry.Hostname, consoleID, emulator, presetID, presetName)
		})
	})
}

// importPeerParams — POST /api/v1/servers/{id}/storage/params {consoleId,
// emulator, preset} : ce qu'un ami prête, rangé ici en préréglage perso sous
// `<serveur>-<préréglage>` (ou `<serveur>` pour ses valeurs) — jamais
// par-dessus un autre, et seulement ce que les paramètres d'ici acceptent.
func (server *Server) importPeerParams(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConsoleID string `json:"consoleId"`
		Emulator  string `json:"emulator"`
		Preset    string `json:"preset"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corps invalide")

		return
	}
	ref := emuparams.Ref{ConsoleID: body.ConsoleID, Emulator: body.Emulator}
	if !emuparams.IsShipped(ref) {
		writeError(w, http.StatusNotFound, "pas de paramètres pour "+ref.ConsoleID+"/"+ref.Emulator)

		return
	}
	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}
	client, err := remote.New(entry.Link, entry.Token)
	if err != nil {
		writeError(w, http.StatusBadGateway, "this address did not answer")

		return
	}
	lent, err := client.Params(ref.ConsoleID, ref.Emulator, body.Preset)
	switch {
	case errors.Is(err, remote.ErrNotLent):
		writeError(w, http.StatusNotFound, "ce serveur ne prête pas (ou plus) ce réglage")

		return
	case err != nil:
		refused := remote.Refusal(err)
		writeError(w, refused.Status, refused.Message)

		return
	}

	presetName := ""
	if body.Preset != "" {
		presetName = lent.Name
	}
	imported, ignored, err := paramsStore().Import(ref, consoleNames()[ref.ConsoleID],
		emuparams.ImportName(entry.Hostname, presetName), lent.Values)
	answerParamsImport(w, ref, entry.Hostname, imported, ignored, err)
}

// answerParamsImport — la réponse à un import de paramètres, d'un ami ou d'un
// disque interne (`source`) : la même pour les deux.
func answerParamsImport(w http.ResponseWriter, ref emuparams.Ref, source string, imported emuparams.Emulator, ignored []string, err error) {
	var refused *emuparams.RefusedError
	switch {
	case errors.Is(err, emuparams.ErrExists):
		writeError(w, http.StatusConflict, "ce préréglage est déjà ici : le supprimer d'abord pour en reprendre une version")
	case errors.As(err, &refused):
		writeError(w, http.StatusBadRequest, refused.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		slog.Info("préréglage importé", "source", source, "console", ref.ConsoleID,
			"émulateur", ref.Emulator, "préréglage", imported.SavedPreset, "écartés", ignored)
		writeJSON(w, http.StatusOK, map[string]any{
			"section": "params", "preset": imported.SavedPreset, "ignored": ignored, "source": source,
		})
	}
}
