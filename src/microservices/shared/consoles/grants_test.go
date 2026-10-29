package consoles

import "testing"

// LE DROIT « RETRO » OUVRE TOUT LE RÉTRO, ET RIEN D'AUTRE. Une console de
// bureau ou de Remote Play se donne une par une.
func TestRetroOpensEveryRetroConsoleAndNothingElse(t *testing.T) {
	grants := []string{GrantRetro}
	for _, id := range []string{"nes", "ps1", "psp", "dreamcast"} {
		if !Allows(grants, id) {
			t.Errorf("retro devrait ouvrir %s", id)
		}
	}
	for _, id := range []string{"ps3", "switch", "rp-ps5"} {
		if Allows(grants, id) {
			t.Errorf("retro ne devrait pas ouvrir %s", id)
		}
	}
}

// TOUT, C'EST TOUT — consoles à venir comprises. Rien, c'est rien : le
// compte ne joue nulle part, son jeton n'ouvre que le catalogue.
func TestAllAndNothing(t *testing.T) {
	if !Allows([]string{GrantAll}, "ps5") || !Allows([]string{GrantAll}, "rp-ps4") || !Allows([]string{GrantAll}, "nes") {
		t.Fatal("* devrait tout ouvrir")
	}
	if Allows(nil, "nes") || AllowsAny(nil) || AllowsAny(SplitGrants("")) {
		t.Fatal("aucun droit ne devrait rien ouvrir")
	}
}

// UNE CONSOLE DE BUREAU OU DE REMOTE PLAY SE DONNE SEULE.
func TestAStreamexOrRemotePlayConsoleIsGivenAlone(t *testing.T) {
	grants := SplitGrants("ps3,rp-ps5")
	if !Allows(grants, "ps3") || !Allows(grants, "rp-ps5") {
		t.Fatal("ps3 et rp-ps5 sont accordées")
	}
	if Allows(grants, "switch") || Allows(grants, "rp-ps4") || Allows(grants, "nes") {
		t.Fatal("seules ps3 et rp-ps5 sont accordées")
	}
}

// LA LISTE RANGÉE EST STABLE ; un droit inconnu, ou une console du rétro donnée
// seule, est refusé ; `*` l'emporte.
func TestParseGrants(t *testing.T) {
	got, err := ParseGrants([]string{"switch", " ps3 ", "retro", "ps3", ""})
	if err != nil || got != "ps3,retro,switch" {
		t.Fatalf("ParseGrants = %q / %v, attendu \"ps3,retro,switch\"", got, err)
	}
	if got, err := ParseGrants([]string{"ps3", GrantAll}); err != nil || got != GrantAll {
		t.Fatalf("avec * : %q / %v", got, err)
	}
	if got, err := ParseGrants(nil); err != nil || got != "" {
		t.Fatalf("rien : %q / %v", got, err)
	}
	for _, bad := range []string{"ps33", "nes", "keytest"} {
		if _, err := ParseGrants([]string{bad}); err == nil {
			t.Errorf("%q devrait être refusé", bad)
		}
	}
}
