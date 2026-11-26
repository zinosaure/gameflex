package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gameflex/kernel/internal/config"
)

// LA CONFIGURATION DU SERVEUR (chantier 81) : enregistrée, elle vaut aussitôt
// et se relit au démarrage ; une valeur fausse est refusée et ne change rien.
func TestTheServerConfigAppliesAndSurvivesARestart(t *testing.T) {
	t.Cleanup(func() { config.ApplyLive(config.DefaultLive()) })
	root := t.TempDir()
	server := &Server{storageRoot: root}

	put := func(body string) int {
		recorder := httptest.NewRecorder()
		server.handlePutServerConfig(recorder, httptest.NewRequest(http.MethodPut, "/storage/server-config", strings.NewReader(body)))

		return recorder.Code
	}
	if code := put(`{"serverName":" Salon ","players":{"wiiu":2},"consoleLimits":{"switch":2,"ps3":3,"ps5":0},"gpu":""}`); code != http.StatusOK {
		t.Fatalf("PUT = %d", code)
	}
	if live := config.CurrentLive(); live.ServerName != "Salon" || live.Players["wiiu"] != 2 || live.Limits()["ps3"] != 3 {
		t.Fatalf("en vigueur : %+v", live)
	}
	if code := put(`{"players":{"wiiu":0}}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("PUT faux = %d", code)
	}
	if config.CurrentLive().Players["wiiu"] != 2 {
		t.Fatal("un refus ne doit rien changer")
	}

	config.ApplyLive(config.DefaultLive())
	if live := LoadServerConfig(root); live.ServerName != "Salon" || len(live.ConsoleLimits) != 2 || live.Limits()["switch"] != 2 {
		t.Fatalf("relu au démarrage : %+v", live)
	}
	if live := LoadServerConfig(t.TempDir()); len(live.Players) != 0 || live.Limits()["ps5"] != 1 {
		t.Fatalf("sans fichier, les défauts : %+v", live)
	}
}
