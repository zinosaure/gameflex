package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"gameflex/credential/internal/auth"
)

const (
	maxItemsDefault = 20
	allItems        = 1000
	positioningMax  = 10
)

// ── History ──

func (server *Server) handleListHistory(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	itemsPerPage := queryInt(r, "items_per_page", maxItemsDefault)
	if itemsPerPage < 1 || itemsPerPage > 100 {
		itemsPerPage = maxItemsDefault
	}

	var count int
	if err := server.db.QueryRow(
		`SELECT COUNT(*) FROM play_history WHERE user_id = ?`, principal.ID,
	).Scan(&count); err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}

	rows, err := server.db.Query(
		`SELECT id, user_id, console_id, reference, title, go_session_id, started_at, ended_at
		 FROM play_history WHERE user_id = ?
		 ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`,
		principal.ID, itemsPerPage, (page-1)*itemsPerPage,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	defer rows.Close()

	items := []historyResponse{}
	for rows.Next() {
		var entry historyResponse
		var title, goSessionID, startedAt, endedAt sql.NullString
		if err := rows.Scan(&entry.ID, &entry.UserID, &entry.ConsoleID, &entry.Reference, &title, &goSessionID, &startedAt, &endedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		entry.Title, entry.GoSessionID = text(title), text(goSessionID)
		entry.StartedAt, entry.EndedAt = isoDatetime(startedAt), isoDatetime(endedAt)
		items = append(items, entry)
	}
	respondItems(w, http.StatusOK, items, newPagination(count, page, itemsPerPage))
}

func (server *Server) handleStartHistory(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		ConsoleID string  `json:"console_id"`
		Reference string  `json:"reference"`
		Title     *string `json:"title"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	result, err := server.db.Exec(
		`INSERT INTO play_history (user_id, console_id, reference, title, started_at) VALUES (?, ?, ?, ?, ?)`,
		principal.ID, body.ConsoleID, body.Reference, body.Title, now(),
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	id, _ := result.LastInsertId()

	respondItem(w, http.StatusCreated, server.fetchHistory(id))
}

func (server *Server) handleUpdateHistory(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusNotFound, "History entry not found.")
		return
	}

	var ownerID int64
	err = server.db.QueryRow(`SELECT user_id FROM play_history WHERE id = ?`, id).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && ownerID != principal.ID) {
		respondError(w, http.StatusNotFound, "History entry not found.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}

	var body struct {
		GoSessionID *string `json:"go_session_id"`
		EndedAt     *string `json:"ended_at"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	if body.GoSessionID != nil {
		if _, err := server.db.Exec(`UPDATE play_history SET go_session_id = ? WHERE id = ?`, *body.GoSessionID, id); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
	}
	if body.EndedAt != nil {
		if _, err := server.db.Exec(`UPDATE play_history SET ended_at = ? WHERE id = ?`, *body.EndedAt, id); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
	}
	respondItem(w, http.StatusOK, server.fetchHistory(id))
}

func (server *Server) fetchHistory(id int64) historyResponse {
	var entry historyResponse
	var title, goSessionID, startedAt, endedAt sql.NullString
	row := server.db.QueryRow(
		`SELECT id, user_id, console_id, reference, title, go_session_id, started_at, ended_at FROM play_history WHERE id = ?`, id,
	)
	if err := row.Scan(&entry.ID, &entry.UserID, &entry.ConsoleID, &entry.Reference, &title, &goSessionID, &startedAt, &endedAt); err != nil {
		return entry
	}
	entry.Title, entry.GoSessionID = text(title), text(goSessionID)
	entry.StartedAt, entry.EndedAt = isoDatetime(startedAt), isoDatetime(endedAt)
	return entry
}

// ── Controller mapping (keyboard / gamepad columns on the same row) ──

func (server *Server) handleGetControllerMapping(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	consoleID := r.PathValue("console_id")
	kind := r.PathValue("kind")
	if kind != "keyboard" && kind != "gamepad" {
		respondError(w, http.StatusBadRequest, "kind must be 'keyboard' or 'gamepad'.")
		return
	}

	row := server.mappingRow(principal.ID, consoleID)
	if row == nil {
		respondItem(w, http.StatusOK, map[string]any{"consoleId": consoleID, "mapping": nil})
		return
	}
	if kind == "keyboard" {
		respondItem(w, http.StatusOK, map[string]any{"consoleId": consoleID, "mapping": row.Keyboard})
	} else {
		respondItem(w, http.StatusOK, map[string]any{"consoleId": consoleID, "mapping": row.Gamepad})
	}
}

func (server *Server) handleSaveControllerMapping(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	consoleID := r.PathValue("console_id")
	kind := r.PathValue("kind")
	if kind != "keyboard" && kind != "gamepad" {
		respondError(w, http.StatusBadRequest, "kind must be 'keyboard' or 'gamepad'.")
		return
	}

	var body struct {
		Mapping json.RawMessage `json:"mapping"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if string(body.Mapping) == "" {
		body.Mapping = json.RawMessage("null")
	}

	if existing := server.mappingRow(principal.ID, consoleID); existing != nil {
		if _, err := server.db.Exec(
			`UPDATE controller_mappings SET `+kind+` = ?, updated_at = ? WHERE id = ?`,
			string(body.Mapping), now(), existing.ID,
		); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		respondItem(w, http.StatusOK, *server.mappingRow(principal.ID, consoleID))
		return
	}

	result, err := server.db.Exec(
		`INSERT INTO controller_mappings (user_id, console_id, `+kind+`, updated_at) VALUES (?, ?, ?, ?)`,
		principal.ID, consoleID, string(body.Mapping), now(),
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	id, _ := result.LastInsertId()

	respondItem(w, http.StatusOK, *server.mappingRowByID(id))
}

func (server *Server) mappingRow(userID int64, consoleID string) *controllerMappingResponse {
	var row controllerMappingResponse
	var keyboard, gamepad, updatedAt sql.NullString
	err := server.db.QueryRow(
		`SELECT id, user_id, console_id, keyboard, gamepad, updated_at
		 FROM controller_mappings WHERE user_id = ? AND console_id = ?`, userID, consoleID,
	).Scan(&row.ID, &row.UserID, &row.ConsoleID, &keyboard, &gamepad, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return nil
	}
	row.Keyboard = nullableJSON(keyboard)
	row.Gamepad = nullableJSON(gamepad)
	row.UpdatedAt = isoDatetime(updatedAt)
	return &row
}

func (server *Server) mappingRowByID(id int64) *controllerMappingResponse {
	var row controllerMappingResponse
	var consoleID string
	var keyboard, gamepad, updatedAt sql.NullString
	err := server.db.QueryRow(
		`SELECT id, user_id, console_id, keyboard, gamepad, updated_at FROM controller_mappings WHERE id = ?`, id,
	).Scan(&row.ID, &row.UserID, &consoleID, &keyboard, &gamepad, &updatedAt)
	if err != nil {
		return nil
	}
	row.ConsoleID = consoleID
	row.Keyboard = nullableJSON(keyboard)
	row.Gamepad = nullableJSON(gamepad)
	row.UpdatedAt = isoDatetime(updatedAt)
	return &row
}

func nullableJSON(raw sql.NullString) any {
	if !raw.Valid {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw.String), &decoded); err != nil {
		return nil
	}
	return decoded
}

// ── Key mappings (physical device -> virtual controller) ──

func (server *Server) handleListKeyMappings(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	where := "user_id = ?"
	args := []any{principal.ID}
	if deviceID := r.URL.Query().Get("physical_device_id"); deviceID != "" {
		where += " AND physical_device_id = ?"
		args = append(args, deviceID)
	}
	if controllerName := r.URL.Query().Get("controller_name"); controllerName != "" {
		where += " AND controller_name = ?"
		args = append(args, controllerName)
	}

	rows, err := server.db.Query(
		`SELECT id, user_id, physical_device_id, physical_device_type, controller_name, mapping, updated_at
		 FROM key_mappings WHERE `+where+` ORDER BY id`, args...,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	defer rows.Close()

	items := []keyMappingResponse{}
	for rows.Next() {
		var item keyMappingResponse
		var mappingJSON string
		var updatedAt sql.NullString
		if err := rows.Scan(&item.ID, &item.UserID, &item.PhysicalDeviceID, &item.PhysicalDeviceType, &item.ControllerName, &mappingJSON, &updatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		item.Mapping = json.RawMessage(mappingJSON)
		item.UpdatedAt = isoDatetime(updatedAt)
		items = append(items, item)
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

func (server *Server) handleSaveKeyMapping(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		PhysicalDeviceID   string          `json:"physical_device_id"`
		PhysicalDeviceType string          `json:"physical_device_type"`
		ControllerName     string          `json:"controller_name"`
		Mapping            json.RawMessage `json:"mapping"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	var existingID int64
	err := server.db.QueryRow(
		`SELECT id FROM key_mappings WHERE user_id = ? AND physical_device_id = ? AND controller_name = ?`,
		principal.ID, body.PhysicalDeviceID, body.ControllerName,
	).Scan(&existingID)

	var result sql.Result
	if err == nil {
		result, _ = server.db.Exec(
			`UPDATE key_mappings SET physical_device_type = ?, mapping = ?, updated_at = ? WHERE id = ?`,
			body.PhysicalDeviceType, string(body.Mapping), now(), existingID,
		)
	} else {
		result, err = server.db.Exec(
			`INSERT INTO key_mappings (user_id, physical_device_id, physical_device_type, controller_name, mapping, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			principal.ID, body.PhysicalDeviceID, body.PhysicalDeviceType, body.ControllerName, string(body.Mapping), now(),
		)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}

	var id int64
	if existingID != 0 {
		id = existingID
	} else {
		id, _ = result.LastInsertId()
	}
	respondItem(w, http.StatusCreated, server.fetchKeyMapping(id))
}

func (server *Server) handleDeleteKeyMapping(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	deviceID := r.URL.Query().Get("physical_device_id")
	controllerName := r.URL.Query().Get("controller_name")
	if deviceID == "" || controllerName == "" {
		respondError(w, http.StatusBadRequest, "Missing physical_device_id or controller_name.")
		return
	}
	_, _ = server.db.Exec(
		`DELETE FROM key_mappings WHERE user_id = ? AND physical_device_id = ? AND controller_name = ?`,
		principal.ID, deviceID, controllerName,
	)
	w.WriteHeader(http.StatusNoContent)
}

func (server *Server) fetchKeyMapping(id int64) keyMappingResponse {
	var item keyMappingResponse
	var mappingJSON string
	var updatedAt sql.NullString
	row := server.db.QueryRow(
		`SELECT id, user_id, physical_device_id, physical_device_type, controller_name, mapping, updated_at FROM key_mappings WHERE id = ?`, id,
	)
	if err := row.Scan(&item.ID, &item.UserID, &item.PhysicalDeviceID, &item.PhysicalDeviceType, &item.ControllerName, &mappingJSON, &updatedAt); err != nil {
		return item
	}
	item.Mapping = json.RawMessage(mappingJSON)
	item.UpdatedAt = isoDatetime(updatedAt)
	return item
}

// ── Key positionings (virtual controller layouts) ──

func (server *Server) handleListKeyPositionings(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	where := "user_id = ?"
	args := []any{principal.ID}
	if controllerName := r.URL.Query().Get("controller_name"); controllerName != "" {
		where += " AND controller_name = ?"
		args = append(args, controllerName)
	}

	rows, err := server.db.Query(
		`SELECT id, user_id, controller_name, slot_index, is_default, layout, opacity, updated_at
		 FROM key_positionings WHERE `+where+` ORDER BY slot_index`, args...,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	defer rows.Close()

	items := []keyPositioningResponse{}
	for rows.Next() {
		var item keyPositioningResponse
		var layoutJSON string
		var updatedAt sql.NullString
		if err := rows.Scan(&item.ID, &item.UserID, &item.ControllerName, &item.SlotIndex, &item.IsDefault, &layoutJSON, &item.Opacity, &updatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		item.Layout = json.RawMessage(layoutJSON)
		item.UpdatedAt = isoDatetime(updatedAt)
		items = append(items, item)
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, positioningMax))
}

func (server *Server) handleSaveKeyPositioning(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		ControllerName string          `json:"controller_name"`
		SlotIndex      int             `json:"slot_index"`
		Layout         json.RawMessage `json:"layout"`
		IsDefault      bool            `json:"is_default"`
		Opacity        *float64        `json:"opacity"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.SlotIndex < 1 || body.SlotIndex > 5 {
		respondError(w, http.StatusBadRequest, "slot_index must be between 1 and 5.")
		return
	}
	opacity := 1.0
	if body.Opacity != nil {
		opacity = *body.Opacity
	}

	// A new default clears the flag on the other slots of this controller.
	if body.IsDefault {
		if _, err := server.db.Exec(
			`UPDATE key_positionings SET is_default = 0 WHERE user_id = ? AND controller_name = ?`,
			principal.ID, body.ControllerName,
		); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
	}

	var existingID int64
	err := server.db.QueryRow(
		`SELECT id FROM key_positionings WHERE user_id = ? AND controller_name = ? AND slot_index = ?`,
		principal.ID, body.ControllerName, body.SlotIndex,
	).Scan(&existingID)

	var result sql.Result
	if err == nil {
		result, _ = server.db.Exec(
			`UPDATE key_positionings SET layout = ?, is_default = ?, opacity = ?, updated_at = ? WHERE id = ?`,
			string(body.Layout), body.IsDefault, opacity, now(), existingID,
		)
	} else {
		result, err = server.db.Exec(
			`INSERT INTO key_positionings (user_id, controller_name, slot_index, is_default, layout, opacity, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			principal.ID, body.ControllerName, body.SlotIndex, body.IsDefault, string(body.Layout), opacity, now(),
		)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}

	var id int64
	if existingID != 0 {
		id = existingID
	} else {
		id, _ = result.LastInsertId()
	}
	respondItem(w, http.StatusCreated, server.fetchKeyPositioning(id))
}

func (server *Server) handleDeleteKeyPositioning(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	controllerName := r.URL.Query().Get("controller_name")
	slotIndex := queryInt(r, "slot_index", 0)
	if controllerName == "" || slotIndex == 0 {
		respondError(w, http.StatusBadRequest, "Missing controller_name or slot_index.")
		return
	}
	_, _ = server.db.Exec(
		`DELETE FROM key_positionings WHERE user_id = ? AND controller_name = ? AND slot_index = ?`,
		principal.ID, controllerName, slotIndex,
	)
	w.WriteHeader(http.StatusNoContent)
}

func (server *Server) fetchKeyPositioning(id int64) keyPositioningResponse {
	var item keyPositioningResponse
	var layoutJSON string
	var updatedAt sql.NullString
	row := server.db.QueryRow(
		`SELECT id, user_id, controller_name, slot_index, is_default, layout, opacity, updated_at FROM key_positionings WHERE id = ?`, id,
	)
	if err := row.Scan(&item.ID, &item.UserID, &item.ControllerName, &item.SlotIndex, &item.IsDefault, &layoutJSON, &item.Opacity, &updatedAt); err != nil {
		return item
	}
	item.Layout = json.RawMessage(layoutJSON)
	item.UpdatedAt = isoDatetime(updatedAt)
	return item
}

// ── Notification preferences ──

var notificationTypes = []string{"join_request", "user_connected", "stream_started", "maintenance", "general"}

func (server *Server) handleListNotificationPreferences(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	rows, err := server.db.Query(
		`SELECT notification_type, enabled FROM notification_preferences WHERE user_id = ?`, principal.ID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	defer rows.Close()

	byType := map[string]bool{}
	for rows.Next() {
		var notificationType string
		var enabled bool
		if err := rows.Scan(&notificationType, &enabled); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		byType[notificationType] = enabled
	}

	// Always all five known types, defaulting to enabled — a fresh account
	// starts "everything on" without seeded rows.
	items := make([]notificationPreferenceResponse, 0, len(notificationTypes))
	for _, notificationType := range notificationTypes {
		enabled, exists := byType[notificationType]
		items = append(items, notificationPreferenceResponse{NotificationType: notificationType, Enabled: !exists || enabled})
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, len(notificationTypes)))
}

func (server *Server) handleSaveNotificationPreference(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		NotificationType string `json:"notification_type"`
		Enabled          bool   `json:"enabled"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if !contains(notificationTypes, body.NotificationType) {
		respondError(w, http.StatusBadRequest, "Unknown notification_type.")
		return
	}

	var existingID int64
	err := server.db.QueryRow(
		`SELECT id FROM notification_preferences WHERE user_id = ? AND notification_type = ?`,
		principal.ID, body.NotificationType,
	).Scan(&existingID)
	if err == nil {
		_, _ = server.db.Exec(
			`UPDATE notification_preferences SET enabled = ?, updated_at = ? WHERE id = ?`,
			body.Enabled, now(), existingID,
		)
	} else {
		_, err = server.db.Exec(
			`INSERT INTO notification_preferences (user_id, notification_type, enabled, updated_at) VALUES (?, ?, ?, ?)`,
			principal.ID, body.NotificationType, body.Enabled, now(),
		)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
	}

	respondItem(w, http.StatusOK, notificationPreferenceResponse{
		NotificationType: body.NotificationType,
		Enabled:          body.Enabled,
	})
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
