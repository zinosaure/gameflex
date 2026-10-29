package retrogaming

// RunChild is the entry point for a per-session child process, spawned by
// supervisor.go's Spawn — see stream_session.go's top-of-file doc and
// HandleStreamWS for the overall parent/child split. Invoked from
// cmd/server/main.go when os.Args indicate child mode: this re-execs the
// SAME server binary rather than needing a separate build/entrypoint, since
// it only needs the retrogaming package's own code, not the rest of the
// server's routes.
import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gameflex/gatewayclient"
	"gameflex/libretro/engine"
	"gameflex/retrogaming/internal/storage_client"
	"gameflex/sessionstatus"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// childSessionID is this child's own sessionID (the same one
// supervisor.Spawn minted and passed as this process's argv, see RunChild
// below) — echoed back to the owning browser in the "started" message
// (sendGatewayStarted) so the frontend can report it as part of its own
// presence ("I'm playing X, here's the session to invite people to" — see
// python/src/app/presence/registry.py's go_session_id / Phase 5's join
// invitations). A package-level var (not a field threaded through
// everywhere) since it's fixed for this child process's entire lifetime,
// same reasoning as activeChildSession in stream_session.go.
var childSessionID string

// childGatewayClient is the shared internal transport to gatewaywebrtc (see
// docs/WEBRTC_GATEWAY.md) — the child's ONLY media path since its in-process
// WebRTC left (#257, 2026-10-15). Dialed at boot, BEFORE any "start" arrives:
// the browser's viewer-offer hits the gateway first and the stream must
// already exist.
var childGatewayClient *gatewayclient.Client

// RunChild adopts the listener the parent pre-bound and handed down via
// cmd.ExtraFiles (always fd 3 — the first of ExtraFiles, since fd 0-2 are
// stdin/stdout/stderr), and serves exactly one route, /session, which is
// serveEmulationSession unchanged from when it ran directly in the shared
// server process. The actual consoleID/reference/core arrive over the
// "start" message the parent relays through once connected (see
// HandleStreamWS).
//
// libretro.InitEnvPaths is NOT called here (unlike before the per-user save
// namespacing change) — it needs the "start" message's username, which
// isn't known yet at process boot, so handleStart calls it itself instead
// once that arrives.
func RunChild(sessionID string) {
	childSessionID = sessionID
	// The parent (the kernel) resolved the section paths and passed them
	// down as env vars — the env vars are this child's whole world (see
	// storage_client).
	storagePaths = storage_client.New(storageRootFromEnv())
	storagePaths.AdoptEnv()

	// The browsers' WebRTC lives in gatewaywebrtc (GAMEFLEX_GATEWAY_URL): this
	// process pushes its encoded frames and receives browser input over the
	// one shared transport (src/microservices/shared/gatewayclient), and its
	// WebSockets carry the control plane only. There is no other path — the
	// in-process WebRTC fallback left with #257, the pages had long stopped
	// answering it — so, like streamex, the child does not start without the
	// gateway rather than run a game nobody can see.
	gatewayURL := os.Getenv("GAMEFLEX_GATEWAY_URL")
	if gatewayURL == "" {
		log.Fatalf("[retrogaming-child %s] GAMEFLEX_GATEWAY_URL is not set — the child streams through gatewaywebrtc only", sessionID)
	}
	handlers := gatewayclient.Handlers{
		OnInput: func(viewerID string, data []byte) {
			activeChildSessionMu.Lock()
			sess := activeChildSession
			activeChildSessionMu.Unlock()
			if sess != nil {
				sess.handleGatewayInput(viewerID, data)
			}
		},
		OnViewerLeft: func(viewerID string) {
			activeChildSessionMu.Lock()
			sess := activeChildSession
			activeChildSessionMu.Unlock()
			if sess != nil {
				sess.handleGatewayViewerLeft(viewerID)
			}
		},
	}
	client, err := gatewayclient.Dial(gatewayURL, sessionID, "retro",
		os.Getenv("GAMEFLEX_INTERNAL_SECRET"), gatewayclient.MediaInfo{}, handlers)
	if err != nil {
		log.Fatalf("[retrogaming-child %s] gatewaywebrtc unreachable: %v", sessionID, err)
	}
	childGatewayClient = client
	log.Printf("[retrogaming-child %s] attached to gatewaywebrtc", sessionID)

	h := NewHandler()

	// Listener: GAMEFLEX_CHILD_PORT makes this process bind its own TCP port
	// (published by the session container), exactly like streamex. The kernel
	// always sets it (docker.go). The other bootstrap path — a pre-bound
	// socket on fd 3, handed down by the supervisor of the long-lived
	// retrogaming service — left with that supervisor (chantier 18).
	port := os.Getenv("GAMEFLEX_CHILD_PORT")
	if port == "" {
		log.Fatalf("[retrogaming-child %s] GAMEFLEX_CHILD_PORT is not set", sessionID)
	}
	ln, err := net.Listen("tcp", "0.0.0.0:"+port)
	if err != nil {
		log.Fatalf("[retrogaming-child %s] failed to bind 0.0.0.0:%s: %v", sessionID, port, err)
	}
	log.Printf("[retrogaming-child %s] listening on 0.0.0.0:%s (container mode)", sessionID, port)

	// Graceful shutdown on SIGTERM/SIGINT — sent by a clean container stop.
	// Running the existing
	// shutdown() logic (rather than a bare process exit) matters here
	// specifically to stop the ffmpeg encoder subprocesses cleanly —
	// otherwise they'd be orphaned (reparented, same as this child would be
	// if ITS parent died) rather than exiting with their pipe closed.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		log.Printf("[retrogaming-child %s] received shutdown signal", sessionID)
		activeChildSessionMu.Lock()
		sess := activeChildSession
		activeChildSessionMu.Unlock()
		if sess != nil {
			sess.shutdown()
		}
		os.Exit(0)
	}()

	mux := http.NewServeMux()
	// chantier-4 — the new session authority, served BESIDE the three routes
	// below rather than in their place. The v1 model is what people are
	// playing on; an unvalidated path must not be able to break it.
	if v4Enabled() {
		startV4Host(sessionID)
		registerV4Routes(mux)
	} else {
		log.Printf("[retrogaming-child %s] chantier-4 host disabled (no owner/JWT secret in env)", sessionID)
	}

	mux.HandleFunc("/session", h.serveEmulationSession)
	mux.HandleFunc("/session/join", serveJoinerSession)
	mux.HandleFunc("/session/spectate", serveSpectatorSession)
	// Container-mode liveness/telemetry: the sessions manager polls
	// /health while the container boots and /status for the watchdog
	// (players/spectators) and the performance panel (CPU/RAM).
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/status", serveSelfStatus)

	if err := http.Serve(ln, mux); err != nil {
		log.Printf("[retrogaming-child %s] serve error: %v", sessionID, err)
	}
}

// selfStatusMu guards the /proc sampling below — one sample pair at a time,
// and the delta needs the PREVIOUS sample, so concurrent polls would race.
var selfStatusMu sync.Mutex

var selfLastCPUAt time.Time
var selfLastUtime, selfLastStime int64

// serveSelfStatus reports THIS process's own CPU/RAM (container mode: the
// child is PID 1 of the session container) plus the live player/spectator
// counts of its one emulationSession — same shape the sessions manager's
// watchdog expects (see docker.ContainerStatus for the external consoles).
func serveSelfStatus(w http.ResponseWriter, r *http.Request) {
	selfStatusMu.Lock()
	defer selfStatusMu.Unlock()

	pid := os.Getpid()
	now := time.Now()
	utime, stime, rss := readProcTimes(pid)

	status := map[string]any{
		"pid":       pid,
		"rss_bytes": rss,
	}
	if !selfLastCPUAt.IsZero() {
		if elapsed := now.Sub(selfLastCPUAt).Seconds(); elapsed > 0 {
			// Same semantics as supervisor.Status: fraction of one core,
			// can exceed 100 on multicore.
			deltaTicks := (utime + stime) - (selfLastUtime + selfLastStime)
			status["cpu_percent"] = math.Round(float64(deltaTicks)/elapsed*10) / 10
		}
	}
	selfLastCPUAt, selfLastUtime, selfLastStime = now, utime, stime

	players, spectators := activeChildCounts()
	status["players"], status["spectators"] = players, spectators
	status["fps"] = activeChildFPS()
	status["gpu"] = readGPUStatus()
	status["emulator"] = activeChildCore()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// activeChildFPS is the emulation loop's own latest sampled frame rate — the
// same number its 5-second log reports (see stream_session.go's loop).
func activeChildFPS() float64 {
	activeChildSessionMu.Lock()
	sess := activeChildSession
	activeChildSessionMu.Unlock()
	if sess == nil {
		return 0
	}

	if eng := sess.currentEngine(); eng != nil {
		return eng.FPS()
	}

	return 0
}

// activeChildCore — le cœur libretro de la partie et sa version, tels qu'il
// se déclare, pour le panneau Performance ; vide avant son chargement.
func activeChildCore() string {
	activeChildSessionMu.Lock()
	sess := activeChildSession
	activeChildSessionMu.Unlock()
	if sess == nil {
		return ""
	}
	if eng := sess.currentEngine(); eng != nil {
		return eng.CoreName()
	}

	return ""
}

// readGPUStatus — l'occupation et la mémoire vidéo du GPU que ce conteneur
// a reçu (sessionstatus.GPUUsage, la même source que le /status de
// streamex). Absentes quand le pilote ne les dit pas (le pilote propriétaire
// de NVIDIA compris), et la page montre « — ».
func readGPUStatus() map[string]any {
	return sessionstatus.GPUUsage()
}

// activeChildCounts counts the owner (once attached, always player 1) and
// the live joiner/spectator connections of this process's session.
func activeChildCounts() (players, spectators int) {
	activeChildSessionMu.Lock()
	sess := activeChildSession
	activeChildSessionMu.Unlock()
	if sess == nil {
		return 0, 0
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()
	players = 1 // owner, port 0 — reserved from handleStart
	for _, joiner := range sess.joiners {
		if joiner != nil {
			players++
		}
	}
	spectators = len(sess.spectators)
	return players, spectators
}

// serveJoinerSession attaches another player to the one emulationSession
// this child process is running (activeChildSession — see stream_session.go's
// doc), on whichever libretro controller port is free (port 1..maxPlayers-1 —
// port 0 is always the owner). The browser's viewer attaches to gatewaywebrtc
// directly; this WebSocket is the control plane only, and the viewerId the
// relay passes along maps the gateway's input to this port. Unlike
// serveEmulationSession, this doesn't drive a "start" handshake — the game is
// already running.
func serveJoinerSession(w http.ResponseWriter, r *http.Request) {
	activeChildSessionMu.Lock()
	sess := activeChildSession
	activeChildSessionMu.Unlock()
	if sess == nil {
		http.Error(w, "no active session", http.StatusNotFound)
		return
	}

	// QUI OCCUPE CE PORT — le nom que le JETON porte, vérifié ici. Il servait
	// aux annonces « playerleft »/« playerjoined » et à la passation de
	// l'initiateur (ownerDisconnected), et venait d'un paramètre que le
	// relais écrivait : le conteneur écoutant sur le réseau local, n'importe
	// qui pouvait s'y présenter sous n'importe quel nom (relevé le
	// 2026-10-08). Sans hôte v4 (pas de secret), on retombe sur l'ancien
	// paramètre.
	username := r.URL.Query().Get("username")
	if v4 != nil {
		verified, ok := v4.Identify(r)
		if !ok {
			http.Error(w, "invalid or missing token", http.StatusUnauthorized)

			return
		}
		username = verified
	}
	if username == "" {
		username = "anonymous"
	}

	// CE POINTEUR EST NOTRE IDENTITÉ, et il sert jusqu'au ménage final.
	//
	// Le créneau peut être vidé sous nos pieds par le chemin passerelle (« ce
	// viewer est parti »), puis REPRIS par un autre joiner. Comparer ce
	// pointeur — et pas seulement « non nil » — est ce qui distingue « mon
	// créneau » de « le créneau de quelqu'un d'autre » (voir
	// `releaseJoinerSlot`, stream_session.go).
	mine := &joinerConn{username: username}

	// Le verrou est pris dans une closure à `defer` : une section courte, et
	// surtout impossible à laisser tenue si quoi que ce soit panique dedans.
	// C'est exactement ce qui a coûté 16 h de session fantôme le 2026-09-19.
	port := func() int {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		for i, j := range sess.joiners {
			if j == nil {
				// Réserver le créneau IMMÉDIATEMENT pour qu'une seconde
				// demande concurrente ne puisse pas passer le même test.
				sess.joiners[i] = mine
				return i + 1 // joiners[i] est le port i+1 — le port 0 est l'owner
			}
		}
		return -1
	}()
	if port == -1 {
		http.Error(w, "session is full", http.StatusConflict)
		return
	}

	clearJoinerSlot := func() { sess.releaseJoinerSlot(port, mine) }

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[retrogaming-stream] joiner WebSocket upgrade error: %v", err)
		clearJoinerSlot()
		return
	}
	defer ws.Close()

	// gorilla/websocket forbids concurrent calls to WriteMessage on the same
	// *Conn — sendToJoiner is called from multiple goroutines (the session's
	// broadcasts, the heartbeat). Without this mutex, two writes close
	// together corrupt gorilla's internal frame writer and panic ("concurrent
	// write to websocket connection") — confirmed via a real crash log, back
	// when pion's ICE candidates were the concurrent writers: the panic took
	// down this whole child process. Same pattern as the owner's own
	// connection (see emulationSession.sendMessage/wsMu in stream_session.go).
	var wsWriteMu sync.Mutex
	sendToJoiner := func(msg streamMessage) {
		data, _ := json.Marshal(msg)
		wsWriteMu.Lock()
		defer wsWriteMu.Unlock()
		ws.WriteMessage(websocket.TextMessage, data)
	}

	// Ghost-session fix: unlike serveEmulationSession's WS (relayed through
	// relayWebSocket, which already pings/enforces a deadline), this
	// connection had no heartbeat at all until now — a joiner whose browser
	// vanishes without a clean close left this loop's ReadMessage blocked
	// forever, holding the port slot open indefinitely. See startHeartbeat's
	// doc for the full mechanism.
	stopHeartbeat := startHeartbeat(ws, &wsWriteMu)
	defer stopHeartbeat()

	// The browser's viewer attaches to gatewaywebrtc directly; viewerId
	// (relayed as a query param by the sessions service) maps the gateway's
	// input to this port.
	viewerID := r.URL.Query().Get("viewerId")
	log.Printf("[retrogaming-stream] joiner attached via gateway (port %d, viewer %s)", port, viewerID)

	// LE CRÉNEAU EST-IL TOUJOURS LE NÔTRE ? Le chemin passerelle a pu le
	// vider depuis la réservation (« ce viewer est parti »). Écrire dans un
	// `joinerConn` que plus personne ne référence ferait tourner toute la
	// suite pour un port qu'on n'occupe plus.
	stillOurs := func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		if sess.joiners[port-1] != mine {
			return false
		}
		mine.ViewerID = viewerID
		if viewerID != "" {
			sess.viewerPorts[viewerID] = port
		}
		return true
	}()
	if !stillOurs {
		log.Printf("[retrogaming-stream] joiner slot %d reprise entre-temps — on se retire", port)
		return
	}

	// Ack with the same "started" message the owner gets: it is what
	// triggers the frontend's gateway signaling. Without it a joiner sat on
	// "Connecting…" forever (live bug, cast receiver, 2026-08-20).
	sendToJoiner(streamMessage{Type: "started", SessionID: childSessionID})

	// Best-effort — see the identical comment on the disconnect side below.
	// Only meaningful as a genuine "reconnect" signal for a port that was
	// previously vacated (RETRO_DEVICE_JOYPAD is already the state every
	// port starts in, set once in handleStart); harmless to call again if
	// this core doesn't care either way.
	sess.setControllerType(port, engine.DeviceJoypad)
	sess.registerBroadcastTarget(fmt.Sprintf("joiner-%d", port), username, sendToJoiner)
	sess.broadcast(streamMessage{Type: "playerjoined", Username: username, Port: port})

readLoop:
	for {
		_, msgBytes, err := ws.ReadMessage()
		if err != nil {
			break readLoop
		}
		var msg streamMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}
		if msg.Type == "visibility" {
			sess.updateVisibility(fmt.Sprintf("joiner-%d", port), msg.Visible)
		}
	}

	// Le ménage passe par le MÊME libérateur gardé que les chemins d'échec :
	// une seule implémentation, qui vérifie que le créneau est encore le nôtre
	// et remet l'état d'entrée à zéro. C'est ici que le déréférencement sans
	// garde a paniqué le 2026-09-19.
	sess.releaseJoinerSlot(port, mine)
	log.Printf("[retrogaming-stream] joiner disconnected (port %d)", port)

	// Best-effort — some cores only read a port's device type once, before
	// Init() (confirmed in this exact bridge, see SetControllerPortDevice's
	// call in handleStart), and silently ignore a later change; for those
	// this is a no-op, and the existing input-zeroing above (guaranteed
	// everywhere) is what actually stops a stuck input. For a core that DOES
	// respond, the character just stops responding rather than continuing
	// to render an idle-but-still-plugged-in controller.
	sess.setControllerType(port, engine.DeviceNone)
	sess.unregisterBroadcastTarget(fmt.Sprintf("joiner-%d", port))
	sess.broadcast(streamMessage{Type: "playerleft", Username: username, Port: port})
	// Rien d'autre : la fin de la partie appartient à la file (P4).
}

func randomSpectatorID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// serveSpectatorSession attaches a watch-only connection to the one
// emulationSession this child process is running — unlimited, no controller
// port: the gateway's input is routed by viewerId, and a spectator's viewer
// is mapped to none, so it has no structural way to act on the game, not
// just a UI convention. The browser's viewer attaches to gatewaywebrtc
// directly; this WebSocket carries requestslot/visibility and the session's
// broadcasts — see spectatorConn's doc (stream_session.go) for why there's no
// cap here, unlike the joiner slots.
func serveSpectatorSession(w http.ResponseWriter, r *http.Request) {
	activeChildSessionMu.Lock()
	sess := activeChildSession
	activeChildSessionMu.Unlock()
	if sess == nil {
		http.Error(w, "no active session", http.StatusNotFound)
		return
	}

	id, err := randomSpectatorID()
	if err != nil {
		http.Error(w, "failed to allocate spectator id", http.StatusInternalServerError)
		return
	}

	// Le nom d'un spectateur — celui que le JETON porte quand il y en a un,
	// vérifié ici ; sinon « anonymous » : regarder ne demande pas de compte.
	// Il ne venait que d'un paramètre écrit par le relais, que n'importe qui
	// sur le réseau local pouvait écrire aussi (relevé le 2026-10-08).
	username := "anonymous"
	if v4 != nil {
		if verified, ok := v4.Identify(r); ok {
			username = verified
		}
	} else if named := r.URL.Query().Get("username"); named != "" {
		username = named
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[retrogaming-stream] spectator WebSocket upgrade error: %v", err)
		return
	}
	defer ws.Close()

	// Same concurrent-write guard as serveJoinerSession's sendToJoiner —
	// see its doc for the real crash this pattern fixes.
	var wsWriteMu sync.Mutex
	sendToSpectator := func(msg streamMessage) {
		data, _ := json.Marshal(msg)
		wsWriteMu.Lock()
		defer wsWriteMu.Unlock()
		ws.WriteMessage(websocket.TextMessage, data)
	}

	// Ghost-session fix — see the identical comment in serveJoinerSession
	// and startHeartbeat's doc. A watch-only connection has no player slot
	// to leak, but it would otherwise stay counted forever for a browser that
	// never sent a clean close.
	stopHeartbeat := startHeartbeat(ws, &wsWriteMu)
	defer stopHeartbeat()

	// Same "started" ack as the joiner (see its doc): without it the
	// spectator's frontend never runs gateway signaling and sits on
	// "Connecting…" forever (live bug, 2026-08-20).
	sendToSpectator(streamMessage{Type: "started", SessionID: childSessionID})

	sess.spectatorsMu.Lock()
	sess.spectators[id] = &spectatorConn{}
	sess.spectatorsMu.Unlock()
	sess.registerBroadcastTarget("spectator-"+id, username, sendToSpectator)
	defer sess.unregisterBroadcastTarget("spectator-" + id)

	for {
		_, msgBytes, err := ws.ReadMessage()
		if err != nil {
			break
		}
		var msg streamMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}
		if msg.Type == "visibility" {
			sess.updateVisibility("spectator-"+id, msg.Visible)
		}
	}

	sess.spectatorsMu.Lock()
	delete(sess.spectators, id)
	sess.spectatorsMu.Unlock()
	log.Printf("[retrogaming-stream] spectator disconnected")
}
