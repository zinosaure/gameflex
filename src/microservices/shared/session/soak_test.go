package session

// Long ping-pong soaks over the real queue model (chantier-4). The bugs that
// motivated them (2026-08-24/25) only showed up "after a few back-and-forths":
// a holder that never re-arms, a pad drifting away, a token lost after several
// exchanges. Single-shot tests cannot see that — only long loops can.
//
// Each loop also re-attaches a FRESH socket per round, exactly what a browser
// tab does: the previous tab closes and the newest one is the one that acts.
// The websocket IS the identity, so replacing it is the whole story.

import (
	"fmt"
	"testing"
	"time"
)

const soakRounds = 100

// helperAssert keeps the loops readable: fail with the round number.
func failAt(t *testing.T, round int, format string, args ...any) {
	t.Helper()

	t.Fatalf("round %d: %s", round, fmt.Sprintf(format, args...))
}

// TestLoanPingPong — the owner lends their pad and gets it back, over and
// over. Every round the borrower acts from a BRAND-NEW socket (a reopened tab)
// and the loan ends one of its three ways in turn: the owner takes it back,
// the borrower returns it, the borrower's tab closes. Throughout, the owner
// keeps their place — ONE entry, theirs, on slot 1 — and only the hands on the
// pad change (2026-10-09).
func TestLoanPingPong(t *testing.T) {
	host, rec := newHost(t, 4)
	const owner, ownerSocket, borrower = "gino", "gino-1", "dave"

	host.Attach(ownerSocket, owner)
	host.Submit(Command{Kind: CmdJoin, Username: owner, SocketID: ownerSocket}, now)

	for round := 1; round <= soakRounds; round++ {
		// The borrower reopens its tab: a new participant, a new socket.
		borrowerSocket := fmt.Sprintf("%s-%d", borrower, round)
		host.Attach(borrowerSocket, borrower)

		host.Submit(Command{Kind: CmdRequestHandover, Username: borrower, SocketID: borrowerSocket, Target: owner}, now)
		host.Submit(Command{Kind: CmdGrantHandover, Username: owner, SocketID: ownerSocket}, now)

		if result := rec.last(borrowerSocket, "result"); result == nil || result["status"] != "granted" {
			failAt(t, round, "grant result = %+v, want granted", rec.last(borrowerSocket, "result"))
		}

		// The borrower's NEW socket drives the pad, and only it.
		if host.Session().ControlledSlot(borrowerSocket) != 1 || host.Session().ControlledSlot(ownerSocket) != 0 {
			failAt(t, round, "pad 1 driven from %q, want %s", host.Session().ControllerOf(1), borrowerSocket)
		}
		// One entry, the owner's, on slot 1.
		players := host.Session().Players
		if len(players) != 1 || players[0].Username != owner || players[0].SlotID != 1 || players[0].SocketID != ownerSocket {
			failAt(t, round, "players = %+v, want only %s on slot 1", players, owner)
		}
		if host.Session().Owner != owner || host.Session().PendingHandover != nil {
			failAt(t, round, "owner = %q, pending = %+v", host.Session().Owner, host.Session().PendingHandover)
		}

		switch round % 3 {
		case 0:
			host.Submit(Command{Kind: CmdReclaim, Username: owner, SocketID: ownerSocket}, now)
		case 1:
			host.Submit(Command{Kind: CmdReturn, Username: borrower, SocketID: borrowerSocket}, now)
		case 2:
			host.Detach(borrowerSocket, now.Add(time.Duration(round)*time.Second))
		}

		if host.Session().Lent() || host.Session().ControlledSlot(ownerSocket) != 1 {
			failAt(t, round, "the pad did not come back to its owner (end %d)", round%3)
		}
		if round%3 != 2 {
			host.Detach(borrowerSocket, now.Add(time.Duration(round)*time.Second))
		}
	}

	if host.Session().IdleSince != nil {
		t.Fatal("a session with a player must not be idle")
	}
}

// TestTakeoverPingPong — one player claims their pad from a NEW tab over and
// over. Each round: the new socket takes over, the queue entry is rebuilt on
// it, and the previous socket reads "watching" on its very next state. No
// grace window, no arbitration — the socket IS the identity.
func TestTakeoverPingPong(t *testing.T) {
	host, rec := newHost(t, 4)

	host.Attach("gino-1", "gino")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "gino-1"}, now)

	holdingSocket := "gino-1"

	for round := 1; round <= soakRounds; round++ {
		nextSocket := fmt.Sprintf("gino-takeover-%d", round+1)
		host.Attach(nextSocket, "gino")

		host.Submit(Command{Kind: CmdTakeover, Username: "gino", SocketID: nextSocket}, now)
		if result := rec.last(nextSocket, "result"); result == nil || result["status"] != "granted" {
			failAt(t, round, "takeover result = %+v, want granted", rec.last(nextSocket, "result"))
		}

		if !host.Session().IsPlaying("gino", nextSocket) {
			failAt(t, round, "the new tab does not hold the pad")
		}
		if host.Session().IsPlaying("gino", holdingSocket) {
			failAt(t, round, "the previous tab still reads playing")
		}

		// Invariant 1: never two entries, never more than one.
		if len(host.Session().Players) != 1 {
			failAt(t, round, "players = %+v, want exactly 1", host.Session().Players)
		}
		if p := host.Session().PlayerOf("gino"); p == nil || p.SocketID != nextSocket {
			failAt(t, round, "entry on socket %+v, want %s", p, nextSocket)
		}

		// The retired tab closes. Its socket is NOT the entry's, so the
		// slot survives — closing a stale tab must never cost the pad.
		host.Detach(holdingSocket, now.Add(time.Duration(round)*time.Second))
		if !host.Session().IsPlaying("gino", nextSocket) {
			failAt(t, round, "closing the old tab released the pad it no longer held")
		}

		holdingSocket = nextSocket
	}
}

// TestHandoverPingPongKeepsSlotsDense — the density rule lives at JOIN, not
// at hand-over: a guest giving their place MOVES a seat and can leave a hole
// behind, and the next joiner must always take the LOWEST free slot. With
// 1/2/3/4 full, P2 leaves and zoe takes 2; then carol (slot 3) gives her place
// to zoe, who moves onto 3 and frees 2 again: the newcomer still gets 2, never
// a higher one.
func TestHandoverPingPongKeepsSlotsDense(t *testing.T) {
	host, _ := newHost(t, 4)

	for _, username := range []string{"gino", "dave", "carol", "eve"} {
		socketID := "sock-" + username
		host.Attach(socketID, username)
		host.Submit(Command{Kind: CmdJoin, Username: username, SocketID: socketID}, now)
	}
	if len(host.Session().Players) != 4 {
		t.Fatalf("setup: players = %+v, want 4", host.Session().Players)
	}

	// dave (slot 2) leaves — the next joiner takes 2.
	host.Submit(Command{Kind: CmdLeave, Username: "dave", SocketID: "sock-dave"}, now)
	host.Attach("sock-zoe", "zoe")
	host.Submit(Command{Kind: CmdJoin, Username: "zoe", SocketID: "sock-zoe"}, now)

	if p := host.Session().PlayerOf("zoe"); p == nil || p.SlotID != 2 {
		t.Fatalf("zoe got slot %+v, want 2 — slots must be reused in order", p)
	}

	// Ping-pong slot 3 between carol and zoe. zoe already plays on slot 2, so
	// the first gift MOVES her onto slot 3 and frees slot 2 again.
	for round := 1; round <= soakRounds/2; round++ {
		host.Submit(Command{Kind: CmdRequestHandover, Username: "zoe", SocketID: "sock-zoe", Target: "carol"}, now)
		host.Submit(Command{Kind: CmdGrantHandover, Username: "carol", SocketID: "sock-carol"}, now)

		if p := host.Session().PlayerOf("zoe"); p == nil || p.SlotID != 3 {
			failAt(t, round, "zoe slot = %+v, want 3", p)
		}

		host.Submit(Command{Kind: CmdRequestHandover, Username: "carol", SocketID: "sock-carol", Target: "zoe"}, now)
		host.Submit(Command{Kind: CmdGrantHandover, Username: "zoe", SocketID: "sock-zoe"}, now)

		if p := host.Session().PlayerOf("carol"); p == nil || p.SlotID != 3 {
			failAt(t, round, "carol slot = %+v, want 3", p)
		}
	}

	// The queue has 3 players (slots 1, 3, 4) and slot 2 is free again: the
	// next joiner gets 2 — the lowest free slot, never 4.
	host.Attach("sock-nina", "nina")
	host.Submit(Command{Kind: CmdJoin, Username: "nina", SocketID: "sock-nina"}, now)

	if p := host.Session().PlayerOf("nina"); p == nil || p.SlotID != 2 {
		t.Fatalf("nina got slot %+v, want 2 — a join must take the lowest free slot", p)
	}
}

// TestCapacityRefusesOverflowAndNobodyLosesTheirSeat — the capacity
// invariants under real load, for every meaningful console size. With 1, 2
// or 4 players already in the queue, every extra join MUST be refused, the
// queue MUST never exceed maxPlayers, and — the part that bit us before —
// a refused newcomer must not displace anybody: the players who were there
// keep their exact slot AND socket.
func TestCapacityRefusesOverflowAndNobodyLosesTheirSeat(t *testing.T) {
	for _, maxPlayers := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("maxPlayers-%d", maxPlayers), func(t *testing.T) {
			host, rec := newHost(t, maxPlayers)

			// Fill the queue to capacity: the owner first, then joiners.
			host.Attach("sock-gino", "gino")
			host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-gino"}, now)

			fillers := []string{"dave", "carol", "eve"}
			for index := 0; index < maxPlayers-1; index++ {
				username := fillers[index]
				socketID := "sock-" + username
				host.Attach(socketID, username)
				host.Submit(Command{Kind: CmdJoin, Username: username, SocketID: socketID}, now)
			}

			if len(host.Session().Players) != maxPlayers {
				t.Fatalf("setup: players = %+v, want %d", host.Session().Players, maxPlayers)
			}

			before := seatsSnapshot(host.Session().Players)

			// Three extra users try to join. Every single one is refused,
			// and the seats are bit-for-bit identical afterwards.
			for _, extra := range []string{"zoe", "nina", "paul"} {
				socketID := "sock-" + extra
				host.Attach(socketID, extra)
				host.Submit(Command{Kind: CmdJoin, Username: extra, SocketID: socketID}, now)

				if result := rec.last(socketID, "result"); result == nil || result["status"] != "refused" {
					t.Fatalf("%s join = %+v, want refused", extra, rec.last(socketID, "result"))
				}
				if len(host.Session().Players) != maxPlayers {
					t.Fatalf("after %s: players = %+v, want still %d", extra, host.Session().Players, maxPlayers)
				}
				if got := seatsSnapshot(host.Session().Players); !seatsEqual(before, got) {
					t.Fatalf("after %s refused: seats moved from %+v to %+v", extra, before, got)
				}
				if state := rec.last(socketID, "state"); state != nil && state["availableSlots"] != 0 {
					t.Fatalf("%s reads availableSlots=%v, want 0 on a full session", extra, state["availableSlots"])
				}
			}
		})
	}
}

// TestCapacitySurvivesTakeoverAndHandover — at full capacity, a takeover
// moves a seat and the owner's loan moves none: neither ever adds one, and the
// queue stays at maxPlayers through both.
func TestCapacitySurvivesTakeoverAndHandover(t *testing.T) {
	host, rec := newHost(t, 2)

	host.Attach("sock-gino", "gino")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-gino"}, now)
	host.Attach("sock-dave", "dave")
	host.Submit(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-dave"}, now)

	// A third user at capacity: refused, and the two seats are intact.
	host.Attach("sock-zoe", "zoe")
	host.Submit(Command{Kind: CmdJoin, Username: "zoe", SocketID: "sock-zoe"}, now)
	if result := rec.last("sock-zoe", "result"); result == nil || result["status"] != "refused" {
		t.Fatalf("zoe join = %+v, want refused", rec.last("sock-zoe", "result"))
	}

	// Takeover at capacity: same user, new socket, same single seat.
	host.Attach("sock-gino-2", "gino")
	host.Submit(Command{Kind: CmdTakeover, Username: "gino", SocketID: "sock-gino-2"}, now)
	if result := rec.last("sock-gino-2", "result"); result == nil || result["status"] != "granted" {
		t.Fatalf("takeover = %+v, want granted", rec.last("sock-gino-2", "result"))
	}
	if len(host.Session().Players) != 2 {
		t.Fatalf("after takeover: players = %+v, want still 2", host.Session().Players)
	}
	if !host.Session().IsPlaying("gino", "sock-gino-2") || host.Session().IsPlaying("gino", "sock-gino") {
		t.Fatal("the takeover moved the seat onto the new socket only")
	}

	// The owner lends at capacity to a WATCHER (zoe holds nothing): the place
	// stays the owner's, zoe drives the pad, and the count does not grow.
	host.Submit(Command{Kind: CmdRequestHandover, Username: "zoe", SocketID: "sock-zoe", Target: "gino"}, now)
	host.Submit(Command{Kind: CmdGrantHandover, Username: "gino", SocketID: "sock-gino-2"}, now)

	if result := rec.last("sock-gino-2", "result"); result == nil || result["status"] != "granted" {
		t.Fatalf("grant = %+v, want granted", rec.last("sock-gino-2", "result"))
	}
	if len(host.Session().Players) != 2 {
		t.Fatalf("after the loan: players = %+v, want still 2", host.Session().Players)
	}
	if !host.Session().IsPlaying("gino", "sock-gino-2") || host.Session().ControlledSlot("sock-zoe") != 1 {
		t.Fatal("the loan kept the owner's place and put the pad in the watcher's hands")
	}
}

// seatsSnapshot reduces the queue to what must never move: username, slot
// and socket, in order.
func seatsSnapshot(players []Player) [][3]string {
	snapshot := make([][3]string, 0, len(players))

	for _, player := range players {
		snapshot = append(snapshot, [3]string{player.Username, fmt.Sprint(player.SlotID), player.SocketID})
	}

	return snapshot
}

func seatsEqual(left, right [][3]string) bool {
	if len(left) != len(right) {
		return false
	}

	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}

	return true
}
