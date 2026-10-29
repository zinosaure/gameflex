package api

import (
	"log/slog"
	"net/http"

	"gameflex/credential/internal/auth"
	"github.com/gorilla/websocket"
)

// handlePresenceWS upgrades the presence WebSocket. The JWT travels in the
// query string (browsers cannot set headers on a WS handshake). Invalid
// tokens close the connection with code 1008, like the Python service.
func (server *Server) handlePresenceWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	claims, err := auth.ParseToken(server.config.JWTSecret, token)
	if err != nil {
		server.rejectPresence(w, r)
		return
	}

	var principal auth.Principal
	principal.ID, err = claimsUserID(claims)
	if err != nil {
		server.rejectPresence(w, r)
		return
	}
	err = server.db.QueryRow(
		`SELECT id, username, display_name, role_level FROM users WHERE id = ?`, principal.ID,
	).Scan(&principal.ID, &principal.Username, &principal.DisplayName, &principal.RoleLevel)
	if err != nil {
		server.rejectPresence(w, r)
		return
	}

	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		return
	}
	server.presence.Connect(principal.ID, principal.Username, principal.DisplayName, conn)
}

func (server *Server) rejectPresence(w http.ResponseWriter, r *http.Request) {
	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		return
	}
	// Close 1008 (policy violation) — the exact code the Python registry
	// used for invalid tokens.
	conn.WriteControl(websocket.CloseMessage, closeMessage(1008), writeDeadline())
	conn.Close()
	slog.Debug("presence rejected: invalid token")
}
