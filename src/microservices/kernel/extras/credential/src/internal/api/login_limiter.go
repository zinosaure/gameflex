package api

// LES ESSAIS DE MOT DE PASSE SONT LIMITÉS (#454, décidé par le porteur le
// 2026-10-24 : « oui, dans GameFlex »).
//
// Rien ne freinait un essai après l'autre : la page de connexion se joint
// depuis Internet derrière le proxy, et la documentation publique en était
// réduite à conseiller de limiter au proxy. Désormais :
//
//   - un compte depuis une adresse : 5 échecs libres, puis une minute
//     d'attente, qui double à chaque échec suivant jusqu'à 15 minutes ;
//   - un compte, toutes adresses confondues : la même chose après 20 échecs.
//     L'adresse est celle que la requête PRÉTEND (claimedClientAddress) :
//     un en-tête que n'importe qui écrit. Sans ce second compteur, un essai
//     par adresse inventée ne serait jamais freiné ; avec lui, deviner un
//     mot de passe reste hors de portée, au prix d'une attente possible pour
//     le vrai titulaire pendant qu'on s'acharne sur son compte.
//
// En mémoire : une connexion réussie efface les compteurs du compte, un
// redémarrage du service efface tout. Un échec vieux d'une heure est oublié.

import (
	"strings"
	"sync"
	"time"
)

const (
	loginFreeTries        = 5
	loginFreeTriesAccount = 20
	loginFirstWait        = time.Minute
	loginMaxWait          = 15 * time.Minute
	loginForget           = time.Hour
	// Au-delà, un échec range d'abord les entrées oubliées : des adresses
	// inventées par milliers ne font pas grossir la mémoire sans fin.
	loginTrackedLimit = 10000
)

type loginFailures struct {
	count int
	last  time.Time
	until time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginFailures
	now     func() time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: map[string]*loginFailures{}, now: time.Now}
}

// loginKeys — le compteur du compte depuis cette adresse, et celui du compte.
func loginKeys(username, address string) (pair, account string) {
	name := strings.ToLower(strings.TrimSpace(username))

	return "pair\x00" + name + "\x00" + address, "account\x00" + name
}

// wait — combien attendre encore avant un essai pour ce compte, depuis cette
// adresse ; zéro, l'essai est libre.
func (limiter *loginLimiter) wait(username, address string) time.Duration {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	longest := time.Duration(0)
	pair, account := loginKeys(username, address)
	for _, key := range []string{pair, account} {
		if entry, ok := limiter.entries[key]; ok && entry.until.After(now) {
			longest = max(longest, entry.until.Sub(now))
		}
	}

	return longest
}

// fail — un échec de plus, sur les deux compteurs.
func (limiter *loginLimiter) fail(username, address string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	if len(limiter.entries) >= loginTrackedLimit {
		for key, entry := range limiter.entries {
			if now.Sub(entry.last) > loginForget {
				delete(limiter.entries, key)
			}
		}
	}
	pair, account := loginKeys(username, address)
	limiter.count(pair, loginFreeTries, now)
	limiter.count(account, loginFreeTriesAccount, now)
}

func (limiter *loginLimiter) count(key string, free int, now time.Time) {
	entry, ok := limiter.entries[key]
	if !ok || now.Sub(entry.last) > loginForget {
		entry = &loginFailures{}
		limiter.entries[key] = entry
	}
	entry.count++
	entry.last = now
	if over := entry.count - free; over >= 0 {
		wait := loginFirstWait
		for i := 0; i < over && wait < loginMaxWait; i++ {
			wait *= 2
		}
		entry.until = now.Add(min(wait, loginMaxWait))
	}
}

// succeed — le bon mot de passe : les compteurs du compte repartent de zéro.
func (limiter *loginLimiter) succeed(username, address string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	pair, account := loginKeys(username, address)
	delete(limiter.entries, pair)
	delete(limiter.entries, account)
}
