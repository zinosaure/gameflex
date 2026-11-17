package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"gameflex/credential/internal/auth"
)

// LES FAVORIS SUIVENT LE COMPTE (le porteur, 2026-11-16) : par les vraies routes de ce service, /api/v1/auth/me/favorites
// — chacun les siens ; un jeu d'ici sans serveur, un jeu fédéré avec le sien, et le refus de ce qui ne désigne rien ; le
// même favori reposé n'en fait pas deux ; un serveur retiré du registre emporte les siens, un compte supprimé aussi.
func TestFavoritesFollowTheAccount(t *testing.T) {
	server, db := tokenServer(t)
	server.mux = http.NewServeMux()
	server.routes()
	marc, err := auth.CreateToken(server.config.JWTSecret, 2, "marc", auth.RoleUser, 0)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := auth.CreateToken(server.config.JWTSecret, 1, "admin", auth.RoleAdmin, 0)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, bearer string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+bearer)
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, request)

		return recorder
	}
	list := func(bearer string) []favoriteResponse {
		t.Helper()
		answer := call(http.MethodGet, "/api/v1/auth/me/favorites", "", bearer)
		var body struct {
			Items []favoriteResponse `json:"items"`
		}
		if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &body) != nil {
			t.Fatalf("la liste des favoris = %d %s", answer.Code, answer.Body.String())
		}

		return body.Items
	}

	if got := list(marc); len(got) != 0 {
		t.Fatalf("un compte neuf a déjà des favoris : %+v", got)
	}
	if answer := call(http.MethodGet, "/api/v1/auth/me/favorites", "", ""); answer.Code != http.StatusUnauthorized {
		t.Fatalf("les favoris sans jeton = %d, attendu 401", answer.Code)
	}

	first := call(http.MethodPost, "/api/v1/auth/me/favorites", `{"console_id":"nes","reference":"0e111a4c","title":"Jeu d'essai"}`, marc)
	again := call(http.MethodPost, "/api/v1/auth/me/favorites", `{"console_id":"nes","reference":"0e111a4c","title":"Autre titre"}`, marc)
	if first.Code != http.StatusCreated || again.Code != http.StatusCreated || !strings.Contains(again.Body.String(), `"title":"Jeu d'essai"`) {
		t.Fatalf("poser puis reposer un favori = %d %s / %d %s", first.Code, first.Body.String(), again.Code, again.Body.String())
	}
	if got := list(marc); len(got) != 1 || got[0].ConsoleID != "nes" || got[0].Reference != "0e111a4c" || got[0].ServerID != nil {
		t.Fatalf("les favoris de marc : %+v", got)
	}
	if got := list(admin); len(got) != 0 {
		t.Fatalf("les favoris de marc chez un autre compte : %+v", got)
	}

	for _, refused := range []struct {
		body string
		want int
	}{
		{`{"console_id":"inconnue","reference":"x"}`, http.StatusBadRequest},
		{`{"console_id":"nes","reference":""}`, http.StatusBadRequest},
		{`{"console_id":"nes","reference":"` + strings.Repeat("r", 256) + `"}`, http.StatusBadRequest},
		{`{"console_id":"nes","reference":"x","server_id":7}`, http.StatusBadRequest},
		{`{"console_id":"nes","reference":"srv7:x"}`, http.StatusBadRequest},
		{`{"console_id":"nes","reference":"srv7:x","server_id":8}`, http.StatusBadRequest},
		{`{"console_id":"nes","reference":"srv7:x","server_id":7}`, http.StatusNotFound},
		{`{"console_id":"nes","reference":"x","inattendu":true}`, http.StatusBadRequest},
	} {
		if answer := call(http.MethodPost, "/api/v1/auth/me/favorites", refused.body, marc); answer.Code != refused.want {
			t.Fatalf("%s = %d %s, attendu %d", refused.body, answer.Code, answer.Body.String(), refused.want)
		}
	}

	// UN JEU FÉDÉRÉ, avec son serveur ; le serveur retiré du registre l'emporte.
	result, err := db.Exec(`INSERT INTO federation_servers (link, token, hostname) VALUES ('http://ami.local', 'jeton', 'ami')`)
	if err != nil {
		t.Fatal(err)
	}
	serverID, _ := result.LastInsertId()
	federated := `{"console_id":"snes","reference":"srv` + strconv.FormatInt(serverID, 10) + `:ab12","server_id":` + strconv.FormatInt(serverID, 10) + `}`
	if answer := call(http.MethodPost, "/api/v1/auth/me/favorites", federated, marc); answer.Code != http.StatusCreated || !strings.Contains(answer.Body.String(), `"server_id":`+strconv.FormatInt(serverID, 10)) {
		t.Fatalf("un favori fédéré = %d %s", answer.Code, answer.Body.String())
	}
	if got := list(marc); len(got) != 2 || got[1].ServerID == nil || *got[1].ServerID != serverID {
		t.Fatalf("les favoris avec un jeu fédéré : %+v", got)
	}
	if _, err := db.Exec(`DELETE FROM federation_servers WHERE id = ?`, serverID); err != nil {
		t.Fatal(err)
	}
	if got := list(marc); len(got) != 1 || got[0].Reference != "0e111a4c" {
		t.Fatalf("le serveur retiré a laissé ses favoris : %+v", got)
	}

	// RETIRER : le favori part ; un favori absent, aussi 204.
	for range 2 {
		if answer := call(http.MethodDelete, "/api/v1/auth/me/favorites?console_id=nes&reference=0e111a4c", "", marc); answer.Code != http.StatusNoContent {
			t.Fatalf("retirer un favori = %d %s", answer.Code, answer.Body.String())
		}
	}
	if answer := call(http.MethodDelete, "/api/v1/auth/me/favorites?console_id=nes", "", marc); answer.Code != http.StatusBadRequest {
		t.Fatalf("retirer sans référence = %d, attendu 400", answer.Code)
	}
	if got := list(marc); len(got) != 0 {
		t.Fatalf("après le retrait : %+v", got)
	}

	// UN COMPTE SUPPRIMÉ EMPORTE LES SIENS.
	call(http.MethodPost, "/api/v1/auth/me/favorites", `{"console_id":"nes","reference":"autre"}`, marc)
	if answer := adminCall(server, server.handleDeleteUser, http.MethodDelete, "2", ""); answer.Code != http.StatusNoContent {
		t.Fatalf("supprimer marc = %d %s", answer.Code, answer.Body.String())
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM favorites`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("les favoris d'un compte supprimé : %d %v", left, err)
	}
}
