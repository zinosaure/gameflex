package session

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"gameflex/auth"
)

const intentSecret = "secret-de-test"

func signedToken(t *testing.T, username string) string {
	t.Helper()
	private, _, err := auth.KeyPair(intentSecret)
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.Sign(private, auth.Claims{Username: username, ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}

	return token
}

// intentKey — la clé publique que la partie reçoit (GAMEFLEX_JWT_PUBLIC_KEY).
func intentKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	_, public, err := auth.KeyPair(intentSecret)
	if err != nil {
		t.Fatal(err)
	}

	return public
}

// fakeKernel answers the one question the child asks before seating anybody
// (P1): elsewhere maps a username to the session they already drive.
func fakeKernel(t *testing.T, elsewhere map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/internal/sessions/s1/elsewhere/") {
			w.WriteHeader(http.StatusOK) // le miroir, et le reste

			return
		}
		username := strings.TrimPrefix(r.URL.Path, "/internal/sessions/s1/elsewhere/")
		if session, playing := elsewhere[username]; playing {
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": session})

			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	return server
}

// childUnderTest — a real HTTPHost behind a real server. ended receives the
// reason each time the child ends its game.
func childUnderTest(t *testing.T, kernelURL string) (*httptest.Server, chan string) {
	t.Helper()
	ended := make(chan string, 4)
	host := NewHTTPHost(Config{
		SessionID: "s1", PlatformID: "retrogaming", Owner: "gino", MaxPlayers: 2,
		JWTPublicKey: intentKey(t), KernelURL: kernelURL, InternalSecret: "interne",
		BeatTimeout: time.Hour, AFKAfter: time.Hour,
		EndSession: func(reason string) { ended <- reason },
	})
	mux := http.NewServeMux()
	host.Routes(mux, "/v4")
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, ended
}

type tab struct {
	t    *testing.T
	conn *websocket.Conn
}

func openTab(t *testing.T, server *httptest.Server, username string) *tab {
	t.Helper()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/v4/ws?token=" + signedToken(t, username)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("tab %s: %v", username, err)
	}
	t.Cleanup(func() { conn.Close() })

	return &tab{t: t, conn: conn}
}

func (tab *tab) intent(action, target string) {
	tab.t.Helper()
	if err := tab.conn.WriteJSON(map[string]string{"type": "intent", "action": action, "target": target}); err != nil {
		tab.t.Fatalf("intent %s: %v", action, err)
	}
}

// next reads until a message of that type arrives.
func (tab *tab) next(kind string) map[string]any {
	tab.t.Helper()
	_ = tab.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var message map[string]any
		if err := tab.conn.ReadJSON(&message); err != nil {
			tab.t.Fatalf("waiting for %q: %v", kind, err)
		}
		if message["type"] == kind {
			return message
		}
	}
}

// L'INTENTION VOYAGE SUR LE SOCKET (P3) : rien à nommer, l'onglet qui envoie
// est celui pour qui elle parle — et la réponse revient sur ce même socket.
func TestAnIntentRidesTheTabsOwnSocket(t *testing.T) {
	server, _ := childUnderTest(t, "")
	gino := openTab(t, server, "gino")
	gino.next("welcome")

	gino.intent("join", "")
	if result := gino.next("result"); result["status"] != "granted" || result["action"] != "join" {
		t.Fatalf("result = %+v, want a granted join", result)
	}
}

// L'ACCORD DU KERNEL (P1) : on ne fait pas asseoir quelqu'un qui conduit déjà
// une autre partie — et un kernel qui se tait ne bloque personne.
func TestTheKernelIsAskedBeforeSeatingAnybody(t *testing.T) {
	kernel := fakeKernel(t, map[string]string{"dave": "autre-partie", "gino": "s1"})
	server, _ := childUnderTest(t, kernel.URL)

	dave := openTab(t, server, "dave")
	dave.next("welcome")
	dave.intent("join", "")
	if result := dave.next("result"); result["status"] != "refused" || result["reason"] != ErrPlaysElsewhere.Error() {
		t.Fatalf("join from a player elsewhere = %+v, want refused (%s)", result, ErrPlaysElsewhere)
	}

	// Déjà joueur ICI selon le kernel : ce n'est pas « ailleurs ».
	gino := openTab(t, server, "gino")
	gino.next("welcome")
	gino.intent("join", "")
	if result := gino.next("result"); result["status"] != "granted" {
		t.Fatalf("join from a player of this very session = %+v, want granted", result)
	}
}

func TestASilentKernelDoesNotStopAnybodyFromPlaying(t *testing.T) {
	server, _ := childUnderTest(t, "http://127.0.0.1:1") // personne n'écoute
	dave := openTab(t, server, "dave")
	dave.next("welcome")
	dave.intent("join", "")
	if result := dave.next("result"); result["status"] != "granted" {
		t.Fatalf("join with the kernel down = %+v, want granted", result)
	}
}

// ARRÊTER PAR LE CHILD (P2) : le kernel transmet le nom qu'il a vérifié, le
// child juge sur sa file — et la partie s'arrête TOUT DE SUITE, pas au
// balayage suivant.
func TestTheDashboardStopIsJudgedByTheChild(t *testing.T) {
	kernel := fakeKernel(t, nil)
	server, ended := childUnderTest(t, kernel.URL)

	terminate := func(username, secret string) int {
		request, _ := http.NewRequest(http.MethodPost, server.URL+"/v4/terminate", strings.NewReader(`{"username":"`+username+`"}`))
		request.Header.Set("X-Internal-Secret", secret)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("terminate: %v", err)
		}
		response.Body.Close()

		return response.StatusCode
	}

	if code := terminate("gino", "faux"); code != http.StatusForbidden {
		t.Fatalf("terminate without the kernel's secret = %d, want 403", code)
	}
	if code := terminate("dave", "interne"); code != http.StatusForbidden {
		t.Fatalf("terminate by a guest = %d, want 403", code)
	}
	if code := terminate("gino", "interne"); code != http.StatusOK {
		t.Fatalf("terminate by the owner = %d, want 200", code)
	}
	select {
	case reason := <-ended:
		if reason != "owner ended the session" {
			t.Fatalf("ended because %q", reason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the game did not end right away")
	}
}

// Un `terminate` de l'onglet du propriétaire arrête lui aussi la partie sur
// le champ — une seule fois.
func TestTheOwnersTerminateEndsTheGameOnce(t *testing.T) {
	server, ended := childUnderTest(t, "")
	gino := openTab(t, server, "gino")
	gino.next("welcome")

	gino.intent("terminate", "")
	gino.intent("terminate", "")
	select {
	case <-ended:
	case <-time.After(3 * time.Second):
		t.Fatal("the owner's terminate did not end the game")
	}
	select {
	case reason := <-ended:
		t.Fatalf("the game ended twice (%s)", reason)
	case <-time.After(500 * time.Millisecond):
	}
}

// LE PRÊT VA JUSQU'AUX TOUCHES. Chaque runtime pose au paquet partagé deux
// questions : quel pad atteignent les touches de ce viewer, et à quel viewer
// envoyer la vibration de ce pad. Un prêt doit changer ces deux réponses — et
// une reprise les rendre au propriétaire.
func TestTheKeysAndTheRumbleFollowTheLoan(t *testing.T) {
	host := NewHTTPHost(Config{
		SessionID: "s1", PlatformID: "retrogaming", Owner: "gino", MaxPlayers: 2,
		JWTPublicKey: intentKey(t), BeatTimeout: time.Hour, AFKAfter: time.Hour,
	})
	mux := http.NewServeMux()
	host.Routes(mux, "/v4")
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	announce := func(tab *tab, viewerID string) {
		t.Helper()
		if err := tab.conn.WriteJSON(map[string]string{"type": "viewer", "viewerId": viewerID}); err != nil {
			t.Fatalf("announce %s: %v", viewerID, err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			if _, managed := host.PortForViewer(viewerID); managed {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("viewer %s never paired with its tab", viewerID)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	expect := func(when, viewerID string, want int) {
		t.Helper()
		if slot, _ := host.PortForViewer(viewerID); slot != want {
			t.Fatalf("%s: %s drives pad %d, want %d", when, viewerID, slot, want)
		}
	}

	gino := openTab(t, server, "gino")
	gino.next("welcome")
	announce(gino, "viewer-gino")
	gino.intent("join", "")
	if result := gino.next("result"); result["status"] != "granted" {
		t.Fatalf("join = %+v", result)
	}

	gixi := openTab(t, server, "gixi")
	gixi.next("welcome")
	announce(gixi, "viewer-gixi")
	expect("before the loan", "viewer-gino", 1)
	expect("before the loan", "viewer-gixi", 0)

	gino.intent("give-handover", "gixi")
	if result := gino.next("result"); result["status"] != "granted" {
		t.Fatalf("lend = %+v", result)
	}
	expect("during the loan", "viewer-gixi", 1)
	expect("during the loan", "viewer-gino", 0)
	if viewer := host.ViewerForSlot(1); viewer != "viewer-gixi" {
		t.Fatalf("pad 1 rumbles for %q during the loan, want the borrower", viewer)
	}

	gino.intent("reclaim", "")
	if result := gino.next("result"); result["status"] != "granted" {
		t.Fatalf("reclaim = %+v", result)
	}
	expect("after taking it back", "viewer-gino", 1)
	expect("after taking it back", "viewer-gixi", 0)
	if viewer := host.ViewerForSlot(1); viewer != "viewer-gino" {
		t.Fatalf("pad 1 rumbles for %q after the reclaim, want the owner", viewer)
	}
}

// LE JEU S'ARRÊTE DE LUI-MÊME (#504) : son émulateur est sorti, personne n'a
// mis fin à la partie. Chaque onglet l'apprend — la fin, et pourquoi — avant
// qu'elle ne se termine, et le journal de la partie le dit à son tour.
func TestAGameThatStopsByItselfEndsTheSessionAndSaysWhy(t *testing.T) {
	ended := make(chan string, 4)
	host := NewHTTPHost(Config{
		SessionID: "s1", PlatformID: "streamex", Owner: "gino", MaxPlayers: 2,
		JWTPublicKey: intentKey(t), BeatTimeout: time.Hour, AFKAfter: time.Hour,
		EndSession: func(reason string) { ended <- reason },
	})
	mux := http.NewServeMux()
	host.Routes(mux, "/v4")
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	gino := openTab(t, server, "gino")
	gino.next("welcome")

	host.EndBecauseGameExited()

	for {
		state := gino.next("state")
		if state["terminated"] != true {
			continue
		}
		if state["gameExited"] != true {
			t.Fatalf("state %v : terminated sans gameExited — la page ne saurait pas que le jeu s'est arrêté", state)
		}

		break
	}
	select {
	case reason := <-ended:
		if !strings.Contains(reason, "emulator exited") {
			t.Fatalf("raison %q, attendu « the game stopped by itself (its emulator exited) »", reason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("la partie ne s'est pas terminée")
	}
}
