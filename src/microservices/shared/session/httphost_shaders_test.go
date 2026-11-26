package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// LA ROUE DE COMPILATION (chantier 50) : chaque onglet reçoit la rafale ; celui qui arrive pendant la reçoit à son
// arrivée ; celui qui arrive après, rien.
func TestShadersReachEveryTabAndALateOne(t *testing.T) {
	host := NewHTTPHost(Config{
		SessionID: "s1", PlatformID: "streamex", Owner: "gino", MaxPlayers: 2,
		JWTPublicKey: intentKey(t), InternalSecret: "interne",
		BeatTimeout: time.Hour, AFKAfter: time.Hour,
		EndSession: func(string) {},
	})
	mux := http.NewServeMux()
	host.Routes(mux, "/v4")
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	first := openTab(t, server, "gino")
	first.next("welcome")
	host.PushShaders(true, 5)
	if got := first.next("shaders"); got["compiling"] != true || got["count"] != float64(5) {
		t.Fatalf("premier onglet : %+v", got)
	}

	late := openTab(t, server, "invite")
	if got := late.next("shaders"); got["compiling"] != true || got["count"] != float64(5) {
		t.Fatalf("onglet arrivé pendant la rafale : %+v", got)
	}

	host.PushShaders(false, 0)
	// Un onglet peut recevoir deux fois le même état (son arrivée croise une poussée) : sans effet, on lit jusqu'à la fin.
	for name, tab := range map[string]*tab{"premier": first, "arrivé pendant": late} {
		got := tab.next("shaders")
		for got["compiling"] == true && got["count"] == float64(5) {
			got = tab.next("shaders")
		}
		if got["compiling"] != false || got["count"] != float64(0) {
			t.Fatalf("fin de rafale, onglet %s : %+v", name, got)
		}
	}

	after := openTab(t, server, "spectateur")
	after.next("welcome")
	_ = after.conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	for {
		var message map[string]any
		if err := after.conn.ReadJSON(&message); err != nil {
			break
		}
		if message["type"] == "shaders" {
			t.Fatalf("hors rafale, un onglet qui arrive ne reçoit rien : %+v", message)
		}
	}
}
