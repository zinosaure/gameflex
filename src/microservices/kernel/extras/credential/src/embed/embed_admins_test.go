package embed

// COMME SUR UN SERVEUR (précisé par le porteur le 2026-10-09) : `root` à la
// première utilisation, avec son mot de passe aléatoire ; avec lui on crée le
// compte administrateur, et c'est lui le propriétaire du téléphone. D'autres
// administrateurs restent possibles, comme sur un serveur — un usage, pas une
// règle — et rouvrir l'annuaire ne rétrograde personne. La règle d'avant,
// « root seul administrateur », avait rétrogradé le compte du porteur.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gameflex/credential/internal/auth"
)

func call(t *testing.T, service *Service, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	service.Handler.ServeHTTP(response, request)

	return response
}

func login(t *testing.T, service *Service, username, password string) string {
	t.Helper()
	answer := call(t, service, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": username, "password": password,
	})
	var session struct {
		Item struct {
			AccessToken string `json:"access_token"`
		} `json:"item"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &session); err != nil || session.Item.AccessToken == "" {
		t.Fatalf("connexion de %s : %d %s", username, answer.Code, answer.Body.String())
	}

	return session.Item.AccessToken
}

func TestTheOwnerIsTheAdministratorRootCreates(t *testing.T) {
	options := Options{
		DBPath:       filepath.Join(t.TempDir(), "identity.db"),
		JWTSecret:    "secret-de-test",
		RootPassword: "mot-de-passe-du-porteur",
	}
	service, err := New(options)
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}

	// Première utilisation : il n'y a que root, c'est lui le propriétaire.
	if name, role, _ := service.Owner(); name != "root" || role != auth.RoleRoot {
		t.Fatalf("propriétaire = %s (%d), attendu root avant tout administrateur", name, role)
	}

	// Root crée le compte administrateur : le propriétaire, désormais.
	root := login(t, service, "root", "mot-de-passe-du-porteur")
	if created := call(t, service, http.MethodPost, "/api/v1/users", root, map[string]any{
		"username": "gino", "password": "un-mot-de-passe-long", "role_level": auth.RoleAdmin,
	}); created.Code != http.StatusCreated {
		t.Fatalf("créer l'administrateur = %d %s", created.Code, created.Body.String())
	}
	if name, role, _ := service.Owner(); name != "gino" || role != auth.RoleAdmin {
		t.Fatalf("propriétaire = %s (%d), attendu gino, l'administrateur", name, role)
	}

	// Un second administrateur reste possible, comme sur un serveur ; le
	// propriétaire reste le premier.
	if second := call(t, service, http.MethodPost, "/api/v1/users", root, map[string]any{
		"username": "lea", "password": "un-mot-de-passe-long", "role_level": auth.RoleAdmin,
	}); second.Code != http.StatusCreated {
		t.Fatalf("créer un second administrateur = %d %s", second.Code, second.Body.String())
	}
	if name, _, _ := service.Owner(); name != "gino" {
		t.Fatalf("propriétaire = %s après un second administrateur, attendu gino", name)
	}

	// Rouvrir l'annuaire ne rétrograde personne.
	service.DB.Close()
	reopened, err := New(options)
	if err != nil {
		t.Fatalf("réouverture : %v", err)
	}
	t.Cleanup(func() { reopened.DB.Close() })
	for _, username := range []string{"gino", "lea"} {
		var level int
		if err := reopened.DB.QueryRow(`SELECT role_level FROM users WHERE username = ?`, username).Scan(&level); err != nil {
			t.Fatalf("relire %s : %v", username, err)
		}
		if level != auth.RoleAdmin {
			t.Fatalf("%s est passé au rang %d à la réouverture, attendu %d", username, level, auth.RoleAdmin)
		}
	}
}
