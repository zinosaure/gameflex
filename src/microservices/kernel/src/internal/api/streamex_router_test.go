package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Le relais streamex ne transmet que ce que la page utilise. Tout le reste —
// la page de débogage du conteneur, ses compteurs de manette, son état — est
// refusé AVANT la recherche de session : un Server sans gestionnaire
// paniquerait s'il cherchait, et ne pas chercher, c'est ne pas dire si la
// session existe (2026-10-02). Les routes de contrôle et le canal d'entrée de
// la passerelle n'existent plus dans le conteneur (P5, 2026-10-09).
func TestTheStreamexRelayRefusesEverythingThePageDoesNotUse(t *testing.T) {
	server := &Server{}

	for _, path := range []string{
		"/abcd1234",
		"/abcd1234/",
		"/abcd1234/diag/pad-stats",
		"/abcd1234/rows",
		// L'état se lit par /status/ws (status_ws.go), plus par une requête.
		"/abcd1234/status",
		"/abcd1234/ws/../games",
	} {
		recorder := httptest.NewRecorder()
		server.serveStreamexChild(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s : %d, attendu 404", path, recorder.Code)
		}
	}
}
