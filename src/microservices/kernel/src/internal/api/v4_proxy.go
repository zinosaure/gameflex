package api

// chantier-4 — reaching the container that owns a session.
//
// Once a child decides for itself, the browser has to talk to it: ONE socket
// for presence, beats and intents (P3 — the webhook beside it is gone). It
// does so THROUGH the
// kernel rather than dialing the container directly, for three reasons that
// have each already cost us something:
//
//   - one origin. A page on http(s)://<app> opening a socket to another
//     host:port hits mixed content and CORS, and every session would need its
//     own certificate;
//   - the port is not the browser's business. It changes on every spawn, and
//     handing it out invites somebody to bookmark it;
//   - the kernel already knows where the container lives. It is the only
//     party that does.
//
// It forwards and nothing else — it reads no body and grants nothing. Deciding
// here would put back exactly what this chantier removed.
//
// It refuses two things: an unknown session, and ANY PATH BUT THE ONE THE PAGE
// USES (`/v4/ws`, a websocket). It used to forward every path, without a
// guard, to a container on host networking — so anyone could reach streamex's
// debug page and its control API (start, stop, restart) through here
// (2026-09-15). The child authenticates its own socket and intents; the
// allow-list keeps everything else the container serves off this door.

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// handleChildProxy forwards /api/v1/sessions/{id}/child/* to the container
// that runs that session, websockets included.
func (server *Server) handleChildProxy(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	// The suffix after /child/ is the child's own path — and only one is
	// reachable from here, as a websocket (see the note at the top of this
	// file).
	suffix := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/api/v1/sessions/%s/child", sessionID))
	if suffix != "/v4/ws" || !isWebSocketUpgrade(r) {
		http.NotFound(w, r)
		return
	}

	port := server.childPort(sessionID)
	if port == 0 {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	proxyWebSocket(w, r, fmt.Sprintf("%s:%d", server.config.PublicHost, port), suffix)
}

// childPort finds the port a session's container listens on, whatever
// platform runs it. Asked of the manager rather than of the pool: the manager
// is what actually spawned the container and knows where it lives.
func (server *Server) childPort(sessionID string) int {
	if record, ok := server.manager.LookupRetro(sessionID); ok && record.Port > 0 {
		return record.Port
	}
	if record, ok := server.manager.LookupStreamex(sessionID); ok && record.Port > 0 {
		return record.Port
	}
	if record, ok := server.manager.LookupRemoteplay(sessionID); ok && record.Port > 0 {
		return record.Port
	}

	return 0
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// proxyWebSocket splices the browser's connection to the child's. httputil's
// reverse proxy cannot do this: an upgraded connection is no longer HTTP, so
// after the handshake there is nothing to do but copy bytes both ways until
// one side stops.
func proxyWebSocket(w http.ResponseWriter, r *http.Request, target, suffix string) {
	upstream, err := net.Dial("tcp", target)
	if err != nil {
		http.Error(w, "session container unreachable", http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	client, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer client.Close()

	// Replay the handshake against the child with the rewritten path. The
	// original headers ride along untouched: the Sec-WebSocket-Key exchange
	// is between the browser and the child, and rewriting it would break the
	// accept hash.
	forwarded := r.Clone(r.Context())
	forwarded.URL.Path = suffix
	forwarded.URL.Scheme = "http"
	forwarded.URL.Host = target
	forwarded.Host = target

	if err := forwarded.Write(upstream); err != nil {
		return
	}

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()
	<-done
}
