package remote

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// LES RESSOURCES D'UN PAIR se lisent dans sa déclaration publique ; un pair
// qui n'en déclare pas n'en a pas.
func TestResourcesReadsThePeerDeclaration(t *testing.T) {
	declared := true
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/capabilities" {
			http.NotFound(w, r)
			return
		}
		if declared {
			w.Write([]byte(`{"capabilities":{"platform":"web","resources":{"gpu":"AMD","vramBytes":8589934592,"cpuThreads":16,"memoryBytes":40000000000}}}`))
		} else {
			w.Write([]byte(`{"capabilities":{"platform":"web"}}`))
		}
	}))
	defer peer.Close()

	raw, ok := PeerResources(Peer{Link: peer.URL, Token: "t"})
	var resources struct {
		GPU        string `json:"gpu"`
		CPUThreads int    `json:"cpuThreads"`
	}
	if !ok || json.Unmarshal(raw, &resources) != nil || resources.GPU != "AMD" || resources.CPUThreads != 16 {
		t.Fatalf("got %s %v", raw, ok)
	}
	declared = false
	if _, ok := PeerResources(Peer{Link: peer.URL, Token: "t"}); ok {
		t.Fatal("un pair sans déclaration ne doit rien rendre")
	}
}
