package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sharedauth "gameflex/auth"
	"gameflex/kernel/internal/config"
)

// LE SECRET D'UNE PARTIE N'OUVRE QUE SA PARTIE (chantier 34, P10). Un conteneur
// de partie ne reçoit plus le secret de l'installation : le sien ouvre ses
// routes internes, pas celles d'une autre ; celui de l'installation, que
// tiennent les services du compose, les ouvre toutes.
func TestASessionSecretOpensOnlyItsSession(t *testing.T) {
	server := &Server{config: config.Config{InternalSecret: "interne"}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/sessions/{id}/mirror", server.sessionSecret(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	call := func(session, secret string) int {
		request := httptest.NewRequest(http.MethodPost, "/internal/sessions/"+session+"/mirror", nil)
		request.Header.Set("X-Internal-Secret", secret)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)

		return recorder.Code
	}

	own := sharedauth.SessionSecret("interne", "retro-1")
	if code := call("retro-1", own); code != http.StatusNoContent {
		t.Errorf("sa propre partie = %d, attendu 204", code)
	}
	if code := call("retro-2", own); code != http.StatusUnauthorized {
		t.Errorf("la partie d'à côté = %d, attendu 401", code)
	}
	if code := call("retro-2", "interne"); code != http.StatusNoContent {
		t.Errorf("le secret de l'installation = %d, attendu 204", code)
	}
	for _, secret := range []string{"", "faux", sharedauth.SessionSecret("une-autre-installation", "retro-1")} {
		if code := call("retro-1", secret); code != http.StatusUnauthorized {
			t.Errorf("secret %q = %d, attendu 401", secret, code)
		}
	}

	// Sans secret d'installation, tout est fermé — même un secret de partie
	// « calculé » sur un secret vide.
	closed := &Server{config: config.Config{}}
	mux = http.NewServeMux()
	mux.HandleFunc("POST /internal/sessions/{id}/mirror", closed.sessionSecret(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if code := call("retro-1", ""); code != http.StatusUnauthorized {
		t.Errorf("kernel sans secret = %d, attendu 401", code)
	}
}
