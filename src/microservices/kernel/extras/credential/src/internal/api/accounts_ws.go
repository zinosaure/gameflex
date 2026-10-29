package api

// L'ÉTAT DES COMPTES, POUSSÉ AU KERNEL (chantier 20) — « ce qu'il faut c'est de
// déconnecter le joueur depuis le serveur s'il n'est plus » (le porteur).
//
// Le kernel croyait un jeton signé jusqu'à son expiration : un compte supprimé
// gardait la main (canal de commandes, lancements), un compte rétrogradé ses
// droits d'administrateur. Il ouvre désormais ce canal interne et reçoit la
// liste des comptes — nom, rang, consoles, état du jeton —, puis la liste à
// jour à chaque changement. Il décide sur cette copie : aucun appel par
// requête, aucun sondage (« Pas de sondage », CODE_CONVENTIONS), et une panne
// de ce service ne l'empêche pas de laisser jouer ceux qu'il connaît déjà.
//
// La liste entière à chaque fois, pas un différentiel : elle tient en
// quelques dizaines de lignes, et une liste ratée est rattrapée par la
// suivante sans qu'on ait à rejouer quoi que ce soit.

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type accountEntry struct {
	ID        int64             `json:"id"`
	Username  string            `json:"username"`
	RoleLevel int               `json:"role_level"`
	Consoles  []string          `json:"consoles"`
	Token     accountTokenState `json:"token"`
}

// accountTokenState — ce que le kernel doit savoir du jeton pour couper les
// sessions qu'il a ouvertes : sa génération (régénéré, elle change), s'il est
// actif, et son échéance (UTC, RFC 3339).
type accountTokenState struct {
	Generation string `json:"generation"`
	Enabled    bool   `json:"enabled"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

// accountsHub — les kernels abonnés. Chacun a une place, qui garde la DERNIÈRE
// liste : un abonné lent ne retient personne, et ne rate jamais la plus récente.
type accountsHub struct {
	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
}

func newAccountsHub() *accountsHub {
	return &accountsHub{subscribers: map[chan []byte]struct{}{}}
}

func (hub *accountsHub) subscribe() chan []byte {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	slot := make(chan []byte, 1)
	hub.subscribers[slot] = struct{}{}

	return slot
}

func (hub *accountsHub) unsubscribe(slot chan []byte) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	delete(hub.subscribers, slot)
}

func (hub *accountsHub) publish(payload []byte) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for slot := range hub.subscribers {
		select {
		case <-slot: // la liste d'avant, pas encore lue : la nouvelle la remplace
		default:
		}
		slot <- payload
	}
}

// accountsSnapshot — la liste des comptes, telle que le kernel la reçoit.
func (server *Server) accountsSnapshot() ([]byte, error) {
	rows, err := server.db.Query(
		`SELECT u.id, u.username, u.role_level, u.consoles, t.token, t.enabled, t.expires_at
		 FROM users u LEFT JOIN account_tokens t ON t.user_id = u.id ORDER BY u.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []accountEntry{}
	for rows.Next() {
		var (
			entry     accountEntry
			stored    string
			token     sql.NullString
			enabled   sql.NullInt64
			expiresAt sql.NullString
		)
		if err := rows.Scan(&entry.ID, &entry.Username, &entry.RoleLevel, &stored, &token, &enabled, &expiresAt); err != nil {
			return nil, err
		}
		entry.Consoles = effectiveGrants(entry.RoleLevel, stored)
		if token.Valid && len(token.String) >= 8 {
			entry.Token.Generation = token.String[:8]
		}
		entry.Token.Enabled = enabled.Valid && enabled.Int64 == 1
		if until, dated := parseStoredTime(expiresAt); dated {
			entry.Token.ExpiresAt = until.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		accounts = append(accounts, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"type": "accounts", "accounts": accounts})
}

// accountsChanged — un compte ou son jeton vient de changer : la liste à jour
// part à chaque kernel abonné.
func (server *Server) accountsChanged() {
	if server.accounts == nil {
		return
	}
	payload, err := server.accountsSnapshot()
	if err != nil {
		slog.Error("accounts snapshot failed", "error", err)

		return
	}
	server.accounts.publish(payload)
}

// handleAccountsWS — GET /internal/accounts/ws : le canal du kernel. La liste
// tout de suite, puis à chaque changement.
func (server *Server) handleAccountsWS(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	slot := server.accounts.subscribe()
	defer server.accounts.unsubscribe(slot)

	// La fermeture se voit en lisant : le kernel n'envoie rien d'autre.
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	payload, err := server.accountsSnapshot()
	if err != nil {
		slog.Error("accounts snapshot failed", "error", err)

		return
	}
	for {
		conn.SetWriteDeadline(writeDeadline())
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
		select {
		case <-gone:
			return
		case payload = <-slot:
		}
	}
}

// OnAccountsChanged — `callback` après chaque changement d'un compte ou de son
// jeton. Pour le téléphone (embed), dont le kernel tourne dans le même
// processus : le même signal que le canal des kernels, sans le canal — il
// rejuge alors lui-même les jetons qu'il tient.
func (server *Server) OnAccountsChanged(callback func()) {
	slot := server.accounts.subscribe()
	go func() {
		for range slot {
			callback()
		}
	}()
}
