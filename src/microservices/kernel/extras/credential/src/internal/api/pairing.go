package api

// LE CODE D'APPAIRAGE — chantier 60 (le porteur, 2026-11-17 : « c'est possible
// d'utiliser un code d'appairage de 6 chiffres ? Que ce soit pour rajouter un
// serveur ou même pour s'authentifier »).
//
// Un code est une INVITATION du chantier 33 (invitations.go), au secret court :
// six chiffres, valables deux minutes, une fois. Il ne s'accepte que pendant un
// MODE DÉCOUVERTE de sa sorte, qui dure cinq minutes et se prolonge :
//
//   - le mode SERVEUR, à l'administrateur : le code `federation`, qui crée le
//     lien entre deux serveurs. C'est le seul qui annonce le serveur sur le
//     réseau local (le kernel le lit dans la trame et pilote la passerelle) ;
//   - le mode COMPTE, que tout membre ouvre pour le sien : le code `account`,
//     qui rend le jeton de son compte. Il n'annonce rien.
//
// Dix puissance six codes, c'est peu pour un secret : tout repose sur le compte
// des essais faux. Deux compteurs, un par sorte, TOUTES ADRESSES CONFONDUES
// (l'adresse se prétend : X-Forwarded-For s'invente) ; au cinquième faux, le
// mode serveur se ferme, ou tous les modes compte. Un compteur repart de zéro
// quand son mode se ferme (pour `account` : le dernier mode compte ouvert) ;
// prolonger ne rend aucun essai.
//
// L'état des modes ne vit qu'en mémoire : ce service redémarré les ferme tous,
// et la base efface les codes restants au démarrage (database.Open). Les
// minuteries sont des événements bornés, une par mode et une par code — rien
// ne se relit à intervalle. Chaque changement part au kernel par la trame
// `pairing` de /internal/accounts/ws (accounts_ws.go), au téléphone par
// OnPairingChanged ; la valeur d'un code n'y passe jamais.

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gameflex/credential/internal/auth"
)

const (
	// pairingModeLifetime — « une option qui permet la découverte et
	// l'invitation venu d'ailleurs pendant une durée de 5 minutes ».
	pairingModeLifetime = 5 * time.Minute
	// pairingCodeLifetime — « 2 minutes suffiront ».
	pairingCodeLifetime = 2 * time.Minute
	// pairingWrongTries — au cinquième code faux d'une sorte, son mode se ferme.
	pairingWrongTries = 5
	// pairingRecent — combien de temps la trame dit ce qu'un code ou un mode
	// est devenu : de quoi renseigner une page qui se reconnecte.
	pairingRecent = time.Minute

	// PairingScopeServer, PairingScopeAccount — les deux modes.
	PairingScopeServer  = "server"
	PairingScopeAccount = "account"

	// Ce que devient un code (`ended`) et pourquoi un mode se ferme (`closed`).
	pairingUsed            = "used"
	pairingExpired         = "expired"
	pairingCancelled       = "cancelled"
	pairingTooManyFailures = "too_many_failures"
	pairingTimeout         = "timeout"
	pairingOwner           = "owner"
)

// pairingCodeForm — six chiffres, zéros de tête compris. Un code de 22
// caractères (une invitation en lien) suit le chemin d'avant.
var pairingCodeForm = regexp.MustCompile(`^[0-9]{6}$`)

// ErrPairingClosed — aucun mode de cette sorte n'est ouvert : le code est
// refusé sans lire la base ni rien compter.
var ErrPairingClosed = errors.New("pairing closed")

// pairingClock — l'horloge des modes et des codes. Les tests en injectent une
// qu'ils font avancer.
type pairingClock interface {
	Now() time.Time
	AfterFunc(wait time.Duration, run func()) (stop func() bool)
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) AfterFunc(wait time.Duration, run func()) func() bool {
	return time.AfterFunc(wait, run).Stop
}

// pairingMode — un mode ouvert : par qui, jusqu'à quand, et sa minuterie.
type pairingMode struct {
	userID   int64
	username string
	until    time.Time
	stop     func() bool
}

// pairingCode — un code vivant, tel que la trame le décrit : jamais sa valeur.
type pairingCode struct {
	id        int64
	kind      string
	authorID  int64
	author    string
	expiresAt time.Time
	stop      func() bool
}

type pairingEnded struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	Author string `json:"author"`
	Reason string `json:"reason"`
	By     string `json:"by,omitempty"`
	From   string `json:"from,omitempty"`
	At     string `json:"at"`
	at     time.Time
}

type pairingClosed struct {
	Scope    string `json:"scope"`
	Username string `json:"username"`
	Reason   string `json:"reason"`
	At       string `json:"at"`
	at       time.Time
}

// pairing — les deux modes, les codes vivants, les deux compteurs.
type pairing struct {
	mu       sync.Mutex
	clock    pairingClock
	server   *pairingMode
	accounts map[int64]*pairingMode
	codes    map[int64]*pairingCode
	failures map[string]int
	ended    []pairingEnded
	closed   []pairingClosed
	// hub — les kernels abonnés, et le téléphone (OnPairingChanged).
	hub *accountsHub
}

func newPairing(clock pairingClock) *pairing {
	return &pairing{
		clock: clock, accounts: map[int64]*pairingMode{}, codes: map[int64]*pairingCode{},
		failures: map[string]int{}, hub: newAccountsHub(),
	}
}

// pairingStamp — un instant de la trame, RFC 3339 UTC à la seconde.
func pairingStamp(at time.Time) string {
	return at.UTC().Format(time.RFC3339)
}

// pairingFrame — l'état, tel que le kernel le reçoit.
func (state *pairing) frameLocked() []byte {
	now := state.clock.Now()
	frame := map[string]any{"type": "pairing", "server": nil}
	if state.server != nil {
		frame["server"] = map[string]string{"open_until": pairingStamp(state.server.until), "opened_by": state.server.username}
	}

	accounts := []map[string]string{}
	for _, mode := range state.accounts {
		accounts = append(accounts, map[string]string{"username": mode.username, "open_until": pairingStamp(mode.until)})
	}
	sort.Slice(accounts, func(left, right int) bool { return accounts[left]["username"] < accounts[right]["username"] })
	frame["accounts"] = accounts

	codes := []map[string]any{}
	for _, code := range state.codes {
		codes = append(codes, map[string]any{"id": code.id, "kind": code.kind, "author": code.author, "expires_at": pairingStamp(code.expiresAt)})
	}
	sort.Slice(codes, func(left, right int) bool { return codes[left]["id"].(int64) < codes[right]["id"].(int64) })
	frame["codes"] = codes

	ended := []pairingEnded{}
	for _, entry := range state.ended {
		if now.Sub(entry.at) < pairingRecent {
			ended = append(ended, entry)
		}
	}
	state.ended = ended
	frame["ended"] = ended

	closed := []pairingClosed{}
	for _, entry := range state.closed {
		if now.Sub(entry.at) < pairingRecent {
			closed = append(closed, entry)
		}
	}
	state.closed = closed
	frame["closed"] = closed

	payload, _ := json.Marshal(frame)

	return payload
}

// publishLocked — la trame à jour part à chaque abonné.
func (state *pairing) publishLocked() {
	state.hub.publish(state.frameLocked())
}

// endCodeLocked — un code fini : il quitte la mémoire, sa minuterie s'arrête,
// et la trame dit pourquoi.
func (state *pairing) endCodeLocked(code *pairingCode, reason, by, from string) {
	if code.stop != nil {
		code.stop()
	}
	delete(state.codes, code.id)
	now := state.clock.Now()
	state.ended = append(state.ended, pairingEnded{
		ID: code.id, Kind: code.kind, Author: code.author, Reason: reason, By: by, From: from, At: pairingStamp(now), at: now,
	})
}

// closeModeLocked — un mode fermé : sa minuterie s'arrête, ses codes vivants
// sont annulés (rows à retirer de la base, rendus à l'appelant), son compteur
// repart de zéro s'il était le dernier de sa sorte.
func (state *pairing) closeModeLocked(scope string, mode *pairingMode, reason string) []int64 {
	if mode.stop != nil {
		mode.stop()
	}
	now := state.clock.Now()
	state.closed = append(state.closed, pairingClosed{Scope: scope, Username: mode.username, Reason: reason, At: pairingStamp(now), at: now})
	codeReason := pairingCancelled
	switch reason {
	case pairingTooManyFailures:
		codeReason = pairingTooManyFailures
	case pairingTimeout:
		codeReason = pairingExpired
	}

	cancelled := []int64{}
	for _, code := range state.codes {
		belongs := code.kind == InvitationFederation
		if scope == PairingScopeAccount {
			belongs = code.kind == InvitationAccount && code.authorID == mode.userID
		}
		if belongs {
			cancelled = append(cancelled, code.id)
			state.endCodeLocked(code, codeReason, "", "")
		}
	}

	if scope == PairingScopeServer {
		state.server = nil
		state.failures[InvitationFederation] = 0
	} else {
		delete(state.accounts, mode.userID)
		if len(state.accounts) == 0 {
			state.failures[InvitationAccount] = 0
		}
	}
	slog.Info("pairing closed", "mode", scope, "by", mode.username, "reason", reason)

	return cancelled
}

// forgetCodes — les lignes des codes annulés quittent la base.
func (server *Server) forgetCodes(ids []int64) {
	for _, id := range ids {
		_, _ = server.db.Exec(`DELETE FROM invitations WHERE id = ? AND pairing = 1`, id)
	}
}

// openMode — ouvrir, ou prolonger de cinq minutes à partir de maintenant. Les
// codes vivants gardent leur échéance.
func (server *Server) openMode(scope string, principal auth.Principal) time.Time {
	state := server.pairing
	state.mu.Lock()
	defer state.mu.Unlock()

	until := state.clock.Now().Add(pairingModeLifetime)
	current := state.server
	if scope == PairingScopeAccount {
		current = state.accounts[principal.ID]
	}
	renewed := current != nil
	if renewed && current.stop != nil {
		current.stop()
	}
	mode := &pairingMode{userID: principal.ID, username: principal.Username, until: until}
	if renewed {
		// Prolongé par un autre administrateur, le mode serveur garde son auteur.
		mode.userID, mode.username = current.userID, current.username
	}
	mode.stop = state.clock.AfterFunc(pairingModeLifetime, func() { server.modeTimedOut(scope, mode) })
	if scope == PairingScopeServer {
		state.server = mode
	} else {
		state.accounts[principal.ID] = mode
	}
	message := "pairing opened"
	if renewed {
		message = "pairing renewed"
	}
	slog.Info(message, "mode", scope, "by", principal.Username, "until", pairingStamp(until))
	state.publishLocked()

	return until
}

// modeTimedOut — la minuterie d'un mode : il s'éteint, s'il est encore celui-là.
func (server *Server) modeTimedOut(scope string, mode *pairingMode) {
	state := server.pairing
	state.mu.Lock()
	current := state.server
	if scope == PairingScopeAccount {
		current = state.accounts[mode.userID]
	}
	if current != mode {
		state.mu.Unlock()

		return
	}
	cancelled := state.closeModeLocked(scope, mode, pairingTimeout)
	state.publishLocked()
	state.mu.Unlock()
	server.forgetCodes(cancelled)
}

// closeMode — l'interrupteur. Faux s'il n'était pas ouvert.
func (server *Server) closeMode(scope string, principal auth.Principal) bool {
	state := server.pairing
	state.mu.Lock()
	mode := state.server
	if scope == PairingScopeAccount {
		mode = state.accounts[principal.ID]
	}
	if mode == nil {
		state.mu.Unlock()

		return false
	}
	cancelled := state.closeModeLocked(scope, mode, pairingOwner)
	state.publishLocked()
	state.mu.Unlock()
	server.forgetCodes(cancelled)

	return true
}

// codeTimedOut — la minuterie d'un code : échu, il quitte la base et la trame
// le dit.
func (server *Server) codeTimedOut(id int64) {
	state := server.pairing
	state.mu.Lock()
	code, alive := state.codes[id]
	if !alive {
		state.mu.Unlock()

		return
	}
	state.endCodeLocked(code, pairingExpired, "", "")
	state.publishLocked()
	state.mu.Unlock()
	server.forgetCodes([]int64{id})
}

// randomPairingCode — six chiffres tirés uniformément, zéros de tête compris.
func randomPairingCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%06d", value.Int64()), nil
}

// createPairingCode — un code de cette sorte, pour l'auteur. Son échéance est
// la plus proche de deux dates : deux minutes, ou la fin de son mode. Il
// remplace le code vivant de son auteur, dans la même transaction.
func (server *Server) createPairingCode(kind, label, scopes string, remoteAdmin bool, userID any, principal auth.Principal) (invitationResponse, error) {
	state := server.pairing
	state.mu.Lock()
	defer state.mu.Unlock()

	mode := state.server
	if kind == InvitationAccount {
		mode = state.accounts[principal.ID]
	}
	if mode == nil {
		return invitationResponse{}, ErrPairingClosed
	}
	instant := state.clock.Now()
	expiresAt := instant.Add(pairingCodeLifetime)
	if mode.until.Before(expiresAt) {
		expiresAt = mode.until
	}

	transaction, err := server.db.Begin()
	if err != nil {
		return invitationResponse{}, err
	}
	defer transaction.Rollback()

	if _, err := transaction.Exec(`DELETE FROM invitations WHERE pairing = 1 AND created_by_id = ?`, principal.ID); err != nil {
		return invitationResponse{}, err
	}
	var (
		code string
		id   int64
	)
	// Un code qui tombe sur un code vivant est refusé par la contrainte UNIQUE :
	// un autre est tiré.
	for attempt := 0; attempt < 20 && id == 0; attempt++ {
		if code, err = randomPairingCode(); err != nil {
			return invitationResponse{}, err
		}
		result, err := transaction.Exec(
			`INSERT INTO invitations (kind, code_hash, label, scopes, remote_admin, user_id, expires_at, created_at, created_by_id, pairing)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			kind, hashToken(code), label, scopes, remoteAdmin, userID, pairingStamp(expiresAt), now(), principal.ID,
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				continue
			}

			return invitationResponse{}, err
		}
		id, _ = result.LastInsertId()
	}
	if id == 0 {
		return invitationResponse{}, errors.New("no free pairing code")
	}
	if err := transaction.Commit(); err != nil {
		return invitationResponse{}, err
	}

	for _, previous := range state.codes {
		if previous.authorID == principal.ID {
			state.endCodeLocked(previous, pairingCancelled, "", "")
		}
	}
	entry := &pairingCode{id: id, kind: kind, authorID: principal.ID, author: principal.Username, expiresAt: expiresAt}
	entry.stop = state.clock.AfterFunc(expiresAt.Sub(instant), func() { server.codeTimedOut(id) })
	state.codes[id] = entry
	slog.Info("pairing code created", "kind", kind, "author", principal.Username)
	state.publishLocked()

	return invitationResponse{ID: id, Kind: kind, Label: label, Scopes: scopes, ExpiresAt: pairingStamp(expiresAt), Code: code}, nil
}

// pairingOpenFor — un mode de cette sorte est-il ouvert ? Pour `account` : au
// moins un, le code étant seul, sans identifiant.
func (state *pairing) pairingOpenFor(kind string) bool {
	state.mu.Lock()
	defer state.mu.Unlock()

	if kind == InvitationFederation {
		return state.server != nil
	}

	return len(state.accounts) > 0
}

// wrongPairingCode — un essai faux de plus au compteur de sa sorte. Au
// cinquième, le mode serveur se ferme, ou tous les modes compte.
func (server *Server) wrongPairingCode(kind, address string) {
	state := server.pairing
	state.mu.Lock()
	state.failures[kind]++
	slog.Warn("pairing code refused", "kind", kind, "from", address, "failures", state.failures[kind])
	cancelled := []int64{}
	if state.failures[kind] >= pairingWrongTries {
		if kind == InvitationFederation && state.server != nil {
			cancelled = append(cancelled, state.closeModeLocked(PairingScopeServer, state.server, pairingTooManyFailures)...)
		}
		if kind == InvitationAccount {
			for _, mode := range state.accounts {
				cancelled = append(cancelled, state.closeModeLocked(PairingScopeAccount, mode, pairingTooManyFailures)...)
			}
		}
	}
	state.publishLocked()
	state.mu.Unlock()
	server.forgetCodes(cancelled)
}

// usedPairingCode — le code servi : la trame dit à qui.
func (server *Server) usedPairingCode(id int64, from, address string) {
	state := server.pairing
	state.mu.Lock()
	defer state.mu.Unlock()

	if code, alive := state.codes[id]; alive {
		slog.Info("pairing code used", "kind", code.kind, "by", from, "from", address)
		state.endCodeLocked(code, pairingUsed, from, address)
		state.publishLocked()
	}
}

// redeemPairingCode — le chemin des six chiffres dans RedeemInvitation.
//
//  1. aucun mode de sa sorte : refusé (ErrPairingClosed), sans rien compter ;
//  2. sous le frein par adresse : ThrottledError, comme une invitation ;
//  3. la sorte fait partie de la recherche : un code de l'autre sorte vaut un
//     code faux, et ne dit pas qu'il existe ;
//  4. introuvable ou échu : un essai faux au compteur de sa sorte, et un échec
//     pour l'adresse ;
//  5. le jeton d'un compte éteint : refusé sans consommer ni compter ;
//  6. sinon le DELETE consomme, et l'échange rend ce qu'il rend aujourd'hui.
func (server *Server) redeemPairingCode(code, kind, from, address string) (Redeemed, error) {
	if server.pairing == nil || !server.pairing.pairingOpenFor(kind) {
		slog.Warn("pairing code refused", "kind", kind, "from", address, "reason", "closed")

		return Redeemed{}, ErrPairingClosed
	}
	limiterKey := "invitation\x00" + address
	if wait := server.logins.wait(limiterKey, address); wait > 0 {
		slog.Warn("invitation delayed", "from", address, "wait", wait.Round(time.Second))

		return Redeemed{}, ThrottledError{Wait: wait}
	}
	refuse := func() (Redeemed, error) {
		server.logins.fail(limiterKey, address)
		server.wrongPairingCode(kind, address)

		return Redeemed{}, ErrInvitationRefused
	}

	var (
		id                       int64
		label, scopes, expiresAt string
		userID, createdBy        sql.NullInt64
		remoteAdmin              bool
	)
	err := server.db.QueryRow(
		`SELECT id, label, scopes, remote_admin, user_id, expires_at, created_by_id FROM invitations WHERE code_hash = ? AND kind = ? AND pairing = 1`,
		hashToken(code), kind,
	).Scan(&id, &label, &scopes, &remoteAdmin, &userID, &expiresAt, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse()
	}
	if err != nil {
		return Redeemed{}, err
	}
	if expiresAt <= pairingStamp(server.pairing.clock.Now()) {
		return refuse()
	}

	var (
		account      TokenAccount
		accountToken string
	)
	if kind == InvitationAccount {
		account, accountToken, err = server.validAccount(`t.user_id = ?`, userID.Int64)
		if errors.Is(err, ErrTokenRefused) {
			return Redeemed{}, ErrInvitationTokenOff
		}
		if err != nil {
			return Redeemed{}, err
		}
	}

	result, err := server.db.Exec(`DELETE FROM invitations WHERE id = ? AND pairing = 1`, id)
	if err != nil {
		return Redeemed{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		// Un autre échange l'a pris entre la lecture et ici.
		return refuse()
	}
	server.logins.succeed(limiterKey, address)
	server.usedPairingCode(id, from, address)

	return server.deliverInvitation(kind, label, scopes, remoteAdmin, createdBy, account, accountToken, address)
}

// handleOpenServerPairing — POST /api/v1/token-invitations/pairing/server/open.
func (server *Server) handleOpenServerPairing(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if refuseFederated(w, principal) {
		return
	}
	respondItem(w, http.StatusOK, map[string]string{"open_until": pairingStamp(server.openMode(PairingScopeServer, principal))})
}

// handleCloseServerPairing — POST /api/v1/token-invitations/pairing/server/close.
func (server *Server) handleCloseServerPairing(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	server.closeMode(PairingScopeServer, principal)
	w.WriteHeader(http.StatusNoContent)
}

// handleOpenAccountPairing — POST /api/v1/token-invitations/pairing/account/open :
// le mode du compte de l'appelant, quand son jeton est actif — un code de
// compte n'aurait rien à rendre sinon.
func (server *Server) handleOpenAccountPairing(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if refuseFederated(w, principal) {
		return
	}
	if _, _, err := server.validAccount(`t.user_id = ?`, principal.ID); err != nil {
		if errors.Is(err, ErrTokenRefused) {
			respondErrorCode(w, http.StatusForbidden, "token_not_enabled", "Your token is not enabled: ask an administrator.")

			return
		}
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	respondItem(w, http.StatusOK, map[string]string{"open_until": pairingStamp(server.openMode(PairingScopeAccount, principal))})
}

// handleCloseAccountPairing — POST /api/v1/token-invitations/pairing/account/close.
func (server *Server) handleCloseAccountPairing(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	server.closeMode(PairingScopeAccount, principal)
	w.WriteHeader(http.StatusNoContent)
}

// handleCreatePairingCode — POST /api/v1/token-invitations/pairing/codes
// {kind, label, scopes}. Les règles d'une invitation de sa sorte (handleCreateInvitation),
// plus son mode ouvert : 409 `pairingClosed` sinon. Le code n'en sort qu'une fois.
func (server *Server) handleCreatePairingCode(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		Kind        string   `json:"kind"`
		Label       string   `json:"label"`
		Scopes      []string `json:"scopes"`
		RemoteAdmin bool     `json:"remote_admin"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if refuseFederated(w, principal) {
		return
	}

	var (
		label, scopes string
		userID        any
	)
	switch body.Kind {
	case InvitationFederation:
		if !principal.IsAdmin() {
			respondErrorCode(w, http.StatusForbidden, "admin_only", "Admin access required.")

			return
		}
		if label = strings.TrimSpace(body.Label); label == "" {
			respondValidationError(w, []map[string]any{{
				"loc": []string{"body", "label"}, "msg": "A name is required.", "type": "value_error.missing",
			}})

			return
		}
		parsed, err := parseScopes(body.Scopes)
		if err != nil {
			respondValidationError(w, []map[string]any{{
				"loc": []string{"body", "scopes"}, "msg": err.Error(), "type": "value_error",
			}})

			return
		}
		scopes = parsed
	case InvitationAccount:
		if _, _, err := server.validAccount(`t.user_id = ?`, principal.ID); err != nil {
			respondErrorCode(w, http.StatusForbidden, "token_not_enabled", "Your token is not enabled: ask an administrator.")

			return
		}
		if label = principal.DisplayName; label == "" {
			label = principal.Username
		}
		userID = principal.ID
	default:
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "kind must be federation or account")

		return
	}

	created, err := server.createPairingCode(body.Kind, label, scopes, body.RemoteAdmin && body.Kind == InvitationFederation, userID, principal)
	if errors.Is(err, ErrPairingClosed) {
		respondErrorCode(w, http.StatusConflict, "pairingClosed", "Discovery mode is off.")

		return
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	respondItem(w, http.StatusCreated, created)
}

// handleCancelPairingCode — DELETE /api/v1/token-invitations/pairing/codes/{id} :
// son auteur l'annule.
func (server *Server) handleCancelPairingCode(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondErrorCode(w, http.StatusNotFound, "not_found", "Code not found.")

		return
	}
	state := server.pairing
	state.mu.Lock()
	code, alive := state.codes[id]
	if !alive || code.authorID != principal.ID {
		state.mu.Unlock()
		respondErrorCode(w, http.StatusNotFound, "not_found", "Code not found.")

		return
	}
	state.endCodeLocked(code, pairingCancelled, "", "")
	state.publishLocked()
	state.mu.Unlock()
	server.forgetCodes([]int64{id})
	w.WriteHeader(http.StatusNoContent)
}

// PairingFrame — l'état des modes et des codes, comme la trame `pairing` le
// porte. Pour le téléphone (embed), dont le kernel le lit sans canal.
func (server *Server) PairingFrame() []byte {
	server.pairing.mu.Lock()
	defer server.pairing.mu.Unlock()

	return server.pairing.frameLocked()
}

// OnPairingChanged — `callback(frame)` après chaque changement d'un mode ou
// d'un code : le même signal que la trame des kernels, sans le canal.
func (server *Server) OnPairingChanged(callback func(frame []byte)) {
	slot := server.pairing.hub.subscribe()
	go func() {
		for frame := range slot {
			callback(frame)
		}
	}()
}
