package consoles

import (
	"fmt"
	"testing"
)

func TestStreamSize(t *testing.T) {
	cases := []struct {
		label         string
		width, height int
		ok            bool
	}{
		{"540P", 960, 540, true},
		{" 720p ", 1280, 720, true},
		{"1080p", 1920, 1080, true},
		{"360p", 640, 360, true}, // revenu le 2026-11-06 (#636)
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

// LE DÉFAUT, 540p À 30, POUR TOUTES LES CONSOLES (le porteur, 2026-11-16) : un
// libellé vide ou hors de l'échelle prend 540p, une cadence absente ou hors des
// bornes prend 30. Avant, ils rendaient « rien », et chaque appelant mettait
// le sien (720p à 60, la taille d'époque à 60).
func TestStreamDefaultIs540pAt30(t *testing.T) {
	for label, want := range map[string]string{"": "540p", "480p": "540p", "4k": "540p", " 720P ": "720p", "360p": "360p", "1080p": "1080p"} {
		if got := StreamResolution(label); got != want {
			t.Errorf("StreamResolution(%q) = %q ; attendu %q", label, got, want)
		}
	}
	if width, height, ok := StreamSize(StreamResolution("")); width != 960 || height != 540 || !ok {
		t.Errorf("la taille du défaut = %dx%d (%v) ; attendu 960x540", width, height, ok)
	}
	if got := StreamFPS(0); got != 30 {
		t.Errorf("StreamFPS(0) = %d ; attendu 30", got)
	}
}

func TestStreamBounds(t *testing.T) {
	for fps, want := range map[int]int{0: 30, 14: 30, 15: 15, 30: 30, 60: 60, 61: 30} {
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
		"switch": ExperimentalHeavy, "wiiu": ExperimentalFirmware, "switch2": ExperimentalGeneral,
	}
	for _, console := range List() {
		if console.Experimental != want[console.ID] {
			t.Errorf("%s : experimental = %q, attendu %q", console.ID, console.Experimental, want[console.ID])
		}
	}
}

// La règle du porteur (chantier 58) : seulement sous le flux, jamais sous 540p,
// jamais au-dessus de ce que l'émulateur rend.
func TestTierHeightsOnlyBelowTheStream(t *testing.T) {
	cases := []struct {
		stream, rendered int
		want             []int
	}{
		{1080, 0, []int{720, 540}},
		{1080, 1080, []int{720, 540}},
		{1080, 2160, []int{720, 540}},
		{720, 0, []int{540}},
		{540, 0, nil},
		{360, 0, nil},
		{1080, 540, []int{540}},
		{1080, 810, []int{720, 540}},
		{720, 480, nil},
	}
	for _, c := range cases {
		got := TierHeights(c.stream, c.rendered)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("TierHeights(%d, %d) = %v, want %v", c.stream, c.rendered, got, c.want)
		}
	}
}

func TestStreamTiersWidthAndBitrate(t *testing.T) {
	got := fmt.Sprint(StreamTiers(1920, 1080, 8000, 0))
	if got != "[{1280 720 3555} {960 540 2000}]" {
		t.Errorf("1080p à 8000 : %s", got)
	}
	if got := fmt.Sprint(StreamTiers(1280, 720, 0, 0)); got != "[{960 540 0}]" {
		t.Errorf("720p sans débit : %s", got)
	}
}
