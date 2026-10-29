// Retro session WS relay — the sessions service is the front door for the
// spawned retrogaming containers, exactly like the old retrogaming service
// was for its child processes: read the browser's first message
// (start/join/spectate), dial the session container's own /session route
// through its network alias, then pump frames both ways.
//
// A PIPE (P4, 2026-10-09). It authenticates, forwards the browser's token so
// the child judges for itself, builds the owner's `start` from the record —
// and decides nothing else. It used to keep its own copy of the players: pads
// and heartbeat targets pushed to the pool, a "session is full" read off the
// old counter, a downgrade for players busy elsewhere, and the end of the
// session whenever the owner's connection closed. The child's queue answers
// all of that now; a second answer next to it is how the bugs came.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/manager"
)

// retroStreamMessage mirrors the old retrogaming streamMessage shape — only
// the fields the RELAY reads (type/sessionId/username) plus what it
// WRITES (error, and the start message it builds from the record).
type retroStreamMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	Username  string `json:"username"`
	Error     string `json:"error,omitempty"`

	// ViewerID — Phase 2: the browser's gatewaywebrtc viewer id, forwarded
	// so the child can map gateway input to a controller port (start: inside
	// the reconstructed start message; join: as a query param on the dial).
	ViewerID string `json:"viewerId,omitempty"`

	ConsoleID string `json:"consoleId,omitempty"`
	Reference string `json:"reference,omitempty"`
	Title     string `json:"title,omitempty"`
	Core      string `json:"core,omitempty"`
	Bios      string `json:"bios,omitempty"`
}

// retroUpgrader mirrors the old service's permissive origin check.
var retroUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// handleRetroStreamWS is the old HandleStreamWS, relaying to a CONTAINER
// instead of a child process — the container is dialed through its network
// alias (retro-<sessionId>) on skinoapp_network, no port lookup needed.
func (server *Server) handleRetroStreamWS(w http.ResponseWriter, r *http.Request) {
	ws, err := retroUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("retro stream upgrade failed", "error", err)
		return
	}
	defer ws.Close()

	_, firstMsgBytes, err := ws.ReadMessage()
	if err != nil {
		return
	}
	var firstMsg retroStreamMessage
	if err := json.Unmarshal(firstMsgBytes, &firstMsg); err != nil {
		slog.Warn("retro stream invalid first message", "error", err)
		return
	}

	// Same connection log the old service had — the only way to tell
	// whether a page reached the relay at all when diagnosing "la session
	// s'est arrêtée sans raison".
	slog.Info("retro stream client connected",
		"type", firstMsg.Type,
		"session_id", firstMsg.SessionID,
		"claimed_username", firstMsg.Username,
		"remote", r.RemoteAddr)

	// Authenticate BEFORE trusting anything the client claims about who it
	// is or what role it wants. This used to be entirely unchecked: any
	// client could send {type:"start", sessionId, username} and be granted
	// full owner control of ANY session, regardless of whose it actually
	// was — a real session-hijack hole, not a theoretical one. The token
	// travels on ?token= (a browser can't set a header on a WS handshake,
	// same reasoning as the facade WS's own auth.ParseToken call,
	// internal/api/api.go's handleWebSocket) — the frontend already sends
	// it (use-retrogaming-streaming-connection.js), it was simply never
	// read here.
	var authedUsername string
	if tok := r.URL.Query().Get("token"); tok != "" {
		if claims, err := auth.ParseToken(server.config.JWTSecret, tok); err == nil {
			// Un compte retiré n'est plus personne (chantier 20).
			if _, admitted := auth.Admit(claims); admitted {
				authedUsername = claims.Username
			}
		}
	}

	var rec managerRetroRecord = server.managerLookupRetro(firstMsg.SessionID)
	if !rec.found {
		_ = ws.WriteMessage(websocket.TextMessage, mustMarshalRetro(retroStreamMessage{Type: "error", Error: "session not found"}))
		return
	}

	// The granted identity/role — derived from the verified token and the
	// session record, NOT from firstMsg.Username/Type. Defaults to
	// anonymous spectate (the one role that never required a login);
	// start/join below either upgrade it or reject outright.
	username := "anonymous"
	if authedUsername != "" {
		username = authedUsername
	}

	childPath := "/session"

	switch firstMsg.Type {
	case "start":
		// "start" is the OWNER's connection: it saves, loads, pauses and
		// ends the game. A verified identity is required here; WHICH identity
		// may own the session is the CHILD's call (chantier 4) — it verifies
		// the token forwarded below and checks it against its own queue. The
		// owner never changes: lending the pad keeps their place (#274).
		//
		// The kernel used to check its own copy instead — the spawn owner, or
		// for a child-managed session nothing at all: right after a handover
		// the new holder dials as owner while the copy still names the
		// previous one ("not authorized to own this session" for a handover
		// that had just succeeded, 2026-08-25), so the check was dropped for
		// those sessions — and with it, any signed-in account could take the
		// owner's connection (found 2026-10-08). The child, which knows,
		// decides now.
		if authedUsername == "" {
			_ = ws.WriteMessage(websocket.TextMessage, mustMarshalRetro(retroStreamMessage{Type: "error", Error: "not authorized to own this session"}))
			return
		}
	case "join":
		if authedUsername == "" {
			_ = ws.WriteMessage(websocket.TextMessage, mustMarshalRetro(retroStreamMessage{Type: "error", Error: "must be logged in to join"}))
			return
		}
		// TOUJOURS LE NOM AUTHENTIFIÉ. Un « join » marqué `guest` gardait le
		// nom que le client envoyait : n'importe quel utilisateur connecté
		// prenait des places sous des noms inventés (relevé le 2026-09-15).
		// La manette locale supplémentaire qui s'en servait a disparu le
		// 2026-09-08 ; les manettes invitées (#125) auront leur propre modèle.
		username = authedUsername
		// No "already playing" downgrade and no "session is full" here: a
		// seat is the queue's to give — it asks the kernel before giving one
		// (P1) — and this connection is only the joiner's transport.
		// The child names each joiner port after its player (playerleft/
		// playerjoined) — relayed via the dial URL.
		childPath = "/session/join?username=" + url.QueryEscape(username)
		if firstMsg.ViewerID != "" {
			childPath += "&viewerId=" + url.QueryEscape(firstMsg.ViewerID)
		}
	case "spectate":
		childPath = "/session/spectate?username=" + url.QueryEscape(username)
	default:
		_ = ws.WriteMessage(websocket.TextMessage, mustMarshalRetro(retroStreamMessage{Type: "error", Error: "expected a start, join, or spectate message"}))
		return
	}

	// Dial by the container's published host port directly (PublicHost:port,
	// resolved from the record) — never by container IP nor DNS alias: the
	// embedded DNS SERVFAILs right after creation and an alias port no
	// longer tracks the real allocated one (see GAMEFLEX_JOURNAL.md,
	// 2026-08-18). The retry loop absorbs the child's http.Serve warm-up.
	//
	// Patient, not just ten quick tries: the FIRST connection a fresh
	// session sees is usually a WATCHER (everybody arrives watching, v4),
	// and the child refuses /session/spectate and /session/join until the
	// emulation exists — which happens when the owner's "start" dial (a
	// parallel connection) boots it, seconds later. Failing the page after
	// one second turned that normal ordering into a fatal error: the page
	// told the player "failed to connect to session" and the game never
	// started (reported live 2026-08-29). So the dial keeps trying while
	// the browser connection lives — the loop ends on the first success,
	// and the browser closing drops the handler below it anyway.
	var childConn *websocket.Conn
	dialTarget := fmt.Sprintf("%s:%d", server.config.PublicHost, rec.record.Port)
	// The URL as logged: it names the path, never the token.
	dialURL := "ws://" + dialTarget + childPath
	// THE BROWSER'S TOKEN GOES TO THE CHILD, which judges who opens which of
	// its doors (chantier 4): the owner's socket only for the owner its own
	// queue names, a joiner under the name the token carries. On the
	// container's own dial only — never into a log line.
	dialWithToken := dialURL
	if token := r.URL.Query().Get("token"); token != "" {
		separator := "?"
		if strings.Contains(childPath, "?") {
			separator = "&"
		}
		dialWithToken += separator + "token=" + url.QueryEscape(token)
	}
	const fastAttempts = 10
	const maxAttempts = 120
	for attempt := 0; attempt < maxAttempts && childConn == nil; attempt++ {
		if attempt > 0 {
			delay := 100 * time.Millisecond
			if attempt >= fastAttempts {
				delay = 450 * time.Millisecond
			}
			time.Sleep(delay)
		}
		var answer *http.Response
		childConn, answer, err = websocket.DefaultDialer.Dial(dialWithToken, nil)
		// A REFUSAL IS AN ANSWER, not a child still warming up: retrying it
		// for fifty seconds would only delay the same "no".
		if answer != nil && (answer.StatusCode == http.StatusUnauthorized || answer.StatusCode == http.StatusForbidden) {
			slog.Info("retro dial refused by the child", "session_id", rec.record.SessionID, "url", dialURL, "username", username, "status", answer.StatusCode)
			_ = ws.WriteMessage(websocket.TextMessage, mustMarshalRetro(retroStreamMessage{Type: "error", Error: "not authorized to own this session"}))
			return
		}
		if err != nil {
			// The dialed URL rides the warning: a "bad handshake" says only
			// that the child answered something other than 101 — WHICH path
			// was refused is the part that tells whose bug it is. Log the
			// first attempts loudly, then every 30th so a stuck child does
			// not drown the log for a minute.
			if attempt < fastAttempts || attempt%30 == 0 {
				slog.Warn("retro dial failed", "session_id", rec.record.SessionID, "target", dialTarget, "url", dialURL, "attempt", attempt+1, "error", err)
			}
		}
	}
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, mustMarshalRetro(retroStreamMessage{Type: "error", Error: "failed to connect to session"}))
		return
	}
	defer childConn.Close()

	switch {
	case firstMsg.Type == "start":
		// The browser sent a bare sessionId — the child needs the actual
		// console/reference/core/username to load the game, which live on
		// the record set at launch time.
		startMsg := retroStreamMessage{
			Type:      "start",
			ConsoleID: rec.record.ConsoleID,
			Reference: rec.record.Reference,
			Title:     rec.record.Title,
			Core:      rec.record.Core,
			Bios:      rec.record.Bios,
			Username:  rec.record.Owner,
			ViewerID:  firstMsg.ViewerID,
		}
		if err := childConn.WriteMessage(websocket.TextMessage, mustMarshalRetro(startMsg)); err != nil {
			slog.Warn("retro start relay failed", "session_id", rec.record.SessionID, "error", err)
			return
		}
	case firstMsg.Type == "spectate":
		// Spectators are WS presence: one connectionPool entry per watching
		// tab, dropped when this relay ends. Anonymous watchers carry an
		// empty username (null on the wire).
		server.pool.AddSpectator(rec.record.SessionID, username)
		defer server.pool.RemoveSpectator(rec.record.SessionID, username)
	}

	browserDropped, browserErr := retroRelayWebSocket(ws, childConn, nil)

	// Why the relay ended matters for diagnosis (Android TV drops): a clean
	// close (1000/1001) is the page closing the socket on purpose, a 1006 is
	// a vanished/reset socket, and a read-deadline timeout means the browser
	// stopped answering pings (frozen/backgrounded tab).
	closeCode := -1
	var closeErr *websocket.CloseError
	if errors.As(browserErr, &closeErr) {
		closeCode = closeErr.Code
	}
	timeout := false
	var netErr net.Error
	if errors.As(browserErr, &netErr) && netErr.Timeout() {
		timeout = true
	}
	slog.Info("retro stream relay ended",
		"session_id", rec.record.SessionID,
		"type", firstMsg.Type,
		"browser_dropped", browserDropped,
		"browser_read_error", browserErr,
		"browser_close_code", closeCode,
		"browser_read_timeout", timeout)

}

// managerRetroRecord — small wrapper so the relay's lookup can be written
// once (found = false keeps zero-value record usage safe).
type managerRetroRecord struct {
	found  bool
	record manager.RetroSession
}

func (server *Server) managerLookupRetro(sessionID string) managerRetroRecord {
	record, ok := server.manager.LookupRetro(sessionID)
	return managerRetroRecord{found: ok, record: record}
}

// mustMarshalRetro — same reasoning as the old mustMarshal: fixed small
// shapes, Marshal cannot fail.
func mustMarshalRetro(msg retroStreamMessage) []byte {
	data, _ := json.Marshal(msg)
	return data
}

// retroRelayWebSocket — the old relayWebSocket pump verbatim: gorilla
// forbids concurrent writes on one *Conn, the heartbeat pinger shares the
// browser connection, and a dropped browser must actively close the child
// side or the container's session hangs forever. Returns which side dropped
// first (the child pump dying = the container ended the session on its own).
func retroRelayWebSocket(browser, child *websocket.Conn, onChildMessage func([]byte)) (browserDropped bool, browserErr error) {
	var browserWriteMu sync.Mutex
	writeBrowser := func(mt int, data []byte) error {
		browserWriteMu.Lock()
		defer browserWriteMu.Unlock()
		return browser.WriteMessage(mt, data)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			mt, data, err := child.ReadMessage()
			if err != nil {
				return
			}
			if onChildMessage != nil {
				onChildMessage(data)
			}
			if writeBrowser(mt, data) != nil {
				return
			}
		}
	}()

	stopHeartbeat := retroStartHeartbeat(browser, &browserWriteMu)
	defer stopHeartbeat()

	for {
		mt, data, err := browser.ReadMessage()
		if err != nil {
			browserDropped = true
			browserErr = err
			break
		}
		if err := child.WriteMessage(mt, data); err != nil {
			break
		}
	}

	// browserDropped=false means a browser→child write failed first — the
	// child conn was already gone (the container ended the session).
	child.Close()
	<-done
	return browserDropped, browserErr
}

// retroStartHeartbeat — the old startHeartbeat verbatim: 25 s pings (under
// any reverse proxy's idle timeout) + a 60 s read deadline each pong pushes
// back. A browser that vanishes without a clean close makes ReadMessage
// time out instead of blocking forever.
func retroStartHeartbeat(ws *websocket.Conn, writeMu *sync.Mutex) (stop func()) {
	const pongWait = 60 * time.Second
	_ = ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	ticker := time.NewTicker(25 * time.Second)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				writeMu.Lock()
				err := ws.WriteMessage(websocket.PingMessage, nil)
				writeMu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()

	return func() {
		ticker.Stop()
		close(done)
	}
}
