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

// CHAQUE COMPTE CHOISIT LES SERVEURS FÉDÉRÉS DONT IL VOIT LES JEUX (le porteur, 2026-11-16) : par les vraies routes,
// /api/v1/auth/me/hidden-servers — tous montrés au départ ; en éteindre un ne touche que ce compte ; l'éteindre deux
// fois n'en fait pas deux ; un serveur inconnu, un identifiant qui n'en est pas un, refusés ; le rallumer, même déjà
// allumé ; un serveur retiré du registre, et un compte supprimé, emportent leurs lignes.
func TestHiddenServersFollowTheAccount(t *testing.T) {
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
	call := func(method, path, bearer string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(""))
		request.Header.Set("Authorization", "Bearer "+bearer)
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, request)

		return recorder
	}
	hidden := func(bearer string) []int64 {
		t.Helper()
		answer := call(http.MethodGet, "/api/v1/auth/me/hidden-servers", bearer)
		var body struct {
			Items []hiddenServerResponse `json:"items"`
		}
		if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &body) != nil {
			t.Fatalf("la liste des serveurs éteints = %d %s", answer.Code, answer.Body.String())
		}
		ids := []int64{}
		for _, item := range body.Items {
			ids = append(ids, item.ServerID)
		}

		return ids
	}

	result, err := db.Exec(`INSERT INTO federation_servers (link, token, hostname) VALUES ('http://ami.local', 'jeton', 'ami')`)
	if err != nil {
		t.Fatal(err)
	}
	serverID, _ := result.LastInsertId()
	path := "/api/v1/auth/me/hidden-servers/" + strconv.FormatInt(serverID, 10)

	if got := hidden(marc); len(got) != 0 {
		t.Fatalf("un compte neuf a déjà des serveurs éteints : %v", got)
	}
	if answer := call(http.MethodGet, "/api/v1/auth/me/hidden-servers", ""); answer.Code != http.StatusUnauthorized {
		t.Fatalf("la liste sans jeton = %d, attendu 401", answer.Code)
	}

	for range 2 {
		if answer := call(http.MethodPut, path, marc); answer.Code != http.StatusNoContent {
			t.Fatalf("éteindre un serveur = %d %s", answer.Code, answer.Body.String())
		}
	}
	if got := hidden(marc); len(got) != 1 || got[0] != serverID {
		t.Fatalf("les serveurs éteints de marc : %v", got)
	}
	if got := hidden(admin); len(got) != 0 {
		t.Fatalf("le choix de marc chez un autre compte : %v", got)
	}

	for _, refused := range []struct {
		path string
		want int
	}{
		{"/api/v1/auth/me/hidden-servers/999", http.StatusNotFound},
		{"/api/v1/auth/me/hidden-servers/abc", http.StatusBadRequest},
		{"/api/v1/auth/me/hidden-servers/0", http.StatusBadRequest},
	} {
		if answer := call(http.MethodPut, refused.path, marc); answer.Code != refused.want {
			t.Fatalf("PUT %s = %d %s, attendu %d", refused.path, answer.Code, answer.Body.String(), refused.want)
		}
	}

	// LE RALLUMER : il repart de la liste ; déjà allumé, 204 aussi.
	for range 2 {
		if answer := call(http.MethodDelete, path, marc); answer.Code != http.StatusNoContent {
			t.Fatalf("rallumer un serveur = %d %s", answer.Code, answer.Body.String())
		}
	}
	if got := hidden(marc); len(got) != 0 {
		t.Fatalf("après l'avoir rallumé : %v", got)
	}

	// UN SERVEUR RELAYÉ (chantier 83) : -(relais × 10⁶ + id), éteint et rallumé comme un autre ; un relais inconnu, refusé.
	relayedPath := "/api/v1/auth/me/hidden-servers/" + strconv.FormatInt(-(serverID*1_000_000+7), 10)
	if answer := call(http.MethodPut, relayedPath, marc); answer.Code != http.StatusNoContent {
		t.Fatalf("éteindre un serveur relayé = %d %s", answer.Code, answer.Body.String())
	}
	if got := hidden(marc); len(got) != 1 || got[0] != -(serverID*1_000_000+7) {
		t.Fatalf("les serveurs éteints de marc, un relayé : %v", got)
	}
	if answer := call(http.MethodPut, "/api/v1/auth/me/hidden-servers/-999000007", marc); answer.Code != http.StatusNotFound {
		t.Fatalf("un relais inconnu = %d, attendu 404", answer.Code)
	}
	if answer := call(http.MethodDelete, relayedPath, marc); answer.Code != http.StatusNoContent || len(hidden(marc)) != 0 {
		t.Fatalf("rallumer un serveur relayé = %d, reste %v", answer.Code, hidden(marc))
	}
	call(http.MethodPut, relayedPath, marc)

	// UN SERVEUR RETIRÉ DU REGISTRE EMPORTE SES LIGNES (celles des serveurs qu'il relayait aussi).
	call(http.MethodPut, path, marc)
	if _, err := db.Exec(`DELETE FROM federation_servers WHERE id = ?`, serverID); err != nil {
		t.Fatal(err)
	}
	if got := hidden(marc); len(got) != 0 {
		t.Fatalf("le serveur retiré est resté éteint : %v", got)
	}

	// UN COMPTE SUPPRIMÉ EMPORTE LES SIENNES.
	result, err = db.Exec(`INSERT INTO federation_servers (link, token, hostname) VALUES ('http://autre.local', 'jeton', 'autre')`)
	if err != nil {
		t.Fatal(err)
	}
	otherID, _ := result.LastInsertId()
	if answer := call(http.MethodPut, "/api/v1/auth/me/hidden-servers/"+strconv.FormatInt(otherID, 10), marc); answer.Code != http.StatusNoContent {
		t.Fatalf("éteindre l'autre serveur = %d %s", answer.Code, answer.Body.String())
	}
	if answer := adminCall(server, server.handleDeleteUser, http.MethodDelete, "2", ""); answer.Code != http.StatusNoContent {
		t.Fatalf("supprimer marc = %d %s", answer.Code, answer.Body.String())
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM hidden_servers`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("les serveurs éteints d'un compte supprimé : %d %v", left, err)
	}
}
