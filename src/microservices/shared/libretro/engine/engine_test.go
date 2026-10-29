package engine

import "testing"

// Les deux moteurs de vibration arrivent séparément ; le second appel rend les
// deux ensemble, pas seulement celui qui vient de changer (repris du serveur).
func TestRumbleCombinesBothMotors(t *testing.T) {
	var gotPort int
	var gotWeak, gotStrong uint16
	e := &Engine{config: Config{OnRumble: func(port int, weak, strong uint16) {
		gotPort, gotWeak, gotStrong = port, weak, strong
	}}}

	// RETRO_RUMBLE_STRONG = 0, RETRO_RUMBLE_WEAK = 1.
	if !e.rumbleState(1, 0, 5000) {
		t.Fatal("STRONG refusé")
	}
	if gotPort != 1 || gotStrong != 5000 || gotWeak != 0 {
		t.Fatalf("après STRONG : port %d faible %d fort %d", gotPort, gotWeak, gotStrong)
	}
	if !e.rumbleState(1, 1, 3000) {
		t.Fatal("WEAK refusé")
	}
	if gotStrong != 5000 || gotWeak != 3000 {
		t.Fatalf("après WEAK : faible %d fort %d, attendu 3000 et 5000", gotWeak, gotStrong)
	}
}

func TestRumbleRejects(t *testing.T) {
	e := &Engine{}
	if e.rumbleState(MaxPorts, 0, 1000) {
		t.Error("un port hors limites est accepté")
	}
	if e.rumbleState(0, 99, 1000) {
		t.Error("un effet inconnu est accepté")
	}
}

// LA CADENCE DU FLUX : la part des images qui partent, régulièrement espacées
// et la première tout de suite (2026-10-11).
func TestPaceFrameKeepsTheChosenShare(t *testing.T) {
	for _, cas := range []struct {
		name         string
		ratio        float64
		frames, kept int
	}{
		{"toutes", 1, 600, 600},
		{"30 sur 60", 0.5, 600, 300},
		{"50 sur 60", 50.0 / 60, 600, 500},
		{"30 sur 50 (PAL)", 0.6, 500, 300},
	} {
		credit := 1 - cas.ratio
		kept, run, longest := 0, 0, 0
		for i := 0; i < cas.frames; i++ {
			if paceFrame(&credit, cas.ratio) {
				if i == 0 && kept != 0 {
					t.Fatalf("%s : compte faux", cas.name)
				}
				kept++
				run++
				if run > longest {
					longest = run
				}
			} else {
				if i == 0 {
					t.Errorf("%s : la première image ne part pas", cas.name)
				}
				run = 0
			}
		}
		if kept != cas.kept {
			t.Errorf("%s : %d images sur %d, attendu %d", cas.name, kept, cas.frames, cas.kept)
		}
		// À 30 sur 60, jamais deux images de suite : sinon la cadence
		// moyenne est juste mais l'image saute par paquets.
		if cas.ratio == 0.5 && longest > 1 {
			t.Errorf("%s : %d images de suite", cas.name, longest)
		}
	}
}
