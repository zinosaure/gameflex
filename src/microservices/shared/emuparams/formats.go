package emuparams

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// format — la grammaire du fichier d'un émulateur : poser une valeur à son
// adresse (section, clé), ou l'en retirer pour qu'il reprenne son défaut.
// L'adresse est celle du paramètre (`config`, « Section > clé »), coupée au
// dernier « > » ; la valeur est le texte du formulaire, que chaque format
// écrit à sa façon.
type format struct {
	set    func(lines []string, section, key, value string) ([]string, bool)
	remove func(lines []string, section, key string) ([]string, bool)
}

var (
	// Le config.yml de RPCS3.
	yamlFormat = format{
		set: func(lines []string, section, key, value string) ([]string, bool) {
			return setYAMLKey(lines, section, key, yamlScalar(value))
		},
		remove: removeYAMLKey,
	}
	// Le qt-config.ini de Citron et d'Eden (QSettings, comme yuzu dont ils viennent).
	qtINIFormat = format{set: setQtINIKey, remove: removeQtINIKey}
	// Le xemu.toml de xemu, le config.toml de shadPS4.
	tomlFormat = format{
		set: func(lines []string, section, key, value string) ([]string, bool) {
			return setTOMLKey(lines, section, key, tomlValue(value))
		},
		remove: removeTOMLKey,
	}
	// Le settings.xml de Cemu.
	xmlFormat = format{set: setXMLKey, remove: removeXMLKey}
	// L'environnement d'un invité du téléphone sous Box64 (#371) — Kyty,
	// shadPS4 : une variable par ligne, que GlibcGuest lit avant de lancer.
	envFormat = format{set: setEnvKey, remove: removeEnvKey}
)

// ── L'environnement (Box64, SDL — les invités du téléphone) ─────────────────
//
// `CLÉ=valeur`, une ligne par variable, sans section : l'adresse d'un
// paramètre est « Environment > CLÉ », et la section ne sert qu'à l'écran.
// Un interrupteur s'écrit 1/0 — ce que Box64 lit ; « auto » s'écrit tel
// quel, et le lanceur ne pose alors rien (le son de SDL suit la session).

var envName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func envValue(value string) string {
	switch value {
	case "true":
		return "1"
	case "false":
		return "0"
	}

	return value
}

func setEnvKey(lines []string, _ string, key, value string) ([]string, bool) {
	if !envName.MatchString(key) {
		return lines, false
	}
	out := append([]string(nil), lines...)
	for i, line := range out {
		if strings.HasPrefix(line, key+"=") {
			out[i] = key + "=" + envValue(value)

			return out, true
		}
	}

	return append(out, key+"="+envValue(value)), true
}

func removeEnvKey(lines []string, _ string, key string) ([]string, bool) {
	out := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			removed = true

			continue
		}
		out = append(out, line)
	}

	return out, removed
}

// ── L'INI de Qt (Citron, Eden) ───────────────────────────────────────────────────
//
// `[Section]`, puis `clé\default=false` et `clé=valeur` : SANS la première
// ligne, l'émulateur ignore la valeur et garde son défaut — c'est ce qui a coûté la
// résolution du flux avant #340. Retirer, c'est retirer les deux lignes : il
// reprend son défaut. Les clés sont cherchées DANS leur section.

// qtINISection — [début des clés, fin[ de `[section]` ; ok faux si elle manque.
func qtINISection(lines []string, section string) (int, int, bool) {
	header := "[" + section + "]"
	for i, line := range lines {
		if strings.TrimSpace(line) != header {
			continue
		}
		end := i + 1
		for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
			end++
		}

		return i + 1, end, true
	}

	return 0, 0, false
}

// qtININame — une section ou une clé qui tient sur sa ligne sans changer de
// sens : ni crochet, ni « = », ni la barre inverse du `\default`.
func qtININame(name string) bool {
	return configName(name) && !strings.ContainsAny(name, `[]=\`)
}

// qtINIValue — la valeur telle que QSettings la relit : nue (un nombre, true,
// false, un mot), entre guillemets dès qu'une virgule en ferait une liste ou
// qu'une espace au bord se perdrait.
func qtINIValue(value string) string {
	if strings.ContainsAny(value, `,"\`) || strings.TrimSpace(value) != value {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
	}

	return value
}

func setQtINIKey(lines []string, section, key, value string) ([]string, bool) {
	if !qtININame(section) || !qtININame(key) {
		return lines, false
	}
	lines, _ = removeQtINIKey(lines, section, key)
	entry := []string{key + `\default=false`, key + "=" + qtINIValue(value)}
	from, _, ok := qtINISection(lines, section)
	if !ok {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}

		return append(append(lines, "["+section+"]"), entry...), true
	}

	return append(lines[:from], append(entry, lines[from:]...)...), true
}

func removeQtINIKey(lines []string, section, key string) ([]string, bool) {
	from, to, ok := qtINISection(lines, section)
	if !ok {
		return lines, false
	}
	out := append([]string(nil), lines[:from]...)
	removed := false
	for _, line := range lines[from:to] {
		if strings.HasPrefix(line, key+"=") || strings.HasPrefix(line, key+`\default=`) {
			removed = true

			continue
		}
		out = append(out, line)
	}

	return append(out, lines[to:]...), removed
}

// ── Le TOML (xemu, shadPS4) ────────────────────────────────────────────────
//
// La table `[section]` — pointée s'il le faut, `[display.quality]` —, puis
// `clé = valeur`. Une table n'apparaît qu'une fois en TOML : créée au bout du
// fichier seulement si elle manque.

var tomlBare = regexp.MustCompile(`^(true|false|-?[0-9]+(\.[0-9]+)?)$`)

// tomlValue — un booléen ou un nombre tels quels, une chaîne entre guillemets.
func tomlValue(value string) string {
	if tomlBare.MatchString(value) {
		return value
	}

	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

// tomlTable — [début des clés, fin[ de `[section]` ; ok faux si elle manque.
func tomlTable(lines []string, section string) (int, int, bool) {
	header := "[" + section + "]"
	for i, line := range lines {
		if strings.TrimSpace(line) != header {
			continue
		}
		end := i + 1
		for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
			end++
		}

		return i + 1, end, true
	}

	return 0, 0, false
}

// tomlKeyOf — la clé d'une ligne `clé = valeur`, vide pour un commentaire ou
// une ligne vide.
func tomlKeyOf(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}
	key, _, found := strings.Cut(trimmed, "=")
	if !found {
		return ""
	}

	return strings.TrimSpace(key)
}

func tomlName(name string) bool {
	return configName(name) && !strings.ContainsAny(name, `[]="' `)
}

func setTOMLKey(lines []string, section, key, value string) ([]string, bool) {
	if !tomlName(section) || !tomlName(key) || strings.Contains(key, ".") {
		return lines, false
	}
	entry := key + " = " + value
	from, to, ok := tomlTable(lines, section)
	if !ok {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}

		return append(lines, "["+section+"]", entry), true
	}
	for i := from; i < to; i++ {
		if tomlKeyOf(lines[i]) == key {
			lines[i] = entry

			return lines, true
		}
	}

	return append(lines[:from], append([]string{entry}, lines[from:]...)...), true
}

func removeTOMLKey(lines []string, section, key string) ([]string, bool) {
	from, to, ok := tomlTable(lines, section)
	if !ok {
		return lines, false
	}
	for i := from; i < to; i++ {
		if tomlKeyOf(lines[i]) == key {
			return append(lines[:i], lines[i+1:]...), true
		}
	}

	return lines, false
}

// ── Le XML (Cemu) ──────────────────────────────────────────────────────────
//
// Le settings.xml de Cemu : un élément par ligne, `<AsyncCompile>true</AsyncCompile>`,
// dans des conteneurs `<Graphic>` … `</Graphic>` sous la racine `<content>`.
// « Graphic > AsyncCompile » désigne cet élément ; « content > fullscreen »
// un enfant direct de la racine. Les commentaires, qui s'étendent sur
// plusieurs lignes dans le gabarit du serveur, sont sautés : une ligne de
// commentaire qui CITE une balise n'est pas cette balise.

var xmlName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// xmlScan — l'indice de la ligne qui ouvre `<name>` entre from et to, hors
// commentaires, et celui de sa fermeture ; -1 si absent.
func xmlScan(lines []string, from, to int, name string) (int, int) {
	inComment := false
	open, depth := -1, 0
	for i := from; i < to; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if inComment {
			if strings.Contains(trimmed, "-->") {
				inComment = false
			}

			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			inComment = !strings.Contains(trimmed, "-->")

			continue
		}
		switch {
		case trimmed == "<"+name+">":
			if open < 0 {
				open = i
			}
			depth++
		case trimmed == "</"+name+">" && open >= 0:
			depth--
			if depth == 0 {
				return open, i
			}
		}
	}

	return -1, -1
}

// xmlRoot — [début, fin[ du contenu de l'élément racine (après le prologue).
func xmlRoot(lines []string) (string, int, int, bool) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<?") || strings.HasPrefix(trimmed, "<!--") || trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "<") || !strings.HasSuffix(trimmed, ">") || strings.HasSuffix(trimmed, "/>") {
			return "", 0, 0, false
		}
		name := strings.Trim(trimmed, "<>")
		_, end := xmlScan(lines, i, len(lines), name)
		if end < 0 {
			return "", 0, 0, false
		}

		return name, i + 1, end, true
	}

	return "", 0, 0, false
}

// xmlChildIndent — l'indentation des enfants d'un conteneur dont la
// fermeture est en `closing` : celle de son premier enfant, sinon la sienne
// plus l'unité du fichier.
func xmlChildIndent(lines []string, from, closing int) string {
	for i := from; i < closing; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "<") && !strings.HasPrefix(trimmed, "<!--") {
			return leadingWhitespace(lines[i])
		}
	}

	return leadingWhitespace(lines[closing]) + xmlIndentUnit(lines)
}

// xmlIndentUnit — l'unité d'indentation du fichier : celle du premier
// élément indenté (une tabulation chez Cemu, quatre espaces dans le gabarit
// du serveur), une tabulation à défaut.
func xmlIndentUnit(lines []string) string {
	for _, line := range lines {
		indent := leadingWhitespace(line)
		trimmed := strings.TrimSpace(line)
		if indent != "" && strings.HasPrefix(trimmed, "<") && !strings.HasPrefix(trimmed, "<!--") {
			return indent
		}
	}

	return "\t"
}

func leadingWhitespace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// xmlContainer — [début, fin[ du conteneur au bout de path, créé s'il manque
// et que create le permet. Refusé si un élément du chemin est une VALEUR
// (`<api>1</api>`) ou un élément vide (`<GraphicPack/>`) : y loger des
// enfants changerait ce que Cemu y lit.
func xmlContainer(lines []string, path []string, create bool) ([]string, int, int, bool) {
	root, from, to, ok := xmlRoot(lines)
	if !ok {
		return lines, 0, 0, false
	}
	if len(path) > 0 && path[0] == root {
		path = path[1:]
	}
	for _, name := range path {
		if !xmlName.MatchString(name) {
			return lines, 0, 0, false
		}
		open, closing := xmlScan(lines, from, to, name)
		if open < 0 {
			for i := from; i < to; i++ {
				trimmed := strings.TrimSpace(lines[i])
				if strings.HasPrefix(trimmed, "<"+name+">") || strings.HasPrefix(trimmed, "<"+name+"/>") || strings.HasPrefix(trimmed, "<"+name+" ") {
					return lines, 0, 0, false
				}
			}
			if !create {
				return lines, 0, 0, false
			}
			indent := xmlChildIndent(lines, from, to)
			block := []string{indent + "<" + name + ">", indent + "</" + name + ">"}
			lines = append(lines[:to], append(block, lines[to:]...)...)
			open, closing = to, to+1
		}
		from, to = open+1, closing
	}

	return lines, from, to, true
}

func xmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
}

// xmlValueLine — l'indice de `<key>…</key>` entre from et to, hors
// commentaires ; -1 si absent.
func xmlValueLine(lines []string, from, to int, key string) int {
	inComment := false
	for i := from; i < to; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if inComment {
			inComment = !strings.Contains(trimmed, "-->")

			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			inComment = !strings.Contains(trimmed, "-->")

			continue
		}
		if strings.HasPrefix(trimmed, "<"+key+">") && strings.HasSuffix(trimmed, "</"+key+">") {
			return i
		}
	}

	return -1
}

func setXMLKey(lines []string, section, key, value string) ([]string, bool) {
	if !xmlName.MatchString(key) {
		return lines, false
	}
	path := strings.Split(section, ">")
	for i := range path {
		path[i] = strings.TrimSpace(path[i])
	}
	lines, from, to, ok := xmlContainer(lines, path, true)
	if !ok {
		return lines, false
	}
	// Un conteneur du même nom que la clé : la remplacer par une valeur
	// détacherait tout son contenu.
	if open, _ := xmlScan(lines, from, to, key); open >= 0 {
		return lines, false
	}
	entry := "<" + key + ">" + xmlEscape(value) + "</" + key + ">"
	if at := xmlValueLine(lines, from, to, key); at >= 0 {
		lines[at] = leadingWhitespace(lines[at]) + entry

		return lines, true
	}
	indent := xmlChildIndent(lines, from, to)

	return append(lines[:to], append([]string{indent + entry}, lines[to:]...)...), true
}

func removeXMLKey(lines []string, section, key string) ([]string, bool) {
	path := strings.Split(section, ">")
	for i := range path {
		path[i] = strings.TrimSpace(path[i])
	}
	lines, from, to, ok := xmlContainer(lines, path, false)
	if !ok {
		return lines, false
	}
	if at := xmlValueLine(lines, from, to, key); at >= 0 {
		return append(lines[:at], lines[at+1:]...), true
	}

	return lines, false
}

// ── Le JSON (shadPS4) ──────────────────────────────────────────────────────
//
// Un objet par section — `{"General": {"console_language": 2}}` —, relu et
// réécrit en entier : les clés retrouvent l'ordre alphabétique où shadPS4 les
// écrit lui-même, indentées de deux espaces. Un fichier illisible n'est pas
// touché.

var jsonFormat = format{set: setJSONKey, remove: removeJSONKey}

// jsonValue — un booléen ou un nombre tels quels, une chaîne sinon.
func jsonValue(value string) any {
	switch value {
	case "true":
		return true
	case "false":
		return false
	}
	if number, err := strconv.ParseInt(value, 10, 64); err == nil {
		return number
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return number
	}

	return value
}

func readJSON(lines []string) (map[string]any, bool) {
	root := map[string]any{}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return root, true
	}
	if err := json.Unmarshal([]byte(text), &root); err != nil {
		return nil, false
	}

	return root, true
}

func writeJSON(root map[string]any) ([]string, bool) {
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false
	}

	return strings.Split(string(encoded), "\n"), true
}

func setJSONKey(lines []string, section, key, value string) ([]string, bool) {
	root, ok := readJSON(lines)
	if !ok || section == "" || key == "" {
		return lines, false
	}
	table, isTable := root[section].(map[string]any)
	if root[section] != nil && !isTable {
		return lines, false
	}
	if table == nil {
		table = map[string]any{}
		root[section] = table
	}
	table[key] = jsonValue(value)
	if written, ok := writeJSON(root); ok {
		return written, true
	}

	return lines, false
}

func removeJSONKey(lines []string, section, key string) ([]string, bool) {
	root, ok := readJSON(lines)
	if !ok {
		return lines, false
	}
	table, isTable := root[section].(map[string]any)
	if !isTable {
		return lines, false
	}
	if _, present := table[key]; !present {
		return lines, false
	}
	delete(table, key)
	if written, ok := writeJSON(root); ok {
		return written, true
	}

	return lines, false
}
