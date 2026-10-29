package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gameflex/credential/internal/auth"
	"github.com/gorilla/websocket"
)

func marshalPayload(payload map[string]any) ([]byte, error) {
	return json.Marshal(payload)
}

func claimsUserID(claims *auth.Claims) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(claims.Subject, "%d", &id)
	return id, err
}

var webSocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return webSocketUpgrader.Upgrade(w, r, nil)
}

func closeMessage(code int) []byte {
	return websocket.FormatCloseMessage(code, "")
}

func writeDeadline() time.Time {
	return time.Now().Add(2 * time.Second)
}

// claimedClientAddress — d'où la requête PRÉTEND venir.
//
// « Prétend », et le mot est choisi : derrière le proxy de la webapp,
// `RemoteAddr` est celui du proxy et ne distingue personne, tandis que
// `X-Forwarded-For` est posé par ce proxy mais reste un en-tête que
// n'importe qui peut écrire. Aucun des deux ne fait preuve.
//
// C'est suffisant pour ce à quoi ça sert — reconnaître un appareil dans un
// journal, voir dix tentatives venir du même endroit — et il ne faut rien en
// déduire de plus. Une décision d'autorisation ne se prend jamais là-dessus.
func claimedClientAddress(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if comma := strings.Index(forwarded, ","); comma > 0 {
			return strings.TrimSpace(forwarded[:comma])
		}

		return strings.TrimSpace(forwarded)
	}

	return r.RemoteAddr
}
