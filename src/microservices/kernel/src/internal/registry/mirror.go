package registry

import (
	"sync"
	"time"
)

// The MIRROR (chantier-4). A runtime child owns its session and tells the
// kernel what it decided; the kernel stores it here and shows it. It never
// decides from it.
//
// That distinction is the whole point, and it has to be defended in code
// rather than in a comment somewhere: the topology "the child pushes, the
// kernel serves the façade" already existed before chantier-4 and is not what
// cost us. What cost us is that the kernel MIRRORED AND DECIDED — RefreshRoles,
// ClaimPad, the idle watchdog — so a mirror drifting from the child's truth
// turned into a behavioural bug instead of a stale number.
//
// Hence the rule: THE MIRROR MAY LIE, AN ACTION NEVER DOES. The Dashboard may
// show a JOIN that gets refused a second later; the refusal comes from the
// child, which is right. That is what lets this be asynchronous at all.

// MirrorPlayer is one queue entry as the child reported it. No socket ids:
// they are the child's business and nobody else's.
type MirrorPlayer struct {
	SlotID   int    `json:"slotId"`
	Username string `json:"username"`
	// LentTo — who drives this pad while its owner lends it (#274): a
	// "playable viewer", with no place of their own but a pad in hand.
	LentTo string `json:"lentTo,omitempty"`
}

// MirrorSnapshot is one session, as its child last described it.
type MirrorSnapshot struct {
	Owner          string         `json:"owner"`
	MaxPlayers     int            `json:"maxPlayers"`
	Players        []MirrorPlayer `json:"players"`
	AvailableSlots int            `json:"availableSlots"`
	IdleSince      *time.Time     `json:"idleSince,omitempty"`
	Terminated     bool           `json:"terminated"`
	// ReceivedAt is stamped by the kernel, not the child: it is how stale
	// this copy is, and only the receiver can know that honestly.
	ReceivedAt time.Time `json:"-"`
}

// Mirrors holds one snapshot per v4-managed session.
type Mirrors struct {
	mu   sync.Mutex
	byID map[string]MirrorSnapshot
}

func NewMirrors() *Mirrors {
	return &Mirrors{byID: make(map[string]MirrorSnapshot)}
}

// Put records what a child reported.
func (mirrors *Mirrors) Put(sessionID string, snapshot MirrorSnapshot) {
	mirrors.mu.Lock()
	defer mirrors.mu.Unlock()

	snapshot.ReceivedAt = time.Now()
	mirrors.byID[sessionID] = snapshot
}

// Get returns the snapshot and whether this session is child-managed at all.
// A session with no mirror is one the kernel still owns the old way — both
// models coexist while the migration runs.
func (mirrors *Mirrors) Get(sessionID string) (MirrorSnapshot, bool) {
	mirrors.mu.Lock()
	defer mirrors.mu.Unlock()

	snapshot, ok := mirrors.byID[sessionID]

	return snapshot, ok
}

// Drop forgets a session. Called when its container goes: a mirror of
// something that no longer exists is exactly the phantom we keep paying for
// (remoteplay's "Someone is connected" on a console that was switched off).
func (mirrors *Mirrors) Drop(sessionID string) {
	mirrors.mu.Lock()
	defer mirrors.mu.Unlock()

	delete(mirrors.byID, sessionID)
}

// RetainOnly forgets every mirror whose session is no longer in the given
// list. Called on the watchdog tick because a container can die without
// anybody calling Drop, and a mirror that outlives its session is precisely
// the class of phantom that showed "Someone is connected" on a PlayStation
// that was switched off.
func (mirrors *Mirrors) RetainOnly(sessions []Session) {
	alive := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		alive[session.ID] = true
	}

	mirrors.mu.Lock()
	defer mirrors.mu.Unlock()

	for sessionID := range mirrors.byID {
		if !alive[sessionID] {
			delete(mirrors.byID, sessionID)
		}
	}
}
