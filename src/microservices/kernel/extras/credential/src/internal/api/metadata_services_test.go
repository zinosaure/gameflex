package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/database"
)

// LES COMPTES DES SERVICES DE MÉTADONNÉES ENTRENT, SERVENT AU KERNEL, ET NE
// RESSORTENT JAMAIS PAR L'ADMINISTRATION (chantier 49).
func TestAMetadataServiceAccountNeverComesBackThroughTheAdministration(t *testing.T) {
	config.DBPath = filepath.Join(t.TempDir(), "credential.db")
	db, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := &Server{db: db, config: config.Config{InternalSecret: "le-secret-interne", JWTSecret: "le-secret-des-jetons"}}
	result, err := db.Exec(`INSERT INTO users (username, hashed_password, display_name, role_level) VALUES ('root', 'x', 'Root', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	admin := auth.Principal{ID: id}

	put := func(service, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/v1/metadata-services/"+service, strings.NewReader(body))
		request.SetPathValue("service", service)
		server.handlePutMetadataService(recorder, request, admin)

		return recorder
	}

	if answer := put("igdb", `{"fields":{"client_id":"mon-client"}}`); answer.Code != http.StatusBadRequest {
		t.Fatalf("missing secret: %d %s", answer.Code, answer.Body.String())
	}
	answer := put("igdb", `{"fields":{"client_id":"mon-client","client_secret":"tres-secret"}}`)
	if answer.Code != http.StatusOK || strings.Contains(answer.Body.String(), "tres-secret") || strings.Contains(answer.Body.String(), "mon-client") {
		t.Fatalf("put: %d %s", answer.Code, answer.Body.String())
	}
	if !strings.Contains(answer.Body.String(), `"service":"igdb","configured":true,"fields":["client_id","client_secret"]`) {
		t.Fatalf("list after put: %s", answer.Body.String())
	}
	// Un champ laissé vide garde sa valeur.
	if answer := put("igdb", `{"fields":{"client_id":"autre-client","client_secret":""}}`); answer.Code != http.StatusOK {
		t.Fatalf("partial put: %d %s", answer.Code, answer.Body.String())
	}
	if answer := put("igdb", `{"fields":{"api_key":"x"}}`); answer.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", answer.Code)
	}
	if answer := put("inconnu", `{"fields":{}}`); answer.Code != http.StatusNotFound {
		t.Fatalf("unknown service: %d", answer.Code)
	}

	var stored string
	if err := db.QueryRow(`SELECT fields FROM metadata_services WHERE service = 'igdb'`).Scan(&stored); err != nil || strings.Contains(stored, "tres-secret") {
		t.Fatalf("stored in clear: %q %v", stored, err)
	}

	read := func(secret string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/internal/metadata/services", nil)
		if secret != "" {
			request.Header.Set("X-Internal-Secret", secret)
		}
		server.handleInternalMetadataServices(recorder, request)

		return recorder
	}
	for _, secret := range []string{"", "pas-le-bon"} {
		if answer := read(secret); answer.Code != http.StatusUnauthorized || strings.Contains(answer.Body.String(), "tres-secret") {
			t.Fatalf("secret %q: %d %s", secret, answer.Code, answer.Body.String())
		}
	}
	if answer := read("le-secret-interne"); answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"client_secret":"tres-secret"`) || !strings.Contains(answer.Body.String(), `"client_id":"autre-client"`) {
		t.Fatalf("kernel: %d %s", answer.Code, answer.Body.String())
	}
}

// ÉCRIRE LES MÉTADONNÉES SUR LE DISQUE DU JEU (le porteur, 2026-11-14) : allumé
// par défaut, lu par tout compte et par le kernel, changé par un administrateur
// seulement — et un corps sans le champ ne le remet pas en silence.
func TestTheMetadataSettingDefaultsToTheGameDisk(t *testing.T) {
	config.DBPath = filepath.Join(t.TempDir(), "credential.db")
	db, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := &Server{db: db, config: config.Config{InternalSecret: "le-secret-interne", JWTSecret: "le-secret-des-jetons"}}
	read := func() string {
		recorder := httptest.NewRecorder()
		server.handleGetMetadataSettings(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/metadata-services/settings", nil), auth.Principal{ID: 2, RoleLevel: auth.RoleUser})
		return recorder.Body.String()
	}
	internal := func() string {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/internal/metadata/settings", nil)
		request.Header.Set("X-Internal-Secret", "le-secret-interne")
		server.handleInternalMetadataSettings(recorder, request)
		return recorder.Body.String()
	}
	put := func(body string) int {
		recorder := httptest.NewRecorder()
		server.handlePutMetadataSettings(recorder, httptest.NewRequest(http.MethodPut, "/api/v1/metadata-services/settings", strings.NewReader(body)), auth.Principal{ID: 1, RoleLevel: auth.RoleAdmin})
		return recorder.Code
	}

	if got := read(); !strings.Contains(got, `"write_on_game_disk":true`) {
		t.Fatalf("par défaut : %s", got)
	}
	if code := put(`{"write_on_game_disk":false}`); code != http.StatusOK {
		t.Fatalf("couper : %d", code)
	}
	if got := internal(); !strings.Contains(got, `"write_on_game_disk":false`) {
		t.Fatalf("le kernel lit : %s", got)
	}
	if code := put(`{}`); code != http.StatusBadRequest {
		t.Fatalf("un corps vide : %d", code)
	}
	if got := read(); !strings.Contains(got, `"write_on_game_disk":false`) {
		t.Fatalf("après un corps vide : %s", got)
	}
	if code := put(`{"write_on_game_disk":true}`); code != http.StatusOK || !strings.Contains(read(), `"write_on_game_disk":true`) {
		t.Fatalf("rallumer : %d %s", code, read())
	}
	recorder := httptest.NewRecorder()
	server.handleInternalMetadataSettings(recorder, httptest.NewRequest(http.MethodGet, "/internal/metadata/settings", nil))
	if recorder.Code == http.StatusOK {
		t.Fatal("la route interne répond sans le secret")
	}
}
