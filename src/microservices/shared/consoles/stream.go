package consoles

import "strings"

// LES RÉGLAGES DE FLUX CHOISIS AVANT DE LANCER — une seule table pour les
// deux kernels (2026-10-11).
//
// Le panneau (`stream/shared/StreamSettings.jsx`) parle le même vocabulaire
// aux trois plateformes : « 360p » … « 1440p », 30, 60 ou 120 images par seconde,
// un débit en kbps. Le serveur le traduisait seul (`manager.resolutionEnvs`),
// et le téléphone l'ignorait : une PS3 lancée sur le S25 restait en 720p à 60
// images quoi qu'on choisisse. Plutôt que d'écrire une seconde table là-bas,
// elle vit ici — le serveur avait déjà vu deux tables écrites par nous dériver
// (360p et 540p manquaient d'un côté, 480p de l'autre).
//
// 360p, RETIRÉ le 2026-10-26 (« de nos jours 540p est assez bien partout »),
// REVIENT le 2026-11-06 (#636) : un jeu que l'émulateur rend en 360p (une PS5 sur
// téléphone) n'a rien à gagner d'un flux plus grand, et le porteur le redemande.
// C'est aussi un des quatre modes de Remote Play.
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
	"360p":  {640, 360},
	"540p":  {960, 540},
	"720p":  {1280, 720},
	"1080p": {1920, 1080},
	// 1440p (le porteur, 2026-11-26 : « on devrait aussi proposer le flux 1440p et 120fps ») — le Remote Play s'arrête
	// à 1080p (RemotePlayResolution).
	"1440p": {2560, 1440},
}

// LE FLUX PAR DÉFAUT : 720p à 60 images par seconde, pour TOUTES les consoles —
// le rétro, les consoles de bureau, le Remote Play —, au serveur comme au
// téléphone (le porteur, 2026-11-23 : « Tu peux mettre pour tous le stream
// settings par défaut : 720p@60 »). Une définition ou une cadence absente, ou
// hors de l'échelle, prend celle-ci.
//
// C'était 540p à 30 depuis le 2026-11-16. À 30, une PS5 sous Kyty, qui rend 25 à
// 45 images irrégulières, perd celles qui passent 30 et attend jusqu'à deux
// périodes le tic du flux : des saccades (journal 808). À 60, chaque image part
// dans les 16,7 ms.
//
// Il n'y a plus de « défaut de l'appelant » : chacun avait le sien, et ils
// avaient dérivé — 720p à 60 pour le Remote Play et les consoles de bureau, la
// taille d'époque à 60 pour le rétro, alors que la page partait de 540p.
const (
	DefaultStreamResolution = "720p"
	DefaultStreamFPS        = 60
)

// StreamSize rend la taille d'un libellé du panneau, ou ok=false pour un
// libellé vide ou inconnu.
func StreamSize(label string) (width, height int, ok bool) {
	size, ok := streamSizes[strings.ToLower(strings.TrimSpace(label))]

	return size[0], size[1], ok
}

// StreamResolution rend le libellé d'un lancement à l'échelle (« 540P » →
// « 540p »), ou le défaut, 720p, pour un libellé vide ou inconnu (« 480p »).
func StreamResolution(label string) string {
	normalized := strings.ToLower(strings.TrimSpace(label))
	if _, ok := streamSizes[normalized]; ok {
		return normalized
	}

	return DefaultStreamResolution
}

// StreamFPS rend la cadence demandée si elle tient entre 15 et 120 images par
// seconde (120 depuis le 2026-11-26), le défaut (60) sinon.
func StreamFPS(fps int) int {
	if fps < 15 || fps > 120 {
		return DefaultStreamFPS
	}

	return fps
}

// LES PALIERS SOUS LE FLUX DE L'HÔTE — chantier 58 (le porteur, 2026-11-16 :
// « en mode que du downgrade. Si un jeu est lancé en 1080p alors on peut
// proposer 720 et 540. Si c'est lancé en 720 alors on propose que 540 et si
// c'est lancé en 540p alors pas d'autres propositions. La cadence suit la
// cadence originale »). Une partie encode, à côté de son flux, des flux plus
// petits pour ses invités — chacun seulement tant que quelqu'un le regarde.
//
// « On ne propose des adaptations de flux que si la console le propose aussi
// […] lié aux réglages de la console » : un palier ne dépasse jamais la
// hauteur que l'émulateur rend d'après ses réglages (renderedHeight). Une
// hauteur inconnue (0) vaut celle du flux : rien n'est inventé au-dessus.
var tierLadder = []int{1080, 720, 540}

// maxTierCount — deux paliers au plus (gatewayclient.MaxTiers) : 1080p et 720p
// sous un 1440p, 720p et 540p sous un 1080p.
const maxTierCount = 2

// TierHeights — les hauteurs des paliers d'un flux de cette hauteur, du plus
// grand au plus petit : les deux plus grandes de l'échelle strictement sous le
// flux, et pas au-dessus de ce que l'émulateur rend. Aucune sous 540p : un flux
// en 540p ou en 360p n'en a pas.
func TierHeights(streamHeight, renderedHeight int) []int {
	if renderedHeight <= 0 || renderedHeight > streamHeight {
		renderedHeight = streamHeight
	}
	var heights []int
	for _, height := range tierLadder {
		if len(heights) < maxTierCount && height < streamHeight && height <= renderedHeight {
			heights = append(heights, height)
		}
	}

	return heights
}

// StreamTier — un palier : sa taille, et son débit en kbps (0 : celui que
// l'encodeur déduit de l'image).
type StreamTier struct {
	Width, Height, Bitrate int
}

// StreamTiers — les paliers d'un flux width×height à kbps (TierHeights) : la
// largeur dans les mêmes proportions, paire, et un débit au rapport des
// surfaces, borné de 1 000 à 15 000 kbps — ou 0 quand le flux n'a pas de débit
// choisi. Une seule règle pour streamex et le téléphone.
func StreamTiers(width, height, kbps, renderedHeight int) []StreamTier {
	var tiers []StreamTier
	for _, tierHeight := range TierHeights(height, renderedHeight) {
		tierWidth := (width * tierHeight / height) &^ 1
		rate := 0
		if kbps > 0 {
			rate = max(1000, min(maxStreamBitrate, kbps*tierWidth*tierHeight/max(1, width*height)))
		}
		tiers = append(tiers, StreamTier{Width: tierWidth, Height: tierHeight, Bitrate: rate})
	}

	return tiers
}

// maxStreamBitrate — le plus haut débit d'un flux, en kbps : de quoi porter un
// 1440p à 120 images par seconde (20 000 jusqu'au 2026-11-26, le 1080p à 60).
const maxStreamBitrate = 40000

// StreamBitrate rend le débit demandé, en kbps, s'il tient entre 500 et
// 40 000, 0 sinon. « Auto » se mesure dans la page, au lancement : le débit
// entre le navigateur et le serveur de la partie, dont le flux prend une part
// (edge, `engine/stream/shared/libs/network-probe.js`), et il arrive ici comme
// un débit choisi. Zéro ne reste que quand la mesure a manqué : l'encodeur
// prend alors son propre repli.
func StreamBitrate(kbps int) int {
	if kbps < 500 || kbps > maxStreamBitrate {
		return 0
	}

	return kbps
}

// LE REMOTE PLAY S'ARRÊTE À 1080p ET 60 IMAGES PAR SECONDE : une PlayStation
// n'ouvre que ses modes (360p à 1080p, 30 ou 60) et refuse la partie sinon
// (« AvCap failed to initialize video »). Plus haut, elle prend son plus grand.

// RemotePlayResolution — StreamResolution, ramenée à 1080p au-delà.
func RemotePlayResolution(label string) string {
	resolution := StreamResolution(label)
	if _, height, _ := StreamSize(resolution); height > 1080 {
		return "1080p"
	}

	return resolution
}

// RemotePlayFPS — StreamFPS, ramenée à 60 au-delà.
func RemotePlayFPS(fps int) int {
	return min(60, StreamFPS(fps))
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
