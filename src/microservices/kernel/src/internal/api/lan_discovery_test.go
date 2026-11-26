package api

// LE RÉSEAU LOCAL PAR LA PASSERELLE (chantier 60) : l'annonce suit le mode
// serveur et lui seul ; la recherche vit tant qu'une page est abonnée.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/registry"
)

// fakeLANGateway — une passerelle qui note ses annonces, ses recherches et
// leurs questions, et rend un serveur trouvé à chaque recherche.
type fakeLANGateway struct {
	mu        sync.Mutex
	calls     []string
	bodies    []map[string]any
	browsing  int
	asks      int
	connected chan struct{}
	left      chan struct{}
}

func newFakeLANGateway(t *testing.T) (*fakeLANGateway, *httptest.Server) {
	t.Helper()
	gateway := &fakeLANGateway{connected: make(chan struct{}, 4), left: make(chan struct{}, 4)}
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Secret") != "secret-interne" {
			w.WriteHeader(http.StatusForbidden)

			return
		}
		switch {
		case r.URL.Path == "/internal/lan/announce":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			gateway.mu.Lock()
			gateway.calls = append(gateway.calls, r.Method)
			gateway.bodies = append(gateway.bodies, body)
			gateway.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/internal/lan/browse":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			gateway.mu.Lock()
			gateway.browsing++
			gateway.mu.Unlock()
			gateway.connected <- struct{}{}
			_ = conn.WriteJSON(map[string]any{"servers": []map[string]string{{
				"instance": "Gamebox (10200)", "name": "Gamebox", "link": "http://192.168.1.20:10200", "address": "192.168.1.20",
			}}})
			for {
				var message map[string]string
				if conn.ReadJSON(&message) != nil {
					break
				}
				if message["type"] == "ask" {
					gateway.mu.Lock()
					gateway.asks++
					gateway.mu.Unlock()
				}
			}
			gateway.mu.Lock()
			gateway.browsing--
			gateway.mu.Unlock()
			gateway.left <- struct{}{}
		}
	}))
	t.Cleanup(server.Close)

	return gateway, server
}

func (gateway *fakeLANGateway) seen() ([]string, []map[string]any) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()

	return append([]string{}, gateway.calls...), append([]map[string]any{}, gateway.bodies...)
}

// L'ANNONCE SUIT LE MODE SERVEUR : ouverte, prolongée, fermée ; une trame
// inchangée ne rappelle rien, la première d'une connexion si ; un mode compte
// seul n'annonce rien.
func TestTheAnnouncementFollowsTheServerModeOnly(t *testing.T) {
	gateway, httpGateway := newFakeLANGateway(t)
	namedServer(t, "Salon")
	server := &Server{config: config.Config{GatewayControlURL: httpGateway.URL, InternalSecret: "secret-interne", PublicHost: "192.168.1.50", EdgePort: "11200"}}
	frame := func(server, accounts string) []byte {
		return []byte(`{"type":"pairing","server":` + server + `,"accounts":` + accounts + `,"codes":[],"ended":[],"closed":[]}`)
	}
	until := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	later := time.Now().Add(6 * time.Minute).UTC().Format(time.RFC3339)

	server.followPairing(frame(`null`, `[{"username":"marc","open_until":"`+until+`"}]`), false)
	if calls, _ := gateway.seen(); len(calls) != 0 {
		t.Fatalf("un mode compte seul a appelé la passerelle : %v", calls)
	}
	server.followPairing(frame(`{"open_until":"`+until+`","opened_by":"claude"}`, `[]`), false)
	server.followPairing(frame(`{"open_until":"`+until+`","opened_by":"claude"}`, `[{"username":"marc","open_until":"`+until+`"}]`), false)
	server.followPairing(frame(`{"open_until":"`+later+`","opened_by":"claude"}`, `[]`), false)
	server.followPairing(frame(`{"open_until":"`+later+`","opened_by":"claude"}`, `[]`), true)
	server.followPairing(frame(`null`, `[]`), false)
	server.followPairing(frame(`null`, `[]`), false)

	calls, bodies := gateway.seen()
	if strings.Join(calls, ",") != "PUT,PUT,PUT,DELETE" {
		t.Fatalf("appels : %v", calls)
	}
	first := bodies[0]
	if first["instance"] != "Salon (11200)" || first["link"] != "http://192.168.1.50:11200" || first["name"] != "Salon" ||
		first["port"] != float64(11200) || first["ttl"].(float64) < 298 || bodies[1]["ttl"].(float64) < 358 {
		t.Fatalf("annonces : %v", bodies)
	}
}

// LA RECHERCHE VIT TANT QU'UNE PAGE EST ABONNÉE AU SUJET `discovery` : la
// liste arrive, « Chercher de nouveau » repose la question, et le socket de la
// passerelle se ferme quand la page se désabonne.
func TestTheDiscoveryTopicOpensAndClosesTheSearch(t *testing.T) {
	gateway, httpGateway := newFakeLANGateway(t)
	server := &Server{
		config: config.Config{JWTSecret: facadeTestSecret, GatewayControlURL: httpGateway.URL, InternalSecret: "secret-interne"},
		pool:   registry.NewPool(), mirrors: registry.NewMirrors(),
	}
	server.discovery.Search = server.searchLAN
	httpServer := httptest.NewServer(http.HandlerFunc(server.handleFacadeWebSocket))
	t.Cleanup(httpServer.Close)

	connection, _, err := facadeTestDial(t, httpServer, testToken(t, auth.Claims{Username: "marc", RoleLevel: 2}))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	facadeTestSend(t, connection, map[string]any{"type": "discovery.search", "id": "0"})
	if reply := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.ID == "0" }); reply.Status != http.StatusConflict {
		t.Fatalf("chercher sans abonnement : %d", reply.Status)
	}

	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "discovery"}})
	select {
	case <-gateway.connected:
	case <-time.After(3 * time.Second):
		t.Fatal("la recherche ne s'est pas ouverte")
	}
	list := facadeTestNext(t, connection, func(push facadeTestPush) bool {
		return push.Topic == "discovery" && strings.Contains(string(push.Data), "Gamebox")
	})
	if !strings.Contains(string(list.Data), `"link":"http://192.168.1.20:10200"`) {
		t.Fatalf("liste : %s", list.Data)
	}

	facadeTestSend(t, connection, map[string]any{"type": "discovery.search", "id": "2"})
	if reply := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.ID == "2" }); reply.Status != http.StatusOK {
		t.Fatalf("chercher de nouveau : %d", reply.Status)
	}
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		gateway.mu.Lock()
		asks := gateway.asks
		gateway.mu.Unlock()
		if asks == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d questions reposées, attendu 1", asks)
		}
	}

	facadeTestSend(t, connection, map[string]any{"type": "unsubscribe", "id": "3", "data": map[string]any{"topic": "discovery"}})
	select {
	case <-gateway.left:
	case <-time.After(3 * time.Second):
		t.Fatal("la recherche ne s'est pas fermée avec le dernier abonné")
	}
}
