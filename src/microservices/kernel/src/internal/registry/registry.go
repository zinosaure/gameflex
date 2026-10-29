// Package registry is the cross-platform session registry inside the
// sessions service — the direct successor of the monolith's
// internal/sessions pool. Platform owners (retro-gaming now, remoteplay
// soon) mirror their live sessions here through the internal HTTP API
// (/internal/sessions/*, see internal/api), so the frontend has one
// façade (/api/v1/facade/*) to list and resolve every session regardless
// of which platform actually runs it, and cross-platform rules that need a
// single source of truth (one active player-session per user) have
// somewhere to live.
package registry

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// PlayerEntry is one connected participant in the unified session model
// (docs/MODELE_SESSION_UNIFIE.md): role distinguishes a controller holder
// from a spectator, padSlot is the controller number (1-based, 0/null for
// spectators). A user may appear twice (player AND spectator
// of their own session) but holds role "player" at most once.
type PlayerEntry struct {
	// Username is null on the wire for tokenless watchers (watch public) —
	// several anonymous tabs are distinct entries, only named users dedupe.
	Username *string `json:"username"`
	Role     string  `json:"role"` // "player" | "spectate"
	JoinedAt string  `json:"joinedAt,omitempty"`
}

// Spectator is one watching browser/tab (WS presence). Username is empty
// for tokenless watchers.
type Spectator struct {
	Username string
	JoinedAt time.Time
}

type Session struct {
	ID        string `json:"id"`
	Platform  string `json:"platform"` // "retrogaming" | "remoteplay" | "streamex"
	ConsoleID string `json:"consoleId"`
	// Reference is a catalog uuid (retrogaming) or the device's own row id
	// (remoteplay) — an opaque identifier, not for display. Title is the
	// human-readable name to actually show (game title / device name).
	Reference string `json:"reference"`
	Title     string `json:"title"`
	// Initiator stays INTERNAL (étape 6): the wire serves owner, the unified
	// name — see Owner below, populated at Register.
	Initiator string    `json:"-"`
	StartedAt time.Time `json:"startedAt"`
	// MaxPlayers is fixed at Register time (console capacity / 1 for
	// remoteplay's single-controller model). PlayerCount is always
	// len(Players) — kept as its own field so JSON consumers don't need to
	// count the array themselves, but Players is the actual source of
	// truth internally (see IsAlreadyPlaying). External (spawned) sessions
	// pad Players with empty strings for the joiners /status counts but
	// cannot name (see api.bridgeManagerToPool).
	// MaxPlayers/Players stay INTERNAL (étape 6): the wire serves openSlots
	// and connectionPool instead — maxPlayers was dropped from the canonical
	// object (docs/MODELE_SESSION_UNIFIE.md §1).
	MaxPlayers  int      `json:"-"`
	PlayerCount int      `json:"playerCount"`
	Players     []string `json:"-"`
	// Spectators + PlayerJoined + PadUsers are internal only (not
	// serialized raw): spectators mirror watching browser tabs (WS
	// presence), PlayerJoined stamps when each player joined,
	// PadUsers is the REAL pad reservation (1-based slot → username).
	Spectators   []Spectator          `json:"-"`
	PlayerJoined map[string]time.Time `json:"-"`
	PadUsers     map[int]string       `json:"-"`
	// Unified-model extras (docs/MODELE_SESSION_UNIFIE.md) — populated at
	// façade serving time: Status is connected|idle (idle = no player),
	// ConnectionPool is the rich per-participant view, OpenSlots is computed
	// backend-side
	// (MaxPlayers - players). Console/Game are set at registration where
	// the platform has them (retrogaming/streamex), left empty (omitted)
	// for remoteplay. Owner is the wire name for Initiator — set once at
	// Register so every serving path carries it.
	Status         string        `json:"status,omitempty"` // "connected" | "idle"
	Console        string        `json:"console,omitempty"`
	Game           string        `json:"game,omitempty"`
	Owner          string        `json:"owner,omitempty"`
	ConnectionPool []PlayerEntry `json:"connectionPool,omitempty"`
	OpenSlots      int           `json:"openSlots"`
	// PadSlots has 4 entries (slot1..slot4), or one per seat beyond: username = occupied,
	// null = available, false = the console doesn't provide this slot
	// (N > MaxPlayers). Populated at façade serving time.
	PadSlots map[string]any `json:"padSlots,omitempty"` // IdleSince marks when the last player left (gracefully) — the basis
	// for the idle countdown (frontend: idleSince + 5 min - now). Nil =
	// session has players. Maintained by AddPlayer/RemovePlayer/Register.
	IdleSince *time.Time `json:"idleSince,omitempty"`
	// IdleTimeout — seconds a session survives without a live player,
	// served alongside IdleSince so a page can render a real countdown
	// instead of hard-coding the server's rule and drifting from it.
	IdleTimeout int `json:"idleTimeout,omitempty"`
	// Me is the REQUESTER's own view of this session, computed per
	// connection at façade serving time (étape 5) — never stored in the
	// pool: every browser tab sees its own role (owner/player/spectate).
	Me SessionMe `json:"me,omitempty"`
}

// SessionMe is one requester's stake in a session (docs/MODELE_SESSION_UNIFIE.md,
// étape 5): who they are, what role they hold, which pad is theirs.
type SessionMe struct {
	Username *string `json:"username"` // null = anonymous (tokenless watch)
	Role     *string `json:"role"`     // "owner" | "player" | "spectate" | null
	PadSlot  *int    `json:"padSlot"`  // 1-based pad, null unless player/owner
}

type Pool struct {
	mu          sync.Mutex
	sessions    map[string]*Session
	subscribers map[chan struct{}]bool
}

func NewPool() *Pool {
	return &Pool{
		sessions:    make(map[string]*Session),
		subscribers: make(map[chan struct{}]bool),
	}
}

// Register inserts or replaces a session's entry wholesale — called once at
// session start by whichever platform owns it. Players/PlayerCount here
// should reflect the initiator alone (see AddPlayer for anyone joining
// after).
func (pool *Pool) Register(session Session) {
	// owner is the wire name for Initiator — set once here so EVERY serving
	// path (façade AND the raw internal lookup the remoteplay control plane
	// reads) carries it.
	session.Owner = session.Initiator
	session.PlayerCount = len(session.Players)
	session.PlayerJoined = make(map[string]time.Time, len(session.Players))
	session.PadUsers = make(map[int]string, session.MaxPlayers)
	for index, username := range session.Players {
		session.PlayerJoined[username] = session.StartedAt
		if index < session.MaxPlayers {
			session.PadUsers[index+1] = username // owner first, joiners after
		}
	}
	if session.PlayerCount == 0 {
		now := time.Now()
		session.IdleSince = &now
	}
	pool.mu.Lock()
	pool.sessions[session.ID] = &session
	pool.mu.Unlock()
	pool.notify()
}

// spawnCaps are the TEMPORARY hard concurrent-session caps per platform
// (docs/MODELE_SESSION_UNIFIE.md, étape 4), checked before every spawn. To
// become dynamic for streamex (per-console weights — switch is heavier
// than PS3/Xbox, a fixed 3 would handicap two parallel PS3/Xbox sessions).
var spawnCaps = map[string]int{
	"retrogaming": 10,
	"streamex":    3,
	"remoteplay":  1,
}

// SpawnCap returns the platform's temporary cap and whether it has one.
func SpawnCap(platform string) (int, bool) {
	maximum, ok := spawnCaps[platform]
	return maximum, ok
}

// ActiveCount reports how many sessions of a platform are registered
// (running or idle — an idle session still holds its container until the
// unified idle watchdog stops it, so it counts toward the spawn cap).
func (pool *Pool) ActiveCount(platform string) int {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	count := 0
	for _, session := range pool.sessions {
		if session.Platform == platform {
			count++
		}
	}
	return count
}

// Unregister removes a session entirely — called once the underlying
// platform has fully torn it down (not on every player leaving, see
// AddPlayer/RemovePlayer for that).
func (pool *Pool) Unregister(id string) {
	pool.mu.Lock()
	delete(pool.sessions, id)
	pool.mu.Unlock()
	pool.notify()
}

// List order is deterministic (oldest session first) — Go map iteration
// order is randomized on every call, which without this sort made every
// consumer (Dashboard's live carousel, NotificationBell) see the session
// array reshuffle on every push even when nothing changed about which
// sessions exist. A frontend list keyed by array POSITION rather than
// session id (react-bootstrap's Carousel tracks its active slide by index)
// visibly breaks under that: the "active" slide silently jumps to a
// different session's video mid-reorder.
func (pool *Pool) List() []Session {
	pool.mu.Lock()
	out := make([]Session, 0, len(pool.sessions))
	for _, session := range pool.sessions {
		out = append(out, *session)
	}
	pool.mu.Unlock()

	// Sorted on the copy, after unlocking — out no longer touches shared
	// state, no reason to hold the lock across the sort.
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

func (pool *Pool) Lookup(id string) (Session, bool) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	session, ok := pool.sessions[id]
	if !ok {
		return Session{}, false
	}
	return *session, true
}

// SetPlayerCount mirrors a platform-owned live player count without
// touching the Players slice — used by the manager bridge for retro
// sessions, whose REAL names are added/removed by the WS relay and must
// not be clobbered by the watchdog's count-only /status.
func (pool *Pool) SetPlayerCount(id string, count int) {
	pool.mu.Lock()
	session, ok := pool.sessions[id]
	if !ok {
		pool.mu.Unlock()
		return
	}
	session.PlayerCount = count
	if count == 0 && session.IdleSince == nil {
		now := time.Now()
		session.IdleSince = &now
	}
	pool.mu.Unlock()
	pool.notify()
}

// AddPlayer/RemovePlayer mirror a player joining/leaving an already-
// registered session — this keeps Players (and therefore PlayerCount and
// IsAlreadyPlaying) accurate as joiners come and go.
func (pool *Pool) AddPlayer(id, username string) {
	pool.mu.Lock()
	session, ok := pool.sessions[id]
	if !ok {
		pool.mu.Unlock()
		return
	}
	for _, existing := range session.Players {
		if existing == username {
			pool.mu.Unlock()
			return
		}
	}
	session.Players = append(session.Players, username)
	session.PlayerCount = len(session.Players)
	if session.PlayerJoined == nil {
		session.PlayerJoined = make(map[string]time.Time)
	}
	session.PlayerJoined[username] = time.Now()
	session.IdleSince = nil // someone is back — the idle clock stops
	// First free pad, positionally — the platform corrects it later with
	// AssignPad when it reports the real pad number.
	for pad := 1; pad <= session.MaxPlayers; pad++ {
		if _, taken := session.PadUsers[pad]; !taken {
			session.PadUsers[pad] = username
			break
		}
	}
	pool.mu.Unlock()
	pool.notify()
}

// AssignPad sets the REAL pad number for a player (the platform reports it
// once its child assigned the controller slot). A player holds exactly one
// pad: any previous reservation of theirs — typically AddPlayer's
// positional first-free-pad guess — is dropped, so a correction doesn't
// leave the same username on two slots (seen in real on streamex).
func (pool *Pool) AssignPad(id string, pad int, username string) {
	pool.mu.Lock()
	session, ok := pool.sessions[id]
	if !ok {
		pool.mu.Unlock()
		return
	}
	if session.PadUsers == nil {
		session.PadUsers = make(map[int]string)
	}
	for existingPad, holder := range session.PadUsers {
		if holder == username {
			delete(session.PadUsers, existingPad)
		}
	}
	session.PadUsers[pad] = username
	platform, maxPlayers := session.Platform, session.MaxPlayers
	pool.mu.Unlock()
	// One "controller n/N" line per assignment — same shape the streamex
	// child logs for its own slots, so retro-gaming sessions are just as
	// diagnosable when two devices fight over a pad.
	slog.Info("controller assigned",
		"session_id", id, "platform", platform, "username", username,
		"controller", fmt.Sprintf("%d/%d", pad, maxPlayers))
	pool.notify()
}

func (pool *Pool) RemovePlayer(id, username string, releasePad bool) {
	pool.mu.Lock()
	session, ok := pool.sessions[id]
	if !ok {
		pool.mu.Unlock()
		return
	}
	for index, existing := range session.Players {
		if existing != username {
			continue
		}
		session.Players = append(session.Players[:index], session.Players[index+1:]...)
		session.PlayerCount = len(session.Players)
		break
	}
	delete(session.PlayerJoined, username)
	// Graceful leave: the pad frees immediately. Abrupt: the reservation
	// survives, and the child decides (rejoin gets the pad back).
	if releasePad {
		for pad, holder := range session.PadUsers {
			if holder == username {
				delete(session.PadUsers, pad)
			}
		}
	}
	if session.PlayerCount == 0 {
		// Last player left gracefully — the idle countdown starts now
		// (kernel's idle watchdog stops the session after 5 minutes).
		now := time.Now()
		session.IdleSince = &now
	}
	pool.mu.Unlock()
	pool.notify()
}

// AddSpectator/RemoveSpectator mirror one watching browser/tab (WS
// presence — a closed tab disappears from connectionPool, no durable
// ghost). Anonymous watchers (empty username) are NOT deduped: several
// tabs are distinct entries; RemoveSpectator drops one entry per leave.
func (pool *Pool) AddSpectator(id, username string) {
	pool.mu.Lock()
	session, ok := pool.sessions[id]
	if !ok {
		pool.mu.Unlock()
		return
	}
	if username != "" {
		for _, existing := range session.Spectators {
			if existing.Username == username {
				pool.mu.Unlock()
				return
			}
		}
	}
	session.Spectators = append(session.Spectators, Spectator{Username: username, JoinedAt: time.Now()})
	pool.mu.Unlock()
	pool.notify()
}

func (pool *Pool) RemoveSpectator(id, username string) {
	pool.mu.Lock()
	session, ok := pool.sessions[id]
	if !ok {
		pool.mu.Unlock()
		return
	}
	for index, existing := range session.Spectators {
		if existing.Username != username {
			continue
		}
		session.Spectators = append(session.Spectators[:index], session.Spectators[index+1:]...)
		break
	}
	pool.mu.Unlock()
	pool.notify()
}

// JoinableSlots counts the pads a JOINER could take right now — pads 2..n
// that are free. Deliberately not "MaxPlayers - PlayerCount": that counted
// the owner's empty pad as joinable and offered a JOIN button on a
// one-player session with nobody in it (reported live 2026-08-24).
//
// Display only, and only until the child's first report: a session's child
// decides who plays, and its mirror carries its own count.
func JoinableSlots(session Session) int {
	free := 0

	for pad := 2; pad <= session.MaxPlayers; pad++ {
		if _, taken := session.PadUsers[pad]; !taken {
			free++
		}
	}

	return free
}

// Subscribe/Unsubscribe feed the façade WebSocket: a signal (never any
// payload — the handler re-reads List() on every push) fires on every
// registry change.
func (pool *Pool) Subscribe() chan struct{} {
	channel := make(chan struct{}, 1)
	pool.mu.Lock()
	pool.subscribers[channel] = true
	pool.mu.Unlock()
	return channel
}

func (pool *Pool) Unsubscribe(channel chan struct{}) {
	pool.mu.Lock()
	delete(pool.subscribers, channel)
	pool.mu.Unlock()
}

func (pool *Pool) notify() {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for subscriber := range pool.subscribers {
		select {
		case subscriber <- struct{}{}:
		default: // already has a pending notification — a burst of changes only needs one re-read
		}
	}
}

// Notify wakes every subscriber without a pool mutation — used by the
// façade's activity reader so an input stamp reaches all session sockets
// (P6: lastEventReceived) instead of waiting for the next registry change.
func (pool *Pool) Notify() {
	pool.notify()
}
