package consoles

import "testing"

func TestStreamSize(t *testing.T) {
	cases := []struct {
		label         string
		width, height int
		ok            bool
	}{
		{"540P", 960, 540, true},
		{" 720p ", 1280, 720, true},
		{"1080p", 1920, 1080, true},
		{"360p", 0, 0, false}, // retiré le 2026-10-26
		{"480p", 0, 0, false}, // retiré le 2026-09-20
		{"", 0, 0, false},
	}
	for _, c := range cases {
		width, height, ok := StreamSize(c.label)
		if width != c.width || height != c.height || ok != c.ok {
			t.Errorf("StreamSize(%q) = %d, %d, %v ; attendu %d, %d, %v",
				c.label, width, height, ok, c.width, c.height, c.ok)
		}
	}
}

func TestStreamBounds(t *testing.T) {
	for fps, want := range map[int]int{0: 0, 14: 0, 15: 15, 30: 30, 60: 60, 61: 0} {
		if got := StreamFPS(fps); got != want {
			t.Errorf("StreamFPS(%d) = %d ; attendu %d", fps, got, want)
		}
	}
	for kbps, want := range map[int]int{0: 0, 499: 0, 500: 500, 8000: 8000, 20000: 20000, 20001: 0} {
		if got := StreamBitrate(kbps); got != want {
			t.Errorf("StreamBitrate(%d) = %d ; attendu %d", kbps, got, want)
		}
	}
}

// LA LANGUE D'UN LANCEMENT : un code de l'application passe, avec ou sans
// région ; une langue inconnue ne passe pas — la console garde la sienne.
func TestLanguage(t *testing.T) {
	cases := map[string]string{"fr": "fr", "FR": "fr", " en ": "en", "fr-FR": "fr", "en-US": "en", "de": "", "": "", "français": ""}
	for raw, want := range cases {
		if got := Language(raw); got != want {
			t.Errorf("Language(%q) = %q, attendu %q", raw, got, want)
		}
	}
}

// LES CONSOLES QUI PRÉVIENNENT AVANT LA PARTIE (D9) : le registre le dit, et
// avec un avertissement que la page connaît. Le jeu à distance (`rp-ps4`,
// `rp-ps5`) est du vrai matériel : rien à dire.
func TestExperimentalConsolesSayWhy(t *testing.T) {
	want := map[string]string{
		"ps4": ExperimentalGeneral, "ps5": ExperimentalGeneral, "xbox360": ExperimentalGeneral,
		"switch": ExperimentalHeavy, "wiiu": ExperimentalFirmware,
	}
	for _, console := range List() {
		if console.Experimental != want[console.ID] {
			t.Errorf("%s : experimental = %q, attendu %q", console.ID, console.Experimental, want[console.ID])
		}
	}
}
