package session

import "time"

// The command queue (chantier-4). Requests do NOT ride the tab's websocket —
// that socket carries presence, input and beats. Intents arrive by webhook,
// get pushed here, and are drained ONE AT A TIME.
//
// Serialising is what removes races by construction, and it is why most of
// chantier 3's machinery is gone: the 15s pad reservation, the anti-snipe
// window and its expiry all existed only because two things could happen at
// the same instant. Here they cannot.
//
// Two rules make it safe, and both are about WHEN a command is checked:
//
//   - validate at EXECUTION, never at push. A tab can die while its command
//     waits its turn;
//   - a command whose precondition has changed is ABANDONED, never replayed.
//     ("Z leaves" landing before "Y granted X's request" drops the grant.)

// HandoverTTL — how long a hand-over request waits for its holder to answer.
// Human-paced: see the prompt, read it, click. Without it, a holder who never
// answers freezes the session's hand-over path for good.
const HandoverTTL = 30 * time.Second

// CommandKind is the closed set of things anybody may ask of a session.
type CommandKind string

const (
	CmdJoin            CommandKind = "join"
	CmdLeave           CommandKind = "leave"
	CmdTakeover        CommandKind = "takeover"
	CmdRequestHandover CommandKind = "request-handover"
	CmdGrantHandover   CommandKind = "grant-handover"
	CmdDenyHandover    CommandKind = "deny-handover"
	// CmdGiveHandover — the holder passes the pad to somebody watching,
	// unasked. The other three verbs all start from the person who WANTS the
	// pad; this is the one that starts from the person who HAS it, which is
	// how a controller actually changes hands in a room. remote-play is where
	// its absence showed: one pad, JOIN structurally impossible, so handover
	// is the only way it ever moves — and half the gesture did not exist.
	CmdGiveHandover CommandKind = "give-handover"
	// CmdReclaim — the owner takes their lent pad back ("take controller
	// back"), when they want and without asking; CmdReturn — the borrower
	// gives it back ("return controller back"). The two ends of a loan: the
	// owner's hand-over lends the pad, it never hands over the session (see
	// Session.Handover).
	CmdReclaim   CommandKind = "reclaim"
	CmdReturn    CommandKind = "return"
	CmdTerminate CommandKind = "terminate"
)

// Command is one asked-for change.
//
// Username is the VERIFIED identity (the child checks the JWT before pushing)
// — never a field the client declares about itself. That is the hole
// retro_relay.go had on 2026-08-23: a self-declared username granted full
// control of anybody's session.
type Command struct {
	Kind       CommandKind
	Username   string
	SocketID   string
	Target     string // hand-over: the holder being asked, or the person being given the pad
	EnqueuedAt time.Time
	// ByAdmin — un arrêt demandé par un administrateur (Live now, 2026-10-25) :
	// il passe la règle du propriétaire. Posé par le kernel seul, sur la route
	// interne que garde le secret — jamais par une page.
	ByAdmin bool
}

// Outcome is what became of a command, for answering its sender.
type Outcome struct {
	Command Command
	Slot    int
	// Abandoned — the command was valid when asked and no longer is. Not an
	// error the caller did anything about: the world moved.
	Abandoned bool
	Err       error
}

// HandoverRequest is the one pending demand of a session. One at a time: a
// second one is refused until the first is answered or times out.
type HandoverRequest struct {
	From       string // who asks
	FromSocket string // and from which tab — the slot lands on THIS socket
	To         string // the holder being asked
	AskedAt    time.Time
}

// Queue holds commands until they are drained, in order.
type Queue struct {
	pending []Command
}

// Push appends a command. Nothing is checked here on purpose — see the note
// at the top of this file.
func (queue *Queue) Push(command Command) {
	queue.pending = append(queue.pending, command)
}

// Len reports how many commands are waiting.
func (queue *Queue) Len() int {
	return len(queue.pending)
}

// Drain applies every pending command to the session, in order, and returns
// what became of each. The session is only ever mutated from here, so there
// is exactly one place where the invariants can be broken — and it is
// single-threaded by construction.
func (queue *Queue) Drain(session *Session, now time.Time) []Outcome {
	outcomes := make([]Outcome, 0, len(queue.pending))
	pending := queue.pending
	queue.pending = nil

	for _, command := range pending {
		outcomes = append(outcomes, session.apply(command, now))
	}

	return outcomes
}

// LiveHandoverAt returns the pending request while it still holds, nil once
// it has timed out. Expiry is LAZY — every reader asks, so there is no sweep
// to forget to run and no window where a stale request blocks a new one.
//
// It takes `now` like everything else here: this package owns no clock, which
// is what makes every timing rule testable without sleeping.
func (session *Session) LiveHandoverAt(now time.Time) *HandoverRequest {
	if session.PendingHandover == nil || session.PendingHandover.AskedAt.Add(HandoverTTL).Before(now) {
		return nil
	}

	return session.PendingHandover
}

func (session *Session) apply(command Command, now time.Time) Outcome {
	outcome := Outcome{Command: command}

	// Every command comes from a tab. If that tab is gone, the command is
	// abandoned — not failed: it was legitimate when asked.
	if command.SocketID != "" && !session.hasParticipant(command.SocketID) {
		outcome.Abandoned = true

		return outcome
	}
	// And that tab is the caller's own: a refusal this time, not an
	// abandonment — the command was never legitimate.
	if command.SocketID != "" && command.Username != "" && !session.participantIs(command.SocketID, command.Username) {
		outcome.Err = ErrNotYourTab

		return outcome
	}

	switch command.Kind {
	case CmdJoin:
		slot, err := session.Join(command.Username, command.SocketID, now)
		outcome.Slot, outcome.Err = slot, err

	case CmdLeave:
		outcome.Err = session.Leave(command.Username, now)

	case CmdTakeover:
		// Open to ANY player, not just the owner: a joiner whose entry sits
		// on another tab is otherwise trapped — they cannot take it back, and
		// they cannot re-join either, because the queue refuses a second
		// entry for the same username.
		slot, err := session.Takeover(command.Username, command.SocketID, now)
		outcome.Slot, outcome.Err = slot, err

	case CmdRequestHandover:
		outcome.Err = session.requestHandover(command, now)

	case CmdGrantHandover, CmdDenyHandover:
		outcome = session.answerHandover(command, now, outcome)

	case CmdGiveHandover:
		// A gift settles any demand that was waiting on this holder: they no
		// longer hold the pad, so an answer from them would mean nothing.
		// Dropped rather than left to expire — a prompt still on screen for a
		// pad that has moved is a lie the page would act on.
		if pending := session.LiveHandoverAt(now); pending != nil && pending.To == command.Username {
			session.PendingHandover = nil
		}

		slot, err := session.GiveTo(command.Username, command.SocketID, command.Target, now)
		outcome.Slot, outcome.Err = slot, err

	case CmdReclaim:
		slot, err := session.Reclaim(command.Username, command.SocketID, now)
		outcome.Slot, outcome.Err = slot, err
		session.dropStaleHandover(now)

	case CmdReturn:
		slot, err := session.GiveBack(command.Username, command.SocketID, now)
		outcome.Slot, outcome.Err = slot, err

	case CmdTerminate:
		if !command.ByAdmin && !session.IsOwner(command.Username) {
			outcome.Err = ErrNotOwner
			break
		}
		session.Terminated = true
		if !session.IsOwner(command.Username) {
			session.TerminatedBy = command.Username
		}

	default:
		outcome.Err = ErrUnknownCommand
	}

	return outcome
}

// dropStaleHandover — a demand addressed to somebody who no longer holds a pad
// means nothing: their answer would hand over what they do not have. Dropped
// rather than left to expire — a prompt still on screen for a pad that has
// moved is a lie the page would act on.
func (session *Session) dropStaleHandover(now time.Time) {
	if pending := session.LiveHandoverAt(now); pending != nil && session.PlayerOf(pending.To) == nil {
		session.PendingHandover = nil
	}
}

func (session *Session) requestHandover(command Command, now time.Time) error {
	if command.Target == "" || command.Target == command.Username {
		return ErrNoSuchHolder
	}
	if session.LiveHandoverAt(now) != nil {
		return ErrHandoverPending
	}
	if session.PlayerOf(command.Target) == nil {
		return ErrNoSuchHolder
	}
	// A lent pad is not the owner's to give right now, and not the
	// borrower's either: only its owner takes it back.
	if session.IsOwner(command.Target) && session.Lent() {
		return ErrPadLent
	}

	session.PendingHandover = &HandoverRequest{
		From:       command.Username,
		FromSocket: command.SocketID,
		To:         command.Target,
		AskedAt:    now,
	}

	return nil
}

func (session *Session) answerHandover(command Command, now time.Time, outcome Outcome) Outcome {
	request := session.LiveHandoverAt(now)
	if request == nil {
		// Asked, then timed out before the holder got to it. Nothing failed.
		outcome.Abandoned = true

		return outcome
	}
	// Only the holder being asked may answer. Anybody else is a no-op, not a
	// silent grant.
	if request.To != command.Username {
		outcome.Err = ErrNotHolder

		return outcome
	}

	session.PendingHandover = nil

	if command.Kind == CmdDenyHandover {
		return outcome
	}

	// The requester's tab may have closed while the holder was deciding, or
	// the holder may have lost the slot meanwhile. Either way the world moved:
	// abandon, never replay.
	if !session.hasParticipant(request.FromSocket) || session.PlayerOf(request.To) == nil {
		outcome.Abandoned = true

		return outcome
	}

	slot, err := session.Handover(request.To, request.From, request.FromSocket, now)
	outcome.Slot, outcome.Err = slot, err

	return outcome
}
