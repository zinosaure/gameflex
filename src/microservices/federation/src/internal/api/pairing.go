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

	"github.com/go-chi/chi/v5"

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
	id      int64
	label   string
	scopes  []string
	expires time.Time
}

// pairingEnded — ce qu'un code est devenu (`used`, `cancelled`, `expired`, `too_many_failures`), et, servi, par qui
// (le nom que l'appareil donne) et d'où : la fenêtre du code le dit en se fermant (PairingReveal d'edge).
type pairingEnded struct {
	ID     int64     `json:"id"`
	Reason string    `json:"reason"`
	By     string    `json:"by,omitempty"`
	From   string    `json:"from,omitempty"`
	At     time.Time `json:"at"`
}

// pairingClosure — pourquoi le mode s'est fermé : `owner`, `timeout`, `too_many_failures`.
type pairingClosure struct {
	Scope  string    `json:"scope"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// Ce que le sujet garde des fins : les dernières, assez pour qu'une fenêtre ouverte lise la sienne.
const pairingKeptEnds = 16

// pairing — le mode découverte et ses codes.
type pairing struct {
	mu        sync.Mutex
	until     time.Time
	link      string
	timer     *time.Timer
	codes     map[string]pairingCodeEntry
	nextID    int64
	ended     []pairingEnded
	closed    []pairingClosure
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
	p.timer = time.AfterFunc(discoveryLifetime, func() { p.closeFor("timeout") })
	p.announceLocked()
	p.wakeLocked()

	return p.until
}

// closeFor — ferme le mode, et dit pourquoi : plus de code accepté, plus d'annonce.
func (p *pairing) closeFor(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked(reason)
}

func (p *pairing) endLocked(entry pairingCodeEntry, reason, by, from string) {
	p.ended = append(p.ended, pairingEnded{ID: entry.id, Reason: reason, By: by, From: from, At: time.Now()})
	if len(p.ended) > pairingKeptEnds {
		p.ended = p.ended[len(p.ended)-pairingKeptEnds:]
	}
}

func (p *pairing) closeLocked(reason string) {
	if !p.until.IsZero() {
		p.closed = append(p.closed, pairingClosure{Scope: "server", Reason: reason, At: time.Now()})
		if len(p.closed) > pairingKeptEnds {
			p.closed = p.closed[len(p.closed)-pairingKeptEnds:]
		}
	}
	ending := "expired"
	if reason == "too_many_failures" {
		ending = reason
	}
	for _, entry := range p.codes {
		p.endLocked(entry, ending, "", "")
	}
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

// newCode — un code neuf pour `label` et ses portées, mode ouvert seulement : son numéro, sa valeur, son échéance.
func (p *pairing) newCode(label string, scopes []string) (int64, string, time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.openLocked() {
		return 0, "", time.Time{}, errPairingClosed
	}
	for {
		n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return 0, "", time.Time{}, err
		}
		code := fmt.Sprintf("%06d", n.Int64())
		if _, taken := p.codes[code]; taken {
			continue
		}
		p.nextID++
		id, expires := p.nextID, time.Now().Add(pairingLifetime)
		p.codes[code] = pairingCodeEntry{id: id, label: label, scopes: store.Scopes(scopes), expires: expires}
		time.AfterFunc(pairingLifetime, func() {
			p.mu.Lock()
			if entry, ok := p.codes[code]; ok && entry.id == id && !time.Now().Before(entry.expires) {
				delete(p.codes, code)
				p.endLocked(entry, "expired", "", "")
				p.wakeLocked()
			}
			p.mu.Unlock()
		})
		p.wakeLocked()

		return id, code, expires, nil
	}
}

// cancel — l'administrateur annule un code : faux s'il n'existe plus.
func (p *pairing) cancel(id int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for code, entry := range p.codes {
		if entry.id == id {
			delete(p.codes, code)
			p.endLocked(entry, "cancelled", "", "")
			p.wakeLocked()

			return true
		}
	}

	return false
}

// redeem — un code contre un jeton ; au cinquième code faux, le mode se ferme. `by`, `from` : qui l'a tapé, et d'où.
func (p *pairing) redeem(st *store.Store, code, by, from string) (store.Token, string, error) {
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
			p.closeLocked("too_many_failures")
		}
		p.mu.Unlock()

		return store.Token{}, "", errPairingRefused
	}
	delete(p.codes, code)
	p.endLocked(entry, "used", by, from)
	p.wakeLocked()
	p.mu.Unlock()

	return st.IssueToken(entry.label, entry.scopes)
}

// state — ce que l'administration voit : le mode, et l'échéance de chaque code (jamais leur valeur).
func (p *pairing) state() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	// LA FORME DU SUJET `pairing` DE GAMEFLEX (chantier 61, B2) : les pages de Réglages › Fédération d'edge la lisent
	// telle quelle — le mode du serveur et son échéance, aucun mode de compte ici, les fermetures, les codes vivants
	// (jamais leur valeur), ce que les derniers sont devenus.
	codes := []map[string]any{}
	for _, entry := range p.codes {
		if time.Now().Before(entry.expires) {
			codes = append(codes, map[string]any{"id": entry.id, "label": entry.label, "expires_at": entry.expires})
		}
	}
	state := map[string]any{"server": nil, "account": nil, "closed": p.closed, "codes": codes, "ended": p.ended,
		"announced": p.announcer != nil && p.openLocked()}
	if p.closed == nil {
		state["closed"] = []pairingClosure{}
	}
	if p.ended == nil {
		state["ended"] = []pairingEnded{}
	}
	if p.openLocked() {
		state["server"] = map[string]any{"open_until": p.until}
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
		s.pairing.closeFor("owner")
	}
	writeJSON(w, http.StatusOK, s.pairing.state())
}

func (s *Server) pairingCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label  string   `json:"label"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Label == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "a code needs a name (the device that will type it)")

		return
	}
	id, code, expires, err := s.pairing.newCode(body.Label, body.Scopes)
	if errors.Is(err, errPairingClosed) {
		writeError(w, http.StatusLocked, "pairing_closed", "open the discovery mode first")

		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pairing_error", err.Error())

		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "code": code, "label": body.Label, "expires_at": expires})
}

// pairingCancel — DELETE /api/v1/admin/pairing/code/{id} : le code ne vaut plus.
func (s *Server) pairingCancel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || !s.pairing.cancel(id) {
		writeError(w, http.StatusNotFound, "not_found", "no such code")

		return
	}
	w.WriteHeader(http.StatusNoContent)
}
