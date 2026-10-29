package emuparams

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

// mu — le fichier se lit au lancement pendant que l'écran l'écrit peut-être :
// une lecture ne doit jamais tomber au milieu d'une écriture.
var mu sync.Mutex

// Store — le dossier `params/` d'une bibliothèque.
//
// QUI POSSÈDE QUOI. Le fichier d'un émulateur est à l'admin : GameFlex n'y
// écrit que par le formulaire — sauf, sur le téléphone, une fois, au premier
// démarrage (SeedOnFirstRun) : il y pose le fichier livré, pour garder ce que
// l'application imposait à RPCS3 avant ce fichier. Le serveur n'imposait
// rien : il ne pose rien, et sans fichier rien n'est appliqué. Sans fichier,
// le formulaire part de la liste livrée, et l'enregistrer le crée.
//
// Les préréglages livrés sont à GameFlex : réécrits à chaque démarrage, pour
// qu'une valeur corrigée atteigne la bibliothèque. Un préréglage d'un autre
// nom est à l'admin, jamais touché.
type Store struct {
	// Root — la racine de la bibliothèque : `params/` est dessous.
	Root string
	// SeedOnFirstRun — poser le fichier livré de chaque émulateur quand le
	// dossier presets/ de sa console n'existe pas encore.
	SeedOnFirstRun bool
	// AfterWrite — appelé sur chaque dossier créé et chaque fichier écrit :
	// le kernel du serveur tourne en root, et rend au propriétaire de la
	// bibliothèque ce qu'il y pose.
	AfterWrite func(path string)
}

func (s Store) emulatorFile(ref Ref) string {
	return filepath.Join(s.Root, "params", ref.ConsoleID, ref.Emulator+".json")
}

func (s Store) presetsDir(consoleID string) string {
	return filepath.Join(s.Root, "params", consoleID, "presets")
}

// mkdirAll — les dossiers qui manquent, chacun passé à AfterWrite.
func (s Store) mkdirAll(dir string) error {
	relative, err := filepath.Rel(s.Root, dir)
	if err != nil || strings.HasPrefix(relative, "..") {
		return os.MkdirAll(dir, 0o755)
	}
	current := s.Root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		if _, err := os.Stat(current); err == nil {
			continue
		}
		if err := os.Mkdir(current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if s.AfterWrite != nil {
			s.AfterWrite(current)
		}
	}

	return nil
}

// writeFile — à côté puis renommé : un fichier à moitié écrit serait illisible
// au lancement suivant, et ses valeurs perdues sans le dire.
func (s Store) writeFile(target string, content []byte) error {
	if err := s.mkdirAll(filepath.Dir(target)); err != nil {
		return err
	}
	partial := target + ".part"
	if err := os.WriteFile(partial, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(partial, target); err != nil {
		_ = os.Remove(partial)

		return err
	}
	if s.AfterWrite != nil {
		s.AfterWrite(target)
	}

	return nil
}

// Ensure — les préréglages livrés, réécrits s'ils ont changé ; ce que
// GameFlex ne livre plus, retiré (pruneRetiredLocked) ; et — si le Store le
// demande — le fichier de chaque émulateur posé au PREMIER démarrage
// seulement : supprimé ensuite, il ne revient pas.
func (s Store) Ensure() error {
	if strings.TrimSpace(s.Root) == "" {
		return errors.New("library root required")
	}
	mu.Lock()
	defer mu.Unlock()

	var failures []string
	firstRun := map[string]bool{}
	for _, ref := range Shipped() {
		if _, seen := firstRun[ref.ConsoleID]; seen {
			continue
		}
		_, err := os.Stat(s.presetsDir(ref.ConsoleID))
		firstRun[ref.ConsoleID] = errors.Is(err, os.ErrNotExist)
		names, contents := shippedPresets(ref.ConsoleID)
		for _, name := range names {
			target := filepath.Join(s.presetsDir(ref.ConsoleID), name+".json")
			if current, err := os.ReadFile(target); err == nil && bytes.Equal(current, contents[name]) {
				continue
			}
			if err := s.writeFile(target, contents[name]); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	failures = append(failures, s.pruneRetiredLocked()...)
	if s.SeedOnFirstRun {
		for _, ref := range Shipped() {
			if !firstRun[ref.ConsoleID] {
				continue
			}
			if _, err := os.Stat(s.emulatorFile(ref)); !errors.Is(err, os.ErrNotExist) {
				continue
			}
			_, content := shippedDefinition(ref)
			if err := s.writeFile(s.emulatorFile(ref), content); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	for _, ref := range Shipped() {
		if err := s.completeLocked(ref); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, " ; "))
	}

	return nil
}

// completeLocked — CE QUE GAMEFLEX LIVRE DE PLUS ARRIVE DANS LE FICHIER
// (2026-10-19, #388). Le fichier d'un émulateur, posé au premier démarrage,
// ne voyait jamais les paramètres livrés après lui : les réglages de RPCS3
// ajoutés pour les accrocs de la PS3 n'auraient jamais atteint le S25. « Le
// fichier de réglage n'est pas à moi, c'est toi qui les avais rajoutés, donc
// tu peux modifier » (le porteur, le même jour).
//
// Un paramètre livré qui manque arrive à sa valeur livrée, à sa place dans
// l'ordre livré ; une option livrée qui manque à un paramètre du même type et
// de la même adresse s'y ajoute. Rien de ce que le fichier porte ne change :
// ni une valeur, ni un paramètre ajouté à la main, ni ses textes. Une adresse
// déjà réglée par un autre paramètre du fichier n'est pas doublée — le
// fichier deviendrait illisible. Un fichier absent, illisible ou d'un autre
// émulateur n'est pas touché.
func (s Store) completeLocked(ref Ref) error {
	mine, exists, err := s.readLocked(ref)
	if err != nil || !exists {
		return nil
	}
	shippedFile, _ := shippedDefinition(ref)
	if !strings.EqualFold(mine.System, shippedFile.System) {
		return nil
	}
	taken := map[string]bool{}
	for _, id := range mine.Params.keys {
		taken[normalConfig(mine.Params.values[id].Config)] = true
	}

	changed := false
	keys := append([]string(nil), mine.Params.keys...)
	for i, id := range shippedFile.Params.keys {
		theirs := shippedFile.Params.values[id]
		if current, present := mine.Params.get(id); present {
			if current.Type == theirs.Type && normalConfig(current.Config) == normalConfig(theirs.Config) &&
				current.Options != nil && theirs.Options != nil {
				if options, added := withMissing(*current.Options, *theirs.Options); added {
					current.Options = &options
					mine.Params.values[id] = current
					changed = true
				}
			}

			continue
		}
		if taken[normalConfig(theirs.Config)] {
			continue
		}
		keys = insertAfterPredecessor(keys, id, shippedFile.Params.keys[:i])
		mine.Params.values[id] = theirs
		taken[normalConfig(theirs.Config)] = true
		changed = true
	}
	if !changed {
		return nil
	}
	mine.Params.keys = keys
	content, err := encode(mine, mine.enabled())
	if err != nil {
		return err
	}

	return s.writeFile(s.emulatorFile(ref), content)
}

// withMissing — les options de `mine`, plus celles de `theirs` qui lui
// manquent, chacune à sa place dans l'ordre de `theirs`.
func withMissing(mine, theirs orderedMap[string]) (orderedMap[string], bool) {
	keys := append([]string(nil), mine.keys...)
	values := map[string]string{}
	for key, value := range mine.values {
		values[key] = value
	}
	added := false
	for i, key := range theirs.keys {
		if _, present := values[key]; present {
			continue
		}
		keys = insertAfterPredecessor(keys, key, theirs.keys[:i])
		values[key] = theirs.values[key]
		added = true
	}

	return orderedMap[string]{keys: keys, values: values}, added
}

// insertAfterPredecessor — `key` posée juste après le plus proche de ses
// prédécesseurs (dans l'ordre livré) que `keys` porte déjà ; en tête s'il
// n'y en a aucun.
func insertAfterPredecessor(keys []string, key string, predecessors []string) []string {
	at := 0
	for j := len(predecessors) - 1; j >= 0; j-- {
		if index := slices.Index(keys, predecessors[j]); index >= 0 {
			at = index + 1

			break
		}
	}

	return slices.Insert(keys, at, key)
}

// pruneRetiredLocked — CE QUE GAMEFLEX NE LIVRE PLUS PART (2026-10-16). Un
// émulateur retiré (Eden, journal 174) laissait dans la bibliothèque son
// fichier de paramètres et ses préréglages livrés, qu'aucun écran ne montre
// plus et qu'aucun lancement n'applique. Partent le fichier d'un émulateur
// que sa console ne livre plus, et un préréglage que GameFlex avait livré
// (`"override": false`, écrit en toutes lettres) et ne livre plus. Un
// préréglage de l'admin ou d'un ami (`override` vrai ou absent), ou
// illisible, ne se touche jamais.
func (s Store) pruneRetiredLocked() []string {
	consoles, err := os.ReadDir(filepath.Join(s.Root, "params"))
	if err != nil {
		return nil
	}
	shippedRefs := map[Ref]bool{}
	for _, ref := range Shipped() {
		shippedRefs[ref] = true
	}
	var failures []string
	remove := func(target string) {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err.Error())
		}
	}
	for _, console := range consoles {
		if !console.IsDir() {
			continue
		}
		consoleID := console.Name()
		entries, _ := os.ReadDir(filepath.Join(s.Root, "params", consoleID))
		for _, entry := range entries {
			emulator := strings.TrimSuffix(entry.Name(), ".json")
			if entry.IsDir() || emulator == entry.Name() || shippedRefs[Ref{ConsoleID: consoleID, Emulator: emulator}] {
				continue
			}
			remove(filepath.Join(s.Root, "params", consoleID, entry.Name()))
		}

		names, _ := shippedPresets(consoleID)
		stillShipped := map[string]bool{}
		for _, name := range names {
			stillShipped[name] = true
		}
		presets, _ := os.ReadDir(s.presetsDir(consoleID))
		for _, entry := range presets {
			id := strings.TrimSuffix(entry.Name(), ".json")
			if entry.IsDir() || id == entry.Name() || stillShipped[id] {
				continue
			}
			target := filepath.Join(s.presetsDir(consoleID), entry.Name())
			content, err := os.ReadFile(target)
			if err != nil {
				continue
			}
			var p preset
			if json.Unmarshal(content, &p) != nil || p.Override == nil || *p.Override {
				continue
			}
			remove(target)
		}
	}

	return failures
}

// readLocked — le fichier de l'émulateur : absent (exists faux), illisible
// (erreur), ou lu.
func (s Store) readLocked(ref Ref) (f file, exists bool, err error) {
	content, err := os.ReadFile(s.emulatorFile(ref))
	if errors.Is(err, os.ErrNotExist) {
		return file{}, false, nil
	}
	if err != nil {
		return file{}, true, err
	}
	f, err = decode(content)
	if err != nil {
		return file{}, true, fmt.Errorf("%s is unreadable: %w", ref.Emulator+".json", err)
	}

	return f, true, nil
}

// definitionLocked — ce que le formulaire montre : le fichier de l'émulateur
// s'il se lit, la liste livrée sinon — absent, l'enregistrer le crée ;
// illisible, l'enregistrer le remplace.
func (s Store) definitionLocked(ref Ref) (definition file, exists bool, err error) {
	definition, exists, err = s.readLocked(ref)
	if err != nil || !exists {
		definition, _ = shippedDefinition(ref)
	}

	return definition, exists, err
}

// Entry — une valeur à écrire, à son adresse.
type Entry struct {
	Config string `json:"config"`
	Value  string `json:"value"`
}

// Settings — ce qu'un lancement doit appliquer à un émulateur.
//
// State vaut "enabled" (Entries dans l'ordre du fichier), "absent" (pas de
// fichier) ou "disabled" (`enabled` à faux) — et alors Entries est vide : ce
// que le fichier avait posé est rendu à l'émulateur —, ou "error", quand le
// fichier ne se lit pas : rien n'est alors touché, et l'émulateur garde ce
// que le lancement précédent lui avait écrit.
type Settings struct {
	Emulator string  `json:"emulator"`
	State    string  `json:"state"`
	Entries  []Entry `json:"entries"`
	Error    string  `json:"error,omitempty"`
	// Engine, EnginePreset — le moteur du jeu dont le préréglage est passé
	// par-dessus (SettingsFor, chantier 36) ; EngineError — pourquoi il ne
	// l'a pas été.
	Engine       string `json:"engine,omitempty"`
	EnginePreset string `json:"enginePreset,omitempty"`
	EngineError  string `json:"engineError,omitempty"`
}

// Settings — lu dans params/<console>/<émulateur>.json.
func (s Store) Settings(ref Ref) Settings {
	settings := Settings{Emulator: ref.Emulator, Entries: []Entry{}}
	mu.Lock()
	f, exists, err := s.readLocked(ref)
	mu.Unlock()
	switch {
	case err != nil:
		settings.State, settings.Error = "error", err.Error()
	case !exists:
		settings.State = "absent"
	case !f.enabled():
		settings.State = "disabled"
	default:
		settings.State = "enabled"
		for _, id := range f.Params.keys {
			p := f.Params.values[id]
			value, _ := scalarText(p.Value)
			settings.Entries = append(settings.Entries, Entry{Config: normalConfig(p.Config), Value: value})
		}
	}

	return settings
}

// LA FORME DE L'API — des LISTES et non des objets : un navigateur relit
// `{"50": …, "auto": …}` en mettant les clés numériques devant, et l'ordre des
// options serait perdu en route.

// Option — un choix d'une liste.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Param — un champ du formulaire.
type Param struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Config      string   `json:"config"`
	Value       any      `json:"value"`
	Options     []Option `json:"options,omitempty"`
}

// Preset — un préréglage tel que le formulaire le prend : ses valeurs déjà
// passées par les paramètres qu'elles remplissent. Celles qui ne remplissent
// rien sont dites, pas jetées en silence.
type Preset struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	File        string         `json:"file"`
	Values      map[string]any `json:"values"`
	Ignored     []string       `json:"ignored,omitempty"`
	Error       string         `json:"error,omitempty"`
	// Override — se remplace et se supprime depuis l'écran. Faux pour ceux
	// que GameFlex livre : en lecture seule (décidé par le porteur le
	// 2026-10-12), on en part pour en créer un sous un autre nom.
	Override bool `json:"override"`
}

// Emulator — un émulateur tel que l'écran le montre.
type Emulator struct {
	ConsoleID    string   `json:"consoleId"`
	Console      string   `json:"console"`
	Emulator     string   `json:"emulator"`
	File         string   `json:"file"`
	Exists       bool     `json:"exists"`
	Enabled      bool     `json:"enabled"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	System       string   `json:"system"`
	Requirements []string `json:"requirements"`
	Params       []Param  `json:"params"`
	Presets      []Preset `json:"presets"`
	Error        string   `json:"error,omitempty"`
	// SavedPreset — le préréglage que cet enregistrement a écrit, s'il en a
	// écrit un (« Save as preset »).
	SavedPreset string `json:"savedPreset,omitempty"`
	// Engines — le préréglage que chaque moteur de jeu pose (chantier 36) ;
	// aucun pour une console dont les jeux n'ont pas de moteur reconnu.
	Engines []EngineSetting `json:"engines,omitempty"`
}

// presetsLocked — les préréglages du dossier presets/ de la console qui
// visent cet émulateur (`system`), livrés ou écrits à la main, celui par
// défaut en tête. Un préréglage illisible reste dans la liste, avec son
// erreur.
//
// LE DÉFAUT D'UN SECOND ÉMULATEUR porte son nom devant (`eden-default.json`,
// chantier 26) : le dossier est celui de la console, et `default.json` y est
// déjà celui du premier.
func (s Store) presetsLocked(ref Ref, definition file) []Preset {
	names, _ := filepath.Glob(filepath.Join(s.presetsDir(ref.ConsoleID), "*.json"))
	isDefault := func(name string) bool {
		base := filepath.Base(name)

		return base == "default.json" || strings.HasSuffix(base, "-default.json")
	}
	sort.Slice(names, func(i, j int) bool {
		iDefault := isDefault(names[i])
		jDefault := isDefault(names[j])
		if iDefault != jDefault {
			return iDefault
		}

		return names[i] < names[j]
	})
	shippedIDs := shippedPresetIDs(ref.ConsoleID)
	presets := []Preset{}
	for _, name := range names {
		item := Preset{ID: strings.TrimSuffix(filepath.Base(name), ".json"), File: name, Values: map[string]any{}}
		item.Name = item.ID
		var p preset
		content, err := os.ReadFile(name)
		if err == nil {
			err = json.Unmarshal(content, &p)
		}
		if err != nil {
			item.Error = "unreadable preset: " + err.Error()
			presets = append(presets, item)

			continue
		}
		if !strings.EqualFold(p.System, definition.System) {
			continue
		}
		if p.Name != "" {
			item.Name = p.Name
		}
		item.Description = p.Description
		item.Override = p.overridable() && !shippedIDs[item.ID]
		ids := make([]string, 0, len(p.Values))
		for id := range p.Values {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			target, known := definition.Params.get(id)
			if !known {
				item.Ignored = append(item.Ignored, id)

				continue
			}
			value, err := target.accept(p.Values[id])
			if err != nil {
				item.Ignored = append(item.Ignored, target.Label)

				continue
			}
			item.Values[id] = value
		}
		presets = append(presets, item)
	}

	return presets
}

// withShippedTexts — les textes que GameFlex livre, à la place de ceux du
// fichier, pour chaque paramètre qu'il livre lui-même (même identifiant, même
// type, même adresse). Les textes livrés sont en anglais (« il faut seulement
// que ça soit en anglais par défaut », le porteur, 2026-10-12) ; un fichier
// posé par une version précédente portait les siens en français, et une page
// en anglais mélangeait les deux langues. Ce ne sont que des textes : les
// valeurs, l'interrupteur et les adresses restent ceux du fichier, qui fait
// foi. Un paramètre ajouté à la main garde les siens — ils ne se traduisent
// pas.
func withShippedTexts(definition file, ref Ref) file {
	shippedFile, _ := shippedDefinition(ref)
	if !strings.EqualFold(definition.System, shippedFile.System) {
		return definition
	}
	definition.Name, definition.Description = shippedFile.Name, shippedFile.Description
	params := orderedMap[param]{}
	for _, id := range definition.Params.keys {
		mine := definition.Params.values[id]
		theirs, known := shippedFile.Params.get(id)
		if known && theirs.Type == mine.Type && normalConfig(theirs.Config) == normalConfig(mine.Config) {
			mine.Label, mine.Description = theirs.Label, theirs.Description
			if mine.Options != nil && theirs.Options != nil {
				options := orderedMap[string]{}
				for _, value := range mine.Options.keys {
					text := mine.Options.values[value]
					if shippedText, ok := theirs.Options.get(value); ok {
						text = shippedText
					}
					options.set(value, text)
				}
				mine.Options = &options
			}
		}
		params.set(id, mine)
	}
	definition.Params = params

	return definition
}

// describeLocked — voir Describe.
func (s Store) describeLocked(ref Ref, consoleName string) Emulator {
	definition, exists, err := s.definitionLocked(ref)
	definition = withShippedTexts(definition, ref)
	item := Emulator{
		ConsoleID:    ref.ConsoleID,
		Console:      consoleName,
		Emulator:     ref.Emulator,
		File:         s.emulatorFile(ref),
		Exists:       exists,
		Enabled:      exists && err == nil && definition.enabled(),
		Name:         definition.Name,
		Description:  definition.Description,
		System:       definition.System,
		Requirements: []string{},
		Params:       []Param{},
		Presets:      s.presetsLocked(ref, definition),
		Engines:      s.enginesLocked(ref, definition),
	}
	if err != nil {
		item.Error = err.Error()
	}
	if definition.Requirements != nil {
		item.Requirements = definition.Requirements
	}
	for _, id := range definition.Params.keys {
		p := definition.Params.values[id]
		field := Param{
			ID: id, Type: p.Type, Label: p.Label, Description: p.Description,
			Config: normalConfig(p.Config), Value: p.Value,
		}
		if p.Options != nil {
			for _, value := range p.Options.keys {
				field.Options = append(field.Options, Option{Value: value, Label: p.Options.values[value]})
			}
		}
		item.Params = append(item.Params, field)
	}

	return item
}

// Describe — un émulateur tel que l'écran le montre : son fichier (ou la
// liste livrée), et ses préréglages.
func (s Store) Describe(ref Ref, consoleName string) Emulator {
	mu.Lock()
	defer mu.Unlock()

	return s.describeLocked(ref, consoleName)
}

// RefusedError — des valeurs que leurs paramètres refusent ; rien n'a été
// écrit.
type RefusedError struct{ Reasons []string }

func (e *RefusedError) Error() string {
	return strings.Join(e.Reasons, "; ")
}

// Save — l'interrupteur et les valeurs du formulaire, écrits dans le fichier
// de l'émulateur (créé s'il manquait, remplacé s'il était illisible). Aucune
// valeur n'est posée si une seule est refusée.
//
// `presetName` — « Save as preset » : les mêmes valeurs, gardées aussi en
// préréglage sous ce nom (`presets/<slug>.json`, `"override": true`), pour y
// revenir plus tard. Le même nom remplace le préréglage qui le porte, s'il se
// remplace ; celui d'un préréglage en lecture seule — ceux de GameFlex — est
// refusé : on en part pour en créer un autre. Refusé, rien n'est écrit, pas
// même le fichier de l'émulateur.
//
// `engines` — le préréglage que chaque moteur de jeu pose, et son
// interrupteur (chantier 36) ; nil n'y touche pas.
func (s Store) Save(ref Ref, consoleName string, enabled *bool, values map[string]any, presetName string,
	engines []EngineSetting) (Emulator, error) {
	mu.Lock()
	defer mu.Unlock()

	f, exists, readErr := s.definitionLocked(ref)
	presetName = strings.TrimSpace(presetName)
	presetID := ""
	if presetName != "" {
		id, err := s.customPresetLocked(ref, f, presetName)
		if err != nil {
			return Emulator{}, err
		}
		presetID = id
	}
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var refused []string
	for _, id := range ids {
		target, known := f.Params.get(id)
		if !known {
			refused = append(refused, id+": unknown parameter")

			continue
		}
		value, err := target.accept(values[id])
		if err != nil {
			refused = append(refused, target.Label+": "+err.Error())

			continue
		}
		target.Value = value
		f.Params.set(id, target)
	}
	if len(refused) > 0 {
		return Emulator{}, &RefusedError{Reasons: refused}
	}
	if err := s.checkEnginesLocked(ref, f, engines, presetID); err != nil {
		return Emulator{}, err
	}
	on := !exists || readErr != nil || f.enabled()
	if enabled != nil {
		on = *enabled
	}
	encoded, err := encode(f, on)
	if err == nil {
		err = s.writeFile(s.emulatorFile(ref), encoded)
	}
	if err == nil && presetID != "" {
		err = s.writePresetLocked(ref, f, presetID, presetName)
	}
	if err == nil {
		err = s.writeEnginesLocked(ref, f, engines)
	}
	if err != nil {
		return Emulator{}, err
	}

	item := s.describeLocked(ref, consoleName)
	item.SavedPreset = presetID

	return item, nil
}

// ErrNoPreset — ce préréglage n'existe pas pour cet émulateur.
var ErrNoPreset = errors.New("no such preset")

// DeletePreset — supprime un préréglage qui se remplace (`override`). Ceux en
// lecture seule sont refusés — ceux de GameFlex reviendraient au démarrage
// suivant, et le bouton aurait menti.
func (s Store) DeletePreset(ref Ref, consoleName, id string) (Emulator, error) {
	mu.Lock()
	defer mu.Unlock()

	// UN IDENTIFIANT EST UN SLUG, et rien d'autre : il devient un nom de
	// fichier, et « ../rpcs3 » désignerait le fichier de l'émulateur.
	if id == "" || Slug(id) != id {
		return Emulator{}, ErrNoPreset
	}
	definition, _, _ := s.definitionLocked(ref)
	target := filepath.Join(s.presetsDir(ref.ConsoleID), id+".json")
	found, mine := s.presetOfLocked(target, definition)
	if !mine {
		return Emulator{}, ErrNoPreset
	}
	if !found.overridable() || shippedPresetIDs(ref.ConsoleID)[id] {
		return Emulator{}, &RefusedError{Reasons: []string{"this preset is read-only: it cannot be deleted"}}
	}
	if err := os.Remove(target); err != nil {
		return Emulator{}, err
	}

	return s.describeLocked(ref, consoleName), nil
}

// customPresetLocked — l'identifiant sous lequel ranger un préréglage de
// l'admin, ou le refus qui dit pourquoi pas.
func (s Store) customPresetLocked(ref Ref, definition file, name string) (string, error) {
	id := Slug(name)
	if id == "" {
		return "", &RefusedError{Reasons: []string{"the preset name needs letters or digits"}}
	}
	readOnly := &RefusedError{Reasons: []string{"« " + name + " » is a read-only preset: choose another name"}}
	// UN PRÉRÉGLAGE DE GAMEFLEX, même effacé à la main : le démarrage suivant
	// le réécrirait par-dessus.
	if shippedPresetIDs(ref.ConsoleID)[id] {
		return "", readOnly
	}
	// LE DOSSIER EST CELUI DE LA CONSOLE, partagé par ses émulateurs : un
	// préréglage d'un autre émulateur ne s'écrase pas sous un nom qui
	// tomberait sur le même fichier.
	target := filepath.Join(s.presetsDir(ref.ConsoleID), id+".json")
	if _, err := os.Stat(target); err == nil {
		found, mine := s.presetOfLocked(target, definition)
		switch {
		case !mine:
			return "", &RefusedError{Reasons: []string{"« " + name + " » is already a preset of another emulator: choose another name"}}
		case !found.overridable():
			return "", readOnly
		}
	}

	return id, nil
}

// presetOfLocked — ce fichier, lu, et s'il est un préréglage de cet émulateur.
func (s Store) presetOfLocked(target string, definition file) (preset, bool) {
	content, err := os.ReadFile(target)
	if err != nil {
		return preset{}, false
	}
	var p preset
	if err := json.Unmarshal(content, &p); err != nil {
		return preset{}, false
	}

	return p, strings.EqualFold(p.System, definition.System)
}

// customPreset — un préréglage de l'admin, tel qu'il s'écrit : ses valeurs
// dans l'ordre du formulaire, comme ceux que GameFlex livre, et
// `"override": true` — il se remplace et se supprime.
type customPreset struct {
	Name     string          `json:"name"`
	Override bool            `json:"override"`
	System   string          `json:"system"`
	Values   orderedMap[any] `json:"values"`
}

// writePresetLocked — les valeurs du fichier, toutes, gardées en préréglage.
// Rien n'est réécrit quand le préréglage dit déjà la même chose.
func (s Store) writePresetLocked(ref Ref, f file, id, name string) error {
	values := orderedMap[any]{}
	for _, paramID := range f.Params.keys {
		values.set(paramID, f.Params.values[paramID].Value)
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(customPreset{Name: name, Override: true, System: f.System, Values: values}); err != nil {
		return err
	}
	target := filepath.Join(s.presetsDir(ref.ConsoleID), id+".json")
	if current, err := os.ReadFile(target); err == nil && bytes.Equal(current, out.Bytes()) {
		return nil
	}

	return s.writeFile(target, out.Bytes())
}

// slugAccents — les lettres accentuées qu'un nom français porte, rendues à
// leur lettre : « Réglage S25 » donne « reglage-s25 » et non « r-glage-s25 ».
var slugAccents = map[rune]string{
	'à': "a", 'â': "a", 'ä': "a", 'á': "a", 'ã': "a", 'å': "a",
	'ç': "c",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e",
	'î': "i", 'ï': "i", 'í': "i", 'ì': "i",
	'ô': "o", 'ö': "o", 'ó': "o", 'ò': "o", 'õ': "o",
	'ù': "u", 'û': "u", 'ü': "u", 'ú': "u",
	'ÿ': "y", 'ý': "y",
	'ñ': "n",
	'œ': "oe", 'æ': "ae",
}

// slugMax — un nom de fichier, pas une phrase.
const slugMax = 64

// Slug — le nom de fichier d'un préréglage, tiré de son nom : minuscules,
// lettres et chiffres, tout le reste en un tiret (« S25, trial 1: Giga » →
// « s25-trial-1-giga »).
func Slug(name string) string {
	var out strings.Builder
	pendingDash := false
	write := func(text string) {
		if pendingDash && out.Len() > 0 {
			out.WriteByte('-')
		}
		pendingDash = false
		out.WriteString(text)
	}
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			write(string(r))
		case slugAccents[r] != "":
			write(slugAccents[r])
		default:
			pendingDash = true
		}
	}
	slug := out.String()
	if len(slug) > slugMax {
		slug = strings.TrimRight(slug[:slugMax], "-")
	}

	return slug
}

// Shared — ce qu'un émulateur a de PROPRE à cette machine, et qu'un autre
// serveur peut lui emprunter (chantier 15) : des valeurs choisies, et des
// préréglages perso. Les préréglages livrés sont les mêmes partout, et ne
// s'y trouvent pas.
type Shared struct {
	Ref     Ref
	Values  bool
	Presets []Preset
}

// Inventory — pour chaque émulateur dont GameFlex livre les paramètres, ce
// qu'il a de propre ici ; un émulateur qui n'a rien de propre n'y figure pas.
func (s Store) Inventory() []Shared {
	mu.Lock()
	defer mu.Unlock()

	var shared []Shared
	for _, ref := range Shipped() {
		described := s.describeLocked(ref, "")
		entry := Shared{Ref: ref, Values: described.Exists && ownValues(ref, described)}
		for _, preset := range described.Presets {
			if preset.Override && preset.Error == "" {
				entry.Presets = append(entry.Presets, preset)
			}
		}
		if entry.Values || len(entry.Presets) > 0 {
			shared = append(shared, entry)
		}
	}

	return shared
}

// ownValues — le fichier d'un émulateur porte-t-il des valeurs CHOISIES ici ?
// Le téléphone le pose d'office avec les valeurs livrées : ce n'est pas un
// choix, et un ami n'aurait rien à y prendre.
func ownValues(ref Ref, described Emulator) bool {
	shipped, _ := shippedDefinition(ref)
	for _, field := range described.Params {
		original, known := shipped.Params.get(field.ID)
		if !known || fmt.Sprint(original.Value) != fmt.Sprint(field.Value) {
			return true
		}
	}

	return false
}

// Lent — ce qu'un émulateur prête à un ami (chantier 15) : ses valeurs
// choisies, ou un de ses préréglages perso — un nom, la machine qu'il vise,
// et des valeurs par paramètre.
type Lent struct {
	Name   string         `json:"name"`
	System string         `json:"system"`
	Values map[string]any `json:"values"`
}

// Lend — ce que cet émulateur prête : ses valeurs choisies (`presetID` vide),
// ou ce préréglage perso. ErrNoPreset pour tout le reste — un préréglage de
// GameFlex est le même chez l'ami, et des valeurs qui n'ont pas été choisies
// ici n'ont rien à lui apprendre.
func (s Store) Lend(ref Ref, presetID string) (Lent, error) {
	mu.Lock()
	defer mu.Unlock()

	described := s.describeLocked(ref, "")
	if presetID == "" {
		if !described.Exists || described.Error != "" || !ownValues(ref, described) {
			return Lent{}, ErrNoPreset
		}

		return Lent{Name: described.Emulator, System: described.System, Values: valuesOf(described.Params)}, nil
	}
	for _, preset := range described.Presets {
		if preset.ID == presetID && preset.Override && preset.Error == "" {
			return Lent{Name: preset.Name, System: described.System, Values: preset.Values}, nil
		}
	}

	return Lent{}, ErrNoPreset
}

func valuesOf(params []Param) map[string]any {
	values := map[string]any{}
	for _, field := range params {
		values[field.ID] = field.Value
	}

	return values
}

// ErrExists — un préréglage de ce nom est déjà ici : rien n'écrase rien.
var ErrExists = errors.New("this preset is already here")

// Import — des valeurs venues d'un ami, rangées ici en préréglage perso sous
// ce nom (`<serveur>-<préréglage>`, décidé par le porteur le 2026-10-13 : deux
// amis qui ont chacun un « hd » ne s'écrasent pas). Jamais par-dessus un
// préréglage qui existe ; seules les valeurs que les paramètres d'ici
// acceptent sont gardées, les autres sont rendues — dites, pas posées. Rend
// l'émulateur, avec l'identifiant du préréglage écrit (`SavedPreset`).
func (s Store) Import(ref Ref, consoleName, name string, values map[string]any) (Emulator, []string, error) {
	mu.Lock()
	defer mu.Unlock()

	definition, _, _ := s.definitionLocked(ref)
	id := Slug(name)
	if id == "" {
		return Emulator{}, nil, &RefusedError{Reasons: []string{"the preset name needs letters or digits"}}
	}
	if shippedPresetIDs(ref.ConsoleID)[id] {
		return Emulator{}, nil, &RefusedError{Reasons: []string{"« " + name + " » is a read-only preset: choose another name"}}
	}
	target := filepath.Join(s.presetsDir(ref.ConsoleID), id+".json")
	if _, err := os.Stat(target); err == nil {
		return Emulator{}, nil, ErrExists
	}

	kept := orderedMap[any]{}
	ignored := []string{}
	for _, paramID := range definition.Params.keys {
		value, given := values[paramID]
		if !given {
			continue
		}
		accepted, err := definition.Params.values[paramID].accept(value)
		if err != nil {
			ignored = append(ignored, paramID)

			continue
		}
		kept.set(paramID, accepted)
	}
	for paramID := range values {
		if _, known := definition.Params.get(paramID); !known {
			ignored = append(ignored, paramID)
		}
	}
	sort.Strings(ignored)
	if len(kept.keys) == 0 {
		return Emulator{}, ignored, &RefusedError{Reasons: []string{"none of these values fits this emulator here"}}
	}

	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(customPreset{Name: name, Override: true, System: definition.System, Values: kept}); err != nil {
		return Emulator{}, ignored, err
	}
	if err := s.writeFile(target, out.Bytes()); err != nil {
		return Emulator{}, ignored, err
	}

	item := s.describeLocked(ref, consoleName)
	item.SavedPreset = id

	return item, ignored, nil
}

// ImportName — le nom sous lequel ce qu'un ami prête entre ici : `<serveur>`
// pour ses valeurs, `<serveur>-<préréglage>` pour un préréglage (décision 10
// du chantier 15).
func ImportName(server, presetName string) string {
	if presetName == "" {
		return server
	}

	return server + "-" + presetName
}

// Offer — un prêt d'un ami, vu d'ici : le préréglage qu'il deviendrait (`as`),
// et s'il y est déjà.
type Offer struct {
	Emulator  string `json:"emulator"`
	Preset    string `json:"preset,omitempty"`
	Name      string `json:"name"`
	As        string `json:"as"`
	Installed bool   `json:"installed"`
}

// OfferOf — ce prêt de `server` : ses valeurs d'un émulateur (`presetID`
// vide), ou ce préréglage.
func (s Store) OfferOf(server, consoleID, emulator, presetID, presetName string) Offer {
	id := Slug(ImportName(server, presetName))
	_, err := os.Stat(filepath.Join(s.presetsDir(consoleID), id+".json"))
	name := presetName
	if presetID == "" {
		name = emulator
	}

	return Offer{Emulator: emulator, Preset: presetID, Name: name, As: id, Installed: err == nil}
}
