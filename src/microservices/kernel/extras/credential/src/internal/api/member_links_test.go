package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/presence"

	"github.com/gorilla/websocket"
)

const strongPassword = "Gameflex#2026"

// memberServer — tokenServer, avec le frein, la liste des kernels et la présence.
func memberServer(t *testing.T) *Server {
	t.Helper()
	server, _ := tokenServer(t)
	server.logins = newLoginLimiter()
	server.accounts = newAccountsHub()
	server.presence = presence.NewRegistry()

	return server
}

var admin = auth.Principal{ID: 1, Username: "admin", DisplayName: "admin", RoleLevel: auth.RoleAdmin}

// envelopeOf — la réponse d'une route : son élément, son refus.
type envelopeOf[T any] struct {
	Item      T `json:"item"`
	Exception struct {
		Detail any      `json:"detail"`
		Codes  []string `json:"codes"`
	} `json:"exception"`
}

func decoded[T any](t *testing.T, recorder *httptest.ResponseRecorder) envelopeOf[T] {
	t.Helper()
	var answer envelopeOf[T]
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("%d %s : %v", recorder.Code, recorder.Body.String(), err)
	}

	return answer
}

// asAdmin — une route d'administration, au nom de `principal`.
func asAdmin(server *Server, handler func(http.ResponseWriter, *http.Request, auth.Principal), principal auth.Principal, id, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	request.SetPathValue("id", id)
	handler(recorder, request, principal)

	return recorder
}

// public — une route sans jeton (le lien, la connexion), depuis `address`.
func public(handler http.HandlerFunc, body, address string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	request.Header.Set("X-Forwarded-For", address)
	handler(recorder, request)

	return recorder
}

// invite — « Nouveau membre » : le compte créé, et le code de son lien.
func invite(t *testing.T, server *Server, username string) (int64, string) {
	t.Helper()
	recorder := asAdmin(server, server.handleCreateUser, admin, "", `{"username":"`+username+`","display_name":"Léa Martin","role_level":2}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("création : %d %s", recorder.Code, recorder.Body.String())
	}
	answer := decoded[userResponse](t, recorder)
	if answer.Item.Invitation == nil || len(answer.Item.Invitation.Code) < 22 || answer.Item.Invitation.Kind != InvitationJoin {
		t.Fatalf("le lien de la création : %+v", answer.Item.Invitation)
	}

	return answer.Item.ID, answer.Item.Invitation.Code
}

type joined struct {
	AccessToken string       `json:"access_token"`
	User        userResponse `json:"user"`
}

func join(server *Server, code, password, address string) *httptest.ResponseRecorder {
	return public(server.handleJoin, `{"code":"`+code+`","password":"`+password+`"}`, address)
}

// me — /auth/me avec ce jeton : nil s'il vaut.
func me(server *Server, token string) error {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	_, err := auth.LoadUser(server.db, server.config.JWTSecret, request)

	return err
}

// UN NOUVEAU MEMBRE N'A PAS DE MOT DE PASSE, MAIS UN LIEN : la connexion le refuse
// avec la réponse d'un mot de passe faux, et compte l'échec ; son lien se lit sans
// compte, refuse un mot de passe faible (422, ses codes), puis l'accepte une fois
// — la personne est connectée, son jeton vaut, sa date est posée — et ne vaut
// plus rien ensuite (410).
func TestANewMemberJoinsThroughItsLinkOnce(t *testing.T) {
	server := memberServer(t)
	id, code := invite(t, server, "lea")

	login := func(password string) *httptest.ResponseRecorder {
		return public(server.handleLogin, `{"username":"lea","password":"`+password+`"}`, "10.0.0.5")
	}
	if recorder := login(""); recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "Invalid username or password.") {
		t.Fatalf("connexion d'un compte en attente : %d %s", recorder.Code, recorder.Body.String())
	}
	if pair, _ := loginKeys("lea", "10.0.0.5"); server.logins.entries[pair] == nil || server.logins.entries[pair].count != 1 {
		t.Fatal("l'échec d'un compte en attente n'est pas compté")
	}

	check := public(server.handleCheckJoin, `{"code":"`+code+`"}`, "10.0.0.5")
	if checked := decoded[map[string]any](t, check); check.Code != http.StatusOK || checked.Item["username"] != "lea" ||
		checked.Item["display_name"] != "Léa Martin" || checked.Item["kind"] != InvitationJoin || checked.Item["expires_at"] == "" {
		t.Fatalf("lecture du lien : %d %s", check.Code, check.Body.String())
	}

	weak := join(server, code, "motdepasse", "10.0.0.5")
	if refusal := decoded[map[string]any](t, weak); weak.Code != http.StatusUnprocessableEntity || strings.Join(refusal.Exception.Codes, ",") != "no_uppercase,no_digit,no_special" {
		t.Fatalf("mot de passe faible : %d %s", weak.Code, weak.Body.String())
	}

	accepted := join(server, code, strongPassword, "10.0.0.5")
	answer := decoded[joined](t, accepted).Item
	if accepted.Code != http.StatusOK || answer.AccessToken == "" || answer.User.Username != "lea" || answer.User.PasswordSetAt == nil {
		t.Fatalf("le lien accepté : %d %s", accepted.Code, accepted.Body.String())
	}
	if err := me(server, answer.AccessToken); err != nil {
		t.Fatalf("le jeton du lien sur /auth/me : %v", err)
	}
	if recorder := login(strongPassword); recorder.Code != http.StatusOK {
		t.Fatalf("connexion avec le mot de passe choisi : %d", recorder.Code)
	}
	if again := join(server, code, strongPassword, "10.0.0.5"); again.Code != http.StatusGone {
		t.Fatalf("second envoi : %d, attendu 410", again.Code)
	}
	if server.memberLink(id) != nil {
		t.Fatal("le lien survit au mot de passe choisi")
	}
}

// UN LIEN ÉCHU NE VAUT RIEN (410) ; UN NOUVEAU LIEN TUE L'ANCIEN, et n'est donné
// qu'à un membre qui attend (409 sinon : c'est une réinitialisation).
func TestALinkExpiresAndANewOneReplacesIt(t *testing.T) {
	server := memberServer(t)
	id, first := invite(t, server, "lea")

	renewed := asAdmin(server, server.handleNewMemberLink, admin, "2", "")
	if renewed.Code != http.StatusConflict {
		t.Fatalf("nouveau lien pour un membre qui a rejoint : %d, attendu 409", renewed.Code)
	}
	renewed = asAdmin(server, server.handleNewMemberLink, admin, "3", "")
	second := decoded[userResponse](t, renewed).Item.Invitation
	if renewed.Code != http.StatusCreated || second == nil || second.Code == "" || second.Code == first {
		t.Fatalf("nouveau lien : %d %s", renewed.Code, renewed.Body.String())
	}
	if recorder := public(server.handleCheckJoin, `{"code":"`+first+`"}`, "a"); recorder.Code != http.StatusGone {
		t.Fatalf("l'ancien lien : %d, attendu 410", recorder.Code)
	}
	var links int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM invitations WHERE user_id = ?`, id).Scan(&links)
	if links != 1 {
		t.Fatalf("%d liens vivants, attendu 1", links)
	}

	if _, err := server.db.Exec(`UPDATE invitations SET expires_at = ? WHERE user_id = ?`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), id); err != nil {
		t.Fatal(err)
	}
	if recorder := join(server, second.Code, strongPassword, "a"); recorder.Code != http.StatusGone {
		t.Fatalf("lien échu : %d, attendu 410", recorder.Code)
	}
}

// HUIT ENVOIS SIMULTANÉS, UN SEUL GAGNANT — comme les invitations du chantier 33.
func TestConcurrentJoinsHaveOneWinner(t *testing.T) {
	server := memberServer(t)
	_, code := invite(t, server, "lea")

	var group sync.WaitGroup
	codes := make(chan int, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			codes <- join(server, code, strongPassword, "a").Code
		}()
	}
	group.Wait()
	close(codes)
	won := 0
	for status := range codes {
		switch status {
		case http.StatusOK:
			won++
		case http.StatusGone:
		default:
			t.Fatalf("envoi : %d", status)
		}
	}
	if won != 1 {
		t.Fatalf("%d gagnants, attendu 1", won)
	}
}

// LA FÉDÉRATION NE VOIT PAS LES LIENS DES COMPTES : ni dans sa liste, ni à sa
// révocation, et un code de lien présenté à son échange — sous sa sorte ou sous
// la sienne — répond « d'une autre sorte », sans être consommé.
func TestTheFederationIgnoresMemberLinks(t *testing.T) {
	server := memberServer(t)
	_, code := invite(t, server, "lea")

	if list := adminCall(server, server.handleListInvitations, http.MethodGet, "", ""); strings.Contains(list.Body.String(), "Léa") {
		t.Fatalf("la liste de la fédération montre un lien de compte : %s", list.Body.String())
	}
	var linkID string
	_ = server.db.QueryRow(`SELECT id FROM invitations WHERE kind = ?`, InvitationJoin).Scan(&linkID)
	if recorder := adminCall(server, server.handleRevokeInvitation, http.MethodDelete, linkID, ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("révocation d'un lien de compte par la fédération : %d, attendu 404", recorder.Code)
	}
	for _, kind := range []string{InvitationFederation, InvitationAccount, InvitationJoin, InvitationReset} {
		if _, err := server.RedeemInvitation(code, kind, "", "a"); !errors.Is(err, ErrInvitationKind) {
			t.Fatalf("échange d'un lien de compte en %q : %v, attendu « d'une autre sorte »", kind, err)
		}
	}
	if recorder := join(server, code, strongPassword, "a"); recorder.Code != http.StatusOK {
		t.Fatalf("le lien après les échanges refusés : %d", recorder.Code)
	}
}

// L'ADMINISTRATEUR NE POSE PLUS DE MOT DE PASSE : `password` est refusé à la
// création comme à la modification (400).
func TestAnAdministratorNoLongerSetsAPassword(t *testing.T) {
	server := memberServer(t)
	if recorder := asAdmin(server, server.handleCreateUser, admin, "", `{"username":"lea","display_name":"Léa","password":"Gameflex#2026"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("création avec un mot de passe : %d, attendu 400", recorder.Code)
	}
	if recorder := asAdmin(server, server.handleUpdateUser, admin, "2", `{"password":"Gameflex#2026"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("modification avec un mot de passe : %d, attendu 400", recorder.Code)
	}
}

// LES CODES DEVINÉS SONT FREINÉS PAR ADRESSE (429 et Retry-After) ; une autre
// adresse ne l'est pas.
func TestGuessedLinksAreThrottled(t *testing.T) {
	server := memberServer(t)
	for range loginFreeTries {
		if recorder := public(server.handleCheckJoin, `{"code":"pas-un-code"}`, "10.0.0.9"); recorder.Code != http.StatusGone {
			t.Fatalf("code inconnu : %d", recorder.Code)
		}
	}
	if recorder := public(server.handleJoin, `{"code":"pas-un-code","password":"x"}`, "10.0.0.9"); recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("après %d échecs : %d, Retry-After %q", loginFreeTries, recorder.Code, recorder.Header().Get("Retry-After"))
	}
	if recorder := public(server.handleCheckJoin, `{"code":"pas-un-code"}`, "10.0.0.10"); recorder.Code != http.StatusGone {
		t.Fatalf("une autre adresse : %d", recorder.Code)
	}
}

// LE FILTRE « INVITATION VALIDÉE » : password_set_at se cherche.
func TestTheMembersListFiltersOnAJoinedInvitation(t *testing.T) {
	server := memberServer(t)
	invite(t, server, "lea")
	if _, err := server.db.Exec(`UPDATE users SET password_set_at = CURRENT_TIMESTAMP WHERE id IN (1, 2)`); err != nil {
		t.Fatal(err)
	}
	list := func(search string) string {
		recorder := httptest.NewRecorder()
		server.handleListUsers(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/users?search="+search, nil), admin)
		var answer struct {
			Items []userResponse `json:"items"`
		}
		_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
		names := []string{}
		for _, item := range answer.Items {
			names = append(names, item.Username)
		}

		return strings.Join(names, ",")
	}
	if got := list(`{"password_set_at":{"not_null":true}}`); got != "admin,marc" {
		t.Fatalf("ont rejoint : %q", got)
	}
	if got := list(`{"password_set_at":{"null":true}}`); got != "lea" {
		t.Fatalf("attendent : %q", got)
	}
}

// presenceOf — une connexion de présence avec ce jeton, et ce qui la ferme.
func presenceOf(t *testing.T, server *Server, token string) (*websocket.Conn, func() int) {
	t.Helper()
	listener := httptest.NewServer(http.HandlerFunc(server.handlePresenceWS))
	t.Cleanup(listener.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(listener.URL, "http")+"/?token="+token, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	// La connexion s'inscrit après la poignée de main : l'attendre.
	for deadline := time.Now().Add(3 * time.Second); len(server.presence.Snapshot()) == 0 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}

	return conn, func() int {
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				var closed *websocket.CloseError
				if errors.As(err, &closed) {
					return closed.Code
				}

				return 0
			}
		}
	}
}

// RÉINITIALISER COUPE TOUT, ET DONNE UN NOUVEAU LIEN : l'ancien mot de passe est
// refusé, l'ancien jeton aussi (/auth/me), sa présence se ferme en 1008, la liste
// des kernels porte la génération des sessions qui a avancé, et celle des parties
// (le kernel les finit) ; le lien `reset` le fait rejoindre, et le jeton du
// compte n'a pas changé.
func TestAResetEndsEverySessionAndGivesANewLink(t *testing.T) {
	server := memberServer(t)
	id, code := invite(t, server, "lea")
	session := decoded[joined](t, join(server, code, strongPassword, "a")).Item.AccessToken
	accountToken := tokenOf(t, server.db, id)
	_, closedWith := presenceOf(t, server, session)
	slot := server.accounts.subscribe()

	reset := asAdmin(server, server.handleResetMember, admin, "3", "")
	link := decoded[userResponse](t, reset).Item.Invitation
	if reset.Code != http.StatusOK || link == nil || link.Kind != InvitationReset || link.Code == "" || link.CreatedBy != "admin" {
		t.Fatalf("réinitialisation : %d %s", reset.Code, reset.Body.String())
	}
	if err := me(server, session); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("l'ancien jeton après la réinitialisation : %v, attendu ErrInvalidToken", err)
	}
	if code := closedWith(); code != websocket.ClosePolicyViolation {
		t.Fatalf("la présence de l'ancien jeton : fermée en %d, attendu 1008", code)
	}
	if recorder := public(server.handleLogin, `{"username":"lea","password":"`+strongPassword+`"}`, "a"); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("l'ancien mot de passe : %d, attendu 401", recorder.Code)
	}
	var list []accountEntry
	select {
	case payload := <-slot:
		accounts, _ := decodeAccounts(payload)
		list = accounts
	case <-time.After(3 * time.Second):
		t.Fatal("aucune liste aux kernels")
	}
	if entry := list[2]; entry.Username != "lea" || entry.SessionGeneration != 1 || entry.GamesGeneration != 1 {
		t.Fatalf("la liste des kernels : %+v", entry)
	}
	if tokenOf(t, server.db, id) != accountToken {
		t.Fatal("la réinitialisation a changé le jeton du compte")
	}

	if check := decoded[map[string]any](t, public(server.handleCheckJoin, `{"code":"`+link.Code+`"}`, "a")); check.Item["kind"] != InvitationReset {
		t.Fatalf("le lien de réinitialisation : %+v", check.Item)
	}
	again := decoded[joined](t, join(server, link.Code, "Nouveau#2026", "a")).Item
	if err := me(server, again.AccessToken); err != nil {
		t.Fatalf("le jeton du nouveau lien : %v", err)
	}
}

func decodeAccounts(payload []byte) ([]accountEntry, error) {
	var message struct {
		Accounts []accountEntry `json:"accounts"`
	}
	err := json.Unmarshal(payload, &message)

	return message.Accounts, err
}

// NI ROOT, NI SON PROPRE COMPTE (403), NI UN MEMBRE QUI ATTEND (409).
func TestAResetIsRefusedForRootOneselfAndAPendingMember(t *testing.T) {
	server := memberServer(t)
	if _, err := server.db.Exec(`INSERT INTO users (username, hashed_password, display_name, role_level) VALUES ('root', 'x', 'Root', 0)`); err != nil {
		t.Fatal(err)
	}
	invite(t, server, "lea")
	for _, check := range []struct {
		id   string
		want int
	}{{"3", http.StatusForbidden}, {"1", http.StatusForbidden}, {"4", http.StatusConflict}, {"99", http.StatusNotFound}} {
		if recorder := asAdmin(server, server.handleResetMember, admin, check.id, ""); recorder.Code != check.want {
			t.Fatalf("réinitialiser %s : %d, attendu %d", check.id, recorder.Code, check.want)
		}
	}
}

// MON MOT DE PASSE SUIT LA RÈGLE ET DÉCONNECTE LES AUTRES APPAREILS : un mot de
// passe faible est refusé (422) ; accepté, l'ancien jeton ne vaut plus, la réponse
// porte un jeton neuf qui vaut, et un autre compte n'est pas touché.
func TestChangingMyPasswordSignsOutMyOtherDevices(t *testing.T) {
	server := memberServer(t)
	id, code := invite(t, server, "lea")
	before := decoded[joined](t, join(server, code, strongPassword, "a")).Item.AccessToken
	other, err := auth.CreateToken(server.config.JWTSecret, 2, "marc", auth.RoleUser, 0)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := auth.LoadToken(server.db, server.config.JWTSecret, before)
	if err != nil {
		t.Fatal(err)
	}
	change := func(next string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		server.handleUpdateMe(recorder, httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"current_password":"`+strongPassword+`","new_password":"`+next+`"}`)), principal)

		return recorder
	}

	if weak := change("Court#1"); weak.Code != http.StatusUnprocessableEntity || strings.Join(decoded[map[string]any](t, weak).Exception.Codes, ",") != "too_short" {
		t.Fatalf("mot de passe faible : %d %s", weak.Code, weak.Body.String())
	}
	changed := change("Nouveau#2026")
	answer := decoded[struct {
		AccessToken string `json:"access_token"`
		Username    string `json:"username"`
	}](t, changed).Item
	if changed.Code != http.StatusOK || answer.AccessToken == "" || answer.Username != "lea" {
		t.Fatalf("changement : %d %s", changed.Code, changed.Body.String())
	}
	if err := me(server, before); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("l'ancien jeton : %v, attendu ErrInvalidToken", err)
	}
	if err := me(server, answer.AccessToken); err != nil {
		t.Fatalf("le jeton neuf : %v", err)
	}
	if err := me(server, other); err != nil {
		t.Fatalf("un autre compte touché : %v", err)
	}
	_ = id
}

// SE DÉCONNECTER DES AUTRES APPAREILS, OU DE TOUS : la génération avance ; le
// premier rend un jeton neuf qui vaut, le second rien.
func TestSigningOutOtherDevicesOrAll(t *testing.T) {
	server := memberServer(t)
	_, code := invite(t, server, "lea")
	first := decoded[joined](t, join(server, code, strongPassword, "a")).Item.AccessToken
	second := decoded[joined](t, public(server.handleLogin, `{"username":"lea","password":"`+strongPassword+`"}`, "b")).Item.AccessToken
	principal, err := auth.LoadToken(server.db, server.config.JWTSecret, first)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	server.handleLogoutOthers(recorder, httptest.NewRequest(http.MethodPost, "/", nil), principal)
	kept := decoded[map[string]any](t, recorder).Item["access_token"]
	if recorder.Code != http.StatusOK || kept == nil {
		t.Fatalf("les autres appareils : %d %s", recorder.Code, recorder.Body.String())
	}
	if me(server, first) == nil || me(server, second) == nil {
		t.Fatal("un appareil d'avant vaut encore")
	}
	if err := me(server, kept.(string)); err != nil {
		t.Fatalf("le jeton gardé : %v", err)
	}

	principal, _ = auth.LoadToken(server.db, server.config.JWTSecret, kept.(string))
	recorder = httptest.NewRecorder()
	server.handleLogoutAll(recorder, httptest.NewRequest(http.MethodPost, "/", nil), principal)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "access_token") {
		t.Fatalf("tous les appareils : %d %s", recorder.Code, recorder.Body.String())
	}
	if me(server, kept.(string)) == nil {
		t.Fatal("cet appareil vaut encore après « tous les appareils »")
	}
	federated := principal
	federated.Federated = true
	recorder = httptest.NewRecorder()
	server.handleLogoutAll(recorder, httptest.NewRequest(http.MethodPost, "/", nil), federated)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("une session ouverte par jeton coupe le compte : %d", recorder.Code)
	}
}

// leaInTheList — la prochaine liste des kernels : la ligne de lea, et les comptes
// qu'elle dit supprimés.
func leaInTheList(t *testing.T, slot chan []byte) (entry accountEntry, found bool, deleted []string) {
	t.Helper()
	select {
	case payload := <-slot:
		var message struct {
			Accounts []accountEntry `json:"accounts"`
			Deleted  []string       `json:"deleted"`
		}
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatal(err)
		}
		for _, account := range message.Accounts {
			if account.Username == "lea" {
				return account, true, message.Deleted
			}
		}

		return accountEntry{}, false, message.Deleted
	case <-time.After(3 * time.Second):
		t.Fatal("aucune liste aux kernels")
	}

	return accountEntry{}, false, nil
}

// TOUS LES APPAREILS FINIT AUSSI SES PARTIES, PAS LES AUTRES APPAREILS NI « MON MOT
// DE PASSE » (le porteur, 2026-11-14 : « Oui ») : la liste des kernels porte la
// génération des parties, inchangée tant que cet appareil reste connecté, avancée
// avec « Déconnecter tous les appareils ».
func TestSigningOutAllDevicesEndsTheGamesButNotTheOthers(t *testing.T) {
	server := memberServer(t)
	_, code := invite(t, server, "lea")
	session := decoded[joined](t, join(server, code, strongPassword, "a")).Item.AccessToken
	slot := server.accounts.subscribe()
	principalOf := func(token string) auth.Principal {
		t.Helper()
		principal, err := auth.LoadToken(server.db, server.config.JWTSecret, token)
		if err != nil {
			t.Fatal(err)
		}

		return principal
	}
	keptToken := func(recorder *httptest.ResponseRecorder) string {
		t.Helper()
		token, _ := decoded[map[string]any](t, recorder).Item["access_token"].(string)
		if recorder.Code != http.StatusOK || token == "" {
			t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
		}

		return token
	}

	recorder := httptest.NewRecorder()
	server.handleLogoutOthers(recorder, httptest.NewRequest(http.MethodPost, "/", nil), principalOf(session))
	session = keptToken(recorder)
	if entry, _, _ := leaInTheList(t, slot); entry.SessionGeneration != 1 || entry.GamesGeneration != 0 {
		t.Fatalf("les autres appareils : %+v, attendu les sessions à 1 et les parties à 0", entry)
	}

	recorder = httptest.NewRecorder()
	server.handleUpdateMe(recorder, httptest.NewRequest(http.MethodPatch, "/",
		strings.NewReader(`{"current_password":"`+strongPassword+`","new_password":"Nouveau#2026"}`)), principalOf(session))
	session = keptToken(recorder)
	if entry, _, _ := leaInTheList(t, slot); entry.SessionGeneration != 2 || entry.GamesGeneration != 0 {
		t.Fatalf("mon mot de passe : %+v, attendu les sessions à 2 et les parties à 0", entry)
	}

	recorder = httptest.NewRecorder()
	server.handleLogoutAll(recorder, httptest.NewRequest(http.MethodPost, "/", nil), principalOf(session))
	if recorder.Code != http.StatusOK {
		t.Fatalf("tous les appareils : %d %s", recorder.Code, recorder.Body.String())
	}
	if entry, _, _ := leaInTheList(t, slot); entry.SessionGeneration != 3 || entry.GamesGeneration != 1 {
		t.Fatalf("tous les appareils : %+v, attendu les sessions à 3 et les parties à 1", entry)
	}
}

// SUPPRIMER UN MEMBRE LE NOMME AUX KERNELS (le porteur, 2026-11-14 : « Supprimer ») :
// la liste qui suit ne l'a plus, et `deleted` le nomme — le kernel efface ses
// sauvegardes sur ce mot, jamais sur un nom qui manque. Une heure après, la liste
// ne le dit plus.
func TestADeletedMemberIsNamedToTheKernels(t *testing.T) {
	server := memberServer(t)
	invite(t, server, "lea")
	slot := server.accounts.subscribe()

	if recorder := asAdmin(server, server.handleDeleteUser, admin, "3", ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("suppression : %d %s", recorder.Code, recorder.Body.String())
	}
	_, found, deleted := leaInTheList(t, slot)
	if found || strings.Join(deleted, ",") != "lea" {
		t.Fatalf("la liste après la suppression : lea présente %v, supprimés %v", found, deleted)
	}
	if names := server.deleted.recent(time.Now().Add(deletedAccountsKept)); len(names) != 0 {
		t.Fatalf("une heure après : %v, attendu plus rien", names)
	}
	if payload, err := server.accountsSnapshot(); err != nil || !strings.Contains(string(payload), `"deleted":[]`) {
		t.Fatalf("la liste d'un service qui n'a rien supprimé (ou oublié) : %s %v", payload, err)
	}
}
