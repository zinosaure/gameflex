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
	// Un code que la page traduit (chantier 59, P5) : 401, la session n'est plus valable ; 403, ce compte n'en a pas le droit.
	code := "forbidden"
	if status == http.StatusUnauthorized {
		code = "auth_required"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errorCode": code, "errorMessage": message})
}
