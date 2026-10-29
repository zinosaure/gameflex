// Package ticket — le téléchargement par ticket, pour que le NAVIGATEUR
// télécharge lui-même un jeu de la bibliothèque.
//
// LE PROBLÈME. Toute route qui lit un jeu exige un compte, et un lien
// (`<a download>`, une navigation) ne peut PAS porter d'en-tête
// Authorization. La page téléchargeait donc elle-même, en mémoire, puis
// remettait un blob au navigateur — qui perdait sa barre de progression, sa
// reprise après coupure, son dossier Téléchargements, et un jeu de plusieurs
// gigaoctets ne tenait pas en mémoire.
//
// UN TICKET, ET NON UNE SIGNATURE. Un HMAC sur (console, référence,
// expiration) reste rejouable jusqu'à son expiration sans que l'hôte en garde
// trace ; un ticket ALÉATOIRE tenu par l'hôte est consommé à la lecture — il
// disparaît au premier usage. C'est le patron que le dépôt a déjà choisi pour
// le pad libéré : « nominative, single-use and short ».
//
// CE QU'UN TICKET N'EST PAS : un jeton de session. Il ne porte aucune identité
// et ne vaut que pour UN jeu nommé à l'émission. L'émission est authentifiée,
// la consommation non : c'est tout l'objet.
//
// PARTAGÉ PAR LES DEUX KERNELS (chantier 14, 2026-10-12) : le serveur l'avait
// seul, et le téléphone n'avait pas de téléchargement par le navigateur.
package ticket

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// DefaultTTL — une minute. Le ticket ne couvre pas la DURÉE du téléchargement,
// seulement le temps qui sépare le clic de la première requête du
// navigateur : une fois la réponse commencée, elle va jusqu'au bout. Assez
// long pour un aller-retour et un onglet lent, assez court pour qu'un lien
// copié par-dessus l'épaule ne serve à rien.
const DefaultTTL = time.Minute

type ticket struct {
	consoleID string
	ref       string
	expiresAt time.Time
}

// Store — les tickets en cours. Le zéro est prêt à servir.
type Store struct {
	mu      sync.Mutex
	byNonce map[string]ticket
	// TTL — zéro : DefaultTTL.
	TTL time.Duration
	// now — l'horloge, remplacée dans les tests.
	now func() time.Time
}

func (store *Store) clock() time.Time {
	if store.now != nil {
		return store.now()
	}

	return time.Now()
}

// Mint — un ticket neuf pour ce jeu. 32 octets de crypto/rand : un ticket
// devinable ne vaudrait rien.
func (store *Store) Mint(consoleID, ref string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	nonce := base64.RawURLEncoding.EncodeToString(raw)

	store.mu.Lock()
	defer store.mu.Unlock()

	// BALAYAGE À L'ÉMISSION, plutôt qu'une tâche de fond : les tickets sont
	// rares et minuscules.
	now := store.clock()
	if store.byNonce == nil {
		store.byNonce = map[string]ticket{}
	}
	for key, existing := range store.byNonce {
		if now.After(existing.expiresAt) {
			delete(store.byNonce, key)
		}
	}
	ttl := store.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	store.byNonce[nonce] = ticket{consoleID: consoleID, ref: ref, expiresAt: now.Add(ttl)}

	return nonce, nil
}

// Consume — le rend UNE fois, et jamais deux. La suppression est
// inconditionnelle : un ticket présenté est consommé, même expiré — sinon une
// seconde présentation apprendrait au moins qu'il a existé.
func (store *Store) Consume(nonce string) (consoleID string, ref string, ok bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	found, known := store.byNonce[nonce]
	if !known {
		return "", "", false
	}
	delete(store.byNonce, nonce)

	if store.clock().After(found.expiresAt) {
		return "", "", false
	}

	return found.consoleID, found.ref, true
}
