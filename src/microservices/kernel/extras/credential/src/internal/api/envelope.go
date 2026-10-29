package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Envelope replicates libs/skeleton.py's StandardResponse wrapper exactly —
// the whole frontend reads result.item / result.items / result.exception
// through it (see AuthContext.jsx's authApi). Reference "items" with items
// AND pagination hoisted to the top level; errors carry {name, description,
// detail} with the exact detail strings the UI displays.
type envelope struct {
	At         string     `json:"at"`
	Code       int        `json:"code"`
	State      string     `json:"state"`
	Reference  string     `json:"reference"`
	Item       any        `json:"item,omitempty"`
	Items      any        `json:"items,omitempty"`
	Pagination any        `json:"pagination,omitempty"`
	Exception  *exception `json:"exception,omitempty"`
}

type exception struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Detail      any    `json:"detail"`
}

type pagination struct {
	Count        int   `json:"count"`
	Page         int   `json:"page"`
	ItemsPerPage int   `json:"items_per_page"`
	TotalPages   int   `json:"total_pages"`
	Sorts        []any `json:"sorts"`
}

func newPagination(count, page, itemsPerPage int) pagination {
	totalPages := 0
	if itemsPerPage > 0 {
		totalPages = (count + itemsPerPage - 1) / itemsPerPage
	}
	return pagination{Count: count, Page: page, ItemsPerPage: itemsPerPage, TotalPages: totalPages, Sorts: []any{}}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("write response failed", "error", err)
	}
}

func respondItem(w http.ResponseWriter, statusCode int, item any) {
	writeJSON(w, statusCode, envelope{
		At: nowStamp(), Code: statusCode, State: "success",
		Reference: "item", Item: item,
	})
}

func respondItems(w http.ResponseWriter, statusCode int, items any, page pagination) {
	writeJSON(w, statusCode, envelope{
		At: nowStamp(), Code: statusCode, State: "success",
		Reference: "items", Items: items, Pagination: page,
	})
}

// respondError mirrors HTTPException handling: name = HTTP status name,
// description = its standard text, detail = the message the UI reads.
func respondError(w http.ResponseWriter, statusCode int, detail string) {
	name := strings.ToUpper(strings.ReplaceAll(http.StatusText(statusCode), " ", "_"))
	if name == "" {
		name = "INTERNAL_SERVER_ERROR"
	}
	writeJSON(w, statusCode, envelope{
		At: nowStamp(), Code: statusCode, State: "failed",
		Reference: "exception",
		Exception: &exception{
			Name:        name,
			Description: http.StatusText(statusCode),
			Detail:      detail,
		},
	})
}

// respondValidationError mirrors the Pydantic validation failures the
// frontend surfaces on form errors.
func respondValidationError(w http.ResponseWriter, fields []map[string]any) {
	writeJSON(w, http.StatusBadRequest, envelope{
		At: nowStamp(), Code: http.StatusBadRequest, State: "failed",
		Reference: "exception",
		Exception: &exception{
			Name:        "SCHEMA_VALIDATION_ERROR",
			Description: "Bad Request",
			Detail:      fields,
		},
	})
}

func nowStamp() string {
	return time.Now().UTC().Truncate(time.Second).Format("2006-01-02T15:04:05")
}
