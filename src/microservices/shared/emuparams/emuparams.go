// Package emuparams — LES PARAMÈTRES DES ÉMULATEURS,
// `params/<console>/<émulateur>.json` (2026-10-12), communs au serveur et au
// téléphone.
//
// « Il faut faire d'une façon où on peut changer les réglages sans
// reconstruire l'APK à chaque fois » (le porteur, en réglant RPCS3), puis,
// le même jour : « c'est une feature en commun » — la webapp du serveur a le
// même écran, et ses émulateurs lisent le même fichier. La forme est la
// sienne, posée en plusieurs messages :
//
//   - le fichier d'un émulateur porte LA LISTE de ses paramètres — type,
//     libellé, description, valeur, options —, et l'écran en construit un
//     formulaire dynamique ;
//
//   - « avant de lancer n'importe quel émulateur, le launcher vérifie si un
//     fichier params existe […] et si ça existe alors il applique les params
//     qui sont dedans » ; c'est lui qui fait foi, avec un attribut `enabled`
//     pour l'activer ou non depuis le formulaire ;
//
//   - un fichier par ÉMULATEUR et non par console : « on peut avoir plusieurs
//     émulateurs pour une console » ;
//
//   - des préréglages, `params/<console>/presets/*.json`, lus par le
//     formulaire seul : « ils permettent seulement de remplir les champs avec
//     des valeurs », et l'admin enregistre ensuite.
//
//     {
//     "enabled": true,
//     "name": "PlayStation 3",
//     "description": "…",
//     "system": "RPCS3",
//     "requirements": ["PS3UPDAT.PUP"],
//     "params": {
//     "spuBlockSize": {
//     "type": "select", "label": "…", "description": "…",
//     "config": "Core > SPU Block Size",
//     "value": "Mega",
//     "options": { "Safe": "Safe", "Mega": "Mega", "Giga": "Giga" }
//     }
//     }
//     }
//
// Trois types : `select` (une des `options`, clé → libellé), `switch` (vrai ou
// faux) et `text` (une ligne). `config` est le seul champ qui n'était pas dans
// l'exemple du porteur : l'adresse du réglage dans le fichier de l'émulateur,
// avec ses propres noms — ceux du config.yml de RPCS3. La barre oblique est
// prise (RPCS3 a une section « Input/Output ») : les niveaux se séparent par
// « > ».
//
// Un préréglage ne porte que des valeurs, et dit par `system` de quel
// émulateur il remplit le formulaire :
//
//	{ "name": "Performance", "description": "…", "system": "RPCS3",
//	  "values": { "resolutionScale": "50", "msaa": "Disabled" } }
//
// Ce paquet porte le format, ce que GameFlex livre (shipped/), le dossier
// d'une bibliothèque (Store) et l'écriture dans le fichier de l'émulateur
// (Apply). Chaque hôte n'y ajoute que ses routes et son lanceur.
package emuparams

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

//go:embed shipped
var shipped embed.FS

// orderedMap — un objet JSON dont l'ORDRE compte : celui des paramètres est
// celui du formulaire, celui des options celui de la liste déroulante. Une
// map de Go les relirait dans le désordre et les réécrirait triés.
type orderedMap[T any] struct {
	keys   []string
	values map[string]T
}

func (m *orderedMap[T]) get(key string) (T, bool) {
	value, ok := m.values[key]

	return value, ok
}

func (m *orderedMap[T]) set(key string, value T) {
	if m.values == nil {
		m.values = map[string]T{}
	}
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

func (m *orderedMap[T]) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return errors.New("an object was expected")
	}
	m.keys, m.values = nil, map[string]T{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key := token.(string)
		var value T
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		m.set(key, value)
	}
	_, err := decoder.Token()

	return err
}

func (m orderedMap[T]) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, key := range m.keys {
		if i > 0 {
			out.WriteByte(',')
		}
		name, err := marshalPlain(key)
		if err != nil {
			return nil, err
		}
		value, err := marshalPlain(m.values[key])
		if err != nil {
			return nil, err
		}
		out.Write(name)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')

	return out.Bytes(), nil
}

// marshalPlain — SANS L'ÉCHAPPEMENT HTML de Go, qui écrirait « Video >
// Resolution Scale » dans un fichier qui se lit et se corrige à la main.
func marshalPlain(value any) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	return bytes.TrimRight(out.Bytes(), "\n"), nil
}

// param — un paramètre : de quoi construire son champ, et où l'écrire.
type param struct {
	Type        string              `json:"type"`
	Label       string              `json:"label"`
	Description string              `json:"description,omitempty"`
	Config      string              `json:"config"`
	Value       any                 `json:"value"`
	Options     *orderedMap[string] `json:"options,omitempty"`
}

// file — le fichier d'un émulateur.
type file struct {
	// Absent, il vaut vrai : un fichier écrit à la main sans lui s'applique,
	// puisqu'il existe.
	Enabled      *bool             `json:"enabled,omitempty"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	System       string            `json:"system"`
	Requirements []string          `json:"requirements,omitempty"`
	Params       orderedMap[param] `json:"params"`
}

func (f file) enabled() bool {
	return f.Enabled == nil || *f.Enabled
}

// preset — un préréglage : des valeurs, par paramètre.
type preset struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Override — le préréglage peut-il être remplacé ou supprimé depuis
	// l'écran ? Faux pour ceux que GameFlex livre ; vrai pour ceux qu'on
	// enregistre ; absent (écrit à la main), il est à l'admin, donc vrai.
	// « Il faut juste rajouter dans le fichier un attribut, override :
	// true|false » (le porteur, 2026-10-12).
	Override *bool          `json:"override,omitempty"`
	System   string         `json:"system"`
	Values   map[string]any `json:"values"`
}

// overridable — voir preset.Override.
func (p preset) overridable() bool {
	return p.Override == nil || *p.Override
}

// Ref — un émulateur d'une console.
type Ref struct{ ConsoleID, Emulator string }

// Shipped — les émulateurs dont GameFlex livre les paramètres
// (shipped/<console>/<émulateur>.json), dans l'ordre de l'arborescence. Seuls
// ceux-là sont proposés à l'écran : un hôte ne sait appliquer que ceux-là.
func Shipped() []Ref {
	var refs []Ref
	consoles, _ := fs.ReadDir(shipped, "shipped")
	for _, console := range consoles {
		if !console.IsDir() {
			continue
		}
		entries, _ := fs.ReadDir(shipped, path.Join("shipped", console.Name()))
		for _, entry := range entries {
			if !entry.IsDir() && path.Ext(entry.Name()) == ".json" {
				refs = append(refs, Ref{ConsoleID: console.Name(), Emulator: strings.TrimSuffix(entry.Name(), ".json")})
			}
		}
	}

	return refs
}

// IsShipped — cet émulateur a-t-il des paramètres livrés ?
func IsShipped(ref Ref) bool {
	for _, candidate := range Shipped() {
		if candidate == ref {
			return true
		}
	}

	return false
}

// shippedDefinition — le fichier que GameFlex livre pour cet émulateur.
func shippedDefinition(ref Ref) (file, []byte) {
	content, err := shipped.ReadFile(path.Join("shipped", ref.ConsoleID, ref.Emulator+".json"))
	if err != nil {
		panic("paramètres livrés introuvables : " + err.Error())
	}
	definition, err := decode(content)
	if err != nil {
		panic("paramètres livrés invalides (" + ref.Emulator + ") : " + err.Error())
	}

	return definition, content
}

// shippedPresets — les préréglages livrés d'une console : nom → contenu.
// shippedPresetIDs — les préréglages que GameFlex livre pour cette console.
func shippedPresetIDs(consoleID string) map[string]bool {
	names, _ := shippedPresets(consoleID)
	ids := make(map[string]bool, len(names))
	for _, name := range names {
		ids[name] = true
	}

	return ids
}

func shippedPresets(consoleID string) (names []string, contents map[string][]byte) {
	contents = map[string][]byte{}
	entries, _ := fs.ReadDir(shipped, path.Join("shipped", consoleID, "presets"))
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
			continue
		}
		content, err := shipped.ReadFile(path.Join("shipped", consoleID, "presets", entry.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		names = append(names, name)
		contents[name] = content
	}

	return names, contents
}

// configName — un nom de section ou de clé : il tient sur une ligne du
// fichier de l'émulateur. Un deux-points y couperait la ligne au mauvais
// endroit, un retour à la ligne y écrirait une clé de plus.
func configName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && !strings.ContainsAny(name, ":>\r\n")
}

// validConfig — « Section > Clé », ou plus profond : « Video > Performance
// Overlay > Enabled ». Une clé hors de toute section n'existe pas chez RPCS3.
func validConfig(config string) bool {
	parts := strings.Split(config, ">")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !configName(strings.TrimSpace(part)) {
			return false
		}
	}

	return true
}

// normalConfig — « Video>Vulkan » et « Video > Vulkan » sont la même adresse.
func normalConfig(config string) string {
	parts := strings.Split(config, ">")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	return strings.Join(parts, " > ")
}

// scalarText — un nombre écrit sans guillemets, à la main, vaut sa chaîne.
func scalarText(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(typed), true
	}

	return "", false
}

// accept — la valeur telle que le paramètre la garde, ou pourquoi il la
// refuse.
func (p param) accept(value any) (any, error) {
	switch p.Type {
	case "switch":
		if typed, ok := value.(bool); ok {
			return typed, nil
		}
		if text, ok := value.(string); ok && (text == "true" || text == "false") {
			return text == "true", nil
		}

		return nil, errors.New("true or false expected")
	case "select":
		text, ok := scalarText(value)
		if !ok || p.Options == nil {
			return nil, errors.New("one of the options expected")
		}
		if _, known := p.Options.get(text); !known {
			return nil, fmt.Errorf("« %s » is not one of the options", text)
		}

		return text, nil
	case "text":
		text, ok := scalarText(value)
		if !ok || strings.TrimSpace(text) == "" || strings.ContainsAny(text, "\r\n") {
			return nil, errors.New("one line of text expected")
		}

		return text, nil
	}

	return nil, fmt.Errorf("unknown type « %s »: select, switch or text", p.Type)
}

// decode — le fichier lu, ses valeurs ramenées à leur type, ou ce qui ne va
// pas, paramètre par paramètre.
func decode(content []byte) (file, error) {
	var f file
	if err := json.Unmarshal(content, &f); err != nil {
		return file{}, err
	}
	if len(f.Params.keys) == 0 {
		return file{}, errors.New("no parameters")
	}
	seen := map[string]string{}
	for _, id := range f.Params.keys {
		p := f.Params.values[id]
		if !validConfig(p.Config) {
			return file{}, fmt.Errorf("%s: invalid address « %s »", id, p.Config)
		}
		if other, taken := seen[normalConfig(p.Config)]; taken {
			return file{}, fmt.Errorf("%s: « %s » is already set by %s", id, p.Config, other)
		}
		seen[normalConfig(p.Config)] = id
		if p.Type == "select" && (p.Options == nil || len(p.Options.keys) == 0) {
			return file{}, fmt.Errorf("%s: a list without options", id)
		}
		value, err := p.accept(p.Value)
		if err != nil {
			return file{}, fmt.Errorf("%s: %w", id, err)
		}
		p.Value = value
		if p.Label == "" {
			p.Label = id
		}
		f.Params.values[id] = p
	}

	return f, nil
}

// encode — le fichier tel qu'il s'écrit : `enabled` en tête, dit, et les
// noms lisibles.
func encode(f file, enabled bool) ([]byte, error) {
	f.Enabled = &enabled
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(f); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}
