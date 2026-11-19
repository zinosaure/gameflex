package session

import "testing"

// MA PLACE, DEPUIS UN AUTRE APPAREIL (chantier 65, #793) — le même compte, sur
// le téléphone pendant que le PC tient la place, demande au PC de la libérer.
// L'onglet qui la tient répond ; accordée, la place passe au téléphone.

func TestMoveGrantedByTheHoldingTab(t *testing.T) {
	session := ready(t, 4, "sock-D2")
	slot := mustJoin(t, session, "dave", "sock-D")

	outcomes := drain(session, now, Command{Kind: CmdRequestHandover, Username: "dave", SocketID: "sock-D2", Target: "dave"})
	if outcomes[0].Err != nil {
		t.Fatalf("request from my other device = %v, want accepted", outcomes[0].Err)
	}

	// L'appareil qui demande ne s'accorde pas lui-même la place : le nom est le
	// même des deux côtés, seul l'onglet qui la tient répond.
	outcomes = drain(session, now, Command{Kind: CmdGrantHandover, Username: "dave", SocketID: "sock-D2"})
	if outcomes[0].Err != ErrNotHolder {
		t.Fatalf("the asking device granting itself = %v, want ErrNotHolder", outcomes[0].Err)
	}
	if session.LiveHandoverAt(now) == nil {
		t.Fatal("the request must still be pending")
	}

	outcomes = drain(session, now, Command{Kind: CmdGrantHandover, Username: "dave", SocketID: "sock-D"})
	if outcomes[0].Err != nil || outcomes[0].Slot != slot {
		t.Fatalf("grant = (%d, %v), want (%d, nil)", outcomes[0].Slot, outcomes[0].Err, slot)
	}
	if !session.IsPlaying("dave", "sock-D2") || session.IsPlaying("dave", "sock-D") {
		t.Fatalf("players = %+v — want the place on the phone (sock-D2), the PC watching", session.Players)
	}
	if session.ControlledSlot("sock-D2") != slot {
		t.Fatal("the phone must drive the pad at once")
	}
}

func TestMoveDeniedTellsWhoAsked(t *testing.T) {
	session := ready(t, 4, "sock-D2")
	mustJoin(t, session, "dave", "sock-D")
	drain(session, now, Command{Kind: CmdRequestHandover, Username: "dave", SocketID: "sock-D2", Target: "dave"})

	outcomes := drain(session, now, Command{Kind: CmdDenyHandover, Username: "dave", SocketID: "sock-D"})
	if outcomes[0].Err != nil {
		t.Fatalf("deny = %v", outcomes[0].Err)
	}
	denied := outcomes[0].Denied
	if denied == nil || denied.FromSocket != "sock-D2" || denied.From != "dave" || denied.To != "dave" {
		t.Fatalf("denied = %+v, want the phone's demand", denied)
	}
	if !session.IsPlaying("dave", "sock-D") {
		t.Fatal("refused: the PC keeps the place")
	}
}

func TestMoveFromTheHoldingTabIsRefused(t *testing.T) {
	session := ready(t, 4)
	mustJoin(t, session, "dave", "sock-D")

	outcomes := drain(session, now, Command{Kind: CmdRequestHandover, Username: "dave", SocketID: "sock-D", Target: "dave"})
	if outcomes[0].Err != ErrSameSocket {
		t.Fatalf("asking my own tab for my own place = %v, want ErrSameSocket", outcomes[0].Err)
	}
}

func TestMoveNeedsAPlace(t *testing.T) {
	session := ready(t, 4, "sock-D", "sock-D2")

	outcomes := drain(session, now, Command{Kind: CmdRequestHandover, Username: "dave", SocketID: "sock-D2", Target: "dave"})
	if outcomes[0].Err != ErrNoSuchHolder {
		t.Fatalf("a move with no place anywhere = %v, want ErrNoSuchHolder", outcomes[0].Err)
	}
}

// Le refus atteint l'onglet qui a demandé, par un `result` — avant, la demande
// disparaissait seulement de l'état, et la page ne pouvait pas dire « non ».
func TestHostTellsTheAskerOfARefusal(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-X", "gixi")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)
	host.Submit(Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "gino"}, now)
	host.Submit(Command{Kind: CmdDenyHandover, Username: "gino", SocketID: "sock-G"}, now)

	result := rec.last("sock-X", "result")
	if result == nil || result["status"] != "denied" || result["action"] != string(CmdRequestHandover) || result["move"] != false {
		t.Fatalf("asker's result = %+v, want a denied request-handover, not a move", result)
	}
}
