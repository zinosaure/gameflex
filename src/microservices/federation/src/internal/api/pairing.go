package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"gameflex/federation/internal/store"
	"gameflex/lan"
)

// LE CODE D'APPAIRAGE ET LE MODE DÉCOUVERTE (chantier 61, d'après le chantier 60 de GameFlex ; le porteur, 2026-11-18 :
// « on aura aussi le mode découverte pour se connecter avec le code appairage »).
//
// Les règles de GameFlex, telles quelles :
//
//   - le MODE DÉCOUVERTE s'ouvre par l'administrateur pour cinq minutes, et se prolonge ; pendant qu'il est ouvert, le
//     service s'annonce sur le réseau local (DNS-SD sur mDNS, gameflex/lan) — à condition d'être en réseau hôte : le
//     multicast ne traverse pas un bridge Docker ; sinon l'appareil tape l'adresse, le repli prévu ;
//   - un CODE de six chiffres vaut deux minutes, une fois, et ne s'accepte que mode ouvert (sinon 423) ;
//   - dix puissance six codes, c'est peu pour un secret : au cinquième code faux, toutes adresses confondues, le mode
//     se ferme ;
//   - tout ne vit qu'en mémoire : un redémarrage ferme le mode et oublie les codes. Les minuteries sont des événements
//     bornés, rien ne se relit à intervalle.
//
// Les jetons, eux, ne demandent aucun mode découverte (le porteur, chantier 60).

const (
	discoveryLifetime = 5 * time.Minute
	pairingLifetime   = 2 * time.Minute
	pairingMaxFails   = 5
)

var (
	errPairingClosed  = errors.New("pairing closed")
	errPairingRefused = errors.New("pairing refused")
	pairingCodeForm   = regexp.MustCompile(`^[0-9]{6}$`)
)

func isPairingCode(code string) bool { return pairingCodeForm.MatchString(code) }

type pairingCodeEntry struct {
	label   string
	expires time.Time
}

// pairing — le mode découverte et ses codes.
type pairing struct {
	mu        sync.Mutex
	until     time.Time
	link      string
	timer     *time.Timer
	codes     map[string]pairingCodeEntry
	fails     int
	wake      chan struct{}
	announcer *lan.Announcer
	name      func() string
}

func newPairing(name func() string) *pairing {
	p := &pairing{codes: map[string]pairingCodeEntry{}, wake: make(chan struct{}), name: name}
	if announcer, err := lan.NewAnnouncer(); err == nil {
		p.announcer = announcer
	} else {
		slog.Info("pas d'annonce sur le réseau local (réseau hôte requis) : les appareils taperont l'adresse", "error", err)
	}

	return p
}

func (p *pairing) changed() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.wake
}

func (p *pairing) wakeLocked() {
	close(p.wake)
	p.wake = make(chan struct{})
}

func (p *pairing) openLocked() bool { return time.Now().Before(p.until) }

// open — ouvre ou prolonge le mode pour cinq minutes ; `link` est l'adresse que l'administrateur voit (annoncée).
func (p *pairing) open(link string) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.openLocked() {
		p.fails = 0
	}
	p.until = time.Now().Add(discoveryLifetime)
	if link != "" {
		p.link = link
	}
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(discoveryLifetime, func() { p.close() })
	p.announceLocked()
	p.wakeLocked()

	return p.until
}

// close — ferme le mode : plus de code accepté, plus d'annonce.
func (p *pairing) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

func (p *pairing) closeLocked() {
	p.until = time.Time{}
	p.codes = map[string]pairingCodeEntry{}
	p.fails = 0
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	if p.announcer != nil {
		p.announcer.Withdraw()
	}
	p.wakeLocked()
}

func (p *pairing) announceLocked() {
	if p.announcer == nil || p.link == "" {
		return
	}
	parsed, err := url.Parse(p.link)
	if err != nil {
		return
	}
	port, _ := strconv.Atoi(parsed.Port())
	if port == 0 {
		port = 80
		if parsed.Scheme == "https" {
			port = 443
		}
	}
	err = p.announcer.Announce(lan.Announcement{
		Instance: fmt.Sprintf("%s %d", p.name(), port),
		Name:     p.name(),
		Link:     p.link,
		Version:  os.Getenv("GAMEFLEX_VERSION"),
		Address:  net.ParseIP(parsed.Hostname()),
		Port:     port,
		TTL:      time.Until(p.until),
	})
	if err != nil {
		slog.Warn("annonce sur le réseau local impossible", "error", err)
	}
}

// newCode — un code neuf pour `label`, mode ouvert seulement.
func (p *pairing) newCode(label string) (string, time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.openLocked() {
		return "", time.Time{}, errPairingClosed
	}
	for {
		n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return "", time.Time{}, err
		}
		code := fmt.Sprintf("%06d", n.Int64())
		if _, taken := p.codes[code]; taken {
			continue
		}
		expires := time.Now().Add(pairingLifetime)
		p.codes[code] = pairingCodeEntry{label: label, expires: expires}
		time.AfterFunc(pairingLifetime, func() {
			p.mu.Lock()
			if entry, ok := p.codes[code]; ok && !time.Now().Before(entry.expires) {
				delete(p.codes, code)
				p.wakeLocked()
			}
			p.mu.Unlock()
		})
		p.wakeLocked()

		return code, expires, nil
	}
}

// redeem — un code contre un jeton ; au cinquième code faux, le mode se ferme.
func (p *pairing) redeem(st *store.Store, code string) (store.Token, string, error) {
	p.mu.Lock()
	if !p.openLocked() {
		p.mu.Unlock()

		return store.Token{}, "", errPairingClosed
	}
	entry, ok := p.codes[code]
	if !ok || !time.Now().Before(entry.expires) {
		p.fails++
		if p.fails >= pairingMaxFails {
			slog.Warn("cinq codes d'appairage faux : le mode découverte se ferme")
			p.closeLocked()
		}
		p.mu.Unlock()

		return store.Token{}, "", errPairingRefused
	}
	delete(p.codes, code)
	p.wakeLocked()
	p.mu.Unlock()

	return st.IssueToken(entry.label)
}

// state — ce que l'administration voit : le mode, et l'échéance de chaque code (jamais leur valeur).
func (p *pairing) state() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	codes := []time.Time{}
	for _, entry := range p.codes {
		if time.Now().Before(entry.expires) {
			codes = append(codes, entry.expires)
		}
	}
	state := map[string]any{"open": p.openLocked(), "codes": codes, "announced": p.announcer != nil && p.openLocked()}
	if p.openLocked() {
		state["until"] = p.until
	}

	return state
}

// ── Les routes de l'administration ──────────────────────────────────────────

func (s *Server) pairingState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.pairing.state())
}

func (s *Server) pairingDiscovery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Open bool   `json:"open"`
		Link string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	if body.Open {
		s.pairing.open(body.Link)
	} else {
		s.pairing.close()
	}
	writeJSON(w, http.StatusOK, s.pairing.state())
}

func (s *Server) pairingCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Label == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "a code needs a name (the device that will type it)")

		return
	}
	code, expires, err := s.pairing.newCode(body.Label)
	if errors.Is(err, errPairingClosed) {
		writeError(w, http.StatusLocked, "pairing_closed", "open the discovery mode first")

		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pairing_error", err.Error())

		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"code": code, "expiresAt": expires})
}
