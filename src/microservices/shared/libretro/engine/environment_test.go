package engine

import "testing"

// La valeur par défaut d'une option, telle que le cœur la déclare : celle que
// GET_VARIABLE rend, comme RetroArch et LibretroDroid.
func TestDeclaredDefault(t *testing.T) {
	cases := map[string]string{
		"GFX Plugin; gliden64|glide64|gln64|rice|angrylion|parallel": "gliden64",
		"Region; Auto|NTSC|PAL|Dendy":                                "Auto",
		"Master Volume; 7|0|1|2":                                     "7",
	}
	for declaration, want := range cases {
		if got, ok := declaredDefault(declaration); !ok || got != want {
			t.Fatalf("%q : %q (%v), attendu %q", declaration, got, ok, want)
		}
	}
	for _, broken := range []string{"", "Pas de liste", "Vide; "} {
		if got, ok := declaredDefault(broken); ok {
			t.Fatalf("%q ne déclare rien, et rend %q", broken, got)
		}
	}
}

// La résolution interne au plus près du flux (#340), sur les déclarations
// relevées le 2026-10-12 en chargeant chaque cœur du serveur — telles quelles.
func TestRenderResolutionFollowsTheStream(t *testing.T) {
	declared := map[string]string{
		"ppsspp_internal_resolution":           "Rendering Resolution; 480x272|960x544|1440x816|1920x1088|2400x1360|2880x1632|3360x1904|3840x2176|4320x2448|4800x2720",
		"parallel-n64-screensize":              "Resolution; 640x480|320x240|960x720|1280x960|1440x1080|1600x1200|1920x1440|2240x1680|2880x2160|5760x4320",
		"reicast_internal_resolution":          "Internal Resolution; 640x480|320x240|800x600|960x720|1024x768|1280x960|1440x1080|1600x1200|1920x1440|2560x1920",
		"desmume_internal_resolution":          "Internal Resolution; 256x192|512x384|768x576|1024x768|1280x960|1536x1152|1792x1344|2048x1536|2304x1728|2560x1920",
		"dolphin_efb_scale":                    "Graphics > Settings > Internal Resolution; 1|2|3|4|5|6",
		"pcsx2_upscale_multiplier":             "Video > Internal Resolution (Restart); 1x Native (PS2)|2x Native (~720p)|3x Native (~1080p)|4x Native (~1440p/2K)|5x Native (~1800p/3K)|6x Native (~2160p/4K)",
		"kronos_resolution_mode":               "Resolution; original|2X|4X|8X",
		"pcsx_rearmed_neon_enhancement_enable": "(GPU) Enhanced Resolution; disabled|enabled",
	}
	// Pour chaque clé : ce que doivent donner 360p, 540p, 720p et 1080p.
	want := map[string][4]string{
		"ppsspp_internal_resolution":           {"480x272", "960x544", "1440x816", "1920x1088"},
		"parallel-n64-screensize":              {"320x240", "640x480", "960x720", "1440x1080"},
		"reicast_internal_resolution":          {"320x240", "640x480", "960x720", "1440x1080"},
		"desmume_internal_resolution":          {"256x192", "256x192", "512x384", "768x576"}, // deux écrans
		"dolphin_efb_scale":                    {"1", "1", "1", "2"},
		"pcsx2_upscale_multiplier":             {"1x Native (PS2)", "1x Native (PS2)", "2x Native (~720p)", "3x Native (~1080p)"},
		"kronos_resolution_mode":               {"original", "2X", "2X", "4X"},
		"pcsx_rearmed_neon_enhancement_enable": {"disabled", "enabled", "enabled", "enabled"},
	}
	for key, values := range want {
		option, known := internalResolutions[key]
		if !known {
			t.Fatalf("%s n'est pas dans internalResolutions", key)
		}
		for i, height := range []int{360, 540, 720, 1080} {
			if got := renderResolution(option, declared[key], height); got != values[i] {
				t.Errorf("%s en %dp : %q, attendu %q", key, height, got, values[i])
			}
		}
	}

	// Rien à lire, rien de choisi : le cœur garde son défaut.
	for _, broken := range []string{"", "Résolution", "Résolution; Auto|Native"} {
		if got := renderResolution(resolutionOption{}, broken, 540); got != "" {
			t.Errorf("%q : %q, attendu rien", broken, got)
		}
	}
	if got := renderResolution(resolutionOption{}, declared["ppsspp_internal_resolution"], 0); got != "" {
		t.Errorf("sans hauteur de flux : %q, attendu rien", got)
	}
}
