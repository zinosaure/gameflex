package api

// LE CODE D'APPAIRAGE, CÔTÉ KERNEL (chantier 60). Le service des comptes tient
// les modes découverte et les codes (credential, pairing.go) ; il pousse leur
// état par la trame `pairing` de /internal/accounts/ws, que la copie des
// comptes reçoit sur le même canal (internal/accounts). Le kernel en garde la
// dernière, et la pousse au sujet `pairing` du centre de messages : chacun y
// voit son mode compte et ses codes, un administrateur le mode serveur en plus
// (gameflex/facade, pairing.go).

import (
	"log/slog"
	"net/http"

	"gameflex/facade"
	"gameflex/kernel/internal/auth"
)

// followPairing — une trame `pairing` arrive. `first` : la première d'une
// connexion au service des comptes.
func (server *Server) followPairing(frame []byte, first bool) {
	if err := server.pairing.Set(frame); err != nil {
		slog.Warn("pairing: unreadable frame", "error", err)

		return
	}
	// SEUL LE MODE SERVEUR PILOTE L'ANNONCE (lan_discovery.go) : un mode compte
	// n'annonce rien.
	server.followServerMode(server.pairing.Frame().Server, first)
}

// connAdmin — cette connexion du centre de messages est-elle celle d'un
// administrateur ? La même garde que les routes (adminOnly), rejouée à vide
// avec son jeton : le rang est celui de la copie des comptes, pas du jeton.
func (server *Server) connAdmin(conn *facade.Conn) bool {
	status, _ := facade.Call(conn, auth.Middleware(server.config.JWTSecret, adminOnly(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), http.MethodGet, "/", nil, nil)

	return status == http.StatusNoContent
}
