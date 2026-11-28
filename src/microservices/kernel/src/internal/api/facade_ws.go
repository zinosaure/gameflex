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
//   - `download/<id>` — l'état d'un téléchargement en tâche de fond, le
//     dernier d'abord (gameflex/facade, downloads.go), pour le compte qui l'a
//     lancé ;
//   - `downloads` — les téléchargements de ce compte qui tournent ou attendent
//     dans la file, leur rang et leur progression, à chaque changement ;
//   - `pairing` — les modes découverte et les codes d'appairage (chantier 60,
//     pairing.go) : le mode compte et les codes de cette connexion, le mode
//     serveur et les codes `federation` en plus pour un administrateur ;
//   - `discovery` — les serveurs du réseau local en mode découverte : la
//     recherche ne vit que tant qu'une page y est abonnée (lan_discovery.go).
//
// Commandes :
//
//   - `launch` — le corps de l'ancien `POST /api/v1/launch` ;
//   - `session.stop` {platform, sessionId} — réservé au propriétaire ;
//   - `download` {route, …} — un téléchargement en tâche de fond
//     (facadeDownload), réservé aux administrateurs ; un jeu prend la file ;
//   - `download.cancel` {downloadId} — l'arrêter, ou le retirer de la file,
//     par le compte qui l'a lancé ;
//   - `discovery.search` — reposer la question du réseau local, une fois : le
//     geste « Chercher de nouveau ».
//
// Rejoindre, demander ou céder la manette n'en font pas partie : c'est au
// child qui tient la session d'en décider, et la page le lui dit sur son
// canal à lui.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
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
		writeErrorCode(w, http.StatusUnauthorized, "auth_required", "invalid or missing token")

		return
	}
	// Le compte doit exister encore (chantier 20) : un compte retiré n'ouvre
	// plus de canal, même avec un jeton qui n'a pas expiré.
	if _, admitted := auth.Admit(claims); !admitted {
		writeErrorCode(w, http.StatusUnauthorized, "account_no_access", "this account no longer has access")

		return
	}

	socket, err := facade.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	facade.Serve(socket, claims.Username, token, facade.Router{
		Topic: server.facadeTopic,
		Commands: map[string]func(*facade.Conn, facade.Request){
			"launch":           server.facadeLaunch,
			"session.stop":     server.facadeStop,
			"session.standby":  server.facadeStandby,
			"download":         server.facadeDownload,
			"download.cancel":  server.facadeDownloadCancel,
			"discovery.search": server.facadeDiscoverySearch,
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

	// LES PARTIES D'UN PAIR (`#747`, federation_sessions.go) et L'ÉTAT DE SES CONSOLES (`#750`, federation_devices.go) :
	// tant que la page y est abonnée, le socket du pair, ouvert avec notre jeton chez lui, qui ne sort pas d'ici.
	case strings.HasPrefix(wanted.Topic, "servers/"):
		peer, kind, found := server.peerFromTopic(wanted.Topic)
		if !found {
			return nil, http.StatusNotFound, "server not found"
		}
		follow := facade.FollowPeerSessions
		if kind == "devices" {
			follow = facade.FollowPeerDevices
		}

		return func(ctx context.Context) {
			follow(ctx, conn, wanted.Topic, peer.Link, peer.Token)
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

	case strings.HasPrefix(wanted.Topic, "metadata-orphans/"):
		// LES EMPREINTES DES JEUX D'UNE CONSOLE SONT-ELLES LUES ? (#751) La page des métadonnées sans jeu reçoit un
		// 503 `games_not_ready` tant qu'elles ne le sont pas ; elle relisait sa liste toutes les quatre secondes. Elle
		// s'abonne ici : `{ready}` repart à la fin de chaque lecture, et seulement s'il a changé. Un administrateur
		// seulement, comme la liste.
		consoleID := strings.TrimPrefix(wanted.Topic, "metadata-orphans/")
		if !consoles.Exists(consoleID) {
			return nil, http.StatusBadRequest, "unknown console"
		}
		if status, _ := facade.Call(conn, auth.Middleware(server.config.JWTSecret, adminOnly(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})), http.MethodGet, "/", nil, nil); status != http.StatusNoContent {
			return nil, status, "administrators only"
		}

		return func(ctx context.Context) {
			wake, stop := watchGameKeys()
			defer stop()

			facade.PushOnChange(ctx, conn, facade.Push{Type: "metadata-orphans", Topic: wanted.Topic}, wake, func() any {
				_, _, ready := server.accessibleGameKeys(ctx, consoleID)

				return map[string]bool{"ready": ready}
			})
		}, 0, ""

	case wanted.Topic == "disks":
		// LES DISQUES INTERNES, présents, absents, coupés (chantier 86) : repoussés à chaque changement.
		return func(ctx context.Context) {
			server.followDisks(ctx, conn)
		}, 0, ""

	case wanted.Topic == "resources":
		// LES RESSOURCES EXTERNES ET LEURS PARCOURS (`#808`, resources_api.go) : un administrateur seulement.
		if !server.connAdmin(conn) {
			return nil, http.StatusForbidden, "administrators only"
		}

		return func(ctx context.Context) {
			server.followResources(ctx, conn)
		}, 0, ""

	case wanted.Topic == "pairing":
		admin := server.connAdmin(conn)

		return func(ctx context.Context) {
			server.pairing.Follow(ctx, conn, admin)
		}, 0, ""

	case wanted.Topic == "discovery":
		return func(ctx context.Context) {
			server.discovery.Follow(ctx, conn)
		}, 0, ""

	case wanted.Topic == "downloads":
		// LA FILE ET CE QUI TOURNE, pour ce compte (le porteur, 2026-11-16, 2.4) :
		// la page qu'on rouvre la relit, et chaque départ, étape, fin ou rang la
		// repousse.
		return func(ctx context.Context) {
			wake, stop := server.downloads.Watch()
			defer stop()

			facade.PushOnChange(ctx, conn, facade.Push{Type: "downloads", Topic: "downloads"}, wake, func() any {
				return server.downloads.List(conn.Username())
			})
		}, 0, ""

	case strings.HasPrefix(wanted.Topic, "download/"):
		downloadID := strings.TrimPrefix(wanted.Topic, "download/")
		steps, last, unsubscribe, known := server.downloads.Subscribe(downloadID, conn.Username())
		if !known {
			unsubscribe()

			return nil, http.StatusNotFound, "unknown download"
		}

		return func(ctx context.Context) {
			defer unsubscribe()
			frame := facade.Push{Type: "download", Topic: wanted.Topic}
			frame.Data = last
			conn.Send(frame)
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
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "invalid request body"))

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
			conn.Reply(request.ID, http.StatusConflict, facade.ErrorBody("launch_in_progress", "a launch is already in progress for this account"))

			return
		}
		defer server.launching.Delete(username)
	}

	launchID := newLaunchID()
	server.launches.open(launchID)
	body["launchId"] = launchID
	raw, err := json.Marshal(body)
	if err != nil {
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "invalid request body"))

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
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "missing session id"))

		return
	}

	handlers := map[string]http.HandlerFunc{
		"retrogaming": server.handleRetroStop,
		"streamex":    server.handleStreamexStop,
		"remoteplay":  server.handleRemoteplayStop,
	}
	handler, known := handlers[target.Platform]
	if !known {
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "unknown platform: "+target.Platform))

		return
	}

	path := fmt.Sprintf("/api/v1/%s/sessions/%s/stop", target.Platform, target.SessionID)
	status, answer := facade.Call(conn, auth.Middleware(server.config.JWTSecret, handler),
		http.MethodPost, path, nil, map[string]string{"id": target.SessionID})

	conn.Reply(request.ID, status, answer)
}

// facadeStandby — `{type:"session.standby", id, data:{sessionId}}` : met en veille la console d'une partie Remote Play,
// SANS arrêter la partie (chantier 69 ; le porteur, 2026-11-19 : « c'est meilleur de voir la console s'éteindre avant
// de quitter la session […] tant qu'on a de l'image la session ne se coupe pas ») : la console éteinte, la partie se
// termine d'elle-même. Réservé à qui peut arrêter la partie.
func (server *Server) facadeStandby(conn *facade.Conn, request facade.Request) {
	var target struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(request.Data, &target) != nil || target.SessionID == "" {
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "missing session id"))

		return
	}
	path := fmt.Sprintf("/api/v1/remoteplay/sessions/%s/standby", target.SessionID)
	status, answer := facade.Call(conn, auth.Middleware(server.config.JWTSecret, server.handleRemoteplayStandby),
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

// facadeDownload — `{type:"download", id, data:{route, …, body, total, label,
// key, source}}` : un téléchargement en tâche de fond (gameflex/facade,
// downloads.go). Le kernel frappe son identifiant et le rend AUSSITÔT
// (`{downloadId}`) ; le geste tourne ensuite, détaché de la connexion, et son
// état passe sur `download/<id>` et dans la liste `downloads`. Les gestes sont
// ceux des POST d'autrefois — partis depuis, la commande est la seule porte
// (chantier 53) —, avec leurs gardes (administrateur, liste à rafraîchir) :
//
//   - `cores` {body} — un cœur chez libretro, ou un émulateur chez son éditeur
//     (handleStorageCoreDownload) ;
//   - `server` {server, section, body} — un cœur, un BIOS, un correctif ou un
//     jeu pris chez un serveur fédéré (handleRemoteStorageImport) ;
//   - `disk` {disk, section, body} — la même chose depuis un disque interne
//     (handleDiskStorageImport).
//
// UN JEU PREND LA FILE (le porteur, 2026-11-16 : « c'est possible de mettre les
// jeux à télécharger en queue ? ») : la section `roms`, d'un serveur fédéré ou
// d'un disque, attend que le jeu d'avant ait fini. Le reste part tout de suite :
// un émulateur, un BIOS ou un correctif ne patientent pas derrière un jeu de
// plusieurs dizaines de Go. La même chose (`key`) ne se demande pas deux fois
// tant qu'elle attend ou tourne (409).
//
// `total` : la taille que la liste annonçait, quand la réponse ne la dira pas
// (un dossier de plusieurs fichiers) ; `label`, `key`, `source` : ce que la
// liste `downloads` en montre (son titre, ce qu'on télécharge, d'où), pour la
// page qui revient après avoir été fermée.
func (server *Server) facadeDownload(conn *facade.Conn, request facade.Request) {
	var command struct {
		Route  string `json:"route"`
		Server string `json:"server"`
		Disk   string `json:"disk"`
		// Resource — une ressource externe (`#808`) : la route `resource`, un jeu importé dans la bibliothèque.
		Resource string          `json:"resource"`
		Section  string          `json:"section"`
		Body     json.RawMessage `json:"body"`
		Total    int64           `json:"total"`
		Label    string          `json:"label"`
		Key      string          `json:"key"`
		Source   string          `json:"source"`
	}
	if json.Unmarshal(request.Data, &command) != nil || len(command.Body) == 0 {
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "invalid request body"))

		return
	}

	var (
		handler http.HandlerFunc
		path    string
		values  = map[string]string{}
		chiArgs = map[string]string{}
	)
	switch command.Route {
	case "cores":
		path = "/api/v1/retrogaming/storage/cores"
		handler = adminOnly(server.refreshing(sectionNamed(listing.Cores), server.handleStorageCoreDownload))
	case "server":
		if command.Server == "" || command.Section == "" {
			conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "server and section are required"))

			return
		}
		path = "/api/v1/servers/" + command.Server + "/storage/" + command.Section
		values["id"], values["section"] = command.Server, command.Section
		handler = adminOnly(server.refreshing(
			func(r *http.Request) string { return r.PathValue("section") }, server.handleRemoteStorageImport))
	case "disk":
		if command.Disk == "" || command.Section == "" {
			conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "disk and section are required"))

			return
		}
		path = "/api/v1/retrogaming/disks/" + command.Disk + "/storage/" + command.Section
		chiArgs["name"], chiArgs["section"] = command.Disk, command.Section
		handler = adminOnly(server.refreshing(
			func(r *http.Request) string { return chi.URLParam(r, "section") }, server.handleDiskStorageImport))
	case "resource":
		if command.Resource == "" || command.Section != listing.Roms {
			conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "a resource and the roms section are required"))

			return
		}
		path = "/api/v1/retrogaming/resources/" + command.Resource + "/storage/" + command.Section
		chiArgs["id"], chiArgs["section"] = command.Resource, command.Section
		handler = adminOnly(server.refreshing(sectionNamed(listing.Roms), server.handleResourceStorageImport))
	case "official":
		// LA SOURCE OFFICIELLE (chantier 63, P6) : un correctif depuis le paquet `streamex-patches` de l'index.
		if command.Section != listing.Patches {
			conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "the official source offers patches only"))

			return
		}
		path = "/api/v1/retrogaming/storage/official/patches"
		handler = adminOnly(server.refreshing(sectionNamed(listing.Patches), server.handleOfficialPatchImport))
	default:
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "route must be cores, server, disk, resource or official"))

		return
	}

	// L'ADMINISTRATEUR SE VÉRIFIE À LA DEMANDE, pas seulement quand le geste
	// part : un jeu de la file peut attendre des heures, et un refus doit se
	// dire tout de suite. La même garde que le geste (adminOnly), rejouée à vide.
	if status, answer := facade.Call(conn, auth.Middleware(server.config.JWTSecret, adminOnly(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), http.MethodPost, path, nil, nil); status != http.StatusNoContent {
		conn.Reply(request.ID, status, answer)

		return
	}

	downloadID, err := server.downloads.Start(facade.DownloadJob{
		Owner: conn.Username(), Label: command.Label, Key: command.Key, Source: command.Source, Total: command.Total,
		Queued: takesTheQueue(command.Route, command.Section),
		Run: func(ctx context.Context) (int, []byte) {
			if len(chiArgs) > 0 {
				route := chi.NewRouteContext()
				for key, value := range chiArgs {
					route.URLParams.Add(key, value)
				}
				ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
			}

			return facade.CallContext(ctx, conn, auth.Middleware(server.config.JWTSecret, handler),
				http.MethodPost, path, command.Body, values)
		},
	})
	if errors.Is(err, facade.ErrDownloadPending) {
		conn.Reply(request.ID, http.StatusConflict, facade.ErrorBody("download_failed", err.Error()))

		return
	}
	conn.Reply(request.ID, http.StatusAccepted, facade.OKBody(map[string]string{"downloadId": downloadID}))
}

// takesTheQueue — un jeu (la section `roms`, d'un serveur fédéré ou d'un disque)
// attend son tour ; le reste part tout de suite.
func takesTheQueue(route, section string) bool {
	return section == listing.Roms && (route == "server" || route == "disk" || route == "resource")
}

// facadeDownloadCancel — `{type:"download.cancel", id, data:{downloadId}}` :
// arrêter un téléchargement en cours (le porteur, 2026-11-14 : « 3. A »), ou
// retirer un jeu de la file (2026-11-16). Seul le compte qui l'a lancé le fait ;
// le salon finit en `failed`, « cancelled », et le fichier partiel est retiré
// par le geste lui-même.
func (server *Server) facadeDownloadCancel(conn *facade.Conn, request facade.Request) {
	var target struct {
		DownloadID string `json:"downloadId"`
	}
	if json.Unmarshal(request.Data, &target) != nil || target.DownloadID == "" {
		conn.Reply(request.ID, http.StatusBadRequest, facade.ErrorBody("bad_request", "missing download id"))

		return
	}
	if !server.downloads.Cancel(target.DownloadID, conn.Username()) {
		conn.Reply(request.ID, http.StatusNotFound, facade.ErrorBody("not_found", "no running download of yours with this id"))

		return
	}
	answer, _ := json.Marshal(map[string]any{"success": true, "data": map[string]string{"downloadId": target.DownloadID}})
	conn.Reply(request.ID, http.StatusOK, answer)
}
