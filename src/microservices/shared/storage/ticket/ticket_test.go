package ticket

import (
	"testing"
	"time"
)

// UN TICKET NE SERT QU'UNE FOIS — la propriété pour laquelle le mécanisme
// existe, et qui le distingue d'une signature rejouable.
func TestUnTicketNeSeConsommeQuUneFois(t *testing.T) {
	var store Store
	nonce, err := store.Mint("nes", "abc-123")
	if err != nil {
		t.Fatal(err)
	}
	if console, ref, ok := store.Consume(nonce); !ok || console != "nes" || ref != "abc-123" {
		t.Fatalf("premier usage = (%q, %q, %v)", console, ref, ok)
	}
	if _, _, ok := store.Consume(nonce); ok {
		t.Error("le MÊME ticket a été accepté deux fois")
	}
}

// Un ticket inconnu, vide ou presque bon : le même refus, sans distinction.
func TestUnTicketInconnuEstRefuse(t *testing.T) {
	var store Store
	for _, nonce := range []string{"un-ticket-qui-n-existe-pas", "", "AAAA"} {
		if _, _, ok := store.Consume(nonce); ok {
			t.Errorf("%q accepté", nonce)
		}
	}
}

// L'EXPIRATION COMPTE — et le ticket expiré est quand même consommé.
func TestUnTicketExpireNeSertPlus(t *testing.T) {
	now := time.Now()
	store := Store{now: func() time.Time { return now }}
	nonce, err := store.Mint("ps1", "def-456")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(DefaultTTL + time.Second)
	if _, _, ok := store.Consume(nonce); ok {
		t.Error("un ticket expiré a été accepté")
	}
	if _, remains := store.byNonce[nonce]; remains {
		t.Error("le ticket expiré est resté en mémoire")
	}
}

// Chaque ticket ne rend que SON jeu.
func TestChaqueTicketNeRendQueSonJeu(t *testing.T) {
	var store Store
	first, _ := store.Mint("nes", "mario")
	second, _ := store.Mint("psp", "crisis-core")
	if first == second {
		t.Fatal("deux tickets identiques")
	}
	if console, ref, _ := store.Consume(second); console != "psp" || ref != "crisis-core" {
		t.Errorf("second = (%q, %q)", console, ref)
	}
	if console, ref, _ := store.Consume(first); console != "nes" || ref != "mario" {
		t.Errorf("premier = (%q, %q)", console, ref)
	}
}
