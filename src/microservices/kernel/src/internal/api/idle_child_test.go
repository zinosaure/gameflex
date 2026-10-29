package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// Le contrôle des sessions fantômes parle à l'enfant sur le port de son
// conteneur, là où chaque runtime monte ses routes : sous /v4. Le faux enfant
// ci-dessous ne sert que ça, comme les vrais — l'ancienne adresse
// (/api/v1/sessions/<id>/child/v4/…) y reçoit un 404, ce qu'elle recevait
// en production depuis le 2026-09-09.
func TestTheGhostCheckReachesTheChildWhereItListens(t *testing.T) {
	child := http.NewServeMux()
	child.HandleFunc("GET /v4/report", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	child.HandleFunc("POST /v4/wind-down", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ending":true}`))
	})
	server := httptest.NewServer(child)
	defer server.Close()

	host, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)

	report, err := http.Get(childV4URL(host, port, "report"))
	if err != nil || report.StatusCode != http.StatusOK {
		t.Fatalf("report : %v %v, attendu 200", err, report)
	}
	report.Body.Close()

	windDown, err := http.Post(childV4URL(host, port, "wind-down"), "application/json", nil)
	if err != nil || windDown.StatusCode != http.StatusOK {
		t.Fatalf("wind-down : %v %v, attendu 200", err, windDown)
	}
	windDown.Body.Close()
}
