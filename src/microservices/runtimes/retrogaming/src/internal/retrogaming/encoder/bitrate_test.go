package encoder

import "testing"

// LE DÉBIT SUIT L'IMAGE. Une valeur ronde ne peut être juste que pour une
// seule taille — et ce n'était pas celle des consoles qui en demandent le
// plus (signalé le 2026-09-05 sur Crash Team Racing, en 640×480).
func TestAutoBitrateFollowsTheFrame(t *testing.T) {
	for _, cas := range []struct {
		nom      string
		w, h     int
		fps      float64
		min, max int
		pourquoi string
	}{
		{"NES 256×240@60", 256, 240, 60, autoBitrateFloor, autoBitrateFloor,
			"petite image : le plancher suffit, et rien ne devient pire qu'avant"},
		{"PS1 640×480@50", 640, 480, 50, 3000, 3200,
			"c'est le cas signalé : plus de débit qu'avant, sans excès"},
		{"PS1 640×480@60", 640, 480, 60, 3600, 3800,
			"la même en 60 images, un peu plus"},
	} {
		got := AutoBitrate(cas.w, cas.h, cas.fps)
		if got < cas.min || got > cas.max {
			t.Fatalf("%s → %dk, attendu entre %d et %d (%s)",
				cas.nom, got, cas.min, cas.max, cas.pourquoi)
		}
	}
}

// JAMAIS PIRE QU'AVANT : l'ancienne valeur fixe est devenue le plancher, donc
// ce changement ne peut qu'améliorer.
func TestAutoBitrateNeverGoesBelowTheOldFixedValue(t *testing.T) {
	for _, cas := range [][3]int{{256, 240, 60}, {160, 144, 60}, {1, 1, 1}} {
		if got := AutoBitrate(cas[0], cas[1], float64(cas[2])); got < autoBitrateFloor {
			t.Fatalf("%dx%d@%d → %dk, sous le plancher %dk",
				cas[0], cas[1], cas[2], got, autoBitrateFloor)
		}
	}
}

// Et jamais au-delà de ce qu'un réseau suit — le tampon VBV en dépend.
func TestAutoBitrateIsCapped(t *testing.T) {
	if got := AutoBitrate(1920, 1080, 60); got != autoBitrateCeiling {
		t.Fatalf("1920x1080@60 → %dk, attendu le plafond %dk", got, autoBitrateCeiling)
	}
}

// Une géométrie absurde ne doit pas produire un débit absurde : on retombe
// sur le plancher plutôt que sur zéro, qui ferait taire l'encodeur.
func TestAutoBitrateSurvivesNonsense(t *testing.T) {
	for _, cas := range [][3]int{{0, 480, 60}, {640, 0, 60}, {640, 480, 0}} {
		if got := AutoBitrate(cas[0], cas[1], float64(cas[2])); got != autoBitrateFloor {
			t.Fatalf("%v → %dk, attendu le plancher", cas, got)
		}
	}
}
