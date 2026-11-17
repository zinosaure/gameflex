package facade

// LE SUJET `discovery` (chantier 60) — les serveurs GameFlex trouvés sur le
// réseau local, ceux qui sont en mode découverte. Les deux kernels y branchent
// leur recherche : la passerelle du serveur, qui est en réseau hôte ; mDNS en
// direct au téléphone (gameflex/lan).
//
// LA RECHERCHE NE VIT QUE TANT QU'UNE PAGE Y EST ABONNÉE : le premier abonné
// l'ouvre, le dernier qui part la ferme. Elle pose une question à son ouverture
// et n'en pose plus d'elle-même ; `Ask` (la commande `discovery.search`, le
// geste « Chercher de nouveau ») en repose une. Un serveur qui ouvre son mode
// s'annonce lui-même : une page déjà ouverte le voit arriver sans rien
// redemander.

import (
	"context"
	"sync"
)

// DiscoveredServer — un serveur trouvé, tel que gameflex/lan le décrit.
type DiscoveredServer struct {
	Instance string `json:"instance"`
	Name     string `json:"name"`
	Link     string `json:"link"`
	Address  string `json:"address"`
}

// Discovery — la recherche d'un kernel, partagée par ses abonnés.
type Discovery struct {
	// Search ouvre la recherche jusqu'à la fin de ctx : `found` reçoit la liste
	// entière à chaque changement ; `ask` repose la question une fois.
	Search func(ctx context.Context, found func([]DiscoveredServer)) (ask func())

	mu          sync.Mutex
	subscribers int
	cancel      context.CancelFunc
	ask         func()
	list        []DiscoveredServer
	watchers    map[chan struct{}]struct{}
}

func (discovery *Discovery) wakeLocked() {
	for watcher := range discovery.watchers {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}
}

func (discovery *Discovery) found(list []DiscoveredServer) {
	discovery.mu.Lock()
	defer discovery.mu.Unlock()

	if discovery.cancel == nil {
		return
	}
	discovery.list = append([]DiscoveredServer{}, list...)
	discovery.wakeLocked()
}

// join — un abonné de plus ; le premier ouvre la recherche.
func (discovery *Discovery) join() (<-chan struct{}, func()) {
	discovery.mu.Lock()
	defer discovery.mu.Unlock()

	if discovery.watchers == nil {
		discovery.watchers = map[chan struct{}]struct{}{}
	}
	watcher := make(chan struct{}, 1)
	discovery.watchers[watcher] = struct{}{}
	discovery.subscribers++
	if discovery.subscribers == 1 && discovery.Search != nil {
		ctx, cancel := context.WithCancel(context.Background())
		discovery.cancel = cancel
		discovery.list = nil
		// Search peut rappeler `found` aussitôt : la recherche part hors du verrou.
		go func() {
			ask := discovery.Search(ctx, discovery.found)
			discovery.mu.Lock()
			defer discovery.mu.Unlock()
			if ctx.Err() == nil {
				discovery.ask = ask
			}
		}()
	}

	return watcher, func() {
		discovery.mu.Lock()
		defer discovery.mu.Unlock()
		delete(discovery.watchers, watcher)
		discovery.subscribers--
		if discovery.subscribers == 0 && discovery.cancel != nil {
			discovery.cancel()
			discovery.cancel, discovery.ask, discovery.list = nil, nil, nil
		}
	}
}

// List — ce qui est trouvé maintenant.
func (discovery *Discovery) List() []DiscoveredServer {
	discovery.mu.Lock()
	defer discovery.mu.Unlock()

	return append([]DiscoveredServer{}, discovery.list...)
}

// Ask — reposer la question, une fois. Faux si aucune recherche n'est ouverte.
func (discovery *Discovery) Ask() bool {
	discovery.mu.Lock()
	ask := discovery.ask
	discovery.mu.Unlock()
	if ask == nil {
		return false
	}
	ask()

	return true
}

// Follow — le sujet `discovery` d'une connexion : la liste tout de suite, puis
// à chaque changement, tant qu'elle y est abonnée.
func (discovery *Discovery) Follow(ctx context.Context, conn *Conn) {
	wake, leave := discovery.join()
	defer leave()

	PushOnChange(ctx, conn, Push{Type: "discovery", Topic: "discovery"}, wake, func() any {
		return map[string]any{"servers": discovery.List()}
	})
}
