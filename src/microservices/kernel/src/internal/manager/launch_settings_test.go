package manager

import "testing"

// LA LANGUE DE LA PAGE ARRIVE À LA PARTIE (chantier 21) : une langue de
// l'application passe, ramenée à son code ; une autre est retirée, et la
// console garde la sienne.
func TestNormalizeLaunchSettingsKeepsTheLanguage(t *testing.T) {
	cases := map[string]string{"fr": "fr", "fr-FR": "fr", "en": "en", "de": "", "": ""}
	for sent, want := range cases {
		if got := normalizeLaunchSettings(LaunchSettings{Language: sent}).Language; got != want {
			t.Errorf("langue %q → %q, attendu %q", sent, got, want)
		}
	}
	// Le reste du panneau ne bouge pas.
	settings := normalizeLaunchSettings(LaunchSettings{Resolution: "720p", FPS: 30, Bitrate: 4000, Language: "fr"})
	if settings.Resolution != "1280x720" || settings.FPS != 30 || settings.Bitrate != 4000 || settings.Language != "fr" {
		t.Errorf("réglages : %+v", settings)
	}
}
