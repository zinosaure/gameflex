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
//
// PAR LoadToken DEPUIS LE CHANTIER 55 : la présence lisait le jeton et la ligne
// elle-même, sans la règle du jeton de compte ni la génération des sessions — un
// compte réinitialisé y restait en ligne. Elle juge désormais comme toutes les
// routes du service.
func (server *Server) handlePresenceWS(w http.ResponseWriter, r *http.Request) {
	principal, err := auth.LoadToken(server.db, server.config.JWTSecret, r.URL.Query().Get("token"))
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
