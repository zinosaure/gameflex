package session

import "time"

// IsPlaying is THE rule the interface hangs on (chantier-4, frontend):
//
//	isWatching = true, sauf si username ∈ queue ET le socket de CET onglet
//	             est celui de l'entrée
//
// A second tab of the same player watches by construction — it cannot be the
// socket of an entry that holds only one. No role to interpret, no election,
// no "which tab is driving": the question answers itself.
func (session *Session) IsPlaying(username, socketID string) bool {
	player := session.PlayerOf(username)

	return player != nil && player.SocketID == socketID
}

// PlayerOf returns the queue entry for a username, nil when absent.
func (session *Session) PlayerOf(username string) *Player {
	for index := range session.Players {
		if session.Players[index].Username == username {
			return &session.Players[index]
		}
	}

	return nil
}

// IsOwner — ownership is a plain field, never a computed role, and it NEVER
// MOVES. A hand-over lends the pad, not the session (decided by the owner of
// the project on 2026-10-09): the saves stay the owner's, and on a phone the
// one administrator stays the one owner. Who drives slot 1 and who owns the
// session are two questions — see Lent.
func (session *Session) IsOwner(username string) bool {
	return session.hasOwner() && username != "" && username == session.Owner
}

// hasOwner — is there a real person behind Owner? Empty means the spawner
// never knew who was asking. "anonymous" is treated the same on purpose: it
// is not a user, it is the absence of one wearing a name, and letting it act
// as a user is what reserved a slot for a ghost.
func (session *Session) hasOwner() bool {
	return session.Owner != "" && session.Owner != "anonymous"
}

// ownerIsPlaying reports whether the owner currently holds a slot. Invariant
// 3 hangs on it: while they do not, one slot stays held for them.
func (session *Session) ownerIsPlaying() bool {
	return session.PlayerOf(session.Owner) != nil
}

// hasParticipant is invariant 6: A SLOT IS ONLY EVER HELD FROM AN OPEN TAB.
//
// Checked inside the queue mutations themselves, not only in the command
// layer: an entry whose socket is gone is a phantom holding a pad nobody can
// take, which is the exact shape of the bug v1 spent days on. Making it an
// invariant of the structure means no caller can create one, whatever path
// they came through.
func (session *Session) hasParticipant(socketID string) bool {
	for _, participant := range session.Participants {
		if participant.SocketID == socketID {
			return true
		}
	}

	return false
}

// participantIs reports whether that open tab was opened by username — the
// identity its socket authenticated with, never what a command says.
func (session *Session) participantIs(socketID, username string) bool {
	for _, participant := range session.Participants {
		if participant.SocketID == socketID {
			return participant.Username == username
		}
	}

	return false
}

// capacityFor is invariant 3, in one place.
//
// The owner is never refused: their slot is reserved, so uniqueness is the
// only rule that applies to them. Everybody else may fill the queue up to
// MaxPlayers, minus the seat kept for an absent owner.
//
// The consequence worth naming: on a ONE-pad console the joiner capacity is
// zero, always. Such a session can only ever be watched — and that falls out
// of the invariant, without a line of per-platform code.
func (session *Session) capacityFor(username string) int {
	if session.IsOwner(username) {
		return session.MaxPlayers
	}
	if session.ownerIsPlaying() {
		return session.MaxPlayers
	}
	// Same rule as freeSlot: a seat held for nobody is not held. Without
	// this, a one-slot session with no real owner has capacity zero for
	// everyone, forever.
	if !session.hasOwner() {
		return session.MaxPlayers
	}

	return session.MaxPlayers - 1
}

// AvailableSlots is how many players could still JOIN — what the Dashboard
// shows, and what its join button is gated on. Zero for a one-pad session
// whose owner is away, because that seat is not free, it is theirs.
func (session *Session) AvailableSlots() int {
	free := session.capacityFor("") - len(session.Players)
	if free < 0 {
		return 0
	}

	return free
}

// freeSlot picks the SMALLEST available slot id, and never slot 1 for anybody
// but the owner (invariant 4). Returns 0 when there is none.
//
// Smallest-first, scanning 1..MaxPlayers in order — never ranging over a map.
// Map iteration order deciding a winner has already cost us a real bug, with
// roles oscillating on every push.
//
// Two consequences, both wanted:
//   - the owner is ALWAYS player 1, because the smallest is always theirs to
//     take. Their number is stable across comings and goings without pinning
//     it explicitly: it falls out, it is not a special case;
//   - slots are REUSED. With 1/2/3 taken, when 2 leaves the free ports are 2
//     and 4, and the next arrival gets 2 — never 4. The queue stays dense and
//     a multiplayer game keeps contiguous controller ports, which some cores
//     expect. The newcomer therefore inherits the departed player's NUMBER,
//     hence their character, and on PS1 the memory card of that port. Never
//     reusing a port would exhaust all four after a few comings and goings.
func (session *Session) freeSlot(username string) int {
	first := 1
	// A slot reserved for NOBODY is not reserved. Holding slot 1 for an owner
	// no one can ever be makes a one-slot session unplayable by construction:
	// every arrival is a non-owner, every non-owner starts at 2, and 2 does
	// not exist. Seen live on 2026-08-25 — a PS5 container spawned with
	// GAMEFLEX_SESSION_OWNER unset answered "no slot available" to everyone,
	// its real owner included, for the session's whole life.
	//
	// The reservation exists to protect a person, so it lapses when there is
	// no person to protect.
	if session.hasOwner() && !session.IsOwner(username) {
		first = 2
	}

	for slot := first; slot <= session.MaxPlayers; slot++ {
		taken := false

		for index := range session.Players {
			if session.Players[index].SlotID == slot {
				taken = true
				break
			}
		}
		if !taken {
			return slot
		}
	}

	return 0
}

// Join puts a username on the smallest slot they may take. Refused when they
// already hold one (invariant 1), when the queue is full for them (invariants
// 2 and 3), or when the socket is not an open tab (invariant 6).
func (session *Session) Join(username, socketID string, now time.Time) (int, error) {
	if username == "" {
		return 0, ErrEmptyUsername
	}
	if !session.hasParticipant(socketID) {
		return 0, ErrSocketClosed
	}
	if session.PlayerOf(username) != nil {
		return 0, ErrAlreadyPlaying
	}
	if len(session.Players) >= session.capacityFor(username) {
		return 0, ErrNoSlot
	}

	slot := session.freeSlot(username)
	if slot == 0 {
		return 0, ErrNoSlot
	}

	// A "playable viewer" who takes a place of their own stops borrowing:
	// one person, one pad.
	session.stopBorrowing(username)
	session.appendPlayer(username, socketID, slot, now)

	return slot, nil
}

// Takeover moves an existing entry to ANOTHER socket of the same user — the
// same person, on a different tab or device. The entry is destroyed and
// rebuilt because joinedAt and the socket both change.
//
// It needs no arbitration and no window: the queue holds one socket, the
// takeover replaces it, and the tab that used to hold it starts watching on
// its very next render because IsPlaying stops being true for it.
func (session *Session) Takeover(username, socketID string, now time.Time) (int, error) {
	player := session.PlayerOf(username)
	if player == nil {
		return 0, ErrNotPlaying
	}
	if player.SocketID == socketID {
		return 0, ErrSameSocket
	}
	if !session.hasParticipant(socketID) {
		return 0, ErrSocketClosed
	}

	slot := player.SlotID
	// The loan belongs to the PLACE: the owner moving to another tab is still
	// the owner who lent, and the borrower is still driving.
	lentTo, lentToSocket := player.LentTo, player.LentToSocket
	session.removePlayer(username)
	session.appendPlayer(username, socketID, slot, now)
	if moved := session.PlayerOf(username); moved != nil {
		moved.LentTo, moved.LentToSocket = lentTo, lentToSocket
	}

	return slot, nil
}

// Handover passes `from`'s pad to `to`, in ONE step. Two gestures share it,
// decided by WHO passes (2026-10-09, #274):
//
//   - THE OWNER LENDS. They keep their place — the queue entry, the beat, the
//     rights, the saves — and `to` becomes a "playable viewer" whose keys
//     drive the pad instead of theirs (see lend).
//   - A GUEST GIVES THEIR PLACE. The seat moves to `to`, who becomes a player;
//     the guest watches and keeps nothing.
//
// Atomic either way, which is what deletes the 15s pad reservation of
// chantier 3. That window existed only because the cede FREED the pad and
// made the requester claim it afterwards: in between, the slot belonged to
// nobody, so a third party could take it and the session could be left with
// an empty pad. Nothing to reserve, nothing to expire, nothing to snipe. A
// holder who takes their time to answer creates no window either — they keep
// their slot until this runs.
func (session *Session) Handover(from, to, toSocketID string, now time.Time) (int, error) {
	if to == "" {
		return 0, ErrEmptyUsername
	}

	holder := session.PlayerOf(from)
	if holder == nil {
		return 0, ErrNotPlaying
	}
	if from == to {
		return 0, ErrAlreadyPlaying
	}
	if !session.hasParticipant(toSocketID) {
		return 0, ErrSocketClosed
	}
	if session.IsOwner(from) {
		return session.lend(holder, to, toSocketID, now)
	}

	slot := holder.SlotID
	// The requester may already be playing on another slot — they move, they
	// do not end up with two entries (invariant 1). A borrower who is given a
	// place stops borrowing: one person, one pad.
	session.stopBorrowing(to)
	session.removePlayer(to)
	session.removePlayer(from)
	session.appendPlayer(to, toSocketID, slot, now)

	return slot, nil
}

// lend — the owner's half of Handover.
//
// The borrower must be WATCHING: a "playable viewer" is a spectator whose keys
// drive a pad, never somebody who already holds one — one person, one pad. And
// the pad is lent once at a time: a second loan would be taking it from the
// first borrower behind the owner's back.
func (session *Session) lend(owner *Player, to, toSocketID string, now time.Time) (int, error) {
	if owner.LentTo != "" {
		return 0, ErrPadLent
	}
	if session.PlayerOf(to) != nil {
		return 0, ErrAlreadyPlaying
	}
	if !session.participantIs(toSocketID, to) {
		return 0, ErrNotYourTab
	}

	owner.LentTo = to
	owner.LentToSocket = toSocketID
	// A loan IS activity: somebody new just took the pad. Starting the AFK
	// clock at zero is what stops a borrower from being asked "still there?"
	// seconds after picking it up.
	owner.LastInputAt = now
	owner.AFKWarnedAt = time.Time{}

	return owner.SlotID, nil
}

// Lent — is the owner's pad in somebody else's hands?
func (session *Session) Lent() bool {
	return session.loan() != nil
}

// Borrower — who drives the owner's pad, "" when nobody borrowed it.
func (session *Session) Borrower() string {
	if loan := session.loan(); loan != nil {
		return loan.LentTo
	}

	return ""
}

// loan returns the owner's entry while it is lent, nil otherwise.
func (session *Session) loan() *Player {
	for index := range session.Players {
		if session.Players[index].LentTo != "" {
			return &session.Players[index]
		}
	}

	return nil
}

// stopBorrowing ends the loan held by `username` — called when they take a
// place of their own. One person, one pad.
func (session *Session) stopBorrowing(username string) {
	if loan := session.loan(); loan != nil && loan.LentTo == username {
		loan.LentTo, loan.LentToSocket = "", ""
	}
}

// holderOf returns the entry holding a slot, nil when it is free.
func (session *Session) holderOf(slot int) *Player {
	for index := range session.Players {
		if session.Players[index].SlotID == slot {
			return &session.Players[index]
		}
	}

	return nil
}

// ControlledSlot — which pad THIS tab's keys drive, 0 when none.
//
// The question every input path asks, and the one place a loan changes the
// answer: the borrower's tab drives the owner's pad, and the owner's own keys
// drive nothing until they take it back. A seat and the pad its keys reach
// are the same thing everywhere else.
func (session *Session) ControlledSlot(socketID string) int {
	if socketID == "" {
		return 0
	}

	for _, player := range session.Players {
		if player.LentTo != "" {
			if player.LentToSocket == socketID {
				return player.SlotID
			}
			if player.SocketID == socketID {
				return 0
			}

			continue
		}
		if player.SocketID == socketID {
			return player.SlotID
		}
	}

	return 0
}

// ControllerOf — the tab whose keys drive this pad, "" when none: the inverse
// of ControlledSlot. Force feedback goes there, to the hands on the pad.
func (session *Session) ControllerOf(slot int) string {
	holder := session.holderOf(slot)
	if holder == nil {
		return ""
	}
	if holder.LentTo != "" {
		return holder.LentToSocket
	}

	return holder.SocketID
}

// Reclaim — THE OWNER TAKES THEIR PAD BACK ("take controller back"): when they
// want, without asking. Their keys drive it again; the borrower goes back to
// watching.
//
// From any of the owner's tabs: when it is not the one holding the place, the
// place moves to it — it is the one they are looking at.
func (session *Session) Reclaim(owner, socketID string, now time.Time) (int, error) {
	if !session.IsOwner(owner) {
		return 0, ErrNotOwner
	}
	if !session.participantIs(socketID, owner) {
		return 0, ErrSocketClosed
	}

	loan := session.loan()
	if loan == nil {
		return 0, ErrNotLent
	}

	loan.LentTo, loan.LentToSocket = "", ""
	loan.SocketID = socketID
	// Taking the pad back is a gesture: the AFK clock restarts from it.
	loan.LastBeatAt = now
	loan.LastInputAt = now
	loan.AFKWarnedAt = time.Time{}

	return loan.SlotID, nil
}

// GiveBack — THE BORROWER RETURNS THE PAD ("return controller back"), from the
// tab that borrowed it. Nothing else to settle: the place never left the owner.
func (session *Session) GiveBack(borrower, socketID string, now time.Time) (int, error) {
	loan := session.loan()
	if loan == nil || loan.LentTo != borrower || loan.LentToSocket != socketID {
		return 0, ErrNotBorrower
	}

	loan.LentTo, loan.LentToSocket = "", ""
	loan.LastInputAt = now
	loan.AFKWarnedAt = time.Time{}

	return loan.SlotID, nil
}

// Watchers lists the people present who hold no slot — one entry per person,
// not per tab, and never the anonymous ones (they cannot be handed a pad they
// have no identity to hold).
//
// It exists because a hand-over has TWO directions and only one of them had a
// list. "Who can I ask for the pad" reads the players; "who can I give it to"
// reads this.
func (session *Session) Watchers() []string {
	seen := make(map[string]bool, len(session.Participants))
	watchers := make([]string, 0, len(session.Participants))

	for _, participant := range session.Participants {
		if participant.Username == "" || seen[participant.Username] {
			continue
		}
		if session.PlayerOf(participant.Username) != nil {
			continue
		}
		seen[participant.Username] = true
		watchers = append(watchers, participant.Username)
	}

	return watchers
}

// newestSocketOf — which tab of theirs receives the pad, when someone is
// handed one they did not ask for.
//
// A request carries its own socket: the tab that asked is the tab that plays.
// A GIFT has no such socket, and the recipient may have three tabs open. The
// newest wins, for the same reason it wins everywhere else in this model — it
// is the one they are looking at. Older tabs keep watching, exactly as a
// second tab always has.
func (session *Session) newestSocketOf(username string) string {
	socketID := ""
	for _, participant := range session.Participants {
		if participant.Username == username {
			socketID = participant.SocketID
		}
	}

	return socketID
}

// GiveTo hands this holder's slot to somebody who is watching — the gesture of
// passing a controller across a sofa, which the queue had no verb for.
//
// Only the tab that HOLDS the slot may give it: `socketID` is checked, not
// just the username, so a second tab of the same person cannot give away a pad
// it does not hold. The recipient must already be present — a pad cannot be
// pushed at somebody who is not there to receive it.
func (session *Session) GiveTo(from, socketID, to string, now time.Time) (int, error) {
	if to == "" {
		return 0, ErrEmptyUsername
	}
	if !session.IsPlaying(from, socketID) {
		return 0, ErrNotPlaying
	}
	if from == to {
		return 0, ErrAlreadyPlaying
	}
	if session.PlayerOf(to) != nil {
		return 0, ErrAlreadyPlaying
	}

	target := session.newestSocketOf(to)
	if target == "" {
		return 0, ErrSocketClosed
	}

	return session.Handover(from, to, target, now)
}

// Leave removes a username from the queue whatever socket they were on — a
// deliberate departure.
func (session *Session) Leave(username string, now time.Time) error {
	if session.PlayerOf(username) == nil {
		return ErrNotPlaying
	}
	session.removePlayer(username)
	session.touchIdle(now)

	return nil
}

// DropSocket is what a dead socket triggers: v1's central rule, "socket mort =
// dehors". It removes the queue entry ONLY when the dead socket is the one
// holding it — a player whose OTHER tab just closed keeps playing.
//
// It also drops the participant, so presence needs no separate bookkeeping.
func (session *Session) DropSocket(socketID string, now time.Time) (string, bool) {
	session.removeParticipant(socketID)

	// THE BORROWER LEFT: the pad comes back to its owner by itself — the place
	// never left them, there is nothing else to settle.
	if loan := session.loan(); loan != nil && loan.LentToSocket == socketID {
		loan.LentTo, loan.LentToSocket = "", ""
	}

	for index := range session.Players {
		if session.Players[index].SocketID != socketID {
			continue
		}

		username := session.Players[index].Username
		session.removePlayer(username)
		session.touchIdle(now)

		return username, true
	}

	return "", false
}

// OpenTab registers an open tab. Watching is all it grants: opening a session
// link creates a PARTICIPANT, never a queue entry. That is what makes a
// streaming link inert, and therefore safe to share.
func (session *Session) OpenTab(socketID, username string) {
	for index := range session.Participants {
		if session.Participants[index].SocketID == socketID {
			session.Participants[index].Username = username
			return
		}
	}

	session.Participants = append(session.Participants, Participant{SocketID: socketID, Username: username})
}

// Beat records a liveness tick. hadInput carries the SECOND meaning: the
// message arriving proves the tab's JS runs, hadInput proves a human is
// there. Never bump LastInputAt from the tick alone — a periodic beat proves
// the tab runs, not that anybody is behind it, and conflating the two is what
// made an AFK warning fire at the wrong moment.
func (session *Session) Beat(socketID string, hadInput bool, now time.Time) bool {
	if socketID == "" {
		return false
	}

	for index := range session.Players {
		player := &session.Players[index]

		// PENDANT UN PRÊT, LES DEUX SENS SE SÉPARENT (2026-10-09). Le
		// battement reste celui du propriétaire : c'est son onglet qui prouve
		// qu'il est là, et sa présence qui porte le prêt. Les touches, elles,
		// sont celles de l'emprunteur. Un geste du propriétaire ne fait alors
		// que répondre à « toujours là ? » — s'il est là, la partie n'est pas
		// abandonnée.
		if player.LentTo != "" && player.LentToSocket == socketID {
			if hadInput {
				player.LastInputAt = now
			}

			return true
		}
		if player.SocketID != socketID {
			continue
		}

		player.LastBeatAt = now
		if hadInput && (player.LentTo == "" || !player.AFKWarnedAt.IsZero()) {
			player.LastInputAt = now
		}

		return true
	}

	return false
}

// MarkAFKWarned note qu'on a demandé « toujours là ? » à ce joueur.
func (session *Session) MarkAFKWarned(socketID string, now time.Time) {
	for index := range session.Players {
		if session.Players[index].SocketID == socketID {
			session.Players[index].AFKWarnedAt = now

			return
		}
	}
}

// ClearAFKWarning efface la question : il a répondu.
//
// N'IMPORTE QUEL GESTE SUFFIT — c'est le `hadInput` du battement qui le dit, et
// c'est bien le point : on demande à quelqu'un qui n'a rien touché depuis
// longtemps, et toucher quoi que ce soit est la réponse. Lui faire chercher un
// bouton « oui je suis là » dans un menu serait un piège de plus dans une
// partie.
func (session *Session) ClearAFKWarning(socketID string) {
	for index := range session.Players {
		if session.Players[index].SocketID == socketID {
			session.Players[index].AFKWarnedAt = time.Time{}

			return
		}
	}
}

// --- internals -----------------------------------------------------------

func (session *Session) appendPlayer(username, socketID string, slot int, now time.Time) {
	session.Players = append(session.Players, Player{
		SlotID:   slot,
		Username: username,
		SocketID: socketID,
		JoinedAt: now,
		// A join IS activity: somebody just acted deliberately. Starting the
		// AFK clock at zero here is what stops a tab that had been watching
		// from being kicked seconds after taking a pad.
		LastBeatAt:  now,
		LastInputAt: now,
	})
	session.IdleSince = nil
}

func (session *Session) removePlayer(username string) {
	kept := session.Players[:0]

	for _, player := range session.Players {
		if player.Username != username {
			kept = append(kept, player)
		}
	}

	session.Players = kept
}

func (session *Session) removeParticipant(socketID string) {
	kept := session.Participants[:0]

	for _, participant := range session.Participants {
		if participant.SocketID != socketID {
			kept = append(kept, participant)
		}
	}

	session.Participants = kept
}

// touchIdle is invariant 5: the idle clock starts when the queue empties and
// stops the moment anybody plays.
func (session *Session) touchIdle(now time.Time) {
	if len(session.Players) == 0 {
		if session.IdleSince == nil {
			session.IdleSince = &now
		}

		return
	}

	session.IdleSince = nil
}
