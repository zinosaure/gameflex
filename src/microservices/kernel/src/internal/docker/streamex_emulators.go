package docker

import (
	"context"
	"sort"
	"strings"
)

// LES ÉMULATEURS DE CHAQUE CONSOLE, ET L'IMAGE DE CHACUN.
//
// Le registre (gameflex/consoles) dit les émulateurs que GameFlex connaît pour
// une console ; un hôte n'en fait tourner qu'une partie. Le téléphone a ses
// invités glibc (GlibcGuest), ce serveur une image par émulateur — une image
// porte UN émulateur.
//
// UNE CONSOLE PEUT EN AVOIR DEUX (chantier 26, 2026-10-23) : la Switch joue
// par Citron (le premier, le défaut) ou par Eden (le second — les jeux du
// firmware 22 que Citron ne lit pas). L'ordre est celui du registre ; la fiche
// d'un jeu choisit, et un lancement sans choix prend le premier.
//
// Un émulateur que le registre déclare et qui n'est pas ici — Eden du
// 2026-10-14 au 2026-10-16, quand il ne tournait que sur le téléphone — est
// grisé par la fiche et refusé au lancement, au lieu qu'on lance un autre à
// sa place. C'est cette table qui le permet.
//
// `core` est le nom du registre — ce que la fiche choisit ; `params` celui des
// paramètres de l'émulateur (`params/<console>/<params>.json`, gameflex/emuparams) ;
// `image` le nom de son image, sans tag (le service compose qui la construit
// porte le même) ; `data`, pour la Switch, où ses données se montent — les
// mêmes sauvegardes du compte pour les deux émulateurs, qui rangent leur NAND
// de la même façon (héritage yuzu).
type streamexEmulator struct {
	core   string
	params string
	image  string
	data   string
}

var streamexEmulators = map[string][]streamexEmulator{
	"switch": {
		{core: "Eden (streamex)", params: "eden", image: "streamex-switch-eden", data: "/root/.local/share/eden"},
		{core: "Citron (streamex)", params: "citron", image: "streamex-switch", data: "/root/.local/share/citron"},
	},
	"ps3":     {{core: "RPCS3 (streamex)", params: "rpcs3", image: "streamex-ps3"}},
	"xbox":    {{core: "xemu (streamex)", params: "xemu", image: "streamex-xbox"}},
	"wiiu":    {{core: "Cemu (streamex)", params: "cemu", image: "streamex-wiiu"}},
	"ps4":     {{core: "shadPS4 (streamex)", params: "shadps4", image: "streamex-ps4"}},
	"ps5":     {{core: "KytyPS5 (streamex)", params: "kyty", image: "streamex-ps5"}},
	"xbox360": {{core: "Xenia Canary (streamex)", params: "xenia", image: "streamex-xbox360"}},
}

// StreamexCores — les cœurs du registre que ce serveur fait tourner pour
// cette console, dans l'ordre : le premier est le défaut. Vide pour une
// console sans image.
func StreamexCores(consoleID string) []string {
	cores := []string{}
	for _, emulator := range streamexEmulators[consoleID] {
		cores = append(cores, emulator.core)
	}

	return cores
}

// streamexEmulatorFor — l'émulateur de ce cœur, ou le premier de la console
// quand aucun n'est demandé ; faux pour un cœur qu'aucune image ne porte.
func streamexEmulatorFor(consoleID, core string) (streamexEmulator, bool) {
	emulators := streamexEmulators[consoleID]
	if len(emulators) == 0 {
		return streamexEmulator{}, false
	}
	if core == "" {
		return emulators[0], true
	}
	for _, emulator := range emulators {
		if emulator.core == core {
			return emulator, true
		}
	}

	return streamexEmulator{}, false
}

// StreamexParams — le nom des paramètres de l'émulateur de ce cœur (le
// premier de la console quand aucun n'est demandé).
func StreamexParams(consoleID, core string) (string, bool) {
	emulator, ok := streamexEmulatorFor(consoleID, core)

	return emulator.params, ok
}

// LE FICHIER QU'UNE IMAGE ATTEND (chantier 29, 2026-10-24).
//
// L'image d'une console ne porte plus son émulateur : elle le déballe au
// lancement depuis `cores/<console>/` (consoles/unpack-emulator.sh), ce qui
// permet de la publier. Elle dit lequel par trois étiquettes : le motif qui
// reconnaît ses versions (le nom de l'éditeur, sa version remplacée par `*`),
// et la version testée avec elle — son nom et son adresse chez l'éditeur, que
// son Dockerfile tire de consoles/emulators.sh. Elles se lisent SUR L'IMAGE, et
// non dans une table recopiée ici : c'est l'image installée qui sait ce
// qu'elle sait déballer.
const (
	labelEmulatorFile    = "gameflex.emulator.file"
	labelEmulatorURL     = "gameflex.emulator.url"
	labelEmulatorPattern = "gameflex.emulator.pattern"
	labelEmulatorTested  = "gameflex.emulator.tested"
)

// RequiredEmulator — l'émulateur qu'une image installée attend. `Console` est
// son dossier, `cores/<Console>/` : celui de la Switch pour Eden comme pour
// Citron. Une partie prend le fichier qui répond à `Pattern` et qu'on a déposé
// en dernier (le porteur, 2026-10-24) ; `File` et `URL` sont la plus récente des
// versions éprouvées, celle qu'on télécharge quand il n'y en a aucune. `Tested`
// les liste toutes : elle informe, elle ne bloque pas.
type RequiredEmulator struct {
	Console string
	Core    string
	Pattern string
	File    string
	URL     string
	Tested  []string
}

// RequiredEmulators — ceux des images installées ici, par console puis dans
// l'ordre du registre. Une image absente n'attend rien : c'est elle qu'il faut
// d'abord tirer ou construire, et son lancement le dit déjà.
func (dockerClient *Client) RequiredEmulators(ctx context.Context) []RequiredEmulator {
	consoleIDs := make([]string, 0, len(streamexEmulators))
	for consoleID := range streamexEmulators {
		consoleIDs = append(consoleIDs, consoleID)
	}
	sort.Strings(consoleIDs)

	required := []RequiredEmulator{}
	for _, consoleID := range consoleIDs {
		for _, emulator := range streamexEmulators[consoleID] {
			if need, ok := dockerClient.requiredBy(ctx, consoleID, emulator.core); ok {
				required = append(required, need)
			}
		}
	}

	return required
}

// RequiredEmulatorFor — le fichier qu'attend l'image d'un lancement (le premier
// émulateur de la console quand aucun n'est choisi). Faux sans image, ou pour
// une image qui n'en attend aucun (keytest).
func (dockerClient *Client) RequiredEmulatorFor(ctx context.Context, consoleID, core string) (RequiredEmulator, bool) {
	emulator, ok := streamexEmulatorFor(consoleID, core)
	if !ok {
		return RequiredEmulator{}, false
	}

	return dockerClient.requiredBy(ctx, consoleID, emulator.core)
}

func (dockerClient *Client) requiredBy(ctx context.Context, consoleID, core string) (RequiredEmulator, bool) {
	image := sessionImage(dockerClient.config, consoleID, core)
	if image == "" {
		return RequiredEmulator{}, false
	}
	inspected, _, err := dockerClient.dockerClient.ImageInspectWithRaw(ctx, image)
	if err != nil || inspected.Config == nil {
		return RequiredEmulator{}, false
	}

	return requiredFromLabels(consoleID, core, inspected.Config.Labels)
}

// requiredFromLabels — ce que disent les étiquettes d'une image.
func requiredFromLabels(consoleID, core string, labels map[string]string) (RequiredEmulator, bool) {
	file := labels[labelEmulatorFile]
	if file == "" {
		return RequiredEmulator{}, false
	}
	pattern := labels[labelEmulatorPattern]
	if pattern == "" {
		pattern = file
	}

	tested := strings.Fields(labels[labelEmulatorTested])
	if len(tested) == 0 {
		tested = []string{file}
	}

	return RequiredEmulator{
		Console: consoleID, Core: core, Pattern: pattern, File: file, URL: labels[labelEmulatorURL], Tested: tested,
	}, true
}
