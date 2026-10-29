// Package gatewayclient is the single, shared implementation of the internal
// transport between a session container (retro-gaming, streamex, remoteplay)
// and the centralized gatewaywebrtc service. The container pushes its
// already-encoded frames up this channel and receives browser input (and
// viewer lifecycle events) back down — it holds NO WebRTC code of its own.
//
// Wire protocol (one WebSocket per session, both directions):
//
// Child → gateway, text (on connect):
//
//	{"type":"register","sessionId":"...","platform":"...","media":{"width":..,"height":..,"fps":..,"bitrate":..,"sd":true}}
//
// Child → gateway, binary frames:
//
//	[0]        kind: 0=video (H264 Annex-B), 1=audio (Opus), 2=second video
//	           stream, 540p à 30 (chantier 31)
//	[1:9]      duration in nanoseconds, big-endian uint64
//	[9:]       encoded payload
//
// Gateway → child, text:
//
//	{"type":"registered"}
//	{"type":"input","viewerId":"...","data":"<base64>"}
//	{"type":"viewer-joined","viewerId":"...","hasInput":true|false}
//	{"type":"viewer-left","viewerId":"..."}
//	{"type":"keyframe","reason":"...","quality":"hd"|"sd"}
//	{"type":"sd","on":true|false}   — encoder ou non le second flux
//	{"type":"error","message":"..."}
//
// DialSealed adds the seal (seal.go, chantier 34 P9): register carries the
// child's ephemeral key, registered carries the gateway's proof, and every
// message after that is sealed — the child streams to OUR gateway only.
package gatewayclient

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MediaInfo describes the encoded stream the child will push (informational —
// the gateway doesn't transcode).
type MediaInfo struct {
	Width   int `json:"width"`
	Height  int `json:"height"`
	FPS     int `json:"fps"`
	Bitrate int `json:"bitrate"`
	// SD — la partie sait encoder le second flux, 540p à 30 (chantier 31),
	// et le sien est au-dessus : la passerelle peut le lui demander.
	SD bool `json:"sd,omitempty"`
}

// Handlers receives gateway → child events. All callbacks run on the client's
// read goroutine — keep them fast and non-blocking.
type Handlers struct {
	OnInput        func(viewerID string, data []byte)
	OnViewerJoined func(viewerID, username string, hasInput bool)
	OnViewerLeft   func(viewerID string)
	OnStatus       func(connected bool, err error)
	// OnKeyframe — la passerelle demande une image clé fraîche (chantier 31
	// P0) : un spectateur vient de se connecter, ou un navigateur a perdu
	// des paquets. Un child sans gestionnaire attend sa prochaine image clé
	// régulière (le rétro, toutes les 250 ms). `quality` dit de quel flux :
	// "hd" (le sien) ou "sd" (le second).
	OnKeyframe func(reason, quality string)
	// OnSD — encoder ou non le second flux (chantier 31) : un spectateur le
	// regarde, ou plus personne depuis un moment.
	OnSD func(on bool)
}

const (
	kindVideo   = 0
	kindAudio   = 1
	kindVideoSD = 2

	childWSPath = "/internal/stream/ws"
)

// Client is the shared child-side connection to gatewaywebrtc.
type Client struct {
	url      string
	session  string
	platform string
	secret   string
	media    MediaInfo
	handlers Handlers
	// gatewayKey — la clé de la passerelle à reconnaître (DialSealed) ; nil,
	// la connexion reste en clair (Dial, le rétro).
	gatewayKey *ecdh.PublicKey

	mu   sync.Mutex
	conn *websocket.Conn
	// seal — celui de `conn`, posé avec elle : chaque connexion refait sa
	// poignée de main, donc ses clés et ses compteurs.
	seal     *Seal
	closed   bool
	writeMu  sync.Mutex
	closeCh  chan struct{}
	reconnCh chan struct{}

	// nilConnDrops — les images jetées faute de connexion à la passerelle,
	// dites au journal une fois sur 120 : une coupure s'y lit sans le noyer.
	nilConnDrops int64
}

// Dial connects to gatewayURL (http:// or ws:// form, host[:port]) and
// registers the session. It returns once the first connection is established
// and reconnects in the background after that (exponential backoff capped at
// 10s) until Close.
func Dial(gatewayURL, sessionID, platform, secret string, media MediaInfo, handlers Handlers) (*Client, error) {
	return dial(gatewayURL, sessionID, platform, secret, nil, media, handlers)
}

// DialSealed is Dial for a child that streams to OUR gateway only (streamex,
// Remote Play): gatewayKey is that gateway's public key, and a gateway that
// cannot prove it holds the private one gets nothing — ErrGatewayNotRecognized
// at every attempt, the reconnect loop retrying as for any other failure.
func DialSealed(gatewayURL, sessionID, platform, secret, gatewayKey string, media MediaInfo, handlers Handlers) (*Client, error) {
	key, err := ParseGatewayPublicKey(gatewayKey)
	if err != nil {
		return nil, fmt.Errorf("gatewayclient: gateway key: %w", err)
	}

	return dial(gatewayURL, sessionID, platform, secret, key, media, handlers)
}

func dial(gatewayURL, sessionID, platform, secret string, gatewayKey *ecdh.PublicKey, media MediaInfo, handlers Handlers) (*Client, error) {
	u, err := childWSURL(gatewayURL, sessionID)
	if err != nil {
		return nil, err
	}
	c := &Client{
		url:        u,
		session:    sessionID,
		platform:   platform,
		secret:     secret,
		media:      media,
		handlers:   handlers,
		gatewayKey: gatewayKey,
		closeCh:    make(chan struct{}),
		reconnCh:   make(chan struct{}, 1),
	}
	if err := c.connect(); err != nil {
		return nil, err
	}
	go c.readLoop()
	go c.reconnectLoop()
	return c, nil
}

func childWSURL(gatewayURL, sessionID string) (string, error) {
	u, err := url.Parse(gatewayURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("gatewayclient: invalid gateway URL %q", gatewayURL)
	}
	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}
	return fmt.Sprintf("%s://%s%s?sessionId=%s", wsScheme, u.Host, childWSPath, url.QueryEscape(sessionID)), nil
}

func (c *Client) connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	header := map[string][]string{}
	if c.secret != "" {
		header["X-Internal-Secret"] = []string{c.secret}
	}
	conn, _, err := dialer.Dial(c.url, header)
	if err != nil {
		return fmt.Errorf("gatewayclient: dial: %w", err)
	}

	registration := map[string]any{
		"type":      "register",
		"sessionId": c.session,
		"platform":  c.platform,
		"media":     c.media,
	}
	var finishSeal func(proof string) (*Seal, error)
	if c.gatewayKey != nil {
		partyKey, finish, err := offerSeal(c.gatewayKey, c.session)
		if err != nil {
			conn.Close()
			return err
		}
		registration["seal"] = partyKey
		finishSeal = finish
	}
	register, err := json.Marshal(registration)
	if err != nil {
		conn.Close()
		return err
	}
	if err := conn.WriteMessage(websocket.TextMessage, register); err != nil {
		conn.Close()
		return fmt.Errorf("gatewayclient: register: %w", err)
	}

	// SCELLÉ, LA PREUVE AVANT TOUT : rien ne part vers une passerelle qui
	// n'a pas montré qu'elle est la nôtre. Elle répond « registered » avant
	// tout autre message (son verrou couvre l'inscription).
	var seal *Seal
	if finishSeal != nil {
		seal, err = c.awaitProof(conn, finishSeal)
		if err != nil {
			conn.Close()
			slog.Warn("gatewayclient: gateway refused — not ours", "session_id", c.session, "error", err)
			return err
		}
	}

	// gorilla answers pings automatically; refresh the read deadline on pong
	// so a dead gateway drops the connection and triggers a reconnect.
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	c.mu.Lock()
	c.conn = conn
	c.seal = seal
	wasClosed := c.closed
	c.mu.Unlock()
	if wasClosed {
		conn.Close()
		return nil
	}
	if c.handlers.OnStatus != nil {
		c.handlers.OnStatus(true, nil)
	}

	// The gateway never sends its own pings, and this client's read deadline
	// is only refreshed by PONGs — without a client-side ping the deadline
	// (60s) expires on an otherwise perfectly healthy connection and forces
	// a needless reconnect every minute (seen live: the fake child
	// re-registered on the gateway every ~60s). Pinging here makes the
	// gateway (gorilla's automatic handler) reply with a pong, which the
	// PongHandler above turns into a fresh deadline. WriteControl is
	// concurrency-safe, unlike WriteMessage.
	go c.pingLoop(conn)
	return nil
}

// awaitProof — la réponse à `register`, lue avant de rendre la connexion :
// `registered` et une preuve qui s'ouvre, ou la passerelle n'est pas la nôtre.
func (c *Client) awaitProof(conn *websocket.Conn, finish func(proof string) (*Seal, error)) (*Seal, error) {
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("gatewayclient: registered: %w", err)
	}
	var registered struct {
		Type  string `json:"type"`
		Proof string `json:"proof"`
	}
	if msgType != websocket.TextMessage || json.Unmarshal(data, &registered) != nil ||
		registered.Type != "registered" || registered.Proof == "" {
		return nil, ErrGatewayNotRecognized
	}

	return finish(registered.Proof)
}

func (c *Client) pingLoop(conn *websocket.Conn) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		current := c.conn
		c.mu.Unlock()
		if current != conn {
			return // superseded by a reconnect
		}
		if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
			return
		}
	}
}

func (c *Client) reconnectLoop() {
	backoff := time.Second
	for {
		select {
		case <-c.closeCh:
			return
		case <-c.reconnCh:
		}
		select {
		case <-c.closeCh:
			return
		case <-time.After(backoff):
		}
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return
		}
		if err := c.connect(); err != nil {
			if c.handlers.OnStatus != nil {
				c.handlers.OnStatus(false, err)
			}
			slog.Warn("gatewayclient: reconnect failed, retrying", "session_id", c.session, "in", backoff*2, "error", err)
			backoff = min(backoff*2, 10*time.Second)
			// ON SE RELANCE SOI-MÊME. Le signal ne vient que d'une connexion
			// qui tombe ; sans connexion, plus rien ne tombe, et la boucle
			// attendait pour toujours après une seule tentative ratée — la
			// partie n'avait plus d'image jusqu'à ce qu'on la relance. Vu le
			// 2026-10-25 : la passerelle de la dev redémarrée deux fois en deux
			// secondes, la tentative est tombée entre les deux.
			c.signalReconnect()
			continue
		}
		backoff = time.Second
		go c.readLoop()
	}
}

func (c *Client) readLoop() {
	conn, seal := c.currentSealed()
	if conn == nil {
		return
	}
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			c.drop(conn)
			c.signalReconnect()
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}
		if seal != nil {
			opened, err := seal.OpenText(data)
			if err != nil {
				// Un message qui ne s'ouvre pas ne vient pas de la passerelle
				// reconnue — ou le fil a perdu l'ordre : la connexion ne vaut
				// plus rien, on la refait.
				slog.Warn("gatewayclient: sealed message refused, reconnecting", "session_id", c.session, "error", err)
				c.drop(conn)
				c.signalReconnect()
				return
			}
			data = opened
		}
		var msg struct {
			Type     string `json:"type"`
			ViewerID string `json:"viewerId"`
			Data     string `json:"data"`
			HasInput bool   `json:"hasInput"`
			Username string `json:"username"`
			Message  string `json:"message"`
			Reason   string `json:"reason"`
			Quality  string `json:"quality"`
			On       bool   `json:"on"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "input":
			if c.handlers.OnInput != nil {
				raw, err := base64.StdEncoding.DecodeString(msg.Data)
				if err == nil {
					c.handlers.OnInput(msg.ViewerID, raw)
				}
			}
		case "viewer-joined":
			if c.handlers.OnViewerJoined != nil {
				c.handlers.OnViewerJoined(msg.ViewerID, msg.Username, msg.HasInput)
			}
		case "viewer-left":
			if c.handlers.OnViewerLeft != nil {
				c.handlers.OnViewerLeft(msg.ViewerID)
			}
		case "keyframe":
			if c.handlers.OnKeyframe != nil {
				quality := msg.Quality
				if quality == "" {
					quality = "hd"
				}
				c.handlers.OnKeyframe(msg.Reason, quality)
			}
		case "sd":
			if c.handlers.OnSD != nil {
				c.handlers.OnSD(msg.On)
			}
		case "error":
			slog.Warn("gatewayclient: gateway error", "session_id", c.session, "error", msg.Message)
		}
	}
}

func (c *Client) current() *websocket.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

// currentSealed — la connexion et son scellement, lus ensemble.
func (c *Client) currentSealed() (*websocket.Conn, *Seal) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn, c.seal
}

// drop closes conn only if it's still the current one.
func (c *Client) drop(conn *websocket.Conn) {
	c.mu.Lock()
	if c.conn == conn {
		c.conn = nil
	}
	c.mu.Unlock()
	conn.Close()
}

func (c *Client) signalReconnect() {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return
	}
	select {
	case c.reconnCh <- struct{}{}:
	default:
	}
}

// PushVideo sends one H264 Annex-B encoded frame.
func (c *Client) PushVideo(nal []byte, duration time.Duration) error {
	return c.push(kindVideo, nal, duration)
}

// PushVideoSD sends one image of the second stream, 540p à 30 (chantier 31).
func (c *Client) PushVideoSD(nal []byte, duration time.Duration) error {
	return c.push(kindVideoSD, nal, duration)
}

// PushAudio sends one Opus packet.
func (c *Client) PushAudio(packet []byte, duration time.Duration) error {
	return c.push(kindAudio, packet, duration)
}

func (c *Client) push(kind byte, payload []byte, duration time.Duration) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	conn, seal := c.currentSealed()
	if conn == nil {
		// Not connected — the frame is dropped; the encoders upstream already
		// have their own drop policies, this just stays non-blocking.
		c.nilConnDrops++
		if c.nilConnDrops%120 == 1 {
			slog.Warn("gatewayclient: push dropped (conn nil)", "session_id", c.session, "drops", c.nilConnDrops)
		}
		return nil
	}
	var frame []byte
	if seal != nil {
		frame = seal.SealFrame(kind, duration, payload)
	} else {
		frame = make([]byte, 9+len(payload))
		frame[0] = kind
		binary.BigEndian.PutUint64(frame[1:9], uint64(duration))
		copy(frame[9:], payload)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		// Dead connection: let the read loop notice and reconnect.
		c.drop(conn)
		c.signalReconnect()
		slog.Warn("gatewayclient: push write failed", "session_id", c.session, "error", err)
		return err
	}
	return nil
}

// SendToViewer pushes a downlink message to one viewer's browser (the
// gateway forwards it over that viewer's input data channel). payload is
// any JSON-serializable shape — typically {"type":"rumble",...}.
func (c *Client) SendToViewer(viewerID string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	message, err := json.Marshal(map[string]any{
		"type":     "to-viewer",
		"viewerId": viewerID,
		"data":     base64.StdEncoding.EncodeToString(encoded),
	})
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	conn, seal := c.currentSealed()
	if conn == nil {
		return nil // dropped, like the frames above — non-blocking
	}
	if seal != nil {
		message = seal.SealText(message)
	}
	if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
		c.drop(conn)
		c.signalReconnect()
		return err
	}
	return nil
}

// Close tears the connection down and stops the reconnect loop.
func (c *Client) Close() {
	c.mu.Lock()
	c.closed = true
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
	close(c.closeCh)
}
