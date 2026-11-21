package main

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterRequiresSecret(t *testing.T) {
	g := &gateway{secret: strings.Repeat("x", 32), sources: map[string]*source{}}
	for _, path := range []string{"/internal/stream/ws?sessionId=" + strings.Repeat("a", 32), "/internal/launches/abc"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if strings.Contains(path, "launches") {
			r.Method = "POST"
		}
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("unauthenticated internal endpoint: %d", w.Code)
		}
	}
}
func TestUnknownSourceCannotAllocateViewer(t *testing.T) {
	g := &gateway{sources: map[string]*source{}}
	r := httptest.NewRequest("POST", "/api/v1/gateway-webrtc/streams/"+strings.Repeat("a", 32)+"/viewer-offer", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestOrigin(t *testing.T) {
	for _, item := range []struct {
		origin string
		ok     bool
	}{{"http://192.168.1.63:2000", true}, {"https://evil.test", false}, {"", true}} {
		r := httptest.NewRequest("GET", "http://192.168.1.63/api", nil)
		r.Header.Set("Origin", item.origin)
		if upgrader.CheckOrigin(r) != item.ok {
			t.Fatal(item.origin)
		}
	}
}
func TestChildRegistrationAndDisconnect(t *testing.T) {
	g := &gateway{secret: strings.Repeat("x", 32), sources: map[string]*source{}}
	server := httptest.NewServer(g)
	defer server.Close()
	id := strings.Repeat("a", 32)
	header := http.Header{"X-Internal-Secret": {sessionSecret(g.secret, id)}}
	ws, _, err := websocket.DefaultDialer.Dial(strings.Replace(server.URL, "http", "ws", 1)+"/internal/stream/ws?sessionId="+id, header)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if ws.WriteJSON(map[string]string{"type": "register", "sessionId": id, "platform": "retro"}) != nil {
		t.Fatal("register")
	}
	var message map[string]any
	if ws.ReadJSON(&message) != nil || message["type"] != "registered" {
		t.Fatal("acknowledgement")
	}
	g.mu.Lock()
	_, exists := g.sources[id]
	g.mu.Unlock()
	if !exists {
		t.Fatal("missing registered source")
	}
}

func TestScopedSecret(t *testing.T) {
	master := "test-master"
	id := strings.Repeat("a", 32)
	if sessionSecret(master, id) != "ScyFLyfMmjM6tKm3gHSBUEOFO9gIF_YhCbJeWMvTfKI" {
		t.Fatal("kernel HMAC contract changed")
	}
	g := &gateway{secret: master, sources: map[string]*source{}}
	for _, key := range []string{master, sessionSecret(master, strings.Repeat("b", 32)), ""} {
		r := httptest.NewRequest("GET", "/internal/stream/ws?sessionId="+id, nil)
		r.Header.Set("X-Internal-Secret", key)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("cross-session registration accepted")
		}
	}
}

func TestPublicICEConfiguration(t *testing.T) {
	config := publicICEConfiguration("")
	if len(config.ICEServers) != 1 || config.ICEServers[0].URLs[0] != "stun:stun.l.google.com:19302" {
		t.Fatalf("default public STUN missing: %#v", config.ICEServers)
	}
	if len(publicICEConfiguration("http://invalid.test").ICEServers) != 0 {
		t.Fatal("non-STUN URL accepted")
	}
}

// The runtime sends SPS/PPS with duration zero, before the timed VCL NAL.
// Exercise the actual registered websocket transport and Pion packetizer in the live probe;
// stripping these messages produces undecodable RTP despite successful ICE.
