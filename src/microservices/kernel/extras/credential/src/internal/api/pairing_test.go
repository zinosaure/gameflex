package api

// LE CODE D'APPAIRAGE (chantier 60), sur une horloge qu'on fait avancer : les
// deux modes, les codes, les deux compteurs, la trame.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/database"
)

// fakeClock — une horloge qui n'avance que quand le test le dit ; ses
// minuteries partent dans Advance, dans l'ordre de leur échéance.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	at      time.Time
	run     func()
	stopped bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 17, 14, 0, 0, 0, time.UTC)}
}

func (clock *fakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	return clock.now
}

func (clock *fakeClock) AfterFunc(wait time.Duration, run func()) func() bool {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	timer := &fakeTimer{at: clock.now.Add(wait), run: run}
	clock.timers = append(clock.timers, timer)

	return func() bool {
		clock.mu.Lock()
		defer clock.mu.Unlock()
		alive := !timer.stopped
		timer.stopped = true

		return alive
	}
}

func (clock *fakeClock) Advance(wait time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(wait)
	due := []*fakeTimer{}
	for _, timer := range clock.timers {
		if !timer.stopped && !timer.at.After(clock.now) {
			timer.stopped = true
			due = append(due, timer)
		}
	}
	clock.mu.Unlock()
	sort.Slice(due, func(left, right int) bool { return due[left].at.Before(due[right].at) })
	for _, timer := range due {
		timer.run()
	}
}

// pairingServer — un service à l'horloge injectée, un administrateur (1) et un
// joueur (2), le jeton du joueur actif ; plus un second joueur (3), actif aussi.
func pairingServer(t *testing.T) (*Server, *fakeClock) {
	t.Helper()
	server := invitationServer(t)
	clock := newFakeClock()
	server.pairing = newPairing(clock)
	if _, err := server.db.Exec(`INSERT INTO users (username, hashed_password, display_name, role_level) VALUES ('lea', 'x', 'Léa', ?)`, auth.RoleUser); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1", "2", "3"} {
		adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, id, `{"enabled":true,"duration":"1w"}`)
	}

	return server, clock
}

var thirdMember = auth.Principal{ID: 3, Username: "lea", DisplayName: "Léa", RoleLevel: auth.RoleUser}

// as — un gestionnaire, au nom de `principal`, avec son corps et son {id}.
func as(handler func(http.ResponseWriter, *http.Request, auth.Principal), principal auth.Principal, body, id string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	request.SetPathValue("id", id)
	handler(recorder, request, principal)

	return recorder
}

// newCode — un code créé, ou l'échec du test.
func newCode(t *testing.T, server *Server, principal auth.Principal, body string) invitationResponse {
	t.Helper()
	recorder := as(server.handleCreatePairingCode, principal, body, "")
	if recorder.Code != http.StatusCreated {
		t.Fatalf("création d'un code : %d %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Item invitationResponse `json:"item"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	if !pairingCodeForm.MatchString(envelope.Item.Code) {
		t.Fatalf("le code n'a pas six chiffres : %q", envelope.Item.Code)
	}

	return envelope.Item
}

// frame — la trame `pairing` d'aujourd'hui, décodée.
type decodedFrame struct {
	Type   string `json:"type"`
	Server *struct {
		OpenUntil string `json:"open_until"`
		OpenedBy  string `json:"opened_by"`
	} `json:"server"`
	Accounts []struct {
		Username  string `json:"username"`
		OpenUntil string `json:"open_until"`
	} `json:"accounts"`
	Codes []struct {
		ID        int64  `json:"id"`
		Kind      string `json:"kind"`
		Author    string `json:"author"`
		ExpiresAt string `json:"expires_at"`
	} `json:"codes"`
	Ended  []pairingEnded  `json:"ended"`
	Closed []pairingClosed `json:"closed"`
}

func frameOf(t *testing.T, server *Server) decodedFrame {
	t.Helper()
	var frame decodedFrame
	if err := json.Unmarshal(server.PairingFrame(), &frame); err != nil {
		t.Fatal(err)
	}

	return frame
}

func endedReason(frame decodedFrame, id int64) string {
	for _, entry := range frame.Ended {
		if entry.ID == id {
			return entry.Reason
		}
	}

	return ""
}

// LES MODES : le serveur à l'administrateur, le compte à chacun pour le sien,
// et seulement quand son jeton est actif.
func TestPairingModesBelongToTheirOwners(t *testing.T) {
	server, clock := pairingServer(t)

	if recorder := as(server.withAdmin(server.handleOpenServerPairing), invitee, "", ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("un membre ouvre le mode serveur : %d", recorder.Code)
	}
	recorder := as(server.handleOpenAccountPairing, invitee, "", "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), pairingStamp(clock.Now().Add(5*time.Minute))) {
		t.Fatalf("un membre ouvre son mode compte : %d %s", recorder.Code, recorder.Body.String())
	}
	frame := frameOf(t, server)
	if frame.Server != nil || len(frame.Accounts) != 1 || frame.Accounts[0].Username != "marc" {
		t.Fatalf("trame : %+v", frame)
	}

	federated := thirdMember
	federated.Federated = true
	if recorder := as(server.handleOpenAccountPairing, federated, "", ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("une session ouverte par jeton ouvre un mode : %d", recorder.Code)
	}
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "3", `{"enabled":false}`)
	if recorder := as(server.handleOpenAccountPairing, thirdMember, "", ""); recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "token_not_enabled") {
		t.Fatalf("jeton éteint : %d %s", recorder.Code, recorder.Body.String())
	}

	// Fermer le sien ne ferme pas celui d'un autre.
	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	as(server.handleCloseAccountPairing, inviter, "", "")
	if frame := frameOf(t, server); frame.Server == nil || len(frame.Accounts) != 1 {
		t.Fatalf("l'administrateur ferme le mode compte d'un membre : %+v", frame)
	}
}

// PROLONGER REPART DE CINQ MINUTES, ET LES CODES GARDENT LEUR ÉCHÉANCE ; À CINQ
// MINUTES, LE MODE S'ÉTEINT ET SES CODES PARTENT.
func TestPairingModeRenewsThenTimesOut(t *testing.T) {
	server, clock := pairingServer(t)
	as(server.handleOpenAccountPairing, invitee, "", "")
	clock.Advance(4 * time.Minute)
	code := newCode(t, server, invitee, `{"kind":"account"}`)
	if code.ExpiresAt != pairingStamp(clock.Now().Add(time.Minute)) {
		t.Fatalf("un code survit à son mode : %s", code.ExpiresAt)
	}

	as(server.handleOpenAccountPairing, invitee, "", "")
	frame := frameOf(t, server)
	if frame.Accounts[0].OpenUntil != pairingStamp(clock.Now().Add(5*time.Minute)) || frame.Codes[0].ExpiresAt != code.ExpiresAt {
		t.Fatalf("prolongation : %+v", frame)
	}

	clock.Advance(time.Minute)
	if frame := frameOf(t, server); len(frame.Codes) != 0 || endedReason(frame, code.ID) != pairingExpired || len(frame.Accounts) != 1 {
		t.Fatalf("le code à son échéance : %+v", frame)
	}
	if _, err := server.RedeemInvitation(code.Code, InvitationAccount, "Gamebox", "a"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatalf("code échu : %v", err)
	}

	second := newCode(t, server, invitee, `{"kind":"account"}`)
	clock.Advance(4 * time.Minute)
	frame = frameOf(t, server)
	if len(frame.Accounts) != 0 || len(frame.Closed) != 1 || frame.Closed[0].Reason != pairingTimeout || endedReason(frame, second.ID) != pairingExpired {
		t.Fatalf("le mode à cinq minutes : %+v", frame)
	}
	var rows int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM invitations WHERE pairing = 1`).Scan(&rows)
	if rows != 0 {
		t.Fatalf("%d codes restent en base", rows)
	}
}

// L'INTERRUPTEUR FERME, ET LE REDÉMARRAGE AUSSI : la base efface les codes.
func TestPairingClosesBySwitchAndAtRestart(t *testing.T) {
	server, _ := pairingServer(t)
	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	code := newCode(t, server, inviter, `{"kind":"federation","label":"Marc","scopes":["catalog"]}`)
	if recorder := as(server.withAdmin(server.handleCloseServerPairing), inviter, "", ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("fermer : %d", recorder.Code)
	}
	frame := frameOf(t, server)
	if frame.Server != nil || endedReason(frame, code.ID) != pairingCancelled || frame.Closed[0].Reason != pairingOwner {
		t.Fatalf("fermé par l'interrupteur : %+v", frame)
	}
	if _, err := server.RedeemInvitation(code.Code, InvitationFederation, "Gamebox", "a"); !errors.Is(err, ErrPairingClosed) {
		t.Fatalf("après la fermeture : %v", err)
	}

	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	newCode(t, server, inviter, `{"kind":"federation","label":"Marc"}`)
	server.db.Close()
	db, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var rows int
	_ = db.QueryRow(`SELECT COUNT(*) FROM invitations WHERE pairing = 1`).Scan(&rows)
	if rows != 0 {
		t.Fatalf("%d codes restent après un redémarrage", rows)
	}
}

// LES CODES : chacun sous son mode, et le mode de l'un n'ouvre pas l'autre.
func TestPairingCodesNeedTheModeOfTheirKind(t *testing.T) {
	server, _ := pairingServer(t)
	if recorder := as(server.handleCreatePairingCode, inviter, `{"kind":"federation","label":"Marc"}`, ""); recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "pairingClosed") {
		t.Fatalf("code federation sans mode : %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder := as(server.handleCreatePairingCode, invitee, `{"kind":"account"}`, ""); recorder.Code != http.StatusConflict {
		t.Fatalf("code account sans mode : %d", recorder.Code)
	}

	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	if recorder := as(server.handleCreatePairingCode, inviter, `{"kind":"account"}`, ""); recorder.Code != http.StatusConflict {
		t.Fatalf("le mode serveur ouvre les codes de compte : %d", recorder.Code)
	}
	if recorder := as(server.handleCreatePairingCode, invitee, `{"kind":"federation","label":"x"}`, ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("un membre crée un code federation : %d", recorder.Code)
	}
	as(server.handleOpenAccountPairing, invitee, "", "")
	code := newCode(t, server, invitee, `{"kind":"account"}`)
	as(server.withAdmin(server.handleCloseServerPairing), inviter, "", "")
	if _, err := server.RedeemInvitation(code.Code, InvitationFederation, "Gamebox", "a"); !errors.Is(err, ErrPairingClosed) {
		t.Fatalf("le mode compte ouvre les codes federation : %v", err)
	}
}

// UN CODE SERT UNE FOIS : le jeton de catalogue sous son nom et ses portées,
// `ended` dit à qui, puis plus rien. La liste et la révocation ne le voient pas.
func TestPairingCodeIsRedeemedOnce(t *testing.T) {
	server, _ := pairingServer(t)
	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	code := newCode(t, server, inviter, `{"kind":"federation","label":"Marc","scopes":["catalog","devices"]}`)
	_, link := createInvitation(t, server, inviter, `{"kind":"federation","label":"Lien"}`)

	list := adminCall(server, server.handleListInvitations, http.MethodGet, "", "")
	if strings.Contains(list.Body.String(), `"Marc"`) || !strings.Contains(list.Body.String(), "Lien") {
		t.Fatalf("la liste montre le code : %s", list.Body.String())
	}
	if recorder := adminCall(server, server.handleRevokeInvitation, http.MethodDelete, fmt.Sprint(code.ID), ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("la révocation voit le code : %d", recorder.Code)
	}

	redeemed, err := server.RedeemInvitation(code.Code, InvitationFederation, "Gamebox", "192.168.1.20")
	if err != nil || redeemed.Identity != "Marc" || redeemed.Scopes != "catalog,devices" {
		t.Fatalf("échange : %+v %v", redeemed, err)
	}
	if recorder := internalCall(server, server.handleVerifyFederationToken, redeemed.Token); recorder.Code != http.StatusOK {
		t.Fatalf("le jeton né du code est refusé : %d", recorder.Code)
	}
	frame := frameOf(t, server)
	if len(frame.Ended) != 1 || frame.Ended[0].Reason != pairingUsed || frame.Ended[0].By != "Gamebox" || frame.Ended[0].From != "192.168.1.20" {
		t.Fatalf("ended : %+v", frame.Ended)
	}
	if strings.Contains(string(server.PairingFrame()), code.Code) {
		t.Fatal("la trame porte la valeur du code")
	}
	if _, err := server.RedeemInvitation(code.Code, InvitationFederation, "Gamebox", "192.168.1.20"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatalf("second échange : %v", err)
	}

	// L'invitation en lien suit son chemin, inchangée.
	if _, err := server.RedeemInvitation(link.Code, InvitationFederation, "", "a"); err != nil {
		t.Fatalf("le lien de 22 caractères : %v", err)
	}
}

// LE CODE D'UN COMPTE REND SON JETON ; LE JETON ÉTEINT, IL ATTEND SANS RIEN
// COMPTER ; UN SECOND CODE DU MÊME AUTEUR TUE LE PREMIER ; L'AUTEUR L'ANNULE.
func TestAccountPairingCode(t *testing.T) {
	server, _ := pairingServer(t)
	as(server.handleOpenAccountPairing, invitee, "", "")
	first := newCode(t, server, invitee, `{"kind":"account"}`)
	second := newCode(t, server, invitee, `{"kind":"account"}`)
	if reason := endedReason(frameOf(t, server), first.ID); reason != pairingCancelled {
		t.Fatalf("le premier code : %q", reason)
	}
	if _, err := server.RedeemInvitation(first.Code, InvitationAccount, "Gamebox", "a"); !errors.Is(err, ErrInvitationRefused) && first.Code != second.Code {
		t.Fatalf("le premier code vaut encore : %v", err)
	}

	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":false}`)
	for range 6 {
		if _, err := server.RedeemInvitation(second.Code, InvitationAccount, "Gamebox", "a"); !errors.Is(err, ErrInvitationTokenOff) {
			t.Fatalf("jeton éteint : %v", err)
		}
	}
	adminCall(server, server.handleUpdateAccountToken, http.MethodPatch, "2", `{"enabled":true,"duration":"1d"}`)
	redeemed, err := server.RedeemInvitation(second.Code, InvitationAccount, "Gamebox", "a")
	if err != nil || redeemed.Token != tokenOf(t, server.db, 2) || redeemed.Identity != "marc" {
		t.Fatalf("échange : %+v %v", redeemed, err)
	}

	third := newCode(t, server, invitee, `{"kind":"account"}`)
	if recorder := as(server.handleCancelPairingCode, thirdMember, "", fmt.Sprint(third.ID)); recorder.Code != http.StatusNotFound {
		t.Fatalf("un autre annule le code : %d", recorder.Code)
	}
	if recorder := as(server.handleCancelPairingCode, invitee, "", fmt.Sprint(third.ID)); recorder.Code != http.StatusNoContent {
		t.Fatalf("l'auteur annule : %d", recorder.Code)
	}
	if _, err := server.RedeemInvitation(third.Code, InvitationAccount, "Gamebox", "a"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatalf("code annulé : %v", err)
	}
}

// HUIT ÉCHANGES SIMULTANÉS, UN SEUL GAGNANT.
func TestConcurrentPairingRedeemsHaveOneWinner(t *testing.T) {
	server, _ := pairingServer(t)
	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	code := newCode(t, server, inviter, `{"kind":"federation","label":"Marc"}`)

	var group sync.WaitGroup
	results := make(chan error, 8)
	for index := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := server.RedeemInvitation(code.Code, InvitationFederation, "Gamebox", fmt.Sprintf("10.0.0.%d", index))
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
		case !errors.Is(err, ErrInvitationRefused) && !errors.Is(err, ErrPairingClosed):
			t.Fatalf("échec inattendu : %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d gagnants, attendu 1", won)
	}
}

// wrongCode — un code de six chiffres qui n'est pas `except`.
func wrongCode(except ...string) string {
	for value := 0; ; value++ {
		candidate := fmt.Sprintf("%06d", value)
		taken := false
		for _, code := range except {
			taken = taken || code == candidate
		}
		if !taken {
			return candidate
		}
	}
}

// CINQ FAUX `federation`, DEPUIS CINQ ADRESSES : le mode serveur se ferme et
// ses codes partent ; les modes compte n'en savent rien. Prolonger ne rend
// aucun essai.
func TestFiveWrongFederationCodesCloseTheServerMode(t *testing.T) {
	server, _ := pairingServer(t)
	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	as(server.handleOpenAccountPairing, invitee, "", "")
	code := newCode(t, server, inviter, `{"kind":"federation","label":"Marc"}`)
	accountCode := newCode(t, server, invitee, `{"kind":"account"}`)
	wrong := wrongCode(code.Code, accountCode.Code)

	for index := range 4 {
		if _, err := server.RedeemInvitation(wrong, InvitationFederation, "x", fmt.Sprintf("10.0.1.%d", index)); !errors.Is(err, ErrInvitationRefused) {
			t.Fatalf("faux %d : %v", index, err)
		}
	}
	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	if _, err := server.RedeemInvitation(wrong, InvitationFederation, "x", "10.0.1.9"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatalf("cinquième faux : %v", err)
	}

	frame := frameOf(t, server)
	if frame.Server != nil || endedReason(frame, code.ID) != pairingTooManyFailures || len(frame.Accounts) != 1 || len(frame.Codes) != 1 {
		t.Fatalf("après cinq faux : %+v", frame)
	}
	if _, err := server.RedeemInvitation(code.Code, InvitationFederation, "x", "a"); !errors.Is(err, ErrPairingClosed) {
		t.Fatalf("le bon code, après : %v", err)
	}

	// Rouvert, son compteur repart de zéro.
	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	if _, err := server.RedeemInvitation(wrong, InvitationFederation, "x", "10.0.2.1"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatal(err)
	}
	if frameOf(t, server).Server == nil {
		t.Fatal("le compteur n'est pas reparti de zéro")
	}
}

// CINQ FAUX `account` FERMENT TOUS LES MODES COMPTE, LE MODE SERVEUR RESTE ;
// UN CODE DE LA MAUVAISE SORTE COMPTE UN FAUX À SA SORTE, SANS RIEN CONSOMMER.
func TestFiveWrongAccountCodesCloseEveryAccountMode(t *testing.T) {
	server, _ := pairingServer(t)
	as(server.withAdmin(server.handleOpenServerPairing), inviter, "", "")
	as(server.handleOpenAccountPairing, invitee, "", "")
	as(server.handleOpenAccountPairing, thirdMember, "", "")
	federationCode := newCode(t, server, inviter, `{"kind":"federation","label":"Marc"}`)
	accountCode := newCode(t, server, invitee, `{"kind":"account"}`)

	// La mauvaise sorte : un faux au compteur `account`, le code federation reste.
	if _, err := server.RedeemInvitation(federationCode.Code, InvitationAccount, "x", "10.0.3.0"); !errors.Is(err, ErrInvitationRefused) {
		t.Fatalf("mauvaise sorte : %v", err)
	}
	wrong := wrongCode(federationCode.Code, accountCode.Code)
	for index := range 4 {
		if _, err := server.RedeemInvitation(wrong, InvitationAccount, "x", fmt.Sprintf("10.0.3.%d", index+1)); !errors.Is(err, ErrInvitationRefused) {
			t.Fatalf("faux %d : %v", index, err)
		}
	}

	frame := frameOf(t, server)
	if len(frame.Accounts) != 0 || frame.Server == nil || endedReason(frame, accountCode.ID) != pairingTooManyFailures || len(frame.Closed) != 2 {
		t.Fatalf("après cinq faux : %+v", frame)
	}
	if _, err := server.RedeemInvitation(federationCode.Code, InvitationFederation, "Gamebox", "a"); err != nil {
		t.Fatalf("le code federation, après : %v", err)
	}
}

// UN CODE DE SIX CHIFFRES HORS DE TOUT MODE EST REFUSÉ SANS RIEN COMPTER, et
// la route interne le dit en 423.
func TestPairingCodeOutsideAnyModeIsLocked(t *testing.T) {
	server, _ := pairingServer(t)
	for range 10 {
		if _, err := server.RedeemInvitation("123456", InvitationAccount, "x", "10.0.0.1"); !errors.Is(err, ErrPairingClosed) {
			t.Fatalf("hors mode : %v", err)
		}
	}
	as(server.handleOpenAccountPairing, invitee, "", "")
	code := newCode(t, server, invitee, `{"kind":"account"}`)
	if _, err := server.RedeemInvitation(code.Code, InvitationAccount, "x", "10.0.0.1"); err != nil {
		t.Fatalf("le frein a compté les essais hors mode : %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"code":"123456","kind":"federation","from":"x","address":"a"}`))
	request.Header.Set("X-Internal-Secret", testInternalSecret)
	server.handleRedeemInvitation(recorder, request)
	if recorder.Code != http.StatusLocked || !strings.Contains(recorder.Body.String(), "pairingClosed") {
		t.Fatalf("route interne : %d %s", recorder.Code, recorder.Body.String())
	}
}
