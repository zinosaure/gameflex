// Streamex (external console) data plane — sessions becomes the front door
// for spawned streamex containers here too, same as the retro and
// remoteplay kinds: nodejs's webpack proxy used to resolve
// /api/v1/streamex/<sessionId>/* straight to the container's own DNS name
// (streamex-<sessionId>) on the shared bridge network. That broke the same
// way retro's own alias-based dial did — the embedded DNS can SERVFAIL
// right after container creation — and it hardcoded port 8080, which no
// longer holds once CreateAndStart assigns each session its own unique
// port (see docker.go's own doc on why: network_mode: host, same iptables
// FORWARD fix already applied to retro and remoteplay). Routing through
// here instead means one lookup (manager.Lookup) resolves both the port
// AND avoids container DNS entirely.
package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
)

// serveStreamexChild resolves the leading path segment as a session id,
// looks it up, and reverse-proxies the rest of the path to the container's
// host-published address — PublicHost:port, exactly like the retro and
// remoteplay data-plane proxies.
func (server *Server) serveStreamexChild(w http.ResponseWriter, r *http.Request) {
	segments := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	sessionID := segments[0]
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}

	// The webpack proxy's own pathRewrite stripped "/api/v1/streamex/<id>"
	// down to just the remainder ("" becomes "/") — this handler is
	// already mounted stripped of "/api/v1/streamex" (see Routes()), so only
	// the session id segment itself needs dropping here.
	remainder := "/"
	if len(segments) > 1 {
		remainder = "/" + segments[1]
	}
	r.URL.Path = remainder

	// SEULS LES CHEMINS QUE LA PAGE UTILISE (2026-10-02) : le canal de la
	// manette et le profil de boutons. L'état de la partie n'en est plus : le
	// kernel le pousse lui-même, sur le centre de messages (facade_ws.go).
	// Ce relais transmettait TOUT, sans jeton — la page de débogage du
	// conteneur, sa liste de jeux et ses compteurs de manette étaient
	// lisibles par quiconque connaissait un identifiant de session, y compris
	// par le domaine public. Le défaut avait été corrigé le 2026-09-15 sur le
	// relais voisin (`v4_proxy.go`) ; celui-ci était resté ouvert. Le refus
	// vient AVANT la recherche : il ne dit pas si la session existe.
	if remainder != "/ws" && remainder != "/profile.json" {
		http.NotFound(w, r)

		return
	}

	record, ok := server.manager.LookupStreamex(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "no active session for this id")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   server.config.PublicHost + ":" + strconv.Itoa(record.Port),
	})
	proxy.ServeHTTP(w, r)
}

// streamexRouter mounts the data-plane surface under /api/v1/streamex.
func (server *Server) streamexRouter() http.Handler {
	return http.HandlerFunc(server.serveStreamexChild)
}
