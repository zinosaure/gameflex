package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"gameflex/federation/internal/store"
	"gameflex/logfile"
)

// LES JOURNAUX DU SERVICE (chantier 61 ; le porteur, 2026-11-18 : « on a oublié de rajouter une fonctionnalité de log
// dans gameflex-federation. C'est important je trouve »). Ceux de GameFlex, par le même paquet (gameflex/logfile) : le
// journal du jour, `<données>/logs/<jour>/federation.log`, ses niveaux et le journal d'accès HTTP au choix, les secrets
// masqués à la lecture comme au téléchargement. Un service qui tourne seul sur un NAS doit se lire sans `docker logs`.

// SettingLogs — les réglages des journaux, gardés dans la base du service.
const SettingLogs = "log_settings"

// LoadLogSettings — les réglages enregistrés, ceux de GameFlex par défaut.
func LoadLogSettings(st *store.Store) logfile.Settings {
	settings := logfile.DefaultSettings()
	if text := st.Setting(SettingLogs, ""); text != "" {
		var saved logfile.Settings
		if json.Unmarshal([]byte(text), &saved) == nil {
			if normalized, err := saved.Normalized(); err == nil {
				settings = normalized
			}
		}
	}

	return settings
}

// logView — ce que la page montre des réglages : seulement ce qui vaut pour le service (ni parties ni signalements).
type logView struct {
	Enabled bool     `json:"enabled"`
	Levels  []string `json:"levels"`
	HTTP    bool     `json:"http"`
}

func (s *Server) getLogSettings(w http.ResponseWriter, _ *http.Request) {
	current := logfile.Current()
	writeJSON(w, http.StatusOK, logView{Enabled: current.Kernel, Levels: current.Levels, HTTP: current.HTTP})
}

func (s *Server) putLogSettings(w http.ResponseWriter, r *http.Request) {
	var body logView
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	settings := logfile.Current()
	settings.Kernel, settings.Levels, settings.HTTP = body.Enabled, body.Levels, body.HTTP
	normalized, err := settings.Normalized()
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	text, _ := json.Marshal(normalized)
	if err := s.Store.SetSetting(SettingLogs, string(text)); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	logfile.Apply(normalized)
	writeJSON(w, http.StatusOK, logView{Enabled: normalized.Kernel, Levels: normalized.Levels, HTTP: normalized.HTTP})
}

func (s *Server) listLogs(w http.ResponseWriter, _ *http.Request) {
	entries, err := logfile.List(s.DataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "logs_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// logPathError — 400 un chemin qui sort du dossier des journaux, 404 un journal absent.
func logPathError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, logfile.ErrOutsideRoot):
		writeError(w, http.StatusBadRequest, "bad_path", err.Error())
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "not_found", "no such log")
	default:
		writeError(w, http.StatusInternalServerError, "logs_error", err.Error())
	}
}

// tailLog — les dernières lignes d'un journal, lisibles (heure, niveau, message, détails), les secrets masqués.
func (s *Server) tailLog(w http.ResponseWriter, r *http.Request) {
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 || lines > 2000 {
		lines = 300
	}
	content, err := logfile.Tail(s.DataDir, r.URL.Query().Get("path"), lines)
	if err != nil {
		logPathError(w, err)

		return
	}
	writeJSON(w, http.StatusOK, logfile.ParseTail(content))
}

// downloadLog — le journal entier, les secrets masqués.
func (s *Server) downloadLog(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	name := path[strings.LastIndex(path, "/")+1:]
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	if err := logfile.WriteRedacted(w, s.DataDir, path); err != nil {
		w.Header().Del("Content-Disposition")
		logPathError(w, err)
	}
}

func (s *Server) removeLog(w http.ResponseWriter, r *http.Request) {
	if err := logfile.Remove(s.DataDir, r.URL.Query().Get("path")); err != nil {
		logPathError(w, err)

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": r.URL.Query().Get("path")})
}
