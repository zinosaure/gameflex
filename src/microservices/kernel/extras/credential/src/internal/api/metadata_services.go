package api

// LES COMPTES DES SERVICES DE MÉTADONNÉES (chantier 49, #659) — IGDB et
// SteamGridDB : ce qu'il faut au kernel pour aller chercher les fonds,
// jaquettes et résumés que les jeux ne portent pas. ScreenScraper n'en est plus
// (le porteur, 2026-11-13 : « Oui, en plus c'est payant ») : il n'avait jamais
// été écrit côté kernel, seul son compte se saisissait.
//
// Même règle que les partages montés : les valeurs entrent par la page
// d'administration, sont scellées en base (secretbox, la clé de GameFlex), et
// ne ressortent JAMAIS par l'API d'administration — elle dit seulement quels
// champs sont remplis. Le kernel les lit par sa route interne.

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/secretbox"
)

// metadataServiceFields — les services connus, leurs champs, et ceux qu'il
// faut pour que le service soit utilisable.
var metadataServiceFields = map[string]struct {
	fields   []string
	required []string
}{
	"igdb":        {fields: []string{"client_id", "client_secret"}, required: []string{"client_id", "client_secret"}},
	"steamgriddb": {fields: []string{"api_key"}, required: []string{"api_key"}},
}

type metadataServiceResponse struct {
	Service    string   `json:"service"`
	Configured bool     `json:"configured"`
	Fields     []string `json:"fields"` // les champs remplis, jamais leurs valeurs
	Required   []string `json:"required"`
	UpdatedAt  any      `json:"updated_at"`
}

// handleListMetadataServices — GET /api/v1/metadata-services : chaque service
// connu, configuré ou non.
func (server *Server) handleListMetadataServices(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	stored, updated, err := server.readMetadataServices()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	names := make([]string, 0, len(metadataServiceFields))
	for name := range metadataServiceFields {
		names = append(names, name)
	}
	sort.Strings(names)

	items := []metadataServiceResponse{}
	for _, name := range names {
		spec := metadataServiceFields[name]
		item := metadataServiceResponse{Service: name, Fields: []string{}, Required: spec.required, UpdatedAt: updated[name]}
		for _, field := range spec.fields {
			if stored[name][field] != "" {
				item.Fields = append(item.Fields, field)
			}
		}
		item.Configured = len(stored[name]) > 0
		for _, field := range spec.required {
			if stored[name][field] == "" {
				item.Configured = false
			}
		}
		items = append(items, item)
	}

	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

// handlePutMetadataService — PUT /api/v1/metadata-services/{service}
// {fields: {client_id: "…"}}. Un champ absent ou vide garde sa valeur : la
// page n'a jamais la valeur à renvoyer, elle ne renvoie que ce qui change.
func (server *Server) handlePutMetadataService(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	name := r.PathValue("service")
	spec, known := metadataServiceFields[name]
	if !known {
		respondErrorCode(w, http.StatusNotFound, "not_found", "Unknown metadata service.")
		return
	}
	var body struct {
		Fields map[string]string `json:"fields"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	stored, _, err := server.readMetadataServices()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	values := stored[name]
	if values == nil {
		values = map[string]string{}
	}
	for field, value := range body.Fields {
		if !contains(spec.fields, field) {
			respondValidationError(w, []map[string]any{{
				"loc": []string{"body", "fields", field}, "msg": "Unknown field for this service.", "type": "value_error",
			}})
			return
		}
		if value = strings.TrimSpace(value); value != "" {
			values[field] = value
		}
	}
	for _, field := range spec.required {
		if values[field] == "" {
			respondValidationError(w, []map[string]any{{
				"loc": []string{"body", "fields", field}, "msg": "This field is required.", "type": "value_error.missing",
			}})
			return
		}
	}

	plain, _ := json.Marshal(values)
	sealed, err := secretbox.Seal(server.config.JWTSecret, string(plain))
	if err != nil {
		slog.Error("metadata service: cannot seal its account", "error", err)
		respondErrorCode(w, http.StatusInternalServerError, "secret_missing", "This server cannot store secrets: its secret is not set.")
		return
	}
	if _, err := server.db.Exec(
		`INSERT INTO metadata_services (service, fields, updated_at, updated_by_id) VALUES (?, ?, ?, ?)
		 ON CONFLICT(service) DO UPDATE SET fields = excluded.fields, updated_at = excluded.updated_at, updated_by_id = excluded.updated_by_id`,
		name, sealed, now(), principal.ID,
	); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	server.handleListMetadataServices(w, r, principal)
}

// handleDeleteMetadataService — DELETE /api/v1/metadata-services/{service}.
func (server *Server) handleDeleteMetadataService(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if _, known := metadataServiceFields[r.PathValue("service")]; !known {
		respondErrorCode(w, http.StatusNotFound, "not_found", "Unknown metadata service.")
		return
	}
	if _, err := server.db.Exec(`DELETE FROM metadata_services WHERE service = ?`, r.PathValue("service")); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	server.handleListMetadataServices(w, r, principal)
}

// handleInternalMetadataServices — GET /internal/metadata/services : les
// comptes en clair, pour le kernel seul.
func (server *Server) handleInternalMetadataServices(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	stored, _, err := server.readMetadataServices()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	respondItem(w, http.StatusOK, stored)
}

// readMetadataServices — les comptes ouverts, par service, et leur date. Un
// compte illisible (le secret a changé) est ignoré et le journal le dit : il
// se ressaisit.
func (server *Server) readMetadataServices() (map[string]map[string]string, map[string]any, error) {
	rows, err := server.db.Query(`SELECT service, fields, updated_at FROM metadata_services`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	stored, updated := map[string]map[string]string{}, map[string]any{}
	for rows.Next() {
		var name, sealed string
		var updatedAt sql.NullString
		if err := rows.Scan(&name, &sealed, &updatedAt); err != nil {
			return nil, nil, err
		}
		opened, err := secretbox.Open(server.config.JWTSecret, sealed)
		if err != nil {
			slog.Warn("metadata service: account cannot be opened", "service", name, "error", err)
			continue
		}
		values := map[string]string{}
		if err := json.Unmarshal([]byte(opened), &values); err != nil {
			continue
		}
		stored[name] = values
		updated[name] = isoDatetime(updatedAt)
	}

	return stored, updated, rows.Err()
}

// LES RÉGLAGES DES MÉTADONNÉES, à côté des comptes (le porteur, 2026-11-14) —
// un seul : écrire les métadonnées d'un jeu sur le disque qui le porte, ou dans
// le dossier du serveur. « Sur téléphone, quand on rajoute un disque NTFS, le
// disque devient read-only. Ce qui fait que c'est impossible d'écrire sur le
// disque. Du coup ce qu'il faut […] c'est de rajouter un switch qui demande si
// on veut écrire les metadata sur le même disque où le jeu se trouve. Par
// défaut c'est checked. » Coupé, les métadonnées d'un jeu d'un disque se lisent
// et s'écrivent dans le dossier du serveur (« si le Switch est off alors les
// metadata s'écrit dans le dossier principal gameflex/metadata ») : le kernel
// en décide en un seul point (gameMetadataRoots). Gardé dans la table
// `settings`, créée au besoin comme pour la clé VAPID (le téléphone ne l'a pas
// dans son schéma).
type metadataSettings struct {
	WriteOnGameDisk bool `json:"write_on_game_disk"`
}

const writeOnGameDiskKey = "metadata_write_on_game_disk"

func (server *Server) readMetadataSettings() (metadataSettings, error) {
	settings := metadataSettings{WriteOnGameDisk: true}
	if _, err := server.db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		return settings, err
	}
	var stored string
	err := server.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, writeOnGameDiskKey).Scan(&stored)
	if err == nil {
		settings.WriteOnGameDisk = stored != "0"
	} else if err != sql.ErrNoRows {
		return settings, err
	}

	return settings, nil
}

// handleGetMetadataSettings — GET /api/v1/metadata-services/settings : pour
// tout compte (un écran peut avoir à le dire).
func (server *Server) handleGetMetadataSettings(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	settings, err := server.readMetadataSettings()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	respondItem(w, http.StatusOK, settings)
}

// handlePutMetadataSettings — PUT /api/v1/metadata-services/settings
// {write_on_game_disk} : un administrateur seulement. Le champ est exigé : un
// corps vide ne remet pas le réglage par défaut en silence.
func (server *Server) handlePutMetadataSettings(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		WriteOnGameDisk *bool `json:"write_on_game_disk"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.WriteOnGameDisk == nil {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "write_on_game_disk is required.")
		return
	}
	if _, err := server.readMetadataSettings(); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	value := "0"
	if *body.WriteOnGameDisk {
		value = "1"
	}
	if _, err := server.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, writeOnGameDiskKey, value); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	server.handleGetMetadataSettings(w, r, principal)
}

// handleInternalMetadataSettings — GET /internal/metadata/settings : pour le
// kernel, qui choisit d'après lui où vivent les métadonnées d'un jeu.
func (server *Server) handleInternalMetadataSettings(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	settings, err := server.readMetadataSettings()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	respondItem(w, http.StatusOK, settings)
}

// MetadataAccounts — les comptes des services en clair, pour le kernel du TÉLÉPHONE (chantier 57, P4), qui embarque ce
// service dans son propre processus (gameflex/credential/embed) : ce que la route interne rend au kernel d'un serveur,
// sans port ni secret pour se parler à soi-même.
func (server *Server) MetadataAccounts() (map[string]map[string]string, error) {
	stored, _, err := server.readMetadataServices()

	return stored, err
}

// MetadataWriteOnGameDisk — l'interrupteur « Écrire les métadonnées sur le disque du jeu », pour le même kernel.
func (server *Server) MetadataWriteOnGameDisk() (bool, error) {
	settings, err := server.readMetadataSettings()

	return settings.WriteOnGameDisk, err
}
