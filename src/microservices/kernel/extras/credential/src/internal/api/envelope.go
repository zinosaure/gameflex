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
	// Code — le code du refus, que la page traduit (chantier 59, P5) ; Detail, en anglais, ne sert qu'aux journaux.
	Code string `json:"code,omitempty"`
	// Codes — un refus que la page traduit, une raison par code (le mot de
	// passe d'une personne, chantier 55) ; Detail, en anglais, reste le repli.
	Codes []string `json:"codes,omitempty"`
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

// respondErrorCode mirrors HTTPException handling: name = HTTP status name,
// description = its standard text, code = what the page translates
// (`kernelErrors.<code>`, chantier 59, P5), detail = the English message, for
// the logs only. Every refusal carries a code: respondError is gone.
func respondErrorCode(w http.ResponseWriter, statusCode int, code, detail string) {
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
			Code:        code,
		},
	})
}

// respondPasswordRefused — 422, le mot de passe choisi ne suit pas la règle
// (auth.PasswordRefusals) : chaque ligne manquée par son code.
func respondPasswordRefused(w http.ResponseWriter, codes []string) {
	writeJSON(w, http.StatusUnprocessableEntity, envelope{
		At: nowStamp(), Code: http.StatusUnprocessableEntity, State: "failed",
		Reference: "exception",
		Exception: &exception{
			Name:        "WEAK_PASSWORD",
			Description: http.StatusText(http.StatusUnprocessableEntity),
			Detail:      "The password must be 8 to 20 characters long, with an uppercase letter, a lowercase letter, a digit and a special character.",
			Codes:       codes,
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
