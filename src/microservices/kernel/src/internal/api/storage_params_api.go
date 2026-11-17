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
// la bibliothèque (storage.AdoptParentOwner) : il tourne en root.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/emuparams"
	"gameflex/kernel/internal/config"
	"gameflex/storage"
	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

// paramsRoot — le dossier, où vivent `params/` et `patches/` ; une
// variable pour qu'un test pose ses fichiers ailleurs que dans la
// bibliothèque.
var paramsRoot = config.StorageDir

func paramsStore() emuparams.Store {
	return emuparams.Store{Root: paramsRoot, AfterWrite: storage.AdoptParentOwner}
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
		writeErrorCode(w, http.StatusNotFound, "not_found", "pas de paramètres pour "+ref.ConsoleID+"/"+ref.Emulator)

		return
	}
	var body struct {
		Enabled *bool                     `json:"enabled"`
		Values  map[string]any            `json:"values"`
		Preset  string                    `json:"preset"`
		Engines []emuparams.EngineSetting `json:"engines"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "corps invalide")

		return
	}
	saved, err := paramsStore().Save(ref, consoleNames()[ref.ConsoleID], body.Enabled, body.Values, body.Preset, body.Engines)
	var refused *emuparams.RefusedError
	switch {
	case errors.As(err, &refused):
		writeErrorCode(w, http.StatusBadRequest, "bad_request", refused.Error())
	case err != nil:
		writeErrorCode(w, http.StatusInternalServerError, "storage_failed", err.Error())
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
		writeErrorCode(w, http.StatusNotFound, "not_found", "pas de paramètres pour "+ref.ConsoleID+"/"+ref.Emulator)

		return
	}
	preset := chi.URLParam(r, "preset")
	described, err := paramsStore().DeletePreset(ref, consoleNames()[ref.ConsoleID], preset)
	var refused *emuparams.RefusedError
	switch {
	case errors.As(err, &refused):
		writeErrorCode(w, http.StatusBadRequest, "bad_request", refused.Error())
	case errors.Is(err, emuparams.ErrNoPreset):
		writeErrorCode(w, http.StatusNotFound, "storage_failed", err.Error())
	case err != nil:
		writeErrorCode(w, http.StatusInternalServerError, "storage_failed", err.Error())
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
		writeErrorCode(w, http.StatusNotFound, "not_found", "pas de paramètres pour "+ref.ConsoleID+"/"+ref.Emulator)

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
		writeErrorCode(w, http.StatusNotFound, "not_lent", "rien de prêté ici")

		return
	}

	writeJSON(w, http.StatusOK, lent)
}

// lentPreset — ce qu'une autre source prête des paramètres d'un émulateur — un
// serveur fédéré, un disque interne : ses valeurs choisies (`preset` vide), ou
// un de ses préréglages perso —, tel que le formulaire d'ici le prend : un
// préréglage de plus dans son sélecteur, ses valeurs passées par les paramètres
// d'ici (emuparams.Fit), celles qui ne remplissent rien dites dans `ignored`.
// Le choisir remplit les champs, et rien ne s'écrit avant « Enregistrer » (le
// porteur, 2026-11-14 : « au lieu de copier le fichier on va juste copier les
// données comme preset. Quand on choisit un preset venant d'un autre serveur,
// on charge que les données de celui-ci »). `error` : la source ne l'a pas
// rendu — `unreachable`, `denied` (notre jeton refusé), `lacks` (elle ne le
// prête plus).
type lentPreset struct {
	Emulator string         `json:"emulator"`
	Preset   string         `json:"preset,omitempty"`
	Name     string         `json:"name"`
	Values   map[string]any `json:"values"`
	Ignored  []string       `json:"ignored,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// paramsWanted — la section des paramètres d'une source, réduite aux
// émulateurs réglables ici, et à la console et à l'émulateur que la page
// demande (`?console=&emulator=`) : chaque prêt se lit chez la source, et la
// page des paramètres d'un émulateur n'en veut que les siens.
func paramsWanted(r *http.Request, items map[string][]listing.Item) map[string][]listing.Item {
	consoleID, emulator := r.URL.Query().Get("console"), r.URL.Query().Get("emulator")
	wanted := map[string][]listing.Item{}
	for id, list := range items {
		if consoleID != "" && id != consoleID {
			continue
		}
		for _, item := range list {
			if (emulator == "" || item.Name == emulator) && emuparams.IsShipped(emuparams.Ref{ConsoleID: id, Emulator: item.Name}) {
				wanted[id] = append(wanted[id], item)
			}
		}
	}

	return wanted
}

// lentSections — chaque prêt de la section (remote.ParamsSections), ses valeurs
// lues à la source par `lend` (rien, et la raison, quand elle ne les rend pas).
func lentSections(items map[string][]listing.Item, lend func(ref emuparams.Ref, presetID string) (map[string]any, string)) []remote.ParamsSection {
	store := paramsStore()

	return remote.ParamsSections(items, func(consoleID, emulator, presetID, presetName string) any {
		ref := emuparams.Ref{ConsoleID: consoleID, Emulator: emulator}
		lent := lentPreset{Emulator: emulator, Preset: presetID, Name: presetName, Values: map[string]any{}}
		if presetID == "" {
			lent.Name = emulator
		}
		values, failure := lend(ref, presetID)
		if failure != "" {
			lent.Error = failure

			return lent
		}
		lent.Values, lent.Ignored = store.Fit(ref, values)

		return lent
	})
}

// handleRemoteStorageParams — GET /api/v1/servers/{id}/storage/params
// [?console=&emulator=] : ce qu'un ami prête (la copie de son listing le dit),
// chaque prêt avec ses valeurs, demandées à lui (lentPreset). Un ami qui ne
// répond pas, ou refuse notre jeton, n'est demandé qu'une fois : ses autres
// prêts portent la même raison.
//
// L'IMPORT QUI COPIAIT CE PRÊT EN PRÉRÉGLAGE ICI EST PARTI (chantier 53, après
// la P7) : le formulaire prend ses valeurs, et « Enregistrer » les écrit dans
// le fichier d'ici — ou en préréglage d'ici, sous le nom qu'on lui donne.
func (server *Server) handleRemoteStorageParams(w http.ResponseWriter, r *http.Request) {
	server.serveRemoteStorage(w, r, listing.Params, func(entry registeredServer, items map[string][]listing.Item) any {
		client, err := remote.New(entry.Link, entry.Token)
		failed := ""
		if err != nil {
			failed = "unreachable"
		}

		return lentSections(paramsWanted(r, items), func(ref emuparams.Ref, presetID string) (map[string]any, string) {
			if failed != "" {
				return nil, failed
			}
			lent, err := client.Params(ref.ConsoleID, ref.Emulator, presetID)
			switch {
			case errors.Is(err, remote.ErrNotLent):
				return nil, "lacks"
			case errors.Is(err, remote.ErrDenied):
				failed = "denied"
			case err != nil:
				failed = "unreachable"
			default:
				return lent.Values, ""
			}
			slog.Info("peer params: not lent", "server", entry.Hostname, "console", ref.ConsoleID, "emulator", ref.Emulator, "error", err)

			return nil, failed
		})
	})
}
