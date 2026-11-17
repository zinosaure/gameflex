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

// SANS CHOIX, 540p À 30, POUR TOUTES LES CONSOLES (le porteur, 2026-11-16 :
// « La valeur par défaut ça toujours été 540p@30 pour tout le stream settings
// de toutes les consoles »). Avant, la définition partait vide et la cadence à
// zéro : le conteneur rétro gardait sa taille d'époque à 60 images, streamex
// son 720p à 60. Un débit absent reste zéro (le repli de l'encodeur).
func TestNormalizeLaunchSettingsDefaultsTo540pAt30(t *testing.T) {
	for _, sent := range []LaunchSettings{{}, {Resolution: "480p", FPS: 240}, {Resolution: "4k", FPS: 5}} {
		got := normalizeLaunchSettings(sent)
		if got.Resolution != "960x540" || got.FPS != 30 || got.Bitrate != 0 {
			t.Errorf("normalizeLaunchSettings(%+v) = %+v ; attendu 960x540 à 30, débit 0", sent, got)
		}
	}
	// Un débit mesuré par la page (« Auto ») passe comme un débit choisi.
	if got := normalizeLaunchSettings(LaunchSettings{Bitrate: 7300}); got.Bitrate != 7300 {
		t.Errorf("débit mesuré : %+v", got)
	}
}
