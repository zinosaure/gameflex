package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gameflex/credential/internal/auth"
)

// UN NOM DE COMPTE DEVIENT UN DOSSIER (saves/<nom>) : ni `/`, ni `..`, et le
// motif est celui que le kernel accepte pour lancer une partie (#601).
func TestAUsernameIsAFolderName(t *testing.T) {
	for _, name := range []string{"gino", "claude", "marc.dupont", "a_b-c", "abc", strings.Repeat("a", 32)} {
		if !usernamePattern.MatchString(name) {
			t.Errorf("%q refusé", name)
		}
	}
	for _, name := range []string{"..", "...a/", "../bob", "a/b", `a\b`, "ab", "Gino D", "élodie", strings.Repeat("a", 33), ""} {
		if usernamePattern.MatchString(name) {
			t.Errorf("%q accepté", name)
		}
	}
}

// LE FILTRE « JETON ACTIVÉ » DES MEMBRES (chantier 53, P11) : appliqué par le
// serveur, compteur compris, et combiné en ET avec les groupes de `search`, qui
// se combinent entre eux en OU.
func TestTheMembersListFiltersOnAnEnabledToken(t *testing.T) {
	server, db := tokenServer(t)
	if _, err := db.Exec(`INSERT INTO users (username, hashed_password, display_name, role_level) VALUES ('lea', 'x', 'lea', 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE account_tokens SET enabled = 1, expires_at = ? WHERE user_id IN (1, 2)`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	list := func(query string) (names []string, count int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/users?"+query, nil)
		server.handleListUsers(recorder, request, auth.Principal{ID: 1, Username: "admin", RoleLevel: auth.RoleAdmin})
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s : %d %s", query, recorder.Code, recorder.Body.String())
		}
		var answer struct {
			Items []struct {
				Username string `json:"username"`
			} `json:"items"`
			Pagination struct {
				Count int `json:"count"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
		for _, item := range answer.Items {
			names = append(names, item.Username)
		}

		return names, answer.Pagination.Count
	}

	if names, count := list("token_enabled=1"); strings.Join(names, ",") != "admin,marc" || count != 2 {
		t.Fatalf("jeton activé : %v (%d), attendu admin,marc (2)", names, count)
	}
	if names, count := list(""); len(names) != 3 || count != 3 {
		t.Fatalf("sans filtre : %v (%d), attendu les trois", names, count)
	}
	// Deux groupes en OU (le nom, le nom affiché), chacun restreint aux joueurs ; le jeton s'ajoute aux deux.
	search := url.QueryEscape(`[{"username":{"like":"a"},"role_level":{"in":[2]}},{"display_name":{"like":"a"},"role_level":{"in":[2]}}]`)
	if names, count := list("search=" + search + "&token_enabled=1"); strings.Join(names, ",") != "marc" || count != 1 {
		t.Fatalf("joueurs en « a », jeton activé : %v (%d), attendu marc (1)", names, count)
	}
	if names, _ := list("search=" + search); strings.Join(names, ",") != "marc,lea" {
		t.Fatalf("joueurs en « a » : %v, attendu marc,lea", names)
	}
}
