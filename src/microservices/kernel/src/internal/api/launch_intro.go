package api

// L'INTRO PENDANT LE LANCEMENT — chantier 35.
//
// La page de jeu veut, dès le clic sur « Jouer », son fond : l'image tenue de
// l'intro, qui ne sort que de la passerelle. Or il n'y a pas encore de partie,
// donc pas de flux. Le kernel en fait ouvrir un sous l'identifiant du
// lancement qu'il vient de frapper, AVANT la première étape et avant de le
// dire à la page ; il le rattache à la partie quand elle existe, ou
// l'abandonne si elle n'existe jamais.
//
// RIEN D'UN LANCEMENT N'EN DÉPEND : une passerelle muette, ou plus ancienne,
// laisse la page sur son fond noir d'avant. D'où des appels brefs, dont
// l'échec ne fait que s'écrire au journal.

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// launchStreamTimeout — la passerelle est sur la même machine : au-delà, elle
// ne répondra pas, et le lancement n'attend pas pour elle.
const launchStreamTimeout = 2 * time.Second

// announceLaunchStream ouvre le flux en attente d'un lancement.
func (server *Server) announceLaunchStream(launchID string) {
	server.callLaunchStream(http.MethodPost, launchID, "", nil)
}

// settleLaunchStream — la réponse du lancement est connue : la partie qu'il
// a créée prend ses spectateurs, ou son flux se referme.
func (server *Server) settleLaunchStream(launchID string, status int, answer []byte) {
	if sessionID := answeredSessionID(answer); status >= 200 && status < 300 && sessionID != "" {
		body, _ := json.Marshal(map[string]string{"sessionId": sessionID})
		server.callLaunchStream(http.MethodPost, launchID, "/session", body)

		return
	}
	server.callLaunchStream(http.MethodDelete, launchID, "", nil)
}

func (server *Server) callLaunchStream(method, launchID, suffix string, body []byte) {
	base := strings.TrimRight(server.config.GatewayControlURL, "/")
	if base == "" || server.config.InternalSecret == "" || launchID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), launchStreamTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, base+"/internal/launches/"+launchID+suffix, bytes.NewReader(body))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		slog.Warn("la passerelle ne répond pas sur le flux du lancement", "launch_id", launchID, "method", method, "error", err)

		return
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// 404 : une passerelle d'avant le chantier 35, qui n'a pas la route.
		slog.Warn("la passerelle refuse le flux du lancement", "launch_id", launchID, "method", method, "status", response.StatusCode)
	}
}
