package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/mfa"
)

// LA DOUBLE AUTHENTIFICATION (chantier 80, mfa.go), de bout en bout, par les routes.

const mfaPassword = "le-mot-de-passe-de-marc"

func mfaServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	server, db := tokenServer(t)
	hashed, err := auth.HashPassword(mfaPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET hashed_password = ? WHERE username IN ('marc', 'admin')`, hashed); err != nil {
		t.Fatal(err)
	}

	return server, db
}

func mfaAnswer(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Item map[string]any `json:"item"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}

	return envelope.Item
}

func mfaLogin(t *testing.T, server *Server, username string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"`+username+`","password":"`+mfaPassword+`"}`))
	request.RemoteAddr = "203.0.113.7:51000"
	server.handleLogin(response, request)

	return response
}

func mfaSecondStep(t *testing.T, server *Server, token, code string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/mfa",
		strings.NewReader(`{"mfa_token":"`+token+`","code":"`+code+`"}`))
	request.RemoteAddr = "203.0.113.7:51000"
	server.handleLoginMfa(response, request)

	return response
}

func mfaCall(t *testing.T, handler func(http.ResponseWriter, *http.Request, auth.Principal), principal auth.Principal, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	handler(response, request, principal)

	return response
}

func codeFor(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := mfa.CodeAt(secret, at)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func userIDOf(t *testing.T, db *sql.DB, username string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id); err != nil {
		t.Fatal(err)
	}

	return id
}

// marc active sa double authentification lui-même ; rend son secret et ses codes de secours.
func enableFor(t *testing.T, server *Server, principal auth.Principal) (string, []string) {
	t.Helper()
	setup := mfaCall(t, server.handleMfaSetup, principal, `{"password":"`+mfaPassword+`"}`)
	if setup.Code != http.StatusOK {
		t.Fatalf("setup: %d %s", setup.Code, setup.Body.String())
	}
	secret := mfaAnswer(t, setup)["secret"].(string)
	if !strings.Contains(mfaAnswer(t, setup)["uri"].(string), "otpauth://totp/") {
		t.Fatal("no otpauth uri")
	}
	if wrong := mfaCall(t, server.handleMfaEnable, principal, `{"code":"000000"}`); wrong.Code == http.StatusOK && codeFor(t, secret, time.Now()) != "000000" {
		t.Fatal("a wrong first code enables it")
	}
	enable := mfaCall(t, server.handleMfaEnable, principal, `{"code":"`+codeFor(t, secret, time.Now())+`"}`)
	if enable.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", enable.Code, enable.Body.String())
	}
	var codes []string
	for _, code := range mfaAnswer(t, enable)["recovery_codes"].([]any) {
		codes = append(codes, code.(string))
	}
	if len(codes) != mfa.RecoveryCount {
		t.Fatalf("%d recovery codes", len(codes))
	}

	return secret, codes
}

// SANS DOUBLE AUTHENTIFICATION, RIEN NE CHANGE : le mot de passe ouvre la session.
func TestWithoutItTheSignInIsUnchanged(t *testing.T) {
	server, _ := mfaServer(t)
	answer := mfaAnswer(t, mfaLogin(t, server, "marc"))
	if answer["access_token"] == nil || answer["mfa"] != nil {
		t.Fatalf("%v", answer)
	}
}

// ACTIVÉE PAR LE JOUEUR : le mot de passe ne suffit plus ; le code juste ouvre, une seule fois ; un code de secours
// aussi, une seule fois.
func TestAnEnabledAccountNeedsItsCode(t *testing.T) {
	server, db := mfaServer(t)
	marc := auth.Principal{ID: userIDOf(t, db, "marc"), Username: "marc", RoleLevel: auth.RoleUser}
	secret, recovery := enableFor(t, server, marc)

	first := mfaAnswer(t, mfaLogin(t, server, "marc"))
	if first["mfa"] != "code" || first["access_token"] != nil {
		t.Fatalf("the password alone signs in: %v", first)
	}
	token := first["mfa_token"].(string)
	if response := mfaSecondStep(t, server, token, "123456"); response.Code != http.StatusUnauthorized && codeFor(t, secret, time.Now()) != "123456" {
		t.Fatalf("a wrong code: %d", response.Code)
	}
	// Le pas de l'activation est pris : le code suivant (dans la marge d'un pas).
	next := codeFor(t, secret, time.Now().Add(30*time.Second))
	granted := mfaSecondStep(t, server, token, next)
	if granted.Code != http.StatusOK || mfaAnswer(t, granted)["access_token"] == nil {
		t.Fatalf("the right code: %d %s", granted.Code, granted.Body.String())
	}

	// LE MÊME CODE NE SERT PAS DEUX FOIS.
	again := mfaAnswer(t, mfaLogin(t, server, "marc"))["mfa_token"].(string)
	if response := mfaSecondStep(t, server, again, next); response.Code != http.StatusUnauthorized {
		t.Fatalf("a code used twice: %d", response.Code)
	}

	// UN CODE DE SECOURS, UNE FOIS.
	viaRecovery := mfaAnswer(t, mfaLogin(t, server, "marc"))["mfa_token"].(string)
	if response := mfaSecondStep(t, server, viaRecovery, strings.ToLower(recovery[0])); response.Code != http.StatusOK {
		t.Fatalf("a recovery code: %d %s", response.Code, response.Body.String())
	}
	reused := mfaAnswer(t, mfaLogin(t, server, "marc"))["mfa_token"].(string)
	if response := mfaSecondStep(t, server, reused, recovery[0]); response.Code != http.StatusUnauthorized {
		t.Fatalf("a recovery code used twice: %d", response.Code)
	}
	state := mfaAnswer(t, mfaCall(t, server.handleMfaState, marc, ""))
	if state["enabled"] != true || state["recovery_left"] != float64(mfa.RecoveryCount-1) {
		t.Fatalf("state: %v", state)
	}
}

// CINQ CODES FAUX, ET L'ÉTAPE NE VAUT PLUS : il faut redonner le mot de passe.
func TestFiveWrongCodesEndTheSignIn(t *testing.T) {
	server, db := mfaServer(t)
	marc := auth.Principal{ID: userIDOf(t, db, "marc"), Username: "marc", RoleLevel: auth.RoleUser}
	secret, _ := enableFor(t, server, marc)
	token := mfaAnswer(t, mfaLogin(t, server, "marc"))["mfa_token"].(string)
	wrong := "000000"
	if codeFor(t, secret, time.Now()) == wrong || codeFor(t, secret, time.Now().Add(30*time.Second)) == wrong {
		wrong = "111111"
	}
	for i := 0; i < mfaChallengeAttempts; i++ {
		mfaSecondStep(t, server, token, wrong)
	}
	response := mfaSecondStep(t, server, token, codeFor(t, secret, time.Now().Add(30*time.Second)))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "mfa_expired") {
		t.Fatalf("after five wrong codes: %d %s", response.Code, response.Body.String())
	}
}

// IMPOSÉE PAR L'ADMINISTRATEUR : le membre l'active à sa connexion, avant d'entrer ; ses codes de secours avec la
// session. Il ne peut plus la désactiver ; l'administrateur peut la réinitialiser.
func TestAnAdministratorRequiresItAndResetsIt(t *testing.T) {
	server, db := mfaServer(t)
	marcID := userIDOf(t, db, "marc")
	admin := auth.Principal{ID: userIDOf(t, db, "admin"), Username: "admin", RoleLevel: auth.RoleAdmin}
	if _, err := db.Exec(`UPDATE users SET mfa_required = 1 WHERE id = ?`, marcID); err != nil {
		t.Fatal(err)
	}
	if !server.fetchUserResponse(marcID).MfaRequired {
		t.Fatal("the member does not say it is required")
	}

	first := mfaAnswer(t, mfaLogin(t, server, "marc"))
	if first["mfa"] != "enroll" || first["secret"] == nil || first["access_token"] != nil {
		t.Fatalf("a required account signs in without enrolling: %v", first)
	}
	secret := first["secret"].(string)
	granted := mfaSecondStep(t, server, first["mfa_token"].(string), codeFor(t, secret, time.Now()))
	answer := mfaAnswer(t, granted)
	if granted.Code != http.StatusOK || answer["access_token"] == nil || len(answer["recovery_codes"].([]any)) != mfa.RecoveryCount {
		t.Fatalf("enrolment at sign-in: %d %v", granted.Code, answer)
	}
	if !server.fetchUserResponse(marcID).MfaEnabled {
		t.Fatal("the member does not say it is enabled")
	}

	marc := auth.Principal{ID: marcID, Username: "marc", RoleLevel: auth.RoleUser}
	disable := mfaCall(t, server.handleMfaDisable, marc, `{"password":"`+mfaPassword+`","code":"`+codeFor(t, secret, time.Now().Add(30*time.Second))+`"}`)
	if disable.Code != http.StatusForbidden {
		t.Fatalf("a required one is turned off: %d", disable.Code)
	}

	reset := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.SetPathValue("id", jsonID(marcID))
	server.handleMfaReset(reset, request, admin)
	if reset.Code != http.StatusOK || server.fetchUserResponse(marcID).MfaEnabled {
		t.Fatalf("reset: %d %s", reset.Code, reset.Body.String())
	}
	if mfaAnswer(t, mfaLogin(t, server, "marc"))["mfa"] != "enroll" {
		t.Fatal("after the reset, a required account does not enroll again")
	}

	// L'administrateur ne réinitialise ni root ni lui-même.
	own := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/", nil)
	request.SetPathValue("id", jsonID(admin.ID))
	server.handleMfaReset(own, request, admin)
	if own.Code != http.StatusForbidden {
		t.Fatalf("own reset: %d", own.Code)
	}
}

func jsonID(id int64) string {
	raw, _ := json.Marshal(id)

	return string(raw)
}

// IMPOSÉE À TOUT LE SERVEUR : tout compte l'active à sa connexion. JAMAIS AU TÉLÉPHONE.
func TestTheServerWideRuleIsForServersOnly(t *testing.T) {
	server, db := mfaServer(t)
	admin := auth.Principal{ID: userIDOf(t, db, "admin"), Username: "admin", RoleLevel: auth.RoleAdmin}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"required_all":true}`))
	server.handleMfaPolicy(response, request, admin)
	if response.Code != http.StatusOK || mfaAnswer(t, response)["required_all"] != true {
		t.Fatalf("policy: %d %s", response.Code, response.Body.String())
	}
	if mfaAnswer(t, mfaLogin(t, server, "admin"))["mfa"] != "enroll" {
		t.Fatal("the server-wide rule does not apply")
	}

	server.config.Device = true
	if mfaAnswer(t, mfaLogin(t, server, "marc"))["access_token"] == nil {
		t.Fatal("the server-wide rule applies on the phone")
	}
	refused := httptest.NewRecorder()
	server.handleMfaPolicy(refused, httptest.NewRequest(http.MethodGet, "/", nil), admin)
	if refused.Code != http.StatusNotFound {
		t.Fatalf("the phone has the setting: %d", refused.Code)
	}
}

// LA CLÉ EFFACÉE REMET À ZÉRO : un secret scellé avec l'ancienne ne s'ouvre plus, l'activation se refait.
func TestADeletedKeyStartsEveryoneAgain(t *testing.T) {
	server, db := mfaServer(t)
	marc := auth.Principal{ID: userIDOf(t, db, "marc"), Username: "marc", RoleLevel: auth.RoleUser}
	enableFor(t, server, marc)
	if err := os.Remove(filepath.Join(config.KeysDir, "mfa.key")); err != nil {
		t.Fatal(err)
	}
	server.mfa.key = nil
	if answer := mfaAnswer(t, mfaLogin(t, server, "marc")); answer["access_token"] == nil {
		t.Fatalf("a free account with an unreadable secret: %v", answer)
	}
	if server.fetchUserResponse(marc.ID).MfaEnabled {
		t.Fatal("the unreadable activation is still listed")
	}
}

// UNE SESSION VENUE D'UN SERVEUR FÉDÉRÉ ne touche pas à la double authentification du compte.
func TestAFederatedSessionCannotChangeIt(t *testing.T) {
	server, db := mfaServer(t)
	marc := auth.Principal{ID: userIDOf(t, db, "marc"), Username: "marc", RoleLevel: auth.RoleUser, Federated: true}
	if response := mfaCall(t, server.handleMfaSetup, marc, `{"password":"`+mfaPassword+`"}`); response.Code != http.StatusForbidden {
		t.Fatalf("setup from a federated session: %d", response.Code)
	}
}
