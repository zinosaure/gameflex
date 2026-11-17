package api

import (
	"path/filepath"
	"testing"
	"time"
)

// La fin d'une lecture d'empreintes réveille les pages abonnées (#751) — elles ne relisent plus la liste toutes les
// quatre secondes. Une lecture qui échoue réveille aussi : la page redemande, et l'erreur se dit.
func TestGameKeysReadWakesTheWatchers(t *testing.T) {
	wake, stop := watchGameKeys()
	defer stop()

	warmGameKeys(filepath.Join(t.TempDir(), "absent"), "nes")

	select {
	case <-wake:
	case <-time.After(5 * time.Second):
		t.Fatal("la lecture finie n'a réveillé personne")
	}
}

// Un abonné parti n'est plus réveillé, et rien ne bloque.
func TestGameKeysWatcherLeaves(t *testing.T) {
	wake, stop := watchGameKeys()
	stop()

	warmGameKeys(filepath.Join(t.TempDir(), "absent"), "nes")
	time.Sleep(200 * time.Millisecond)
	select {
	case <-wake:
		t.Fatal("un abonné parti a été réveillé")
	default:
	}
}
