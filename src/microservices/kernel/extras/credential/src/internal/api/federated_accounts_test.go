package api

// MON COMPTE CHEZ UN AMI (chantier 20) : le jeton qu'un membre d'ici a collé
// pour un serveur fédéré ne se lit, ne s'écrit et ne s'oublie que par le plan
// interne — le kernel, avec le secret partagé. Il ouvre un compte chez
// quelqu'un d'autre : aucun navigateur ne doit pouvoir le relire.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func federatedCall(server *Server, handler http.HandlerFunc, method, query, body, secret string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/internal/federation/accounts"+query, strings.NewReader(body))
	if secret != "" {
		request.Header.Set("X-Internal-Secret", secret)
	}
	handler(recorder, request)

	return recorder
}

func TestAMembersTokenForAFriendStaysOnTheInternalPlane(t *testing.T) {
	server, db := tokenServer(t)
	if _, err := db.Exec(`INSERT INTO federation_servers (link, token, hostname) VALUES ('https://ami.example', 'jeton-du-lien', 'Chez l''ami')`); err != nil {
		t.Fatal(err)
	}

	// Sans le secret, rien : ni lire, ni écrire.
	if answer := federatedCall(server, server.handleInternalSetFederatedAccount, http.MethodPut, "",
		`{"username":"marc","server":1,"token":"jeton-de-marc"}`, ""); answer.Code != http.StatusUnauthorized {
		t.Fatalf("écrire sans le secret : %d, attendu 401", answer.Code)
	}
	if answer := federatedCall(server, server.handleInternalSetFederatedAccount, http.MethodPut, "",
		`{"username":"marc","server":1,"token":"jeton-de-marc"}`, "un-autre-secret"); answer.Code != http.StatusUnauthorized {
		t.Fatalf("écrire avec un mauvais secret : %d, attendu 401", answer.Code)
	}

	if answer := federatedCall(server, server.handleInternalSetFederatedAccount, http.MethodPut, "",
		`{"username":"marc","server":1,"token":"jeton-de-marc"}`, testInternalSecret); answer.Code != http.StatusNoContent {
		t.Fatalf("écrire : %d %s", answer.Code, answer.Body.String())
	}
	if answer := federatedCall(server, server.handleInternalSetFederatedAccount, http.MethodPut, "",
		`{"username":"marc","server":7,"token":"jeton-de-marc"}`, testInternalSecret); answer.Code != http.StatusNotFound {
		t.Fatalf("un serveur inconnu : %d, attendu 404", answer.Code)
	}

	if answer := federatedCall(server, server.handleInternalFederatedAccount, http.MethodGet, "?username=marc&server=1", "", ""); answer.Code != http.StatusUnauthorized ||
		strings.Contains(answer.Body.String(), "jeton-de-marc") {
		t.Fatalf("lire sans le secret : %d %s", answer.Code, answer.Body.String())
	}
	if answer := federatedCall(server, server.handleInternalFederatedAccount, http.MethodGet, "?username=marc&server=1", "", testInternalSecret); answer.Code != http.StatusOK ||
		!strings.Contains(answer.Body.String(), `"token":"jeton-de-marc"`) {
		t.Fatalf("lire : %d %s", answer.Code, answer.Body.String())
	}
	// Celui de Marc n'est pas celui de l'administrateur.
	if answer := federatedCall(server, server.handleInternalFederatedAccount, http.MethodGet, "?username=admin&server=1", "", testInternalSecret); answer.Code != http.StatusNotFound {
		t.Fatalf("un autre membre : %d, attendu 404", answer.Code)
	}

	if answer := federatedCall(server, server.handleInternalForgetFederatedAccount, http.MethodDelete, "?username=marc&server=1", "", testInternalSecret); answer.Code != http.StatusNoContent {
		t.Fatalf("oublier : %d %s", answer.Code, answer.Body.String())
	}
	if answer := federatedCall(server, server.handleInternalFederatedAccount, http.MethodGet, "?username=marc&server=1", "", testInternalSecret); answer.Code != http.StatusNotFound {
		t.Fatalf("après l'oubli : %d, attendu 404", answer.Code)
	}
}
