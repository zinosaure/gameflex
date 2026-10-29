package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	sharedauth "gameflex/auth"
	"gameflex/kernel/internal/config"
)

// fakeChild answers `/v4/terminate` like a child would: the owner may end the
// game, and an administrator the kernel vouches for; anybody else may not.
// It holds ITS session's secret, as a container does (chantier 34, P10) —
// the installation's would be refused.
func fakeChild(t *testing.T, owner string, status int) (port int) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v4/terminate" || r.Header.Get("X-Internal-Secret") != sharedauth.SessionSecret("interne", "s1") {
			http.NotFound(w, r)

			return
		}
		if status != 0 {
			w.WriteHeader(status)

			return
		}
		var body struct {
			Username string `json:"username"`
			Admin    bool   `json:"admin"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Username != owner && !body.Admin {
			w.WriteHeader(http.StatusForbidden)

			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	_, raw, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ = strconv.Atoi(raw)

	return port
}

// LE CHILD JUGE L'ARRÊT DEMANDÉ AU TABLEAU DE BORD (P2) ; le kernel n'arrête
// seul qu'un child qui ne répond pas.
func TestTheChildJudgesTheDashboardStop(t *testing.T) {
	server := &Server{config: config.Config{PublicHost: "127.0.0.1", InternalSecret: "interne"}}
	port := fakeChild(t, "gino", 0)

	if handled, status := server.endThroughChild(context.Background(), "s1", "", port, "gino", false); !handled || status != http.StatusOK {
		t.Fatalf("the owner's stop = (%v, %d), want handled 200", handled, status)
	}
	if handled, status := server.endThroughChild(context.Background(), "s1", "", port, "dave", false); !handled || status != http.StatusForbidden {
		t.Fatalf("a guest's stop = (%v, %d), want handled 403", handled, status)
	}

	// UN ADMINISTRATEUR, PAS PROPRIÉTAIRE (Live now, 2026-10-25) : le kernel
	// le dit à la partie, qui accepte.
	if handled, status := server.endThroughChild(context.Background(), "s1", "", port, "root", true); !handled || status != http.StatusOK {
		t.Fatalf("an administrator's stop = (%v, %d), want handled 200", handled, status)
	}

	// Une image d'avant ce chantier ne connaît pas la route : le kernel
	// reprend la main.
	old := fakeChild(t, "gino", http.StatusNotFound)
	if handled, _ := server.endThroughChild(context.Background(), "s1", "", old, "gino", false); handled {
		t.Fatal("a child without the route must leave the stop to the kernel")
	}

	// Personne n'écoute : idem.
	if handled, _ := server.endThroughChild(context.Background(), "s1", "", 1, "gino", false); handled {
		t.Fatal("an unreachable child must leave the stop to the kernel")
	}
}
