// Internal registry routes and the cross-platform façade they feed.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	sharedauth "gameflex/auth"
	"gameflex/kernel/internal/registry"
)

// sessionSecret guards the service-to-service routes of ONE session (its id
// in the path) with a constant-time compare — these routes are called by
// platform owners (retro-gaming, remote-play), never by the browser.
//
// Two secrets open them: the installation's (GAMEFLEX_INTERNAL_SECRET), which
// the long-running services hold, and THAT session's own
// (gameflex/auth.SessionSecret), the only one its container receives
// (chantier 34, P10). A container that runs foreign code — an emulator — can
// therefore touch its own session and nothing else.
func (server *Server) sessionSecret(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		presented := r.Header.Get("X-Internal-Secret")
		if !sharedauth.SameSecret(presented, server.config.InternalSecret) &&
			!sharedauth.SameSecret(presented, sharedauth.SessionSecret(server.config.InternalSecret, r.PathValue("id"))) {
			writeError(w, http.StatusUnauthorized, "invalid or missing internal secret")
			return
		}
		next(w, r)
	}
}

func (server *Server) handleInternalUnregister(w http.ResponseWriter, r *http.Request) {
	server.pool.Unregister(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]string{"session_id": r.PathValue("id")})
}

// handleInternalLookup answers 200 with the registry entry, or 404 — the
// remote-play monolith uses it for its idempotent-reconnect check (a
// refreshed page reuses the still-registered session instead of
// re-handshaking with the console).
func (server *Server) handleInternalLookup(w http.ResponseWriter, r *http.Request) {
	session, ok := server.pool.Lookup(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (server *Server) handleInternalPlayers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string `json:"username"`
		Join       bool   `json:"join"`
		ReleasePad bool   `json:"releasePad"` // true = graceful leave, pad frees now
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Join {
		server.pool.AddPlayer(r.PathValue("id"), body.Username)
	} else {
		server.pool.RemovePlayer(r.PathValue("id"), body.Username, body.ReleasePad)
	}
	writeJSON(w, http.StatusOK, map[string]string{"session_id": r.PathValue("id")})
}

// handleInternalSpectators mirrors one watching browser/tab (WS presence)
// into the pool — same join/leave shape as handleInternalPlayers, but the
// username may be EMPTY (tokenless watch public): anonymous tabs are
// distinct connectionPool entries.
func (server *Server) handleInternalSpectators(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Join     bool   `json:"join"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Join {
		server.pool.AddSpectator(r.PathValue("id"), body.Username)
	} else {
		server.pool.RemoveSpectator(r.PathValue("id"), body.Username)
	}
	writeJSON(w, http.StatusOK, map[string]string{"session_id": r.PathValue("id")})
}

// handleInternalPad sets the REAL pad number (1-based) a player holds — the
// platform child reports it once its controller slot is assigned.
func (server *Server) handleInternalPad(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Pad      int    `json:"pad"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Pad < 1 {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	server.pool.AssignPad(r.PathValue("id"), body.Pad, body.Username)
	writeJSON(w, http.StatusOK, map[string]string{"session_id": r.PathValue("id")})
}

// alreadyPlaying — la partie où ce compte tient déjà une manette : la règle
// « une seule partie jouée à la fois », pour tout le serveur.
//
// D'APRÈS CE QUE LES CHILDS RAPPORTENT (chantier 4). C'est le child qui sait
// qui joue chez lui. Le kernel lisait jusqu'ici sa propre liste de joueurs,
// nourrie par les relais et le pont du gestionnaire : une copie de plus, que
// la documentation disait déjà remplacée (« sur ce que les childs
// rapportent ») et que le code lisait toujours. Une session sans miroir vient
// d'être lancée : celui qui l'a lancée va y jouer, il compte.
func (server *Server) alreadyPlaying(username string) (string, bool) {
	if username == "" {
		return "", false
	}

	for _, session := range server.pool.List() {
		if mirror, mirrored := server.mirrors.Get(session.ID); mirrored {
			for _, player := range mirror.Players {
				// Celui qui EMPRUNTE une manette joue aussi : il n'a pas de
				// place à lui, mais une personne ne tient qu'une manette, ici
				// ou ailleurs (#274).
				if player.Username == username || player.LentTo == username {
					return session.ID, true
				}
			}

			continue
		}

		if session.Initiator == username {
			return session.ID, true
		}
	}

	return "", false
}

// ownedGameGoesOn — la partie que ce compte POSSÈDE et où d'autres jouent sans
// lui (#276, décidé par le porteur le 2026-10-09). Il en est parti, ou en a été
// exclu pour inactivité : sa place lui reste réservée, et les autres
// continuent. Relancer la leur coupait sans prévenir — le kernel arrête les
// parties de celui qui relance. Le lancement est désormais refusé, et la page
// lui propose d'y retourner ou de la terminer pour tout le monde.
//
// Une partie où plus personne ne joue n'est pas concernée : l'arrêter ne coupe
// personne, la relance continue de le faire en silence. Lu sur le miroir, comme
// alreadyPlaying — qui passe avant : s'il joue lui-même quelque part, c'est
// l'autre réponse.
func (server *Server) ownedGameGoesOn(username string) (registry.Session, []string, bool) {
	if username == "" {
		return registry.Session{}, nil, false
	}

	for _, session := range server.pool.List() {
		if session.Initiator != username {
			continue
		}
		mirror, mirrored := server.mirrors.Get(session.ID)
		if !mirrored {
			continue
		}

		others := []string{}
		for _, player := range mirror.Players {
			if player.Username != username {
				others = append(others, player.Username)
			}
		}
		if len(others) > 0 {
			return session, others, true
		}
	}

	return registry.Session{}, nil, false
}

// handleInternalIsAlreadyPlaying answers the cross-platform
// one-active-session-per-player rule: 200 with the occupying session id,
// or 404 when this username holds no player slot anywhere.
func (server *Server) handleInternalIsAlreadyPlaying(w http.ResponseWriter, r *http.Request) {
	sessionID, already := server.alreadyPlaying(r.PathValue("username"))
	if !already {
		writeError(w, http.StatusNotFound, "player has no active session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"session_id": sessionID})
}

// facadeViewer describes the REQUESTING connection for the me field —
// empty username = anonymous (tokenless public watch), forceSpectate makes
// the public session route's requester a spectator by definition (they are
// about to watch this exact session).
type facadeViewer struct {
	username      string
	forceSpectate bool
}

// handleFacadeControlRequest — POST /api/v1/sessions/{id}/control-request.
// REMOVED with P5 (2026-08-24): the unified façade WS request protocol
// (requestslot/cedeslot/slotrequestdenied) replaced the legacy B2 HTTP
// control flow entirely — the kernel is still the control authority, the
// transport is the WS now.

// enrichSessions returns façade-ready copies of the pool list — Status,
// OpenSlots and the rich per-participant ConnectionPool (one entry per
// connected browser/tab — the WS IS the presence; spectateurs arrive in
// étape 2 via the internal spectators route)
// are computed here: the registry stores raw facts, the façade serves the
// unified model (docs/MODELE_SESSION_UNIFIE.md). Pad slots in étape 1 are
// positional placeholders (pool players are usernames only); real roles
// and pads arrive in étape 2. `me` (étape 5) is computed PER REQUESTER —
// never stored — so every tab sees its own stake in each session.
func (server *Server) enrichSessions(list []registry.Session, viewer facadeViewer) []registry.Session {
	out := make([]registry.Session, 0, len(list))
	for _, session := range list {
		// chantier-4: where a child owns its session, what it reported wins.
		// Everything below then works off the child's truth instead of the
		// kernel's own bookkeeping — which is the point: two copies is what
		// kept diverging.
		session = server.applyMirror(session)
		_, mirrored := server.mirrors.Get(session.ID)
		session.ConnectionPool = make([]registry.PlayerEntry, 0, len(session.Players))
		for _, username := range session.Players {
			if username == "" {
				username = "anonymous"
			}
			entry := registry.PlayerEntry{Username: &username, Role: "player"}
			if joined, ok := session.PlayerJoined[username]; ok {
				entry.JoinedAt = joined.UTC().Format(time.RFC3339)
			}
			session.ConnectionPool = append(session.ConnectionPool, entry)
		}
		// Watchers — one entry per connected tab (WS presence). Anonymous ones
		// carry username: null on the wire.
		for _, spectator := range session.Spectators {
			entry := registry.PlayerEntry{Role: "spectate"}
			if spectator.Username != "" {
				username := spectator.Username
				entry.Username = &username
			}
			entry.JoinedAt = spectator.JoinedAt.UTC().Format(time.RFC3339)
			session.ConnectionPool = append(session.ConnectionPool, entry)
		}
		if session.PlayerCount > 0 {
			session.Status = "connected"
		} else {
			session.Status = "idle"
		}
		// openSlots is what a JOINER could take: pads 2..n that are free and
		// unpromised. Never "MaxPlayers - PlayerCount" — that counts the
		// owner's empty pad, and a pad promised to a named user during a
		// hand-over, both of which produce a JOIN button that gets refused.
		// A mirrored session already carries the child's own count — the
		// kernel must not recompute it, or it would be deciding again.
		if !mirrored {
			session.OpenSlots = registry.JoinableSlots(session)
		}
		session.IdleTimeout = int(unifiedIdleTimeout.Seconds())
		// padSlots — 4 entries, or one per seat beyond (the Switch has 8,
		// #443): username = occupied, null = free, false = the console doesn't
		// provide this pad (N > maxPlayers).
		// Served from the REAL pad reservation (PadUsers) — a reservation
		// survives an abrupt leave until the heartbeat expires (rejoin gets
		// the pad back); graceful leave released it immediately.
		slots := max(4, session.MaxPlayers)
		session.PadSlots = make(map[string]any, slots)
		for slot := 1; slot <= slots; slot++ {
			key := "slot" + strconv.Itoa(slot)
			if slot > session.MaxPlayers {
				session.PadSlots[key] = false
				continue
			}
			if username, held := session.PadUsers[slot]; held && username != "" {
				session.PadSlots[key] = username
			} else {
				session.PadSlots[key] = nil
			}
		}

		// me — this requester's own stake. Owner > player > spectate, then
		// null (the requester has no stake here). Anonymous watchers are
		// spectate without a username.
		session.Me = registry.SessionMe{}
		if viewer.username != "" {
			session.Me.Username = &viewer.username
		}
		padFor := func() *int {
			for pad, holder := range session.PadUsers {
				if holder == viewer.username {
					padNumber := pad
					return &padNumber
				}
			}
			return nil
		}
		switch {
		case viewer.username != "" && viewer.username == session.Initiator && sessionHasPlayer(session, viewer.username):
			role := "owner"
			session.Me.Role = &role
			session.Me.PadSlot = padFor()
		case viewer.username != "" && sessionHasPlayer(session, viewer.username):
			role := "player"
			session.Me.Role = &role
			session.Me.PadSlot = padFor()
		case viewer.forceSpectate || sessionHasSpectator(session, viewer.username):
			// Anonymous public watchers land here too (username empty): the
			// public session route itself IS spectator presence.
			role := "spectate"
			session.Me.Role = &role
		}

		out = append(out, session)
	}
	return out
}

// sessionHasPlayer reports whether username is in the session's player list.
func sessionHasPlayer(session registry.Session, username string) bool {
	for _, player := range session.Players {
		if player == username {
			return true
		}
	}
	return false
}

// sessionHasSpectator reports whether username has a watching tab in the
// session. Anonymous watchers (empty username) always match — the presence
// of an anonymous entry is what the public route's me.role=spectate rides on.
func sessionHasSpectator(session registry.Session, username string) bool {
	for _, spectator := range session.Spectators {
		if spectator.Username == username {
			return true
		}
	}
	return false
}

// handlePublicSession answers the watch route's tokenless lookup: spectating
// requires only an ACTIVE session id, never a login (see the route doc in
// api.go). The id is the bearer token for watching, the same way the
// retro-gaming spectate WS already treats it.
func (server *Server) handlePublicSession(w http.ResponseWriter, r *http.Request) {
	session, found := server.pool.Lookup(r.PathValue("id"))
	if !found {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	session = server.enrichSessions([]registry.Session{session}, facadeViewer{forceSpectate: true})[0]

	// writeJSON already wraps the payload in the {success, data} envelope —
	// passing a pre-wrapped map here produced a DOUBLED data field and the
	// frontend resolved a record whose id/platform were undefined (live bug
	// on the cast receiver: it fell into the remoteplay fallback branch and
	// the viewer offer hit "session id is required").
	writeJSON(w, http.StatusOK, session)
}

// handleSessionMirror receives what a child-managed session says about
// itself (chantier-4). The kernel stores it and shows it; it decides nothing
// from it.
//
// Behind the internal secret: only a session container may describe itself,
// and a snapshot is never taken from a browser.
func (server *Server) handleSessionMirror(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	var snapshot registry.MirrorSnapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		writeError(w, http.StatusBadRequest, "invalid snapshot")
		return
	}

	// LA PREMIÈRE FOIS, ON DIT QUI GOUVERNE. Une seule ligne par session, et
	// elle répond d'avance à la question qu'on s'est posée toute la journée du
	// 2026-09-09 : pourquoi le kernel n'expulse-t-il pas ce joueur silencieux ?
	// Parce que ce n'est pas lui qui en juge.
	if _, known := server.mirrors.Get(sessionID); !known {
		slog.Info("session pilotée par son child : le kernel écoute et diffuse, il ne juge pas ses joueurs",
			"session_id", sessionID, "owner", snapshot.Owner, "players", len(snapshot.Players))
	}

	server.mirrors.Put(sessionID, snapshot)

	// Every open façade socket re-pushes, so a Dashboard sees the change
	// without polling.
	server.pool.Notify()

	writeJSON(w, http.StatusOK, map[string]any{"stored": true})
}

// applyMirror overlays what a child reported onto the session the façade is
// about to serve. Where a mirror exists it WINS — the child is the authority,
// this copy is the only thing the kernel knows.
//
// Sessions with no mirror are untouched: the pre-chantier-4 model still runs
// for platforms not yet migrated, and both coexist without a flag.
func (server *Server) applyMirror(session registry.Session) registry.Session {
	snapshot, ok := server.mirrors.Get(session.ID)
	if !ok {
		return session
	}

	if snapshot.Owner != "" {
		session.Owner = snapshot.Owner
		session.Initiator = snapshot.Owner
	}
	if snapshot.MaxPlayers > 0 {
		session.MaxPlayers = snapshot.MaxPlayers
	}

	players := make([]string, 0, len(snapshot.Players))
	pads := make(map[int]string, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, player.Username)
		pads[player.SlotID] = player.Username
	}

	session.Players = players
	session.PlayerCount = len(players)
	session.PadUsers = pads
	session.OpenSlots = snapshot.AvailableSlots
	session.IdleSince = snapshot.IdleSince

	return session
}
