package facade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// serve — un kernel réduit au centre de messages et au routeur donné.
func serve(t *testing.T, router Router) *websocket.Conn {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		Serve(socket, "root", "jeton", router)
	}))
	t.Cleanup(server.Close)

	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("poignée de main : %v", err)
	}
	t.Cleanup(func() { connection.Close() })

	return connection
}

type frame struct {
	Type   string          `json:"type"`
	ID     string          `json:"id"`
	Topic  string          `json:"topic"`
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data"`
}

func next(t *testing.T, connection *websocket.Conn, wanted func(frame) bool) frame {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_ = connection.SetReadDeadline(deadline)
		var got frame
		if err := connection.ReadJSON(&got); err != nil {
			t.Fatalf("aucune trame attendue n'est arrivée : %v", err)
		}
		if wanted(got) {
			return got
		}
	}
}

func send(t *testing.T, connection *websocket.Conn, message map[string]any) {
	t.Helper()
	if err := connection.WriteJSON(message); err != nil {
		t.Fatalf("envoi : %v", err)
	}
}

// Ce qui n'existe pas se dit : une page ne doit pas attendre une réponse qui
// ne viendra jamais.
func TestWhatTheKernelCannotDoIsAnswered(t *testing.T) {
	connection := serve(t, Router{
		Topic: func(*Conn, Subscription) (func(context.Context), int, string) {
			return nil, http.StatusNotFound, "session not found"
		},
	})

	send(t, connection, map[string]any{"type": "bogus", "id": "1"})
	if got := next(t, connection, func(got frame) bool { return got.ID == "1" }); got.Type != "reply" || got.Status != http.StatusBadRequest {
		t.Fatalf("type inconnu : %+v", got)
	}

	send(t, connection, map[string]any{"type": "subscribe", "id": "2", "data": map[string]any{"topic": "status/x/y"}})
	if got := next(t, connection, func(got frame) bool { return got.ID == "2" }); got.Status != http.StatusNotFound {
		t.Fatalf("sujet refusé : %+v", got)
	}

	send(t, connection, map[string]any{"type": "subscribe", "id": "3"})
	if got := next(t, connection, func(got frame) bool { return got.ID == "3" }); got.Status != http.StatusBadRequest {
		t.Fatalf("sujet manquant : %+v", got)
	}
}

// Une commande reçoit son message et répond sur son id — le type dit à qui
// le message s'adresse.
func TestACommandIsDispatchedByItsType(t *testing.T) {
	connection := serve(t, Router{
		Commands: map[string]func(*Conn, Request){
			"echo": func(conn *Conn, request Request) {
				conn.Reply(request.ID, http.StatusAccepted, OKBody(map[string]string{"by": conn.Username(), "data": string(request.Data)}))
			},
		},
	})

	send(t, connection, map[string]any{"type": "echo", "id": "e", "data": "bonjour"})
	got := next(t, connection, func(got frame) bool { return got.ID == "e" })
	if got.Status != http.StatusAccepted || !strings.Contains(string(got.Data), `"by":"root"`) || !strings.Contains(string(got.Data), `bonjour`) {
		t.Fatalf("réponse : %+v (%s)", got, got.Data)
	}
}

// S'abonner deux fois au même sujet remplace le premier abonnement ; se
// désabonner arrête les poussées.
func TestASubscriptionIsReplacedThenEnded(t *testing.T) {
	var running atomic.Int64
	connection := serve(t, Router{
		Topic: func(conn *Conn, subscription Subscription) (func(context.Context), int, string) {
			return func(ctx context.Context) {
				running.Add(1)
				defer running.Add(-1)
				PushEvery(ctx, conn, Push{Type: "tick", Topic: subscription.Topic}, 20*time.Millisecond, func() (any, bool) { return 1, true })
			}, 0, ""
		},
	})

	send(t, connection, map[string]any{"type": "subscribe", "data": map[string]any{"topic": "t"}})
	next(t, connection, func(got frame) bool { return got.Type == "tick" })
	send(t, connection, map[string]any{"type": "subscribe", "data": map[string]any{"topic": "t"}})
	next(t, connection, func(got frame) bool { return got.Type == "tick" })

	time.Sleep(100 * time.Millisecond)
	if count := running.Load(); count != 1 {
		t.Fatalf("%d abonnements vivants au même sujet, attendu 1", count)
	}

	send(t, connection, map[string]any{"type": "unsubscribe", "id": "u", "data": map[string]any{"topic": "t"}})
	next(t, connection, func(got frame) bool { return got.ID == "u" })
	time.Sleep(100 * time.Millisecond)
	if count := running.Load(); count != 0 {
		t.Fatalf("%d abonnements vivants après le désabonnement", count)
	}
}

// Une liste identique à la précédente ne repart pas.
func TestPushOnChangeSkipsWhatDidNotChange(t *testing.T) {
	wake := make(chan struct{}, 1)
	var value atomic.Int64
	connection := serve(t, Router{
		Topic: func(conn *Conn, subscription Subscription) (func(context.Context), int, string) {
			return func(ctx context.Context) {
				PushOnChange(ctx, conn, Push{Type: "list", Topic: subscription.Topic}, wake, func() any { return value.Load() })
			}, 0, ""
		},
	})

	send(t, connection, map[string]any{"type": "subscribe", "data": map[string]any{"topic": "l"}})
	if got := next(t, connection, func(got frame) bool { return got.Type == "list" }); string(got.Data) != "0" {
		t.Fatalf("premier instantané : %s", got.Data)
	}

	wake <- struct{}{}
	value.Store(1)
	wake <- struct{}{}
	if got := next(t, connection, func(got frame) bool { return got.Type == "list" }); string(got.Data) != "1" {
		t.Fatalf("changement : %s — un réveil sans changement est reparti", got.Data)
	}
}

// Un relevé manqué ne part pas, et rien ne part après la fin.
func TestPushEverySkipsAMissedReading(t *testing.T) {
	var reads atomic.Int64
	connection := serve(t, Router{
		Topic: func(conn *Conn, subscription Subscription) (func(context.Context), int, string) {
			return func(ctx context.Context) {
				PushEvery(ctx, conn, Push{Type: "status", Topic: subscription.Topic}, 30*time.Millisecond, func() (any, bool) {
					count := reads.Add(1)

					return count, count != 2
				})
			}, 0, ""
		},
	})

	send(t, connection, map[string]any{"type": "subscribe", "data": map[string]any{"topic": "s"}})
	first := next(t, connection, func(got frame) bool { return got.Type == "status" })
	second := next(t, connection, func(got frame) bool { return got.Type == "status" })
	if string(first.Data) != "1" || string(second.Data) != "3" {
		t.Fatalf("relevés : %s puis %s — le deuxième, manqué, ne devait pas partir", first.Data, second.Data)
	}
}

// Le rythme demandé est borné.
func TestThePaceIsBounded(t *testing.T) {
	cases := map[int]time.Duration{0: PaceDefault, -5: PaceDefault, 10: PaceMin, 3000: 3 * time.Second, 999999: PaceMax}
	for milliseconds, expected := range cases {
		if got := Pace(milliseconds); got != expected {
			t.Errorf("every=%d → %v, attendu %v", milliseconds, got, expected)
		}
	}
}

// Une commande rejouée arrive au gestionnaire comme la requête qu'elle
// remplace : le jeton de la connexion, les valeurs du chemin, le corps — et
// elle rend ce qu'il a écrit.
func TestCallReplaysTheHandlerItReplaces(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer jeton" || r.PathValue("id") != "abc" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusTeapot)

			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write(ErrorBody("déjà en train de jouer"))
	})

	connection := serve(t, Router{
		Commands: map[string]func(*Conn, Request){
			"stop": func(conn *Conn, request Request) {
				status, body := Call(conn, handler, http.MethodPost, "/stop", nil, map[string]string{"id": "abc"})
				conn.Reply(request.ID, status, body)
			},
		},
	})

	send(t, connection, map[string]any{"type": "stop", "id": "s"})
	got := next(t, connection, func(got frame) bool { return got.ID == "s" })
	if got.Status != http.StatusConflict || !strings.Contains(string(got.Data), "déjà en train de jouer") {
		t.Fatalf("rejeu : %+v (%s)", got, got.Data)
	}
}

// LE SERVEUR FERME LES CONNEXIONS D'UN COMPTE (chantier 20) : celles qu'il
// désigne, et pas les autres.
func TestTheServerClosesTheConnectionsOfAnAccount(t *testing.T) {
	page := serve(t, Router{})
	time.Sleep(100 * time.Millisecond) // la connexion est inscrite à son ouverture

	if closed := Close(func(username, token string) bool { return username == "quelqu-un-d-autre" }); closed != 0 {
		t.Fatalf("%d connexion(s) fermée(s) pour un autre compte", closed)
	}
	if closed := Close(func(username, token string) bool { return username == "root" && token == "jeton" }); closed != 1 {
		t.Fatalf("%d connexion(s) fermée(s), attendu 1", closed)
	}
	page.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := page.ReadMessage(); err == nil {
		t.Fatal("la page lit encore : sa connexion n'a pas été fermée")
	}
}
