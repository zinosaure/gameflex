package session

import (
	"fmt"
	"sync"
	"time"
)

// Host wraps a Session and its command Queue with the one thing neither of
// them has: a lock, and therefore a guarantee that commands really are
// applied one at a time.
//
// It is still transport-agnostic. The child owns the websockets and the HTTP
// routes; it hands this type the events (a tab opened, a beat arrived, an
// intent was posted) and supplies a Send. Keeping the decisions here rather
// than in the child's handlers is what lets the whole model be tested without
// a socket — and what stops three runtimes from each growing their own
// slightly different copy of these rules, which is how we got here.
//
// Messages are collected under the lock and sent AFTER releasing it: a Send
// that blocks (a slow client, a full buffer) must never freeze the session
// for everybody else.
type Host struct {
	mu      sync.Mutex
	session *Session
	queue   Queue
	nextID  uint64
	// lastClosed — pourquoi le dernier balayage a fermé ce qu'il a fermé.
	// Lu par l'hôte HTTP pour le journaliser (voir ClosedPlayer).
	lastClosed []ClosedPlayer

	// send delivers one message to one socket, and REPORTS FAILURE. The
	// error matters: a write that fails is the cheapest and most definitive
	// proof a socket is gone — see deliver.
	send func(socketID string, message map[string]any) error

	// OnChange fires after every mutation, outside the lock. The child uses
	// it to report to the kernel's mirror.
	//
	// A hook rather than a call at each site: there are five places state can
	// change, and "remember to report here too" is the kind of instruction
	// that gets forgotten on the sixth.
	OnChange func()

	// OnPadHandover fires, outside the lock, for each pad whose HANDS just
	// changed — a lend, a take-back, a return, a place given, a departure, an
	// exclusion, a frozen tab dropped. The runtime puts that pad back to
	// neutral: a button the previous hands were holding must not stay held
	// for the next ones, or for nobody (found on streamex, 2026-10-09).
	//
	// Decided HERE, once, for every runtime: the queue is the one place that
	// knows every way a pad can change hands, and "remember to release it on
	// this path too" is exactly the instruction that gets forgotten.
	OnPadHandover func(slot int)
	// controllers — who drove each pad the last time it was asked.
	controllers map[int]string
}

// NewHost binds a session to a transport.
func NewHost(session *Session, send func(socketID string, message map[string]any) error) *Host {
	return &Host{session: session, send: send}
}

// Session exposes the underlying session for read-only reporting (the mirror
// pushed to the kernel). Callers must not mutate it — every change goes
// through a command.
func (host *Host) Session() *Session {
	host.mu.Lock()
	defer host.mu.Unlock()

	return host.session
}

// Attach registers an open tab and tells it who it is. The socket id is what
// the tab will put in its webhook calls, so it has to learn it — and it is
// the ONLY socket id it ever sees: nobody is told anybody else's.
func (host *Host) Attach(socketID, username string) {
	host.mu.Lock()
	host.session.OpenTab(socketID, username)
	messages := host.stateMessages()
	host.mu.Unlock()

	host.send(socketID, map[string]any{"type": "welcome", "socketId": socketID})
	host.deliver(messages)
	host.changed()
}

// Detach is a dead socket: v1's central rule, "socket mort = dehors". It
// releases the slot only when the dead socket was the one holding it, so a
// player closing their OTHER tab keeps playing.
func (host *Host) Detach(socketID string, now time.Time) {
	host.mu.Lock()
	host.session.DropSocket(socketID, now)
	messages := host.stateMessages()
	host.mu.Unlock()

	host.deliver(messages)
	host.changed()
}

// Beat records liveness. The message arriving proves the tab's JS runs;
// hadInput proves a human is there. Never conflate the two.
func (host *Host) Beat(socketID string, hadInput bool, now time.Time) {
	host.mu.Lock()
	host.session.Beat(socketID, hadInput, now)
	host.mu.Unlock()
}

// Submit is the webhook path: an intent is pushed and drained under the same
// lock, so it is applied strictly after everything already queued.
//
// It returns a requestId immediately. The OUTCOME does not come back here —
// it rides the requester's websocket, because the resulting state change has
// several recipients and must not exist in two copies. See the chantier's
// "requête par webhook, réponse par socket".
func (host *Host) Submit(command Command, now time.Time) string {
	requestID, _ := host.submit(command, now)

	return requestID
}

// Do applies one command and returns what became of it — for a caller with no
// socket to hear the answer on: the kernel relaying the dashboard's stop (P2),
// which must answer its own HTTP request.
func (host *Host) Do(command Command, now time.Time) Outcome {
	_, outcomes := host.submit(command, now)
	for _, outcome := range outcomes {
		if outcome.Command.Kind == command.Kind && outcome.Command.Username == command.Username {
			return outcome
		}
	}

	return Outcome{Command: command, Abandoned: true}
}

// Borrower — who drives the owner's lent pad, "" when nobody borrowed it,
// read under the lock.
func (host *Host) Borrower() string {
	host.mu.Lock()
	defer host.mu.Unlock()

	return host.session.Borrower()
}

// Owner — who owns the session, read under the lock.
func (host *Host) Owner() string {
	host.mu.Lock()
	defer host.mu.Unlock()

	return host.session.Owner
}

// GameExited — LE JEU S'EST ARRÊTÉ DE LUI-MÊME (#504) : son émulateur est
// sorti, personne n'a mis fin à la partie. Elle se termine comme sur l'« exit »
// de son propriétaire, et l'état dit pourquoi, à chaque onglet. Pas une
// commande : aucun onglet ne peut l'envoyer, seul le processus qui voit
// l'émulateur mourir.
func (host *Host) GameExited() {
	host.mu.Lock()
	host.session.Terminated = true
	host.session.GameExited = true
	messages := host.stateMessages()
	host.mu.Unlock()
	host.deliver(messages)
	host.changed()
}

// Terminated — has the owner ended the session, read under the lock.
func (host *Host) Terminated() bool {
	host.mu.Lock()
	defer host.mu.Unlock()

	return host.session.Terminated
}

// PendingFrom — who is waiting for a pad from this holder, "" when nobody is.
// The child asks the kernel about THEM before letting a grant seat them.
func (host *Host) PendingFrom(holder string, now time.Time) string {
	host.mu.Lock()
	defer host.mu.Unlock()

	if pending := host.session.LiveHandoverAt(now); pending != nil && pending.To == holder {
		return pending.From
	}

	return ""
}

func (host *Host) submit(command Command, now time.Time) (string, []Outcome) {
	host.mu.Lock()

	host.nextID++
	requestID := fmt.Sprintf("req-%d", host.nextID)
	command.EnqueuedAt = now
	host.queue.Push(command)

	outcomes := host.queue.Drain(host.session, now)
	messages := host.stateMessages()

	// The result message is FOR THE HUMAN — a toast. The interface derives
	// its state from the state push alone, never from this. A message that
	// drives nothing cannot break anything by going missing, which is
	// exactly how chantier 3's requestResult broke a hand-over.
	for _, outcome := range outcomes {
		if outcome.Command.SocketID == "" {
			continue
		}

		result := map[string]any{
			"type":      "result",
			"requestId": requestID,
			"action":    string(outcome.Command.Kind),
			"status":    outcomeStatus(outcome),
		}
		if outcome.Err != nil {
			result["reason"] = outcome.Err.Error()
		}
		messages = append(messages, addressed{socketID: outcome.Command.SocketID, message: result})

		// A GIFT has a second human in it, and only one of the two asked for
		// anything. Without this the recipient finds a pad in their hands with
		// no idea where it came from — the state push moves the slot, it does
		// not say who held it out. Addressed to the socket that now holds it,
		// so a second tab of theirs is not told it is playing.
		if outcome.Command.Kind == CmdGiveHandover && outcome.Err == nil && !outcome.Abandoned {
			if socketID := host.recipientSocket(outcome.Command.Target); socketID != "" {
				messages = append(messages, addressed{
					socketID: socketID,
					message: map[string]any{
						"type":   "result",
						"action": string(CmdGiveHandover),
						"status": "received",
						"from":   outcome.Command.Username,
						// A loan or a place: the recipient's page says which.
						"lent": host.session.Borrower() == outcome.Command.Target,
					},
				})
			}
		}
	}

	host.mu.Unlock()
	host.deliver(messages)
	host.changed()

	return requestID, outcomes
}

// recipientSocket — the tab a pad just landed on: the place the recipient now
// holds, or, for a loan, the tab that borrowed it. Callers hold the lock.
func (host *Host) recipientSocket(username string) string {
	if loan := host.session.loan(); loan != nil && loan.LentTo == username {
		return loan.LentToSocket
	}
	if player := host.session.PlayerOf(username); player != nil {
		return player.SocketID
	}

	return ""
}

// ControlledSlot — which pad this tab's keys drive, read under the lock (see
// Session.ControlledSlot).
func (host *Host) ControlledSlot(socketID string) int {
	host.mu.Lock()
	defer host.mu.Unlock()

	return host.session.ControlledSlot(socketID)
}

// ControllerOf — the tab whose keys drive this pad, read under the lock (see
// Session.ControllerOf).
func (host *Host) ControllerOf(slot int) string {
	host.mu.Lock()
	defer host.mu.Unlock()

	return host.session.ControllerOf(slot)
}

// Refuse answers a command that is NOT pushed — the kernel said its recipient
// already drives another game (ErrPlaysElsewhere). Same `result` as a refusal
// from the queue, on the same socket: the page tells no difference between a
// "no" from the child and a "no" the child relayed.
func (host *Host) Refuse(command Command, err error) {
	if command.SocketID == "" {
		return
	}

	host.mu.Lock()
	host.nextID++
	requestID := fmt.Sprintf("req-%d", host.nextID)
	host.mu.Unlock()

	host.deliver([]addressed{{socketID: command.SocketID, message: map[string]any{
		"type":      "result",
		"requestId": requestID,
		"action":    string(command.Kind),
		"status":    "refused",
		"reason":    err.Error(),
	}}})
}

// Sweep applies the time-based rules and returns the sockets the child should
// close. Called on a ticker by the child; it owns no clock itself so every
// rule stays testable without sleeping.
//
//   - a player whose tab stopped beating is gone (catches the FROZEN tab,
//     whose socket stays open while its JS no longer runs — the socket alone
//     cannot tell);
//   - a player with no real input past afkAfter is asked, then kicked.
//
// THE OWNER IS KICKED LIKE ANYBODY ELSE, AND THE GAME GOES ON. It used to end
// the session — but the others may be playing very well, and an owner who
// drifted away is in the same place as one whose connection dropped: their
// pad stays reserved (slot 1), and the game is idle only once NOBODY plays
// (touchIdle). Rule set by the owner of the project on 2026-10-09: "if
// activePlayers > 0 then continue, else idle". Only a deliberate terminate
// ends a game for everybody at once.
//
// Sweep rend aussi les joueurs à PRÉVENIR — voir AFKWarning.
func (host *Host) Sweep(now time.Time, beatTimeout, afkAfter, afkGrace time.Duration) ([]string, []AFKWarning) {
	host.mu.Lock()

	closing := []string{}
	reasons := []ClosedPlayer{}
	warnings := []AFKWarning{}

	for _, player := range host.session.Players {
		switch {
		case now.Sub(player.LastBeatAt) > beatTimeout:
			closing = append(closing, player.SocketID)
			reasons = append(reasons, ClosedPlayer{
				SocketID: player.SocketID, Username: player.Username,
				Why: "onglet gelé — plus de battement depuis " + now.Sub(player.LastBeatAt).Round(time.Second).String(),
			})

		// IL A RÉPONDU : toucher n'importe quoi efface l'avertissement.
		case !player.AFKWarnedAt.IsZero() && player.LastInputAt.After(player.AFKWarnedAt):
			host.session.ClearAFKWarning(player.SocketID)

		// Le compte à rebours est écoulé sans un geste — c'est seulement
		// MAINTENANT qu'on exclut.
		case !player.AFKWarnedAt.IsZero() && now.Sub(player.AFKWarnedAt) > afkGrace:
			closing = append(closing, player.SocketID)
			reasons = append(reasons, ClosedPlayer{
				SocketID: player.SocketID, Username: player.Username,
				Why:   "sans réponse à « toujours là ? » après " + afkGrace.String(),
				Owner: host.session.IsOwner(player.Username),
			})

		// Plus d'entrée depuis longtemps : ON DEMANDE, on n'exclut pas.
		// Individuel, parce que c'est ce joueur-là qui n'a rien touché — les
		// autres jouent peut-être très bien.
		case player.AFKWarnedAt.IsZero() && now.Sub(player.LastInputAt) > afkAfter:
			host.session.MarkAFKWarned(player.SocketID, now)
			warnings = append(warnings, AFKWarning{
				SocketID: player.SocketID,
				Username: player.Username,
				Deadline: now.Add(afkGrace),
			})
			// PENDANT UN PRÊT, LA QUESTION VA AUX DEUX : à l'emprunteur, dont
			// les touches font tourner ce compteur, et au propriétaire, qui
			// peut reprendre sa manette. Un geste de l'un ou de l'autre y
			// répond ; sans réponse, c'est le cas classique — la place est
			// exclue, et le prêt avec elle.
			if player.LentTo != "" {
				warnings = append(warnings, AFKWarning{
					SocketID: player.LentToSocket,
					Username: player.LentTo,
					Deadline: now.Add(afkGrace),
				})
			}
		}
	}

	for _, socketID := range closing {
		host.session.DropSocket(socketID, now)
	}
	messages := host.stateMessages()

	host.mu.Unlock()
	host.deliver(messages)
	host.changed()

	host.lastClosed = reasons

	return closing, warnings
}

// ClosedPlayer — qui a été fermé, et POURQUOI.
//
// Le balayage ne rendait que des identifiants de socket : les journaux
// disaient « fermé », jamais « gelé » ou « parti sans répondre ». Sur un défaut
// qui met une demi-heure à se produire, c'est la différence entre comprendre
// et refaire l'enquête.
type ClosedPlayer struct {
	SocketID string
	Username string
	Why      string
	Owner    bool
}

// LastClosed rend les raisons du dernier balayage.
func (host *Host) LastClosed() []ClosedPlayer {
	host.mu.Lock()
	defer host.mu.Unlock()

	return host.lastClosed
}

// AFKWarning — la question posée à UN joueur, et jusqu'à quand il peut y
// répondre.
type AFKWarning struct {
	SocketID string
	Username string
	Deadline time.Time
}

// --- state ---------------------------------------------------------------

type addressed struct {
	socketID string
	message  map[string]any
}

// stateMessages builds ONE message per open tab. It is per-socket on purpose:
// "am I playing" is a question about THIS tab, and answering it here means the
// page never has to compare socket ids itself — nor is any tab ever told
// another's socket id.
//
// Callers must hold the lock; delivery happens after releasing it.
func (host *Host) stateMessages() []addressed {
	players := make([]map[string]any, 0, len(host.session.Players))
	for _, player := range host.session.Players {
		players = append(players, map[string]any{
			"slotId":   player.SlotID,
			"username": player.Username,
			"isOwner":  host.session.IsOwner(player.Username),
			// Who drives this pad while it is lent — "" when its holder does.
			"lentTo": player.LentTo,
		})
	}

	base := map[string]any{
		"type":      "state",
		"sessionId": host.session.ID,
		"owner":     host.session.Owner,
		"players":   players,
		// Who is here WITHOUT a pad. The page needs it to offer the other
		// direction of a hand-over — "give the pad to…" — which had no list to
		// draw from, only `players`. One entry per person, not per tab.
		"watchers":       host.session.Watchers(),
		"availableSlots": host.session.AvailableSlots(),
		"terminated":     host.session.Terminated,
		// Why, when nobody ended it: the game itself stopped (#504).
		"gameExited": host.session.GameExited,
		// Who drives the owner's pad, "" when nobody borrowed it. The owner
		// stays the owner (see Session.Handover).
		"lentTo": host.session.Borrower(),
	}
	if host.session.IdleSince != nil {
		base["idleSince"] = host.session.IdleSince.UTC().Format(time.RFC3339)
	}
	if pending := host.session.PendingHandover; pending != nil {
		base["pendingHandover"] = map[string]any{"from": pending.From, "to": pending.To}
	}

	out := make([]addressed, 0, len(host.session.Participants))
	for _, participant := range host.session.Participants {
		message := make(map[string]any, len(base)+1)
		for key, value := range base {
			message[key] = value
		}

		playing := host.session.IsPlaying(participant.Username, participant.SocketID)
		owner := host.session.IsOwner(participant.Username)
		loan := host.session.loan()
		you := map[string]any{
			"username":  participant.Username,
			"isPlaying": playing,
			"isOwner":   owner,
			// WHICH PAD THIS TAB'S KEYS DRIVE, 0 for none. It differs from
			// slotId during a loan only: the owner keeps their place and drives
			// nothing, the borrower — a "playable viewer" — holds no place and
			// drives the owner's pad. The page enables its controller on this,
			// never on isPlaying.
			"controls": host.session.ControlledSlot(participant.SocketID),
			// THE TWO ENDS OF A LOAN, said by the child rather than worked out
			// by the page: the owner may take their pad back, the borrower —
			// from the tab that borrowed it — may give it back.
			"canReclaim": owner && loan != nil,
			"canReturn":  loan != nil && loan.LentToSocket == participant.SocketID,
		}
		if playing {
			you["slotId"] = host.session.PlayerOf(participant.Username).SlotID
		}
		message["you"] = you

		out = append(out, addressed{socketID: participant.SocketID, message: message})
	}

	return out
}

// deliver sends, and treats a FAILED WRITE as a death.
//
// A ping-then-insert guard was considered and rejected: waiting for a pong
// means either holding the lock through the timeout — freezing the session for
// everybody — or leaving it and coming back, which reopens the very window
// serialising the queue had closed. And a pong proves little anyway: the
// browser's network layer answers it, so a FROZEN tab pongs perfectly.
//
// A failed write costs nothing, needs no round-trip and no waiting, and is
// definitive. Since a state message goes out after every command, it shrinks
// the phantom window from "one beat timeout" to "the next message we try to
// send".
//
// It is not a complete answer, and nothing here pretends otherwise: TCP
// accepts writes into its send buffer for a while after the peer is gone, so
// a dead socket can still swallow a few messages. That residue is what the
// beat sweep is for. Three layers, each catching what the one before cannot:
// known departures (Detach), failed writes (here), and silence (Sweep).
func (host *Host) deliver(messages []addressed) {
	if host.send == nil {
		return
	}

	dead := map[string]bool{}
	for _, entry := range messages {
		if err := host.send(entry.socketID, entry.message); err != nil {
			dead[entry.socketID] = true
		}
	}
	if len(dead) == 0 {
		return
	}

	// Dropping a dead socket produces a fresh state for everybody left, which
	// may itself reveal more dead sockets. The set only ever shrinks, so this
	// terminates — but it walks one generation at a time rather than
	// recursing blindly.
	host.mu.Lock()
	now := time.Now()
	for socketID := range dead {
		host.session.DropSocket(socketID, now)
	}
	messages = host.stateMessages()
	host.mu.Unlock()

	host.deliver(messages)
}

// changed fires OnPadHandover for every pad that changed hands, then
// OnChange — never under the lock: the child reports over the network from
// it, and a slow kernel must not freeze the session.
func (host *Host) changed() {
	if host.OnPadHandover != nil {
		for _, slot := range host.padsThatChangedHands() {
			host.OnPadHandover(slot)
		}
	}
	if host.OnChange != nil {
		host.OnChange()
	}
}

// padsThatChangedHands compares who drives each pad now with who drove it
// the last time this was asked. The sweep calls changed() on every tick, so
// a drop that happened without one — a write that failed mid-delivery — is
// still caught within a tick.
func (host *Host) padsThatChangedHands() []int {
	host.mu.Lock()
	defer host.mu.Unlock()

	current := make(map[int]string, host.session.MaxPlayers)
	for slot := 1; slot <= host.session.MaxPlayers; slot++ {
		if socketID := host.session.ControllerOf(slot); socketID != "" {
			current[slot] = socketID
		}
	}

	changed := []int{}
	for slot := 1; slot <= host.session.MaxPlayers; slot++ {
		if current[slot] != host.controllers[slot] {
			changed = append(changed, slot)
		}
	}
	host.controllers = current

	return changed
}

// Claim re-owns the session for a NEW game.
//
// A host that runs one game at a time — the phone, where a libretro core is a
// library with global state and the engine holds one per process — reuses
// the same session object from launch to launch. Its owner and its pad count
// belong to the GAME being started, not to whoever booted the runtime.
//
// Without this, the phone's session kept the owner it was configured with at
// startup ("root"): a player who launched a game under their own account
// watched it appear owned by somebody else, with the stop button out of
// reach. The page used to hide the mistake by declaring itself owner no
// matter what — until that lie was removed (chantier 8, C8-P1) and the
// truth underneath showed.
//
// The queue is emptied on purpose: a new game means nobody holds a pad yet.
// Open tabs are KEPT — their sockets are still live, and a tab that stays
// through a launch should keep talking, it just no longer holds a seat.
func (host *Host) Claim(owner string, maxPlayers int, now time.Time) {
	host.mu.Lock()
	host.session.Owner = owner
	if maxPlayers > 0 {
		host.session.MaxPlayers = maxPlayers
	}
	host.session.Players = nil
	host.session.PendingHandover = nil
	host.session.Terminated = false
	host.session.TerminatedBy = ""
	host.session.GameExited = false
	host.session.StartedAt = now
	host.session.IdleSince = &now
	messages := host.stateMessages()
	host.mu.Unlock()

	host.deliver(messages)
	host.changed()
}

// Snapshot describes the session for the kernel's mirror. Deliberately
// without socket ids: they are the child's business, and nobody outside needs
// to address a tab it does not own.
type Snapshot struct {
	Owner      string `json:"owner"`
	MaxPlayers int    `json:"maxPlayers"`
	// StartedAt — depuis quand cette partie tourne. L'hôte le savait déjà
	// (host.session.StartedAt, posé au démarrage du jeu) mais ne le disait à
	// personne : le panneau de performances du téléphone n'avait donc aucun
	// moyen d'afficher « En cours depuis », alors que la même ligne existe
	// côté serveur. Un instant, jamais une durée — voir sessionstatus.Status,
	// qui explique pourquoi.
	StartedAt time.Time       `json:"startedAt,omitempty"`
	Players   []SnapshotEntry `json:"players"`
	// Watchers — ceux qui sont là sans tenir de place, une fois par nom
	// (Session.Watchers). La file connaît chaque onglet ouvert : c'est elle
	// qui dit qui regarde, comme elle dit qui joue.
	Watchers       []string   `json:"watchers,omitempty"`
	AvailableSlots int        `json:"availableSlots"`
	IdleSince      *time.Time `json:"idleSince,omitempty"`
	Terminated     bool       `json:"terminated"`
}

// SnapshotEntry is one queue entry, reduced to what anybody else may know.
type SnapshotEntry struct {
	SlotID   int    `json:"slotId"`
	Username string `json:"username"`
	// LentTo — who drives this pad while its owner lends it. The kernel counts
	// them as playing: one person, one pad, here or anywhere else.
	LentTo string `json:"lentTo,omitempty"`
}

// Snapshot builds the report. Taken under the lock so it is never a torn read
// of a queue being mutated.
func (host *Host) Snapshot() Snapshot {
	host.mu.Lock()
	defer host.mu.Unlock()

	players := make([]SnapshotEntry, 0, len(host.session.Players))
	for _, player := range host.session.Players {
		players = append(players, SnapshotEntry{SlotID: player.SlotID, Username: player.Username, LentTo: player.LentTo})
	}

	return Snapshot{
		StartedAt:      host.session.StartedAt,
		Owner:          host.session.Owner,
		MaxPlayers:     host.session.MaxPlayers,
		Players:        players,
		Watchers:       host.session.Watchers(),
		AvailableSlots: host.session.AvailableSlots(),
		IdleSince:      host.session.IdleSince,
		Terminated:     host.session.Terminated,
	}
}

func outcomeStatus(outcome Outcome) string {
	switch {
	case outcome.Abandoned:
		// Not a failure: it was legitimate when asked, the world moved. Worth
		// its own word so the page can say the right thing instead of
		// inventing a reason.
		return "abandoned"
	case outcome.Err != nil:
		return "refused"
	default:
		return "granted"
	}
}
