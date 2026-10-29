package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	sharedauth "gameflex/auth"
)

// ARRÊTER PAR LE CHILD (P2, chantier 12).
//
// Le tableau de bord n'a pas de socket vers le child qui tient une partie : il
// demande au kernel. Le kernel tuait alors le conteneur, après avoir vérifié
// le propriétaire sur SA copie — il intervenait sur une partie sans l'accord
// de son child, et c'est en jugeant sur une copie qu'il s'était déjà trompé
// (le 403 « Could not end the session » du 2026-08-25).
//
// Désormais il TRANSMET, avec le nom qu'il a vérifié, et c'est le child qui
// juge sur sa file (`/v4/terminate`) puis s'arrête lui-même — proprement : le
// rétro termine sa boucle d'émulation, donc ses sauvegardes, avant de sortir.
// Le kernel attend que le conteneur soit sorti, pour que l'arrêt reste
// immédiat à l'écran, puis range ses registres.
//
// Un child qui ne répond pas — conteneur planté, ou image d'avant ce chantier,
// qui ne connaît pas la route — est le seul cas où le kernel agit seul, comme
// avant.

// childEndWait — combien de temps le child a pour sortir de lui-même après
// avoir accepté. Au-delà, le kernel arrête le conteneur : le child a dit oui.
const childEndWait = 10 * time.Second

// endThroughChild asks the child on `port` to end its game for `asking` —
// an administrator's request (`admin`) ends any game, not only their own.
//
// handled=false: the child did not answer (or does not know the route) — the
// caller falls back on its own stop. Otherwise status is the child's verdict:
// 200 once the container is gone, 403 when the game is not theirs to end.
func (server *Server) endThroughChild(ctx context.Context, sessionID, container string, port int, asking string, admin bool) (handled bool, status int) {
	if port <= 0 || asking == "" || server.config.InternalSecret == "" {
		return false, 0
	}

	body, _ := json.Marshal(map[string]any{"username": asking, "admin": admin})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		childV4URL(server.config.PublicHost, port, "terminate"), bytes.NewReader(body))
	if err != nil {
		return false, 0
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Secret", sharedauth.SessionSecret(server.config.InternalSecret, sessionID))

	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		slog.Warn("le child ne répond pas à la demande d'arrêt — le kernel arrête seul",
			"container", container, "error", err)

		return false, 0
	}
	response.Body.Close()

	switch response.StatusCode {
	case http.StatusForbidden:
		return true, http.StatusForbidden
	case http.StatusOK:
		server.waitForExit(ctx, container)

		return true, http.StatusOK
	}

	// 404 : une image d'avant P2, qui ne connaît pas la route.
	return false, 0
}

// waitForExit — le conteneur sort de lui-même, borné par childEndWait.
func (server *Server) waitForExit(ctx context.Context, container string) {
	if server.manager == nil || container == "" {
		return
	}

	deadline := time.Now().Add(childEndWait)
	for time.Now().Before(deadline) {
		running, err := server.manager.IsRunning(ctx, container)
		if err != nil || !running {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
	slog.Warn("le child a accepté l'arrêt mais ne sort pas — le kernel l'arrête", "container", container)
}
