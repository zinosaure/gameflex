package docker

import (
	"slices"
	"testing"

	"gameflex/consoles"
	"gameflex/kernel/internal/config"
)

// CHAQUE IMAGE PORTE UN ÉMULATEUR DU REGISTRE, dans l'ordre du registre, et
// chaque console streamex du registre a au moins une image : un nom écrit
// autrement ici qu'au registre ferait griser, dans la fiche, l'émulateur même
// que l'image fait tourner — et un ordre différent changerait le défaut.
func TestEachStreamexImageRunsADeclaredCore(t *testing.T) {
	for consoleID := range streamexEmulators {
		running := StreamexCores(consoleID)
		declared := consoles.CoresFor(consoleID)
		at := -1
		for _, core := range running {
			index := slices.Index(declared, core)
			if index < 0 {
				t.Errorf("%s : « %s » n'est pas un cœur du registre (%v)", consoleID, core, declared)
			}
			if index < at {
				t.Errorf("%s : « %s » n'est pas dans l'ordre du registre (%v)", consoleID, core, declared)
			}
			at = index
		}
	}
	for _, console := range consoles.All() {
		// La Xbox One est déclarée sans émulateur : rien à faire tourner,
		// donc pas d'image.
		if console.Method != consoles.MethodStreamex || len(console.Cores) == 0 {
			continue
		}
		if len(StreamexCores(console.ID)) == 0 {
			t.Errorf("%s est une console streamex sans émulateur d'image", console.ID)
		}
	}
}

// LA SWITCH A DEUX IMAGES (chantier 26) : Eden par défaut (#438), Citron quand
// la fiche le choisit — chacun avec ses paramètres, son image et l'endroit où ses
// données se montent ; un émulateur qu'aucune image ne porte n'a pas d'image.
func TestTheSwitchRunsEdenFirstAndCitronOnRequest(t *testing.T) {
	cfg := config.Config{SwitchImage: "streamex-switch:latest", SwitchEdenImage: "streamex-switch-eden:latest", PS3Image: "streamex-ps3:latest"}
	cases := []struct {
		core, params, image, data string
	}{
		{"", "eden", cfg.SwitchEdenImage, "/root/.local/share/eden"},
		{"Citron (streamex)", "citron", cfg.SwitchImage, "/root/.local/share/citron"},
		{"Eden (streamex)", "eden", cfg.SwitchEdenImage, "/root/.local/share/eden"},
	}
	for _, c := range cases {
		if params, _ := StreamexParams("switch", c.core); params != c.params {
			t.Errorf("« %s » : paramètres %q, attendu %q", c.core, params, c.params)
		}
		if image := sessionImage(cfg, "switch", c.core); image != c.image {
			t.Errorf("« %s » : image %q, attendu %q", c.core, image, c.image)
		}
		if data := switchDataTarget(c.core); data != c.data {
			t.Errorf("« %s » : données sur %q, attendu %q", c.core, data, c.data)
		}
	}
	if image := sessionImage(cfg, "switch", "Yuzu (streamex)"); image != "" {
		t.Errorf("un émulateur sans image en a une : %q", image)
	}
	if image := sessionImage(cfg, "ps3", ""); image != cfg.PS3Image {
		t.Errorf("ps3 : %q", image)
	}
}

// UNE IMAGE DIT L'ÉMULATEUR QU'ELLE ATTEND (chantier 29) : le motif de ses
// versions, et la version testée — son nom et son adresse chez l'éditeur —,
// par trois étiquettes. Sans étiquette — keytest, qui n'a pas d'émulateur —,
// elle n'attend rien.
func TestAnImageSaysWhichEmulatorFileItWants(t *testing.T) {
	need, ok := requiredFromLabels("switch", "Eden (streamex)", map[string]string{
		labelEmulatorFile:    "Eden-Linux-de7e6e01d4-amd64-clang-pgo.AppImage",
		labelEmulatorURL:     "https://nightly.eden-emu.dev/eden.AppImage",
		labelEmulatorPattern: "Eden-Linux-*-amd64-clang-pgo.AppImage",
		labelEmulatorTested:  "Eden-Linux-0000000000-amd64-clang-pgo.AppImage Eden-Linux-de7e6e01d4-amd64-clang-pgo.AppImage",
	})
	if !ok || need.Console != "switch" || need.File != "Eden-Linux-de7e6e01d4-amd64-clang-pgo.AppImage" ||
		need.URL != "https://nightly.eden-emu.dev/eden.AppImage" || need.Core != "Eden (streamex)" ||
		need.Pattern != "Eden-Linux-*-amd64-clang-pgo.AppImage" || len(need.Tested) != 2 {
		t.Fatalf("étiquettes lues : %+v %v", need, ok)
	}
	if need, ok := requiredFromLabels("keytest", "", map[string]string{"gameflex.prune": "keep"}); ok {
		t.Fatalf("une image sans étiquette n'attend rien : %+v", need)
	}
}
