package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/database"

	"github.com/gorilla/websocket"
)

const testInternalSecret = "le-secret-interne"

// tokenServer — un service sur une base neuve, avec un administrateur (id 1) et
// un joueur (id 2).
func tokenServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	config.DBPath = filepath.Join(t.TempDir(), "credential.db")
	db, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, user := range []struct {
		name string
		role int
	}{{"admin", auth.RoleAdmin}, {"marc", auth.RoleUser}} {
		if _, err := db.Exec(`INSERT INTO users (username, hashed_password, display_name, role_level) VALUES (?, 'x', ?, ?)`,
			user.name, user.name, user.role); err != nil {
			t.Fatal(err)
		}
	}

	return &Server{db: db, config: config.Config{InternalSecret: testInternalSecret, JWTSecret: "le-secret-des-jetons"}}, db
}

func tokenOf(t *testing.T, db *sql.DB, userID int64) string {
	t.Helper()
	var token string
	if err := db.QueryRow(`SELECT token FROM account_tokens WHERE user_id = ?`, userID).Scan(&token); err != nil {
		t.Fatal(err)
	}

	return token
}

func adminCall(server *Server, handler func(http.ResponseWriter, *http.Request, auth.Principal), method, id, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/", strings.NewReader(body))
	request.SetPathValue("id", id)
	handler(recorder, request, auth.Principal{ID: 1, Username: "admin", RoleLevel: auth.RoleAdmin})

	return recorder
}

func internalCall(server *Server, handler http.HandlerFunc, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"token":"`+token+`"}`))
	request.Header.Set("X-Internal-Secret", testInternalSecret)
	handler(recorder, request)

	return recorder
}

// TOUT COMPTE A SON JETON, DÉSACTIVÉ — créé avec lui, quel que soit le chemin
// (le déclencheur), et pour les comptes d'avant (le démarrage).
func TestEveryAccountHasADisabledToken(t *testing.T) {
	_, db := tokenServer(t)
	var enabled, count int
	if err := db.QueryRow(`SELECT COUNT(*), MAX(enabled) FROM account_tokens`).Scan(&count, &enabled); err != nil {
		t.Fatal(err)
	}
	if count != 2 || enabled != 0 {
		t.Fatalf("%d jeton(s), actif max %d : attendu 2 jetons désactivés", count, enabled)
	}

	// Un compte d'avant le déclencheur : son jeton arrive au démarrage suivant.
	if _, err := db.Exec(`DELETE FROM account_tokens WHERE user_id = 2`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	reopened, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if tokenOf(t, reopened, 2) == "" {
		t.Fatal("le compte sans jeton n'en a pas reçu au démarrage")
	}
}

// ACTIVER DEMANDE UNE DURÉE ; désactivé, le jeton n'ouvre rien ; actif, il
// ouvre une session à son compte, et rend ses consoles.
func TestATokenOpensNothingUntilEnabledForAWhile(t *testing.T) {
	server, db := tokenServer(t)
	token := tokenOf(t, db, 2)

	if answer := internalCall(server, server.handleFederationSession, token); answer.Code != http.StatusUnauthorized {
		t.Fatalf("désactivé : %d, attendu 401", answer.Code)
	}
	if answer := adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true}`); answer.Code != http.StatusBadRequest {
		t.Fatalf("activé sans durée : %d, attendu 400", answer.Code)
	}
	if answer := adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"1m"}`); answer.Code != http.StatusOK {
		t.Fatalf("activé pour un mois : %d %s", answer.Code, answer.Body.String())
	}
	answer := internalCall(server, server.handleFederationSession, token)
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"username":"marc"`) || !strings.Contains(answer.Body.String(), `"consoles":["*"]`) {
		t.Fatalf("actif : %d %s", answer.Code, answer.Body.String())
	}

	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":false}`)
	if answer := internalCall(server, server.handleFederationSession, token); answer.Code != http.StatusUnauthorized {
		t.Fatalf("désactivé de nouveau : %d, attendu 401", answer.Code)
	}
}

// UN JETON ÉCHU NE VAUT PLUS, même resté actif.
func TestAnExpiredTokenIsRefused(t *testing.T) {
	server, db := tokenServer(t)
	token := tokenOf(t, db, 2)
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE account_tokens SET enabled = 1, expires_at = ? WHERE user_id = 2`, past); err != nil {
		t.Fatal(err)
	}
	if answer := internalCall(server, server.handleFederationSession, token); answer.Code != http.StatusUnauthorized {
		t.Fatalf("échu : %d, attendu 401", answer.Code)
	}
}

// RÉGÉNÉRER RETIRE L'ANCIEN : il ne vaut plus, le nouveau vaut, l'état reste,
// les compteurs repartent.
func TestRegeneratingRetiresTheOldToken(t *testing.T) {
	server, db := tokenServer(t)
	old := tokenOf(t, db, 2)
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"1w"}`)
	internalCall(server, server.handleFederationSession, old)

	if answer := adminCall(server, server.handleRegenerateAccountToken, http.MethodPost, "2", ""); answer.Code != http.StatusOK {
		t.Fatalf("régénérer : %d %s", answer.Code, answer.Body.String())
	}
	fresh := tokenOf(t, db, 2)
	if fresh == old {
		t.Fatal("le jeton n'a pas changé")
	}
	var uses int
	if err := db.QueryRow(`SELECT use_count FROM account_tokens WHERE user_id = 2`).Scan(&uses); err != nil || uses != 0 {
		t.Fatalf("compteur après régénération : %d / %v, attendu 0", uses, err)
	}
	if answer := internalCall(server, server.handleFederationSession, old); answer.Code != http.StatusUnauthorized {
		t.Fatalf("l'ancien : %d, attendu 401", answer.Code)
	}
	if answer := internalCall(server, server.handleFederationSession, fresh); answer.Code != http.StatusOK {
		t.Fatalf("le nouveau : %d, attendu 200 (l'état actif est gardé)", answer.Code)
	}
}

// LA SESSION D'UN JETON N'EST JAMAIS ADMINISTRATEUR, porte la génération du
// jeton, ne dure pas plus que lui, et compte une connexion.
func TestATokenSessionIsNeverAdminAndCountsAConnection(t *testing.T) {
	server, db := tokenServer(t)
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "1", `{"enabled":true,"duration":"1d"}`)
	token := tokenOf(t, db, 1)

	answer := internalCall(server, server.handleFederationSession, token)
	if answer.Code != http.StatusOK {
		t.Fatalf("session : %d %s", answer.Code, answer.Body.String())
	}
	var body struct {
		Item struct {
			AccessToken string `json:"access_token"`
			User        struct {
				Consoles []string `json:"consoles"`
			} `json:"user"`
		} `json:"item"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	claims, err := auth.ParseToken(server.config.JWTSecret, body.Item.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.RoleLevel != auth.RoleUser || claims.Username != "admin" || claims.Fed != token[:8] {
		t.Fatalf("session : rang %d, nom %q, génération %q", claims.RoleLevel, claims.Username, claims.Fed)
	}
	if time.Until(claims.ExpiresAt.Time) > 25*time.Hour {
		t.Fatalf("la session dure plus que le jeton (un jour) : %v", time.Until(claims.ExpiresAt.Time))
	}
	if len(body.Item.User.Consoles) != 1 || body.Item.User.Consoles[0] != "*" {
		t.Fatalf("consoles d'un administrateur : %v, attendu [*]", body.Item.User.Consoles)
	}
	var uses int
	if err := db.QueryRow(`SELECT use_count FROM account_tokens WHERE user_id = 1`).Scan(&uses); err != nil || uses != 1 {
		t.Fatalf("connexions : %d / %v, attendu 1", uses, err)
	}
}

// LES CONSOLES D'UN COMPTE se règlent, une console inconnue est refusée, un
// rang inventé aussi ; un administrateur a tout, quoi que dise sa ligne.
func TestConsolesAndRolesAreBounded(t *testing.T) {
	server, db := tokenServer(t)
	// La réponse remplace la ligne du membre sur place : son jeton y est.
	if answer := adminCall(server, server.handleUpdateUser, http.MethodPatch, "2", `{"consoles":["switch","retro"]}`); answer.Code != http.StatusOK ||
		!strings.Contains(answer.Body.String(), `"consoles":["retro","switch"]`) || !strings.Contains(answer.Body.String(), tokenOf(t, db, 2)) {
		t.Fatalf("consoles : %d %s", answer.Code, answer.Body.String())
	}
	if answer := adminCall(server, server.handleUpdateUser, http.MethodPatch, "2", `{"consoles":["ps33"]}`); answer.Code != http.StatusBadRequest {
		t.Fatalf("console inconnue : %d, attendu 400", answer.Code)
	}
	if answer := adminCall(server, server.handleUpdateUser, http.MethodPatch, "2", `{"consoles":[]}`); answer.Code != http.StatusOK ||
		!strings.Contains(answer.Body.String(), `"consoles":[]`) {
		t.Fatalf("aucune console : %d %s", answer.Code, answer.Body.String())
	}
	for _, role := range []string{"-1", "3"} {
		if answer := adminCall(server, server.handleUpdateUser, http.MethodPatch, "2", `{"role_level":`+role+`}`); answer.Code != http.StatusBadRequest {
			t.Fatalf("rang %s : %d, attendu 400", role, answer.Code)
		}
	}
	if _, err := db.Exec(`UPDATE users SET consoles = '' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if answer := adminCall(server, server.handleFetchUser, http.MethodGet, "1", ""); !strings.Contains(answer.Body.String(), `"consoles":["*"]`) {
		t.Fatalf("un administrateur a tout : %s", answer.Body.String())
	}
}

// LE PROFIL NE MONTRE LE JETON QUE S'IL EST ACTIF ; ses chiffres, toujours.
func TestMyTokenShowsItsValueOnlyWhenEnabled(t *testing.T) {
	server, db := tokenServer(t)
	token := tokenOf(t, db, 2)
	mine := func() string {
		recorder := httptest.NewRecorder()
		server.handleMyAccountToken(recorder, httptest.NewRequest(http.MethodGet, "/", nil),
			auth.Principal{ID: 2, Username: "marc", RoleLevel: auth.RoleUser})

		return recorder.Body.String()
	}
	if body := mine(); strings.Contains(body, token) || !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("désactivé : %s", body)
	}
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"6m"}`)
	if body := mine(); !strings.Contains(body, token) || !strings.Contains(body, `"consoles":["*"]`) {
		t.Fatalf("actif : %s", body)
	}
}

// LE JETON PART AVEC LE COMPTE.
func TestDeletingAnAccountDeletesItsToken(t *testing.T) {
	server, db := tokenServer(t)
	if answer := adminCall(server, server.handleDeleteUser, http.MethodDelete, "2", ""); answer.Code != http.StatusNoContent {
		t.Fatalf("supprimer : %d %s", answer.Code, answer.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM account_tokens WHERE user_id = 2`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("jetons restants : %d / %v", count, err)
	}
}

// LE KERNEL REÇOIT L'ÉTAT DES COMPTES : sans le secret interne, rien ; avec,
// la liste tout de suite, puis la liste à jour quand un compte change — ici,
// un compte supprimé qui disparaît de la liste.
func TestTheKernelReceivesTheAccounts(t *testing.T) {
	server, db := tokenServer(t)
	server.accounts = newAccountsHub()
	listener := httptest.NewServer(http.HandlerFunc(server.handleAccountsWS))
	defer listener.Close()
	address := "ws" + strings.TrimPrefix(listener.URL, "http")

	if _, response, err := websocket.DefaultDialer.Dial(address, nil); err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sans secret : %v, attendu 401", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(address, http.Header{"X-Internal-Secret": {testInternalSecret}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	read := func() []accountEntry {
		t.Helper()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var message struct {
			Accounts []accountEntry `json:"accounts"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatal(err)
		}

		return message.Accounts
	}
	if accounts := read(); len(accounts) != 2 || accounts[1].Username != "marc" || accounts[1].Token.Generation == "" || accounts[1].Token.Enabled {
		t.Fatalf("liste d'ouverture : %+v", accounts)
	}

	if _, err := db.Exec(`DELETE FROM users WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	server.accountsChanged()
	if accounts := read(); len(accounts) != 1 || accounts[0].Username != "admin" {
		t.Fatalf("après la suppression : %+v", accounts)
	}
}

// /auth/me REFUSE UNE SESSION DONT LE JETON A ÉTÉ RETIRÉ : la page du serveur qui
// l'a ouverte apprend ainsi qu'il faut la redemander.
func TestATokenSessionDiesWithItsToken(t *testing.T) {
	server, db := tokenServer(t)
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"1w"}`)
	answer := internalCall(server, server.handleFederationSession, tokenOf(t, db, 2))
	var body struct {
		Item struct {
			AccessToken string `json:"access_token"`
		} `json:"item"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	me := func() error {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		request.Header.Set("Authorization", "Bearer "+body.Item.AccessToken)
		_, err := auth.LoadUser(db, server.config.JWTSecret, request)

		return err
	}
	if err := me(); err != nil {
		t.Fatalf("session neuve : %v", err)
	}
	adminCall(server, server.handleRegenerateAccountToken, http.MethodPost, "2", "")
	if err := me(); err != auth.ErrInvalidToken {
		t.Fatalf("après régénération : %v, attendu ErrInvalidToken", err)
	}
}

// UNE SESSION OUVERTE PAR JETON JOUE, ELLE NE GÈRE PAS LE COMPTE : tous les
// membres du serveur qui l'a ouverte passent par elle — ils ne relisent pas le
// jeton, et ne changent pas le compte de celui qui le prête. Le compte, lui,
// connecté normalement, relit son jeton.
func TestATokenSessionCannotManageTheAccount(t *testing.T) {
	server, db := tokenServer(t)
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"1w"}`)
	answer := internalCall(server, server.handleFederationSession, tokenOf(t, db, 2))
	var body struct {
		Item struct {
			AccessToken string `json:"access_token"`
		} `json:"item"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	call := func(handler http.HandlerFunc, method, path, payload, bearer string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(payload))
		request.Header.Set("Authorization", "Bearer "+bearer)
		recorder := httptest.NewRecorder()
		handler(recorder, request)

		return recorder
	}

	if got := call(server.withUser(server.handleMyAccountToken), http.MethodGet, "/api/v1/auth/me/token", "", body.Item.AccessToken); got.Code != http.StatusForbidden || strings.Contains(got.Body.String(), tokenOf(t, db, 2)) {
		t.Fatalf("le jeton relu par sa propre session : %d %s", got.Code, got.Body.String())
	}
	if got := call(server.withUser(server.handleUpdateMe), http.MethodPatch, "/api/v1/auth/me", `{"display_name":"Pirate"}`, body.Item.AccessToken); got.Code != http.StatusForbidden {
		t.Fatalf("le compte changé par une session de jeton : %d %s", got.Code, got.Body.String())
	}
	owner, err := auth.CreateToken(server.config.JWTSecret, 2, "marc", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if got := call(server.withUser(server.handleMyAccountToken), http.MethodGet, "/api/v1/auth/me/token", "", owner); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), tokenOf(t, db, 2)) {
		t.Fatalf("le propriétaire ne relit pas son jeton : %d %s", got.Code, got.Body.String())
	}
}
