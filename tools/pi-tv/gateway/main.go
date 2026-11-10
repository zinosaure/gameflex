// Local, LAN-only WebRTC adapter for the open retrogaming runtime.
// Implements the documented gatewayclient protocol; no proprietary seals or Remote Play.
package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

var validID = regexp.MustCompile(`^[a-f0-9]{16,128}$`)
var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, e := url.Parse(origin)
	return e == nil && u.Hostname() == strings.Split(r.Host, ":")[0]
}}

type source struct {
	mu          sync.Mutex
	conn        *websocket.Conn
	viewers     map[string]*viewer
	videoConfig [][]byte
	closed      bool
}

func (s *source) send(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return s.conn.WriteJSON(v)
}

type viewer struct {
	id           string
	pc           *webrtc.PeerConnection
	video, audio *webrtc.TrackLocalStaticSample
	once         sync.Once
	source       *source
	signalMu     sync.Mutex
	signalUsed   bool
	input        *webrtc.DataChannel
	hasInput     bool
	fallbackMu   sync.Mutex
	fallback     *websocket.Conn
}

func (v *viewer) close() {
	v.once.Do(func() {
		v.source.mu.Lock()
		delete(v.source.viewers, v.id)
		v.source.mu.Unlock()
		v.fallbackMu.Lock()
		if v.fallback != nil {
			v.fallback.Close()
			v.fallback = nil
		}
		v.fallbackMu.Unlock()
		v.pc.Close()
		v.source.send(map[string]any{"type": "viewer-left", "viewerId": v.id})
	})
}

func (v *viewer) fallbackReady() bool {
	v.fallbackMu.Lock()
	defer v.fallbackMu.Unlock()
	return v.fallback != nil
}

func (v *viewer) sendFallback(data []byte) {
	v.sendFallbackMessage(websocket.BinaryMessage, data, nil)
}

func (v *viewer) sendFallbackMessage(kind int, data []byte, expected *websocket.Conn) bool {
	v.fallbackMu.Lock()
	defer v.fallbackMu.Unlock()
	if v.fallback == nil || (expected != nil && v.fallback != expected) {
		return false
	}
	v.fallback.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if v.fallback.WriteMessage(kind, data) != nil {
		v.fallback.Close()
		v.fallback = nil
		return false
	}
	return true
}

type gateway struct {
	mu         sync.Mutex
	sources    map[string]*source
	api        *webrtc.API
	peerConfig webrtc.Configuration
	secret     string
	offerSlots chan struct{}
}

func publicICEConfiguration(raw string) webrtc.Configuration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "stun:stun.l.google.com:19302"
	}
	if !strings.HasPrefix(raw, "stun:") && !strings.HasPrefix(raw, "stuns:") {
		return webrtc.Configuration{}
	}
	return webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{raw}}}}
}

func jsonReply(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(body)
}
func (g *gateway) authorized(r *http.Request) bool {
	v := r.Header.Get("X-Internal-Secret")
	return len(g.secret) > 0 && len(v) == len(g.secret) && subtle.ConstantTimeCompare([]byte(v), []byte(g.secret)) == 1
}

// Same HMAC domain and encoding as shared/auth.SessionSecret. A runtime never gets the installation key.
func sessionSecret(master, id string) string {
	if master == "" || id == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(master))
	mac.Write([]byte("gameflex/auth: session secret v1\x00" + id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (g *gateway) child(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("sessionId")
	expected := sessionSecret(g.secret, id)
	if expected == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Secret")), []byte(expected)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	if !validID.MatchString(id) {
		http.Error(w, "invalid session", 400)
		return
	}
	conn, e := upgrader.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(4 << 20)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var registration struct {
		Type, SessionID, Platform string
		Seal                      json.RawMessage
	}
	if conn.ReadJSON(&registration) != nil || registration.Type != "register" || registration.SessionID != id || registration.Platform != "retro" || len(registration.Seal) > 0 {
		return
	}
	s := &source{conn: conn, viewers: map[string]*viewer{}}
	g.mu.Lock()
	old := g.sources[id]
	if len(g.sources) >= 4 && old == nil {
		g.mu.Unlock()
		return
	}
	g.sources[id] = s
	g.mu.Unlock()
	if old != nil {
		old.conn.Close()
	}
	defer func() {
		g.mu.Lock()
		if g.sources[id] == s {
			delete(g.sources, id)
		}
		g.mu.Unlock()
		s.mu.Lock()
		s.closed = true
		vs := []*viewer{}
		for _, v := range s.viewers {
			vs = append(vs, v)
		}
		s.mu.Unlock()
		for _, v := range vs {
			v.close()
		}
	}()
	if s.send(map[string]any{"type": "registered"}) != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})
	for {
		kind, data, e := conn.ReadMessage()
		if e != nil {
			return
		}
		if kind == websocket.TextMessage {
			var down struct{ Type, ViewerID, Data string }
			if json.Unmarshal(data, &down) == nil && down.Type == "to-viewer" {
				raw, err := base64.StdEncoding.DecodeString(down.Data)
				s.mu.Lock()
				target := s.viewers[down.ViewerID]
				s.mu.Unlock()
				if err == nil && len(raw) <= 65536 && target != nil && target.input != nil && target.input.ReadyState() == webrtc.DataChannelStateOpen {
					target.input.SendText(string(raw))
				}
			}
			continue
		}
		if kind != websocket.BinaryMessage || len(data) < 10 {
			continue
		}
		duration := time.Duration(binary.BigEndian.Uint64(data[1:9]))
		if duration < 0 || duration > time.Second || (data[0] == 1 && duration == 0) {
			continue
		}
		s.mu.Lock()
		if data[0] == 0 && duration == 0 && len(data) <= 65536 {
			// Keep the latest Annex-B codec headers. A public HTTPS viewer can
			// join long after the encoder emitted SPS/PPS; without replaying
			// them its first IDR is valid H264 but cannot be decoded.
			s.videoConfig = append(s.videoConfig, append([]byte(nil), data...))
			if len(s.videoConfig) > 4 {
				s.videoConfig = s.videoConfig[len(s.videoConfig)-4:]
			}
		}
		vs := []*viewer{}
		for _, v := range s.viewers {
			vs = append(vs, v)
		}
		s.mu.Unlock()
		for _, v := range vs {
			v.sendFallback(data)
			if data[0] == 0 {
				if duration == 0 {
					continue
				}
				v.video.WriteSample(media.Sample{Data: data[9:], Duration: duration})
			} else if data[0] == 1 {
				v.audio.WriteSample(media.Sample{Data: data[9:], Duration: duration})
			}
		}
	}
}
func (g *gateway) offer(w http.ResponseWriter, r *http.Request, s *source) {
	select {
	case g.offerSlots <- struct{}{}:
		defer func() { <-g.offerSlots }()
	default:
		http.Error(w, "offer limit", 429)
		return
	}
	var options struct {
		HasInput bool   `json:"hasInput"`
		Username string `json:"username"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&options) != nil || len(options.Username) > 128 {
		http.Error(w, "invalid body", 400)
		return
	}
	s.mu.Lock()
	full := len(s.viewers) >= 8 || s.closed
	s.mu.Unlock()
	if full {
		http.Error(w, "viewer limit", 429)
		return
	}
	pc, e := g.api.NewPeerConnection(g.peerConfig)
	if e != nil {
		http.Error(w, "peer unavailable", 503)
		return
	}
	b := make([]byte, 16)
	if _, e = rand.Read(b); e != nil {
		pc.Close()
		http.Error(w, "random unavailable", 503)
		return
	}
	video, e := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"}, "video", "gameflex")
	if e != nil {
		pc.Close()
		http.Error(w, "track unavailable", 503)
		return
	}
	audio, e := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "audio", "gameflex")
	if e != nil {
		pc.Close()
		http.Error(w, "track unavailable", 503)
		return
	}
	v := &viewer{id: hex.EncodeToString(b), pc: pc, video: video, audio: audio, source: s, hasInput: options.HasInput}
	for _, track := range []*webrtc.TrackLocalStaticSample{video, audio} {
		sender, err := pc.AddTrack(track)
		if err != nil {
			pc.Close()
			http.Error(w, "track unavailable", 503)
			return
		}
		go func() {
			buf := make([]byte, 1500)
			for {
				if _, _, e := sender.Read(buf); e != nil {
					return
				}
				s.send(map[string]any{"type": "keyframe", "reason": "rtcp", "quality": "hd"})
			}
		}()
	}
	input, e := pc.CreateDataChannel("input", nil)
	if e != nil {
		pc.Close()
		http.Error(w, "input unavailable", 503)
		return
	}
	v.input = input
	input.OnMessage(func(m webrtc.DataChannelMessage) {
		if len(m.Data) <= 65536 {
			s.send(map[string]any{"type": "input", "viewerId": v.id, "data": base64.StdEncoding.EncodeToString(m.Data)})
		}
	})
	control, e := pc.CreateDataChannel("control", nil)
	if e != nil {
		pc.Close()
		http.Error(w, "control unavailable", 503)
		return
	}
	control.OnOpen(func() {
		control.SendText(`{"type":"intro","phase":"live"}`)
		control.SendText(`{"type":"quality","current":"hd","want":"hd","available":false}`)
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("WebRTC peer state: %s", state.String())
		if state == webrtc.PeerConnectionStateClosed || (state == webrtc.PeerConnectionStateFailed && !v.fallbackReady()) {
			go v.close()
		}
		if state == webrtc.PeerConnectionStateConnected {
			s.send(map[string]any{"type": "keyframe", "reason": "connected", "quality": "hd"})
		}
	})
	offer, e := pc.CreateOffer(nil)
	if e != nil {
		pc.Close()
		http.Error(w, "offer unavailable", 503)
		return
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if pc.SetLocalDescription(offer) != nil {
		pc.Close()
		http.Error(w, "offer unavailable", 503)
		return
	}
	select {
	case <-gathered:
	case <-time.After(5 * time.Second):
		pc.Close()
		http.Error(w, "ICE timeout", 503)
		return
	}
	s.mu.Lock()
	if len(s.viewers) >= 8 || s.closed {
		s.mu.Unlock()
		pc.Close()
		http.Error(w, "viewer limit", 429)
		return
	}
	s.viewers[v.id] = v
	s.mu.Unlock()
	if s.send(map[string]any{"type": "viewer-joined", "viewerId": v.id, "hasInput": options.HasInput, "username": options.Username}) != nil {
		v.close()
		http.Error(w, "source unavailable", 503)
		return
	}
	time.AfterFunc(30*time.Second, func() {
		if pc.ConnectionState() != webrtc.PeerConnectionStateConnected && !v.fallbackReady() {
			v.close()
		}
	})
	log.Print("WebRTC offer ready")
	jsonReply(w, 200, map[string]any{"viewerId": v.id, "sdp": pc.LocalDescription().SDP})
}

func (g *gateway) fallback(w http.ResponseWriter, r *http.Request, v *viewer) {
	conn, e := upgrader.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	v.source.mu.Lock()
	config := append([][]byte(nil), v.source.videoConfig...)
	v.source.mu.Unlock()
	v.fallbackMu.Lock()
	if v.fallback != nil {
		v.fallback.Close()
	}
	v.fallback = conn
	for _, frame := range config {
		conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if conn.WriteMessage(websocket.BinaryMessage, frame) != nil {
			v.fallback = nil
			v.fallbackMu.Unlock()
			conn.Close()
			return
		}
	}
	v.fallbackMu.Unlock()
	// A public fallback usually joins mid-stream. Ask the runtime for an IDR
	// so SPS/PPS and a decodable keyframe arrive immediately instead of waiting
	// for the encoder's next periodic keyframe.
	v.source.send(map[string]any{"type": "keyframe", "reason": "https-fallback", "quality": "hd"})
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case sentAt := <-ticker.C:
				payload, _ := json.Marshal(map[string]any{
					"type":   "fallback-ping",
					"sentAt": sentAt.UnixMilli(),
				})
				if !v.sendFallbackMessage(websocket.TextMessage, payload, conn) {
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer func() {
		v.fallbackMu.Lock()
		if v.fallback == conn {
			v.fallback = nil
		}
		v.fallbackMu.Unlock()
		conn.Close()
	}()
	log.Print("HTTPS media fallback connected")
	conn.SetReadLimit(64 << 10)
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if kind == websocket.TextMessage {
			var control struct {
				Type   string `json:"type"`
				SentAt int64  `json:"sentAt"`
			}
			if json.Unmarshal(data, &control) == nil && control.Type == "fallback-pong" {
				sentAt := time.UnixMilli(control.SentAt)
				latency := time.Since(sentAt)
				if latency >= 0 && latency <= 30*time.Second {
					payload, _ := json.Marshal(map[string]any{
						"type":      "fallback-latency",
						"latencyMs": float64(latency.Microseconds()) / 1000,
					})
					v.sendFallbackMessage(websocket.TextMessage, payload, conn)
				}
				continue
			}
		}
		if v.hasInput && (kind == websocket.TextMessage || kind == websocket.BinaryMessage) && len(data) <= 65536 {
			if v.source.send(map[string]any{"type": "input", "viewerId": v.id, "data": base64.StdEncoding.EncodeToString(data)}) != nil {
				return
			}
		}
	}
}

func (g *gateway) signal(w http.ResponseWriter, r *http.Request, v *viewer) {
	v.signalMu.Lock()
	if v.signalUsed {
		v.signalMu.Unlock()
		http.Error(w, "signal already used", 409)
		return
	}
	v.signalUsed = true
	v.signalMu.Unlock()
	conn, e := upgrader.Upgrade(w, r, nil)
	if e != nil {
		v.close()
		return
	}
	defer conn.Close()
	log.Print("WebRTC signaling connected")
	conn.SetReadLimit(128 << 10)
	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	conn.WriteJSON(map[string]any{"type": "done"})
	answerSet := false
	pending := []webrtc.ICECandidateInit{}
	for {
		var message struct {
			Type, SDP string
			Candidate webrtc.ICECandidateInit
		}
		if conn.ReadJSON(&message) != nil {
			return
		}
		switch message.Type {
		case "answer":
			if answerSet || pcAnswer(v.pc, message.SDP) != nil {
				v.close()
				return
			}
			answerSet = true
			for _, c := range pending {
				v.pc.AddICECandidate(c)
			}
			pending = nil
		case "candidate":
			if answerSet {
				v.pc.AddICECandidate(message.Candidate)
			} else if len(pending) < 64 {
				pending = append(pending, message.Candidate)
			}
		}
	}
}
func pcAnswer(pc *webrtc.PeerConnection, sdp string) error {
	return pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp})
}
func (g *gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		jsonReply(w, 200, map[string]any{"ok": true, "adapter": "native-retrogaming-lan"})
		return
	}
	if r.URL.Path == "/internal/stream/ws" && r.Method == "GET" {
		g.child(w, r)
		return
	}
	// Local runtime launch announcements carry no media; this adapter has no intro renderer.
	if strings.HasPrefix(r.URL.Path, "/internal/launches/") && r.Method == "POST" {
		if !g.authorized(r) {
			http.Error(w, "unauthorized", 401)
			return
		}
		io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, 65536))
		jsonReply(w, 200, map[string]any{"success": true})
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 6 || strings.Join(parts[:4], "/") != "api/v1/gateway-webrtc/streams" || !validID.MatchString(parts[4]) {
		http.NotFound(w, r)
		return
	}
	g.mu.Lock()
	s := g.sources[parts[4]]
	g.mu.Unlock()
	if s == nil {
		http.Error(w, "source unavailable", 404)
		return
	}
	if len(parts) == 6 && parts[5] == "viewer-offer" && r.Method == "POST" {
		g.offer(w, r, s)
		return
	}
	id := r.URL.Query().Get("viewerId")
	if len(parts) == 7 && parts[5] == "viewer" {
		id = parts[6]
	}
	if !validID.MatchString(id) {
		http.Error(w, "invalid viewer", 400)
		return
	}
	s.mu.Lock()
	v := s.viewers[id]
	s.mu.Unlock()
	if v == nil {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 6 && parts[5] == "viewer-ws" && r.Method == "GET" {
		g.signal(w, r, v)
		return
	}
	if len(parts) == 6 && parts[5] == "viewer-fallback" && r.Method == "GET" {
		g.fallback(w, r, v)
		return
	}
	if len(parts) == 7 && parts[5] == "viewer" && r.Method == "DELETE" {
		v.close()
		jsonReply(w, 200, map[string]any{"success": true})
		return
	}
	http.NotFound(w, r)
}
func main() {
	secret := os.Getenv("GAMEFLEX_INTERNAL_SECRET")
	if len(secret) < 16 {
		log.Fatal("Internal secret required")
	}
	engine := webrtc.SettingEngine{}
	if e := engine.SetEphemeralUDPPortRange(9201, 9300); e != nil {
		log.Fatal("Invalid UDP range")
	}
	engine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	if name := os.Getenv("GAMEFLEX_ICE_INTERFACE"); name != "" {
		engine.SetInterfaceFilter(func(s string) bool { return s == name })
	}
	// Host candidates keep LAN play direct. STUN adds a server-reflexive
	// candidate so the same session can cross a normal home NAT from the
	// published HTTPS URL (for example an iPhone on 5G). TURN remains an
	// optional operator setting for symmetric NATs.
	engine.SetIPFilter(func(ip net.IP) bool { return ip.IsPrivate() && !ip.IsLoopback() })
	g := &gateway{sources: map[string]*source{}, api: webrtc.NewAPI(webrtc.WithSettingEngine(engine)), peerConfig: publicICEConfiguration(os.Getenv("GAMEFLEX_STUN_URL")), secret: secret, offerSlots: make(chan struct{}, 8)}
	server := http.Server{Addr: "127.0.0.1:10202", Handler: g, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	log.Print("Native LAN retrogaming gateway ready")
	if server.ListenAndServe() != nil {
		log.Fatal("Gateway stopped")
	}
}
