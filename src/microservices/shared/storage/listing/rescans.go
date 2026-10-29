package listing

import (
	"sort"
	"sync"
)

// Rescans — quels parcours tournent, et comment le dernier s'est terminé.
// L'interface a besoin des deux : « ça travaille » n'est pas « ça a marché ».
//
// UNE FOIS pour les deux kernels : le serveur et le téléphone en tenaient
// chacun une copie, et leurs résumés avaient déjà divergé (le téléphone
// comptait encore les BIOS que #153 a retirés des montages).
//
// Et il le DIT sans qu'on demande : chaque début et chaque fin réveillent les
// abonnés — le sujet `mount/<id>` du centre de messages —, là où la page
// relisait le résumé toutes les 4 s tant qu'un parcours tournait.
type Rescans struct {
	mu          sync.Mutex
	running     map[string]bool
	failed      map[string]string
	subscribers map[chan struct{}]struct{}
}

// Start inscrit un parcours, ou dit qu'il en tourne déjà un : deux en
// parallèle doubleraient les requêtes réseau pour écrire le même fichier.
func (t *Rescans) Start(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running == nil {
		t.running, t.failed = map[string]bool{}, map[string]string{}
	}
	if t.running[key] {
		return false
	}
	t.running[key] = true
	delete(t.failed, key)
	t.wakeLocked()

	return true
}

// Finish clôt un parcours — APRÈS l'écriture du catalogue, pour que les
// abonnés réveillés lisent déjà le nouveau.
func (t *Rescans) Finish(key string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.running, key)
	if err != nil {
		t.failed[key] = err.Error()
	}
	t.wakeLocked()
}

// State — le parcours tourne-t-il, et pourquoi le dernier a-t-il échoué.
func (t *Rescans) State(key string) (running bool, failure string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.running[key], t.failed[key]
}

// Subscribe rend un signal réveillé à chaque début et chaque fin, de
// n'importe quelle ressource : l'abonné relit la sienne, et une valeur
// identique ne repart pas (facade.PushOnChange).
func (t *Rescans) Subscribe() chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.subscribers == nil {
		t.subscribers = map[chan struct{}]struct{}{}
	}
	signal := make(chan struct{}, 1)
	t.subscribers[signal] = struct{}{}

	return signal
}

// Unsubscribe retire un signal rendu par Subscribe.
func (t *Rescans) Unsubscribe(signal chan struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.subscribers, signal)
}

// wakeLocked — sans jamais attendre : un abonné qui a déjà un réveil en
// attente relira de toute façon l'état le plus récent.
func (t *Rescans) wakeLocked() {
	for signal := range t.subscribers {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
}

// Summary — ce que l'écran d'une ressource affiche avant qu'on ouvre quoi que
// ce soit : l'a-t-on parcourue, quand, combien de jeux par console, et un
// parcours tourne-t-il. La même réponse sur les deux kernels, par
// construction.
//
// Les jeux seulement : une ressource ne fournit ni cœurs ni BIOS (#153).
func Summary(cacheDir, key string, scanning bool, failure string) map[string]any {
	header, err := LoadHeader(cacheDir, key)
	if err != nil {
		// Jamais parcourue : ce n'est pas une panne, c'est la première fois.
		return map[string]any{"scanned": false, "scanning": scanning, "error": failure}
	}

	// Dans un ordre fixe : le sujet ne repart que si la valeur change, et
	// l'ordre d'une table Go change à chaque lecture.
	roms := header.Sections[Roms].Consoles
	ids := make([]string, 0, len(roms))
	for console := range roms {
		ids = append(ids, console)
	}
	sort.Strings(ids)
	consoles := make([]map[string]any, 0, len(ids))
	for _, console := range ids {
		consoles = append(consoles, map[string]any{"id": console, "roms": roms[console]})
	}

	return map[string]any{
		"scanned":   true,
		"scanning":  scanning,
		"error":     failure,
		"scannedAt": header.UpdatedAt,
		"consoles":  consoles,
	}
}
