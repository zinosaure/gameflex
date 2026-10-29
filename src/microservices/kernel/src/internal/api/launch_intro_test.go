package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/registry"
)

// fakeGateway — une passerelle qui garde ce que le kernel lui demande.
type fakeGateway struct {
	mu      sync.Mutex
	calls   []string
	secrets []string
}

func (gateway *fakeGateway) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	gateway.mu.Lock()
	gateway.calls = append(gateway.calls, r.Method+" "+r.URL.Path+" "+string(body))
	gateway.secrets = append(gateway.secrets, r.Header.Get("X-Internal-Secret"))
	gateway.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (gateway *fakeGateway) seen() ([]string, []string) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()

	return append([]string(nil), gateway.calls...), append([]string(nil), gateway.secrets...)
}

// LE FLUX DU LANCEMENT S'OUVRE AVANT QUE LA PAGE N'EN SACHE RIEN (chantier
// 35) : la passerelle l'a reçu quand le premier message de la commande — sans
// étape, avec l'identifiant — part vers la page. Un lancement refusé le
// referme.
func TestALaunchOpensItsStreamFirstAndClosesItWhenRefused(t *testing.T) {
	gateway := &fakeGateway{}
	gatewayServer := httptest.NewServer(http.HandlerFunc(gateway.handler))
	t.Cleanup(gatewayServer.Close)
	server := &Server{
		config: config.Config{
			JWTSecret:         facadeTestSecret,
			InternalSecret:    "secret-interne",
			GatewayControlURL: gatewayServer.URL,
		},
		pool:    registry.NewPool(),
		mirrors: registry.NewMirrors(),
	}
	httpServer := httptest.NewServer(http.HandlerFunc(server.handleFacadeWebSocket))
	t.Cleanup(httpServer.Close)

	connection, _, err := facadeTestDial(t, httpServer, facadeTestToken(t, "root"))
	if err != nil {
		t.Fatalf("poignée de main : %v", err)
	}
	defer connection.Close()

	facadeTestSend(t, connection, map[string]any{"type": "launch", "id": "9", "data": map[string]any{"platform": "bogus"}})
	first := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.ID == "9" })
	if first.Type != "progress" || first.LaunchID == "" || len(first.Data) != 0 {
		t.Fatalf("premier message de la commande : %+v", first)
	}
	calls, _ := gateway.seen()
	if len(calls) == 0 || calls[0] != "POST /internal/launches/"+first.LaunchID+" " {
		t.Fatalf("la passerelle n'avait pas ouvert le flux avant le premier message : %v", calls)
	}

	reply := facadeTestNext(t, connection, func(push facadeTestPush) bool { return push.Type == "reply" && push.ID == "9" })
	calls, secrets := gateway.seen()
	if len(calls) != 2 || calls[1] != "DELETE /internal/launches/"+reply.LaunchID+" " {
		t.Fatalf("appels à la passerelle : %v", calls)
	}
	for _, secret := range secrets {
		if secret != "secret-interne" {
			t.Fatalf("un appel sans le secret interne : %q", secret)
		}
	}
}

// UNE PARTIE CRÉÉE PREND LES SPECTATEURS DE SON LANCEMENT : l'identifiant de
// session se lit sous ses deux formes (`sessionId`, `session_id` à streamex).
func TestASucceededLaunchAttachesItsStreamToTheSession(t *testing.T) {
	gateway := &fakeGateway{}
	gatewayServer := httptest.NewServer(http.HandlerFunc(gateway.handler))
	t.Cleanup(gatewayServer.Close)
	server := &Server{config: config.Config{InternalSecret: "secret-interne", GatewayControlURL: gatewayServer.URL}}

	server.settleLaunchStream("launch-a", http.StatusOK, []byte(`{"data":{"session_id":"streamex-1"}}`))
	server.settleLaunchStream("launch-b", http.StatusOK, []byte(`{"data":{"sessionId":"retro-1"}}`))
	server.settleLaunchStream("launch-c", http.StatusOK, []byte(`{"data":{}}`))

	calls, _ := gateway.seen()
	expected := []string{
		`POST /internal/launches/launch-a/session {"sessionId":"streamex-1"}`,
		`POST /internal/launches/launch-b/session {"sessionId":"retro-1"}`,
		`DELETE /internal/launches/launch-c `,
	}
	if len(calls) != len(expected) {
		t.Fatalf("appels : %v", calls)
	}
	for index := range expected {
		if calls[index] != expected[index] {
			t.Fatalf("appel %d : %q, attendu %q", index, calls[index], expected[index])
		}
	}
}

// SANS PASSERELLE DÉCLARÉE, RIEN NE PART — et le lancement n'en dépend pas.
func TestNoGatewayMeansNoLaunchStream(t *testing.T) {
	server := &Server{config: config.Config{InternalSecret: "secret-interne"}}
	server.announceLaunchStream("launch-x")
	server.settleLaunchStream("launch-x", http.StatusOK, []byte(`{"data":{"sessionId":"s"}}`))
}
