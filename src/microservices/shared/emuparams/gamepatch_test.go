package emuparams

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const renderHeight = "Environment > KYTY_UE_RENDER_HEIGHT"

var kyty = Ref{ConsoleID: "ps5", Emulator: "kyty"}

// writeGamePatch — patches/<console>/<jeu>/settings.json sous `dir`.
func writeGamePatch(t *testing.T, dir, consoleID, game, system string, values map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(gamePatch{Name: game, System: system, Values: values})
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, consoleID, game)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, gamePatchFile), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
}

// L'ORDRE : la fiche, le moteur, le correctif. Pour ce jeu seul, et il gagne ;
// supprimé, il ne pose plus rien.
func TestAGamePatchLaysItsValuesOverTheEngine(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(shadps4, "PlayStation 4", nil, map[string]any{"dynarec": true}, "",
		[]EngineSetting{{Engine: "unity", Preset: "translator-safest", Enabled: true}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	const game = "CUSA07010"
	writeGamePatch(t, filepath.Join(store.Root, "patches"), "ps4", game, "shadPS4", map[string]any{"dynarec": false})

	if settings := store.SettingsFor(shadps4, "unity", "CUSA00003"); settings.Game != "" {
		t.Fatalf("un autre jeu : %+v", settings)
	}
	settings := store.SettingsFor(shadps4, "unity", game)
	if got := valueAt(settings, dynarec); got != "false" {
		t.Fatalf("ce jeu : %s = %q, attendu false", dynarec, got)
	}
	if settings.Game != game || settings.EnginePreset != "translator-safest" {
		t.Fatalf("ce qui est dit : %+v", settings)
	}

	if err := os.RemoveAll(filepath.Join(store.Root, "patches", "ps4", game)); err != nil {
		t.Fatal(err)
	}
	if got := store.SettingsFor(shadps4, "", game); got.Game != "" || valueAt(got, dynarec) != "true" {
		t.Fatalf("supprimé : %+v", got)
	}
}

// CE QU'UN CORRECTIF NE POSE PAS RESTE À LA FICHE : Wukong suit la hauteur qu'on
// y choisit, Tekken 8 garde la sienne.
func TestAGamePatchLeavesTheRestToTheForm(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	patches := filepath.Join(store.Root, "patches")
	writeGamePatch(t, patches, "ps5", "PPSA23226", "KytyPS5", map[string]any{"renderBase": "1080"})
	writeGamePatch(t, patches, "ps5", "PPSA10594", "KytyPS5", map[string]any{"renderHeight": "720", "renderBase": "1080"})

	// Sans fiche enregistrée (la dev), c'est le lanceur qui décide : 540p.
	home := t.TempDir()
	if report := Apply("kyty", store.SettingsFor(kyty, "unreal", "PPSA23226"), home, 720, ""); report.Error != "" {
		t.Fatalf("Apply : %s", report.Error)
	}
	found := false
	for _, line := range Environment("kyty", home) {
		found = found || line == "KYTY_UE_RENDER_HEIGHT=540"
	}
	if !found {
		t.Fatalf("Wukong, sans fiche enregistrée : %v", Environment("kyty", home))
	}

	if _, err := store.Save(kyty, "PlayStation 5", nil, map[string]any{"renderHeight": "360"}, "", nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	wukong := store.SettingsFor(kyty, "unreal", "PPSA23226")
	if got := valueAt(wukong, renderHeight); got != "360" {
		t.Fatalf("Wukong : la hauteur = %q, attendu celle de la fiche, 360", got)
	}
	if got := valueAt(wukong, "Environment > KYTY_UE_RENDER_BASE"); got != "1080" {
		t.Fatalf("Wukong : la base = %q", got)
	}
	if got := valueAt(store.SettingsFor(kyty, "unreal", "PPSA10594"), renderHeight); got != "720" {
		t.Fatalf("Tekken 8 : la hauteur = %q, attendu la sienne, 720", got)
	}
	if got := valueAt(store.SettingsFor(kyty, "unreal", "PPSA01911"), renderHeight); got != "360" {
		t.Fatalf("un jeu sans correctif : la hauteur = %q", got)
	}
}

// LA BIBLIOTHÈQUE, PUIS LES DISQUES. Un correctif sur un disque compte ; celui de
// la bibliothèque passe avant.
func TestAGamePatchIsLookedForInTheLibraryThenOnTheDisks(t *testing.T) {
	disk := t.TempDir()
	store := Store{Root: t.TempDir(), PatchDirs: []string{filepath.Join(t.TempDir(), "absent"), disk}}
	const game = "PPSA23226"
	writeGamePatch(t, disk, "ps5", game, "KytyPS5", map[string]any{"renderHeight": "720"})
	if got := valueAt(store.SettingsFor(kyty, "unreal", game), renderHeight); got != "720" {
		t.Fatalf("sur le disque : la hauteur = %q", got)
	}
	writeGamePatch(t, filepath.Join(store.Root, "patches"), "ps5", game, "KytyPS5", map[string]any{"renderHeight": "360"})
	if got := valueAt(store.SettingsFor(kyty, "unreal", game), renderHeight); got != "360" {
		t.Fatalf("dans la bibliothèque : la hauteur = %q", got)
	}
}

// UN CORRECTIF ILLISIBLE SE DIT, ET LA PARTIE PART : les valeurs de la fiche et du
// moteur restent. Celui d'un autre émulateur ne le concerne pas ; un dossier de
// correctif sans réglages non plus. Un identifiant hors forme n'est même pas cherché.
func TestABadGamePatchDoesNotStopTheGame(t *testing.T) {
	store := Store{Root: t.TempDir()}
	patches := filepath.Join(store.Root, "patches")
	const game = "PPSA23226"
	folder := filepath.Join(patches, "ps5", game)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := store.SettingsFor(kyty, "unreal", game); got.Game != "" || got.GameError != "" {
		t.Fatalf("un dossier sans réglages : %+v", got)
	}
	if err := os.WriteFile(filepath.Join(folder, gamePatchFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings := store.SettingsFor(kyty, "unreal", game)
	if settings.GameError == "" || settings.Game != "" || settings.State == "error" {
		t.Fatalf("illisible : %+v", settings)
	}
	if !strings.Contains(settings.GameError, game) {
		t.Fatalf("l'erreur ne nomme pas le jeu : %s", settings.GameError)
	}

	writeGamePatch(t, patches, "ps5", game, "shadPS4", map[string]any{"renderHeight": "720"})
	if got := store.SettingsFor(kyty, "unreal", game); got.Game != "" || got.GameError != "" {
		t.Fatalf("d'un autre émulateur : %+v", got)
	}

	writeGamePatch(t, filepath.Join(store.Root, ".."), "ps5", "etc", "KytyPS5", map[string]any{"renderHeight": "720"})
	if got := store.SettingsFor(kyty, "unreal", "../../etc"); got.Game != "" || got.GameError != "" {
		t.Fatalf("hors forme : %+v", got)
	}
}

// RIEN NE S'ÉCRIT AU LANCEMENT, et aucun panneau n'a besoin d'avoir été ouvert.
func TestReadingAGamePatchWritesNothing(t *testing.T) {
	store := Store{Root: t.TempDir(), PatchDirs: []string{t.TempDir()}}
	writeGamePatch(t, store.PatchDirs[0], "ps5", "PPSA10594", "KytyPS5", map[string]any{"renderHeight": "720"})
	if got := store.SettingsFor(kyty, "unreal", "PPSA10594"); got.Game != "PPSA10594" {
		t.Fatalf("Tekken 8 : %+v", got)
	}
	if entries, _ := os.ReadDir(store.Root); len(entries) != 0 {
		t.Fatalf("le lancement a écrit dans la bibliothèque : %v", entries)
	}
}

// DRIVECLUB, DE BOUT EN BOUT : son correctif arrive dans le config.json de shadPS4,
// en nombre, comme shadPS4 le lit. Sans lui, un monde noir.
func TestAGamePatchReachesShadps4Config(t *testing.T) {
	home := t.TempDir()
	store := Store{Root: t.TempDir()}
	writeGamePatch(t, filepath.Join(store.Root, "patches"), "ps4", "CUSA00003", "shadPS4", map[string]any{"readbacksMode": "1"})
	settings := store.SettingsFor(shadps4, "", "CUSA00003")
	if settings.Game != "CUSA00003" {
		t.Fatalf("Driveclub : %+v", settings)
	}
	if report := Apply("shadps4", settings, filepath.Join(home, ".config"), 720, ""); report.Error != "" {
		t.Fatalf("Apply : %s", report.Error)
	}
	content, err := os.ReadFile(filepath.Join(home, ".local", "share", "shadPS4", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		GPU map[string]any `json:"GPU"`
	}
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatal(err)
	}
	if got := config.GPU["readbacks_mode"]; got != float64(1) {
		t.Fatalf("GPU.readbacks_mode = %#v, attendu le nombre 1\n%s", got, content)
	}
}

// TEKKEN 8, DE BOUT EN BOUT : ce que son correctif pose arrive dans l'environnement
// de Kyty, tel quel, et les valeurs de la fiche restent.
func TestAGamePatchReachesKytyEnvironment(t *testing.T) {
	home := t.TempDir()
	store := Store{Root: t.TempDir()}
	writeGamePatch(t, filepath.Join(store.Root, "patches"), "ps5", "PPSA10594", "KytyPS5", map[string]any{
		"renderHeight":   "720",
		"renderBase":     "1080",
		"renderVariable": "r.SecondaryScreenPercentage.GameViewport",
	})
	settings := store.SettingsFor(kyty, "unreal", "PPSA10594")
	if report := Apply("kyty", settings, home, 720, ""); report.Error != "" {
		t.Fatalf("Apply : %s", report.Error)
	}
	got := map[string]bool{}
	for _, line := range Environment("kyty", home) {
		got[line] = true
	}
	for _, want := range []string{
		"KYTY_UE_RENDER_HEIGHT=720",
		"KYTY_UE_RENDER_BASE=1080",
		"KYTY_UE_RENDER_VARIABLE=r.SecondaryScreenPercentage.GameViewport",
		"KYTY_UNIFIED_MEMORY=1",
	} {
		if !got[want] {
			t.Fatalf("%s manque : %v", want, got)
		}
	}
}

// CLAIR OBSCUR, L'AVANCE SUR LE GPU (#654, Kyty 0097) : son correctif la relève à 8, un
// autre jeu garde celle de la fiche, 1. Une fiche enregistrée avant ce paramètre ne pose
// pas la valeur du correctif (layValues ne connaît que ses paramètres) tant qu'un Ensure
// (completeLocked, la page des réglages) ne l'y a pas ajouté.
func TestAGamePatchRaisesKytySuspendPointsAhead(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(kyty, "PlayStation 5", nil, map[string]any{"occlusionQueries": true}, "", nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	writeGamePatch(t, filepath.Join(store.Root, "patches"), "ps5", "PPSA17599", "KytyPS5",
		map[string]any{"suspendPointsAhead": "8"})
	const ahead = "Environment > KYTY_SUSPEND_POINTS_AHEAD"

	// La fiche d'avant ce paramètre.
	content, err := os.ReadFile(store.emulatorFile(kyty))
	if err != nil {
		t.Fatal(err)
	}
	var old map[string]any
	if err := json.Unmarshal(content, &old); err != nil {
		t.Fatal(err)
	}
	delete(old["params"].(map[string]any), "suspendPointsAhead")
	encoded, _ := json.Marshal(old)
	if err := os.WriteFile(store.emulatorFile(kyty), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := valueAt(store.SettingsFor(kyty, "unreal", "PPSA17599"), ahead); got != "" {
		t.Fatalf("fiche d'avant : %s = %q, attendu rien", ahead, got)
	}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if got := valueAt(store.SettingsFor(kyty, "unreal", "PPSA17599"), ahead); got != "8" {
		t.Fatalf("Clair Obscur : %s = %q, attendu 8", ahead, got)
	}
	if got := valueAt(store.SettingsFor(kyty, "unreal", "PPSA10594"), ahead); got != "1" {
		t.Fatalf("un autre jeu : %s = %q, attendu 1", ahead, got)
	}
	home := t.TempDir()
	if report := Apply("kyty", store.SettingsFor(kyty, "unreal", "PPSA17599"), home, 540, ""); report.Error != "" {
		t.Fatalf("Apply : %s", report.Error)
	}
	found := false
	for _, line := range Environment("kyty", home) {
		found = found || line == "KYTY_SUSPEND_POINTS_AHEAD=8"
	}
	if !found {
		t.Fatalf("KYTY_SUSPEND_POINTS_AHEAD=8 manque : %v", Environment("kyty", home))
	}
}

// LES PRÉRÉGLAGES PAR JEU NE SONT PLUS LIVRÉS, ET CE QU'UNE VERSION PRÉCÉDENTE AVAIT
// POSÉ S'EN VA AU DÉMARRAGE (pas de rétro compatibilité) : la liste n'a plus aucune
// ligne par jeu.
func TestNoPresetIsLeftForAGame(t *testing.T) {
	store := Store{Root: t.TempDir()}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	retired := filepath.Join(store.Root, "params", "ps5", "presets", "game-tekken-8.json")
	content := `{"name": "Tekken 8", "override": false, "system": "KytyPS5", "values": {"renderHeight": "720"}}`
	if err := os.WriteFile(retired, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(retired); !os.IsNotExist(err) {
		t.Fatalf("le préréglage d'une version précédente est resté : %v", err)
	}
	for _, ref := range Shipped() {
		for _, item := range store.Describe(ref, "").Presets {
			if strings.HasPrefix(item.ID, "game-") {
				t.Fatalf("%s : %s", ref.Emulator, item.ID)
			}
		}
	}
}
