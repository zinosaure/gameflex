package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/database"
)

// CHANTIER 66 — couper des consoles pour tout le serveur, ou pour un accès délivré ; et la session fédérée qui dit
// par quel accès elle est venue.

// snapshotOf — ce que les kernels reçoivent avec les comptes : les accès et les consoles coupées.
func snapshotOf(t *testing.T, server *Server) (accesses []accessEntry, disabled []string) {
	t.Helper()
	payload, err := server.accountsSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var message struct {
		Accesses         []accessEntry `json:"accesses"`
		DisabledConsoles []string      `json:"disabledConsoles"`
	}
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatal(err)
	}

	return message.Accesses, message.DisabledConsoles
}

func TestAConsoleIsSwitchedOffAndOnForTheWholeServer(t *testing.T) {
	server, _ := tokenServer(t)

	if answer := adminCall(server, server.handleSetConsoleSwitch, http.MethodPut, "ps5", `{"enabled":false}`); answer.Code != http.StatusOK {
		t.Fatalf("couper la PS5 : %d %s", answer.Code, answer.Body.String())
	}
	if _, disabled := snapshotOf(t, server); len(disabled) != 1 || disabled[0] != "ps5" {
		t.Fatalf("consoles coupées envoyées aux kernels : %v, attendu [ps5]", disabled)
	}
	if answer := adminCall(server, server.handleSetConsoleSwitch, http.MethodPut, "nintendo-game-boy-inventee", `{"enabled":false}`); answer.Code != http.StatusBadRequest {
		t.Fatalf("une console inconnue : %d, attendu 400", answer.Code)
	}
	if answer := adminCall(server, server.handleSetConsoleSwitch, http.MethodPut, "ps5", `{"enabled":true}`); answer.Code != http.StatusOK {
		t.Fatalf("rallumer la PS5 : %d %s", answer.Code, answer.Body.String())
	}
	if _, disabled := snapshotOf(t, server); len(disabled) != 0 {
		t.Fatalf("consoles coupées après rallumage : %v, attendu aucune", disabled)
	}
}

func TestAnAccessIsRestrictedLikeAMember(t *testing.T) {
	server, db := tokenServer(t)
	var id string
	if err := db.QueryRow(`SELECT id FROM federation_tokens WHERE token_hash = ?`, hashToken(testAccess)).Scan(&id); err != nil {
		t.Fatal(err)
	}

	accesses, _ := snapshotOf(t, server)
	if len(accesses) != 1 || len(accesses[0].Consoles) != 1 || accesses[0].Consoles[0] != "*" {
		t.Fatalf("un accès neuf : %+v, attendu toutes les consoles", accesses)
	}
	answer := adminCall(server, server.handleUpdateFederationToken, http.MethodPatch, id, `{"consoles":["retro","rp-ps5"]}`)
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"consoles":["retro","rp-ps5"]`) {
		t.Fatalf("restreindre l'accès : %d %s", answer.Code, answer.Body.String())
	}
	if accesses, _ := snapshotOf(t, server); strings.Join(accesses[0].Consoles, ",") != "retro,rp-ps5" {
		t.Fatalf("l'accès envoyé aux kernels : %+v", accesses)
	}
	// Changer ses consoles ne touche pas ses portées.
	var scopes string
	_ = db.QueryRow(`SELECT scopes FROM federation_tokens WHERE id = ?`, id).Scan(&scopes)
	if scopes != "catalog" {
		t.Fatalf("portées après les consoles : %q, attendu catalog", scopes)
	}
	if answer := adminCall(server, server.handleUpdateFederationToken, http.MethodPatch, id, `{"consoles":["ps9"]}`); answer.Code != http.StatusBadRequest {
		t.Fatalf("une console inconnue : %d, attendu 400", answer.Code)
	}
}

func TestTheFederatedSessionCarriesItsAccess(t *testing.T) {
	server, db := tokenServer(t)
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"1m"}`)
	var id int64
	if err := db.QueryRow(`SELECT id FROM federation_tokens WHERE token_hash = ?`, hashToken(testAccess)).Scan(&id); err != nil {
		t.Fatal(err)
	}

	answer := sessionCall(server, tokenOf(t, db, 2))
	var body struct {
		Item struct {
			AccessToken string `json:"access_token"`
		} `json:"item"`
	}
	if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &body) != nil {
		t.Fatalf("session : %d %s", answer.Code, answer.Body.String())
	}
	claims, err := auth.ParseToken(server.config.JWTSecret, body.Item.AccessToken)
	if err != nil || claims.Acc != id {
		t.Fatalf("l'accès de la session : %+v (%v), attendu %d", claims, err, id)
	}
}

// LA PORTÉE `devices` QUITTE LES JETONS EXISTANTS au démarrage (database.Open) : ils gardent le reste.
func TestTheDevicesScopeLeavesStoredTokens(t *testing.T) {
	_, db := tokenServer(t)
	if _, err := db.Exec(`UPDATE federation_tokens SET scopes = 'catalog,devices,metadata-providers'`); err != nil {
		t.Fatal(err)
	}
	reopened, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var scopes string
	_ = reopened.QueryRow(`SELECT scopes FROM federation_tokens`).Scan(&scopes)
	if scopes != "catalog,metadata-providers" {
		t.Fatalf("portées : %q, attendu catalog,metadata-providers", scopes)
	}
}

// ADMINISTRER DEPUIS UN SERVEUR FÉDÉRÉ (chantier 73) : coupé par défaut, la session ouverte par le jeton d'un
// administrateur n'est que joueur ; autorisé par son propriétaire, elle garde son rang ; coupé de nouveau, elle le perd
// tout de suite.
func TestRemoteAdministrationIsGrantedPerAccess(t *testing.T) {
	server, db := tokenServer(t)
	var id int64
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, strconv.FormatInt(id, 10), `{"enabled":true,"duration":"1m"}`)
	answer := sessionCall(server, tokenOf(t, db, id))
	var body struct {
		Item struct {
			AccessToken string `json:"access_token"`
		} `json:"item"`
	}
	if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &body) != nil {
		t.Fatalf("session : %d %s", answer.Code, answer.Body.String())
	}
	role := func() int {
		principal, err := auth.LoadToken(db, server.config.JWTSecret, body.Item.AccessToken)
		if err != nil {
			t.Fatal(err)
		}
		return principal.RoleLevel
	}
	if got := role(); got != auth.RoleUser {
		t.Fatalf("coupé par défaut : rang %d, attendu joueur", got)
	}
	// /auth/me le dit aussi : c'est là que la page du serveur ami lit s'il peut administrer.
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+body.Item.AccessToken)
	recorder := httptest.NewRecorder()
	server.withUser(server.handleMe)(recorder, request)
	if !strings.Contains(recorder.Body.String(), `"role_level":2`) {
		t.Fatalf("/auth/me d'une session non autorisée : %s, attendu le rang de joueur", recorder.Body.String())
	}
	var access int64
	if err := db.QueryRow(`SELECT id FROM federation_tokens WHERE token_hash = ?`, hashToken(testAccess)).Scan(&access); err != nil {
		t.Fatal(err)
	}
	// Un AUTRE accès qui l'a reçu ne vaut rien pour celui-ci (le porteur : « pas ma smart TV »).
	_, _ = db.Exec(`INSERT INTO federation_tokens (label, token_hash, prefix, scopes, created_at) VALUES ('tv', 'x', 'x', 'catalog', '2026-11-20')`)
	_, _ = db.Exec(`UPDATE federation_tokens SET remote_admin = 1 WHERE label = 'tv'`)
	if got := role(); got != auth.RoleUser {
		t.Fatalf("un autre accès autorisé : rang %d, attendu joueur", got)
	}
	accessID := strconv.FormatInt(access, 10)
	// Jamais depuis une session venue d'ailleurs.
	refused := httptest.NewRecorder()
	server.handleUpdateFederationToken(refused, withID(httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"remote_admin":true}`)), accessID),
		auth.Principal{ID: id, Username: "admin", RoleLevel: auth.RoleAdmin, Federated: true})
	if refused.Code != http.StatusForbidden {
		t.Fatalf("depuis une session venue d'ailleurs : %d, attendu 403", refused.Code)
	}
	if answer := adminCall(server, server.handleUpdateFederationToken, http.MethodPatch, accessID, `{"remote_admin":true}`); answer.Code != http.StatusOK {
		t.Fatalf("allumer : %d %s", answer.Code, answer.Body.String())
	}
	if got := role(); got != auth.RoleAdmin {
		t.Fatalf("autorisé : rang %d, attendu administrateur", got)
	}
	adminCall(server, server.handleUpdateFederationToken, http.MethodPatch, accessID, `{"remote_admin":false}`)
	if got := role(); got != auth.RoleUser {
		t.Fatalf("coupé de nouveau : rang %d, attendu joueur", got)
	}
}

func withID(request *http.Request, id string) *http.Request {
	request.SetPathValue("id", id)

	return request
}
