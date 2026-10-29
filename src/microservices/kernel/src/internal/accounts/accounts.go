// Package accounts — l'état des comptes, tel que le service d'identité le
// pousse au kernel (chantier 20, le porteur, 2026-10-21 : « déconnecter le
// joueur depuis le serveur s'il n'est plus »).
//
// Le kernel croyait un jeton signé jusqu'à son expiration — sept jours sur un
// serveur : un compte supprimé gardait la main, un compte rétrogradé ses droits
// d'administrateur, et personne ne savait quelles consoles un compte pouvait
// lancer. Il tient désormais une COPIE des comptes (nom, rang, consoles, état
// du jeton), reçue par le canal interne du service d'identité
// (`/internal/accounts/ws`) et remplacée à chaque changement. Toute décision se
// prend sur elle : aucun appel par requête, aucun sondage.
//
// LA PROMESSE DE `internal/auth` EST GARDÉE : « une panne du service d'identité
// ne doit jamais empêcher un joueur déjà connecté de lancer ». Tant qu'aucune
// liste n'est arrivée (le service ne répond pas au démarrage), tout jeton
// valide passe, comme avant. Une fois la liste reçue, un compte retiré reste
// refusé, même si le service tombe ensuite : la copie ne s'efface pas.
package accounts

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// RoleAdmin, RoleUser — la même échelle que le service d'identité.
const (
	RoleAdmin = 1
	RoleUser  = 2
)

// Account — un compte, tel que le service d'identité le décrit.
type Account struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	RoleLevel int        `json:"role_level"`
	Consoles  []string   `json:"consoles"`
	Token     TokenState `json:"token"`
}

// TokenState — le jeton de fédération du compte : sa génération (régénéré, elle
// change), s'il est actif, et son échéance.
type TokenState struct {
	Generation string    `json:"generation"`
	Enabled    bool      `json:"enabled"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// valid — le jeton de cette génération vaut-il encore, maintenant ?
func (token TokenState) valid(generation string, now time.Time) bool {
	return token.Enabled && generation != "" && token.Generation == generation && now.Before(token.ExpiresAt)
}

// Change — ce qu'une nouvelle liste retire, pour couper.
type Change struct {
	// Deleted — les comptes qui n'existent plus.
	Deleted []string
	// Demoted — les administrateurs devenus joueurs.
	Demoted []string
	// TokenRetired — les comptes dont le jeton a été désactivé, régénéré ou
	// raccourci : les sessions ouvertes avec lui depuis un autre serveur
	// tombent.
	TokenRetired []string
}

// Empty — rien à couper.
func (change Change) Empty() bool {
	return len(change.Deleted) == 0 && len(change.Demoted) == 0 && len(change.TokenRetired) == 0
}

// Store — la copie des comptes.
type Store struct {
	mu     sync.RWMutex
	ready  bool
	byName map[string]Account
	now    func() time.Time

	// OnChange — appelé après chaque liste qui retire quelque chose.
	OnChange func(Change)
}

// New — une copie vide : tout jeton valide passe jusqu'à la première liste.
func New() *Store {
	return &Store{byName: map[string]Account{}, now: time.Now}
}

// Ready — une liste est-elle arrivée ?
func (store *Store) Ready() bool {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return store.ready
}

// Replace — une nouvelle liste. Rend ce qu'elle retire par rapport à la
// précédente (rien pour la première), et prévient OnChange.
func (store *Store) Replace(list []Account) Change {
	next := make(map[string]Account, len(list))
	for _, account := range list {
		next[account.Username] = account
	}

	store.mu.Lock()
	previous, wasReady := store.byName, store.ready
	store.byName, store.ready = next, true
	store.mu.Unlock()

	change := Change{}
	if wasReady {
		for name, before := range previous {
			after, kept := next[name]
			switch {
			case !kept:
				change.Deleted = append(change.Deleted, name)
			default:
				if before.RoleLevel <= RoleAdmin && after.RoleLevel > RoleAdmin {
					change.Demoted = append(change.Demoted, name)
				}
				if before.Token.Generation != after.Token.Generation ||
					(before.Token.Enabled && !after.Token.Enabled) ||
					after.Token.ExpiresAt.Before(before.Token.ExpiresAt) {
					change.TokenRetired = append(change.TokenRetired, name)
				}
			}
		}
	}
	if !change.Empty() && store.OnChange != nil {
		store.OnChange(change)
	}

	return change
}

// Admit — ce jeton (déjà vérifié : signature, date) vaut-il encore ici, et
// avec quel rang ? `subject` est l'identifiant du compte que le jeton nomme ;
// `fed` est la génération du jeton de compte qui a ouvert la session depuis un
// autre serveur, vide pour une connexion ordinaire.
//
// Avant toute liste : oui, avec le rang du jeton — la promesse de
// `internal/auth`. Ensuite : le compte doit exister, ce doit être LE MÊME — un
// compte supprimé puis recréé sous ce nom a un autre identifiant, et les
// jetons de l'ancien ne sont pas à lui (#601) —, le rang est le sien (une
// rétrogradation mord tout de suite), et une session ouverte par jeton suit
// son jeton — jamais administrateur. Le service d'identité met `sub` dans
// tout jeton qu'il signe.
func (store *Store) Admit(username, subject, fed string, claimedRole int) (int, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	if !store.ready {
		return claimedRole, true
	}
	account, known := store.byName[username]
	if !known {
		return 0, false
	}
	if subject != "" && subject != strconv.FormatInt(account.ID, 10) {
		return 0, false
	}
	if fed != "" {
		if !account.Token.valid(fed, store.now()) {
			return 0, false
		}

		return RoleUser, true
	}

	return account.RoleLevel, true
}

// Grants — les consoles que ce compte peut lancer. `known` est faux avant
// toute liste : l'appelant laisse alors passer, comme avant.
func (store *Store) Grants(username string) (grants []string, known bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	if !store.ready {
		return nil, false
	}
	account, found := store.byName[username]
	if !found {
		return []string{}, true
	}

	return account.Consoles, true
}

// Follow — se tenir à la liste du service d'identité : ouvrir son canal,
// remplacer la copie à chaque liste, et se reconnecter s'il tombe (une seconde,
// puis le double, jusqu'à trente). Rend la main quand ctx se termine.
func (store *Store) Follow(ctx context.Context, url, secret string) {
	if url == "" || secret == "" {
		slog.Warn("accounts: no identity service or internal secret — every valid token is admitted")

		return
	}
	wait := time.Second
	for ctx.Err() == nil {
		if store.follow(ctx, url, secret) {
			wait = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait *= 2; wait > 30*time.Second {
			wait = 30 * time.Second
		}
	}
}

// follow — une connexion, jusqu'à sa fin. Vrai si une liste est arrivée.
func (store *Store) follow(ctx context.Context, url, secret string) bool {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, http.Header{"X-Internal-Secret": {secret}})
	if err != nil {
		slog.Warn("accounts: identity service unreachable", "error", err)

		return false
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	received := false
	for {
		var message struct {
			Type     string    `json:"type"`
			Accounts []Account `json:"accounts"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			if ctx.Err() == nil {
				slog.Warn("accounts: identity channel closed", "error", err)
			}

			return received
		}
		if message.Type != "accounts" {
			continue
		}
		change := store.Replace(message.Accounts)
		received = true
		if !change.Empty() {
			slog.Info("accounts: cut", "deleted", change.Deleted, "demoted", change.Demoted, "token_retired", change.TokenRetired)
		}
	}
}

// decodeList — pour les tests : une liste telle que le canal la donne.
func decodeList(payload []byte) ([]Account, error) {
	var message struct {
		Accounts []Account `json:"accounts"`
	}
	err := json.Unmarshal(payload, &message)

	return message.Accounts, err
}
