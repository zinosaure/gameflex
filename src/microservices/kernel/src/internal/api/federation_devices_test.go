package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
