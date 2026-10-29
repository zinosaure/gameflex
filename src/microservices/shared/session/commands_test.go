package session

import (
	"testing"
	"time"
)

// tabOwners — à qui appartient chaque onglet de ces tests. Un onglet porte
// toujours le nom VÉRIFIÉ de son socket (HTTPHost.serveSocket) : l'ouvrir
// anonyme décrivait une session qui n'existe pas, et la règle ErrNotYourTab
// refuse désormais qu'une commande parle pour l'onglet d'un autre.
var tabOwners = map[string]string{
	"sock-A": "alex", "sock-C": "carol", "sock-D": "dave", "sock-D2": "dave",
	"sock-G": "gino", "sock-X": "gixi",
}

// ready builds a session with `owner` playing and every listed socket open as
// a participant — the state a real child is in when commands start arriving.
func ready(t *testing.T, maxPlayers int, sockets ...string) *Session {
	t.Helper()

	session := newSession(maxPlayers)
	for _, socket := range sockets {
		session.OpenTab(socket, tabOwners[socket])
	}

	return session
}

func drain(session *Session, now time.Time, commands ...Command) []Outcome {
	queue := &Queue{}
	for _, command := range commands {
		queue.Push(command)
	}

	return queue.Drain(session, now)
}

// --- ordre et sérialisation ----------------------------------------------

// Les commandes s'appliquent dans l'ordre reçu — premier arrivé, premier
// servi, ce qui est exactement la règle de la queue.
func TestCommandsApplyInOrder(t *testing.T) {
	session := ready(t, 4, "sock-D", "sock-A", "sock-C")

	outcomes := drain(session, now,
		Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D"},
		Command{Kind: CmdJoin, Username: "alex", SocketID: "sock-A"},
		Command{Kind: CmdJoin, Username: "carol", SocketID: "sock-C"},
	)

	wantSlots := []int{2, 3, 4}
	for index, outcome := range outcomes {
		if outcome.Err != nil {
			t.Fatalf("command %d: %v", index, outcome.Err)
		}
		if outcome.Slot != wantSlots[index] {
			t.Fatalf("command %d took slot %d, want %d", index, outcome.Slot, wantSlots[index])
		}
	}
}

// --- valider à l'exécution, jamais à l'empilement -------------------------

// Un onglet peut mourir pendant que sa commande attend son tour. Elle est
// alors ABANDONNÉE : elle était légitime quand elle a été faite, rien n'a
// échoué.
func TestCommandFromAClosedTabIsAbandoned(t *testing.T) {
	session := ready(t, 4, "sock-D")

	queue := &Queue{}
	queue.Push(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D"})
	// L'onglet se ferme AVANT le drain.
	session.DropSocket("sock-D", now)

	outcomes := queue.Drain(session, now)
	if !outcomes[0].Abandoned {
		t.Fatalf("outcome = %+v, want abandoned", outcomes[0])
	}
	if len(session.Players) != 0 {
		t.Fatalf("a dead tab was given a slot: %+v", session.Players)
	}
}

// --- handover -------------------------------------------------------------

func TestHandoverRequestThenGrant(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-X")
	mustJoin(t, session, "gino", "sock-G")

	outcomes := drain(session, now,
		Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "gino"},
	)
	if outcomes[0].Err != nil {
		t.Fatalf("request = %v", outcomes[0].Err)
	}
	if session.LiveHandoverAt(now) == nil {
		t.Fatal("the request must be pending")
	}

	outcomes = drain(session, now,
		Command{Kind: CmdGrantHandover, Username: "gino", SocketID: "sock-G"},
	)
	if outcomes[0].Err != nil || outcomes[0].Slot != 1 {
		t.Fatalf("grant = (%d, %v), want (1, nil)", outcomes[0].Slot, outcomes[0].Err)
	}
	if session.ControlledSlot("sock-X") != 1 {
		t.Fatal("the requester must drive the pad immediately — nothing to claim")
	}
	// The pad is LENT: the owner stays the owner, and keeps their place
	// (2026-10-09).
	if session.Owner != "gino" || session.Borrower() != "gixi" || !session.IsPlaying("gino", "sock-G") {
		t.Fatalf("owner = %q, borrower = %q, players = %+v — want gino seated, lending to gixi",
			session.Owner, session.Borrower(), session.Players)
	}
}

// Seul le détenteur interrogé peut répondre. N'importe qui d'autre est un
// non-événement, pas un accord silencieux.
func TestOnlyTheHolderMayAnswer(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-X", "sock-D")
	mustJoin(t, session, "gino", "sock-G")
	mustJoin(t, session, "dave", "sock-D")

	drain(session, now, Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "gino"})

	outcomes := drain(session, now, Command{Kind: CmdGrantHandover, Username: "dave", SocketID: "sock-D"})
	if outcomes[0].Err != ErrNotHolder {
		t.Fatalf("a bystander granting = %v, want ErrNotHolder", outcomes[0].Err)
	}
	if session.LiveHandoverAt(now) == nil {
		t.Fatal("the request must still be pending")
	}
}

// « Z leaves » avant « Y granted » : la précondition a changé, la commande est
// abandonnée — jamais rejouée.
func TestGrantIsAbandonedWhenTheRequesterLeft(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-X")
	mustJoin(t, session, "gino", "sock-G")

	drain(session, now, Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "gino"})
	// Le demandeur ferme son onglet pendant que le détenteur réfléchit.
	session.DropSocket("sock-X", now)

	outcomes := drain(session, now, Command{Kind: CmdGrantHandover, Username: "gino", SocketID: "sock-G"})
	if !outcomes[0].Abandoned {
		t.Fatalf("outcome = %+v, want abandoned", outcomes[0])
	}
	if !session.IsPlaying("gino", "sock-G") {
		t.Fatal("the holder must keep their slot when the grant is abandoned")
	}
}

// Une demande sans réponse expire, sinon un détenteur qui ne clique jamais
// gèle le handover de la session pour de bon.
func TestHandoverRequestExpires(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-X", "sock-D")
	mustJoin(t, session, "gino", "sock-G")

	drain(session, now, Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "gino"})

	later := now.Add(HandoverTTL + time.Second)
	if session.LiveHandoverAt(later) != nil {
		t.Fatal("the request must have timed out")
	}
	// …et une nouvelle demande passe.
	outcomes := drain(session, later, Command{Kind: CmdRequestHandover, Username: "dave", SocketID: "sock-D", Target: "gino"})
	if outcomes[0].Err != nil {
		t.Fatalf("a fresh request after the timeout = %v, want accepted", outcomes[0].Err)
	}
}

func TestOnlyOnePendingHandover(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-X", "sock-D")
	mustJoin(t, session, "gino", "sock-G")

	drain(session, now, Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "gino"})
	outcomes := drain(session, now, Command{Kind: CmdRequestHandover, Username: "dave", SocketID: "sock-D", Target: "gino"})

	if outcomes[0].Err != ErrHandoverPending {
		t.Fatalf("second request = %v, want ErrHandoverPending", outcomes[0].Err)
	}
}

func TestHandoverNeedsARealHolder(t *testing.T) {
	session := ready(t, 4, "sock-X")

	outcomes := drain(session, now, Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "nobody"})
	if outcomes[0].Err != ErrNoSuchHolder {
		t.Fatalf("request against a non-player = %v, want ErrNoSuchHolder", outcomes[0].Err)
	}
}

// --- takeover -------------------------------------------------------------

// Ouvert à TOUT joueur, pas au seul owner : un joiner dont l'entrée est sur un
// autre onglet est sinon piégé — il ne peut ni reprendre, ni rejoindre
// (invariant 1).
func TestTakeoverIsOpenToAnyPlayer(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-D", "sock-D2")
	mustJoin(t, session, "gino", "sock-G")
	mustJoin(t, session, "dave", "sock-D")

	outcomes := drain(session, now, Command{Kind: CmdTakeover, Username: "dave", SocketID: "sock-D2"})
	if outcomes[0].Err != nil {
		t.Fatalf("a joiner taking their own slot back = %v, want success", outcomes[0].Err)
	}
	if !session.IsPlaying("dave", "sock-D2") || session.IsPlaying("dave", "sock-D") {
		t.Fatal("the slot must have moved to the asking tab")
	}
}

// --- terminate ------------------------------------------------------------

func TestOnlyTheOwnerTerminates(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-D")
	mustJoin(t, session, "gino", "sock-G")
	mustJoin(t, session, "dave", "sock-D")

	outcomes := drain(session, now, Command{Kind: CmdTerminate, Username: "dave", SocketID: "sock-D"})
	if outcomes[0].Err != ErrNotOwner {
		t.Fatalf("a joiner terminating = %v, want ErrNotOwner", outcomes[0].Err)
	}
	if session.Terminated {
		t.Fatal("a joiner ended somebody else's session")
	}

	outcomes = drain(session, now, Command{Kind: CmdTerminate, Username: "gino", SocketID: "sock-G"})
	if outcomes[0].Err != nil || !session.Terminated {
		t.Fatalf("owner terminate = %v, terminated=%v", outcomes[0].Err, session.Terminated)
	}
}

// UN ADMINISTRATEUR ARRÊTE LA PARTIE D'UN AUTRE (Live now, 2026-10-25) — et la
// mention ne vient que du kernel : sans elle, la même demande reste refusée.
func TestAnAdministratorTerminatesSomebodyElsesSession(t *testing.T) {
	session := ready(t, 4, "sock-G")
	mustJoin(t, session, "gino", "sock-G")

	outcomes := drain(session, now, Command{Kind: CmdTerminate, Username: "root"})
	if outcomes[0].Err != ErrNotOwner || session.Terminated {
		t.Fatalf("an unmarked stranger terminating = %v, terminated=%v", outcomes[0].Err, session.Terminated)
	}

	outcomes = drain(session, now, Command{Kind: CmdTerminate, Username: "root", ByAdmin: true})
	if outcomes[0].Err != nil || !session.Terminated {
		t.Fatalf("admin terminate = %v, terminated=%v", outcomes[0].Err, session.Terminated)
	}
	if session.TerminatedBy != "root" || terminationReason(session) != "an administrator (root) ended the session" {
		t.Fatalf("the end must name its administrator: %q", terminationReason(session))
	}
}

// UNE PASSATION PRÊTE LA MANETTE, ELLE NE DONNE PAS LA PARTIE (2026-10-09) :
// arrêter reste au propriétaire, et l'emprunteur ne le peut pas.
func TestTerminateStaysWithTheOwnerAfterAHandover(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-X")
	mustJoin(t, session, "gino", "sock-G")

	drain(session, now, Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "gino"})
	drain(session, now, Command{Kind: CmdGrantHandover, Username: "gino", SocketID: "sock-G"})

	if outcomes := drain(session, now, Command{Kind: CmdTerminate, Username: "gixi", SocketID: "sock-X"}); outcomes[0].Err != ErrNotOwner {
		t.Fatalf("the borrower terminating = %v, want ErrNotOwner", outcomes[0].Err)
	}

	session.OpenTab("sock-G", "gino") // le propriétaire regarde
	outcomes := drain(session, now, Command{Kind: CmdTerminate, Username: "gino", SocketID: "sock-G"})
	if outcomes[0].Err != nil || !session.Terminated {
		t.Fatalf("the owner terminating = %v, terminated = %v", outcomes[0].Err, session.Terminated)
	}
}

func TestUnknownCommandIsRefused(t *testing.T) {
	session := ready(t, 4, "sock-G")

	outcomes := drain(session, now, Command{Kind: "sudo", Username: "gino", SocketID: "sock-G"})
	if outcomes[0].Err != ErrUnknownCommand {
		t.Fatalf("unknown command = %v, want ErrUnknownCommand", outcomes[0].Err)
	}
}

// UNE COMMANDE PARLE POUR SON PROPRE ONGLET (relevé le 2026-10-08).
//
// Les identifiants de socket voyagent en clair : en nommer un autre que le
// sien faisait asseoir l'onglet de quelqu'un d'autre sous son propre nom — ou
// le faisait céder, ou partir. Refusé, et rien ne bouge.
func TestACommandFromSomebodyElsesTabIsRefused(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-D")
	mustJoin(t, session, "gino", "sock-G")

	outcomes := drain(session, now,
		Command{Kind: CmdJoin, Username: "gixi", SocketID: "sock-D"},
		Command{Kind: CmdLeave, Username: "gixi", SocketID: "sock-G"},
	)
	for _, outcome := range outcomes {
		if outcome.Err != ErrNotYourTab {
			t.Fatalf("%s depuis l'onglet d'un autre = %v, want ErrNotYourTab", outcome.Command.Kind, outcome.Err)
		}
	}
	if session.PlayerOf("gixi") != nil || !session.IsPlaying("gino", "sock-G") {
		t.Fatal("une commande venue de l'onglet d'un autre a changé la file")
	}
}
