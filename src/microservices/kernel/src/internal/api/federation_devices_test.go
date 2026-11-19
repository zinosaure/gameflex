package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"gameflex/facade"
	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/registry"
)

// LES AMIS SONT INTERROGÉS EN PARALLÈLE, ET LEUR ORDRE EST GARDÉ (#106).
//
// Trois amis qui répondent en 900, 600 et 300 ms : en série la liste attendait
// 1,8 s, en parallèle elle attend le plus lent, 0,9 s. Et le PREMIER du registre
// est le plus lent exprès — l'ordre de la liste ne doit pas suivre l'ordre
// d'arrivée, sinon elle changerait d'un affichage à l'autre.
func TestRemoteDevicesAsksEveryFriendAtOnce(t *testing.T) {
	friend := func(name string, wait time.Duration) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			time.Sleep(wait)
			json.NewEncoder(writer).Encode(map[string]any{
				"data": []map[string]any{{"id": name, "hostName": name}},
			})
		}))
	}
	slowest := friend("a", 900*time.Millisecond)
	defer slowest.Close()
	middle := friend("b", 600*time.Millisecond)
	defer middle.Close()
	fastest := friend("c", 300*time.Millisecond)
	defer fastest.Close()

	server := &Server{}
	server.serverRegistry.items = []registeredServer{
		{ID: 1, Link: slowest.URL, Token: "t", Hostname: "un"},
		{ID: 2, Link: middle.URL, Token: "t", Hostname: "deux"},
		{ID: 3, Link: fastest.URL, Token: "t", Hostname: "trois"},
	}
	server.serverRegistry.fetchedAt = time.Now()

	started := time.Now()
	response := httptest.NewRecorder()
	server.handleRemoteDevices(response, httptest.NewRequest(http.MethodGet, "/api/v1/servers/remoteplay/devices", nil))
	elapsed := time.Since(started)

	var payload struct {
		Data []struct {
			Source   string `json:"source"`
			ServerID int64  `json:"serverId"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("réponse illisible : %v", err)
	}

	if elapsed > 1400*time.Millisecond {
		t.Errorf("la liste a attendu %v : les amis ont été interrogés l'un après l'autre (série = 1,8 s)", elapsed)
	}
	want := []string{"un", "deux", "trois"}
	if len(payload.Data) != len(want) {
		t.Fatalf("%d console(s), attendu %d", len(payload.Data), len(want))
	}
	for index, device := range payload.Data {
		if device.Source != want[index] {
			t.Errorf("position %d : %q, attendu %q — l'ordre a suivi l'arrivée, pas le registre", index, device.Source, want[index])
		}
	}
}

// fakeRemoteplay — le service remote-play d'un pair : la liste en HTTP (`my-devices`), et son canal d'état
// (`subscribeDevices` sur le socket d'entrée), qui pousse ce qu'on lui donne. Chaque trame porte aussi une clé
// d'appairage, qui ne doit jamais sortir du pair.
type fakeRemoteplay struct {
	*httptest.Server
	push      chan string
	secrets   chan string
	subscribe chan struct{}
}

func newFakeRemoteplay(t *testing.T, status string) *fakeRemoteplay {
	t.Helper()
	service := &fakeRemoteplay{push: make(chan string, 4), secrets: make(chan string, 4), subscribe: make(chan struct{}, 4)}
	device := func(status string) string {
		return `{"id":"ps5","hostId":"H1","hostName":"PS5-896","hostType":"PS5","ipAddress":"192.168.1.20","status":"` + status + `","hasActiveSession":false,"registKey":"secret"}`
	}
	service.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/playstation/my-devices":
			_, _ = w.Write([]byte(`{"data":[` + device(status) + `]}`))
		case "/api/v1/remoteplay/input/ws":
			service.secrets <- r.Header.Get("X-Internal-Secret")
			socket, err := facade.Upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer socket.Close()
			var hello struct {
				Type string `json:"type"`
			}
			if socket.ReadJSON(&hello) != nil || hello.Type != "subscribeDevices" {
				return
			}
			service.subscribe <- struct{}{}
			for next := range service.push {
				if socket.WriteMessage(websocket.TextMessage, []byte(`{"type":"devices","devices":[`+device(next)+`]}`)) != nil {
					return
				}
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(func() {
		close(service.push)
		service.Close()
	})

	return service
}

// L'ÉTAT D'UNE CONSOLE PRÊTÉE ARRIVE SEUL (`#750`) — de serveur à serveur, par deux vrais routeurs : le sujet
// `servers/3/devices` de notre canal suit le socket du pair, ouvert avec notre jeton des consoles chez lui ; la PS5 en
// veille à l'abonnement, puis réveillée dès que son service le pousse, puis rendormie. Rien d'autre que les champs de
// la liste ne traverse (la clé d'appairage reste chez le pair), et un jeton de catalogue seul n'ouvre pas le socket.
func TestPeerDevicesReachOurPageThroughOurChannel(t *testing.T) {
	service := newFakeRemoteplay(t, "standby")
	identity := newFakeIdentity(t, map[string]string{"catalogue": "catalog", "consoles": "catalog"}, nil)
	peer := &Server{
		config: config.Config{CredentialURL: identity.URL, InternalSecret: federationTestInternal, JWTSecret: facadeTestSecret,
			RemoteplayControlURL: service.URL},
		pool:    registry.NewPool(),
		mirrors: registry.NewMirrors(),
	}
	surface := httptest.NewServer(http.StripPrefix("/api/v1/federation", peer.federationRouter()))
	t.Cleanup(surface.Close)

	// Le jeton de catalogue seul ouvre aussi ce socket depuis le chantier 66 : la portée `devices` est partie.

	client, httpServer := facadeTestServer(t)
	client.serverRegistry.items = []registeredServer{{ID: 3, Link: surface.URL, Token: "consoles", Hostname: "ami"}}
	client.serverRegistry.fetchedAt = time.Now()
	connection, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "gino"))
	if err != nil {
		t.Fatalf("poignée de main : %v", err)
	}
	defer connection.Close()

	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "id": "x", "data": map[string]any{"topic": "servers/9/devices"}})
	if missing := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.ID == "x" }); missing.Status != http.StatusNotFound {
		t.Fatalf("un serveur absent du registre : %+v", missing)
	}

	facadeTestSend(t, connection, map[string]any{"type": "subscribe", "id": "1", "data": map[string]any{"topic": "servers/3/devices"}})
	read := func() []map[string]any {
		push := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "devices" && push.Topic == "servers/3/devices" })
		var list []map[string]any
		if err := json.Unmarshal(push.Data, &list); err != nil {
			t.Fatalf("liste illisible : %s", push.Data)
		}

		return list
	}
	first := read()
	if len(first) != 1 || first[0]["id"] != "ps5" || first[0]["status"] != "standby" {
		t.Fatalf("premier instantané : %+v", first)
	}
	if _, leaked := first[0]["registKey"]; leaked || len(first[0]) != 7 {
		t.Fatalf("champs servis : %+v", first[0])
	}
	if secret := <-service.secrets; secret != federationTestInternal {
		t.Fatalf("secret présenté au service : %q", secret)
	}
	<-service.subscribe

	service.push <- "awake"
	if awake := read(); len(awake) != 1 || awake[0]["status"] != "awake" {
		t.Fatalf("la console réveillée : %+v", awake)
	}
	service.push <- "standby"
	if asleep := read(); len(asleep) != 1 || asleep[0]["status"] != "standby" {
		t.Fatalf("la console rendormie : %+v", asleep)
	}
}
