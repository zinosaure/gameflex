package emuparams

// LES RÉGLAGES D'UN JEU SONT DES CORRECTIFS (chantier 43, #630) — « on ne va pas
// rajouter des lignes dans le réglage pour les milliers de jeux qui existent » (le
// porteur, 2026-11-05). Ils vivent où vivent les correctifs :
//
//	patches/<console>/<identifiant>/settings.json
//
// et se listent, se suppriment et se rapatrient comme eux : l'écran Correctifs, la
// fédération, le miroir. Rien ici ne parcourt de liste : un lancement lit le seul
// fichier de son jeu. Sans lui, le jeu suit la fiche de l'émulateur et son moteur.
//
// L'identifiant (CUSA…, PPSA…, 0100D1B006744000 pour la Switch), chaque lanceur le
// lit au jeu (storage.GameTitleID) : un nom de dossier change d'un joueur à l'autre.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// gamePatchFile — le fichier d'un correctif qui porte des réglages.
const gamePatchFile = "settings.json"

// gamePattern — la forme d'un identifiant : il devient un nom de dossier. Celui
// d'un jeu PlayStation, ou les 16 chiffres hexadécimaux d'un titre Switch.
var gamePattern = regexp.MustCompile(`^(?:[A-Z]{4}[0-9]{5}|[0-9A-F]{16})$`)

func validGame(game string) bool { return gamePattern.MatchString(game) }

// gamePatch — `settings.json`. `name` et `description` sont pour qui le lit : ce
// qui a été mesuré, avec la date.
type gamePatch struct {
	Name        string         `json:"name,omitempty"`
	System      string         `json:"system"`
	Description string         `json:"description,omitempty"`
	Values      map[string]any `json:"values"`
}

// gamePatchValues — ce que le correctif de ce jeu pose sur cet émulateur : celui de
// la bibliothèque, sinon celui d'un disque interne (PatchDirs). Un fichier d'un
// autre émulateur (`system`) ne le concerne pas. nil : aucun correctif.
func (s Store) gamePatchValues(definition file, consoleID, game string) (map[string]any, error) {
	for _, dir := range append([]string{filepath.Join(s.Root, "patches")}, s.PatchDirs...) {
		content, err := os.ReadFile(filepath.Join(dir, consoleID, game, gamePatchFile))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		var patch gamePatch
		if err == nil {
			err = json.Unmarshal(content, &patch)
		}
		if err != nil {
			return nil, fmt.Errorf("game patch %s: unreadable: %w", game, err)
		}
		if !strings.EqualFold(patch.System, definition.System) {
			continue
		}

		return patch.Values, nil
	}

	return nil, nil
}
