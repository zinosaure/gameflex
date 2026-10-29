package docker

import (
	"encoding/json"
	"log/slog"

	"gameflex/emuparams"
)

// emulatorParamsEnv — LES PARAMÈTRES DE L'ÉMULATEUR d'une session streamex
// (#347), lus dans `params/<console>/<émulateur>.json` du dossier et
// passés au conteneur : `GAMEFLEX_EMULATOR_PARAMS=<Settings en JSON>`.
//
// Lus ICI parce que le conteneur ne voit pas le dossier — celui de la
// PS3 ne monte que ses jeux, son firmware et ses propres dossiers. streamex
// les écrit dans le fichier de l'émulateur juste avant de le lancer, donc
// APRÈS les `sed` de l'entrypoint : un paramètre gagne.
//
// Toujours passés, même sans fichier ou désactivés : c'est ce qui permet à
// streamex de RENDRE à l'émulateur ce qu'un lancement précédent avait posé.
// Rien pour une console dont GameFlex ne livre pas de paramètres.
//
// CEUX DE L'ÉMULATEUR CHOISI (StreamexParams) : une console peut livrer les
// paramètres de plusieurs émulateurs, la session n'en fait tourner qu'un.
//
// ET CEUX DU MOTEUR DU JEU par-dessus (`engine`, chantier 36) : le préréglage
// que le moteur désigne, s'il est actif (emuparams.SettingsFor).
func emulatorParamsEnv(root, consoleID, core, engine string) (string, bool) {
	name, ok := StreamexParams(consoleID, core)
	ref := emuparams.Ref{ConsoleID: consoleID, Emulator: name}
	if !ok || !emuparams.IsShipped(ref) {
		return "", false
	}
	settings := emuparams.Store{Root: root}.SettingsFor(ref, engine)
	encoded, err := json.Marshal(settings)
	if err != nil {
		return "", false
	}
	slog.Info("paramètres de l'émulateur transmis", "console", consoleID, "émulateur", ref.Emulator,
		"état", settings.State, "valeurs", len(settings.Entries), "erreur", settings.Error,
		"moteur", settings.Engine, "préréglage du moteur", settings.EnginePreset, "erreur du moteur", settings.EngineError)

	return "GAMEFLEX_EMULATOR_PARAMS=" + string(encoded), true
}
