package lan

// LA DÉCOUVERTE SUR UN GROUPE EN MÉMOIRE : chaque paquet envoyé arrive à tous
// les autres membres, comme le multicast, et le groupe compte les questions.

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

type bus struct {
	mu        sync.Mutex
	members   []*busSocket
	questions int
}

type busSocket struct {
	bus     *bus
	address net.IP
	inbox   chan []byte
	closed  chan struct{}
	once    sync.Once
}

func (group *bus) join(address string) *busSocket {
	group.mu.Lock()
	defer group.mu.Unlock()
	socket := &busSocket{bus: group, address: net.ParseIP(address), inbox: make(chan []byte, 64), closed: make(chan struct{})}
	group.members = append(group.members, socket)

	return socket
}

func (socket *busSocket) Send(payload []byte) error {
	var parser dnsmessage.Parser
	if header, err := parser.Start(payload); err == nil && !header.Response {
		socket.bus.mu.Lock()
		socket.bus.questions++
		socket.bus.mu.Unlock()
	}
	socket.bus.mu.Lock()
	defer socket.bus.mu.Unlock()
	for _, member := range socket.bus.members {
		if member == socket {
			continue
		}
		select {
		case member.inbox <- append(append([]byte(nil), payload...), socket.address...):
		case <-member.closed:
		}
	}

	return nil
}

func (socket *busSocket) Receive() ([]byte, net.IP, error) {
	select {
	case packet := <-socket.inbox:
		return packet[:len(packet)-net.IPv6len], net.IP(packet[len(packet)-net.IPv6len:]), nil
	case <-socket.closed:
		return nil, nil, errors.New("closed")
	}
}

func (socket *busSocket) Close() error {
	socket.once.Do(func() { close(socket.closed) })

	return nil
}

func (group *bus) questionCount() int {
	group.mu.Lock()
	defer group.mu.Unlock()

	return group.questions
}

// watcher — les listes successives qu'une recherche rend.
type watcher struct {
	lists chan []Found
}

func newWatcher() *watcher { return &watcher{lists: make(chan []Found, 32)} }

func (watcher *watcher) onChange(list []Found) { watcher.lists <- list }

func (watcher *watcher) next(t *testing.T) []Found {
	t.Helper()
	select {
	case list := <-watcher.lists:
		return list
	case <-time.After(5 * time.Second):
		t.Fatal("aucune liste")

		return nil
	}
}

func (watcher *watcher) silent(t *testing.T, wait time.Duration) {
	t.Helper()
	select {
	case list := <-watcher.lists:
		t.Fatalf("une liste inattendue : %+v", list)
	case <-time.After(wait):
	}
}

func gamebox(ttl time.Duration) Announcement {
	return Announcement{
		Instance: "Gamebox (11200)", Name: "Gamebox", Link: "http://192.168.1.10:11200", Version: "0.11.0",
		Address: net.ParseIP("192.168.1.10"), Port: 11200, TTL: ttl,
	}
}

func init() {
	announceInterval = 50 * time.Millisecond
}

// UNE ANNONCE VUE, PUIS SON « AU REVOIR » QUI LA RETIRE ; la recherche ne pose
// qu'une question, et Ask une de plus.
func TestAnAnnouncementIsSeenThenWithdrawn(t *testing.T) {
	group := &bus{}
	announcer := newAnnouncer(group.join("192.168.1.10"))
	defer announcer.transport.Close()
	seen := newWatcher()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	browser, err := browseOn(ctx, group.join("192.168.1.30"), seen.onChange)
	if err != nil {
		t.Fatal(err)
	}

	if err := announcer.Announce(gamebox(5 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	list := seen.next(t)
	if len(list) != 1 || list[0].Name != "Gamebox" || list[0].Link != "http://192.168.1.10:11200" || list[0].Address != "192.168.1.10" ||
		list[0].Instance != "Gamebox (11200)" {
		t.Fatalf("trouvé : %+v", list)
	}
	// Les deux répétitions de l'annonce ne changent pas la liste.
	seen.silent(t, 200*time.Millisecond)

	announcer.Withdraw()
	if list := seen.next(t); len(list) != 0 {
		t.Fatalf("après l'au revoir : %+v", list)
	}
	if got := group.questionCount(); got != 1 {
		t.Fatalf("%d questions, attendu 1", got)
	}
	_ = browser.Ask()
	time.Sleep(50 * time.Millisecond)
	if got := group.questionCount(); got != 2 {
		t.Fatalf("%d questions après Ask, attendu 2", got)
	}
}

// LA QUESTION UNIQUE TROUVE UN SERVEUR DÉJÀ ANNONCÉ : il y répond.
func TestTheQuestionFindsAServerAlreadyAnnounced(t *testing.T) {
	group := &bus{}
	announcer := newAnnouncer(group.join("192.168.1.10"))
	defer announcer.transport.Close()
	if err := announcer.Announce(gamebox(5 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	seen := newWatcher()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := browseOn(ctx, group.join("192.168.1.30"), seen.onChange); err != nil {
		t.Fatal(err)
	}
	if list := seen.next(t); len(list) != 1 || list[0].Name != "Gamebox" {
		t.Fatalf("trouvé : %+v", list)
	}
	seen.silent(t, 200*time.Millisecond)
	if got := group.questionCount(); got != 1 {
		t.Fatalf("%d questions, attendu 1", got)
	}
}

// LE TTL FAIT TOMBER UN SERVEUR QUI NE S'EST PAS DIT AU REVOIR ; UNE
// PROLONGATION, ANNONCÉE AVEC SON NOUVEAU TTL, LE GARDE.
func TestTheTTLDropsAServerAndARenewalKeepsIt(t *testing.T) {
	group := &bus{}
	announcer := newAnnouncer(group.join("192.168.1.10"))
	defer announcer.transport.Close()
	seen := newWatcher()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := browseOn(ctx, group.join("192.168.1.30"), seen.onChange); err != nil {
		t.Fatal(err)
	}

	_ = announcer.Announce(gamebox(2 * time.Second))
	if list := seen.next(t); len(list) != 1 {
		t.Fatalf("trouvé : %+v", list)
	}
	time.Sleep(1200 * time.Millisecond)
	_ = announcer.Announce(gamebox(3 * time.Second))
	seen.silent(t, 1500*time.Millisecond)

	// Plus rien ne part : le TTL de la dernière annonce le fait tomber.
	announcer.mu.Lock()
	announcer.current = nil
	announcer.mu.Unlock()
	if list := seen.next(t); len(list) != 0 {
		t.Fatalf("à la fin du TTL : %+v", list)
	}
}

// UN AUTRE SERVICE, OU UNE QUESTION SUR UN AUTRE NOM, NE DÉCLENCHE RIEN.
func TestOtherTrafficIsIgnored(t *testing.T) {
	if asksForUs([]byte("pas du DNS"), "Gamebox (11200)") {
		t.Fatal("un paquet illisible pris pour une question")
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	_ = builder.StartQuestions()
	_ = builder.Question(dnsmessage.Question{Name: dnsmessage.MustNewName("_googlecast._tcp.local."), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET})
	packet, _ := builder.Finish()
	if asksForUs(packet, "Gamebox (11200)") {
		t.Fatal("une question sur un autre service")
	}
	ours, _ := question()
	if !asksForUs(ours, "Gamebox (11200)") {
		t.Fatal("notre question n'est pas reconnue")
	}
}
