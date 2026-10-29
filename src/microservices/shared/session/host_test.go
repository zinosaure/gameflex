package session

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// recorder captures what each socket was told, so a test can assert on what a
// tab actually sees rather than on internals.
type recorder struct {
	sent map[string][]map[string]any
	dead map[string]bool
}

func newRecorder() *recorder {
	return &recorder{sent: map[string][]map[string]any{}, dead: map[string]bool{}}
}

// dead sockets refuse writes, exactly as a real transport would.
func (r *recorder) send(socketID string, message map[string]any) error {
	if r.dead[socketID] {
		return errWriteFailed
	}
	r.sent[socketID] = append(r.sent[socketID], message)

	return nil
}

// kill makes every later write to that socket fail — a peer that went away
// without the courtesy of a close frame.
func (r *recorder) kill(socketID string) {
	r.dead[socketID] = true
}

var errWriteFailed = errors.New("write failed")

// last returns the most recent message of a kind for a socket.
func (r *recorder) last(socketID, kind string) map[string]any {
	messages := r.sent[socketID]
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index]["type"] == kind {
			return messages[index]
		}
	}

	return nil
}

func newHost(t *testing.T, maxPlayers int) (*Host, *recorder) {
	t.Helper()

	rec := newRecorder()

	return NewHost(New("s1", "retrogaming", "gino", maxPlayers, now), rec.send), rec
}

func you(t *testing.T, rec *recorder, socketID string) map[string]any {
	t.Helper()

	state := rec.last(socketID, "state")
	if state == nil {
		t.Fatalf("socket %q never received a state message", socketID)
	}

	return state["you"].(map[string]any)
}

// --- ce qu'un onglet voit -------------------------------------------------

// Un onglet apprend SON identifiant de socket (il en a besoin pour ses
// webhooks) et jamais celui de quelqu'un d'autre.
func TestAttachTellsTheTabItsOwnSocketOnly(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-D", "dave")

	welcome := rec.last("sock-G", "welcome")
	if welcome == nil || welcome["socketId"] != "sock-G" {
		t.Fatalf("welcome = %+v, want its own socket id", welcome)
	}

	state := rec.last("sock-G", "state")
	for _, player := range state["players"].([]map[string]any) {
		if _, leaked := player["socketId"]; leaked {
			t.Fatal("a socket id leaked into the players list")
		}
	}
}

// « Suis-je en train de jouer » est une question sur CET onglet, donc la
// réponse est par socket — la page n'a jamais à comparer des identifiants.
func TestStateAnswersPerTab(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-A", "gino")
	host.Attach("sock-B", "gino") // second onglet du même joueur
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-A"}, now)

	if you(t, rec, "sock-A")["isPlaying"] != true {
		t.Fatal("the holding tab must be told it is playing")
	}
	if you(t, rec, "sock-B")["isPlaying"] != false {
		t.Fatal("the second tab of the same player must be told it is watching")
	}
}

// LE RAPPORT NOMME AUSSI CEUX QUI REGARDENT — une fois par personne, et jamais
// ceux qui tiennent une place. C'est ce que le /status d'un runtime annonce
// (P5) : la file sait qui est là, elle le dit.
func TestTheSnapshotNamesTheWatchersOncePerPerson(t *testing.T) {
	host, _ := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-A1", "alice")
	host.Attach("sock-A2", "alice") // deux onglets, une personne
	host.Attach("sock-B", "bob")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)

	watchers := host.Snapshot().Watchers
	if fmt.Sprint(watchers) != "[alice bob]" {
		t.Fatalf("watchers = %v, want [alice bob] — the seated owner is not one", watchers)
	}
}

// --- la sérialisation, vue de l'extérieur ---------------------------------

// Deux intentions concurrentes sur la dernière place : une seule passe, et
// l'autre reçoit un refus explicite — pas un silence.
func TestConcurrentJoinsAreSerialised(t *testing.T) {
	host, rec := newHost(t, 2) // owner + une seule place de joiner
	host.Attach("sock-D", "dave")
	host.Attach("sock-C", "carol")

	host.Submit(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D"}, now)
	host.Submit(Command{Kind: CmdJoin, Username: "carol", SocketID: "sock-C"}, now)

	if rec.last("sock-D", "result")["status"] != "granted" {
		t.Fatalf("first join = %+v, want granted", rec.last("sock-D", "result"))
	}
	if rec.last("sock-C", "result")["status"] != "refused" {
		t.Fatalf("second join = %+v, want refused", rec.last("sock-C", "result"))
	}
	if you(t, rec, "sock-C")["isPlaying"] != false {
		t.Fatal("the refused tab must be told it is watching")
	}
}

// Le résultat distingue « abandonnée » de « refusée » : le monde a bougé
// n'est pas la même chose que tu n'avais pas le droit, et les confondre
// produit des messages qui mentent.
func TestAbandonedIsNotRefused(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-X", "gixi")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)
	host.Submit(Command{Kind: CmdRequestHandover, Username: "gixi", SocketID: "sock-X", Target: "gino"}, now)

	host.Detach("sock-X", now) // le demandeur ferme son onglet

	host.Submit(Command{Kind: CmdGrantHandover, Username: "gino", SocketID: "sock-G"}, now)
	if rec.last("sock-G", "result")["status"] != "abandoned" {
		t.Fatalf("grant = %+v, want abandoned", rec.last("sock-G", "result"))
	}
	if you(t, rec, "sock-G")["isPlaying"] != true {
		t.Fatal("the holder must keep their pad when the requester vanished")
	}
}

// --- socket mort = dehors -------------------------------------------------

func TestDetachReleasesOnlyTheHoldingSocket(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-A", "gino")
	host.Attach("sock-B", "gino")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-A"}, now)

	host.Detach("sock-B", now)
	if you(t, rec, "sock-A")["isPlaying"] != true {
		t.Fatal("closing the other tab must not cost the player their slot")
	}

	host.Detach("sock-A", now)
	if len(host.Session().Players) != 0 {
		t.Fatalf("queue = %+v, want empty", host.Session().Players)
	}
}

// --- liveness -------------------------------------------------------------

// L'onglet GELÉ : son socket reste ouvert, son JS ne tourne plus. Seul
// l'arrêt des beats le révèle.
func TestSweepDropsAFrozenTab(t *testing.T) {
	host, _ := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)

	later := now.Add(3 * time.Minute)
	closing, _ := host.Sweep(later, time.Minute, 15*time.Minute, time.Minute)

	if len(closing) != 1 || closing[0] != "sock-G" {
		t.Fatalf("Sweep = %v, want the frozen tab", closing)
	}
	if len(host.Session().Players) != 0 {
		t.Fatal("a frozen tab must not keep holding a pad")
	}
}

// L'onglet ABANDONNÉ : il bat toujours, personne n'est derrière. Le
// propriétaire est exclu comme les autres, et la partie NE S'ARRÊTE PAS : elle
// n'est inactive que lorsque plus personne ne joue (2026-10-09 : « if
// activePlayers > 0 then continue, else idle »).
func TestIdleOwnerAndGuestAreKickedAndTheGameGoesIdle(t *testing.T) {
	host, _ := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-D", "dave")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)
	host.Submit(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D"}, now)

	// Les deux battent régulièrement, sans jamais toucher à rien.
	later := now
	for tick := 0; tick < 40; tick++ {
		later = later.Add(30 * time.Second)
		host.Beat("sock-G", false, later)
		host.Beat("sock-D", false, later)
	}

	// ON PRÉVIENT D'ABORD. Le premier balayage ne fait que poser la
	// question — c'est tout l'objet du compte à rebours, et la session doit
	// survivre à ce moment-là.
	_, warnings := host.Sweep(later, time.Minute, 15*time.Minute, time.Minute)
	if len(warnings) != 2 {
		t.Fatalf("%d avertissements, attendu un par joueur silencieux", len(warnings))
	}
	if host.Session().Terminated {
		t.Fatal("prévenir n'est pas exclure : la session ne doit pas finir sur la question")
	}

	// Personne ne répond, mais les onglets BATTENT toujours — c'est bien le
	// cas qu'on veut : ils tournent, personne n'est derrière. (Cesser de
	// battre serait un onglet gelé, que la branche précédente attrape en
	// premier, et à juste titre.)
	deadline := later.Add(2 * time.Minute)
	host.Beat("sock-G", false, deadline)
	host.Beat("sock-D", false, deadline)

	host.Sweep(deadline, time.Minute, 15*time.Minute, time.Minute)
	if host.Session().Terminated {
		t.Fatal("an idle owner must not END the game — only a terminate does")
	}
	if len(host.Session().Players) != 0 || host.Session().IdleSince == nil {
		t.Fatalf("players = %+v, idleSince = %v — want nobody, and the idle clock running", host.Session().Players, host.Session().IdleSince)
	}
}

// LE PROPRIÉTAIRE PART, LES AUTRES CONTINUENT : sa manette lui reste réservée,
// et la partie n'est pas inactive tant que quelqu'un joue.
func TestAnIdleOwnerLeavesAndTheOthersPlayOn(t *testing.T) {
	host, _ := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-D", "dave")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)
	host.Submit(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D"}, now)

	later := now
	for tick := 0; tick < 40; tick++ {
		later = later.Add(30 * time.Second)
		host.Beat("sock-G", false, later) // le propriétaire ne touche plus à rien
		host.Beat("sock-D", true, later)  // dave joue
	}
	host.Sweep(later, time.Minute, 15*time.Minute, time.Minute)
	deadline := later.Add(2 * time.Minute)
	host.Beat("sock-G", false, deadline)
	host.Beat("sock-D", true, deadline)
	host.Sweep(deadline, time.Minute, 15*time.Minute, time.Minute)

	session := host.Session()
	if session.Terminated || session.IdleSince != nil {
		t.Fatalf("terminated = %v, idleSince = %v — want the game going on", session.Terminated, session.IdleSince)
	}
	if session.PlayerOf("gino") != nil || session.PlayerOf("dave") == nil {
		t.Fatalf("players = %+v, want dave alone", session.Players)
	}
	// Sa place lui reste : personne d'autre ne prend la manette 1.
	if session.PlayerOf("dave").SlotID == 1 || session.freeSlot("zoe") == 1 {
		t.Fatal("slot 1 must stay reserved for the owner")
	}
}

// LA RÉPONSE EFFACE LA QUESTION — n'importe quel geste suffit.
func TestAnAnswerCancelsTheAFKQuestion(t *testing.T) {
	host, _ := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)

	silent := now.Add(20 * time.Minute)
	host.Beat("sock-G", false, silent)

	_, warnings := host.Sweep(silent, time.Minute, 15*time.Minute, time.Minute)
	if len(warnings) != 1 || warnings[0].Username != "gino" {
		t.Fatalf("avertissements = %+v, attendu la question à gino", warnings)
	}

	// Il touche quelque chose pendant le compte à rebours.
	host.Beat("sock-G", true, silent.Add(10*time.Second))

	closing, again := host.Sweep(silent.Add(30*time.Second), time.Minute, 15*time.Minute, time.Minute)
	if len(closing) != 0 {
		t.Errorf("un joueur qui a répondu a été exclu : %v", closing)
	}
	if len(again) != 0 {
		t.Errorf("la question a été reposée alors qu'il venait de répondre : %+v", again)
	}

	// Et il n'est pas exclu non plus une fois la grâce passée : son entrée
	// est récente, la question ne tient plus. Son onglet bat toujours, comme
	// celui de quelqu'un qui joue.
	settled := silent.Add(2 * time.Minute)
	host.Beat("sock-G", false, settled)
	if closing, _ := host.Sweep(settled, time.Minute, 15*time.Minute, time.Minute); len(closing) != 0 {
		t.Errorf("exclu après avoir répondu : %v", closing)
	}
}

// Un joueur qui joue vraiment n'est jamais balayé, même après des heures.
func TestSweepLeavesAnActivePlayerAlone(t *testing.T) {
	host, _ := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)

	later := now
	for tick := 0; tick < 400; tick++ {
		later = later.Add(30 * time.Second)
		host.Beat("sock-G", true, later)
	}

	if closing, _ := host.Sweep(later, time.Minute, 15*time.Minute, time.Minute); len(closing) != 0 {
		t.Fatalf("Sweep = %v, want nobody after 3+ hours of real play", closing)
	}
	if host.Session().Terminated {
		t.Fatal("an active session must not be terminated")
	}
}

// Un spectateur n'est jamais balayé : il ne tient rien, il ne coûte rien.
func TestSweepNeverTouchesWatchers(t *testing.T) {
	host, _ := newHost(t, 4)
	host.Attach("sock-W", "dave")

	if closing, _ := host.Sweep(now.Add(time.Hour), time.Minute, 15*time.Minute, time.Minute); len(closing) != 0 {
		t.Fatalf("Sweep = %v, want watchers left alone", closing)
	}
}

// --- concurrence réelle ---------------------------------------------------

// Le Host n'a qu'une raison d'exister au-dessus de Session + Queue : garantir
// que les commandes s'appliquent VRAIMENT une par une. Un test mono-thread ne
// prouve rien de tout ça, et le détecteur de courses n'a rien à détecter.
//
// Ici N onglets postent en parallèle sur une session qui n'a qu'une place de
// joiner. Exactement un doit l'obtenir, les autres doivent être refusés — et
// aucun accès concurrent ne doit apparaître.
func TestOnlyOneWinsUnderConcurrency(t *testing.T) {
	const contenders = 32

	rec := newRecorder()
	var recorderLock sync.Mutex
	host := NewHost(New("s1", "retrogaming", "gino", 2, now), func(socketID string, message map[string]any) error {
		recorderLock.Lock()
		defer recorderLock.Unlock()

		return rec.send(socketID, message)
	})

	sockets := make([]string, contenders)
	for index := range sockets {
		sockets[index] = fmt.Sprintf("sock-%d", index)
		host.Attach(sockets[index], fmt.Sprintf("user-%d", index))
	}

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)

	for index, socketID := range sockets {
		done.Add(1)

		go func(index int, socketID string) {
			defer done.Done()
			start.Wait()
			host.Submit(Command{Kind: CmdJoin, Username: fmt.Sprintf("user-%d", index), SocketID: socketID}, now)
		}(index, socketID)
	}

	start.Done()
	done.Wait()

	if got := len(host.Session().Players); got != 1 {
		t.Fatalf("%d players hold a slot, want exactly 1 — the queue was not serialised", got)
	}

	recorderLock.Lock()
	defer recorderLock.Unlock()

	granted := 0
	for _, socketID := range sockets {
		if result := rec.last(socketID, "result"); result != nil && result["status"] == "granted" {
			granted++
		}
	}
	if granted != 1 {
		t.Fatalf("%d tabs were told they got the slot, want exactly 1", granted)
	}
}

// --- l'écriture ratée comme signal de mort --------------------------------

// Un socket peut mourir sans dire au revoir : le pair disparaît, aucune trame
// de fermeture n'arrive, et la comptabilité le croit vivant. La première
// écriture qui échoue le révèle — sans round-trip, sans attente, sans verrou
// tenu.
func TestFailedWriteReleasesTheSlot(t *testing.T) {
	host, rec := newHost(t, 4)
	host.Attach("sock-G", "gino")
	host.Attach("sock-W", "dave")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)

	if len(host.Session().Players) != 1 {
		t.Fatal("gino should be holding a slot")
	}

	// Le pair de gino s'évapore. Rien ne le signale : c'est la prochaine
	// écriture qui l'apprendra.
	rec.kill("sock-G")
	host.Submit(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-W"}, now)

	if len(host.Session().Players) != 1 {
		t.Fatalf("players = %+v, want only dave — the dead socket must lose its slot", host.Session().Players)
	}
	if host.Session().PlayerOf("gino") != nil {
		t.Fatal("a socket that cannot be written to must not keep holding a pad")
	}
}

// Le nettoyage en cascade doit s'arrêter : retirer un mort produit un nouvel
// état pour les survivants, qui peut en révéler d'autres.
func TestCascadingDeathsTerminate(t *testing.T) {
	host, rec := newHost(t, 4)
	for _, socketID := range []string{"sock-A", "sock-B", "sock-C"} {
		host.Attach(socketID, "user-"+socketID)
	}
	host.Submit(Command{Kind: CmdJoin, Username: "user-sock-A", SocketID: "sock-A"}, now)

	// Tout le monde meurt d'un coup.
	rec.kill("sock-A")
	rec.kill("sock-B")
	rec.kill("sock-C")

	host.Attach("sock-D", "dave") // déclenche un push d'état à tous

	if got := len(host.Session().Participants); got != 1 {
		t.Fatalf("participants = %d, want only the live one", got)
	}
	if len(host.Session().Players) != 0 {
		t.Fatalf("players = %+v, want empty", host.Session().Players)
	}
}

// --- the mirror's contract -------------------------------------------------
//
// Snapshot is the ONLY thing the kernel ever learns about a session it no
// longer owns. Everything the kernel then does — who may stop the session, who
// may dial as owner, whether a page should leave — is read out of these
// fields, and nothing else re-derives them. Three separate failures on
// 2026-08-25 came from a copy that never learned what the child knew, so the
// payload deserves tests of its own rather than being covered by accident.

// After a hand-over the snapshot still names the OWNER, on slot 1, and who
// borrows the pad: the owner lends it (2026-10-09). The kernel writes the
// owner straight onto the session record, and the stop endpoints answer from
// it — a guest passed the pad for a level must not come away owning the game.
func TestSnapshotKeepsTheOwnerAfterAHandover(t *testing.T) {
	host, _ := newHost(t, 4)

	host.Attach("gino-1", "gino")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "gino-1"}, now)
	host.Attach("dave-1", "dave")
	host.Submit(Command{Kind: CmdRequestHandover, Username: "dave", SocketID: "dave-1", Target: "gino"}, now)
	host.Submit(Command{Kind: CmdGrantHandover, Username: "gino", SocketID: "gino-1"}, now)

	snapshot := host.Snapshot()

	if snapshot.Owner != "gino" {
		t.Fatalf("snapshot owner = %q, want gino — lending the pad does not hand over the game", snapshot.Owner)
	}
	if len(snapshot.Players) != 1 {
		t.Fatalf("snapshot players = %+v, want exactly one", snapshot.Players)
	}
	if entry := snapshot.Players[0]; entry.Username != "gino" || entry.SlotID != 1 || entry.LentTo != "dave" {
		t.Fatalf("snapshot entry = %+v, want gino on slot 1, lent to dave", entry)
	}
}

// The end of a session travels the same way. The page redirects on it, and a
// page that never hears it sits on a dead screen until someone reloads into a
// session that no longer exists.
func TestSnapshotReportsTheEnd(t *testing.T) {
	host, _ := newHost(t, 4)

	host.Attach("gino-1", "gino")
	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "gino-1"}, now)

	if host.Snapshot().Terminated {
		t.Fatal("a live session reports itself terminated")
	}

	host.Submit(Command{Kind: CmdTerminate, Username: "gino", SocketID: "gino-1"}, now)

	if !host.Snapshot().Terminated {
		t.Fatal("the session ended and the snapshot does not say so")
	}
}

// AvailableSlots is what the Dashboard's join button reads. A seat held for an
// owner nobody can be is not held — the same rule freeSlot follows, asserted
// on the other side of the model because this is the number the UI shows.
func TestSnapshotOffersEverySlotWhenNobodyOwnsTheSession(t *testing.T) {
	for _, owner := range []string{"", "anonymous"} {
		host := NewHost(New("s1", "remoteplay", owner, 1, now), func(string, map[string]any) error { return nil })

		if got := host.Snapshot().AvailableSlots; got != 1 {
			t.Fatalf("owner=%q: availableSlots = %d, want 1 — a one-pad session nobody owns is joinable", owner, got)
		}
	}
}

// UNE MANETTE QUI CHANGE DE MAINS EST REMISE AU NEUTRE — et seulement elle.
// Prêt, reprise, retour, place donnée, départ : chaque façon dont un pad
// change de mains passe par la file, donc par ce seul crochet.
func TestAPadIsReleasedEachTimeItChangesHands(t *testing.T) {
	host, _ := newHost(t, 4)
	released := []int{}
	host.OnPadHandover = func(slot int) { released = append(released, slot) }
	expect := func(when string, want ...int) {
		t.Helper()
		if fmt.Sprint(released) != fmt.Sprint(want) {
			t.Fatalf("%s: released pads %v, want %v", when, released, want)
		}
		released = released[:0]
	}

	host.Attach("sock-G", "gino")
	host.Attach("sock-D", "dave")
	host.Attach("sock-X", "gixi")
	expect("tabs opening")

	host.Submit(Command{Kind: CmdJoin, Username: "gino", SocketID: "sock-G"}, now)
	host.Submit(Command{Kind: CmdJoin, Username: "dave", SocketID: "sock-D"}, now)
	expect("two joins", 1, 2)

	host.Submit(Command{Kind: CmdGiveHandover, Username: "gino", SocketID: "sock-G", Target: "gixi"}, now)
	expect("the owner lends", 1)
	host.Beat("sock-X", true, now)
	expect("a beat changes no hands")
	host.Submit(Command{Kind: CmdReclaim, Username: "gino", SocketID: "sock-G"}, now)
	expect("the owner takes it back", 1)

	host.Submit(Command{Kind: CmdGiveHandover, Username: "dave", SocketID: "sock-D", Target: "gixi"}, now)
	expect("a guest gives their place", 2)

	host.Detach("sock-X", now)
	expect("the new holder leaves", 2)
}
