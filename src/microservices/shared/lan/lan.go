// Package lan — LA DÉCOUVERTE DES SERVEURS GAMEFLEX EN RÉSEAU LOCAL (chantier
// 60) : DNS-SD sur mDNS (RFC 6762 et 6763), le service `_gameflex._tcp.local.`.
//
// Deux pièces, et rien d'autre :
//
//   - l'ANNONCE (Announcer) : un serveur en mode découverte se signale, trois
//     fois à une seconde d'écart (RFC 6762 § 8.3), avec un TTL égal au temps
//     qui reste à son mode ; il répond aux questions tant qu'il est annoncé, et
//     dit « au revoir » (TTL 0) quand son mode se ferme ;
//   - la RECHERCHE (Browse) : UNE question PTR à l'ouverture, puis l'écoute.
//     Un serveur trouvé reste dans la liste jusqu'à son « au revoir » ou la fin
//     de son TTL ; Ask repose la question une fois, pour le geste « Chercher de
//     nouveau ».
//
// POURQUOI CE PAQUET, et pas une bibliothèque zeroconf : celles qu'on trouve
// reposent leurs questions à intervalle tant qu'on cherche, ce qui est un
// sondage (CODE_CONVENTIONS.md, « Pas de sondage »). Celui-ci ne pose une
// question que quand on le lui demande, et ne fait qu'écouter le reste du temps.
//
// La passerelle WebRTC du serveur l'emploie, parce qu'elle est en réseau hôte
// (le kernel est sur un bridge que le multicast ne traverse pas), et le kernel
// du téléphone, qui est sur le vrai réseau. Le socket est ouvert en
// SO_REUSEADDR et SO_REUSEPORT et répond en multicast : il cohabite avec un
// avahi qui tient déjà le port 5353. Les interfaces se listent par anet : sur
// Android 11 et après, net.Interfaces échoue (#753).
package lan

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// ServiceName — le type de service DNS-SD de GameFlex.
const ServiceName = "_gameflex._tcp.local."

// announceRepeats, announceInterval — l'annonce part trois fois, à une seconde
// d'écart : une page déjà ouverte la voit arriver sans rien redemander, même si
// un paquet se perd sur le Wi-Fi.
var (
	announceRepeats  = 3
	announceInterval = time.Second
)

// Announcement — ce qu'un serveur annonce.
type Announcement struct {
	// Instance — le nom de l'instance : le nom du serveur et son port, pour que
	// deux piles du même hôte ne se confondent pas.
	Instance string
	// Name, Link, Version — le TXT : `name`, `link`, `v`.
	Name    string
	Link    string
	Version string
	// Address, Port — l'enregistrement SRV et son A.
	Address net.IP
	Port    int
	// TTL — le temps qui reste au mode découverte.
	TTL time.Duration
}

// Found — un serveur trouvé.
type Found struct {
	Instance string `json:"instance"`
	Name     string `json:"name"`
	Link     string `json:"link"`
	// Address — d'où sa réponse est venue : la liste la montre, et un nom vu à
	// deux adresses se signale.
	Address string `json:"address"`
}

// transport — un socket mDNS : envoyer au groupe, recevoir du groupe. Les
// tests en branchent un en mémoire.
type transport interface {
	Send(payload []byte) error
	Receive() (payload []byte, from net.IP, err error)
	Close() error
}

// instanceName — le nom complet d'une instance.
func instanceName(instance string) string {
	return escapeLabel(instance) + "." + ServiceName
}

// escapeLabel — un nom d'instance peut porter des points et des espaces ; le
// point devient un tiret pour rester une seule étiquette.
func escapeLabel(label string) string {
	return strings.ReplaceAll(strings.TrimSpace(label), ".", "-")
}

// hostName — la cible du SRV : un nom à part, pour ne pas toucher à celui que
// l'hôte annonce déjà (avahi).
func hostName(instance string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 'a' - 'A'
		}

		return '-'
	}, instance)

	return "gameflex-" + strings.Trim(cleaned, "-") + ".local."
}

// records — les quatre enregistrements d'une annonce, au TTL donné (0 pour
// l'« au revoir »).
func records(announcement Announcement, ttl uint32) ([]byte, error) {
	service := dnsmessage.MustNewName(ServiceName)
	instance, err := dnsmessage.NewName(instanceName(announcement.Instance))
	if err != nil {
		return nil, err
	}
	host, err := dnsmessage.NewName(hostName(announcement.Instance))
	if err != nil {
		return nil, err
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true})
	builder.EnableCompression()
	if err := builder.StartAnswers(); err != nil {
		return nil, err
	}
	header := func(name dnsmessage.Name, cacheFlush bool) dnsmessage.ResourceHeader {
		class := dnsmessage.ClassINET
		if cacheFlush {
			class |= 1 << 15
		}

		return dnsmessage.ResourceHeader{Name: name, Class: class, TTL: ttl}
	}
	if err := builder.PTRResource(header(service, false), dnsmessage.PTRResource{PTR: instance}); err != nil {
		return nil, err
	}
	if err := builder.SRVResource(header(instance, true), dnsmessage.SRVResource{Port: uint16(announcement.Port), Target: host}); err != nil {
		return nil, err
	}
	txt := []string{"link=" + announcement.Link, "name=" + announcement.Name, "v=" + announcement.Version}
	if err := builder.TXTResource(header(instance, true), dnsmessage.TXTResource{TXT: txt}); err != nil {
		return nil, err
	}
	if address := announcement.Address.To4(); address != nil {
		var a [4]byte
		copy(a[:], address)
		if err := builder.AResource(header(host, true), dnsmessage.AResource{A: a}); err != nil {
			return nil, err
		}
	}

	return builder.Finish()
}

// question — la question PTR du service, en multicast (QM).
func question() ([]byte, error) {
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	if err := builder.StartQuestions(); err != nil {
		return nil, err
	}
	if err := builder.Question(dnsmessage.Question{
		Name: dnsmessage.MustNewName(ServiceName), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET,
	}); err != nil {
		return nil, err
	}

	return builder.Finish()
}

// asksForUs — ce paquet est-il une question sur notre service ou notre
// instance ?
func asksForUs(packet []byte, instance string) bool {
	var parser dnsmessage.Parser
	header, err := parser.Start(packet)
	if err != nil || header.Response {
		return false
	}
	ours := strings.ToLower(instanceName(instance))
	for {
		asked, err := parser.Question()
		if err != nil {
			return false
		}
		name := strings.ToLower(asked.Name.String())
		if (name == strings.ToLower(ServiceName) && (asked.Type == dnsmessage.TypePTR || asked.Type == dnsmessage.TypeALL)) || name == ours {
			return true
		}
	}
}

// Announcer — l'annonce d'un serveur, et ses réponses aux questions.
type Announcer struct {
	transport transport
	mu        sync.Mutex
	current   *Announcement
	until     time.Time
	repeats   []*time.Timer
}

// NewAnnouncer — un socket mDNS, qui ne dit rien tant qu'on ne lui demande
// pas d'annoncer.
func NewAnnouncer() (*Announcer, error) {
	socket, err := openMulticast()
	if err != nil {
		return nil, err
	}

	return newAnnouncer(socket), nil
}

func newAnnouncer(socket transport) *Announcer {
	announcer := &Announcer{transport: socket}
	go announcer.answer()

	return announcer
}

// remainingTTL — le temps qui reste à l'annonce, en secondes ; zéro sinon.
func (announcer *Announcer) remainingLocked() uint32 {
	if announcer.current == nil {
		return 0
	}
	left := time.Until(announcer.until).Round(time.Second)
	if left <= 0 {
		return 0
	}

	return uint32(left / time.Second)
}

// sendLocked — les enregistrements, au TTL qui reste.
func (announcer *Announcer) sendLocked() {
	ttl := announcer.remainingLocked()
	if ttl == 0 {
		return
	}
	if packet, err := records(*announcer.current, ttl); err == nil {
		_ = announcer.transport.Send(packet)
	}
}

func (announcer *Announcer) stopRepeatsLocked() {
	for _, timer := range announcer.repeats {
		timer.Stop()
	}
	announcer.repeats = nil
}

// Announce — annoncer, ou annoncer de nouveau avec un nouveau TTL (une
// prolongation) : trois fois, à une seconde d'écart. Une autre instance
// annoncée avant reçoit son « au revoir ».
func (announcer *Announcer) Announce(announcement Announcement) error {
	if announcement.Instance == "" || announcement.Port == 0 || announcement.TTL < time.Second {
		return errors.New("lan: an announcement needs an instance, a port and a TTL")
	}
	announcer.mu.Lock()
	defer announcer.mu.Unlock()

	if announcer.current != nil && announcer.current.Instance != announcement.Instance {
		announcer.goodbyeLocked()
	}
	announcer.stopRepeatsLocked()
	announcer.current = &announcement
	announcer.until = time.Now().Add(announcement.TTL)
	announcer.sendLocked()
	for repeat := 1; repeat < announceRepeats; repeat++ {
		announcer.repeats = append(announcer.repeats, time.AfterFunc(time.Duration(repeat)*announceInterval, func() {
			announcer.mu.Lock()
			defer announcer.mu.Unlock()
			if announcer.current != nil {
				announcer.sendLocked()
			}
		}))
	}

	return nil
}

func (announcer *Announcer) goodbyeLocked() {
	if packet, err := records(*announcer.current, 0); err == nil {
		_ = announcer.transport.Send(packet)
	}
}

// Withdraw — l'« au revoir » (TTL 0), une fois ; rien s'il n'y avait rien.
func (announcer *Announcer) Withdraw() {
	announcer.mu.Lock()
	defer announcer.mu.Unlock()

	announcer.stopRepeatsLocked()
	if announcer.current == nil {
		return
	}
	if announcer.remainingLocked() > 0 {
		announcer.goodbyeLocked()
	}
	announcer.current = nil
}

// Current — l'annonce en cours, ou nil.
func (announcer *Announcer) Current() *Announcement {
	announcer.mu.Lock()
	defer announcer.mu.Unlock()

	if announcer.current == nil || announcer.remainingLocked() == 0 {
		return nil
	}
	copied := *announcer.current

	return &copied
}

// answer — répondre aux questions sur notre service tant qu'il est annoncé.
// Le socket vit autant que la passerelle.
func (announcer *Announcer) answer() {
	for {
		packet, _, err := announcer.transport.Receive()
		if err != nil {
			return
		}
		announcer.mu.Lock()
		if announcer.current != nil && asksForUs(packet, announcer.current.Instance) {
			announcer.sendLocked()
		}
		announcer.mu.Unlock()
	}
}
