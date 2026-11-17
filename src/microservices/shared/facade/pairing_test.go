package facade

import (
	"testing"
	"time"
)

// LE SUJET `pairing` : un membre voit son mode compte et ses codes, rien
// d'autre ; un administrateur voit en plus le mode serveur et les codes
// `federation` (chantier 60).
func TestPairingViewIsTheShareOfItsViewer(t *testing.T) {
	var pairing Pairing
	raw := []byte(`{"type":"pairing",
		"server":{"open_until":"2026-11-17T14:05:00Z","opened_by":"claude"},
		"accounts":[{"username":"marc","open_until":"2026-11-17T14:04:00Z"},{"username":"claude","open_until":"2026-11-17T14:03:00Z"}],
		"codes":[{"id":1,"kind":"federation","author":"claude","expires_at":"x"},{"id":2,"kind":"account","author":"marc","expires_at":"x"},{"id":3,"kind":"federation","author":"root","expires_at":"x"}],
		"ended":[{"id":4,"kind":"account","author":"lea","reason":"used","at":"x"},{"id":5,"kind":"account","author":"marc","reason":"expired","at":"x"}],
		"closed":[{"scope":"account","username":"lea","reason":"too_many_failures","at":"x"},{"scope":"server","username":"claude","reason":"owner","at":"x"}]}`)
	wake, stop := pairing.Watch()
	defer stop()
	if err := pairing.Set(raw); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("aucun réveil")
	}

	member := pairing.Frame().View("marc", false)
	if member.Server != nil || member.Account == nil || member.Account.OpenUntil != "2026-11-17T14:04:00Z" ||
		len(member.Codes) != 1 || member.Codes[0].ID != 2 || len(member.Ended) != 1 || member.Ended[0].ID != 5 || len(member.Closed) != 0 {
		t.Fatalf("la part d'un membre : %+v", member)
	}

	admin := pairing.Frame().View("claude", true)
	if admin.Server == nil || admin.Account == nil || len(admin.Codes) != 2 || len(admin.Ended) != 0 || len(admin.Closed) != 1 || admin.Closed[0].Scope != "server" {
		t.Fatalf("la part d'un administrateur : %+v", admin)
	}

	nobody := pairing.Frame().View("lea", false)
	if nobody.Account != nil || len(nobody.Codes) != 0 || len(nobody.Ended) != 1 || len(nobody.Closed) != 1 {
		t.Fatalf("la part d'un membre sans mode : %+v", nobody)
	}
}
