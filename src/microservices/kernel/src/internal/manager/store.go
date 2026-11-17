package manager

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"gameflex/kernel/internal/docker"
)

// BaseSession holds the fields every platform's session record shares —
// SessionID/Container/Owner/Port/StartedAt/State were
// redeclared identically in Session, RetroSession and RemoteplaySession
// (confirmed byte-for-byte identical at the field level, see the plan's
// "Refacto complet — store de session générique" doc). Embedding this
// instead of repeating the shared fields does NOT change the JSON wire shape:
// Go flattens an anonymous embedded struct's tagged fields into the
// parent's own JSON object, so `Session{BaseSession: ...}` still marshals
// exactly like the old flat struct did.
type BaseSession struct {
	SessionID string    `json:"session_id"`
	Container string    `json:"container"`
	Owner     string    `json:"owner"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"started_at"`
	State     string    `json:"state"`
}

// hasBase is the one method every platform's session struct implements
// (a one-line `func (s *Session) base() *BaseSession { return &s.BaseSession }`)
// so the generic store below can reach the shared fields without knowing
// anything else about T. base() needs a POINTER receiver (stopSession
// reaches the fields of a copy through it) — Go has no way to say "T's
// pointer type implements this interface" with a single type parameter,
// hence the T/PT pair below (the standard library idiom for exactly this:
// a generic container whose element type's methods live on the pointer).
type hasBase interface {
	base() *BaseSession
}

// sessionStore is the map+mutex every platform's session map used to
// reimplement on its own (manager.sessions/retro/remoteplay), each with
// its own byte-identical Lookup*/Stop*/List*/runningCount* pair. One
// generic type now backs all three; the platform-specific Manager methods
// (LookupRetro, StopStreamex, ...) become thin wrappers around this.
//
// T is the plain struct (Session, RetroSession, RemoteplaySession); PT is
// pinned to *T and constrained to implement hasBase — callers instantiate
// as e.g. sessionStore[Session, *Session].
type sessionStore[T any, PT interface {
	*T
	hasBase
}] struct {
	mu    sync.Mutex
	items map[string]*T
}

func newSessionStore[T any, PT interface {
	*T
	hasBase
}]() *sessionStore[T, PT] {
	return &sessionStore[T, PT]{items: map[string]*T{}}
}

// register inserts or replaces a record, keyed by its own SessionID.
func (s *sessionStore[T, PT]) register(record *T) {
	s.mu.Lock()
	s.items[PT(record).base().SessionID] = record
	s.mu.Unlock()
}

func (s *sessionStore[T, PT]) lookup(sessionID string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.items[sessionID]
	if !ok {
		var zero T
		return zero, false
	}
	return *record, true
}

// list returns copies sorted by start time (oldest first) — deterministic
// order so WS consumers never see the list reshuffle on every push, same
// guarantee the three original List/ListRetro/ListRemoteplay gave.
func (s *sessionStore[T, PT]) list() []T {
	s.mu.Lock()
	records := make([]T, 0, len(s.items))
	for _, record := range s.items {
		records = append(records, *record)
	}
	s.mu.Unlock()

	sort.Slice(records, func(i, j int) bool {
		return PT(&records[i]).base().StartedAt.Before(PT(&records[j]).base().StartedAt)
	})
	return records
}

// take removes and returns a record in one step — of two callers stopping the
// same session at once (a stop asked from the page, and the container's exit
// that Docker reports), only one gets it.
func (s *sessionStore[T, PT]) take(sessionID string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.items[sessionID]
	if !ok {
		var zero T
		return zero, false
	}
	delete(s.items, sessionID)

	return *record, true
}

// runningCount counts running records matching an extra platform-specific
// predicate (same console, same host device...) — the predicate is the
// only part that ever differed between runningCount/runningRetroCount/
// runningRemoteplayCount.
func (s *sessionStore[T, PT]) runningCount(match func(T) bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, record := range s.items {
		if PT(record).base().State == "running" && match(*record) {
			count++
		}
	}
	return count
}

// ownedRunning lists the session IDs a user currently has running — the
// "stop my previous session before spawning a new one" rule, identical
// across streamex/retro (remoteplay has no per-user cap, its capacity is
// per physical device instead — deliberately not given this method, see
// the plan's note on asymmetries to preserve rather than paper over).
func (s *sessionStore[T, PT]) ownedRunning(username string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	owned := make([]string, 0)
	for id, record := range s.items {
		b := PT(record).base()
		if b.Owner == username && b.State == "running" {
			owned = append(owned, id)
		}
	}
	return owned
}

// stopSession is the container-teardown sequence every platform's Stop*
// duplicated verbatim: take the record, stop its container, release its
// port. Reports whether the session existed. The record goes FIRST: the
// removal makes Docker report an exit, and containerExited must then find
// nothing left to stop.
func stopSession[T any, PT interface {
	*T
	hasBase
}](store *sessionStore[T, PT], dockerHandler *docker.Client, ctx context.Context, sessionID, kind string) bool {
	record, ok := store.take(sessionID)
	if !ok {
		return false
	}
	b := PT(&record).base()

	if err := dockerHandler.Stop(ctx, b.Container); err != nil {
		slog.Warn("stopping container failed", "session_id", sessionID, "container", b.Container, "error", err)
	}
	// The container is gone — its TCP port is free again, when it was
	// reserved at adoption (#103).
	dockerHandler.ReleasePort(b.Port)

	slog.Info(kind+" session stopped", "session_id", sessionID)
	return true
}
