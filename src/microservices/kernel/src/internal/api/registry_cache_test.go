package api

import (
	"testing"
)

// Un registre qui change entre deux appels : chaque appel rend la version
// suivante, et compte combien de fois on l'a demandé.
type changingRegistry struct {
	versions [][]int64
	calls    int
}

func (registry *changingRegistry) fetch() ([]int64, error) {
	index := registry.calls
	if index >= len(registry.versions) {
		index = len(registry.versions) - 1
	}
	registry.calls++

	return registry.versions[index], nil
}

func is(wanted int64) func(int64) bool {
	return func(id int64) bool { return id == wanted }
}

// Le cas de #230, mesuré le 2026-10-02 : un partage créé juste après une
// lecture du registre répondait « mount not found » pendant 10 s, parce que
// la liste mémorisée ne le contenait pas encore et que rien ne l'oubliait.
func TestAnUnknownIDRefreshesTheRegistryOnce(t *testing.T) {
	registry := &changingRegistry{versions: [][]int64{{1}, {1, 2}}}
	var cache listCache[int64]

	if _, err := cache.get(registry.fetch); err != nil {
		t.Fatalf("première lecture : %v", err)
	}
	found, ok, err := cache.find(registry.fetch, is(2))
	if err != nil || !ok || found != 2 {
		t.Fatalf("le partage tout neuf : (%d, %v, %v), attendu (2, true, nil)", found, ok, err)
	}
	if registry.calls != 2 {
		t.Fatalf("%d lectures du registre, attendu 2 (la mémoire, puis une seule relecture)", registry.calls)
	}
}

// Un identifiant connu ne coûte rien : la mémoire répond.
func TestAKnownIDIsServedFromMemory(t *testing.T) {
	registry := &changingRegistry{versions: [][]int64{{1, 2}}}
	var cache listCache[int64]

	for attempt := 0; attempt < 3; attempt++ {
		if _, ok, _ := cache.find(registry.fetch, is(2)); !ok {
			t.Fatalf("essai %d : l'identifiant connu n'est pas trouvé", attempt)
		}
	}
	if registry.calls != 1 {
		t.Fatalf("%d lectures du registre, attendu 1", registry.calls)
	}
}

// Un identifiant qui n'existe vraiment pas coûte UNE relecture, pas une
// boucle : le refus reste rapide, et la réponse est « inconnu ».
func TestATrulyUnknownIDCostsOneRefreshOnly(t *testing.T) {
	registry := &changingRegistry{versions: [][]int64{{1}}}
	var cache listCache[int64]

	_, ok, err := cache.find(registry.fetch, is(9))
	if err != nil || ok {
		t.Fatalf("identifiant inexistant : (%v, %v), attendu (false, nil)", ok, err)
	}
	if registry.calls != 2 {
		t.Fatalf("%d lectures du registre, attendu 2", registry.calls)
	}
}
