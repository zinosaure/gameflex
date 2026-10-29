// Remoteplay data plane — sessions is the front door for the per-session
// stream traffic, exactly like the retro kind: the webpack proxy routes
// /api/v1/remoteplay/webrtc|input|streaming here, and this router resolves the
// session (device id or the child-issued WebRTC id) to its container's
// published host port and reverse-proxies the request. The control plane
// (remoteplay service) keeps devices/PSN/discovery and starts/stops
// sessions through /internal/remoteplay/*.
package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/auth"
)

// remoteplayWebrtcMu guards remoteplayWebrtcToSession: answer/ice calls
// carry ONLY the child-issued WebRTC session id (returned by the child's
// own offer), so the router must remember which device session each
// WebRTC id belongs to. Stored as SESSION id (not port) so a recycled
// allocator port can never misroute.
var remoteplayWebrtcMu sync.Mutex
var remoteplayWebrtcToSession = map[string]string{}

// remoteplaySessionIDFromRequest extracts the session id the way each call
// carries it: path segment, query param, or JSON body field. The router is
// a catch-all, so path ids are parsed from the raw path (the same segments
// the control plane's chi routes declared as {sessionId}/{remotePlayId}).
func remoteplaySessionIDFromRequest(r *http.Request) string {
	if segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/"); len(segments) >= 2 {
		switch segments[0] {
		case "webrtc":
			switch segments[1] {
			case "ice", "latency", "session":
				if len(segments) > 2 {
					return segments[2]
				}
			case "connect":
				// /webrtc/connect/{webrtcId}/{remotePlayId}
				if len(segments) > 3 {
					return segments[3]
				}
			case "viewer":
				// /webrtc/viewer/ice/{sessionId}/{viewerId} or
				// /webrtc/viewer/session/{sessionId}/{viewerId}
				if len(segments) > 3 && (segments[2] == "ice" || segments[2] == "session") {
					return segments[3]
				}
			}
		case "streaming":
			if segments[1] == "session" && len(segments) > 2 {
				return segments[2]
			}
		}
	}
	if id := r.URL.Query().Get("sessionId"); id != "" {
		return id
	}

	// Body-carried ids (offer/answer/ice/viewer): peek without consuming —
	// the proxy forwards the original body untouched.
	if body, err := io.ReadAll(r.Body); err == nil {
		r.Body = io.NopCloser(bytes.NewReader(body))
		var probe struct {
			SessionID           string `json:"sessionId"`
			DeviceID            string `json:"deviceId"`
			RemotePlaySessionID string `json:"remotePlaySessionId"`
		}
		_ = json.Unmarshal(body, &probe)
		for _, candidate := range []string{probe.SessionID, probe.DeviceID, probe.RemotePlaySessionID} {
			if candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

// serveRemoteplayChild resolves the request's session and proxies it to the
// session container (host networking → the host's LAN address on the port
// the allocator assigned).
func (server *Server) serveRemoteplayChild(w http.ResponseWriter, r *http.Request) {
	sessionID := remoteplaySessionIDFromRequest(r)
	if sessionID == "" {
		// Two routes under this catch-all carry no session id by design —
		// neither is scoped to a particular game/device session, so
		// neither has anything for remoteplaySessionIDFromRequest to find:
		//   - /input/ws — the device-status input WebSocket; the control
		//     plane still owns device events, the child has nothing to
		//     bind such a connection to.
		//   - /streaming/webrtc/config — the global ICE server list AND
		//     retro-gaming's own borrowed TURN lookup (see
		//     use-retro-streaming-connection.js's getTurnConfig doc), which
		//     hits this exact path too even though it never has a
		//     remoteplay session at all. Confirmed live: every retro AND
		//     remoteplay session hit "session id is required" here on
		//     first connect (2026-08-18) because only /input/ws was
		//     exempted — the frontend was never missing anything, this
		//     router just didn't know where else to send a session-less
		//     call.
		if r.URL.Path == "/input/ws" || r.URL.Path == "/streaming/webrtc/config" {
			server.proxyRemoteplayControlPlane(w, r)
			return
		}
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}

	// answer/ice carry the WebRTC id, not the device id.
	remoteplayWebrtcMu.Lock()
	if mapped := remoteplayWebrtcToSession[sessionID]; mapped != "" {
		sessionID = mapped
	}
	remoteplayWebrtcMu.Unlock()

	record, ok := server.manager.LookupRemoteplay(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "no active session for this id")
		return
	}

	target := server.config.PublicHost + ":" + strconv.Itoa(record.Port)

	// Same reason as the control plane above: a session-scoped path can be a
	// WebSocket too, and httputil would kill it at the handshake.
	if isWebSocketUpgrade(r) {
		proxyWebSocket(w, r, target, "/api/v1/remoteplay"+r.URL.Path)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   target,
	})
	// The mux mounts this router with StripPrefix("/api/v1/remoteplay"), but the
	// CHILD serves the FULL paths (/api/v1/remoteplay/webrtc/...): re-prepend the
	// stripped prefix on the way out or the child 404s every call.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = "/api/v1/remoteplay" + r.URL.Path
		req.URL.RawPath = ""
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		// Capture the WebRTC id the child issued for this session — the
		// very next calls (answer/ice) only carry that id.
		if r.Method == http.MethodPost && r.URL.Path == "/webrtc/offer" && response.StatusCode < 300 {
			if body, err := io.ReadAll(response.Body); err == nil {
				response.Body = io.NopCloser(bytes.NewReader(body))
				var envelope struct {
					Data struct {
						SessionID string `json:"sessionId"`
					} `json:"data"`
				}
				if json.Unmarshal(body, &envelope) == nil && envelope.Data.SessionID != "" {
					remoteplayWebrtcMu.Lock()
					remoteplayWebrtcToSession[envelope.Data.SessionID] = record.SessionID
					remoteplayWebrtcMu.Unlock()
				}
			}
		}
		return nil
	}
	proxy.ServeHTTP(w, r)
}

// proxyRemoteplayControlPlane forwards control-plane-only calls (the
// device-status input WebSocket has no session id) to the remoteplay service.
// Same prefix discipline as the child proxy: the request arrived stripped.
func (server *Server) proxyRemoteplayControlPlane(w http.ResponseWriter, r *http.Request) {
	host := server.config.RemoteplayControlURL[len("http://"):]

	// /input/ws IS a browser WebSocket upgrade, and it comes through here.
	// httputil cannot carry one: once the handshake succeeds the connection
	// stops being HTTP, so the reverse proxy closes it and the browser sees
	// "Connection closed before receiving a handshake response" /
	// ERR_CONNECTION_RESET — reported live 2026-08-25, with no controller as
	// the visible symptom. The comment below this function asserted the
	// opposite ("no browser WS upgrade happens at this layer"); it was wrong
	// about the one route that is exempted from needing a session id.
	if isWebSocketUpgrade(r) {
		proxyWebSocket(w, r, host, "/api/v1/remoteplay"+r.URL.Path)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   host,
	})
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = "/api/v1/remoteplay" + r.URL.Path
		req.URL.RawPath = ""
	}
	proxy.ServeHTTP(w, r)
}

// remoteplayRouter mounts the data-plane surface under /api/v1/remoteplay (the
// webpack proxy forwards exactly these prefixes; everything else stays on
// the control plane). One catch-all: the child serves the same paths, so no
// rewrite is needed.
// remoteplayRouter — this was, until now, the ONLY /api/... mount with no
// authentication at all: any party who could observe/guess a session id
// could hit webrtc/offer, webrtc/ice, or the input path for someone else's
// remote-play session, no token required. Same class of hole retroRouter's
// own doc describes having been fixed for retrogaming — mirrored here.
// Every route below is proxied; the WS ones (/input/ws) are spliced by
// proxyWebSocket rather than handed to httputil, which cannot follow an
// upgrade. This comment used to claim no upgrade happened here at all — it
// did, and the input socket died at every handshake because of it. A single
// auth.Middleware wrap covers the whole thing.
func (server *Server) remoteplayRouter() http.Handler {
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(auth.Middleware(server.config.JWTSecret, next.ServeHTTP))
	})
	router.HandleFunc("/*", server.serveRemoteplayChild)
	return router
}
