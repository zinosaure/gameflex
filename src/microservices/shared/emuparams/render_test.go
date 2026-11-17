package emuparams

import (
	"os"
	"path/filepath"
	"testing"
)

// La hauteur rendue se lit dans les réglages écrits (chantier 58) : celle
// qu'Apply pose pour un flux, et un paramètre de l'utilisateur par-dessus.
func TestRenderHeightReadsTheSettingsInForce(t *testing.T) {
	for _, c := range []struct {
		emulator string
		stream   int
		want     int
	}{
		{"eden", 540, 540},
		{"eden", 720, 810},
		{"eden", 1080, 1080},
		{"citron", 1080, 1080},
		{"rpcs3", 540, 540},
		{"rpcs3", 1080, 1080},
		{"xemu", 720, 480},
		{"xemu", 1080, 960},
	} {
		home := filepath.Join(t.TempDir(), ".config")
		Apply(c.emulator, Settings{Emulator: c.emulator, State: "enabled"}, home, c.stream, "")
		if got := RenderHeight(c.emulator, home); got != c.want {
			t.Errorf("%s pour un flux de %d : %d, attendu %d", c.emulator, c.stream, got, c.want)
		}
	}
}

func TestRenderHeightSwitchHandheld(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".config")
	file := filepath.Join(home, "eden", "qt-config.ini")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("[Renderer]\nresolution_setup=3\n[System]\nuse_docked_mode=0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RenderHeight("eden", home); got != 720 {
		t.Errorf("×1 en portable : %d, attendu 720", got)
	}
}

// Kyty : la plus petite de sa sortie et de la hauteur Unreal.
func TestRenderHeightKyty(t *testing.T) {
	if got := kytyRenderHeight([]string{"KYTY_OUTPUT_RESOLUTION=1080", "KYTY_UE_RENDER_HEIGHT=540"}); got != 540 {
		t.Errorf("1080 et 540 : %d", got)
	}
	if got := kytyRenderHeight([]string{"KYTY_OUTPUT_RESOLUTION=1080"}); got != 1080 {
		t.Errorf("1080 seul : %d", got)
	}
	if got := kytyRenderHeight(nil); got != 0 {
		t.Errorf("rien : %d", got)
	}
}

// Rien ne dit la taille que rend shadPS4, Cemu ou Xenia : 0, la partie prend
// celle de son flux.
func TestRenderHeightUnknown(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".config")
	for _, emulator := range []string{"shadps4", "cemu", "xenia", "inconnu"} {
		if got := RenderHeight(emulator, home); got != 0 {
			t.Errorf("%s : %d, attendu 0", emulator, got)
		}
	}
}
