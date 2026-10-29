package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gameflex/kernel/internal/config"
)

// LE MIROIR, DÉCLARÉ À LA PAGE (chantier 29) : la variable passe telle quelle,
// et sans elle la clé reste vide — aucun écran ne propose rien.
func TestTheServerDeclaresItsMirror(t *testing.T) {
	for _, mirror := range []string{"https://mirror.example.org/gameflex", ""} {
		server := &Server{config: config.Config{MirrorURL: mirror}}
		recorder := httptest.NewRecorder()
		server.handleCapabilities(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))
		var declared struct {
			Capabilities map[string]any `json:"capabilities"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &declared); err != nil {
			t.Fatal(err)
		}
		if got, present := declared.Capabilities["mirrorUrl"]; !present || got != mirror {
			t.Errorf("mirrorUrl = %v (présent : %v), attendu %q", got, present, mirror)
		}
	}
}
