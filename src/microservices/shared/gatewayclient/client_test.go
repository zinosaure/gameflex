package gatewayclient

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeGateway — une passerelle qui compte les inscriptions, et qu'on peut
// arrêter puis relancer sur le même port, comme un redémarrage.
type fakeGateway struct {
	t             *testing.T
	address       string
	registrations chan struct{}

	mu     sync.Mutex
	server *http.Server
	conns  []*websocket.Conn
}

func (gateway *fakeGateway) start() {
	listener, err := net.Listen("tcp", gateway.address)
	if err != nil {
		gateway.t.Fatalf("écoute sur %s : %v", gateway.address, err)
	}
	gateway.address = listener.Addr().String()
	upgrader := websocket.Upgrader{}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		gateway.mu.Lock()
		gateway.conns = append(gateway.conns, conn)
		gateway.mu.Unlock()
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		gateway.registrations <- struct{}{}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})}
	gateway.mu.Lock()
	gateway.server = server
	gateway.mu.Unlock()
	go server.Serve(listener)
}

// stop — l'écoute ET les connexions établies : http.Server.Close ne ferme pas
// les connexions détournées par le WebSocket.
func (gateway *fakeGateway) stop() {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	_ = gateway.server.Close()
	for _, conn := range gateway.conns {
		_ = conn.Close()
	}
	gateway.conns = nil
}

// La passerelle redémarre, et la première tentative de reconnexion tombe
// pendant qu'elle est encore absente : le client doit réessayer de lui-même.
// Avant la correction, il attendait pour toujours un signal qui ne venait plus,
// et la partie n'avait plus d'image (2026-10-25).
func TestTheClientRetriesAfterAFailedReconnect(t *testing.T) {
	gateway := &fakeGateway{t: t, address: "127.0.0.1:0", registrations: make(chan struct{}, 8)}
	gateway.start()
	defer gateway.stop()

	client, err := Dial("http://"+gateway.address, "partie", "test", "", MediaInfo{}, Handlers{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	<-gateway.registrations

	// Absente 1,5 s : la tentative d'après une seconde échoue.
	gateway.stop()
	time.Sleep(1500 * time.Millisecond)
	gateway.start()

	select {
	case <-gateway.registrations:
	case <-time.After(8 * time.Second):
		t.Fatal("aucune réinscription après une tentative de reconnexion ratée")
	}
}

// LA PASSERELLE DEMANDE UNE IMAGE CLÉ (chantier 31 P0) : le child l'apprend par
// OnKeyframe, avec la raison.
func TestTheGatewayAsksForAKeyframe(t *testing.T) {
	gateway := &fakeGateway{t: t, address: "127.0.0.1:0", registrations: make(chan struct{}, 8)}
	gateway.start()
	defer gateway.stop()

	reasons := make(chan string, 2)
	orders := make(chan string, 2)
	client, err := Dial("http://"+gateway.address, "partie", "test", "", MediaInfo{},
		Handlers{
			OnKeyframe: func(reason string, index int) { reasons <- fmt.Sprintf("%s/%d", reason, index) },
			OnTier:     func(index int, on bool) { orders <- fmt.Sprintf("%d/%v", index, on) },
		})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	<-gateway.registrations

	gateway.mu.Lock()
	conn := gateway.conns[len(gateway.conns)-1]
	gateway.mu.Unlock()
	// Sans indice : le flux de l'hôte. Puis le second palier (chantier 58),
	// un palier qui n'existe pas (ignoré), et l'ordre d'encoder le premier.
	for _, message := range []map[string]any{
		{"type": "keyframe", "reason": "browser"},
		{"type": "keyframe", "reason": "switch", "index": 2},
		{"type": "tier", "index": 3, "on": true},
		{"type": "tier", "index": 1, "on": true},
	} {
		if err := conn.WriteJSON(message); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"browser/0", "switch/2"} {
		select {
		case reason := <-reasons:
			if reason != want {
				t.Fatalf("raison %q, attendu %q", reason, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("OnKeyframe n'a jamais été appelé")
		}
	}
	select {
	case order := <-orders:
		if order != "1/true" {
			t.Fatalf("ordre %q, attendu 1/true (le palier 3 n'existe pas)", order)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnTier n'a jamais été appelé")
	}
}
