package facade

import (
	"context"
	"sync"
	"testing"
	"time"
)

// LA RECHERCHE NE VIT QUE TANT QU'UNE PAGE Y EST ABONNÉE : le premier abonné
// l'ouvre, le second la partage, le dernier qui part la ferme ; Ask ne sert
// qu'à une recherche ouverte.
func TestTheSearchLivesAsLongAsItsSubscribers(t *testing.T) {
	var (
		mu     sync.Mutex
		opened int
		closed int
		asked  int
		report func([]DiscoveredServer)
	)
	discovery := &Discovery{Search: func(ctx context.Context, found func([]DiscoveredServer)) func() {
		mu.Lock()
		opened++
		report = found
		mu.Unlock()
		go func() {
			<-ctx.Done()
			mu.Lock()
			closed++
			mu.Unlock()
		}()

		return func() {
			mu.Lock()
			asked++
			mu.Unlock()
		}
	}}
	count := func() (int, int, int) {
		mu.Lock()
		defer mu.Unlock()

		return opened, closed, asked
	}
	settle := func(want func() bool) {
		t.Helper()
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if want() {
				return
			}
		}
		t.Fatal("état attendu jamais atteint")
	}

	if discovery.Ask() {
		t.Fatal("Ask sans recherche ouverte")
	}
	firstWake, firstLeave := discovery.join()
	_, secondLeave := discovery.join()
	settle(func() bool { opened, _, _ := count(); return opened == 1 && discovery.Ask() })

	mu.Lock()
	report([]DiscoveredServer{{Name: "Gamebox", Link: "http://192.168.1.10:11200", Address: "192.168.1.10"}})
	mu.Unlock()
	select {
	case <-firstWake:
	case <-time.After(time.Second):
		t.Fatal("aucun réveil à l'arrivée d'un serveur")
	}
	if list := discovery.List(); len(list) != 1 || list[0].Name != "Gamebox" {
		t.Fatalf("liste : %+v", list)
	}

	firstLeave()
	if _, closed, _ := count(); closed != 0 {
		t.Fatal("la recherche s'est fermée avec un abonné encore là")
	}
	secondLeave()
	settle(func() bool { _, closed, _ := count(); return closed == 1 })
	if len(discovery.List()) != 0 || discovery.Ask() {
		t.Fatal("la recherche vit sans abonné")
	}
	if opened, _, asked := count(); opened != 1 || asked != 1 {
		t.Fatalf("%d ouvertures, %d questions", opened, asked)
	}
}
