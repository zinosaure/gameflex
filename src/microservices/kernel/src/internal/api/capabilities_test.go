package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gameflex/kernel/internal/config"
)

// LA CONFIGURATION DU SERVEUR SE RÈGLE DE LA PAGE (chantier 81) : le serveur le déclare ; le miroir, supprimé, n'y est
// plus.
func TestTheServerDeclaresItsConfiguration(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&Server{}).handleCapabilities(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))
	var declared struct {
		Capabilities map[string]any `json:"capabilities"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &declared); err != nil {
		t.Fatal(err)
	}
	if declared.Capabilities["serverConfig"] != true {
		t.Errorf("serverConfig = %v", declared.Capabilities["serverConfig"])
	}
	if _, present := declared.Capabilities["mirrorUrl"]; present {
		t.Error("mirrorUrl ne doit plus être déclaré")
	}
}

// namedServer — ce serveur sous un nom (Réglages › Configuration du serveur), le temps d'un test.
func namedServer(t *testing.T, name string) {
	live := config.DefaultLive()
	live.ServerName = name
	config.ApplyLive(live)
	t.Cleanup(func() { config.ApplyLive(config.DefaultLive()) })
}
