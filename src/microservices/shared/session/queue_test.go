package session

import (
	"testing"
	"time"
)

var now = time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

func newSession(maxPlayers int) *Session {
	return New("s1", "retrogaming", "gino", maxPlayers, now)
}

func mustJoin(t *testing.T, session *Session, username, socketID string) int {
	t.Helper()

	// The tab connects BEFORE it can ask for anything — a slot is only ever
	// held from an open tab (invariant 6). Shortcutting this in tests would
	// describe a sequence that cannot happen in production.
	session.OpenTab(socketID, username)

	slot, err := session.Join(username, socketID, now)
	if err != nil {
		t.Fatalf("Join(%q) = %v, want success", username, err)
	}

	return slot
}

// --- la règle que l'interface consomme ------------------------------------

// Un onglet joue si et seulement si SON socket est celui de l'entrée. Le
// second onglet du même joueur regarde, par construction : il ne peut pas
// être le socket d'une entrée qui n'en contient qu'un.
func TestIsPlayingIsTheSocket(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-A")

	if !session.IsPlaying("gino", "sock-A") {
		t.Fatal("the socket holding the slot must be playing")
	}
	if session.IsPlaying("gino", "sock-B") {
		t.Fatal("a second tab of the same player must be watching")
	}
	if session.IsPlaying("dave", "sock-A") {
		t.Fatal("another username must never be playing on someone else's socket")
	}
}

// Ouvrir la page crée un participant, jamais une entrée de queue : c'est ce
// qui rend un lien de streaming inerte, donc sûr à partager.
func TestOpenTabGrantsNothing(t *testing.T) {
	session := newSession(4)
	session.OpenTab("sock-A", "gino")

	if len(session.Players) != 0 {
		t.Fatalf("opening a tab created a queue entry: %+v", session.Players)
	}
	if session.IsPlaying("gino", "sock-A") {
		t.Fatal("a participant must not be playing")
	}
}

// --- attribution des slots ------------------------------------------------

func TestOwnerIsAlwaysPlayerOne(t *testing.T) {
	session := newSession(4)

	if slot := mustJoin(t, session, "dave", "sock-D"); slot != 2 {
		t.Fatalf("first joiner got slot %d, want 2 — slot 1 is the owner's", slot)
	}
	if slot := mustJoin(t, session, "alex", "sock-A"); slot != 3 {
		t.Fatalf("second joiner got slot %d, want 3", slot)
	}
	if slot := mustJoin(t, session, "gino", "sock-G"); slot != 1 {
		t.Fatalf("owner got slot %d, want 1 — theirs is always free", slot)
	}
}

// L'exemple de l'utilisateur, mot pour mot : 1/2/3 occupés, 2 part, les ports
// libres sont 2 et 4 — le suivant reçoit 2, jamais 4.
func TestSlotsAreReusedSmallestFirst(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-G") // 1
	mustJoin(t, session, "dave", "sock-D") // 2
	mustJoin(t, session, "alex", "sock-A") // 3

	if err := session.Leave("dave", now); err != nil {
		t.Fatalf("Leave = %v", err)
	}
	if slot := mustJoin(t, session, "carol", "sock-C"); slot != 2 {
		t.Fatalf("newcomer got slot %d, want 2 (smallest free), never 4", slot)
	}
}

// --- invariants de la queue -----------------------------------------------

func TestOneEntryPerUsername(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "dave", "sock-D")

	session.OpenTab("sock-D2", "dave")
	if _, err := session.Join("dave", "sock-D2", now); err != ErrAlreadyPlaying {
		t.Fatalf("second join for the same username = %v, want ErrAlreadyPlaying", err)
	}
}

// Une console à un pad ne se rejoint JAMAIS, elle se regarde — et ça tombe de
// l'invariant, sans code par plateforme.
func TestOnePadSessionIsNeverJoinable(t *testing.T) {
	session := newSession(1)

	if got := session.AvailableSlots(); got != 0 {
		t.Fatalf("AvailableSlots on a one-pad session = %d, want 0", got)
	}
	session.OpenTab("sock-D", "dave")
	if _, err := session.Join("dave", "sock-D", now); err != ErrNoSlot {
		t.Fatalf("joiner on a one-pad session = %v, want ErrNoSlot", err)
	}
	if slot := mustJoin(t, session, "gino", "sock-G"); slot != 1 {
		t.Fatalf("owner on a one-pad session got %d, want 1", slot)
	}
}

// La queue ne peut pas se remplir à 100 % en l'absence du owner : c'est ce qui
// garde son slot libre sans le réserver explicitement.
func TestQueueNeverFillsWithoutTheOwner(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "dave", "sock-D")
	mustJoin(t, session, "alex", "sock-A")
	mustJoin(t, session, "carol", "sock-C")

	if got := session.AvailableSlots(); got != 0 {
		t.Fatalf("AvailableSlots = %d, want 0 — the last seat is the owner's", got)
	}
	session.OpenTab("sock-E", "eve")
	if _, err := session.Join("eve", "sock-E", now); err != ErrNoSlot {
		t.Fatalf("fourth joiner = %v, want ErrNoSlot", err)
	}
	// …et le owner rentre malgré tout.
	if slot := mustJoin(t, session, "gino", "sock-G"); slot != 1 {
		t.Fatalf("owner returning to a 'full' session got %d, want 1", slot)
	}
	if len(session.Players) != 4 {
		t.Fatalf("queue = %d entries, want 4", len(session.Players))
	}
}

func TestAnonymousNeverPlays(t *testing.T) {
	session := newSession(4)

	if _, err := session.Join("", "sock-X", now); err != ErrEmptyUsername {
		t.Fatalf("anonymous join = %v, want ErrEmptyUsername", err)
	}
}

// --- takeover -------------------------------------------------------------

func TestTakeoverMovesTheSlotToTheOtherTab(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-A")

	session.OpenTab("sock-B", "gino")
	slot, err := session.Takeover("gino", "sock-B", now)
	if err != nil || slot != 1 {
		t.Fatalf("Takeover = (%d, %v), want (1, nil)", slot, err)
	}
	if session.IsPlaying("gino", "sock-A") {
		t.Fatal("the old tab must stop playing the moment the slot moves")
	}
	if !session.IsPlaying("gino", "sock-B") {
		t.Fatal("the new tab must be playing")
	}
	if len(session.Players) != 1 {
		t.Fatalf("takeover duplicated the entry: %+v", session.Players)
	}
}

func TestTakeoverNeedsAnExistingSlot(t *testing.T) {
	session := newSession(4)

	if _, err := session.Takeover("dave", "sock-D", now); err != ErrNotPlaying {
		t.Fatalf("takeover without a slot = %v, want ErrNotPlaying", err)
	}
}

// --- handover -------------------------------------------------------------

// Le passage est ATOMIQUE : à aucun instant le pad n'appartient à personne.
// C'est ce qui supprime la réservation de 15 s du chantier 3, et avec elle le
// snipe et l'expiration. Le propriétaire, lui, PRÊTE (2026-10-09) : il garde
// sa place, et les touches de l'emprunteur pilotent son pad sur-le-champ.
func TestHandoverIsAtomicAndTheOwnerLends(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-G")

	session.OpenTab("sock-X", "gixi")
	slot, err := session.Handover("gino", "gixi", "sock-X", now)
	if err != nil || slot != 1 {
		t.Fatalf("Handover = (%d, %v), want (1, nil)", slot, err)
	}
	if session.Owner != "gino" || session.Borrower() != "gixi" {
		t.Fatalf("owner = %q, borrower = %q — want gino lending to gixi", session.Owner, session.Borrower())
	}
	if !session.IsPlaying("gino", "sock-G") || session.PlayerOf("gixi") != nil {
		t.Fatalf("players = %+v — the owner keeps the place, the borrower takes none", session.Players)
	}
	if session.ControlledSlot("sock-X") != 1 {
		t.Fatal("the borrower must drive the pad immediately, with nothing to claim")
	}
}

func TestHandoverOfAnotherSlotKeepsOwnership(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-G")
	mustJoin(t, session, "dave", "sock-D")

	session.OpenTab("sock-X", "gixi")
	if _, err := session.Handover("dave", "gixi", "sock-X", now); err != nil {
		t.Fatalf("Handover = %v", err)
	}
	if session.Owner != "gino" {
		t.Fatalf("owner = %q after a P2 hand-over, want gino unchanged", session.Owner)
	}
}

// Un joueur à qui un invité donne sa place CHANGE de slot, il n'en tient pas
// deux (invariant 1).
func TestAGuestGivingTheirPlaceToAPlayerMovesThem(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-G")  // 1
	mustJoin(t, session, "dave", "sock-D")  // 2
	mustJoin(t, session, "carol", "sock-C") // 3

	if _, err := session.Handover("carol", "dave", "sock-D", now); err != nil {
		t.Fatalf("Handover = %v", err)
	}
	if len(session.Players) != 2 {
		t.Fatalf("queue = %+v, want two entries", session.Players)
	}
	if player := session.PlayerOf("dave"); player == nil || player.SlotID != 3 {
		t.Fatalf("dave = %+v, want slot 3", player)
	}
	if session.Owner != "gino" {
		t.Fatalf("owner = %q, want gino — a guest's place moved, the game did not", session.Owner)
	}
}

// --- socket mort = dehors -------------------------------------------------

func TestDropSocketOnlyReleasesTheHoldingSocket(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-A")
	session.OpenTab("sock-B", "gino") // second onglet du même joueur

	if _, dropped := session.DropSocket("sock-B", now); dropped {
		t.Fatal("closing the OTHER tab must not cost the player their slot")
	}
	if !session.IsPlaying("gino", "sock-A") {
		t.Fatal("the holder must still be playing")
	}

	username, dropped := session.DropSocket("sock-A", now)
	if !dropped || username != "gino" {
		t.Fatalf("DropSocket = (%q, %v), want (gino, true)", username, dropped)
	}
	if len(session.Players) != 0 {
		t.Fatalf("queue = %+v, want empty", session.Players)
	}
}

// --- idle -----------------------------------------------------------------

func TestIdleStartsWhenTheQueueEmpties(t *testing.T) {
	session := newSession(4)

	if session.IdleSince == nil {
		t.Fatal("a session nobody plays yet must be idle from birth")
	}

	mustJoin(t, session, "gino", "sock-G")
	if session.IdleSince != nil {
		t.Fatal("somebody is playing — the idle clock must stop")
	}

	if err := session.Leave("gino", now); err != nil {
		t.Fatalf("Leave = %v", err)
	}
	if session.IdleSince == nil {
		t.Fatal("the queue emptied — the idle clock must start")
	}
}

// --- liveness -------------------------------------------------------------

// Le tick prouve que l'onglet TOURNE ; seul hadInput prouve qu'un HUMAIN est
// là. Les confondre, c'est ce qui avait fait prévenir l'AFK au mauvais moment.
func TestBeatSeparatesTabAliveFromHumanPresent(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-G")

	later := now.Add(10 * time.Minute)
	if !session.Beat("sock-G", false, later) {
		t.Fatal("a beat from the holding socket must be recorded")
	}

	player := session.PlayerOf("gino")
	if !player.LastBeatAt.Equal(later) {
		t.Fatal("an idle beat must still prove the tab runs")
	}
	if !player.LastInputAt.Equal(now) {
		t.Fatal("an idle beat must NOT count as human activity")
	}

	evenLater := later.Add(time.Minute)
	session.Beat("sock-G", true, evenLater)
	if !session.PlayerOf("gino").LastInputAt.Equal(evenLater) {
		t.Fatal("a beat carrying input must count as human activity")
	}
}

func TestBeatFromAWatcherIsIgnored(t *testing.T) {
	session := newSession(4)
	session.OpenTab("sock-W", "dave")

	if session.Beat("sock-W", true, now) {
		t.Fatal("a watcher holds no slot — there is nothing to stamp")
	}
}

// --- déterminisme ---------------------------------------------------------

// L'attribution ne doit dépendre d'aucun ordre d'itération : un ordre de map
// décidant d'un gagnant nous a déjà coûté un vrai bug.
func TestSlotAssignmentIsDeterministic(t *testing.T) {
	for iteration := 0; iteration < 200; iteration++ {
		session := newSession(4)
		mustJoin(t, session, "gino", "sock-G")
		mustJoin(t, session, "dave", "sock-D")
		mustJoin(t, session, "alex", "sock-A")

		if err := session.Leave("dave", now); err != nil {
			t.Fatalf("Leave = %v", err)
		}
		if slot := mustJoin(t, session, "carol", "sock-C"); slot != 2 {
			t.Fatalf("iteration %d: newcomer got slot %d, want 2 every single time", iteration, slot)
		}
	}
}

// --- invariant 6 : un slot ne se tient que depuis un onglet ouvert --------

// Une entrée dont le socket n'existe pas est un fantôme qui tient un pad que
// personne ne peut prendre — la forme exacte du bug sur lequel la v1 a passé
// des jours. L'invariant est porté par la queue elle-même, donc aucun
// appelant ne peut en fabriquer un, quel que soit le chemin emprunté.
func TestJoinNeedsAnOpenTab(t *testing.T) {
	session := newSession(4)

	if _, err := session.Join("gino", "sock-ghost", now); err != ErrSocketClosed {
		t.Fatalf("join from a socket that never opened = %v, want ErrSocketClosed", err)
	}
	if len(session.Players) != 0 {
		t.Fatalf("a phantom got a slot: %+v", session.Players)
	}
}

func TestJoinRefusedOnceTheTabClosed(t *testing.T) {
	session := newSession(4)
	session.OpenTab("sock-G", "gino")
	session.DropSocket("sock-G", now)

	if _, err := session.Join("gino", "sock-G", now); err != ErrSocketClosed {
		t.Fatalf("join from a closed tab = %v, want ErrSocketClosed", err)
	}
}

func TestTakeoverNeedsAnOpenTab(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-A")

	if _, err := session.Takeover("gino", "sock-ghost", now); err != ErrSocketClosed {
		t.Fatalf("takeover onto a dead socket = %v, want ErrSocketClosed", err)
	}
	if !session.IsPlaying("gino", "sock-A") {
		t.Fatal("the live tab must keep the slot when the takeover is refused")
	}
}

// Le cas qui compte le plus : le détenteur ne doit pas perdre sa manette
// parce que le demandeur s'est évaporé.
func TestHandoverNeedsTheRequesterStillThere(t *testing.T) {
	session := newSession(4)
	mustJoin(t, session, "gino", "sock-G")

	if _, err := session.Handover("gino", "gixi", "sock-ghost", now); err != ErrSocketClosed {
		t.Fatalf("hand-over to a dead socket = %v, want ErrSocketClosed", err)
	}
	if !session.IsPlaying("gino", "sock-G") {
		t.Fatal("the holder must keep their pad when the requester has vanished")
	}
	if session.Owner != "gino" {
		t.Fatalf("owner = %q, want gino — nothing moved", session.Owner)
	}
}

// A session whose owner is nobody must still be playable. Regression test for
// the PS5 that answered "no slot available" to its own owner for its entire
// life (2026-08-25): slot 1 was held for "anonymous", and with MaxPlayers 1
// every real arrival started looking at slot 2, which does not exist.
func TestOwnerlessSessionIsStillPlayable(t *testing.T) {
	for _, owner := range []string{"", "anonymous"} {
		session := New("s1", "remoteplay", owner, 1, now)
		session.OpenTab("sock-a", "gino")

		if _, err := session.Join("gino", "sock-a", now); err != nil {
			t.Fatalf("owner=%q: join = %v, want a slot", owner, err)
		}
		if got := session.PlayerOf("gino"); got == nil || got.SlotID != 1 {
			t.Fatalf("owner=%q: player = %+v, want slot 1", owner, got)
		}
	}
}

// …but a REAL owner still gets slot 1 held for them.
func TestRealOwnerStillReservesSlotOne(t *testing.T) {
	session := New("s1", "retrogaming", "gino", 2, now)
	session.OpenTab("sock-b", "dave")

	if _, err := session.Join("dave", "sock-b", now); err != nil {
		t.Fatalf("join = %v", err)
	}
	if got := session.PlayerOf("dave"); got == nil || got.SlotID != 2 {
		t.Fatalf("player = %+v, want slot 2 (slot 1 is gino's)", got)
	}
}

// --- give-handover: the gesture that starts from the person holding the pad --

func TestTheOwnersGiftLendsTheOnePad(t *testing.T) {
	now := time.Now()
	session := New("s", "remoteplay", "owner", 1, now)
	session.OpenTab("owner-tab", "owner")
	session.OpenTab("guest-tab", "guest")

	if _, err := session.Join("owner", "owner-tab", now); err != nil {
		t.Fatalf("owner join: %v", err)
	}

	slot, err := session.GiveTo("owner", "owner-tab", "guest", now)
	if err != nil {
		t.Fatalf("give: %v", err)
	}
	if slot != 1 {
		t.Fatalf("the ONE pad is slot 1, got %d", slot)
	}
	if session.ControlledSlot("guest-tab") != 1 {
		t.Fatal("the recipient must drive the pad from their own tab")
	}
	// The owner's gift LENDS, like a requested hand-over (2026-10-09): they
	// keep their place, and the game stays theirs.
	if !session.IsPlaying("owner", "owner-tab") || session.Borrower() != "guest" {
		t.Fatalf("players = %+v, borrower = %q — want the owner seated, lending to guest", session.Players, session.Borrower())
	}
}

func TestGiveToRefusesFromATabThatDoesNotHoldThePad(t *testing.T) {
	now := time.Now()
	session := New("s", "streamex", "owner", 2, now)
	session.OpenTab("first", "owner")
	session.OpenTab("second", "owner") // same person, another tab: it watches
	session.OpenTab("guest-tab", "guest")

	if _, err := session.Join("owner", "first", now); err != nil {
		t.Fatalf("join: %v", err)
	}

	// The socket is the identity. The second tab is not the holder, so it
	// cannot give away what it does not have.
	if _, err := session.GiveTo("owner", "second", "guest", now); err != ErrNotPlaying {
		t.Fatalf("expected ErrNotPlaying from a non-holding tab, got %v", err)
	}
	if !session.IsPlaying("owner", "first") {
		t.Fatal("the real holder must be untouched by a refused gift")
	}
}

func TestGiveToNeedsSomebodyThere(t *testing.T) {
	now := time.Now()
	session := New("s", "remoteplay", "owner", 1, now)
	session.OpenTab("owner-tab", "owner")
	if _, err := session.Join("owner", "owner-tab", now); err != nil {
		t.Fatalf("join: %v", err)
	}

	// Nobody named "ghost" has a tab open — a pad cannot be pushed at someone
	// who is not there to receive it.
	if _, err := session.GiveTo("owner", "owner-tab", "ghost", now); err != ErrSocketClosed {
		t.Fatalf("expected ErrSocketClosed, got %v", err)
	}
	if !session.IsPlaying("owner", "owner-tab") {
		t.Fatal("a refused gift must leave the holder holding")
	}
}

func TestGiveToLandsOnTheRecipientsNewestTab(t *testing.T) {
	now := time.Now()
	session := New("s", "streamex", "owner", 2, now)
	session.OpenTab("owner-tab", "owner")
	session.OpenTab("guest-old", "guest")
	session.OpenTab("guest-new", "guest")

	if _, err := session.Join("owner", "owner-tab", now); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := session.GiveTo("owner", "owner-tab", "guest", now); err != nil {
		t.Fatalf("give: %v", err)
	}

	if session.ControlledSlot("guest-new") != 1 {
		t.Fatal("the newest tab is the one they are looking at")
	}
	if session.ControlledSlot("guest-old") != 0 {
		t.Fatal("an older tab keeps watching, like any second tab")
	}
}

func TestGiveToRefusesAPlayerWhoAlreadyHasAPad(t *testing.T) {
	now := time.Now()
	session := New("s", "retrogaming", "owner", 4, now)
	session.OpenTab("owner-tab", "owner")
	session.OpenTab("guest-tab", "guest")

	if _, err := session.Join("owner", "owner-tab", now); err != nil {
		t.Fatalf("owner join: %v", err)
	}
	if _, err := session.Join("guest", "guest-tab", now); err != nil {
		t.Fatalf("guest join: %v", err)
	}

	if _, err := session.GiveTo("owner", "owner-tab", "guest", now); err != ErrAlreadyPlaying {
		t.Fatalf("expected ErrAlreadyPlaying, got %v", err)
	}
}

func TestWatchersAreThePeopleWithoutAPad(t *testing.T) {
	now := time.Now()
	session := New("s", "streamex", "owner", 4, now)
	session.OpenTab("owner-tab", "owner")
	session.OpenTab("guest-one", "guest")
	session.OpenTab("guest-two", "guest") // same person, two tabs
	session.OpenTab("anon", "")           // anonymous: no identity to hold a pad

	if _, err := session.Join("owner", "owner-tab", now); err != nil {
		t.Fatalf("join: %v", err)
	}

	watchers := session.Watchers()
	if len(watchers) != 1 || watchers[0] != "guest" {
		t.Fatalf("one person, listed once, players and anonymous excluded: %v", watchers)
	}
}
