package session

import (
	"errors"
	"testing"
	"time"
)

// PLUSIEURS MANETTES SUR UN MÊME APPAREIL (chantier 84).

func TestTheActivatedPadsTakeTheFirstSeatsInOrder(t *testing.T) {
	session := newSession(4)
	session.OpenTab("sock-G", "gino")
	for pad := 1; pad <= 3; pad++ {
		slot, err := session.JoinPad("gino", "sock-G", pad, now)
		if err != nil || slot != pad {
			t.Fatalf("pad %d = slot %d, %v ; want slot %d", pad, slot, err, pad)
		}
	}
	for pad := 1; pad <= 3; pad++ {
		if got := session.ControlledSlot("sock-G", pad); got != pad {
			t.Fatalf("pad %d drives slot %d, want %d", pad, got, pad)
		}
	}
	if got := session.ControlledSlot("sock-G", 4); got != 0 {
		t.Fatalf("a pad without a seat drives slot %d, want none", got)
	}
}

func TestAPadWithoutASeatWaitsAndTakesTheNextOne(t *testing.T) {
	session := newSession(3)
	mustJoin(t, session, "gino", "sock-G") // 1
	mustJoin(t, session, "dave", "sock-D") // 2
	session.OpenTab("sock-M", "marc")

	if slot, err := session.JoinPad("marc", "sock-M", 1, now); err != nil || slot != 3 {
		t.Fatalf("marc pad 1 = %d, %v ; want slot 3", slot, err)
	}
	if _, err := session.JoinPad("marc", "sock-M", 2, now); !errors.Is(err, ErrWaiting) {
		t.Fatalf("marc pad 2 = %v, want ErrWaiting", err)
	}
	if pads := session.WaitingPads("sock-M"); len(pads) != 1 || pads[0] != 2 {
		t.Fatalf("waiting = %v, want [2]", pads)
	}

	// dave leaves: slot 2 goes to marc's waiting pad, by itself.
	if err := session.Leave("dave", now); err != nil {
		t.Fatal(err)
	}
	if got := session.ControlledSlot("sock-M", 2); got != 2 {
		t.Fatalf("marc pad 2 drives %d, want 2", got)
	}
	if len(session.Waiting) != 0 {
		t.Fatalf("waiting = %+v, want empty", session.Waiting)
	}
}

// A device holding no seat at all does not queue: it watches, as before.
func TestADeviceWithoutASeatDoesNotWait(t *testing.T) {
	session := newSession(2)
	mustJoin(t, session, "gino", "sock-G")
	mustJoin(t, session, "dave", "sock-D")
	session.OpenTab("sock-M", "marc")

	if _, err := session.JoinPad("marc", "sock-M", 1, now); !errors.Is(err, ErrNoSlot) {
		t.Fatalf("join = %v, want ErrNoSlot", err)
	}
	if len(session.Waiting) != 0 {
		t.Fatalf("waiting = %+v, want empty", session.Waiting)
	}
}

func TestGivingUpPadOneKeepsPadTwoPlaying(t *testing.T) {
	session := newSession(2)
	session.OpenTab("sock-G", "gino")
	mustJoinPad(t, session, "gino", "sock-G", 1) // 1
	mustJoinPad(t, session, "gino", "sock-G", 2) // 2
	session.OpenTab("sock-D", "dave")

	if err := session.LeavePad("gino", "sock-G", 1, now); err != nil {
		t.Fatal(err)
	}
	if session.ControlledSlot("sock-G", 1) != 0 || session.ControlledSlot("sock-G", 2) != 2 {
		t.Fatalf("players = %+v, want pad 2 alone on slot 2", session.Players)
	}
	// The owner still owns, and their freed slot 1 stays theirs to take.
	if _, err := session.JoinPad("dave", "sock-D", 1, now); !errors.Is(err, ErrNoSlot) {
		t.Fatalf("dave = %v, want ErrNoSlot — slot 1 is the owner's", err)
	}
}

func TestTheFreedSeatGoesToTheWaitingPadOfTheSameDevice(t *testing.T) {
	session := newSession(2)
	session.OpenTab("sock-G", "gino")
	mustJoinPad(t, session, "gino", "sock-G", 1)
	mustJoinPad(t, session, "gino", "sock-G", 2)
	if _, err := session.JoinPad("gino", "sock-G", 3, now); !errors.Is(err, ErrWaiting) {
		t.Fatalf("pad 3 = %v, want ErrWaiting", err)
	}

	if err := session.LeavePad("gino", "sock-G", 1, now); err != nil {
		t.Fatal(err)
	}
	if got := session.ControlledSlot("sock-G", 3); got != 1 {
		t.Fatalf("pad 3 drives %d, want slot 1", got)
	}
}

func TestADeviceLeavingTakesAllItsPads(t *testing.T) {
	session := newSession(3)
	session.OpenTab("sock-G", "gino")
	mustJoinPad(t, session, "gino", "sock-G", 1)
	session.OpenTab("sock-M", "marc")
	mustJoinPad(t, session, "marc", "sock-M", 1)
	mustJoinPad(t, session, "marc", "sock-M", 2)
	if _, err := session.JoinPad("marc", "sock-M", 3, now); !errors.Is(err, ErrWaiting) {
		t.Fatalf("pad 3 = %v, want ErrWaiting", err)
	}

	session.DropSocket("sock-M", now)
	if len(session.Players) != 1 || len(session.Waiting) != 0 {
		t.Fatalf("players %+v, waiting %+v ; want gino alone", session.Players, session.Waiting)
	}
}

func TestASecondTabOfTheSameAccountStillWatches(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-G")
	session.OpenTab("sock-G2", "gino")

	if _, err := session.JoinPad("gino", "sock-G2", 2, now); !errors.Is(err, ErrAlreadyPlaying) {
		t.Fatalf("second tab = %v, want ErrAlreadyPlaying", err)
	}
}

// LA DEMANDE DE MANETTE VA À LA MANETTE EN ATTENTE (le porteur : « la demande sera liée à la manette libre »), et
// le demandeur garde son premier siège.
func TestAHandoverRequestSeatsTheWaitingPad(t *testing.T) {
	host, _ := newHost(t, 3)
	for _, entry := range []struct{ user, socket string }{{"gino", "sock-G"}, {"dave", "sock-D"}, {"marc", "sock-M"}} {
		host.Attach(entry.socket, entry.user)
		host.Submit(Command{Kind: CmdJoin, Username: entry.user, SocketID: entry.socket, Pad: 1}, now)
	}
	host.Submit(Command{Kind: CmdJoin, Username: "marc", SocketID: "sock-M", Pad: 2}, now)
	if pads := host.Session().WaitingPads("sock-M"); len(pads) != 1 || pads[0] != 2 {
		t.Fatalf("waiting = %v, want [2]", pads)
	}

	host.Submit(Command{Kind: CmdRequestHandover, Username: "marc", SocketID: "sock-M", Target: "dave"}, now)
	host.Submit(Command{Kind: CmdGrantHandover, Username: "dave", SocketID: "sock-D"}, now.Add(time.Second))

	session := host.Session()
	if session.ControlledSlot("sock-M", 1) != 3 || session.ControlledSlot("sock-M", 2) != 2 {
		t.Fatalf("players = %+v, want marc pad 1 on 3 and pad 2 on 2", session.Players)
	}
	if session.PlayerOf("dave") != nil || len(session.Waiting) != 0 {
		t.Fatalf("players %+v, waiting %+v ; want dave watching", session.Players, session.Waiting)
	}
}

// The seat asked for is the one named: a holder may hold several.
func TestAHandoverRequestNamesTheSeat(t *testing.T) {
	host, _ := newHost(t, 3)
	host.Attach("sock-G", "gino")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G", Pad: 1}, now)
	host.Attach("sock-D", "dave")
	host.Submit(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D", Pad: 1}, now) // 2
	host.Submit(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D", Pad: 2}, now) // 3
	host.Attach("sock-M", "marc")

	host.Submit(Command{Kind: CmdRequestHandover, Username: "marc", SocketID: "sock-M", Target: "dave", Slot: 3}, now)
	host.Submit(Command{Kind: CmdGrantHandover, Username: "dave", SocketID: "sock-D"}, now.Add(time.Second))

	session := host.Session()
	if session.ControlledSlot("sock-M", 1) != 3 || session.ControlledSlot("sock-D", 1) != 2 {
		t.Fatalf("players = %+v, want marc on 3 and dave pad 1 still on 2", session.Players)
	}
}

func TestSplitPadReadsThePrefix(t *testing.T) {
	cases := []struct {
		in   string
		pad  int
		rest string
	}{
		{"kd,KeyA", 1, "kd,KeyA"},
		{"#2|kd,KeyA", 2, "kd,KeyA"},
		{"#12|st,l,0,0", 12, "st,l,0,0"},
		{"#x|kd", 1, "#x|kd"},
		{"#0|kd", 1, "#0|kd"},
		{`{"b":1}`, 1, `{"b":1}`},
	}
	for _, c := range cases {
		pad, rest := SplitPad([]byte(c.in))
		if pad != c.pad || string(rest) != c.rest {
			t.Fatalf("SplitPad(%q) = %d, %q ; want %d, %q", c.in, pad, rest, c.pad, c.rest)
		}
	}
}

func mustJoinPad(t *testing.T, session *Session, username, socketID string, pad int) {
	t.Helper()
	if _, err := session.JoinPad(username, socketID, pad, now); err != nil {
		t.Fatalf("JoinPad(%s, %d) = %v", username, pad, err)
	}
}
