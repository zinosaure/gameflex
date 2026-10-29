package emuparams

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// L'INI D'EDEN : chaque clé à SA section, avec son `\default=false` — sans
// lui Eden ignore la valeur —, et retirée avec lui.
func TestTheQtINIWriterKeepsTheDefaultMarker(t *testing.T) {
	lines := []string{
		"[UI]", `confirmStop\default=false`, "confirmStop=2", "",
		"[Renderer]", `backend\default=true`, "backend=0", `resolution_setup\default=true`, "resolution_setup=3", "",
		"[System]", `use_docked_mode\default=true`, "use_docked_mode=1",
	}
	set := func(section, key, value string) bool {
		var ok bool
		lines, ok = setQtINIKey(lines, section, key, value)

		return ok
	}

	if !set("Renderer", "resolution_setup", "1") {
		t.Fatal("refusé")
	}
	renderer := indexOf(lines, "[Renderer]")
	if lines[renderer+1] != `resolution_setup\default=false` || lines[renderer+2] != "resolution_setup=1" {
		t.Fatalf("la valeur et son marqueur, en tête de la section : %q", lines[renderer:renderer+3])
	}
	if indexOf(lines, "resolution_setup=3") >= 0 || indexOf(lines, `resolution_setup\default=true`) >= 0 {
		t.Fatal("l'ancienne valeur est restée")
	}
	if !set("Core", "use_multi_core", "true") || lines[len(lines)-3] != "[Core]" || lines[len(lines)-1] != "use_multi_core=true" {
		t.Fatalf("section absente, créée en fin de fichier : %q", lines[len(lines)-3:])
	}
	if !set("Audio", "output_device", "a, b") || indexOf(lines, `output_device="a, b"`) < 0 {
		t.Fatal("une virgule ferait une liste pour QSettings : entre guillemets")
	}
	if set("Renderer", `bad\key`, "1") || set("Re]nderer", "x", "1") || set("Renderer", "a=b", "1") {
		t.Fatal("un nom qui changerait le sens de la ligne a été accepté")
	}

	var removed bool
	if lines, removed = removeQtINIKey(lines, "Renderer", "resolution_setup"); !removed {
		t.Fatal("retrait")
	}
	if indexOf(lines, "resolution_setup=1") >= 0 || indexOf(lines, `resolution_setup\default=false`) >= 0 {
		t.Fatal("le retrait doit emporter les deux lignes")
	}
	if _, removed = removeQtINIKey(lines, "UI", "resolution_setup"); removed {
		t.Fatal("une clé d'une autre section a été retirée")
	}
}

// LE TOML DE XEMU : une table pointée, une valeur remplacée en place, une
// chaîne entre guillemets, un nombre et un booléen tels quels.
func TestTheTOMLWriterFindsTheTable(t *testing.T) {
	lines := []string{
		"[general]", "show_welcome = false", "",
		"[display]", "renderer = 'OPENGL'", "",
		"[display.quality]", "surface_scale = 1", "",
		"[sys.files]", "bootrom_path = '/bios/mcpx_1.0.bin'",
	}
	set := func(section, key, value string) bool {
		var ok bool
		lines, ok = setTOMLKey(lines, section, key, tomlValue(value))

		return ok
	}

	if !set("display.quality", "surface_scale", "2") || indexOf(lines, "surface_scale = 2") != 7 {
		t.Fatalf("remplacement en place : %q", lines)
	}
	if !set("display", "renderer", "VULKAN") || indexOf(lines, `renderer = "VULKAN"`) != 4 {
		t.Fatal("une chaîne, entre guillemets, à sa place")
	}
	if !set("display", "vsync", "true") || lines[4] != "vsync = true" {
		t.Fatalf("clé absente, ajoutée en tête de sa table : %q", lines[3:6])
	}
	if !set("audio.vp", "num_workers", "2") || lines[len(lines)-2] != "[audio.vp]" || lines[len(lines)-1] != "num_workers = 2" {
		t.Fatal("table absente, créée en fin de fichier")
	}
	if set("display", "quality.surface_scale", "3") || set("display", "a b", "1") {
		t.Fatal("une clé pointée ou à espace écrirait ailleurs que là où on croit")
	}
	if indexOf(lines, "[display.quality]") < 0 || indexOf(lines, "[display]") < 0 {
		t.Fatal("une table a disparu")
	}

	var removed bool
	if lines, removed = removeTOMLKey(lines, "display.quality", "surface_scale"); !removed || indexOf(lines, "surface_scale = 2") >= 0 {
		t.Fatal("retrait")
	}
	if _, removed = removeTOMLKey(lines, "general", "renderer"); removed {
		t.Fatal("une clé d'une autre table a été retirée")
	}
	if tomlValue(`a"b`) != `"a\"b"` || tomlValue("-1.5") != "-1.5" || tomlValue("false") != "false" {
		t.Fatal("les valeurs TOML")
	}
}

// LE XML DE CEMU : un élément dans son conteneur, un conteneur créé s'il
// manque, et les commentaires du gabarit — qui CITENT des balises — laissés
// tranquilles.
func TestTheXMLWriterSkipsCommentsAndKeepsContainers(t *testing.T) {
	lines := []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		"<content>",
		"    <fullscreen>true</fullscreen>",
		"    <GraphicPack/>",
		"    <Graphic>",
		"        <api>1</api>",
		"        <AsyncCompile>true</AsyncCompile>",
		"    </Graphic>",
		"    <Audio>",
		"        <!-- un <TVDevice> vide ne vaut pas",
		"        <TVDevice>exemple</TVDevice> dans un commentaire -->",
		"        <TVDevice>default</TVDevice>",
		"        <TVVolume>100</TVVolume>",
		"    </Audio>",
		"</content>",
	}
	set := func(section, key, value string) bool {
		var ok bool
		lines, ok = setXMLKey(lines, section, key, value)

		return ok
	}

	if !set("Graphic", "AsyncCompile", "false") || lines[6] != "        <AsyncCompile>false</AsyncCompile>" {
		t.Fatalf("remplacement en place : %q", lines[6])
	}
	if !set("Graphic", "vkAccurateBarriers", "false") || lines[7] != "        <vkAccurateBarriers>false</vkAccurateBarriers>" || lines[8] != "    </Graphic>" {
		t.Fatalf("élément absent, ajouté au bout de son conteneur : %q", lines[5:9])
	}
	if !set("Audio", "TVDevice", "x&y") {
		t.Fatal("refusé")
	}
	if lines[11] != "        <TVDevice>exemple</TVDevice> dans un commentaire -->" || indexOf(lines, "        <TVDevice>x&amp;y</TVDevice>") != 12 {
		t.Fatalf("le commentaire a été touché, ou la vraie valeur pas échappée : %q", lines[9:13])
	}
	if !set("content", "fullscreen", "false") || lines[2] != "    <fullscreen>false</fullscreen>" {
		t.Fatal("un enfant direct de la racine, par son nom")
	}
	if !set("Input", "api", "2") || lines[len(lines)-4] != "    <Input>" || lines[len(lines)-3] != "        <api>2</api>" || lines[len(lines)-2] != "    </Input>" {
		t.Fatalf("conteneur absent, créé avant la fin de la racine : %q", lines[len(lines)-5:])
	}
	if indexOf(lines, "        <api>1</api>") < 0 {
		t.Fatal("l'« api » d'un autre conteneur a été changée")
	}
	if set("GraphicPack", "x", "1") || set("Graphic > api", "x", "1") || set("Graphic", "bad name", "1") {
		t.Fatal("un élément vide, une valeur ou un nom invalide n'est pas un conteneur")
	}

	var removed bool
	if lines, removed = removeXMLKey(lines, "Graphic", "vkAccurateBarriers"); !removed || indexOf(lines, "        <vkAccurateBarriers>false</vkAccurateBarriers>") >= 0 {
		t.Fatal("retrait")
	}
	if _, removed = removeXMLKey(lines, "Audio", "AsyncCompile"); removed {
		t.Fatal("un élément d'un autre conteneur a été retiré")
	}
}

// LA RÉSOLUTION D'EDEN suit le flux, en tenant compte du mode portable que le
// fichier porte déjà — même table que le téléphone et l'entrypoint. Ses
// numéros ne sont pas ceux de Citron, qu'il remplace : ×1,25 vient juste après
// ×1, et il n'y a pas de ×1,75.
func TestEdenResolutionFollowsTheStream(t *testing.T) {
	// 1440 : ×1,25 (1350) ; 2160 : ×2 ; 3240 : ×3.
	for height, want := range map[int]int{0: 3, 360: 1, 540: 1, 720: 2, 1080: 3, 1440: 4, 1620: 5, 2160: 6, 3240: 7} {
		if got := EdenResolution(height, true); got != want {
			t.Errorf("docké, %d lignes : %d, attendu %d", height, got, want)
		}
	}
	if got := EdenResolution(720, false); got != 3 {
		t.Errorf("portable, 720 lignes : %d, attendu ×1 (3)", got)
	}
	handheld := []string{"[System]", `use_docked_mode\default=false`, "use_docked_mode=0"}
	if value, ok := edenAuto("Renderer > resolution_setup", 1080, handheld); !ok || value != "5" {
		t.Errorf("auto en mode portable, 1080 lignes : %q, attendu ×1,5 (5)", value)
	}
	if _, ok := edenAuto("Renderer > use_vsync", 720, nil); ok {
		t.Error("auto n'a de sens que pour la résolution")
	}
	home := filepath.Join(t.TempDir(), ".config")
	report := Apply("eden", Settings{Emulator: "eden", State: "enabled", Entries: []Entry{
		{Config: "Renderer > resolution_setup", Value: "auto"},
	}}, home, 1440, "")
	ini, _ := os.ReadFile(filepath.Join(home, "eden", "qt-config.ini"))
	if report.Error != "" || report.NYI || !strings.Contains(string(ini), "resolution_setup=4") {
		t.Fatalf("Eden : %+v\n%s", report, ini)
	}
}

// LE MODE DE LA CONSOLE CHANGE AVEC LA RÉSOLUTION (#439) : la résolution
// d'Eden est déclarée avant le mode, mais « auto » lit le mode de CETTE partie,
// pas celui que le fichier gardait de la précédente — dans les deux sens.
func TestEdenAutoResolutionReadsTheModeOfThisLaunch(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".config")
	launch := func(mode string) string {
		Apply("eden", Settings{Emulator: "eden", State: "enabled", Entries: []Entry{
			{Config: "Renderer > resolution_setup", Value: "auto"},
			{Config: "System > use_docked_mode", Value: mode},
		}}, home, 540, "")
		ini, _ := os.ReadFile(filepath.Join(home, "eden", "qt-config.ini"))

		return string(ini)
	}
	if ini := launch("1"); !strings.Contains(ini, "resolution_setup=1") {
		t.Fatalf("docké, 540 lignes : attendu ×0,5 (1)\n%s", ini)
	}
	if ini := launch("0"); !strings.Contains(ini, "resolution_setup=2") {
		t.Fatalf("passé au mode portable, 540 lignes : attendu ×0,75 (2)\n%s", ini)
	}
	if ini := launch("1"); !strings.Contains(ini, "resolution_setup=1") {
		t.Fatalf("revenu au mode docké, 540 lignes : attendu ×0,5 (1)\n%s", ini)
	}
}

// APPLY, D'UN FORMAT À L'AUTRE : la même logique — écrire, rendre au
// défaut, le lanceur jamais retiré —, chacun dans son fichier.
func TestApplyWritesEachFormatInItsFile(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".config")

	report := Apply("eden", Settings{State: "enabled", Entries: []Entry{
		{Config: "Renderer > use_asynchronous_shaders", Value: "true"},
		{Config: "Renderer > resolution_setup", Value: "auto"},
	}}, home, 540, "")
	ini, _ := os.ReadFile(filepath.Join(home, "eden", "qt-config.ini"))
	if report.Error != "" || !strings.Contains(string(ini), "use_asynchronous_shaders=true") || !strings.Contains(string(ini), "resolution_setup=1") {
		t.Fatalf("eden : %+v\n%s", report, ini)
	}
	report = Apply("eden", Settings{State: "disabled", Entries: []Entry{}}, home, 1080, "")
	ini, _ = os.ReadFile(filepath.Join(home, "eden", "qt-config.ini"))
	if strings.Contains(string(ini), "use_asynchronous_shaders") || !strings.Contains(string(ini), "resolution_setup=3") {
		t.Fatalf("désactivé : les shaders rendus à Eden, la résolution au lanceur : %+v\n%s", report, ini)
	}

	data := filepath.Join(filepath.Dir(home), ".local", "share", "xemu", "xemu")
	_ = os.MkdirAll(data, 0o755)
	_ = os.WriteFile(filepath.Join(data, "xemu.toml"), []byte("[general]\nshow_welcome = false\n"), 0o644)
	report = Apply("xemu", Settings{State: "enabled", Entries: []Entry{{Config: "display.quality > surface_scale", Value: "2"}}}, home, 540, "")
	toml, _ := os.ReadFile(filepath.Join(data, "xemu.toml"))
	if report.Error != "" || string(toml) != "[general]\nshow_welcome = false\n\n[display.quality]\nsurface_scale = 2\n" {
		t.Fatalf("xemu, dans le dossier de DONNÉES : %+v\n%q", report, toml)
	}

	settings := filepath.Join(home, "Cemu", "settings.xml")
	_ = os.MkdirAll(filepath.Dir(settings), 0o755)
	_ = os.WriteFile(settings, []byte("<?xml version=\"1.0\"?>\n<content>\n\t<Graphic>\n\t\t<api>1</api>\n\t</Graphic>\n</content>\n"), 0o644)
	report = Apply("cemu", Settings{State: "enabled", Entries: []Entry{{Config: "Graphic > AsyncCompile", Value: "true"}}}, home, 540, "")
	xml, _ := os.ReadFile(settings)
	if report.Error != "" || !strings.Contains(string(xml), "\t\t<api>1</api>\n\t\t<AsyncCompile>true</AsyncCompile>\n\t</Graphic>") {
		t.Fatalf("cemu : %+v\n%s", report, xml)
	}
}
