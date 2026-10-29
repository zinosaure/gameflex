package session

import (
	"testing"
	"time"
)

// PRÊTER LA MANETTE (décidé le 2026-10-09, #274). Le propriétaire prête sa
// manette sans quitter sa place : il garde l'entrée de file, son battement et
// ses droits. L'emprunteur est un « playable viewer » — un spectateur dont les
// touches pilotent le pad 1. Le propriétaire la reprend quand il veut, sans
// confirmation ; l'emprunteur peut la rendre. Un invité, lui, DONNE sa place.

// lend — gino (propriétaire) joue sur la manette 1 et la prête à gixi, qui la
// demandait.
func lend(t *testing.T) *Session {
	t.Helper()

	session := ready(t, 4, "sock-G", "sock-X")
	mustJoin(t, session, "gino", "sock-G")
	outcomes := drain(session, now,
		Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "gino"},
		Command{Kind: CmdGrantHandover, Username: "gino", SocketID: "sock-G"},
	)
	if outcomes[1].Err != nil || outcomes[1].Slot != 1 {
		t.Fatalf("grant = (%d, %v), want (1, nil)", outcomes[1].Slot, outcomes[1].Err)
	}

	return session
}

func TestALoanKeepsTheOwnersPlace(t *testing.T) {
	session := lend(t)

	if !session.IsPlaying("gino", "sock-G") || session.PlayerOf("gixi") != nil {
		t.Fatalf("players = %+v — the owner keeps the place, the borrower holds none", session.Players)
	}
	if session.Owner != "gino" || session.Borrower() != "gixi" {
		t.Fatalf("owner = %q, borrower = %q — want gino, gixi", session.Owner, session.Borrower())
	}
	if got := session.ControlledSlot("sock-X"); got != 1 {
		t.Fatalf("the borrower's keys drive pad %d, want 1", got)
	}
	if got := session.ControlledSlot("sock-G"); got != 0 {
		t.Fatalf("the owner's keys drive pad %d during the loan, want none", got)
	}
	if got := session.ControllerOf(1); got != "sock-X" {
		t.Fatalf("pad 1 is driven from %q, want the borrower's tab", got)
	}
	if session.IdleSince != nil {
		t.Fatal("a lent game is being played: it is not idle")
	}
}

// LE PAD 2 RESTE LIBRE. Quand le prêt déplaçait l'entrée de file, la file
// gardait une place de plus au propriétaire « absent » — déjà assis sur le
// pad 1 prêté : sur une partie à deux manettes, plus personne ne pouvait
// prendre le pad 2 (prouvé par une sonde le 2026-10-09).
func TestALoanLeavesTheOtherPadsFree(t *testing.T) {
	session := ready(t, 2, "sock-G", "sock-X", "sock-D")
	mustJoin(t, session, "gino", "sock-G")
	drain(session, now, Command{Kind: CmdGiveHandover, Username: "gino", SocketID: "sock-G", Target: "gixi"})

	if slot, err := session.Join("dave", "sock-D", now); err != nil || slot != 2 {
		t.Fatalf("join during a loan = (%d, %v), want (2, nil)", slot, err)
	}
}

func TestTheOwnerTakesTheirPadBack(t *testing.T) {
	session := lend(t)

	outcomes := drain(session, now, Command{Kind: CmdReclaim, Username: "gino", SocketID: "sock-G"})
	if outcomes[0].Err != nil || outcomes[0].Slot != 1 {
		t.Fatalf("reclaim = (%d, %v), want (1, nil)", outcomes[0].Slot, outcomes[0].Err)
	}
	if session.Lent() || session.ControlledSlot("sock-G") != 1 || session.ControlledSlot("sock-X") != 0 {
		t.Fatalf("players = %+v — want gino driving pad 1 again, gixi watching", session.Players)
	}
}

func TestOnlyTheOwnerTakesThePadBack(t *testing.T) {
	session := lend(t)
	session.OpenTab("sock-D", "dave")

	for _, command := range []Command{
		{Kind: CmdReclaim, Username: "gixi", SocketID: "sock-X"},
		{Kind: CmdReclaim, Username: "dave", SocketID: "sock-D"},
	} {
		if outcomes := drain(session, now, command); outcomes[0].Err != ErrNotOwner {
			t.Fatalf("reclaim by %s = %v, want ErrNotOwner", command.Username, outcomes[0].Err)
		}
	}
	if session.Borrower() != "gixi" {
		t.Fatal("a refused reclaim must leave the loan alone")
	}
}

func TestNothingToTakeBackWhenNothingIsLent(t *testing.T) {
	session := ready(t, 4, "sock-G")
	mustJoin(t, session, "gino", "sock-G")

	if outcomes := drain(session, now, Command{Kind: CmdReclaim, Username: "gino", SocketID: "sock-G"}); outcomes[0].Err != ErrNotLent {
		t.Fatalf("reclaim of an unlent pad = %v, want ErrNotLent", outcomes[0].Err)
	}
}

// LE PROPRIÉTAIRE NE REPREND QUE SA MANETTE (décidé par le porteur le
// 2026-10-10 : « la sienne seulement »). Celle qu'il a prêtée, oui ; celle
// d'un invité, jamais d'autorité — il peut la DEMANDER, comme tout le monde,
// et l'invité peut refuser.
func TestTheOwnerOnlyTakesBackTheirOwnPad(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-D")
	mustJoin(t, session, "gino", "sock-G")
	mustJoin(t, session, "dave", "sock-D")

	// Reprendre : rien n'est prêté, donc rien à reprendre — surtout pas le
	// pad de dave.
	if outcomes := drain(session, now, Command{Kind: CmdReclaim, Username: "gino", SocketID: "sock-G"}); outcomes[0].Err != ErrNotLent {
		t.Fatalf("reclaim with a guest on pad 2 = %v, want ErrNotLent", outcomes[0].Err)
	}

	// Demander, oui ; et dave garde sa manette s'il refuse.
	outcomes := drain(session, now,
		Command{Kind: CmdRequestHandover, Username: "gino", SocketID: "sock-G", Target: "dave"},
		Command{Kind: CmdDenyHandover, Username: "dave", SocketID: "sock-D"},
	)
	if outcomes[0].Err != nil || outcomes[1].Err != nil {
		t.Fatalf("ask then deny = (%v, %v), want both accepted", outcomes[0].Err, outcomes[1].Err)
	}
	if session.ControlledSlot("sock-D") != 2 || session.ControlledSlot("sock-G") != 1 {
		t.Fatalf("players = %+v — want dave still on pad 2, gino on pad 1", session.Players)
	}
}

// Depuis un autre onglet : la place le suit — c'est celui qu'il regarde.
func TestTheOwnerTakesThePadBackFromAnotherTab(t *testing.T) {
	session := lend(t)
	session.OpenTab("sock-G2", "gino")

	if outcomes := drain(session, now, Command{Kind: CmdReclaim, Username: "gino", SocketID: "sock-G2"}); outcomes[0].Err != nil {
		t.Fatalf("reclaim from another tab = %v", outcomes[0].Err)
	}
	if !session.IsPlaying("gino", "sock-G2") || session.ControlledSlot("sock-G2") != 1 {
		t.Fatalf("players = %+v — want the place on the tab that took the pad back", session.Players)
	}
}

func TestTheBorrowerGivesThePadBack(t *testing.T) {
	session := lend(t)

	outcomes := drain(session, now, Command{Kind: CmdReturn, Username: "gixi", SocketID: "sock-X"})
	if outcomes[0].Err != nil || outcomes[0].Slot != 1 {
		t.Fatalf("return = (%d, %v), want (1, nil)", outcomes[0].Slot, outcomes[0].Err)
	}
	if session.Lent() || session.ControlledSlot("sock-G") != 1 {
		t.Fatalf("players = %+v — want gino driving pad 1 again", session.Players)
	}
}

func TestOnlyTheBorrowingTabGivesThePadBack(t *testing.T) {
	session := lend(t)
	session.OpenTab("sock-X2", "gixi")

	for _, command := range []Command{
		{Kind: CmdReturn, Username: "gixi", SocketID: "sock-X2"}, // another tab of the borrower
		{Kind: CmdReturn, Username: "gino", SocketID: "sock-G"},  // the owner has nothing to return
	} {
		if outcomes := drain(session, now, command); outcomes[0].Err != ErrNotBorrower {
			t.Fatalf("return from %s = %v, want ErrNotBorrower", command.SocketID, outcomes[0].Err)
		}
	}
	if session.Borrower() != "gixi" {
		t.Fatal("a refused return must leave the loan alone")
	}
}

// L'onglet de l'emprunteur se ferme : la manette revient d'elle-même, la place
// n'a jamais quitté le propriétaire.
func TestTheBorrowerLeavingGivesThePadBack(t *testing.T) {
	session := lend(t)
	session.DropSocket("sock-X", now)

	if session.Lent() || session.ControlledSlot("sock-G") != 1 {
		t.Fatalf("players = %+v — want gino driving pad 1 again", session.Players)
	}
	if session.IdleSince != nil {
		t.Fatal("the owner still plays: the game is not idle")
	}
}

// L'onglet du propriétaire se ferme : CAS CLASSIQUE. Sa place tombe, le prêt
// avec elle ; personne d'autre ne joue, la partie passe en idle.
func TestTheOwnerLeavingEndsTheLoan(t *testing.T) {
	session := lend(t)
	session.DropSocket("sock-G", now)

	if session.Lent() || session.ControlledSlot("sock-X") != 0 || session.PlayerOf("gino") != nil {
		t.Fatalf("players = %+v — the place falls, and the loan with it", session.Players)
	}
	if session.IdleSince == nil {
		t.Fatal("nobody plays any more: the game must be idle")
	}
}

// Les autres joueurs restent comme avant : passer sa manette, c'est perdre sa
// place.
func TestAGuestGivesTheirPlace(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-D", "sock-X")
	mustJoin(t, session, "gino", "sock-G")
	mustJoin(t, session, "dave", "sock-D")

	outcomes := drain(session, now, Command{Kind: CmdGiveHandover, Username: "dave", SocketID: "sock-D", Target: "gixi"})
	if outcomes[0].Err != nil || outcomes[0].Slot != 2 {
		t.Fatalf("give = (%d, %v), want (2, nil)", outcomes[0].Slot, outcomes[0].Err)
	}
	if !session.IsPlaying("gixi", "sock-X") || session.PlayerOf("dave") != nil || session.Lent() {
		t.Fatalf("players = %+v — gixi holds the place now, dave keeps nothing", session.Players)
	}
}

// Une manette prêtée ne se redemande pas, ne se reprête pas et ne se passe
// pas : seul son propriétaire la reprend.
func TestALentPadIsOnlyTakenBack(t *testing.T) {
	session := lend(t)
	session.OpenTab("sock-D", "dave")

	cases := []struct {
		command Command
		want    error
	}{
		{Command{Kind: CmdRequestHandover, Username: "dave", SocketID: "sock-D", Target: "gino"}, ErrPadLent},
		{Command{Kind: CmdGiveHandover, Username: "gino", SocketID: "sock-G", Target: "dave"}, ErrPadLent},
		{Command{Kind: CmdGiveHandover, Username: "gixi", SocketID: "sock-X", Target: "dave"}, ErrNotPlaying},
	}
	for _, c := range cases {
		if outcomes := drain(session, now, c.command); outcomes[0].Err != c.want {
			t.Fatalf("%s by %s = %v, want %v", c.command.Kind, c.command.Username, outcomes[0].Err, c.want)
		}
	}
	if session.Borrower() != "gixi" {
		t.Fatal("the loan must survive every refused attempt")
	}
}

// L'emprunteur regarde : une personne, une manette.
func TestTheOwnerLendsOnlyToSomebodyWatching(t *testing.T) {
	session := ready(t, 4, "sock-G", "sock-D")
	mustJoin(t, session, "gino", "sock-G")
	mustJoin(t, session, "dave", "sock-D")

	if outcomes := drain(session, now, Command{Kind: CmdGiveHandover, Username: "gino", SocketID: "sock-G", Target: "dave"}); outcomes[0].Err != ErrAlreadyPlaying {
		t.Fatalf("lending to a player = %v, want ErrAlreadyPlaying", outcomes[0].Err)
	}
}

// Prendre une place à soi met fin à l'emprunt.
func TestABorrowerWhoJoinsStopsBorrowing(t *testing.T) {
	session := lend(t)

	if slot, err := session.Join("gixi", "sock-X", now); err != nil || slot != 2 {
		t.Fatalf("join = (%d, %v), want (2, nil)", slot, err)
	}
	if session.Lent() || session.ControlledSlot("sock-X") != 2 || session.ControlledSlot("sock-G") != 1 {
		t.Fatalf("players = %+v — gixi on their own pad, gino back on theirs", session.Players)
	}
}

// Le propriétaire qui change d'onglet garde son prêt : c'est la place qui le
// porte.
func TestATakeoverKeepsTheLoan(t *testing.T) {
	session := lend(t)
	session.OpenTab("sock-G2", "gino")

	if _, err := session.Takeover("gino", "sock-G2", now); err != nil {
		t.Fatalf("takeover = %v", err)
	}
	if session.Borrower() != "gixi" || session.ControlledSlot("sock-X") != 1 || session.ControlledSlot("sock-G2") != 0 {
		t.Fatalf("players = %+v — the loan stays with the place", session.Players)
	}
}

// LE BATTEMENT RESTE AU PROPRIÉTAIRE, LES TOUCHES SONT À L'EMPRUNTEUR.
func TestDuringALoanTheBeatIsTheOwnersAndTheKeysTheBorrowers(t *testing.T) {
	session := lend(t)
	later := now.Add(10 * time.Minute)

	session.Beat("sock-X", true, later) // the borrower plays
	if place := session.PlayerOf("gino"); !place.LastInputAt.Equal(later) || !place.LastBeatAt.Equal(now) {
		t.Fatalf("the borrower's keys feed the place's activity, not its beat: %+v", place)
	}

	evenLater := later.Add(time.Minute)
	session.Beat("sock-G", true, evenLater) // the owner fidgets while watching
	if place := session.PlayerOf("gino"); !place.LastBeatAt.Equal(evenLater) || !place.LastInputAt.Equal(later) {
		t.Fatalf("the owner's beat keeps the place alive, their gesture drives nothing: %+v", place)
	}
}

// --- vu par les onglets -----------------------------------------------------

// L'onglet sait ce qu'il peut faire du prêt — le child le dit, la page ne le
// déduit pas.
func TestEachTabIsToldWhatItCanDoWithALoan(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-X", "gixi")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)
	host.Submit(Command{Kind: CmdGiveHandover, Username: "gino", SocketID: "sock-G", Target: "gixi"}, now)

	owner, borrower := you(t, rec, "sock-G"), you(t, rec, "sock-X")
	if owner["isPlaying"] != true || owner["controls"] != 0 || owner["canReclaim"] != true || owner["canReturn"] != false {
		t.Fatalf("owner = %+v, want a place, no pad, canReclaim only", owner)
	}
	if borrower["isPlaying"] != false || borrower["controls"] != 1 || borrower["canReturn"] != true || borrower["canReclaim"] != false {
		t.Fatalf("borrower = %+v, want no place, pad 1, canReturn only", borrower)
	}
	if state := rec.last("sock-X", "state"); state["lentTo"] != "gixi" || state["owner"] != "gino" {
		t.Fatalf("state = %+v, want lentTo gixi with gino the owner", state)
	}
	if received := rec.last("sock-X", "result"); received["status"] != "received" || received["lent"] != true {
		t.Fatalf("the borrower must be told the pad is lent to them, got %+v", received)
	}
	if snapshot := host.Snapshot(); len(snapshot.Players) != 1 || snapshot.Players[0].LentTo != "gixi" {
		t.Fatalf("snapshot = %+v — the kernel must see who drives the owner's pad", snapshot)
	}
}

// « TOUJOURS LÀ ? » PENDANT UN PRÊT. La question va aux deux ; un geste du
// propriétaire y répond ; sans réponse, cas classique : la place est exclue,
// le prêt avec elle.
func TestTheAFKQuestionDuringALoan(t *testing.T) {
	const (
		beatTimeout = 90 * time.Second
		afkAfter    = 29 * time.Minute
		afkGrace    = time.Minute
	)

	host, rec := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-X", "gixi")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)
	host.Submit(Command{Kind: CmdGiveHandover, Username: "gino", SocketID: "sock-G", Target: "gixi"}, now)

	// The owner keeps beating; nobody touches anything for 29 minutes.
	clock := now
	for clock.Before(now.Add(afkAfter + time.Second)) {
		clock = clock.Add(30 * time.Second)
		host.Beat("sock-G", false, clock)
	}
	_, warnings := host.Sweep(clock, beatTimeout, afkAfter, afkGrace)
	asked := map[string]bool{}
	for _, warning := range warnings {
		asked[warning.SocketID] = true
	}
	if !asked["sock-G"] || !asked["sock-X"] {
		t.Fatalf("warnings = %+v — the question goes to both the owner and the borrower", warnings)
	}

	// The owner answers with a gesture: the question is settled.
	clock = clock.Add(20 * time.Second)
	host.Beat("sock-G", true, clock)
	if closing, _ := host.Sweep(clock, beatTimeout, afkAfter, afkGrace); len(closing) != 0 {
		t.Fatalf("closing = %v — the owner answered, nobody is excluded", closing)
	}
	if !host.Session().PlayerOf("gino").AFKWarnedAt.IsZero() {
		t.Fatal("the owner's gesture must clear the warning")
	}

	// Asked again later, and this time nobody answers: the classic case.
	for clock.Before(now.Add(2*afkAfter + time.Minute)) {
		clock = clock.Add(30 * time.Second)
		host.Beat("sock-G", false, clock)
	}
	host.Sweep(clock, beatTimeout, afkAfter, afkGrace)
	clock = clock.Add(afkGrace + time.Second)
	host.Beat("sock-G", false, clock)
	closing, _ := host.Sweep(clock, beatTimeout, afkAfter, afkGrace)
	if len(closing) != 1 || closing[0] != "sock-G" {
		t.Fatalf("closing = %v — want the owner's place excluded", closing)
	}
	if host.Session().Lent() || host.Session().PlayerOf("gino") != nil {
		t.Fatal("the place is excluded, and the loan with it")
	}
	if you(t, rec, "sock-X")["controls"] != 0 {
		t.Fatal("the borrower's keys must drive nothing once the loan has ended")
	}
}

// LE PROPRIÉTAIRE N'EST PLUS LÀ — onglet gelé : son battement manque, sa place
// tombe, même si l'emprunteur joue. C'est sa présence qui porte le prêt.
func TestAFrozenOwnerEndsTheLoanEvenWhileTheBorrowerPlays(t *testing.T) {
	const beatTimeout = 90 * time.Second

	host, _ := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-X", "gixi")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)
	host.Submit(Command{Kind: CmdGiveHandover, Username: "gino", SocketID: "sock-G", Target: "gixi"}, now)

	clock := now
	for step := 0; step < 4; step++ {
		clock = clock.Add(30 * time.Second)
		host.Beat("sock-X", true, clock) // the borrower plays; the owner's tab is frozen
	}
	closing, _ := host.Sweep(clock, beatTimeout, 29*time.Minute, time.Minute)
	if len(closing) != 1 || closing[0] != "sock-G" {
		t.Fatalf("closing = %v — want the frozen owner's place", closing)
	}
	if host.Session().Lent() {
		t.Fatal("the loan must end with the owner's place")
	}
}
