package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefuse_JSONEnvelope(t *testing.T) {
	recorder := httptest.NewRecorder()
	Refuse(recorder, http.StatusUnauthorized, "invalid or missing token")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, attendu 401", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q : le client lit du JSON", got)
	}
	var body struct {
		Success      *bool  `json:"success"`
		ErrorMessage string `json:"errorMessage"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("corps illisible en JSON : %v (%q)", err, recorder.Body.String())
	}
	if body.Success == nil || *body.Success || body.ErrorMessage != "invalid or missing token" {
		t.Fatalf("enveloppe inattendue : %s", recorder.Body.String())
	}
}
