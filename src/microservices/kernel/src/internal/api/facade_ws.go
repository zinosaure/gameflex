package api

// LE CENTRE DE MESSAGES DU SERVEUR — ce que ce kernel branche sur le paquet
// partagé `gameflex/facade`, qui porte l'enveloppe, l'écrivain unique, la
// détection d'un onglet mort et les abonnements (lire sa documentation
// d'abord). Le kernel du téléphone branche les siens sur le même paquet : la
// page est la même, le protocole l'est donc par construction.
//
// Sujets :
//
//   - `sessions` — la liste des sessions, vue par cette connexion ;
//   - `status/<plateforme>/<session>` — l'état d'une partie, au rythme
//     demandé (`every`) ;
//   - `launch/<launchId>` — les étapes d'un lancement, l'histoire d'abord ;
//   - `mount/<id>` — le résumé d'une ressource distante, à chaque début et
//     chaque fin de parcours.
//
// Commandes :
//
//   - `launch` — le corps de l'ancien `POST /api/v1/launch` ;
//   - `session.stop` {platform, sessionId} — réservé au propriétaire.
//
// Rejoindre, demander ou céder la manette n'en font pas partie : c'est au
// child qui tient la session d'en décider, et la page le lui dit sur son
// canal à lui.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gameflex/facade"
	"gameflex/kernel/internal/auth"
	"gameflex/storage/listing"
)

// handleFacadeWebSocket — GET /api/v1/facade/ws.
//
// Le jeton voyage sur `?token=` : un navigateur ne signe pas une poignée de
// main WebSocket. Sans jeton valide, pas de canal — un spectateur anonyme lit
// sa session par `GET /api/v1/sessions/{id}`, qui n'a besoin que de son
// identifiant.
func (server *Server) handleFacadeWebSocket(w http.ResponseWriter, r *http.Request) {
	token := auth.BearerToken(r)

	claims, err := auth.ParseToken(server.config.JWTSecret, token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or missing token")

		return
	}
	// Le compte doit exister encore (chantier 20) : un compte retiré n'ouvre
	// plus de canal, même avec un jeton qui n'a pas expiré.
	if _, admitted := auth.Admit(claims); !admitted {
		writeError(w, http.StatusUnauthorized, "this account no longer has access")

		return
	}

	socket, err := facade.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	facade.Serve(socket, claims.Username, token, facade.Router{
		Topic: server.facadeTopic,
		Commands: map[string]func(*facade.Conn, facade.Request){
			"launch":       server.facadeLaunch,
			"session.stop": server.facadeStop,
		},
	})
}

// facadeTopic rend de quoi alimenter un sujet, ou pourquoi il n'existe pas.
func (server *Server) facadeTopic(conn *facade.Conn, wanted facade.Subscription) (func(context.Context), int, string) {
	switch {
	case wanted.Topic == "sessions":
		return func(ctx context.Context) {
			// Le pool réveille tous ses abonnés au moindre changement,
			// n'importe où : une liste identique pour CETTE connexion ne
			// repart pas.
			signals := server.pool.Subscribe()
			defer server.pool.Unsubscribe(signals)

			facade.PushOnChange(ctx, conn, facade.Push{Type: "sessions", Topic: "sessions"}, signals, func() any {
				return server.enrichSessions(server.pool.List(), facadeViewer{username: conn.Username()})
			})
		}, 0, ""

	case strings.HasPrefix(wanted.Topic, "status/"):
		platform, sessionID, _ := strings.Cut(strings.TrimPrefix(wanted.Topic, "status/"), "/")

		read := server.statusReader(platform, sessionID)
		if read == nil {
			return nil, http.StatusNotFound, "session not found"
		}

		return func(ctx context.Context) {
			facade.PushEvery(ctx, conn, facade.Push{Type: "status", Topic: wanted.Topic}, facade.Pace(wanted.Every), read)
		}, 0, ""

	case strings.HasPrefix(wanted.Topic, "mount/"):
		// LE RÉSUMÉ D'UNE RESSOURCE DISTANTE, poussé à chaque début et chaque
		// fin de parcours. La page le relisait en HTTP toutes les 4 s tant
		// qu'un parcours tournait — plusieurs minutes sur un vrai drive.
		id, err := strconv.ParseInt(strings.TrimPrefix(wanted.Topic, "mount/"), 10, 64)
		if err != nil {
			return nil, http.StatusBadRequest, "invalid mount id"
		}
		mount, found, err := server.findMount(id)
		if err != nil {
			return nil, http.StatusBadGateway, err.Error()
		}
		if !found {
			return nil, http.StatusNotFound, "mount not found"
		}
		key := resourceKey(mount)

		return func(ctx context.Context) {
			signals := server.rescans.Subscribe()
			defer server.rescans.Unsubscribe(signals)

			facade.PushOnChange(ctx, conn, facade.Push{Type: "mount", Topic: wanted.Topic}, signals, func() any {
				scanning, failure := server.rescans.State(key)

				return listing.Summary(listingCacheDir, key, scanning, failure)
			})
		}, 0, ""

	case strings.HasPrefix(wanted.Topic, "launch/"):
		launchID := strings.TrimPrefix(wanted.Topic, "launch/")
		if launchID == "" {
			return nil, http.StatusBadRequest, "missing launch id"
		}

		return func(ctx context.Context) {
			server.pushLaunch(ctx, conn, launchID, facade.Push{Type: "launch", Topic: wanted.Topic})
		}, 0, ""
	}

	return nil, http.StatusBadRequest, "unknown topic: " + wanted.Topic
}

// pushLaunch — les étapes d'un lancement, l'histoire d'abord, puis la suite
// jusqu'à la fin. `frame` dit sous quelle forme elles partent : un sujet
// (`launch`) pour la page qui s'y abonne, ou l'`id` d'une commande
// (`progress`) pour celle qui a lancé.
func (server *Server) pushLaunch(ctx context.Context, conn *facade.Conn, launchID string, frame facade.Push) {
	steps, replay, unsubscribe := server.launches.subscribe(launchID)
	defer unsubscribe()

	for _, step := range replay {
		frame.Data = step
		conn.Send(frame)
	}

	for {
		select {
		case step, open := <-steps:
			if !open {
				return
			}
			frame.Data = step
			conn.Send(frame)
		case <-ctx.Done():
			return
		}
	}
}

// statusReader — comment lire l'état d'une session, ou nil si elle n'existe
// pas sur cette plateforme.
func (server *Server) statusReader(platform, sessionID string) func() (any, bool) {
	switch platform {
	case "retrogaming":
		if _, ok := server.manager.LookupRetro(sessionID); ok {
			return func() (any, bool) { return server.manager.RetroStatus(sessionID) }
		}
	case "streamex":
		if _, ok := server.manager.LookupStreamex(sessionID); ok {
			return func() (any, bool) { return server.manager.StreamexStatus(sessionID) }
		}
	case "remoteplay":
		if _, ok := server.manager.LookupRemoteplay(sessionID); ok {
			return func() (any, bool) { return server.manager.RemoteplayStatus(sessionID) }
		}
	}

	return nil
}

// facadeLaunch — `{type:"launch", id, data:<le corps de l'ancien POST /api/v1/launch>}`.
//
// L'IDENTIFIANT DU LANCEMENT VIENT D'ICI, plus du navigateur. Il fallait que
// la page en FABRIQUE un, ouvre un canal dessus, puis poste — parce que le
// POST et le canal étaient deux connexions distinctes. Sur un seul canal, la
// commande porte son propre `id` : ses étapes reviennent dessus (`progress`)
// avant la réponse. Le `launchId` part quand même avec la réponse, pour la
// page de jeu qui s'ouvre ensuite et rejoue l'histoire en s'abonnant à
// `launch/<launchId>` — le kernel la garde une minute après la fin.
func (server *Server) facadeLaunch(conn *facade.Conn, request facade.Request) {
	var body map[string]any
	if json.Unmarshal(request.Data, &body) != nil || body == nil {
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("invalid request body"))

		return
	}

	// UN LANCEMENT À LA FOIS PAR COMPTE. « Une partie par joueur » se vérifie
	// sur le pool, qui ne connaît une partie qu'une fois son conteneur
	// inscrit : trois lancements arrivés ensemble passaient tous les trois
	// avant qu'aucun ne le soit — trois parties pour un seul joueur
	// (2026-10-09, une page qui rejouait d'un coup les commandes qu'elle avait
	// déclarées injoignables). Le premier suit son cours, les autres sont
	// refusés.
	if username := conn.Username(); username != "" {
		if _, busy := server.launching.LoadOrStore(username, struct{}{}); busy {
			conn.Reply(request.ID, http.StatusConflict, facade.ErrorBody("a launch is already in progress for this account"))

			return
		}
		defer server.launching.Delete(username)
	}

	launchID := newLaunchID()
	server.launches.open(launchID)
	body["launchId"] = launchID
	raw, err := json.Marshal(body)
	if err != nil {
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("invalid request body"))

		return
	}

	// LE FOND DE L'ÉCRAN DE LANCEMENT (chantier 35) : la passerelle ouvre le
	// flux du lancement, puis la page en apprend l'identifiant — par un
	// premier message, sans étape, qui part avant tout le reste. Elle s'y
	// branche aussitôt et y tient l'image de l'intro, même pendant un
	// lancement qui ne publie aucune étape (le Remote Play, qui réveille une
	// console). Chaque étape porte ensuite le même identifiant.
	server.announceLaunchStream(launchID)
	conn.Send(facade.Push{Type: "progress", ID: request.ID, LaunchID: launchID})

	// Les étapes, sur l'id de la commande. Elles s'arrêtent d'elles-mêmes
	// quand le lancement se termine, ou avec la connexion.
	go server.pushLaunch(conn.Context(), conn, launchID, facade.Push{Type: "progress", ID: request.ID, LaunchID: launchID})

	status, answer := facade.Call(conn, auth.Middleware(server.config.JWTSecret, server.handleUnifiedLaunch),
		http.MethodPost, "/api/v1/launch", raw, nil)

	// UN LANCEMENT SE TERMINE TOUJOURS. Deux chemins rendaient leur réponse
	// sans rien dire au canal (une image de console manquante, et tout
	// lancement remote-play, qui ne publie aucune étape) : celui qui
	// écoutait attendait alors jusqu'à l'expiration du salon.
	server.launches.settle(launchID, status, answer)
	// Et son flux avec lui : rattaché à la partie, ou refermé.
	server.settleLaunchStream(launchID, status, answer)

	conn.Send(facade.Push{Type: "reply", ID: request.ID, Status: status, LaunchID: launchID, Data: json.RawMessage(answer)})
}

// facadeStop — `{type:"session.stop", id, data:{platform, sessionId}}`.
//
// Réservé au propriétaire, comme les trois routes qu'il remplace : c'est leur
// gestionnaire qui juge (`mayEndSession`), avec le jeton de cette connexion.
func (server *Server) facadeStop(conn *facade.Conn, request facade.Request) {
	var target struct {
		Platform  string `json:"platform"`
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(request.Data, &target) != nil || target.SessionID == "" {
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("missing session id"))

		return
	}

	handlers := map[string]http.HandlerFunc{
		"retrogaming": server.handleRetroStop,
		"streamex":    server.handleStreamexStop,
		"remoteplay":  server.handleRemoteplayStop,
	}
	handler, known := handlers[target.Platform]
	if !known {
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("unknown platform: "+target.Platform))

		return
	}

	path := fmt.Sprintf("/api/v1/%s/sessions/%s/stop", target.Platform, target.SessionID)
	status, answer := facade.Call(conn, auth.Middleware(server.config.JWTSecret, handler),
		http.MethodPost, path, nil, map[string]string{"id": target.SessionID})

	conn.Reply(request.ID, status, answer)
}

// newLaunchID — un identifiant de lancement. Il ADRESSE un salon, il ne
// protège rien (le canal exige un jeton) ; aléatoire tout de même, pour qu'un
// compte ne lise pas les lancements des autres en devinant.
func newLaunchID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}

	return hex.EncodeToString(raw)
}
