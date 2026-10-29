package consoles

import "strings"

// LES RÉGLAGES DE FLUX CHOISIS AVANT DE LANCER — une seule table pour les
// deux kernels (2026-10-11).
//
// Le panneau (`stream/shared/StreamSettings.jsx`) parle le même vocabulaire
// aux trois plateformes : « 540p » … « 1080p », 30 ou 60 images par seconde,
// un débit en kbps. Le serveur le traduisait seul (`manager.resolutionEnvs`),
// et le téléphone l'ignorait : une PS3 lancée sur le S25 restait en 720p à 60
// images quoi qu'on choisisse. Plutôt que d'écrire une seconde table là-bas,
// elle vit ici — le serveur avait déjà vu deux tables écrites par nous dériver
// (360p et 540p manquaient d'un côté, 480p de l'autre).
//
// 360p A ÉTÉ RETIRÉ PARTOUT le 2026-10-26 (le porteur : « de nos jours 540p est
// assez bien partout ») ; une préférence encore en « 360p » est ignorée, comme
// une en « 480p ».
//
// 480p A ÉTÉ RETIRÉ PARTOUT le 2026-09-20, sur décision de l'utilisateur.
//
// Un conteneur sait rendre du 640x480 ; une PS5, non — elle n'ouvre que ses
// quatre modes et refuse la partie sinon, en le disant elle-même (« AvCap
// failed to initialize video »). Plutôt que de garder un réglage qui marche
// d'un côté et tue une partie de l'autre, l'échelle est la MÊME pour les trois
// plateformes. Une entrée déjà enregistrée en « 480p » ne casse rien :
// `StreamSize` ne la connaît pas, et la console garde sa géométrie d'époque.
var streamSizes = map[string][2]int{
	"540p":  {960, 540},
	"720p":  {1280, 720},
	"1080p": {1920, 1080},
}

// StreamSize rend la taille d'un libellé du panneau, ou ok=false pour un
// libellé vide ou inconnu : l'appelant garde alors son propre défaut.
func StreamSize(label string) (width, height int, ok bool) {
	size, ok := streamSizes[strings.ToLower(strings.TrimSpace(label))]

	return size[0], size[1], ok
}

// StreamFPS rend la cadence demandée si elle tient entre 15 et 60 images par
// seconde, 0 sinon (le défaut de l'appelant).
func StreamFPS(fps int) int {
	if fps < 15 || fps > 60 {
		return 0
	}

	return fps
}

// StreamBitrate rend le débit demandé, en kbps, s'il tient entre 500 et
// 20 000, 0 sinon (« Auto » : l'encodeur décide).
func StreamBitrate(kbps int) int {
	if kbps < 500 || kbps > 20000 {
		return 0
	}

	return kbps
}

// appLanguages — les langues de l'application (webapp, i18n/index.js), les
// seules qu'une console reçoit au lancement (chantier 21). Une langue de plus
// dans l'application s'ajoute ici, puis dans la table de chaque émulateur.
var appLanguages = map[string]bool{"en": true, "fr": true}

// Language — la langue d'un lancement, ramenée à son code (« fr-FR » → « fr »),
// vide pour une langue que l'application ne connaît pas : la console garde
// alors la sienne. La console démarre par défaut dans la langue de la page qui
// la lance (le porteur, 2026-10-22 : « il faut que ça soit par défaut la
// langue de l'application. Car sinon c'est par défaut en anglais »).
func Language(raw string) string {
	code := strings.ToLower(strings.TrimSpace(raw))
	if base, _, found := strings.Cut(code, "-"); found {
		code = base
	}
	if appLanguages[code] {
		return code
	}

	return ""
}
