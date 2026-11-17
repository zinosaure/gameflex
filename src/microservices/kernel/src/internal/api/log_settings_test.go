package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"gameflex/logfile"
)

func logSettingsRouter(server *Server) http.Handler {
	router := chi.NewRouter()
	router.Get("/storage/logs/settings", server.handleGetLogSettings)
	router.Put("/storage/logs/settings", server.handlePutLogSettings)

	return router
}

func callLogSettings(t *testing.T, router http.Handler, method, body string) (int, logfile.Settings) {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, "/storage/logs/settings", strings.NewReader(body)))
	var envelope struct {
		Data logfile.Settings `json:"data"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)

	return recorder.Code, envelope.Data
}

// LE RÉGLAGE DES JOURNAUX (le porteur, 2026-11-14) : ce que le corps dit change,
// le reste garde sa valeur ; il est gardé dans logs/settings.json, relu tel quel
// au démarrage, et appliqué aussitôt — sans redémarrer le kernel.
func TestTheLogSettingsAreKeptAndAppliedAtOnce(t *testing.T) {
	logfile.Apply(logfile.DefaultSettings())
	t.Cleanup(func() { logfile.Apply(logfile.DefaultSettings()) })
	root := t.TempDir()
	router := logSettingsRouter(&Server{storageRoot: root})

	code, settings := callLogSettings(t, router, http.MethodGet, "")
	if code != http.StatusOK || !settings.Kernel || settings.HTTP || !settings.Sessions || !slices.Equal(settings.Levels, []string{"info", "warn", "error"}) {
		t.Fatalf("le défaut : %d %+v", code, settings)
	}

	code, settings = callLogSettings(t, router, http.MethodPut, `{"http":true}`)
	if code != http.StatusOK || !settings.HTTP || !settings.Kernel || !slices.Equal(settings.Levels, []string{"info", "warn", "error"}) {
		t.Fatalf("les requêtes allumées : %d %+v", code, settings)
	}
	if !logfile.Current().HTTP {
		t.Fatal("le réglage n'est pas appliqué au vol")
	}

	code, settings = callLogSettings(t, router, http.MethodPut, `{"levels":["error","debug","error"],"sessions":false}`)
	if code != http.StatusOK || !slices.Equal(settings.Levels, []string{"debug", "error"}) || settings.Sessions || !settings.HTTP {
		t.Fatalf("les niveaux et les parties : %d %+v", code, settings)
	}

	if _, err := os.Stat(filepath.Join(root, "logs", "settings.json")); err != nil {
		t.Fatalf("pas gardé dans logs/settings.json : %v", err)
	}
	if reloaded := LoadLogSettings(root); !slices.Equal(reloaded.Levels, settings.Levels) || reloaded.HTTP != settings.HTTP || reloaded.Sessions != settings.Sessions || reloaded.Kernel != settings.Kernel {
		t.Fatalf("relu au démarrage : %+v, attendu %+v", reloaded, settings)
	}
	if entries, _ := logfile.List(root); len(entries) != 0 {
		t.Fatalf("le réglage apparaît dans la liste des journaux : %+v", entries)
	}
}

// UN NIVEAU INCONNU EST REFUSÉ, et rien ne change — ni l'enregistré, ni
// l'appliqué. Un corps illisible aussi.
func TestAnUnknownLevelIsRefusedAndChangesNothing(t *testing.T) {
	logfile.Apply(logfile.DefaultSettings())
	t.Cleanup(func() { logfile.Apply(logfile.DefaultSettings()) })
	root := t.TempDir()
	router := logSettingsRouter(&Server{storageRoot: root})

	if code, _ := callLogSettings(t, router, http.MethodPut, `{"levels":["verbose"]}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("un niveau inconnu : %d", code)
	}
	if code, _ := callLogSettings(t, router, http.MethodPut, `{"kernel":`); code != http.StatusBadRequest {
		t.Fatalf("un corps illisible : %d", code)
	}
	if !slices.Equal(logfile.Current().Levels, []string{"info", "warn", "error"}) {
		t.Fatalf("appliqué quand même : %+v", logfile.Current())
	}
	if _, err := os.Stat(filepath.Join(root, "logs", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("enregistré quand même : %v", err)
	}
}

// Sans fichier, ou illisible : le défaut — le kernel démarre toujours.
func TestLoadLogSettingsFallsBackToTheDefault(t *testing.T) {
	root := t.TempDir()
	if settings := LoadLogSettings(root); !settings.Kernel || settings.HTTP || !settings.Sessions {
		t.Fatalf("sans fichier : %+v", settings)
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "settings.json"), []byte(`{"levels":["verbose"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if settings := LoadLogSettings(root); !slices.Equal(settings.Levels, []string{"info", "warn", "error"}) {
		t.Fatalf("illisible : %+v", settings)
	}
}

// LE SERVEUR DÉCLARE LE RÉGLAGE À LA PAGE, qui le montre ; le téléphone ne le
// déclare pas (son journal a son interrupteur sur la console d'accueil).
func TestTheServerDeclaresItsLogSettings(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&Server{}).handleCapabilities(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))
	var declared struct {
		Capabilities map[string]any `json:"capabilities"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &declared); err != nil {
		t.Fatal(err)
	}
	if declared.Capabilities["logSettings"] != true {
		t.Fatalf("logSettings = %v", declared.Capabilities["logSettings"])
	}
}

// L'INTERRUPTEUR DES SIGNALEMENTS (le porteur, 2026-11-14 : « Oui pour le
// reports.log ») : allumé par défaut ; éteint par la route, la capacité
// `reports` le dit aussitôt — l'écran d'erreur cache « Signaler l'erreur » — et
// un signalement envoyé quand même est refusé (409), sans fichier.
func TestTheReportsSwitchHidesTheButtonAndRefusesAReport(t *testing.T) {
	logfile.Apply(logfile.DefaultSettings())
	t.Cleanup(func() { logfile.Apply(logfile.DefaultSettings()) })
	root := t.TempDir()
	server := &Server{storageRoot: root}
	declared := func() any {
		recorder := httptest.NewRecorder()
		server.handleCapabilities(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))
		var answer struct {
			Capabilities map[string]any `json:"capabilities"`
		}
		_ = json.Unmarshal(recorder.Body.Bytes(), &answer)

		return answer.Capabilities["reports"]
	}

	if code, settings := callLogSettings(t, logSettingsRouter(server), http.MethodGet, ""); code != http.StatusOK || !settings.Reports || declared() != true {
		t.Fatalf("le défaut : %d %+v, reports déclaré %v", code, settings, declared())
	}
	code, settings := callLogSettings(t, logSettingsRouter(server), http.MethodPut, `{"reports":false}`)
	if code != http.StatusOK || settings.Reports || !settings.Kernel || !settings.Sessions || declared() != false {
		t.Fatalf("éteints : %d %+v, reports déclaré %v", code, settings, declared())
	}
	if kept := LoadLogSettings(root); kept.Reports {
		t.Fatalf("non gardé : %+v", kept)
	}

	recorder := httptest.NewRecorder()
	server.handleStorageLogReport(recorder, httptest.NewRequest(http.MethodPost, "/storage/logs/report", strings.NewReader(`{"context":"essai"}`)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("un signalement, éteints : %d %s", recorder.Code, recorder.Body.String())
	}
	if _, err := os.Stat(filepath.Join(logfile.LogsRoot(root), logfile.ReportsFile)); !os.IsNotExist(err) {
		t.Fatalf("un fichier, signalements éteints : %v", err)
	}
}
