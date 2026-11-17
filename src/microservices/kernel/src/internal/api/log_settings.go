package api

// CE QUI S'ÉCRIT DANS LES JOURNAUX (le porteur, 2026-11-14 : « Il faut que dans
// logs on puisse avoir la possibilité de l'activer ou pas et aussi choisir les
// level à afficher si on active le log. Comme tu peux voir sinon, on a un log du
// kernel à 18mb ») — le journal du kernel ou non, ses niveaux, les requêtes HTTP
// à part, les journaux des parties ou non, les signalements ou non (le porteur :
// « Oui pour le reports.log » ; gameflex/logfile, Settings). Les FICHIERS
// seulement : la sortie standard ne change pas.
//
// GARDÉ PAR LE KERNEL, comme le défaut d'une console (console_defaults.go), et
// pas par le service des comptes comme l'interrupteur des métadonnées : c'est le
// kernel qui écrit ces journaux, et il écrit sa première ligne avant que le
// service des comptes réponde. Dans le dossier des journaux,
// `logs/settings.json` — que leur liste ignore (elle ne montre que des `.log`)
// et que leur suppression refuse. Lu au démarrage (cmd/sessions/main.go) ;
// changé ici, il vaut dès la ligne suivante, sans redémarrer.
//
//	GET /api/v1/retrogaming/storage/logs/settings   ce qui s'écrit
//	PUT /api/v1/retrogaming/storage/logs/settings   {kernel?, levels?, http?, sessions?, reports?} — administration
//
// Le téléphone sert les mêmes routes pour le seul journal de son kernel
// (`{kernel}`, mobileapp/kernel/logging.go) ; la page ne montre que ce qu'un
// hôte rend, et seulement à un hôte qui déclare `logSettings` (capabilities.go).

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"gameflex/kernel/internal/auth"
	"gameflex/logfile"
	"gameflex/storage"
)

// Un écrivain à la fois : deux administrateurs qui règlent en même temps ne
// mêlent pas leurs changements.
var logSettingsMu sync.Mutex

func logSettingsPath(root string) string {
	return filepath.Join(logfile.LogsRoot(root), "settings.json")
}

// LoadLogSettings — le réglage enregistré, sinon le défaut ; un fichier
// illisible donne le défaut aussi (le journal le dit). Une clé absente garde
// sa valeur par défaut.
func LoadLogSettings(root string) logfile.Settings {
	raw, err := os.ReadFile(logSettingsPath(root))
	if errors.Is(err, fs.ErrNotExist) {
		return logfile.DefaultSettings()
	}
	settings := logfile.DefaultSettings()
	if err == nil {
		err = json.Unmarshal(raw, &settings)
	}
	if err == nil {
		settings, err = settings.Normalized()
	}
	if err != nil {
		slog.Warn("log settings unreadable: the defaults apply", "error", err)

		return logfile.DefaultSettings()
	}

	return settings
}

func writeLogSettings(root string, settings logfile.Settings) error {
	path := logSettingsPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	storage.AdoptParentOwner(filepath.Dir(path))
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".part"
	if err := os.WriteFile(temporary, encoded, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	storage.AdoptParentOwner(path)

	return nil
}

// handleGetLogSettings — GET /api/v1/retrogaming/storage/logs/settings.
func (server *Server) handleGetLogSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, logfile.Current())
}

// handlePutLogSettings — PUT /api/v1/retrogaming/storage/logs/settings. Ce
// que le corps ne dit pas ne change pas : la page envoie l'interrupteur qu'on
// vient de toucher, rien d'autre.
func (server *Server) handlePutLogSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kernel   *bool     `json:"kernel"`
		Levels   *[]string `json:"levels"`
		HTTP     *bool     `json:"http"`
		Sessions *bool     `json:"sessions"`
		Reports  *bool     `json:"reports"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid request body")

		return
	}

	logSettingsMu.Lock()
	defer logSettingsMu.Unlock()
	settings := logfile.Current()
	if body.Kernel != nil {
		settings.Kernel = *body.Kernel
	}
	if body.Levels != nil {
		settings.Levels = *body.Levels
	}
	if body.HTTP != nil {
		settings.HTTP = *body.HTTP
	}
	if body.Sessions != nil {
		settings.Sessions = *body.Sessions
	}
	if body.Reports != nil {
		settings.Reports = *body.Reports
	}
	settings, err := settings.Normalized()
	if err != nil {
		writeErrorCode(w, http.StatusUnprocessableEntity, "logs_failed", err.Error())

		return
	}

	if err := writeLogSettings(server.logsRoot(), settings); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "logs_failed", err.Error())

		return
	}
	logfile.Apply(settings)
	slog.Info("log settings set", "kernel", settings.Kernel, "levels", settings.Levels, "http", settings.HTTP,
		"sessions", settings.Sessions, "reports", settings.Reports, "by", auth.Username(r.Context()))
	writeJSON(w, http.StatusOK, settings)
}
