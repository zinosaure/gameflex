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
)

// invitationServer — tokenServer, avec le frein des échecs.
func invitationServer(t *testing.T) *Server {
	t.Helper()
	server, _ := tokenServer(t)
	server.logins = newLoginLimiter()

	return server
}

// createInvitation — POST /api/v1/token-invitations au nom de `principal`.
func createInvitation(t *testing.T, server *Server, principal auth.Principal, body string) (int, invitationResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.handleCreateInvitation(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), principal)
	var envelope struct {
		Item invitationResponse `json:"item"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)

	return recorder.Code, envelope.Item
}

var (
	inviter = auth.Principal{ID: 1, Username: "admin", DisplayName: "admin", RoleLevel: auth.RoleAdmin}
	invitee = auth.Principal{ID: 2, Username: "marc", DisplayName: "Marc", RoleLevel: auth.RoleUser}
)

// UNE INVITATION DE FÉDÉRATION DEVIENT UN JETON DE CATALOGUE, UNE FOIS : son
// nom et ses portées, un jeton que la vérification accepte, puis plus rien.
func TestFederationInvitationIsRedeemedOnce(t *testing.T) {
	server := invitationServer(t)
	code, created := createInvitation(t, server, inviter, `{"kind":"federation","label":"Marc","scopes":["catalog","devices"]}`)
	if code != http.StatusCreated || len(created.Code) < 22 {
		t.Fatalf("création : %d, code %q", code, created.Code)
	}

	redeemed, err := server.RedeemInvitation(created.Code, InvitationFederation, "10.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	if redeemed.Identity != "Marc" || redeemed.Scopes != "catalog,devices" || len(redeemed.Token) < 40 {
		t.Fatalf("échange : %+v", redeemed)
	}
	if recorder := internalCall(server, server.handleVerifyFederationToken, redeemed.Token); recorder.Code != http.StatusOK {
		t.Fatalf("le jeton né de l'échange est refusé : %d", recorder.Code)
	}

	if _, err := server.RedeemInvitation(created.Code, InvitationFederation, "10.0.0.2"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatalf("second échange : %v, attendu un refus", err)
	}
	var count int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM federation_tokens`).Scan(&count)
	if count != 1 {
		t.Fatalf("%d jetons de catalogue, attendu 1", count)
	}
}

// ÉCHUE, ELLE NE VAUT RIEN ET QUITTE LA LISTE ; RÉVOQUÉE, PAREIL.
func TestExpiredOrRevokedInvitationIsRefused(t *testing.T) {
	server := invitationServer(t)
	_, expired := createInvitation(t, server, inviter, `{"kind":"federation","label":"Échue"}`)
	_, revoked := createInvitation(t, server, inviter, `{"kind":"federation","label":"Révoquée"}`)
	if _, err := server.db.Exec(`UPDATE invitations SET expires_at = ? WHERE id = ?`,
		time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), expired.ID); err != nil {
		t.Fatal(err)
	}

	recorder := adminCall(server, server.handleListInvitations, http.MethodGet, "", "")
	if strings.Contains(recorder.Body.String(), "Échue") || !strings.Contains(recorder.Body.String(), "Révoquée") {
		t.Fatalf("liste : %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), revoked.Code) {
		t.Fatal("la liste montre le code")
	}
	if _, err := server.RedeemInvitation(expired.Code, InvitationFederation, "a"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatalf("échue : %v", err)
	}

	if recorder := adminCall(server, server.handleRevokeInvitation, http.MethodDelete, "2", ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("révocation : %d", recorder.Code)
	}
	if _, err := server.RedeemInvitation(revoked.Code, InvitationFederation, "a"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatalf("révoquée : %v", err)
	}
}

// COLLÉE AU MAUVAIS ENDROIT, ELLE N'EST PAS PERDUE : rien n'est consommé.
func TestInvitationOfAnotherKindIsNotConsumed(t *testing.T) {
	server := invitationServer(t)
	_, created := createInvitation(t, server, inviter, `{"kind":"federation","label":"Marc"}`)
	if _, err := server.RedeemInvitation(created.Code, InvitationAccount, "a"); !errors.Is(err, ErrInvitationKind) {
		t.Fatalf("autre sorte : %v", err)
	}
	if _, err := server.RedeemInvitation(created.Code, InvitationFederation, "a"); err != nil {
		t.Fatalf("la bonne sorte, ensuite : %v", err)
	}
}

// LE JETON D'UN COMPTE : son titulaire crée l'invitation quand le jeton est
// actif, l'échange rend CE jeton ; éteint entre-temps, l'invitation attend.
func TestAccountInvitationGivesTheAccountToken(t *testing.T) {
	server := invitationServer(t)
	if code, _ := createInvitation(t, server, invitee, `{"kind":"account"}`); code != http.StatusForbidden {
		t.Fatalf("jeton désactivé : %d, attendu 403", code)
	}
	if code, _ := createInvitation(t, server, invitee, `{"kind":"federation","label":"x"}`); code != http.StatusForbidden {
		t.Fatalf("un joueur crée une invitation de fédération : %d", code)
	}
	federated := invitee
	federated.Federated = true
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"1w"}`)
	if code, _ := createInvitation(t, server, federated, `{"kind":"account"}`); code != http.StatusForbidden {
		t.Fatalf("une session ouverte par jeton crée une invitation : %d", code)
	}

	_, created := createInvitation(t, server, invitee, `{"kind":"account"}`)
	if created.Label != "Marc" {
		t.Fatalf("pour qui : %q", created.Label)
	}
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":false}`)
	if _, err := server.RedeemInvitation(created.Code, InvitationAccount, "a"); !errors.Is(err, ErrInvitationTokenOff) {
		t.Fatalf("jeton éteint : %v", err)
	}

	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"1d"}`)
	redeemed, err := server.RedeemInvitation(created.Code, InvitationAccount, "a")
	if err != nil {
		t.Fatal(err)
	}
	if redeemed.Token != tokenOf(t, server.db, 2) || redeemed.Identity != "marc" {
		t.Fatalf("échange : %+v", redeemed)
	}
}

// DEUX ÉCHANGES SIMULTANÉS, UN SEUL GAGNE.
func TestConcurrentRedeemsHaveOneWinner(t *testing.T) {
	server := invitationServer(t)
	_, created := createInvitation(t, server, inviter, `{"kind":"federation","label":"Marc"}`)

	var group sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := server.RedeemInvitation(created.Code, InvitationFederation, "a")
			results <- err
		}()
	}
	group.Wait()
	close(results)
	won := 0
	for err := range results {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrInvitationRefused):
			t.Fatalf("échec inattendu : %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d gagnants, attendu 1", won)
	}
}

// LES ÉCHECS SONT FREINÉS PAR ADRESSE, comme la connexion : la sixième
// tentative attend, une autre adresse non.
func TestFailedRedeemsAreThrottled(t *testing.T) {
	server := invitationServer(t)
	for range loginFreeTries {
		if _, err := server.RedeemInvitation("pas-un-code", InvitationFederation, "10.0.0.9"); !errors.Is(err, ErrInvitationRefused) {
			t.Fatalf("code inconnu : %v", err)
		}
	}
	var throttled ThrottledError
	if _, err := server.RedeemInvitation("pas-un-code", InvitationFederation, "10.0.0.9"); !errors.As(err, &throttled) {
		t.Fatalf("après %d échecs : %v, attendu un frein", loginFreeTries, err)
	}
	if _, err := server.RedeemInvitation("pas-un-code", InvitationFederation, "10.0.0.10"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatalf("une autre adresse : %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"code":"x","kind":"federation","address":"10.0.0.9"}`))
	request.Header.Set("X-Internal-Secret", testInternalSecret)
	server.handleRedeemInvitation(recorder, request)
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("route interne : %d, Retry-After %q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
}
