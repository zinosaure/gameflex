package facade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakePeer — le bout du pair (ServePeerSessions) derrière un serveur de test : sa liste, de quoi la réveiller, le
// nombre de sockets ouverts et de poignées de main reçues, et le jeton présenté.
type fakePeer struct {
	*httptest.Server
	mu        sync.Mutex
	list      []PeerSession
	wake      chan struct{}
	admitted  atomic.Bool
	open      atomic.Int32
	dials     atomic.Int32
	presented atomic.Value
}

func newFakePeer(t *testing.T, list []PeerSession) *fakePeer {
	t.Helper()
	peer := &fakePeer{list: list, wake: make(chan struct{}, 1)}
	peer.admitted.Store(true)
	peer.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer.dials.Add(1)
		peer.presented.Store(r.Header.Get("Authorization"))
		if r.URL.Path != "/api/v1/federation/sessions" {
			w.WriteHeader(http.StatusNotFound)

			return
		}
		socket, err := Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		peer.open.Add(1)
		defer peer.open.Add(-1)
		ServePeerSessions(socket, peer.wake, func() []PeerSession {
			peer.mu.Lock()
			defer peer.mu.Unlock()

			return append([]PeerSession(nil), peer.list...)
		}, peer.admitted.Load)
	}))
	t.Cleanup(peer.Close)

	return peer
}

func (peer *fakePeer) set(list []PeerSession) {
	peer.mu.Lock()
	peer.list = list
	peer.mu.Unlock()
	peer.wake <- struct{}{}
}

// following — un centre de messages dont le sujet `servers/3/sessions` suit `link`.
func following(t *testing.T, link string) Router {
	t.Helper()

	return Router{Topic: func(conn *Conn, wanted Subscription) (func(context.Context), int, string) {
		if wanted.Topic != "servers/3/sessions" {
			return func(ctx context.Context) { <-ctx.Done() }, 0, ""
		}

		return func(ctx context.Context) { FollowPeerSessions(ctx, conn, wanted.Topic, link, "jeton-catalogue") }, 0, ""
	}}
}

func peerList(t *testing.T, got frame) []PeerSession {
	t.Helper()
	var list []PeerSession
	if err := json.Unmarshal(got.Data, &list); err != nil {
		t.Fatalf("liste illisible : %s", got.Data)
	}

	return list
}

func shortPeerTimings(t *testing.T) {
	t.Helper()
	ping, first, max := peerPingEvery, peerRetryFirst, peerRetryMax
	peerPingEvery, peerRetryFirst, peerRetryMax = 50*time.Millisecond, 20*time.Millisecond, 80*time.Millisecond
	t.Cleanup(func() { peerPingEvery, peerRetryFirst, peerRetryMax = ping, first, max })
}

func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("%s : jamais vrai", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// LE PAIR POUSSE, L'AMI RELAIE (`#747`) : la liste à l'abonnement, puis à chaque changement, avec le jeton de catalogue
// présenté chez le pair ; et le socket du pair se ferme quand la page se désabonne.
func TestPeerSessionsArePushedThroughAndTheSocketClosesWithTheSubscription(t *testing.T) {
	peer := newFakePeer(t, []PeerSession{{ID: "a", Platform: "retrogaming", ConsoleID: "snes", Title: "Aladdin", Owner: "ami", PlayerCount: 1, OpenSlots: 1}})
	connection := serve(t, following(t, peer.URL))

	send(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "servers/3/sessions"}})
	first := peerList(t, next(t, connection, func(got frame) bool { return got.Type == "sessions" }))
	if len(first) != 1 || first[0].ID != "a" || first[0].Title != "Aladdin" {
		t.Fatalf("premier instantané : %+v", first)
	}
	if presented, _ := peer.presented.Load().(string); presented != "Bearer jeton-catalogue" {
		t.Fatalf("jeton présenté au pair : %q", presented)
	}

	peer.set([]PeerSession{})
	if second := peerList(t, next(t, connection, func(got frame) bool { return got.Type == "sessions" })); len(second) != 0 {
		t.Fatalf("la partie finie chez le pair est toujours là : %+v", second)
	}

	send(t, connection, map[string]any{"type": "unsubscribe", "id": "2", "data": map[string]any{"topic": "servers/3/sessions"}})
	eventually(t, "le socket du pair fermé après le désabonnement", func() bool { return peer.open.Load() == 0 })
	if peer.dials.Load() != 1 {
		t.Fatalf("%d poignées de main chez le pair, attendu 1", peer.dials.Load())
	}
}

// UN JETON RÉVOQUÉ FERME LE SOCKET OUVERT, et l'ami n'y retourne pas : la page apprend que la liste est vide.
func TestARevokedTokenClosesThePeerSocketForGood(t *testing.T) {
	shortPeerTimings(t)
	peer := newFakePeer(t, []PeerSession{{ID: "a", Platform: "streamex", ConsoleID: "switch", Title: "Jeu"}})
	connection := serve(t, following(t, peer.URL))

	send(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "servers/3/sessions"}})
	if first := peerList(t, next(t, connection, func(got frame) bool { return got.Type == "sessions" })); len(first) != 1 {
		t.Fatalf("premier instantané : %+v", first)
	}

	peer.admitted.Store(false)
	if after := peerList(t, next(t, connection, func(got frame) bool { return got.Type == "sessions" })); len(after) != 0 {
		t.Fatalf("après la révocation : %+v, attendu une liste vide", after)
	}
	eventually(t, "le socket du pair fermé", func() bool { return peer.open.Load() == 0 })
	time.Sleep(300 * time.Millisecond)
	if peer.dials.Load() != 1 {
		t.Fatalf("%d poignées de main après une révocation, attendu 1", peer.dials.Load())
	}
}

// UN PAIR INJOIGNABLE NE RETIENT RIEN : le centre de messages répond aux autres demandes pendant qu'il réessaie, et
// il réessaie à un pas qui double (pas de boucle serrée) ; un refus à la poignée de main (401) arrête les essais.
func TestAnUnreachablePeerIsRetriedAtABoundedPace(t *testing.T) {
	shortPeerTimings(t)
	var attempts atomic.Int32
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(down.Close)
	connection := serve(t, following(t, down.URL))

	send(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "servers/3/sessions"}})
	send(t, connection, map[string]any{"type": "subscribe", "id": "2", "data": map[string]any{"topic": "autre"}})
	if got := next(t, connection, func(got frame) bool { return got.ID == "2" }); got.Status != http.StatusOK {
		t.Fatalf("une autre demande pendant que le pair est perdu : %+v", got)
	}
	time.Sleep(600 * time.Millisecond)
	// 20, 40, 80, 80… ms : environ huit essais en 600 ms ; une boucle serrée en ferait des centaines.
	if count := attempts.Load(); count < 3 || count > 15 {
		t.Fatalf("%d essais en 600 ms, attendu un pas borné", count)
	}

	var refusals atomic.Int32
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refusals.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(refusing.Close)
	refused := serve(t, following(t, refusing.URL))
	send(t, refused, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "servers/3/sessions"}})
	time.Sleep(300 * time.Millisecond)
	if refusals.Load() != 1 {
		t.Fatalf("%d essais chez un pair qui refuse le jeton, attendu 1", refusals.Load())
	}
}

// L'adresse du socket suit le protocole du lien.
func TestPeerSocketURLFollowsTheLink(t *testing.T) {
	for link, want := range map[string]string{
		"https://ami.example/":      "wss://ami.example/api/v1/federation/sessions",
		"http://192.168.1.13:10200": "ws://192.168.1.13:10200/api/v1/federation/sessions",
	} {
		if got, err := peerSocketURL(link, peerSessionsPath); err != nil || got != want {
			t.Fatalf("%s → %q (%v), attendu %q", link, got, err, want)
		}
	}
	if got, _ := peerSocketURL("https://ami.example", peerDevicesPath); got != "wss://ami.example/api/v1/federation/remoteplay/devices" {
		t.Fatalf("le socket des consoles : %q", got)
	}
	if _, err := peerSocketURL("ftp://ami", peerSessionsPath); err == nil {
		t.Fatal("un lien ftp a été accepté")
	}
}

// LES CONSOLES D'UN PAIR SE RELAIENT COMME SES PARTIES (`#750`) : l'état à l'abonnement, puis la console réveillée dès
// qu'elle se dit prête, sous le type `devices` ; le pair perdu, une liste vide (ses consoles ne se disent plus prêtes).
func TestPeerDevicesArePushedThroughAndALostPeerEmptiesThem(t *testing.T) {
	shortPeerTimings(t)
	var mu sync.Mutex
	list := []PeerDevice{{ID: "ps5", HostID: "H1", HostName: "PS5-896", HostType: "PS5", Status: "standby"}}
	wake := make(chan struct{}, 1)
	sockets := make(chan *websocket.Conn, 4)
	var presented atomic.Value
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented.Store(r.Header.Get("Authorization"))
		if r.URL.Path != "/api/v1/federation/remoteplay/devices" {
			w.WriteHeader(http.StatusNotFound)

			return
		}
		socket, err := Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		sockets <- socket
		ServePeerDevices(socket, wake, func() []PeerDevice {
			mu.Lock()
			defer mu.Unlock()

			return append([]PeerDevice(nil), list...)
		}, func() bool { return true })
	}))
	t.Cleanup(peer.Close)
	connection := serve(t, Router{Topic: func(conn *Conn, wanted Subscription) (func(context.Context), int, string) {
		return func(ctx context.Context) { FollowPeerDevices(ctx, conn, wanted.Topic, peer.URL, "jeton-consoles") }, 0, ""
	}})
	devices := func() []PeerDevice {
		got := next(t, connection, func(got frame) bool { return got.Type == "devices" && got.Topic == "servers/3/devices" })
		var found []PeerDevice
		if err := json.Unmarshal(got.Data, &found); err != nil {
			t.Fatalf("liste illisible : %s", got.Data)
		}

		return found
	}

	send(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "servers/3/devices"}})
	if first := devices(); len(first) != 1 || first[0].ID != "ps5" || first[0].Status != "standby" {
		t.Fatalf("premier instantané : %+v", first)
	}
	if got, _ := presented.Load().(string); got != "Bearer jeton-consoles" {
		t.Fatalf("jeton présenté au pair : %q", got)
	}

	mu.Lock()
	list = []PeerDevice{{ID: "ps5", HostID: "H1", HostName: "PS5-896", HostType: "PS5", Status: "awake"}}
	mu.Unlock()
	wake <- struct{}{}
	if awake := devices(); len(awake) != 1 || awake[0].Status != "awake" {
		t.Fatalf("la console réveillée : %+v", awake)
	}

	// Le pair tombe : son socket se coupe net.
	(<-sockets).Close()
	if lost := devices(); len(lost) != 0 {
		t.Fatalf("le pair perdu : %+v, attendu une liste vide", lost)
	}
}
