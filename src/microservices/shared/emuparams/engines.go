package emuparams

// LES RÉGLAGES D'UN MOTEUR DE JEU (chantier 36, #516) — « mettre des
// préréglages pour les différents moteurs […] il faut qu'on sache avant de
// lancer le jeu c'est quoi le moteur pour adapter le réglage » (le porteur,
// 2026-10-27).
//
// UN MOTEUR DÉSIGNE UN PRÉRÉGLAGE de l'émulateur : `params/<console>/engines/
// <moteur>.json` dit lequel (`preset`), et s'il s'applique (`enabled`). Ses
// valeurs s'écrivent, à chaque partie d'un jeu de ce moteur, PAR-DESSUS celles
// du fichier de l'émulateur — le moteur gagne (décidé par le porteur : il ne
// porte que des réglages mesurés) ; l'admin le coupe d'un interrupteur. Le
// préréglage se crée et s'édite comme les autres (« Save as preset ») : rien
// de neuf à apprendre, et ce qu'un moteur pose se lit dans la liste.
//
// Le moteur d'un jeu, chaque lanceur le reconnaît à ses fichiers
// (storage.GameEngine) et le passe à SettingsFor.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// engineNames — les moteurs que storage.GameEngine reconnaît, sous les mêmes
// noms, dans l'ordre de l'écran.
var engineNames = []struct{ id, name string }{
	{"unity", "Unity"},
	{"unreal", "Unreal Engine"},
	{"frostbite", "Frostbite"},
	{"rage", "RAGE"},
}

// engineConsoles — les consoles dont les jeux ont un moteur reconnu
// (storage.GameEngine : pas la PS3, dont les jeux sont des `.iso`).
var engineConsoles = map[string]bool{"ps4": true, "ps5": true}

// EngineSetting — ce qu'un moteur pose sur cet émulateur : le préréglage dont
// il prend les valeurs ("" : aucun), et l'interrupteur qui le coupe.
type EngineSetting struct {
	Engine  string `json:"engine"`
	Name    string `json:"name"`
	Preset  string `json:"preset"`
	Enabled bool   `json:"enabled"`
	Error   string `json:"error,omitempty"`
}

// engineFile — `params/<console>/engines/<moteur>.json`.
type engineFile struct {
	System  string `json:"system"`
	Preset  string `json:"preset"`
	Enabled *bool  `json:"enabled,omitempty"`
}

func (s Store) engineFilePath(consoleID, engine string) string {
	return filepath.Join(s.Root, "params", consoleID, "engines", engine+".json")
}

// knownEngine — ce nom est-il celui d'un moteur reconnu ? Il devient un nom de
// fichier.
func knownEngine(engine string) (string, bool) {
	for _, known := range engineNames {
		if known.id == engine {
			return known.name, true
		}
	}

	return "", false
}

// engineLocked — le réglage d'un moteur pour cet émulateur. Un fichier d'un
// autre émulateur (`system`) ne le concerne pas : rien de posé.
func (s Store) engineLocked(ref Ref, definition file, engine, name string) EngineSetting {
	setting := EngineSetting{Engine: engine, Name: name, Enabled: true}
	content, err := os.ReadFile(s.engineFilePath(ref.ConsoleID, engine))
	if errors.Is(err, os.ErrNotExist) {
		return setting
	}
	var f engineFile
	if err == nil {
		err = json.Unmarshal(content, &f)
	}
	if err != nil {
		setting.Error = "unreadable engine setting: " + err.Error()

		return setting
	}
	if f.System != "" && !strings.EqualFold(f.System, definition.System) {
		return setting
	}
	setting.Preset = f.Preset
	if f.Enabled != nil {
		setting.Enabled = *f.Enabled
	}

	return setting
}

// enginesLocked — les réglages des moteurs d'un émulateur, pour l'écran ; aucun
// pour une console dont les jeux n'ont pas de moteur reconnu.
func (s Store) enginesLocked(ref Ref, definition file) []EngineSetting {
	if !engineConsoles[ref.ConsoleID] {
		return nil
	}
	settings := make([]EngineSetting, 0, len(engineNames))
	for _, known := range engineNames {
		settings = append(settings, s.engineLocked(ref, definition, known.id, known.name))
	}

	return settings
}

// checkEnginesLocked — les réglages des moteurs, tels que le formulaire les
// envoie, vérifiés AVANT toute écriture. Un préréglage inconnu de cet émulateur
// est refusé : le moteur appliquerait des valeurs que personne ne voit.
// `saving` — le préréglage que ce même enregistrement écrit (« Save as
// preset ») : un moteur peut le désigner tout de suite.
func (s Store) checkEnginesLocked(ref Ref, definition file, engines []EngineSetting, saving string) error {
	if len(engines) == 0 {
		return nil
	}
	if !engineConsoles[ref.ConsoleID] {
		return &RefusedError{Reasons: []string{ref.ConsoleID + ": its games have no recognised engine"}}
	}
	presets := map[string]bool{saving: saving != ""}
	for _, item := range s.presetsLocked(ref, definition) {
		if item.Error == "" {
			presets[item.ID] = true
		}
	}
	var refused []string
	for _, setting := range engines {
		if _, ok := knownEngine(setting.Engine); !ok {
			refused = append(refused, setting.Engine+": unknown engine")
		} else if setting.Preset != "" && !presets[setting.Preset] {
			refused = append(refused, setting.Engine+": unknown preset "+setting.Preset)
		}
	}
	if len(refused) > 0 {
		return &RefusedError{Reasons: refused}
	}

	return nil
}

// writeEnginesLocked — les réglages des moteurs, vérifiés, écrits.
func (s Store) writeEnginesLocked(ref Ref, definition file, engines []EngineSetting) error {
	for _, setting := range engines {
		enabled := setting.Enabled
		encoded, err := json.MarshalIndent(engineFile{System: definition.System, Preset: setting.Preset, Enabled: &enabled}, "", "  ")
		if err != nil {
			return err
		}
		if err := s.writeFile(s.engineFilePath(ref.ConsoleID, setting.Engine), append(encoded, '\n')); err != nil {
			return err
		}
	}

	return nil
}

// SettingsFor — ce qu'un lancement applique à cet émulateur pour un jeu de ce
// moteur : Settings, et par-dessus les valeurs du préréglage que le moteur
// désigne, s'il est actif. Elles gagnent sur celles du fichier ; et elles
// s'écrivent même quand le fichier est absent ou coupé — ce que le moteur pose
// n'en dépend pas. Un moteur inconnu, sans préréglage ou coupé : Settings tel
// quel. Un préréglage illisible ou disparu : Settings tel quel, et l'erreur
// dite (EngineError).
func (s Store) SettingsFor(ref Ref, engine string) Settings {
	settings := s.Settings(ref)
	name, known := knownEngine(engine)
	if !known || settings.State == "error" || !engineConsoles[ref.ConsoleID] {
		return settings
	}

	mu.Lock()
	defer mu.Unlock()
	definition, _, _ := s.definitionLocked(ref)
	setting := s.engineLocked(ref, definition, engine, name)
	if setting.Error != "" {
		settings.EngineError = setting.Error

		return settings
	}
	if !setting.Enabled || setting.Preset == "" {
		return settings
	}
	var chosen *Preset
	for _, item := range s.presetsLocked(ref, definition) {
		if item.ID == setting.Preset {
			chosen = &item

			break
		}
	}
	if chosen == nil || chosen.Error != "" {
		settings.EngineError = "engine " + engine + ": preset " + setting.Preset + " missing or unreadable"

		return settings
	}

	ids := make([]string, 0, len(chosen.Values))
	for id := range chosen.Values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		target, ok := definition.Params.get(id)
		if !ok {
			continue
		}
		value, ok := scalarText(chosen.Values[id])
		if !ok {
			continue
		}
		entry := Entry{Config: normalConfig(target.Config), Value: value}
		replaced := false
		for index := range settings.Entries {
			if settings.Entries[index].Config == entry.Config {
				settings.Entries[index] = entry
				replaced = true

				break
			}
		}
		if !replaced {
			settings.Entries = append(settings.Entries, entry)
		}
	}
	settings.Engine = engine
	settings.EnginePreset = setting.Preset

	return settings
}
