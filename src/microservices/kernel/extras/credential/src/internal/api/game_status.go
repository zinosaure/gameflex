package api

// LE STATUT DES CONSOLES ET DES JEUX, EN BASE (chantier 30 ; le porteur, 2026-11-17 : « Il faut que ça soit dans la base
// de données. On ne va pas créer des fichiers pour tout les 20000 jeux »). La forme est celle de gamestatus.Console
// (gameflex/storage/gamestatus) : le kernel du serveur la lit et l'écrit par les routes /internal/game-status, celui du
// téléphone par credential/embed — les mêmes fonctions. Les règles (statuts valides, note bornée, administrateur)
// restent celles du kernel ; ici, le rangement.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// GameStatusReport — un signalement, tel que gamestatus.Report le sérialise.
type GameStatusReport struct {
	Status    string            `json:"status"`
	Emulators []string          `json:"emulators,omitempty"`
	Versions  map[string]string `json:"versions,omitempty"`
	Note      string            `json:"note,omitempty"`
	By        string            `json:"by"`
	At        time.Time         `json:"at"`
}

// GameStatusConsole — une console : son statut et les signalements de ses jeux (gamestatus.Console).
type GameStatusConsole struct {
	Status string                        `json:"status,omitempty"`
	By     string                        `json:"by,omitempty"`
	At     *time.Time                    `json:"at,omitempty"`
	Games  map[string][]GameStatusReport `json:"games,omitempty"`
}

// LoadGameStatus — une console, vide quand rien n'en parle.
func LoadGameStatus(db *sql.DB, consoleID string) (GameStatusConsole, error) {
	var console GameStatusConsole
	var by sql.NullString
	var at sql.NullTime
	err := db.QueryRow(`SELECT status, set_by, set_at FROM game_status_consoles WHERE console_id = ?`, consoleID).Scan(&console.Status, &by, &at)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return console, err
	}
	console.By = by.String
	if at.Valid {
		stamp := at.Time.UTC()
		console.At = &stamp
	}
	rows, err := db.Query(`SELECT game_id, status, emulators, versions, note, reported_by, reported_at
		FROM game_status_reports WHERE console_id = ? ORDER BY game_id, id`, consoleID)
	if err != nil {
		return console, err
	}
	defer rows.Close()
	for rows.Next() {
		var gameID string
		var report GameStatusReport
		var emulators, versions, note, reportedBy sql.NullString
		var reportedAt sql.NullTime
		if err := rows.Scan(&gameID, &report.Status, &emulators, &versions, &note, &reportedBy, &reportedAt); err != nil {
			return console, err
		}
		if emulators.String != "" {
			_ = json.Unmarshal([]byte(emulators.String), &report.Emulators)
		}
		if versions.String != "" {
			_ = json.Unmarshal([]byte(versions.String), &report.Versions)
		}
		report.Note, report.By = note.String, reportedBy.String
		if reportedAt.Valid {
			report.At = reportedAt.Time.UTC()
		}
		if console.Games == nil {
			console.Games = map[string][]GameStatusReport{}
		}
		console.Games[gameID] = append(console.Games[gameID], report)
	}

	return console, rows.Err()
}

// AddGameStatusReport — un signalement de plus pour ce jeu.
func AddGameStatusReport(db *sql.DB, consoleID, gameID string, report GameStatusReport) error {
	var emulators, versions []byte
	if len(report.Emulators) > 0 {
		emulators, _ = json.Marshal(report.Emulators)
	}
	if len(report.Versions) > 0 {
		versions, _ = json.Marshal(report.Versions)
	}
	_, err := db.Exec(`INSERT INTO game_status_reports (console_id, game_id, status, emulators, versions, note, reported_by, reported_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, consoleID, gameID, report.Status, nullable(emulators), nullable(versions), report.Note, report.By, report.At.UTC())

	return err
}

// SetGameStatusConsole — le statut de toute la console.
func SetGameStatusConsole(db *sql.DB, consoleID, status, by string, at time.Time) error {
	_, err := db.Exec(`INSERT INTO game_status_consoles (console_id, status, set_by, set_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(console_id) DO UPDATE SET status = excluded.status, set_by = excluded.set_by, set_at = excluded.set_at`,
		consoleID, status, by, at.UTC())

	return err
}

// GameStatuses — le statut de chaque console réglée (l'en-tête du listing).
func GameStatuses(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT console_id, status FROM game_status_consoles`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statuses := map[string]string{}
	for rows.Next() {
		var consoleID, status string
		if err := rows.Scan(&consoleID, &status); err != nil {
			return nil, err
		}
		statuses[consoleID] = status
	}

	return statuses, rows.Err()
}

func nullable(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}

	return string(raw)
}

// ── Les routes du kernel (le secret interne) ─────────────────────────────

// handleInternalGameStatus — GET /internal/game-status/{console} : la console et ses signalements.
func (server *Server) handleInternalGameStatus(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	console, err := LoadGameStatus(server.db, r.PathValue("console"))
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	respondItem(w, http.StatusOK, console)
}

// handleInternalGameStatuses — GET /internal/game-status : le statut de chaque console réglée.
func (server *Server) handleInternalGameStatuses(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	statuses, err := GameStatuses(server.db)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	respondItem(w, http.StatusOK, statuses)
}

// handleInternalGameStatusReport — POST /internal/game-status/{console}/games/{game} : un signalement.
func (server *Server) handleInternalGameStatusReport(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	var report GameStatusReport
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&report) != nil || report.Status == "" {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "Invalid report.")

		return
	}
	if err := AddGameStatusReport(server.db, r.PathValue("console"), r.PathValue("game"), report); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	respondItem(w, http.StatusOK, map[string]bool{"saved": true})
}

// handleInternalGameStatusConsole — PUT /internal/game-status/{console} `{status, by, at}` : le statut de la console.
func (server *Server) handleInternalGameStatusConsole(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	var body struct {
		Status string    `json:"status"`
		By     string    `json:"by"`
		At     time.Time `json:"at"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&body) != nil || body.Status == "" {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "Invalid status.")

		return
	}
	if err := SetGameStatusConsole(server.db, r.PathValue("console"), body.Status, body.By, body.At); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	respondItem(w, http.StatusOK, map[string]bool{"saved": true})
}
