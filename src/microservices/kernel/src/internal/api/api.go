// Package api exposes the sessions service REST/WS contract (/api/v1/*,
// see docs/GAMEFLEX.md). The response envelope mirrors the monolith:
// {success, data} / {success:false, errorMessage}.
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"

	"gameflex/kernel/internal/accounts"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/catalog"
	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/manager"
	"gameflex/kernel/internal/registry"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

type Server struct {
	manager *manager.Manager
	catalog *catalog.Catalog
	config  config.Config
	pool    *registry.Pool
	// windDownAsks compte les sollicitations d'arrêt restées sans réponse,
	// par session. Le kernel ne conclut jamais du premier coup : voir
	// askChildToWindDown, qui explique pourquoi ce compteur existe.
	windDownAsks sync.Map
	// storageRoot — le dossier de ce serveur (config.StorageDir) ; un test
	// le remplace par un dossier à lui.
	storageRoot string
	// internalDisks — les disques internes (chantier 23), quand un test les
	// donne ; nil, le manager les découvre.
	internalDisks func(context.Context) []docker.InternalDisk
	// emulatorsRequired — les fichiers d'émulateur qu'attendent les images
	// (chantier 29), quand un test les donne ; nil, le manager lit les images.
	emulatorsRequired func(context.Context) []docker.RequiredEmulator
	// mirrors holds what child-managed sessions report about themselves
	// (chantier-4). The kernel SHOWS these and never decides from them.
	mirrors *registry.Mirrors
	// peerCopies — les copies des listings des serveurs enregistrés
	// (chantier 15), dans le cache des listings : voir copies().
	peerCopies remote.Copies
	copiesOnce sync.Once
	// rescans — les parcours de ressource en cours. Un scan complet dépasse
	// les deux minutes : il tourne en fond, son état se lit ici, et ses débuts
	// et fins réveillent le sujet `mount/<id>` du centre de messages.
	rescans listing.Rescans
	// launches — la progression des lancements en cours (#191), avant qu'une
	// session existe. Zéro-valeur utilisable, comme `rescans` juste au-dessus :
	// rien à initialiser dans NewServer, donc rien à y oublier.
	launches launchProgress
	// reports — les signalements de chaque compte dans l'heure (#601).
	// Zéro-valeur utilisable.
	reports reportBudget
	// spawns — les places réservées par les lancements en cours (#601).
	spawns spawnReservations
	// launching — les comptes dont un lancement est en cours : un seul à la
	// fois par compte (voir facadeLaunch). Zéro-valeur utilisable.
	launching sync.Map
	// serverRegistry / mountRegistry — les deux listes que le service
	// d'identité tient, mémorisées dix secondes (#190). Elles étaient
	// redemandées à CHAQUE affichage de jeux, avant même de lire le moindre
	// cache : deux allers-retours HTTP pour une donnée qui ne bouge que
	// lorsqu'on ajoute un serveur ou un partage.
	serverRegistry listCache[registeredServer]
	mountRegistry  listCache[mountedShare]
	// accounts — la copie des comptes que le service d'identité pousse
	// (chantier 20) : qui existe encore, son rang, ses consoles, son jeton.
	// Voir accounts_cut.go.
	accounts *accounts.Store
	// peerSessions — les sessions ouvertes chez nos amis, par membre d'ici,
	// avec le jeton de son compte là-bas (chantier 20). Zéro-valeur
	// utilisable. Voir federation_client.go.
	peerSessions remote.Sessions
	// savesDeletions — les suppressions de sauvegardes en cours. Effacer le
	// dossier d'un joueur prend des MINUTES (29 Go, 76 000 fichiers ici) :
	// la route répond aussitôt et le travail continue en fond, son état se
	// lisant ici. Zéro-valeur utilisable, comme les caches au-dessus.
	savesDeletions savesDeletionTracker
}

func NewServer(sessionManager *manager.Manager, gameCatalog *catalog.Catalog, cfg config.Config, pool *registry.Pool) *Server {
	// The façade pool must include the manager's SPAWNED sessions
	// (streamex platform) alongside the registered ones — without this
	// bridge, /streaming can never resolve a freshly launched streamex
	// session ("This session no longer exists" while the container runs),
	// because both /facade/sessions and /facade/ws only read the pool.
	// Double-subscription by design: every manager change (launch, stop,
	// adoption) re-syncs the streamex entries, so a session stopped by the
	// idle watchdog is removed from the pool too — nothing leaks (see the
	// old note about registering spawned sessions naively).
	go bridgeManagerToPool(sessionManager, pool)

	server := &Server{
		mirrors:     registry.NewMirrors(),
		manager:     sessionManager,
		catalog:     gameCatalog,
		config:      cfg,
		pool:        pool,
		storageRoot: config.StorageDir,
	}

	// Les comptes, leur porte et leur coupure (chantier 20).
	server.followAccounts()

	// The unified 5-min idle shutdown (étape 3) — replaces the manager's
	// per-platform TTL/grace rules.
	go server.idleWatchdog()
	// Le listing de ce serveur (chantier 15) : construit s'il manque.
	server.startSelfListing()

	return server
}

// bridgeManagerToPool mirrors the manager's spawned (streamex) sessions
// into the cross-platform pool on every manager change. Re-syncing from
// scratch each time (rather than patching deltas) keeps the pool exact
// even if a notification is missed or two changes coalesce.
func bridgeManagerToPool(manager *manager.Manager, pool *registry.Pool) {
	// Manager-owned retro session IDs, persisted across syncs: the unregister
	// sweep must only remove entries THIS bridge registered — during the
	// migration the old retrogaming service still registers its own
	// platform:"retrogaming" sessions and they must pass through untouched.
	ownedRetro := make(map[string]bool)
	ownedRemoteplay := make(map[string]bool)

	sync := func() {
		current := make(map[string]bool)
		currentRetro := make(map[string]bool)
		currentRemoteplay := make(map[string]bool)

		for _, spawned := range manager.ListRetro() {
			currentRetro[spawned.SessionID] = true

			// The pool entry keeps REAL player names (the WS relay calls
			// Add/RemovePlayer) — the watchdog's count is mirrored as a
			// number only, so it never clobbers the names.
			if ownedRetro[spawned.SessionID] {
				if existing, ok := pool.Lookup(spawned.SessionID); ok && existing.PlayerCount != spawned.PlayerCount {
					pool.SetPlayerCount(spawned.SessionID, spawned.PlayerCount)
				}
				continue
			}

			pool.Register(registry.Session{
				ID:         spawned.SessionID,
				Platform:   "retrogaming",
				ConsoleID:  spawned.ConsoleID,
				Reference:  spawned.Reference,
				Title:      spawned.Title,
				Console:    spawned.ConsoleID,
				Game:       spawned.Title,
				Initiator:  spawned.Owner,
				StartedAt:  spawned.StartedAt,
				MaxPlayers: spawned.MaxPlayers,
				Players:    []string{spawned.Owner},
			})
			ownedRetro[spawned.SessionID] = true
		}
		for sessionID := range ownedRetro {
			if !currentRetro[sessionID] {
				pool.Unregister(sessionID)
				delete(ownedRetro, sessionID)
			}
		}

		for _, spawnedRemoteplay := range manager.ListRemoteplay() {
			currentRemoteplay[spawnedRemoteplay.SessionID] = true

			// Remoteplay sessions — same mirror as the retro kind: real
			// player names via Add/RemovePlayer, watchdog count mirrored as
			// a number only. MaxPlayers is 1: one controller slot per
			// session (the capacity per DEVICE is the manager's data).
			if ownedRemoteplay[spawnedRemoteplay.SessionID] {
				if existing, ok := pool.Lookup(spawnedRemoteplay.SessionID); ok && existing.PlayerCount != spawnedRemoteplay.PlayerCount {
					pool.SetPlayerCount(spawnedRemoteplay.SessionID, spawnedRemoteplay.PlayerCount)
				}
				continue
			}
			pool.Register(registry.Session{
				ID:         spawnedRemoteplay.SessionID,
				Platform:   "remoteplay",
				ConsoleID:  spawnedRemoteplay.HostType,
				Reference:  spawnedRemoteplay.HostID,
				Title:      spawnedRemoteplay.HostName,
				Initiator:  spawnedRemoteplay.Owner,
				StartedAt:  spawnedRemoteplay.StartedAt,
				MaxPlayers: 1,
				Players:    []string{spawnedRemoteplay.Owner},
				// The owner holds pad 1 from the start — the kernel is the
				// control authority (B2), so the initial holder must be
				// known here, not only in the child.
				PadUsers: map[int]string{1: spawnedRemoteplay.Owner},
			})
			ownedRemoteplay[spawnedRemoteplay.SessionID] = true
		}
		for sessionID := range ownedRemoteplay {
			if !currentRemoteplay[sessionID] {
				pool.Unregister(sessionID)
				delete(ownedRemoteplay, sessionID)
			}
		}

		for _, spawned := range manager.List() {
			current[spawned.SessionID] = true

			// Streamex sessions feed the pool THEMSELVES: the child reports
			// real player names, pads and heartbeats through the internal
			// routes, so an existing entry is NEVER re-registered — a
			// wholesale Register would reset Players/PadUsers/IdleSince and
			// resurrect an owner who already left (seen in real: the owner's
			// graceful leave was undone on the next watchdog tick).
			if _, ok := pool.Lookup(spawned.SessionID); ok {
				continue
			}
			pool.Register(registry.Session{
				ID:         spawned.SessionID,
				Platform:   "streamex",
				ConsoleID:  spawned.Console,
				Reference:  spawned.GameID,
				Title:      spawned.Game,
				Console:    spawned.Console,
				Game:       spawned.Game,
				Initiator:  spawned.Owner,
				StartedAt:  spawned.StartedAt,
				MaxPlayers: manager.Seats(spawned.Console),
				Players:    []string{spawned.Owner},
			})
		}
		for _, registered := range pool.List() {
			if registered.Platform == "streamex" && !current[registered.ID] {
				pool.Unregister(registered.ID)
			}
		}
	}

	sync()
	for range manager.Subscribe() {
		sync()
	}
}

func (server *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// TOUTE ROUTE PUBLIQUE EST SOUS /api/v1/<domaine>, et un domaine
	// appartient à un seul service : le proxy du webapp aiguille sur ce seul
	// segment, sans règle d'ordre (webpack.config.js). Lister et relire une
	// partie rétro passent par le routeur rétro, monté plus bas. Le service à
	// service reste sous /internal, hors de cette surface.
	//
	// LANCER, ARRÊTER, DEMANDER LA MANETTE ne sont plus des routes : ce sont
	// des commandes du centre de messages (`/api/v1/facade/ws`, voir
	// facade_ws.go), qui les rejoue sur les mêmes gestionnaires.
	mux.HandleFunc("GET /api/v1/health", server.handleHealth)
	// Ce que cette installation sait faire — lu par la page AVANT la connexion,
	// donc sans jeton (voir capabilities.go, #105).
	mux.HandleFunc("GET /api/v1/capabilities", server.handleCapabilities)
	// Retro API surface — same paths the old retrogaming service exposed,
	// served through a chi subrouter: the stdlib ServeMux rejects the
	// overlapping wildcard patterns (e.g. /{consoleId}/roms/{ref} vs
	// /sessions/{id}/status — /sessions/roms/status matches both), which
	// chi resolves by static segments exactly like the old service did.
	// /api/v1/consoles is the STATIC registry (name/cores/maxPlayers — the
	// frontend menu and game-detail panels need it); the disk scan lives
	// on /api/v1/retrogaming/consoles below.
	mux.HandleFunc("GET /api/v1/consoles", server.handleConsolesRegistry)
	mux.Handle("/api/v1/retrogaming/", http.StripPrefix("/api/v1/retrogaming", server.retroRouter()))
	// Surface fédérée (chantier 6) — un jeton de catalogue, pas un compte ;
	// et, à part, l'échange du jeton d'un de nos comptes contre une session
	// (chantier 20). Son propre préfixe : ce qui est ouvert à quelqu'un qui
	// n'a pas de compte ici doit se lire d'un coup d'oeil, pas se déduire
	// d'une liste de middlewares.
	// Le CONTENU d'une ressource distante — voir, relire, récupérer. Son
	// propre préfixe : /api/v1/mount-registry appartient au service d'identité
	// (qui détient les mots de passe), celui-ci au kernel (qui lit les
	// fichiers). Deux services, deux préfixes, aucune règle d'ordre à
	// retenir.
	mux.Handle("/api/v1/mounts/", http.StripPrefix("/api/v1/mounts", server.mountsRouter()))
	mux.Handle("/api/v1/federation/", http.StripPrefix("/api/v1/federation", server.federationRouter()))
	mux.Handle("/api/v1/remoteplay/", http.StripPrefix("/api/v1/remoteplay", server.remoteplayRouter()))
	mux.Handle("/api/v1/streamex/", http.StripPrefix("/api/v1/streamex", server.streamexRouter()))
	// Les sauvegardes par source (chantier 38, saves_api.go) : un principal,
	// et des copies — les backups, et une par serveur fédéré.
	mux.Handle("/api/v1/saves/", http.StripPrefix("/api/v1/saves", server.savesRouter()))
	// Sonder un serveur distant avant de l'enregistrer (chantier 6). Le
	// registre lui-même vit dans le service d'identité ; ici on ne fait que
	// répondre « ça répond, et voilà comment il s'appelle ». À
	// L'ADMINISTRATEUR, comme l'enregistrement qu'elle précède (la page
	// Fédérations est la seule à l'appeler) : ouverte à tout compte, elle
	// faisait sonder au kernel n'importe quelle adresse — le réseau Docker, le
	// réseau local — et sa réponse servait d'oracle (revue de sécurité,
	// 2026-10-28).
	mux.HandleFunc("POST /api/v1/servers/check", auth.Middleware(server.config.JWTSecret, adminOnly(server.handleFederationCheck)))
	// Ou ÉCHANGER UNE INVITATION (chantier 33) : le jeton du lien naît chez lui
	// et arrive ici, sans passer par une messagerie. Ajouter un serveur est à
	// l'administrateur.
	mux.HandleFunc("POST /api/v1/servers/redeem", auth.Middleware(server.config.JWTSecret, adminOnly(server.handleRedeemServerInvitation)))
	// LE QR D'UNE INVITATION : le dessiner, et le relire dans une image déposée.
	mux.HandleFunc("POST /api/v1/qr", auth.Middleware(server.config.JWTSecret, localOnly(server.handleInvitationQR)))
	mux.HandleFunc("POST /api/v1/qr/read", auth.Middleware(server.config.JWTSecret, localOnly(server.handleReadInvitationQR)))
	mux.HandleFunc("GET /api/v1/games", auth.Middleware(server.config.JWTSecret, server.handleGames))
	mux.HandleFunc("GET /api/v1/sessions", auth.Middleware(server.config.JWTSecret, server.handleListSessions))
	// `POST /api/v1/sessions` EST PARTI SUR LA PORTE UNIQUE (#195) — c'était
	// précisément la route dont le commentaire, quelques lignes plus bas,
	// disait qu'elle n'avait de générique que l'apparence. `handleLaunch` vit
	// toujours : l'aiguillage l'appelle. Le GET juste au-dessus reste, et c'est
	// pour ça que la ligne à retirer devait être lue avec ses voisines.
	// Les consoles à distance des serveurs fédérés (chantier 9, C9-P2). Sous
	// /api/v1/servers et non /api/v1/federation : ces deux préfixes ne se lisent pas
	// dans le même sens — l'un est ce qu'on SERT aux autres, l'autre ce
	// qu'on leur DEMANDE, et c'est notre propre compte qui l'ouvre.
	mux.HandleFunc("GET /api/v1/servers/remoteplay/devices",
		auth.Middleware(server.config.JWTSecret, localOnly(server.handleRemoteDevices)))
	// Ce qu'un pair a comme cores et comme BIOS (#148). Réservé aux
	// administrateurs, comme les mêmes listes pour cette machine : c'est une
	// question de machine, pas de joueur. Seul un serveur fédéré peut en
	// fournir — un montage Samba/WebDAV/kDrive ne porte que des ROMs.
	mux.HandleFunc("GET /api/v1/servers/{id}/storage/cores",
		auth.Middleware(server.config.JWTSecret, adminOnly(server.handleRemoteStorageCores)))
	mux.HandleFunc("GET /api/v1/servers/{id}/storage/bios",
		auth.Middleware(server.config.JWTSecret, adminOnly(server.handleRemoteStorageBios)))
	mux.HandleFunc("GET /api/v1/servers/{id}/storage/roms",
		auth.Middleware(server.config.JWTSecret, adminOnly(server.handleRemoteStorageRoms)))
	// Ce qu'un pair prête de ses paramètres, et ses correctifs (chantier 15).
	mux.HandleFunc("GET /api/v1/servers/{id}/storage/params",
		auth.Middleware(server.config.JWTSecret, adminOnly(server.handleRemoteStorageParams)))
	mux.HandleFunc("GET /api/v1/servers/{id}/storage/patches",
		auth.Middleware(server.config.JWTSecret, adminOnly(server.handleRemoteStoragePatches)))
	// L'échange DANS NOTRE SENS (#154) : les trois routes ci-dessus disent ce
	// qu'un pair possède, celle-ci le dépose chez nous. Réservée aux
	// administrateurs, comme tout ce qui écrit dans la bibliothèque.
	mux.HandleFunc("POST /api/v1/servers/{id}/storage/{section}",
		auth.Middleware(server.config.JWTSecret, adminOnly(server.refreshing(
			func(r *http.Request) string { return r.PathValue("section") }, server.handleRemoteStorageImport))))
	// Remettre en cache ce qu'un pair annonce — même nom que pour un montage
	// (POST /api/v1/mounts/{id}/rescan), parce que c'est le même geste.
	mux.HandleFunc("POST /api/v1/servers/{id}/rescan",
		auth.Middleware(server.config.JWTSecret, adminOnly(server.handleRemoteRescan)))
	// Où joindre un serveur enregistré (C9-P4). Sans jeton dans la réponse :
	// l'adresse n'est pas un secret, celui qui l'ouvre en est un.
	mux.HandleFunc("GET /api/v1/servers",
		auth.Middleware(server.config.JWTSecret, localOnly(server.handleKnownServers)))
	// CE serveur, vérifié chez lui à l'instant (chantier 15) : ce que notre
	// jeton nous y ouvre, et s'il répond — la fiche d'un de ses jeux décide de
	// Play là-dessus.
	mux.HandleFunc("GET /api/v1/servers/{id}",
		auth.Middleware(server.config.JWTSecret, localOnly(server.handleKnownServer)))
	// MON COMPTE CHEZ CE SERVEUR (chantier 20) : le jeton que j'y ai reçu, collé
	// une fois et gardé avec mon compte d'ici, puis la session qu'il ouvre à
	// mon nom là-bas — la page y joue avec elle. Le jeton, lui, ne ressort pas.
	mux.HandleFunc("PUT /api/v1/servers/{id}/account",
		auth.Middleware(server.config.JWTSecret, localOnly(server.handleSetServerAccount)))
	mux.HandleFunc("DELETE /api/v1/servers/{id}/account",
		auth.Middleware(server.config.JWTSecret, localOnly(server.handleForgetServerAccount)))
	mux.HandleFunc("GET /api/v1/servers/{id}/session",
		auth.Middleware(server.config.JWTSecret, localOnly(server.handleServerSession)))

	// LE CENTRE DE MESSAGES (facade_ws.go) — un seul canal par onglet : la
	// liste des sessions, l'état d'une partie et la progression d'un
	// lancement y sont des abonnements ; lancer, arrêter et porter une
	// intention au child y sont des commandes.
	mux.HandleFunc("GET /api/v1/facade/ws", server.handleFacadeWebSocket)

	// Public single-session lookup — the watch route is protected only by the
	// session id itself (the same bearer model the retro-gaming spectate WS
	// already uses), while every launch/control route above stays behind the
	// user JWT.
	// chantier-4: the browser reaches a session's own container through
	// here — one origin, and the port stays the kernel's business. The
	// CHILD authenticates: this only forwards.
	mux.HandleFunc("/api/v1/sessions/{id}/child/", server.handleChildProxy)
	mux.HandleFunc("GET /api/v1/sessions/{id}", server.handlePublicSession)

	// Internal service-to-service API — guarded by GAMEFLEX_INTERNAL_SECRET, not
	// a user JWT: platform owners (retro-gaming, then remoteplay) mirror
	// their session state here. /internal/sessions/register was REMOVED in
	// étape 6 (no caller left: the manager bridge registers everything).
	// chantier-4: a child-managed session describes itself here. Internal
	// secret only — a snapshot never comes from a browser. EACH ROUTE NAMES
	// ITS SESSION, and that session's own secret opens it (sessionSecret,
	// chantier 34, P10): a session container no longer holds the
	// installation's secret, and its own opens nothing but its routes.
	mux.HandleFunc("POST /internal/sessions/{id}/mirror", server.sessionSecret(server.handleSessionMirror))
	mux.HandleFunc("GET /internal/remoteplay/sessions/{id}", server.sessionSecret(server.handleInternalRemoteplayLookup))
	mux.HandleFunc("POST /internal/sessions/{id}/unregister", server.sessionSecret(server.handleInternalUnregister))
	mux.HandleFunc("GET /internal/sessions/{id}", server.sessionSecret(server.handleInternalLookup))
	mux.HandleFunc("POST /internal/sessions/{id}/players", server.sessionSecret(server.handleInternalPlayers))
	mux.HandleFunc("POST /internal/sessions/{id}/spectators", server.sessionSecret(server.handleInternalSpectators))
	// The pad report (which username holds which controller slot), POSTed by
	// the session children so padSlots reflect reality without the browser's
	// façade WS being open on that page.
	mux.HandleFunc("POST /internal/sessions/{id}/pad", server.sessionSecret(server.handleInternalPad))
	// « Joue-t-il ailleurs ? » — sous la partie qui pose la question : c'était
	// `/internal/players/{username}`, la seule route qu'une partie appelait
	// hors de la sienne.
	mux.HandleFunc("GET /internal/sessions/{id}/elsewhere/{username}", server.sessionSecret(server.handleInternalIsAlreadyPlaying))

	return withCORS(mux)
}

func (server *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (server *Server) handleGames(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"games": server.catalog.List()})
}

func (server *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": server.manager.List()})
}

// handleRetroLaunch — first migration-step launch route for the retro
// sessions (the whole old /api/v1/retrogaming/* surface moves here in a later
// step). Same envelope as handleLaunch.
// imageNotInstalledMessage — ce que le joueur lit quand l'image de sa console
// manque et que cette installation ne sait pas la construire (#136).
const imageNotInstalledMessage = "the game image for this console is not installed on this server — pull or build it, then try again"

func (server *Server) handleRetroLaunch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConsoleID string `json:"console_id"`
		ConsoleId string `json:"consoleId"`
		Reference string `json:"reference"`
		Core      string `json:"core"`
		Bios      string `json:"bios"`
		Username  string `json:"username"`
		// Resolution — « 540p » … « 1080p », choisie avant le lancement
		// (#150). Même vocabulaire que streamex et remote-play : c'est
		// `normalizeLaunchSettings` qui la traduit en pixels, une seule fois
		// pour les trois plateformes. Vide = la géométrie d'époque de la
		// console, qui reste le défaut.
		Resolution string `json:"resolution"`
		// FPS et Bitrate — la cadence et le débit du même panneau (#332) : le
		// rétro les suit comme les autres plateformes, parce que jouer en ligne
		// dépend de la connexion de chacun. Zéro = le défaut de la console.
		FPS     int `json:"fps"`
		Bitrate int `json:"bitrate"`
		// Language — la langue de la page qui lance (chantier 21) : la console
		// démarre dans la langue de l'application. Vide ou inconnue, elle
		// garde la sienne.
		Language string `json:"language"`
		// LaunchID — l'identifiant que le NAVIGATEUR a fabriqué avant d'appeler
		// (#191), et sur lequel il écoute déjà. Il n'y a pas encore de
		// `sessionId` — c'est ce que cette route va rendre — donc les étapes
		// qui suivent n'ont pas d'autre adresse que celle-là. Vide : personne
		// n'écoute, et tout se passe exactement comme avant.
		LaunchID string `json:"launchId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	consoleID := body.ConsoleID
	if consoleID == "" {
		consoleID = body.ConsoleId
	}
	username := auth.Username(r.Context())
	if username == "" {
		username = body.Username
	}
	if username == "" {
		username = "anonymous"
	}

	// LES ÉTAPES PARTENT VERS L'IDENTIFIANT DE LANCEMENT (#191). Rien ici ne
	// dépend de ce canal : sans écouteur, `publish` ne coûte qu'un verrou.
	launchID := body.LaunchID
	server.launches.publish(launchID, "session.requested", consoleID)

	// Same guard as the old CreateSession: one PLAYING session per user
	// across platforms (the pool holds every platform's Players).
	if otherID, already := server.alreadyPlaying(username); already {
		server.launches.fail(launchID, "session.requested", "already playing another session")
		server.writeAlreadyPlaying(w, otherID)
		return
	}
	if owned, players, goesOn := server.ownedGameGoesOn(username); goesOn {
		server.launches.fail(launchID, "session.requested", "your game goes on without you")
		server.writeOwnedGameGoesOn(w, owned, players)
		return
	}

	// Temporary per-platform cap (étape 4).
	release, reserved := server.reserveSpawn(w, "retrogaming")
	if !reserved {
		server.launches.fail(launchID, "session.requested", "too many sessions running")
		return
	}
	defer release()

	// UN JEU QUI N'EST PAS DANS LA BIBLIOTHÈQUE NE SE LANCE PAS ICI (chantier
	// 14, 2026-10-12). Celui d'un partage monté ou d'un serveur fédéré se
	// copiait d'abord dans un cache — le mode « hybride » —, et ce cache était
	// une seconde bibliothèque : « mettre en cache dans gameflex/cache/roms
	// c'est exactement comme mettre dans gameflex/roms, ça n'a juste pas de
	// sens » (le porteur). Un jeu d'ailleurs entre dans la bibliothèque par
	// Réglages › Jeux ; celui d'un serveur fédéré se joue aussi CHEZ lui.
	//
	// 404 et non 502 : rien n'a échoué, le jeu n'est simplement pas ici — et
	// le message dit où l'ajouter.
	//
	// LES DISQUES INTERNES EN SONT (chantier 23) : la bibliothèque d'abord,
	// puis chaque disque interne dans l'ordre de son nom. Le jeu se joue
	// depuis le premier qui l'a, avec la console de la bibliothèque.
	_, _, disk, err := server.findRom(r.Context(), consoleID, body.Reference)
	if err != nil {
		server.launches.fail(launchID, "session.requested", "this game is not in the library")
		writeError(w, http.StatusNotFound,
			"this game is not in the library: add it from Settings › Games, or play it on its own server")

		return
	}

	// LE CŒUR AVANT LE CONTENEUR — il doit être là, il ne s'installe plus ici.
	//
	// 412 et non 502 : rien n'a échoué côté réseau, c'est une condition
	// préalable qui manque, et le message dit où la remplir. Descendre un
	// binaire jamais éprouvé au moment où quelqu'un appuie sur Jouer est
	// précisément ce qui a rendu deux consoles injouables (#143).
	if err := server.requireRetroCore(consoleID, body.Core); err != nil {
		server.launches.fail(launchID, "core.required", err.Error())
		writeError(w, http.StatusPreconditionFailed, err.Error())

		return
	}

	server.launches.publish(launchID, "core.required", body.Core)

	if err := server.requireBios(consoleID); err != nil {
		server.launches.fail(launchID, "bios.required", err.Error())
		writeError(w, http.StatusPreconditionFailed, err.Error())

		return
	}

	// LE TITRE DU CATALOGUE, pas celui de la requête (#601) : celui de la
	// bibliothèque ou du disque où le jeu vient d'être trouvé.
	romsRoot := server.retroRomsRoot()
	if disk != nil {
		romsRoot = disk.RomsDir()
	}
	title, known := retro_files.CatalogTitle(romsRoot, consoleID, body.Reference)
	if !known {
		title = body.Reference
	}

	record, err := server.manager.LaunchRetro(
		r.Context(), consoleID, body.Reference, title, body.Core, body.Bios, username, disk,
		manager.LaunchSettings{Resolution: body.Resolution, FPS: body.FPS, Bitrate: body.Bitrate, Language: body.Language},
		// Les deux étapes que seul le manager connaît — conteneur créé, puis
		// conteneur qui répond. Elles remontent par le même canal que les
		// autres, sans que le manager sache qu'un canal existe.
		func(step, detail string) { server.launches.publish(launchID, step, detail) },
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, manager.ErrInvalidUsername):
			status = http.StatusBadRequest
		case errors.Is(err, manager.ErrUnsupportedConsole):
			status = http.StatusNotFound
		case errors.Is(err, manager.ErrConsoleLimitReached):
			status = http.StatusConflict
		case errors.Is(err, docker.ErrImageMissing):
			// 503, not 500: nothing is broken, something has not been built.
			// The raw daemon error ("No such image: gameflex-retrogaming:
			// latest") reached the player as-is before — true, and useless.
			status = http.StatusServiceUnavailable
			writeError(w, status, "the game image for this console is being rebuilt — try again in a few minutes")
			return
		case errors.Is(err, docker.ErrImageNotInstalled):
			// Rien ne la reconstruira ici : une installation sans sources la
			// tire ou la construit elle-même (#136).
			writeError(w, http.StatusServiceUnavailable, imageNotInstalledMessage)
			return
		}
		server.launches.fail(launchID, "container.creating", err.Error())
		writeError(w, status, err.Error())
		return
	}

	// LA DERNIÈRE ÉTAPE PORTE LA SESSION. C'est elle qui dit à l'écran d'aller
	// écouter le reste ailleurs : les jalons suivants (cœur, ROM, BIOS, jeu,
	// géométrie) viennent du conteneur, par le WebSocket de la session.
	server.launches.done(launchID, record.SessionID)
	writeJSON(w, http.StatusOK, map[string]any{"sessionId": record.SessionID})
}

// handleRetroList — point-in-time snapshot of the spawned retro sessions.
func (server *Server) handleRetroList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": server.manager.ListRetro()})
}

// handleRetroStop — kills the session container and drops the record.
// Owner-only: this is the one place a session can be stopped WITHOUT being
// the connected WebSocket client (see Dashboard.jsx's MySession, the
// trash-icon force-stop) — unlike every in-session exit path, there's no
// live connection here to already prove who's asking, so the check has to
// happen here instead.
// mayEndSession — who is allowed to stop this session, asked of the RIGHT
// copy. The manager's record names whoever SPAWNED the container and is never
// rewritten; a chantier-4 handover moves ownership inside the child, which
// reports it into the pool. Checking the manager's copy left the new owner
// with a permanent 403 on their own session ("Could not end the session",
// reported live 2026-08-25) while the previous one could still kill it.
//
// The pool is preferred only when a mirror exists — that is exactly the set of
// sessions whose owner can move.
//
// UN ADMINISTRATEUR ARRÊTE AUSSI LA PARTIE D'UN AUTRE (le porteur, 2026-10-25 :
// « en tant qu'admin on puisse terminer une session des autres », depuis Live
// now). Le kernel le sait par le jeton (auth.IsAdmin) ; la partie l'apprend par
// endThroughChild, qui le lui dit.
func (server *Server) mayEndSession(sessionID, spawnOwner, asking string, admin bool) bool {
	if asking == "" {
		return false
	}
	if admin {
		return true
	}
	if _, mirrored := server.mirrors.Get(sessionID); mirrored {
		if session, ok := server.pool.Lookup(sessionID); ok && session.Owner != "" {
			return session.Owner == asking
		}
	}

	return spawnOwner == asking
}

func (server *Server) handleRetroStop(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	record, ok := server.manager.LookupRetro(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// LE CHILD JUGE (P2) — voir child_end.go.
	if handled, status := server.endThroughChild(r.Context(), sessionID, record.Container, record.Port, auth.Username(r.Context()), auth.IsAdmin(r.Context())); handled {
		if status == http.StatusForbidden {
			writeError(w, http.StatusForbidden, "not your session")
			return
		}
		server.manager.StopRetro(r.Context(), sessionID)
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": sessionID})
		return
	}
	if !server.mayEndSession(sessionID, record.Owner, auth.Username(r.Context()), auth.IsAdmin(r.Context())) {
		writeError(w, http.StatusForbidden, "not your session")
		return
	}
	if !server.manager.StopRetro(r.Context(), sessionID) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessionId": sessionID})
}

// handleRemoteplayStop — POST /api/v1/remoteplay/sessions/{id}/stop. Same
// owner-check shape as handleRetroStop; see its own doc on the route
// registration for why this was missing.
func (server *Server) handleRemoteplayStop(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	record, ok := server.manager.LookupRemoteplay(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if handled, status := server.endThroughChild(r.Context(), sessionID, record.Container, record.Port, auth.Username(r.Context()), auth.IsAdmin(r.Context())); handled {
		if status == http.StatusForbidden {
			writeError(w, http.StatusForbidden, "not your session")
			return
		}
		server.manager.StopRemoteplay(r.Context(), sessionID)
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": sessionID})
		return
	}
	if !server.mayEndSession(sessionID, record.Owner, auth.Username(r.Context()), auth.IsAdmin(r.Context())) {
		writeError(w, http.StatusForbidden, "not your session")
		return
	}
	if !server.manager.StopRemoteplay(r.Context(), sessionID) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessionId": sessionID})
}

func (server *Server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Game       string `json:"game"`
		GameID     string `json:"game_id"`
		ConsoleID  string `json:"console_id"`
		Username   string `json:"username"`
		Resolution string `json:"resolution"`
		FPS        int    `json:"fps"`
		Bitrate    int    `json:"bitrate"`
		// LaunchID — le même contrat que le lancement rétro : le navigateur
		// fabrique l'identifiant AVANT d'appeler et écoute déjà dessus. Vide :
		// personne n'écoute, et `publish` ne coûte qu'un verrou.
		LaunchID string `json:"launchId"`
		// Core — l'émulateur choisi dans la fiche ; vide, celui de l'image.
		Core string `json:"core"`
		// Language — la langue de la page qui lance (chantier 21), comme pour
		// le rétro.
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// UN ÉMULATEUR QU'AUCUNE IMAGE NE PORTE SE REFUSE, au lieu de lancer un
	// autre à sa place (docker.StreamexCores). La fiche d'ici le grise déjà ;
	// ce refus vaut pour un pair, dont la fiche ne juge pas nos émulateurs.
	core := strings.TrimSpace(body.Core)
	if cores := docker.StreamexCores(body.ConsoleID); core != "" && len(cores) > 0 && !slices.Contains(cores, core) {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("%s does not run on this server: its %s images run %s", core, body.ConsoleID, strings.Join(cores, ", ")))
		return
	}

	// The JWT is authoritative — the body username is only a fallback for
	// callers that still send it.
	username := auth.Username(r.Context())
	if username == "" {
		username = body.Username
	}

	gameIdentifier := body.Game
	if gameIdentifier == "" {
		gameIdentifier = body.GameID
	}

	// One playing session per user across platforms (étape 4 — the same
	// guard as retro/remoteplay). Keytest previews are exempt, mirroring
	// the manager's own exemption: a test screen must never be blocked by
	// (or kill) a real game in progress.
	// LES ÉTAPES PARTENT VERS L'IDENTIFIANT DE LANCEMENT, ici comme pour le
	// rétro : « session demandée » n'est pas une notion d'émulation, et les
	// deux gardes ci-dessous sont littéralement le même code.
	launchID := body.LaunchID
	server.launches.publish(launchID, "session.requested", body.ConsoleID)

	if body.ConsoleID != "keytest" {
		if otherID, already := server.alreadyPlaying(username); already {
			server.launches.fail(launchID, "session.requested", "already playing another session")
			server.writeAlreadyPlaying(w, otherID)
			return
		}
		if owned, players, goesOn := server.ownedGameGoesOn(username); goesOn {
			server.launches.fail(launchID, "session.requested", "your game goes on without you")
			server.writeOwnedGameGoesOn(w, owned, players)
			return
		}
	}

	// LE FIRMWARE REQUIS, comme pour une partie rétro (#393) : la PS3 sans son
	// PS3UPDAT.PUP, la Switch sans ses prod.keys démarraient quand même, et
	// l'émulateur échouait à sa façon.
	if body.ConsoleID != "keytest" {
		if err := server.requireBios(body.ConsoleID); err != nil {
			server.launches.fail(launchID, "bios.required", err.Error())
			writeError(w, http.StatusPreconditionFailed, err.Error())

			return
		}
	}

	// L'ÉMULATEUR DE L'IMAGE (chantier 29) : elle le déballe au lancement
	// depuis cores/<console>/. Absent, le conteneur s'arrêterait au démarrage ;
	// on le dit avant, en nommant le fichier.
	if err := server.requireEmulator(r.Context(), body.ConsoleID, core); err != nil {
		server.launches.fail(launchID, "emulator.required", err.Error())
		writeError(w, http.StatusPreconditionFailed, err.Error())

		return
	}

	// Temporary per-platform cap (étape 4).
	release, reserved := server.reserveSpawn(w, "streamex")
	if !reserved {
		server.launches.fail(launchID, "session.requested", "too many sessions running")
		return
	}
	defer release()

	session, err := server.manager.Launch(r.Context(), body.ConsoleID, username, gameIdentifier, manager.LaunchSettings{
		Resolution: body.Resolution,
		FPS:        body.FPS,
		Bitrate:    body.Bitrate,
		Language:   body.Language,
		Core:       core,
	},
		// Les deux étapes que seul le manager connaît — conteneur créé, puis
		// conteneur qui répond.
		func(step, detail string) { server.launches.publish(launchID, step, detail) },
	)
	if err != nil {
		// ANNONCÉ AVANT LE TRI DES CAUSES, à la différence du chemin rétro qui
		// rend la main sur une image manquante SANS rien dire au canal : l'écran
		// resterait alors sur sa dernière étape, à tourner, pour un échec déjà
		// connu. Une cause d'échec se raconte, quelle qu'elle soit.
		server.launches.fail(launchID, "container.creating", err.Error())
		switch {
		case errors.Is(err, manager.ErrInvalidUsername):
			writeError(w, http.StatusBadRequest, "invalid username")
		case errors.Is(err, manager.ErrGameNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, docker.ErrImageMissing):
			// Same as the retro path: not broken, just not built. The generic
			// "session could not be started" below hid the one cause the
			// operator can actually do something about.
			slog.Error("launch failed: image missing", "error", err, "console", body.ConsoleID)
			writeError(w, http.StatusServiceUnavailable,
				"the game image for this console is being rebuilt — try again in a few minutes")
		case errors.Is(err, docker.ErrImageNotInstalled):
			slog.Error("launch failed: image not installed", "error", err, "console", body.ConsoleID)
			writeError(w, http.StatusServiceUnavailable, imageNotInstalledMessage)
		default:
			slog.Error("launch failed", "error", err, "username", username, "game", gameIdentifier)
			writeError(w, http.StatusInternalServerError, "session could not be started")
		}
		return
	}

	// LA DERNIÈRE ÉTAPE PORTE LA SESSION, comme pour le rétro : c'est elle qui
	// dit à l'écran où aller écouter la suite. Sans elle le salon resterait
	// ouvert jusqu'à son TTL, et la page n'apprendrait jamais l'identifiant par
	// le canal qu'elle écoute.
	server.launches.done(launchID, session.SessionID)
	writeJSON(w, http.StatusOK, session)
}

// handleStreamexStop — see handleRetroStop's own doc for why the ownership
// check lives here rather than relying on an already-open connection.
// Named explicitly (not the bare "handleStop" it used to be, registered
// under an unprefixed route) to match handleRetroStop/handleRemoteplayStop
// — the generic name/route read as "works for any platform" when it only
// ever covered streamex, confusing next to its siblings (flagged live
// 2026-08-21 debugging the PS5 stop button).
func (server *Server) handleStreamexStop(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	session, ok := server.manager.LookupStreamex(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if handled, status := server.endThroughChild(r.Context(), sessionID, session.Container, session.Port, auth.Username(r.Context()), auth.IsAdmin(r.Context())); handled {
		if status == http.StatusForbidden {
			writeError(w, http.StatusForbidden, "not your session")
			return
		}
		server.manager.StopStreamex(r.Context(), sessionID)
		writeJSON(w, http.StatusOK, map[string]any{"session_id": sessionID, "state": "stopped"})
		return
	}
	if !server.mayEndSession(sessionID, session.Owner, auth.Username(r.Context()), auth.IsAdmin(r.Context())) {
		writeError(w, http.StatusForbidden, "not your session")
		return
	}

	if !server.manager.StopStreamex(r.Context(), sessionID) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"session_id": sessionID, "state": "stopped"})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

// spawnReservations — les lancements passés par le plafond et pas encore
// finis, par plateforme. Zéro-valeur utilisable.
type spawnReservations struct {
	mu      sync.Mutex
	pending map[string]int
}

// reserveSpawn rejects a spawn when the platform's temporary
// concurrent-session cap (étape 4) is reached — checked before every
// launch, same envelope as writeAlreadyPlaying so the frontend maps it.
//
// UNE RÉSERVATION, PAS UN COMPTE (#601) : les parties déjà là ET les
// lancements en cours. Compter les seules parties laissait passer tous les
// lancements simultanés de comptes différents — chacun voyait la place libre,
// aucun n'existait encore. La place se rend à la fin du lancement (`release`,
// à différer) : réussi, la partie compte désormais dans le registre.
func (server *Server) reserveSpawn(w http.ResponseWriter, platform string) (release func(), ok bool) {
	maximum, capped := registry.SpawnCap(platform)
	if !capped {
		return func() {}, true
	}

	server.spawns.mu.Lock()
	defer server.spawns.mu.Unlock()
	if server.pool.ActiveCount(platform)+server.spawns.pending[platform] >= maximum {
		writeError(w, http.StatusConflict, fmt.Sprintf("too many concurrent %s sessions (limit %d)", platform, maximum))

		return nil, false
	}
	if server.spawns.pending == nil {
		server.spawns.pending = map[string]int{}
	}
	server.spawns.pending[platform]++

	return func() {
		server.spawns.mu.Lock()
		defer server.spawns.mu.Unlock()
		server.spawns.pending[platform]--
	}, true
}

// writeAlreadyPlaying answers the cross-platform one-player rule with the
// other session's id/platform/reference — the frontend turns this 409 into a
// "switch to this screen?" confirm (stop the other session, then relaunch)
// instead of a dead-end error toast (Games.jsx's handleConflict).
func (server *Server) writeAlreadyPlaying(w http.ResponseWriter, otherID string) {
	platform, reference := "", ""
	if other, found := server.pool.Lookup(otherID); found {
		platform, reference = other.Platform, other.Reference
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":                false,
		"errorMessage":           "you already have an active player session elsewhere: " + otherID,
		"activeSessionId":        otherID,
		"activeSessionPlatform":  platform,
		"activeSessionReference": reference,
	})
}

// writeOwnedGameGoesOn — 409 « ta partie continue sans toi » (#276). Même forme
// que writeAlreadyPlaying, plus de quoi dire à qui on la couperait : la page
// propose d'y retourner ou de la terminer pour tout le monde.
//
// LE CORPS NU, comme writeAlreadyPlaying — pas writeJSON, qui l'emballe dans
// `{success: true, data: …}` : la page lit les champs à la racine, et un refus
// emballé lui arrivait sans `activeSessionId` ni message (« Failed to load
// games. » au lieu de la question, vu au premier essai le 2026-10-09).
func (server *Server) writeOwnedGameGoesOn(w http.ResponseWriter, session registry.Session, players []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":                false,
		"errorMessage":           "your game goes on without you: " + session.ID,
		"ownedGameGoesOn":        true,
		"activeSessionId":        session.ID,
		"activeSessionPlatform":  session.Platform,
		"activeSessionReference": session.Reference,
		"activeSessionTitle":     session.Title,
		"activeSessionPlayers":   players,
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errorMessage": message})
}

// writeErrorCode — un refus avec, en plus du message, un code que la page
// traduit (`common.errorCodes.<code>`).
func writeErrorCode(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errorCode": code, "errorMessage": message})
}

// LA POLITIQUE D'ORIGINE APPARTIENT À LA PORTE D'ENTRÉE, ET À ELLE SEULE.
//
// Ce kernel est un proxy pour trois familles de conteneurs de session, et
// chacun pose ses propres en-têtes CORS. Ils s'AJOUTENT aux nôtres — même
// `Header()`, même réponse — et le navigateur reçoit
// « Access-Control-Allow-Origin: *, * ». Deux valeurs là où une seule est
// permise : il refuse une réponse qui a pourtant abouti, ce qui se lit
// « ERR_FAILED 200 (OK) » et ne ressemble à rien de connu.
//
// Vu deux fois en une journée (2026-09-07), sur deux proxys différents — la
// configuration TURN, puis le profil de manette d'une session streamex. Le
// corriger proxy par proxy garantissait une troisième fois : c'est la SORTIE
// qu'il faut tenir, pas chaque chemin qui y mène.
//
// D'où l'enveloppe : au moment d'écrire l'en-tête, quoi qu'aient ajouté les
// couches du dessous, ces trois-là sont ramenés à une valeur unique — la
// nôtre. `Set` remplace toutes les valeurs, c'est exactement ce qu'il faut.
func normalizeCORS(header http.Header) {
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	header.Set("Access-Control-Allow-Headers", "*")
}

// corsWriter — normalise juste avant l'envoi, et laisse tout le reste passer.
//
// `Hijack` et `Flush` sont RÉIMPLÉMENTÉS et ce n'est pas de la politesse :
// envelopper un ResponseWriter lui fait perdre les interfaces qu'il portait,
// et ce kernel relaie des WebSockets (le canal de session, les entrées). Sans
// Hijack, chaque poignée de main échouerait — la manette encore, par un autre
// chemin.
type corsWriter struct {
	http.ResponseWriter
	normalized bool
}

func (writer *corsWriter) normalize() {
	if !writer.normalized {
		writer.normalized = true
		normalizeCORS(writer.Header())
	}
}

func (writer *corsWriter) WriteHeader(status int) {
	writer.normalize()
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *corsWriter) Write(payload []byte) (int, error) {
	writer.normalize()

	return writer.ResponseWriter.Write(payload)
}

func (writer *corsWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := writer.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}

	return hijacker.Hijack()
}

func (writer *corsWriter) Flush() {
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		normalizeCORS(w.Header())
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)

			return
		}

		next.ServeHTTP(&corsWriter{ResponseWriter: w}, r)
	})
}
