package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gameflex/federation/internal/store"
)

// LES INVITATIONS ET LE LIEN AVEC UN APPAREIL (chantier 61) — les chemins d'un serveur GameFlex, au même contrat : un
// lien `<service>/invite/<code>` (ou son QR) que l'appareil échange ici contre son jeton, `POST /redeem`
// (shared/storage/remote, Redeem). Seule la sorte « federation » existe : le service n'a pas de comptes.

// redeemThrottle — trop d'échanges refusés depuis une adresse la font attendre (429), comme chez GameFlex.
type redeemThrottle struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

const (
	throttleWindow = 10 * time.Minute
	throttleLimit  = 10
)

func (t *redeemThrottle) blocked(address string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	recent := t.recentLocked(address)

	return len(recent) >= throttleLimit
}

func (t *redeemThrottle) fail(address string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failures == nil {
		t.failures = map[string][]time.Time{}
	}
	t.failures[address] = append(t.recentLocked(address), time.Now())
}

func (t *redeemThrottle) recentLocked(address string) []time.Time {
	kept := []time.Time{}
	for _, at := range t.failures[address] {
		if time.Since(at) < throttleWindow {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(t.failures, address)
	} else {
		t.failures[address] = kept
	}

	return kept
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

// redeem — POST /api/v1/federation/redeem : une invitation contre un jeton.
func (s *Server) redeem(w http.ResponseWriter, r *http.Request) {
	address := clientAddress(r)
	if s.throttle.blocked(address) {
		writeError(w, http.StatusTooManyRequests, "throttled", "too many refused invitations, try again later")

		return
	}
	var body struct {
		Code string `json:"code"`
		Kind string `json:"kind"`
		From string `json:"from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	if body.Kind != "" && body.Kind != "federation" {
		// L'autre sorte (un compte) : rien n'est consommé, elle se colle ailleurs.
		writeError(w, http.StatusConflict, "invitation_kind", "this server only links devices")

		return
	}
	token, value, err := s.redeemCode(strings.TrimSpace(body.Code))
	if errors.Is(err, store.ErrInvitationRefused) || errors.Is(err, errPairingRefused) {
		s.throttle.fail(address)
		writeError(w, http.StatusUnauthorized, "invitation_refused", "invitation already used, expired or unknown")

		return
	}
	if errors.Is(err, errPairingClosed) {
		writeError(w, http.StatusLocked, "pairing_closed", "this server is not in discovery mode")

		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kind": "federation", "token": value, "identity": token.Label, "scopes": token.Scopes, "hostname": s.Name(),
	})
}

// redeemCode — un code de six chiffres est un code d'appairage (pairing.go), tout autre une invitation.
func (s *Server) redeemCode(code string) (store.Token, string, error) {
	if isPairingCode(code) {
		return s.pairing.redeem(s.Store, code)
	}

	return s.Store.RedeemInvitation(code)
}

func (s *Server) listInvitations(w http.ResponseWriter, _ *http.Request) {
	list, err := s.Store.Invitations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createInvitation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	invitation, code, err := s.Store.CreateInvitation(body.Label)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	// Le lien se compose dans la page, à l'adresse par laquelle l'administrateur voit le service : c'est celle que
	// l'appareil devra joindre.
	writeJSON(w, http.StatusCreated, map[string]any{"invitation": invitation, "code": code})
}

func (s *Server) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")

		return
	}
	if err := s.Store.RevokeInvitation(id); errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "not_found", "no such invitation")

		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": id})
}

// events — GET /api/v1/admin/events : un événement « changed » à chaque changement du catalogue (un parcours commencé
// ou fini, une ressource ajoutée ou retirée) et du mode découverte. Jamais de sondage : la page relit quand on la
// réveille. EventSource n'envoie pas d'en-tête : la session voyage dans l'adresse (`?session=`).
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "no_stream", "streaming unsupported")

		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()
	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()
	for {
		catalogChanged := s.Catalog.Changed()
		pairingChanged := s.pairing.changed()
		select {
		case <-r.Context().Done():
			return
		case <-catalogChanged:
		case <-pairingChanged:
		case <-keepAlive.C:
			// Un commentaire garde la connexion ouverte à travers un proxy ; ce n'est pas un réveil de la page.
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()

			continue
		}
		fmt.Fprint(w, "event: changed\ndata: {}\n\n")
		flusher.Flush()
	}
}
