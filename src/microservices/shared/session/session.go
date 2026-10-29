// Package session is the session authority, embedded in every runtime child
// (chantier-4). A session belongs to the container that runs it: the child
// decides, the kernel only mirrors what it is told.
//
// Why it lives here and not in the kernel: every serious bug of 2026-08-24/25
// had the same shape — TWO copies of the same truth drifting apart. The kernel
// had freed a pad while the child still held the port ("session is full" with
// nobody playing); the pool counted zero players while the child had one;
// remoteplay's in-memory map outlived its container. One copy removes the
// class by construction.
//
// This package is deliberately PURE: no network, no docker, no clock of its
// own (callers pass `now`). It is the only part of the chantier that can be
// proven correct without hardware, so it is where the model is settled.
package session

import (
	"errors"
	"time"
)

// Player is one entry of the players queue: somebody who holds a controller
// slot, and the socket they hold it from.
//
// SocketID is the heart of the model. THE WEBSOCKET IS THE IDENTITY: a tab is
// playing if and only if ITS socket is the one in the entry. That removes the
// question v1 spent weeks failing to answer — "is this new socket the same
// person as that dead one?" — which cannot be settled without a token plus a
// grace window, and every grace window is a race. Here there is nothing to
// decide: the socket either is the one in the queue, or it is not.
type Player struct {
	SlotID   int
	Username string
	SocketID string
	JoinedAt time.Time
	// LastBeatAt — last 30s beat, proves the tab's JS still runs (a frozen
	// tab keeps its socket open, so the socket alone cannot tell).
	LastBeatAt time.Time
	// LastInputAt — last beat that carried real input, proves a HUMAN is
	// there. Never bump this from a timer: a periodic tick proves the tab
	// runs, not that anybody is behind it.
	LastInputAt time.Time
	// AFKWarnedAt — quand on a demandé « toujours là ? ». Zéro tant qu'on ne
	// l'a pas demandé.
	//
	// ON PRÉVIENT AVANT D'EXCLURE, et la question est INDIVIDUELLE : c'est ce
	// joueur-là qui n'a pas touché sa manette, pas la tablée. Le compte à
	// rebours qui suit lui laisse le temps de répondre — appuyer sur n'importe
	// quoi suffit, et l'avertissement s'efface tout seul.
	AFKWarnedAt time.Time
	// LentTo, LentToSocket — LE PRÊT (décidé le 2026-10-09, #274).
	//
	// Le propriétaire prête sa manette SANS QUITTER SA PLACE : l'entrée de file
	// reste la sienne, avec son battement et ses droits. L'emprunteur n'est pas
	// un joueur : c'est un « playable viewer », un spectateur dont les touches,
	// envoyées depuis CET onglet (LentToSocket), pilotent ce pad à la place des
	// siennes. Le propriétaire reprend la manette quand il veut, sans
	// confirmation ; l'emprunteur peut la rendre.
	//
	// Porté par la place, et non par une liste à part : quand la place tombe
	// (onglet fermé, gelé, exclu), le prêt tombe avec elle, sans rien à
	// resynchroniser. Deux registres qui doivent rester d'accord, c'est
	// exactement ce qui a fabriqué les défauts de « qui tient la manette ».
	//
	// Vides quand le pad n'est pas prêté. Seul le propriétaire prête : un
	// invité qui passe sa manette donne sa place (Handover).
	LentTo       string
	LentToSocket string
}

// Participant is an open tab, player or not. It exists to answer "who is
// watching, and who stopped" — the socket is the presence, so there is
// nothing to expire.
type Participant struct {
	SocketID string
	Username string // "" — anonymous watcher
}

// Session is the whole truth about one running game.
type Session struct {
	ID            string
	ContainerID   string
	PlatformID    string
	ConsoleID     string
	GameReference string
	StartedAt     time.Time
	Owner         string
	MaxPlayers    int

	Players      []Player
	Participants []Participant

	// IdleSince starts when the queue empties and stops when anybody plays.
	IdleSince *time.Time

	// PendingHandover is the ONE demand awaiting its holder's answer. One at
	// a time, and it times out (HandoverTTL) so a holder who never clicks
	// cannot freeze the session's hand-over path.
	PendingHandover *HandoverRequest

	// Terminated is set by a deliberate end-of-session from the owner. The
	// child stops itself on it; the idle clock is the other way out.
	Terminated bool
	// TerminatedBy — l'administrateur qui a arrêté la partie d'un autre (Live
	// now) ; vide quand c'est son propriétaire. Le journal de la partie le dit
	// au lieu de « owner ended the session ».
	TerminatedBy string
	// GameExited — la partie s'est terminée parce que le JEU s'est arrêté de
	// lui-même : son émulateur est sorti sans que personne ne l'arrête
	// (#504). La page le dit, au lieu de « la session est terminée » — ou,
	// avant, d'une image figée sans un mot.
	GameExited bool
}

var (
	// ErrAlreadyPlaying — invariant 1: one entry per username, never two.
	ErrAlreadyPlaying = errors.New("already in the players queue")
	// ErrNoSlot — invariants 2 and 3: the queue is full for this caller. The
	// owner never sees this: their slot is reserved.
	ErrNoSlot = errors.New("no slot available")
	// ErrNotPlaying — the caller holds no slot.
	ErrNotPlaying = errors.New("not in the players queue")
	// ErrSameSocket — a takeover from the socket that already holds the slot
	// is a no-op, and almost certainly a bug in the caller.
	ErrSameSocket = errors.New("this socket already holds the slot")
	// ErrEmptyUsername — anonymous tabs watch, they never play.
	ErrEmptyUsername = errors.New("a player needs a username")
	// ErrNotOwner — ending the session and writing its save states belong to
	// the owner alone. A joiner is a guest.
	ErrNotOwner = errors.New("only the owner may do that")
	// ErrNotHolder — only the holder being asked may answer a hand-over.
	ErrNotHolder = errors.New("not the holder being asked")
	// ErrNoSuchHolder — the hand-over target holds no slot.
	ErrNoSuchHolder = errors.New("no such slot holder")
	// ErrHandoverPending — one demand at a time, until it is answered or
	// times out.
	ErrHandoverPending = errors.New("a hand-over is already pending")
	// ErrNotLent — taking back a pad that nobody borrowed.
	ErrNotLent = errors.New("the owner's pad is not lent")
	// ErrPadLent — the owner's pad is in somebody else's hands: it is neither
	// lent a second time, nor asked for, nor passed on. Only its owner takes
	// it back.
	ErrPadLent = errors.New("the owner's pad is lent")
	// ErrNotBorrower — only the tab that borrowed the pad gives it back.
	ErrNotBorrower = errors.New("that pad was not lent to this tab")
	// ErrPlaysElsewhere — one person drives ONE game at a time. Said by the
	// kernel, which sees every session; the child asks before seating anybody.
	ErrPlaysElsewhere = errors.New("already playing in another session")
	// ErrUnknownCommand — the command set is closed on purpose.
	ErrUnknownCommand = errors.New("unknown command")
	// ErrSocketClosed — invariant 6: a slot cannot be held from a socket
	// that is not an open tab.
	ErrSocketClosed = errors.New("that socket is not an open tab")
	// ErrNotYourTab — a command speaks for ITS OWN tab. The socket it names
	// must be one its verified user opened: socket ids travel in the clear,
	// and naming somebody else's let one account seat another's tab under
	// its own name (found 2026-10-08).
	ErrNotYourTab = errors.New("that socket belongs to another user")
)

// New builds a session with its owner NOT yet in the queue — the owner joins
// like anybody else, they simply cannot be refused.
func New(id, platformID string, owner string, maxPlayers int, now time.Time) *Session {
	if maxPlayers < 1 {
		maxPlayers = 1
	}

	return &Session{
		ID:         id,
		PlatformID: platformID,
		Owner:      owner,
		MaxPlayers: maxPlayers,
		StartedAt:  now,
		IdleSince:  &now, // nobody is playing yet
	}
}
