package emuparams

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// La structure d'un vrai config.yml de RPCS3 : des sections, des
// sous-sections, une liste, et des noms qui reviennent d'une section à
// l'autre.
var rpcs3Config = []string{
	"Core:", "  PPU Threads: 2", "  Max SPURS Threads: 6", "  Libraries Control:", "    []", "  HLE lwmutex: false",
	"VFS:", "  Enable /host_root/: false",
	"Video:", "  Renderer: Vulkan", "  Aspect ratio: 16:9", "  Vulkan:", `    Adapter: ""`, "    Enabled: true",
	"  Performance Overlay:", "    Enabled: false", "    Detail level: Medium", "  Shader Mode: Async Recompiler with Shader Interpreter",
	"Audio:", "  Renderer: Cubeb", "  Enable Time Stretching: false",
	"Input/Output:", `  Keyboard: "Basic"`,
	"Miscellaneous:", "  Enabled: true",
}

func indexOf(lines []string, line string) int {
	for i, candidate := range lines {
		if candidate == line {
			return i
		}
	}

	return -1
}

// L'ÉCRIVAIN YAML, repris de celui du téléphone (GlibcGuest, Java) et de ses
// 28 cas : chaque clé à SA section, une sous-section créée dans la sienne,
// une section ou une valeur jamais écrasée par une autre forme.
func TestTheYAMLWriterKeepsEachKeyInItsSection(t *testing.T) {
	lines := append([]string(nil), rpcs3Config...)
	set := func(section, key, value string) bool {
		var ok bool
		lines, ok = setYAMLKey(lines, section, key, value)

		return ok
	}

	if !set("Core", "SPU Block Size", "Mega") || lines[1] != "  SPU Block Size: Mega" {
		t.Fatalf("clé absente, ajoutée en tête de Core : %q", lines[1])
	}
	if !set("Audio", "Enable Time Stretching", "true") || indexOf(lines, "  Enable Time Stretching: true") < 0 || indexOf(lines, "  Enable Time Stretching: false") >= 0 {
		t.Fatal("remplacement en place")
	}
	if !set("Video > Performance Overlay", "Enabled", "true") || lines[indexOf(lines, "  Performance Overlay:")+1] != "    Enabled: true" {
		t.Fatal("sous-section")
	}
	if lines[indexOf(lines, "  Vulkan:")+2] != "    Enabled: true" || lines[len(lines)-1] != "  Enabled: true" {
		t.Fatal("les autres « Enabled » ont bougé")
	}
	if !set("Input/Output", "Keyboard", "Null") || indexOf(lines, "  Keyboard: Null") < 0 {
		t.Fatal("section Input/Output")
	}
	if !set("Net", "Internet enabled", "Disconnected") || lines[len(lines)-2] != "Net:" || lines[len(lines)-1] != "  Internet enabled: Disconnected" {
		t.Fatal("section absente, créée en fin de fichier")
	}
	if !set("Video > Shader Loading Dialog", "Allow custom background", "false") {
		t.Fatal("sous-section absente")
	}
	dialog := indexOf(lines, "  Shader Loading Dialog:")
	if dialog < indexOf(lines, "Video:") || dialog > indexOf(lines, "Audio:") || lines[dialog+1] != "    Allow custom background: false" {
		t.Fatal("sous-section créée hors de Video")
	}
	if set("Core", "Libraries Control", "x") || set("Video", "Vulkan", "x") {
		t.Fatal("une clé qui est une section a été remplacée")
	}
	size := len(lines)
	if set("Video > Renderer", "X", "1") || len(lines) != size {
		t.Fatal("une adresse qui traverse une valeur a doublé la clé")
	}

	var removed bool
	if lines, removed = removeYAMLKey(lines, "Core", "Max SPURS Threads"); !removed || indexOf(lines, "  Max SPURS Threads: 6") >= 0 {
		t.Fatal("retrait")
	}
	if _, removed = removeYAMLKey(lines, "Core", "Max SPURS Threads"); removed {
		t.Fatal("retrait d'une clé absente")
	}
	if _, removed = removeYAMLKey(lines, "Nope", "X"); removed {
		t.Fatal("retrait d'une section absente")
	}
	if _, removed = removeYAMLKey(lines, "Video", "Vulkan"); removed {
		t.Fatal("une section a été retirée")
	}
}

func TestYAMLScalarsReadBackAsWritten(t *testing.T) {
	for value, want := range map[string]string{
		"Recompiler (LLVM)": "Recompiler (LLVM)",
		"16:9":              "16:9",
		"4":                 "4",
		"a: b":              `"a: b"`,
		"@x":                `"@x"`,
		`say "hi" # no`:     `"say \"hi\" # no"`,
	} {
		if got := yamlScalar(value); got != want {
			t.Errorf("yamlScalar(%q) = %s, attendu %s", value, got, want)
		}
	}
}

// APPLY, de bout en bout sur un dossier de configuration : écrit, puis rendu
// à l'émulateur quand le fichier est désactivé ; « auto » devient la
// résolution du flux ; un fichier illisible ne touche à rien ; un émulateur
// sans écrivain se dit NYI.
func TestApplyWritesThenReleases(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "rpcs3", "config.yml")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	original := strings.Join(rpcs3Config, "\n") + "\n"
	if err := os.WriteFile(config, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	report := Apply("rpcs3", Settings{State: "enabled", Entries: []Entry{
		{Config: "Video > Resolution Scale", Value: "auto"},
		{Config: "Core > Max SPURS Threads", Value: "4"},
		{Config: "Video > Performance Overlay > Enabled", Value: "true"},
		{Config: "Video > Renderer > X", Value: "1"},
	}}, home, 540, "")
	content, _ := os.ReadFile(config)
	text := string(content)
	if !strings.Contains(text, "  Resolution Scale: 75\n") || !strings.Contains(text, "  Max SPURS Threads: 4\n") ||
		!strings.Contains(text, "  Performance Overlay:\n    Enabled: true\n") {
		t.Fatalf("config.yml après Apply :\n%s", text)
	}
	if len(report.Set) != 3 || len(report.Skipped) != 1 || report.Skipped[0] != "Video > Renderer > X" {
		t.Fatalf("rapport : %+v", report)
	}

	if report := Apply("rpcs3", Settings{State: "error", Error: "illisible"}, home, 540, ""); report.Error == "" {
		t.Fatal("un fichier illisible doit se dire")
	}
	if again, _ := os.ReadFile(config); string(again) != text {
		t.Fatal("un fichier illisible a touché au config.yml")
	}

	// Désactivé : ce que les paramètres avaient posé part, sauf la
	// résolution de rendu, que le lanceur règle lui-même — elle revient au
	// flux, à la hauteur de CE lancement.
	report = Apply("rpcs3", Settings{State: "disabled", Entries: []Entry{}}, home, 1080, "")
	content, _ = os.ReadFile(config)
	text = string(content)
	if strings.Contains(text, "Max SPURS Threads") || strings.Contains(text, "  Performance Overlay:\n    Enabled:") ||
		!strings.Contains(text, "  Resolution Scale: 150\n") || len(report.Released) != 3 ||
		report.Released[0] != "Video > Resolution Scale (suit le flux : 150)" {
		t.Fatalf("désactivé :\n%s\n%+v", text, report)
	}
	if _, err := os.Stat(filepath.Join(home, "rpcs3", "gameflex-params.json")); !os.IsNotExist(err) {
		t.Fatal("la trace est restée")
	}

	// Sans fichier, et quand l'entrypoint a déjà réglé ce que le lanceur
	// règle, le config.yml n'est pas réécrit.
	untouched := []byte("Core:\n  PPU Threads: 2\nVideo:\n  Resolution Scale: 75\n")
	_ = os.WriteFile(config, untouched, 0o644)
	stamp := time.Now().Add(-time.Hour)
	_ = os.Chtimes(config, stamp, stamp)
	if report := Apply("rpcs3", Settings{State: "absent", Entries: []Entry{}}, home, 540, ""); len(report.Set)+len(report.Released) != 0 {
		t.Fatalf("sans fichier : %+v", report)
	}
	if info, _ := os.Stat(config); !info.ModTime().Equal(stamp) {
		t.Fatal("sans fichier, le config.yml a été réécrit")
	}
	// Mais sans résolution réglée, le lanceur la pose : c'était le
	// comportement d'avant les paramètres, sur les deux hôtes.
	_ = os.WriteFile(config, []byte("Core:\n  PPU Threads: 2\n"), 0o644)
	Apply("rpcs3", Settings{State: "absent", Entries: []Entry{}}, home, 360, "")
	if again, _ := os.ReadFile(config); !strings.Contains(string(again), "Video:\n  Resolution Scale: 50\n") {
		t.Fatalf("la résolution du flux n'a pas été posée :\n%s", again)
	}

	if report := Apply("duckstation", Settings{State: "enabled", Entries: []Entry{{Config: "a > b", Value: "1"}}}, home, 540, ""); !report.NYI {
		t.Fatal("un émulateur sans écrivain doit se dire NYI")
	}
}

func TestRenderScaleFollowsTheStream(t *testing.T) {
	for height, want := range map[int]int{0: 100, 360: 50, 540: 75, 720: 100, 1080: 150, 100: 25} {
		if got := RenderScale(height); got != want {
			t.Errorf("RenderScale(%d) = %d, attendu %d", height, got, want)
		}
	}
}

func TestTheEnvironmentWriterSpeaksBox64(t *testing.T) {
	home := t.TempDir()
	report := Apply("kyty", Settings{State: "enabled", Entries: []Entry{
		{Config: "Environment > BOX64_DYNAREC", Value: "false"},
		{Config: "Environment > BOX64_DYNAREC_STRONGMEM", Value: "2"},
		{Config: "Environment > SDL_AUDIO_DRIVER", Value: "auto"},
		{Config: "Environment > bad name", Value: "1"},
	}}, home, 540, "")
	content, _ := os.ReadFile(filepath.Join(home, "kyty", "environment"))
	// Les deux dernières lignes sont celles du lanceur (kytyAuto, #445).
	if string(content) != "BOX64_DYNAREC=0\nBOX64_DYNAREC_STRONGMEM=2\nSDL_AUDIO_DRIVER=auto\nKYTY_OCCLUSION_QUERIES=1\nKYTY_SHADER_CACHE=1\n" {
		t.Fatalf("environment après Apply :\n%s", content)
	}
	if len(report.Set) != 3 || len(report.Skipped) != 1 || report.Skipped[0] != "Environment > bad name" {
		t.Fatalf("rapport : %+v", report)
	}
	if EnvironmentFile("kyty") != "kyty/environment" || EnvironmentFile("shadps4") != "shadPS4/environment" ||
		EnvironmentFile("rpcs3") != "rpcs3/environment" || EnvironmentFile("cemu") != "" {
		t.Fatal("EnvironmentFile ne nomme pas les bons fichiers")
	}

	// Désactivé : chaque variable posée part, rien d'autre n'est touché — les
	// lignes du lanceur restent.
	Apply("kyty", Settings{State: "enabled", Entries: []Entry{{Config: "Environment > BOX64_LOG", Value: "2"}}}, home, 540, "")
	report = Apply("kyty", Settings{State: "disabled", Entries: []Entry{}}, home, 540, "")
	content, _ = os.ReadFile(filepath.Join(home, "kyty", "environment"))
	if string(content) != "KYTY_OCCLUSION_QUERIES=1\nKYTY_SHADER_CACHE=1\n" || len(report.Released) != 1 || report.Released[0] != "Environment > BOX64_LOG" {
		t.Fatalf("désactivé :\n%s\n%+v", content, report)
	}
}

// L'ENVIRONNEMENT DE RPCS3, À CÔTÉ DE SON config.yml (#388, 2026-10-19) : un
// paramètre « Environment > NOM » va dans rpcs3/environment, jamais dans le
// YAML ; Environment rend ce que le serveur passe au processus (« auto » ne
// pose rien) ; retiré du fichier de paramètres, il part de l'environnement
// sans toucher au config.yml, et inversement.
func TestTheEnvironmentOfRpcs3LivesBesideItsConfig(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "rpcs3", "config.yml")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(strings.Join(rpcs3Config, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := Apply("rpcs3", Settings{State: "enabled", Entries: []Entry{
		{Config: "Core > Max SPURS Threads", Value: "4"},
		{Config: "Environment > TU_DEBUG", Value: "flushall"},
		{Config: "Environment > IR3_SHADER_DEBUG", Value: "auto"},
	}}, home, 540, "")
	yaml, _ := os.ReadFile(config)
	environment, _ := os.ReadFile(filepath.Join(home, "rpcs3", "environment"))
	if strings.Contains(string(yaml), "Environment") || strings.Contains(string(yaml), "TU_DEBUG") ||
		!strings.Contains(string(yaml), "  Max SPURS Threads: 4\n") {
		t.Fatalf("config.yml :\n%s", yaml)
	}
	if string(environment) != "TU_DEBUG=flushall\nIR3_SHADER_DEBUG=auto\n" || len(report.Set) != 3 {
		t.Fatalf("environment :\n%s\n%+v", environment, report)
	}
	if got := Environment("rpcs3", home); len(got) != 1 || got[0] != "TU_DEBUG=flushall" {
		t.Fatalf("Environment = %v", got)
	}
	if Environment("kyty", home) != nil || Environment("cemu", home) != nil {
		t.Fatal("Environment ne lit qu'un fichier d'environnement écrit")
	}

	// Le pilote rendu à lui-même : TU_DEBUG part, le config.yml ne bouge pas.
	report = Apply("rpcs3", Settings{State: "enabled", Entries: []Entry{
		{Config: "Core > Max SPURS Threads", Value: "4"},
	}}, home, 540, "")
	environment, _ = os.ReadFile(filepath.Join(home, "rpcs3", "environment"))
	if again, _ := os.ReadFile(config); string(again) != string(yaml) || string(environment) != "" ||
		len(report.Released) != 2 || Environment("rpcs3", home) != nil {
		t.Fatalf("retiré :\nenvironment %q\n%+v", environment, report)
	}
	if _, err := os.Stat(filepath.Join(home, "rpcs3", "gameflex-params-environment.json")); !os.IsNotExist(err) {
		t.Fatal("la trace de l'environnement est restée")
	}
	if _, err := os.Stat(filepath.Join(home, "rpcs3", "gameflex-params.json")); err != nil {
		t.Fatal("la trace du config.yml est partie avec celle de l'environnement")
	}
}

// LE CONTOURNEMENT DE TURNIP DE LA PS4 (2026-10-26) : plusieurs options,
// séparées par des virgules, arrivent telles quelles dans l'environnement de
// shadPS4 ; « auto » n'en pose aucune.
func TestTheTurnipWorkaroundOfShadps4KeepsItsCommas(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(filepath.Dir(home), ".local", "share", "shadPS4")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	Apply("shadps4", Settings{State: "enabled", Entries: []Entry{
		{Config: "Environment > TU_DEBUG", Value: "sysmem,nolrz,noubwc"},
	}}, home, 540, "")
	if got := Environment("shadps4", home); len(got) != 1 || got[0] != "TU_DEBUG=sysmem,nolrz,noubwc" {
		t.Fatalf("Environment = %v", got)
	}
	Apply("shadps4", Settings{State: "enabled", Entries: []Entry{
		{Config: "Environment > TU_DEBUG", Value: "auto"},
	}}, home, 540, "")
	if got := Environment("shadps4", home); got != nil {
		t.Fatalf("« auto » pose %v", got)
	}
}

// LA LANGUE DE L'APPLICATION (chantier 21) : posée dans la configuration de
// RPCS3, d'Eden et de Cemu à chaque lancement — et un paramètre du fichier
// qui la règle gagne. Sans langue connue, rien n'est écrit.
func TestApplySetsTheApplicationLanguage(t *testing.T) {
	home := t.TempDir()

	Apply("rpcs3", Settings{State: "absent", Entries: []Entry{}}, home, 540, "fr")
	yaml, _ := os.ReadFile(filepath.Join(home, "rpcs3", "config.yml"))
	if !strings.Contains(string(yaml), "System:\n  Language: French\n") {
		t.Fatalf("RPCS3 en français :\n%s", yaml)
	}
	// Relancée en anglais, la même configuration change de langue.
	Apply("rpcs3", Settings{State: "absent", Entries: []Entry{}}, home, 540, "en")
	yaml, _ = os.ReadFile(filepath.Join(home, "rpcs3", "config.yml"))
	if !strings.Contains(string(yaml), "  Language: English (US)\n") || strings.Contains(string(yaml), "French") {
		t.Fatalf("RPCS3 relancé en anglais :\n%s", yaml)
	}
	// Un paramètre qui règle la langue gagne sur l'application.
	Apply("rpcs3", Settings{State: "enabled", Entries: []Entry{{Config: "System > Language", Value: "Japanese"}}}, home, 540, "fr")
	yaml, _ = os.ReadFile(filepath.Join(home, "rpcs3", "config.yml"))
	if !strings.Contains(string(yaml), "  Language: Japanese\n") {
		t.Fatalf("le paramètre perdu contre la langue de l'application :\n%s", yaml)
	}

	Apply("eden", Settings{State: "absent", Entries: []Entry{}}, home, 1080, "fr")
	ini, _ := os.ReadFile(filepath.Join(home, "eden", "qt-config.ini"))
	if !strings.Contains(string(ini), "language_index=2") || !strings.Contains(string(ini), "language_index\\default=false") {
		t.Fatalf("Eden en français :\n%s", ini)
	}

	// settings.xml existe toujours avant le lancement : l'entrypoint du
	// serveur et le lanceur du téléphone le sèment. Sa racine aussi, donc.
	if err := os.MkdirAll(filepath.Join(home, "Cemu"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "Cemu", "settings.xml"), []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<content>\n</content>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	Apply("cemu", Settings{State: "absent", Entries: []Entry{}}, home, 540, "fr")
	xml, _ := os.ReadFile(filepath.Join(home, "Cemu", "settings.xml"))
	if !strings.Contains(string(xml), "<console_language>2</console_language>") {
		t.Fatalf("Cemu en français :\n%s", xml)
	}

	// Une langue que l'application ne connaît pas : rien n'est posé.
	other := t.TempDir()
	if report := Apply("cemu", Settings{State: "absent", Entries: []Entry{}}, other, 540, ""); len(report.Set) != 0 {
		t.Fatalf("sans langue, posé : %v", report.Set)
	}
}

// shadPS4 (chantier 21) : la langue dans son config.json, les autres clés
// gardées ; ses paramètres Box64 dans l'environnement d'à côté, au même
// chemin qu'avant. Kyty, lui, la reçoit en ligne de commande.
func TestShadPS4AndKytyGetTheApplicationLanguage(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(filepath.Dir(home), ".local", "share", "shadPS4")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"General": {"console_language": 1, "user_vaddr_max_in_gbytes": 256}, "GPU": {"fullscreen": true}}`
	if err := os.WriteFile(filepath.Join(data, "config.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Apply("shadps4", Settings{State: "enabled", Entries: []Entry{
		{Config: "Environment > BOX64_DYNAREC", Value: "1"},
	}}, home, 540, "fr")
	if report.Error != "" {
		t.Fatalf("Apply : %s", report.Error)
	}
	raw, _ := os.ReadFile(filepath.Join(data, "config.json"))
	var written map[string]map[string]any
	if err := json.Unmarshal(raw, &written); err != nil {
		t.Fatalf("config.json illisible : %v\n%s", err, raw)
	}
	if written["General"]["console_language"] != float64(2) || written["General"]["user_vaddr_max_in_gbytes"] != float64(256) || written["GPU"]["fullscreen"] != true {
		t.Fatalf("config.json : %s", raw)
	}
	environment, _ := os.ReadFile(filepath.Join(home, "shadPS4", "environment"))
	if string(environment) != "BOX64_DYNAREC=1\n" || EnvironmentFile("shadps4") != "shadPS4/environment" {
		t.Fatalf("environnement de shadPS4 : %q (%s)", environment, EnvironmentFile("shadps4"))
	}

	if args := LanguageArgs("kyty", "fr"); len(args) != 2 || args[0] != "--console-language" || args[1] != "2" {
		t.Fatalf("Kyty en français : %v", args)
	}
	if args := LanguageArgs("kyty", ""); args != nil {
		t.Fatalf("Kyty sans langue : %v", args)
	}
	if args := LanguageArgs("rpcs3", "fr"); args != nil {
		t.Fatalf("RPCS3 la lit dans son fichier : %v", args)
	}
}

// Les requêtes d'occlusion de Kyty (#445) : allumées par le lanceur sans aucun
// paramètre, éteintes par le paramètre, et données au processus du serveur.
// LE CACHE DE PIPELINES DE SHADPS4, allumé par le lanceur (#446), éteint par un
// paramètre, rallumé quand le paramètre s'en va.
func TestShadPS4GetsItsPipelineCacheFromTheLauncher(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(filepath.Dir(home), ".local", "share", "shadPS4", "config.json")
	cache := func() any {
		raw, _ := os.ReadFile(config)
		var written map[string]map[string]any
		if err := json.Unmarshal(raw, &written); err != nil {
			t.Fatalf("config.json illisible : %v\n%s", err, raw)
		}

		return written["Vulkan"]["pipeline_cache_enabled"]
	}

	if report := Apply("shadps4", Settings{State: "absent"}, home, 540, ""); report.Error != "" {
		t.Fatalf("Apply : %s", report.Error)
	}
	if got := cache(); got != true {
		t.Fatalf("sans paramètre : %v", got)
	}

	Apply("shadps4", Settings{State: "enabled", Entries: []Entry{
		{Config: "Vulkan > pipeline_cache_enabled", Value: "false"},
	}}, home, 540, "")
	if got := cache(); got != false {
		t.Fatalf("éteint par le paramètre : %v", got)
	}

	Apply("shadps4", Settings{State: "enabled", Entries: nil}, home, 540, "")
	if got := cache(); got != true {
		t.Fatalf("paramètre retiré : %v", got)
	}
}

// LES REQUÊTES D'OCCLUSION ET LE CACHE DE SHADERS DE KYTY, allumés par le
// lanceur (#445, tools/kyty/0016) ; un paramètre les éteint, et retiré, le
// lanceur les rallume.
func TestKytyGetsItsOcclusionQueriesAndShaderCacheFromTheLauncher(t *testing.T) {
	home := t.TempDir()
	report := Apply("kyty", Settings{State: "absent"}, home, 540, "")
	if report.Error != "" {
		t.Fatalf("Apply : %s", report.Error)
	}
	if got := Environment("kyty", home); len(got) != 2 || got[0] != "KYTY_OCCLUSION_QUERIES=1" || got[1] != "KYTY_SHADER_CACHE=1" {
		t.Fatalf("sans paramètre : %v", got)
	}

	Apply("kyty", Settings{State: "enabled", Entries: []Entry{
		{Config: "Environment > KYTY_OCCLUSION_QUERIES", Value: "0"},
		{Config: "Environment > KYTY_SHADER_CACHE", Value: "0"},
		{Config: "Environment > BOX64_DYNAREC", Value: "1"},
	}}, home, 540, "")
	got := Environment("kyty", home)
	if len(got) != 3 || got[0] != "KYTY_OCCLUSION_QUERIES=0" || got[1] != "KYTY_SHADER_CACHE=0" || got[2] != "BOX64_DYNAREC=1" {
		t.Fatalf("éteints par les paramètres : %v", got)
	}

	// Les paramètres retirés : le lanceur les rallume, il ne les retire pas.
	Apply("kyty", Settings{State: "enabled", Entries: nil}, home, 540, "")
	if got := Environment("kyty", home); len(got) != 2 || got[0] != "KYTY_OCCLUSION_QUERIES=1" || got[1] != "KYTY_SHADER_CACHE=1" {
		t.Fatalf("paramètres retirés : %v", got)
	}
}
