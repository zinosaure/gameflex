package auth

import (
	"encoding/json"
	"net/http"
)

// Refuse answers an authentication refusal (401, 403) in the envelope every
// GameFlex client reads: {"success": false, "errorMessage": …}, as JSON.
//
// The services used to write these with http.Error, in text/plain. The page
// reads a body as JSON before anything else, failed there, and reported
// « Invalid response format » where it should have understood « this server
// does not know us » (2026-09-11). One writer shared by the services, so the
// envelope cannot drift apart again.
func Refuse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errorMessage": message})
}
