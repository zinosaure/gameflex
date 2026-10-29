package emuparams

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// writer — où un émulateur prend ses paramètres, et ce que « auto » y veut
// dire. Seuls les émulateurs d'ici ont un écrivain : un fichier activé pour
// un autre le dit (Report.NYI) au lieu de passer pour appliqué.
type writer struct {
	// file — relatif au XDG_CONFIG_HOME de l'émulateur, ou à son
	// XDG_DATA_HOME quand data est vrai (xemu, shadPS4). Les deux hôtes
	// suivent les emplacements par défaut — $HOME/.config et
	// $HOME/.local/share —, le second se déduit donc du premier.
	file string
	data bool
	// format — la grammaire de ce fichier (formats.go).
	format format
	// auto — ce que « auto » veut dire à cette adresse, d'après la hauteur du
	// flux et, si besoin, le fichier tel qu'il est déjà (le mode docké de
	// la Switch change ce que vaut ×1).
	auto func(config string, streamHeight int, lines []string) (string, bool)
	// launcher — les adresses que le LANCEUR règle quand aucun paramètre n'en
	// décide, à leur valeur « auto ». Elles ne sont jamais retirées.
	//
	// La résolution de rendu de RPCS3 suivait le flux avant ce fichier, sur
	// les deux hôtes : l'application l'imposait sur le téléphone, l'entrypoint
	// le fait sur le serveur. Le retirer en désactivant les paramètres la
	// faisait retomber à 100 % : sur le serveur, juste après que l'entrypoint
	// l'avait réglée, pour la partie même ; sur le téléphone, pour toujours.
	launcher []string
	// language — l'adresse de la langue de la console dans ce fichier, et
	// languages sa valeur pour chaque langue de l'application (chantier 21) :
	// la console démarre dans la langue de la page qui lance. Reposée à
	// chaque lancement, comme une adresse du lanceur — la configuration du
	// joueur persiste, une valeur posée une fois resterait celle de la
	// première partie. Un paramètre du fichier qui la règle gagne.
	language  string
	languages map[string]string
	// languagePatch — la langue de la console dans un fichier BINAIRE, quand
	// elle n'est dans aucun fichier texte (l'EEPROM de la Xbox, le
	// xconfig.settings de Xenia). Posée à chaque lancement, comme language.
	languagePatch *binaryPatch
	// env — le fichier est l'environnement d'un invité du téléphone (#371),
	// que GlibcGuest lit avant de lancer : EnvironmentFile le nomme.
	env bool
	// environment — l'environnement d'un émulateur dont le fichier principal
	// n'en est pas un (RPCS3, Citron, Eden), relatif au même XDG_CONFIG_HOME : les
	// paramètres « Environment > NOM » y vont, les autres dans `file`. Les
	// options du pilote Turnip (`TU_DEBUG`…) passent par là (#388, 2026-10-19).
	// Le téléphone le lit comme celui d'un invité sous Box64, et l'adaptateur
	// du serveur le donne au processus de l'émulateur (Environment).
	environment string
}

var writers = map[string]writer{
	"rpcs3": {file: "rpcs3/config.yml", format: yamlFormat, auto: rpcs3Auto, launcher: []string{"Video > Resolution Scale"},
		environment: "rpcs3/environment",
		language:    "System > Language", languages: map[string]string{"fr": "French", "en": "English (US)"}},
	// LES DEUX ÉMULATEURS DE LA SWITCH (chantier 26) : Citron, le premier, et
	// Eden, le second. Leur résolution suit le flux sur les deux hôtes
	// (#340) : le téléphone (GlibcGuest.stageSwitchConfig) et le point
	// d'entrée de chaque image la posent avant les paramètres — même règle que
	// citronAuto et edenAuto, sur la liste de paliers de chacun.
	"citron": {file: "citron/qt-config.ini", format: qtINIFormat, auto: citronAuto, launcher: []string{"Renderer > resolution_setup"},
		environment: "citron/environment",
		language:    "System > language_index", languages: map[string]string{"fr": "2", "en": "1"}},
	"eden": {file: "eden/qt-config.ini", format: qtINIFormat, auto: edenAuto, launcher: []string{"Renderer > resolution_setup"},
		environment: "eden/environment",
		language:    "System > language_index", languages: map[string]string{"fr": "2", "en": "1"}},
	"cemu": {file: "Cemu/settings.xml", format: xmlFormat,
		language: "content > console_language", languages: map[string]string{"fr": "2", "en": "1"}},
	// LES INVITÉS DU TÉLÉPHONE SOUS BOX64 (chantier 17, #371) : leurs réglages
	// sont des variables d'environnement — ceux de Box64 (traducteur,
	// mémoire…), le son de SDL — que GlibcGuest lit dans ce fichier avant de
	// lancer. Le serveur exécute leur x86 en natif et n'y touche pas. Les
	// réglages de shadPS4 lui-même (#354) viendront dans son config.toml.
	// Les requêtes d'occlusion de Kyty, allumées par le lanceur quand aucun
	// paramètre n'en décide (#445) : sans elles, le GPU dessine ce que le jeu
	// voulait sauter (FF7 Remake, 10 i/s au lieu de 13,5). Son cache de
	// shaders aussi, comme le fait son propre lanceur (tools/kyty/0016).
	"kyty": {file: "kyty/environment", format: envFormat, env: true, auto: kytyAuto,
		launcher: []string{"Environment > KYTY_OCCLUSION_QUERIES", "Environment > KYTY_SHADER_CACHE"}},
	// shadPS4 : son config.json (chantier 21, la langue de la console) ; ses
	// paramètres — Box64, le son de SDL — restent dans l'environnement d'à
	// côté, au même chemin pour GlibcGuest. Le téléphone pose aussi la plage
	// d'adresses dans ce config.json (GlibcGuest.stagePs4), avant. Le lanceur
	// allume le cache de pipelines quand aucun paramètre n'en décide (#446) :
	// éteint, shadPS4 recompile tout à chaque partie (Kingdom Hearts III sur le
	// S25 : 241 compilations, des secondes à 2 images/s).
	"shadps4": {file: "shadPS4/config.json", data: true, format: jsonFormat,
		auto: shadps4Auto, launcher: []string{"Vulkan > pipeline_cache_enabled"},
		environment: "shadPS4/environment",
		language:    "General > console_language", languages: map[string]string{"fr": "2", "en": "1"}},
	// La surface de xemu suit le flux sur les deux hôtes (#340) : même règle
	// que xemuAuto.
	"xemu": {file: "xemu/xemu/xemu.toml", data: true, format: tomlFormat, auto: xemuAuto, launcher: []string{"display.quality > surface_scale"},
		languagePatch: &binaryPatch{file: "xemu/xemu/eeprom.bin", apply: xboxEEPROMLanguage, values: map[string]uint32{"fr": 4, "en": 1}}},
	// Xenia (Xbox 360, sur le serveur seulement) : sa configuration vit dans
	// sa racine de stockage, $XDG_DATA_HOME/Xenia. Il réécrit le fichier en
	// entier à chaque démarrage, en gardant ses valeurs : ce qui est posé ici
	// tient.
	"xenia": {file: "Xenia/xenia-canary.config.toml", data: true, format: tomlFormat,
		languagePatch: &binaryPatch{file: "Xenia/xconfig.settings", apply: xeniaLanguage, values: map[string]uint32{"fr": 4, "en": 1}}},
}

// RenderScale — la part de la sortie 1280×720 de RPCS3 qu'un flux de cette
// hauteur demande, en pourcentage — RPCS3 la borne entre 25 et 800 ; 100
// sans hauteur connue. 540p → 75, 720p → 100, 1080p → 150 : RPCS3 dessine à la
// taille choisie pour la partie au lieu de 720p réduit ensuite (« faire en
// sorte que l'émulateur rende les images tel qu'on a choisi dans stream
// settings », le porteur). La même règle que l'entrypoint PS3 du serveur.
func RenderScale(streamHeight int) int {
	if streamHeight <= 0 {
		return 100
	}

	return max(25, min(800, int(math.Round(float64(streamHeight)*100/720))))
}

func rpcs3Auto(config string, streamHeight int, _ []string) (string, bool) {
	if config != "Video > Resolution Scale" {
		return "", false
	}

	return strconv.Itoa(RenderScale(streamHeight)), true
}

// CitronResolution — le palier de Citron le plus proche du flux, en NUMÉRO
// dans sa liste (src/common/settings_enums.h : Res1_4X, Res1_2X, Res3_4X,
// Res1X, Res3_2X, Res2X, Res3X, …, Res5_4X, Res7_4X), à égalité le plus bas.
// Même règle qu'EdenResolution ; ×0,25 est écarté — Citron ne le propose
// même pas. La même que GlibcGuest.citronResolution et le point d'entrée de
// l'image de Citron.
func CitronResolution(streamHeight int, docked bool) int {
	return switchResolution(streamHeight, docked,
		[]int{1, 2, 3, 12, 4, 13, 5, 6}, []float64{0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 3})
}

// EdenResolution — le palier d'Eden le plus proche du flux, en NUMÉRO dans sa
// liste (src/common/settings_enums.h : Res1_4X, Res1_2X, Res3_4X, Res1X,
// Res5_4X, Res3_2X, Res2X, Res3X…, vérifiée à la nocturne de-7e6e01d4), à
// égalité le plus bas. La sortie de la console vaut 1080p docké, 720p en mode
// portable : en docké, 540p → ×0,5, 720p → ×0,75, 1080p → ×1. ×0,25
// est écarté, comme dans les menus d'Eden. ×1 (3) sans hauteur connue. La
// même règle que GlibcGuest.edenResolution et l'entrypoint de la Switch.
func EdenResolution(streamHeight int, docked bool) int {
	return switchResolution(streamHeight, docked,
		[]int{1, 2, 3, 4, 5, 6, 7}, []float64{0.5, 0.75, 1, 1.25, 1.5, 2, 3})
}

// switchResolution — parmi les paliers d'un émulateur Switch (`setups`, ses
// numéros, et leurs facteurs), celui dont la hauteur est la plus proche du
// flux, à égalité le plus bas ; ×1 (3) sans hauteur connue.
func switchResolution(streamHeight int, docked bool, setups []int, factors []float64) int {
	if streamHeight <= 0 {
		return 3
	}
	base := 720.0
	if docked {
		base = 1080
	}
	best, bestDistance, bestHeight := 3, -1.0, 0.0
	for i, setup := range setups {
		height := factors[i] * base
		distance := math.Abs(height - float64(streamHeight))
		if bestDistance < 0 || distance < bestDistance || (distance == bestDistance && height < bestHeight) {
			best, bestDistance, bestHeight = setup, distance, height
		}
	}

	return best
}

// citronAuto — « auto » pour la résolution de Citron : le palier du flux, en
// mode portable si le fichier le dit (`use_docked_mode=0`, posé par un
// paramètre plus haut dans le même fichier, ou par l'émulateur lui-même).
func citronAuto(config string, streamHeight int, lines []string) (string, bool) {
	return switchAuto(CitronResolution, config, streamHeight, lines)
}

// edenAuto — « auto » pour la résolution d'Eden : le palier du flux, en mode
// portable si le fichier le dit (`use_docked_mode=0`, posé par un paramètre
// plus haut dans le même fichier, ou par l'émulateur lui-même).
func edenAuto(config string, streamHeight int, lines []string) (string, bool) {
	return switchAuto(EdenResolution, config, streamHeight, lines)
}

func switchAuto(resolution func(int, bool) int, config string, streamHeight int, lines []string) (string, bool) {
	if config != "Renderer > resolution_setup" {
		return "", false
	}
	docked := true
	if from, to, ok := qtINISection(lines, "System"); ok {
		for _, line := range lines[from:to] {
			if strings.TrimSpace(line) == "use_docked_mode=0" {
				docked = false
			}
		}
	}

	return strconv.Itoa(resolution(streamHeight, docked)), true
}

// XemuSurfaceScale — le multiple de la Xbox (640×480) le plus proche du flux,
// à égalité le plus petit : 540p et 720p → ×1, 1080p → ×2. ×1 sans
// hauteur connue. La même règle que GlibcGuest.xemuSurfaceScale et
// l'entrypoint de la Xbox.
func XemuSurfaceScale(streamHeight int) int {
	return max(1, (streamHeight+239)/480)
}

func xemuAuto(config string, streamHeight int, _ []string) (string, bool) {
	if config != "display.quality > surface_scale" {
		return "", false
	}

	return strconv.Itoa(XemuSurfaceScale(streamHeight)), true
}

// kytyAuto — la valeur que le lanceur donne aux requêtes d'occlusion et au
// cache de shaders de Kyty : allumés. `KYTY_OCCLUSION_QUERIES` vaut
// `--occlusion-queries` (tools/kyty/0015), `KYTY_SHADER_CACHE` vaut
// `--shader-cache` (0016).
func kytyAuto(config string, _ int, _ []string) (string, bool) {
	if config != "Environment > KYTY_OCCLUSION_QUERIES" && config != "Environment > KYTY_SHADER_CACHE" {
		return "", false
	}

	return "1", true
}

// shadps4Auto — la valeur que le lanceur donne au cache de pipelines de
// shadPS4 : allumé.
func shadps4Auto(config string, _ int, _ []string) (string, bool) {
	if config != "Vulkan > pipeline_cache_enabled" {
		return "", false
	}

	return "true", true
}

// languageArgs — l'option de langue d'un émulateur qui la prend en ligne de
// commande, et sa valeur pour chaque langue de l'application (chantier 21).
// KytyPS5 : `--console-language`, relevée dans sa source (main.cpp), absente
// de tout fichier de configuration.
var languageArgs = map[string]struct {
	flag   string
	values map[string]string
}{
	"kyty": {flag: "--console-language", values: map[string]string{"fr": "2", "en": "1"}},
}

// LanguageArgs — les arguments qui donnent à cet émulateur la langue de
// l'application, à ajouter à sa ligne de commande ; rien pour un émulateur
// qui la lit dans son fichier, ou pour une langue inconnue. La même table
// pour le serveur (l'adaptateur de streamex) et le téléphone (GlibcGuest).
func LanguageArgs(emulator, language string) []string {
	entry, known := languageArgs[emulator]
	value, ok := entry.values[language]
	if !known || !ok {
		return nil
	}

	return []string{entry.flag, value}
}

// EnvironmentFile — le fichier d'environnement d'un émulateur, relatif à son
// XDG_CONFIG_HOME : le fichier même d'un invité sous Box64, celui d'à côté
// pour RPCS3, Citron et Eden ; vide pour un émulateur qui n'en a pas.
func EnvironmentFile(emulator string) string {
	w, known := writers[emulator]
	if !known {
		return ""
	}
	if w.env {
		return w.file
	}

	return w.environment
}

// Environment — les variables que les paramètres donnent au processus d'un
// émulateur, « NOM=valeur », lues dans son fichier d'environnement
// (EnvironmentFile) après Apply ; « auto » ne pose rien. Pour l'adaptateur du
// serveur, qui lance l'émulateur lui-même — le téléphone lit le même fichier
// dans GlibcGuest. Un invité sous Box64 le reçoit aussi : le serveur exécute
// son x86 en natif, et les variables `BOX64_*` n'y font rien — celles de
// l'émulateur, oui (`KYTY_OCCLUSION_QUERIES`, #445).
func Environment(emulator, configHome string) []string {
	file := EnvironmentFile(emulator)
	if file == "" {
		return nil
	}
	content, err := os.ReadFile(filepath.Join(configHome, file))
	if err != nil {
		return nil
	}
	var variables []string
	for _, line := range strings.Split(string(content), "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || !envName.MatchString(name) || value == "auto" {
			continue
		}
		variables = append(variables, name+"="+value)
	}

	return variables
}

// Report — ce qu'un lancement a fait des paramètres, pour le journal.
type Report struct {
	State string
	// Set — « adresse = valeur », ce qui a été écrit.
	Set []string
	// Released — ce qu'un lancement précédent avait posé et que le fichier
	// ne porte plus : retiré, l'émulateur reprend son défaut — sauf une
	// adresse du lanceur, remise à sa valeur « auto ».
	Released []string
	// Skipped — une adresse qui ne désigne pas une valeur du fichier de
	// l'émulateur.
	Skipped []string
	// NYI — un fichier activé pour un émulateur que GameFlex ne sait pas
	// encore régler.
	NYI   bool
	Error string
}

// Apply — les paramètres de cet émulateur écrits dans son fichier, sous
// configHome (son XDG_CONFIG_HOME). settings vient de Store.Settings — sur le
// serveur, il a traversé l'environnement du conteneur.
//
// RENDU À L'ÉMULATEUR : ce qu'un lancement précédent avait posé et que le
// fichier ne porte plus — paramètre retiré à la main, fichier désactivé ou
// supprimé — est retiré, et l'émulateur reprend son défaut. Sinon
// l'interrupteur du formulaire mentirait : la valeur resterait écrite. Ce qui
// a été posé est noté à côté (`gameflex-params.json`). Une adresse que le
// lanceur règle lui-même (writer.launcher) n'est jamais retirée : elle revient
// à sa valeur « auto ».
//
// Un fichier illisible (State "error") ne touche à rien.
func Apply(emulator string, settings Settings, configHome string, streamHeight int, language string) Report {
	report := Report{State: settings.State, Error: settings.Error}
	if settings.State == "error" {
		return report
	}
	w, known := writers[emulator]
	if !known {
		report.NYI = settings.State == "enabled" && len(settings.Entries) > 0

		return report
	}
	if w.environment == "" {
		w.write(&report, settings.Entries, configHome, streamHeight, language, "gameflex-params.json")
		w.patchLanguage(&report, configHome, language)

		return report
	}

	// L'ENVIRONNEMENT À PART : « Environment > NOM » va dans le fichier
	// d'environnement, le reste dans celui de l'émulateur — chacun avec sa
	// trace, pour rendre ce qu'il avait posé.
	var main, environment []Entry
	for _, entry := range settings.Entries {
		if section, _, ok := strings.Cut(entry.Config, ">"); ok && strings.TrimSpace(section) == "Environment" {
			environment = append(environment, entry)
		} else {
			main = append(main, entry)
		}
	}
	w.write(&report, main, configHome, streamHeight, language, "gameflex-params.json")
	if report.Error != "" {
		return report
	}
	companion := writer{file: w.environment, format: envFormat, env: true}
	companion.write(&report, environment, configHome, streamHeight, "", "gameflex-params-environment.json")
	w.patchLanguage(&report, configHome, language)

	return report
}

// write — des entrées écrites dans le fichier de cet écrivain, ce que sa
// trace (`trace`, à côté du fichier) disait posé et qu'elles ne portent plus
// retiré, et le tout ajouté au rapport.
func (w writer) write(report *Report, entries []Entry, configHome string, streamHeight int, language, trace string) {
	base := configHome
	if w.data {
		base = filepath.Join(filepath.Dir(configHome), ".local", "share")
	}
	target := filepath.Join(base, w.file)
	var lines []string
	original, err := os.ReadFile(target)
	if err == nil {
		text := strings.TrimRight(string(original), "\n")
		if text != "" {
			lines = strings.Split(text, "\n")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		report.Error = err.Error()

		return
	}

	written := map[string]bool{}
	var order []string
	// « auto » lit le fichier tel que les AUTRES valeurs le laisseront
	// (#439) : la résolution d'une Switch dépend du mode de la console,
	// déclaré après elle. Lue dans l'ordre, elle prenait le mode de la partie
	// précédente — en passant au mode portable, 360p au lieu de 540p.
	settled := append([]string(nil), lines...)
	for _, entry := range entries {
		if cut := strings.LastIndex(entry.Config, ">"); cut > 0 && entry.Value != "auto" {
			settled, _ = w.format.set(settled, strings.TrimSpace(entry.Config[:cut]), strings.TrimSpace(entry.Config[cut+1:]), entry.Value)
		}
	}
	for _, entry := range entries {
		value := entry.Value
		if value == "auto" && w.auto != nil {
			if resolved, ok := w.auto(entry.Config, streamHeight, settled); ok {
				value = resolved
			}
		}
		cut := strings.LastIndex(entry.Config, ">")
		if cut <= 0 {
			report.Skipped = append(report.Skipped, entry.Config)

			continue
		}
		var ok bool
		lines, ok = w.format.set(lines, strings.TrimSpace(entry.Config[:cut]), strings.TrimSpace(entry.Config[cut+1:]), value)
		if !ok {
			report.Skipped = append(report.Skipped, entry.Config)

			continue
		}
		written[entry.Config] = true
		order = append(order, entry.Config)
		report.Set = append(report.Set, entry.Config+" = "+value)
	}

	// LE LANCEUR, là où aucun paramètre n'a décidé (voir writer.launcher).
	launcher := map[string]bool{}
	for _, config := range w.launcher {
		if written[config] {
			continue
		}
		value, ok := w.auto(config, streamHeight, lines)
		cut := strings.LastIndex(config, ">")
		if !ok || cut <= 0 {
			continue
		}
		lines, ok = w.format.set(lines, strings.TrimSpace(config[:cut]), strings.TrimSpace(config[cut+1:]), value)
		launcher[config] = ok
	}

	tracePath := filepath.Join(filepath.Dir(target), trace)
	var previous []string
	if content, err := os.ReadFile(tracePath); err == nil {
		// La trace est la nôtre : abîmée, on ne sait plus quoi rendre — ce
		// qu'on applique reste juste.
		_ = json.Unmarshal(content, &previous)
	}
	for _, config := range previous {
		cut := strings.LastIndex(config, ">")
		if cut <= 0 || written[config] {
			continue
		}
		if launcher[config] {
			value, _ := w.auto(config, streamHeight, lines)
			report.Released = append(report.Released, config+" (suit le flux : "+value+")")

			continue
		}
		var removed bool
		lines, removed = w.format.remove(lines, strings.TrimSpace(config[:cut]), strings.TrimSpace(config[cut+1:]))
		if removed {
			report.Released = append(report.Released, config)
		}
	}

	// LA LANGUE DE L'APPLICATION (chantier 21), là où aucun paramètre n'en
	// décide — après le retrait de ce qu'un lancement précédent avait posé,
	// pour qu'un paramètre retiré ne l'emporte pas avec lui.
	if value, known := w.languages[language]; known && !written[w.language] {
		cut := strings.LastIndex(w.language, ">")
		if next, set := w.format.set(lines, strings.TrimSpace(w.language[:cut]), strings.TrimSpace(w.language[cut+1:]), value); set {
			lines = next
			report.Set = append(report.Set, w.language+" = "+value+" (langue de l'application)")
		}
	}

	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	// Rien n'a changé — un serveur sans fichier, le cas le plus courant :
	// l'entrypoint a déjà réglé ce que le lanceur règle, et le fichier de
	// l'émulateur n'est pas réécrit.
	if content != string(original) {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			report.Error = err.Error()

			return
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			report.Error = err.Error()

			return
		}
	}
	// La trace dit ce que les PARAMÈTRES ont posé — pas le lanceur —, même
	// quand leurs valeurs étaient déjà là.
	if len(order) == 0 {
		_ = os.Remove(tracePath)
	} else if encoded, err := marshalPlain(order); err == nil {
		_ = os.WriteFile(tracePath, append(encoded, '\n'), 0o644)
	}
}

var plainScalar = regexp.MustCompile(`^[A-Za-z0-9_.()+/][A-Za-z0-9_.()+/ :-]*$`)

// yamlScalar — la valeur telle que YAML la relit à l'identique : nue comme
// RPCS3 l'écrit lui-même (« Recompiler (LLVM) », « 16:9 »), entre guillemets
// dès qu'un caractère changerait son sens (« : », « # », un début en
// « @ »…).
func yamlScalar(value string) string {
	if plainScalar.MatchString(value) && !strings.Contains(value, ": ") &&
		!strings.HasSuffix(value, ":") && !strings.HasSuffix(value, " ") {
		return value
	}

	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func leadingSpaces(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " "))]
}

// yamlBlock — le bloc d'une section : [sa première clé, sa fin[.
// « Video > Vulkan » est la sous-section Vulkan de Video. Créée si elle
// manque et que create le permet ; ok faux sinon — et aussi quand une VALEUR
// porte déjà son nom (« Resolution: 1280x720 ») : y poser une section
// écrirait la clé deux fois, et l'émulateur ne lirait plus son fichier.
//
// Les clés se cherchent DANS leur section : RPCS3 a des noms qui reviennent
// d'une section à l'autre (« Enabled », « Renderer »), et chercher une ligne
// n'importe où écrirait au mauvais endroit.
func yamlBlock(lines []string, section string, create bool) ([]string, int, int, bool) {
	from, to := 0, len(lines)
	indent := ""
	for _, raw := range strings.Split(section, ">") {
		header := indent + strings.TrimSpace(raw) + ":"
		at := -1
		for i := from; i < to; i++ {
			if lines[i] == header {
				at = i

				break
			}
		}
		if at < 0 {
			if !create {
				return lines, 0, 0, false
			}
			for i := from; i < to; i++ {
				if strings.HasPrefix(lines[i], header) {
					return lines, 0, 0, false
				}
			}
			lines = append(lines[:to], append([]string{header}, lines[to:]...)...)
			at = to
		}
		indent += "  "
		end := at + 1
		for end < len(lines) && (strings.HasPrefix(lines[end], indent) || strings.TrimSpace(lines[end]) == "") {
			end++
		}
		from, to = at+1, end
	}

	return lines, from, to, true
}

// setYAMLKey — `clé: valeur` dans sa section : remplacée si elle y est,
// ajoutée en tête de section sinon. Refusée si la clé est elle-même une
// section — la remplacer par une valeur détacherait tout son contenu —, ou si
// la section ne peut pas exister.
func setYAMLKey(lines []string, section, key, value string) ([]string, bool) {
	lines, from, to, ok := yamlBlock(lines, section, true)
	if !ok {
		return lines, false
	}
	indent := leadingSpaces(lines[from-1]) + "  "
	for i := from; i < to; i++ {
		if strings.HasPrefix(lines[i], indent+key+":") {
			if i+1 < to && strings.HasPrefix(lines[i+1], indent+"  ") {
				return lines, false
			}
			lines[i] = indent + key + ": " + value

			return lines, true
		}
	}
	lines = append(lines[:from], append([]string{indent + key + ": " + value}, lines[from:]...)...)

	return lines, true
}

// removeYAMLKey — la ligne d'une clé retirée de sa section ; faux si elle n'y
// était pas.
func removeYAMLKey(lines []string, section, key string) ([]string, bool) {
	lines, from, to, ok := yamlBlock(lines, section, false)
	if !ok {
		return lines, false
	}
	indent := leadingSpaces(lines[from-1]) + "  "
	for i := from; i < to; i++ {
		if strings.HasPrefix(lines[i], indent+key+":") && !(i+1 < to && strings.HasPrefix(lines[i+1], indent+"  ")) {
			return append(lines[:i], lines[i+1:]...), true
		}
	}

	return lines, false
}
