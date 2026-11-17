package listing

import (
	"testing"
	"time"
)

// LA DATE DU LISTING SE TIENT EN MÉMOIRE (journal 629) : elle avance avec ce
// qui change le listing, ne recule jamais, et se lit sans rien relire.
func TestFreshnessAdvancesWithWhatChangesTheListing(t *testing.T) {
	var freshness Freshness
	if _, known := freshness.UpdatedAt(nil); known {
		t.Fatal("une date sans en-tête lu")
	}

	written := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)
	freshness.Self(written)
	freshness.Self(written.Add(-time.Hour))
	if at, known := freshness.UpdatedAt(nil); !known || !at.Equal(written) {
		t.Fatalf("l'en-tête = %v %v, attendu %v (la date ne recule pas)", at, known, written)
	}

	// Le démarrage : les disques vus pour la première fois ne la font pas
	// avancer ; le scan de leurs jeux, donné de mémoire, si.
	freshness.Disks(map[string]string{"gamebox": "/mnt/gamebox"})
	scannedAt := written.Add(time.Minute)
	scans := map[string]time.Time{"gamebox": scannedAt}
	scanned := func(name, dir string) time.Time {
		if dir != "/mnt/"+name {
			t.Fatalf("le scan de %s demandé sous %s", name, dir)
		}

		return scans[name]
	}
	if at, _ := freshness.UpdatedAt(scanned); !at.Equal(scannedAt) {
		t.Fatalf("après le scan d'un disque = %v, attendu %v", at, scannedAt)
	}

	// Ses sections relues : leur dernier changement.
	changed := written.Add(2 * time.Minute)
	freshness.Disk("gamebox", changed)
	freshness.Disk("inconnu", changed.Add(time.Hour))
	if at, _ := freshness.UpdatedAt(scanned); !at.Equal(changed) {
		t.Fatalf("après ses sections = %v, attendu %v (un disque inconnu ne compte pas)", at, changed)
	}

	// Les mêmes disques regardés à nouveau : rien n'avance.
	freshness.Disks(map[string]string{"gamebox": "/mnt/gamebox"})
	if at, _ := freshness.UpdatedAt(scanned); !at.Equal(changed) {
		t.Fatalf("les mêmes disques = %v, attendu %v", at, changed)
	}

	// Un disque parti : maintenant.
	before := time.Now().UTC()
	freshness.Disks(map[string]string{})
	at, _ := freshness.UpdatedAt(scanned)
	if at.Before(before) {
		t.Fatalf("un disque parti = %v, attendu au moins %v", at, before)
	}

	// Un disque revenu : maintenant, à nouveau ; ses sections d'avant ne
	// reviennent pas avec lui.
	time.Sleep(time.Millisecond)
	freshness.Disks(map[string]string{"gamebox": "/mnt/gamebox"})
	if again, _ := freshness.UpdatedAt(nil); !again.After(at) {
		t.Fatalf("un disque revenu = %v, attendu après %v", again, at)
	}
}
