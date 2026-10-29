package api

// LES DEUX REGISTRES, MÉMORISÉS QUELQUES SECONDES (#190).
//
// Chaque affichage de la liste des jeux — et chaque frappe de recherche après
// 300 ms — demandait au service d'identité la liste des pairs ET celle des
// partages. Deux requêtes HTTP de plus, avec cinq secondes de délai chacune, et
// surtout **avant toute lecture de cache** : même catalogue parfaitement chaud,
// on payait ces deux allers-retours. Pour une donnée qui ne change que lorsque
// quelqu'un ajoute ou retire un serveur, c'est-à-dire presque jamais.
//
// Court exprès. Un registre qu'on vient de modifier doit se voir tout de suite :
// dix secondes passent inaperçues à la lecture et couvrent une session de
// navigation entière, où la même liste est redemandée des dizaines de fois.

import (
	"sync"
	"time"
)

// registryTTL — la durée de mémoire des deux registres.
const registryTTL = 10 * time.Second

// listCache — une liste, et la date à laquelle on l'a obtenue.
//
// Générique parce que les deux registres ne diffèrent que par leur type
// d'élément : tout le reste — le verrou, l'âge, le repli — est identique, et
// deux copies de dix lignes auraient divergé au premier correctif. Les méthodes
// portent sur le type générique, ce que Go autorise (ce qu'il refuse, ce sont
// des méthodes qui introduiraient leurs PROPRES paramètres de type).
//
// Zéro-valeur utilisable, comme les autres caches du serveur : rien à
// initialiser dans le constructeur, donc rien à y oublier.
type listCache[T any] struct {
	mu        sync.Mutex
	items     []T
	fetchedAt time.Time
}

// get rend la liste mémorisée si elle est encore fraîche, sinon la redemande.
//
// LE VERROU N'EST PAS TENU PENDANT L'APPEL RÉSEAU : le tenir sérialiserait
// toutes les listes de jeux derrière une seule requête au service d'identité,
// ce qui remplacerait un coût par un autre. Deux appelants simultanés sur un
// cache expiré font donc chacun leur requête — c'est le prix, il est payé une
// fois toutes les dix secondes, et il évite un point de contention.
func (cache *listCache[T]) get(fetch func() ([]T, error)) ([]T, error) {
	cache.mu.Lock()
	if !cache.fetchedAt.IsZero() && time.Since(cache.fetchedAt) < registryTTL {
		items := cache.items
		cache.mu.Unlock()

		return items, nil
	}
	cache.mu.Unlock()

	items, err := fetch()
	if err != nil {
		// ON NE MÉMORISE PAS UN ÉCHEC, et on ne sert pas non plus une liste
		// périmée à sa place : l'appelant sait déjà quoi faire d'une erreur
		// (la fédération saute le pair, les montages le disent au journal).
		return nil, err
	}

	cache.mu.Lock()
	cache.items = items
	cache.fetchedAt = time.Now()
	cache.mu.Unlock()

	return items, nil
}

// forget vide la mémoire — pour qu'une modification du registre se voie sans
// attendre la fin du délai.
func (cache *listCache[T]) forget() {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.items = nil
	cache.fetchedAt = time.Time{}
}

// find rend l'élément que `matches` désigne.
//
// UN IDENTIFIANT INCONNU EST LE SIGNE QUE LE REGISTRE A CHANGÉ (#230). Les deux
// registres vivent au service d'identité, et la page y écrit directement :
// aucune route du kernel ne voit passer l'ajout d'un partage ou d'un serveur,
// rien ne pouvait donc appeler `forget` au bon moment. Un partage créé juste
// après une lecture répondait « mount not found » jusqu'à 10 s (mesuré le
// 2026-10-02 : 404 à t+0, 3, 6 et 9 s, 200 à t+11 s). On oublie donc la
// mémoire et on relit UNE fois — le seul coût est pour les identifiants
// inconnus, et un vrai inconnu coûte une relecture, pas une boucle.
func (cache *listCache[T]) find(fetch func() ([]T, error), matches func(T) bool) (T, bool, error) {
	var zero T

	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			cache.forget()
		}

		items, err := cache.get(fetch)
		if err != nil {
			return zero, false, err
		}

		for _, item := range items {
			if matches(item) {
				return item, true, nil
			}
		}
	}

	return zero, false, nil
}
