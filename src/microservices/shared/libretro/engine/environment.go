package engine

import (
	"log"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"gameflex/libretro"
	"gameflex/libretro/gl_context"
)

// forcedVariables — les options qu'un cœur ne lit QUE si le frontal en donne
// une valeur. La convention de libretro, « GET_VARIABLE rend false = garde
// ton défaut », ne tient pas pour elles : leur valeur n'est affectée que dans
// le `if (environ_cb(GET_VARIABLE, …))` du cœur. Chacune a été trouvée sur le
// serveur en lisant la source du cœur.
var forcedVariables = map[string]map[string]string{
	// La DS attend un stylet (position absolue), pas une souris (relative) :
	// sans « touch », l'écran du bas reste mort. « enabled » est le défaut
	// annoncé de pointer_mouse, forcé quand même pour la raison ci-dessus :
	// avec le seul pointer_type, desmume n'interrogeait jamais le pointeur.
	"nds": {
		"desmume_pointer_type":  "touch",
		"desmume_pointer_mouse": "enabled",
	},
	// pcsx_rearmed : la vibration n'est appelée que si l'option est posée, et
	// la combinaison qui bascule la manette analogique n'existe qu'une fois
	// son masque lu.
	"ps1": {
		"pcsx_rearmed_vibration":    "enabled",
		"pcsx_rearmed_analog_combo": "l1+r1+select",
	},
}

// retroLanguages — les langues de l'application dans l'énumération
// RETRO_LANGUAGE de libretro.h (EN 0, FR 2) : ce que GET_LANGUAGE répond.
var retroLanguages = map[string]uint32{"en": 0, "fr": 2}

// languageVariables — l'option de langue des cœurs qui ne suivent pas
// GET_LANGUAGE (chantier 21), et sa valeur pour chaque langue de
// l'application. Relevées dans les journaux des parties (« option du cœur »)
// et dans la source des cœurs. PPSSPP (« Automatic ») et DeSmuME (« Auto »)
// n'y sont pas : ils lisent GET_LANGUAGE d'eux-mêmes. La région ne change
// pas : seule la langue suit l'application (le porteur, 2026-10-22).
var languageVariables = map[string]map[string]string{
	// Dolphin (GameCube, Wii) : la numérotation de la Wii, convertie par le
	// cœur pour le GameCube. Un jeu dont la région n'a pas cette langue garde
	// la sienne — comme la vraie console.
	"dolphin_language": {"fr": "3", "en": "1"},
	// Flycast (Dreamcast) : ses options portent encore le préfixe reicast_.
	"reicast_language": {"fr": "French", "en": "English"},
	// Saturn : Beetle Saturn (posée au chargement du jeu, horloge
	// automatique) et Kronos. Yabause n'a pas d'option de langue.
	"beetle_saturn_autortc_lang": {"fr": "french", "en": "english"},
	"kronos_language_id":         {"fr": "French", "en": "English"},
}

// environment répond aux questions du cœur (retro_environment). Les numéros
// sont ceux de libretro.h ; une commande inconnue rend false, ce que la
// norme prévoit.
func (e *Engine) environment(cmd uint32, data unsafe.Pointer) bool {
	switch cmd {
	case 1: // SET_ROTATION — NYI : l'image part telle que le cœur la rend
		return true
	case 2: // GET_OVERSCAN
		return false
	case 3: // GET_CAN_DUPE
		*(*bool)(data) = true
		return true
	case 6: // SET_MESSAGE
		return true
	case 8: // SET_PERFORMANCE_LEVEL
		return true
	case 9: // GET_SYSTEM_DIRECTORY
		*(*unsafe.Pointer)(data) = libretro.GetSystemDir()
		return true
	case 10: // SET_PIXEL_FORMAT — les trois sont convertis (ToBGRA)
		e.pixelFormat = int(*(*uint32)(data))
		return true
	case 11: // SET_INPUT_DESCRIPTORS
		return true
	case 13: // SET_DISK_CONTROL_INTERFACE — les jeux en plusieurs disques
		// On ne fait que GARDER l'interface : le comptage et le changement
		// passent par Disks, sur le fil du moteur, jamais pendant un retro_run.
		if libretro.HandleSetDiskControl(data) {
			log.Printf("[engine] interface de disques enregistrée (console %s)", e.config.ConsoleID)
			return true
		}
		// Conforme mais inexploitable (pas de get_num_images) : on le dit au
		// cœur plutôt que de prétendre l'avoir prise.
		log.Printf("[engine] interface de disques incomplète, ignorée (console %s)", e.config.ConsoleID)
		return false
	case 14: // SET_HW_RENDER
		return e.acceptHWRender(data)
	case 15: // GET_VARIABLE
		return e.variable(data)
	case 16: // SET_VARIABLES — dit ce que le cœur expose, et retient ses défauts
		e.defaults = map[string]string{}
		for _, variable := range libretro.ListCoreVariables(data) {
			log.Printf("[engine] option du cœur : %s = %q", variable.Key, variable.Value)
			if value, ok := declaredDefault(variable.Value); ok {
				e.defaults[variable.Key] = value
			}
			e.chooseResolution(variable.Key, variable.Value)
		}
		return true
	case 18: // SET_SUPPORT_NO_GAME
		return true
	case 23: // GET_RUMBLE_INTERFACE — un seul pointeur de fonction, écrit comme le journal
		log.Printf("[engine] le cœur demande la vibration (console %s)", e.config.ConsoleID)
		*(*unsafe.Pointer)(data) = libretro.GetRumbleCallback()
		return true
	case 27: // GET_LOG_INTERFACE
		// Le refuser est permis, mais le retro_init de PPSSPP n'initialise
		// son journal QUE dans cette branche, puis l'appelle quand même : un
		// pointeur nul, avant même le chargement du jeu.
		*(*unsafe.Pointer)(data) = libretro.GetCoreLogCallback()
		return true
	case 31: // GET_SAVE_DIRECTORY
		*(*unsafe.Pointer)(data) = libretro.GetSaveDir()
		return true
	case 35: // SET_CONTROLLER_INFO — le cœur dit quelles manettes il sait faire
		for _, controller := range libretro.ListControllerTypes(data) {
			log.Printf("[engine] manette proposée par le cœur : %q (id=%d)", controller.Description, controller.ID)
		}
		return true
	case 37: // SET_GEOMETRY — taille et format d'affichage changent en cours de partie
		if data != nil {
			e.setAspect(libretro.GeometryAt(data), "SET_GEOMETRY")
		}
		return true
	case 39: // GET_LANGUAGE — la langue de l'application (chantier 21)
		*(*uint32)(data) = retroLanguages[e.config.Language]
		return true
	case 52, 55, 68: // GET_CORE_OPTIONS_VERSION / SET_CORE_OPTIONS_DISPLAY / SET_CORE_OPTIONS_V2_INTL
		return true
	default:
		// Ce que le cœur demande et que le moteur ne sait pas donner, dit une
		// fois par commande : un cœur qui se comporte autrement ici que sous
		// LibretroDroid s'explique souvent par une réponse qu'il n'a pas eue.
		if !e.unknownSeen[cmd] {
			if e.unknownSeen == nil {
				e.unknownSeen = map[uint32]bool{}
			}
			e.unknownSeen[cmd] = true
			log.Printf("[engine] commande d'environnement sans réponse : %d", cmd)
		}
		return false
	}
}

// variable répond à GET_VARIABLE : ce que l'appelant impose, puis la
// résolution du flux, la langue de l'application (languageVariables), ce que
// la console exige (forcedVariables), puis la valeur par défaut que le cœur a
// déclarée — comme RetroArch et LibretroDroid, sur le serveur comme sur le
// téléphone (décidé par le porteur le 2026-10-11).
//
// Le serveur répondait `false` jusque-là (« garde ton défaut compilé »), et
// les deux ne disent pas la même chose : parallel_n64 déclare `gliden64` et,
// sans réponse, rendait en logiciel sur le serveur — et prenait sur Android un
// rendu qui tuait l'application (#302). `false` ne reste que pour une clé
// que le cœur n'a pas déclarée.
func (e *Engine) variable(data unsafe.Pointer) bool {
	key := libretro.GetVariableKey(data)
	if value, imposed := e.config.Game.Variables[key]; imposed {
		log.Printf("[engine] GET_VARIABLE %s -> %s (imposé)", key, value)
		libretro.SetVariableValue(data, value)
		return true
	}
	if value, chosen := e.resolutions[key]; chosen {
		log.Printf("[engine] GET_VARIABLE %s -> %s (résolution du flux)", key, value)
		libretro.SetVariableValue(data, value)
		return true
	}
	if value, known := languageVariables[key][e.config.Language]; known {
		log.Printf("[engine] GET_VARIABLE %s -> %s (langue de l'application)", key, value)
		libretro.SetVariableValue(data, value)
		return true
	}
	if value, forced := forcedVariables[e.config.ConsoleID][key]; forced {
		log.Printf("[engine] GET_VARIABLE %s -> %s", key, value)
		libretro.SetVariableValue(data, value)
		return true
	}
	if value, declared := e.defaults[key]; declared {
		libretro.SetVariableValue(data, value)
		return true
	}

	return false
}

// declaredDefault lit la valeur par défaut d'une option déclarée par
// SET_VARIABLES : « Description; défaut|autre|… » — la première de la liste.
func declaredDefault(declaration string) (string, bool) {
	_, values, found := strings.Cut(declaration, "; ")
	if !found || values == "" {
		return "", false
	}
	first, _, _ := strings.Cut(values, "|")

	return first, first != ""
}

// chooseResolution retient, pour l'option de résolution interne d'un cœur
// (internalResolutions), la valeur qu'il a déclarée la plus proche de la
// hauteur du flux choisie. Sans choix (StreamHeight nul), le cœur garde son
// défaut.
func (e *Engine) chooseResolution(key, declaration string) {
	option, known := internalResolutions[key]
	if !known || e.config.StreamHeight <= 0 {
		return
	}
	value := renderResolution(option, declaration, e.config.StreamHeight)
	if value == "" {
		log.Printf("[engine] résolution interne : %s — aucune valeur lisible dans %q", key, declaration)

		return
	}
	if e.resolutions == nil {
		e.resolutions = map[string]string{}
	}
	e.resolutions[key] = value
	log.Printf("[engine] résolution interne : %s = %s (flux de %d lignes)", key, value, e.config.StreamHeight)
}

// renderResolution — parmi les valeurs que le cœur déclare (« Description;
// v1|v2|… »), celle dont la hauteur est la plus proche de `target`. À
// distance égale, la plus basse : qui baisse la résolution du flux cherche
// souvent à alléger, et l'écart ne se voit pas. "" quand aucune ne se lit.
func renderResolution(option resolutionOption, declaration string, target int) string {
	_, list, found := strings.Cut(declaration, "; ")
	if !found || target <= 0 {
		return ""
	}
	best, bestHeight, bestDistance := "", 0, -1
	for _, value := range strings.Split(list, "|") {
		height := valueHeight(value, option.native)
		if height <= 0 {
			continue
		}
		if option.screens > 1 {
			height *= option.screens
		}
		distance := height - target
		if distance < 0 {
			distance = -distance
		}
		if bestDistance < 0 || distance < bestDistance || (distance == bestDistance && height < bestHeight) {
			best, bestHeight, bestDistance = value, height, distance
		}
	}

	return best
}

var (
	sizeValue       = regexp.MustCompile(`(\d+)\s*[xX]\s*(\d+)`)              // 960x544, (640 x 528)
	lineValue       = regexp.MustCompile(`~(\d+)p`)                           // 2x Native (~720p)
	multiplierValue = regexp.MustCompile(`^(?:[xX]\s*(\d+)|(\d+)\s*[xX]?)\b`) // 2X, x2, 3, 1x Native
)

// valueHeight — la hauteur, en lignes, que décrit une valeur d'option :
// « 960x544 » → 544, « 2x Native (~720p) » → 720, puis les multiplicateurs
// (« 2X », « 3 », « original », « enabled ») rapportés à `native`. 0 quand
// elle ne dit rien qu'on sache lire.
func valueHeight(value string, native int) int {
	if match := sizeValue.FindStringSubmatch(value); match != nil {
		height, _ := strconv.Atoi(match[2])

		return height
	}
	if match := lineValue.FindStringSubmatch(value); match != nil {
		height, _ := strconv.Atoi(match[1])

		return height
	}
	if native <= 0 {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "original", "native", "disabled", "off":
		return native
	case "enabled", "on":
		return 2 * native
	}
	if match := multiplierValue.FindStringSubmatch(strings.TrimSpace(value)); match != nil {
		digits := match[1]
		if digits == "" {
			digits = match[2]
		}
		if factor, err := strconv.Atoi(digits); err == nil && factor > 0 {
			return factor * native
		}
	}

	return 0
}

// acceptHWRender garde la demande de rendu matériel ; le contexte s'ouvre
// après retro_load_game (openGL), sur le fil du moteur.
//
// SEULEMENT AUX CONSOLES ÉPROUVÉES (HWRenderAllowed), et seulement ce que la
// plateforme sait ouvrir : Android n'a que GLES. Refuser l'OpenGL de bureau
// ici laisse au cœur la chance de demander autre chose ou de rendre en
// logiciel, quand l'accepter ferait échouer le démarrage plus loin.
func (e *Engine) acceptHWRender(data unsafe.Pointer) bool {
	if !HWRenderAllowed(e.config.ConsoleID) {
		log.Printf("[engine] rendu matériel refusé : pas accordé à la console %q", e.config.ConsoleID)
		return false
	}

	// HandleSetHWRender pose NOS get_current_framebuffer/get_proc_address
	// dans la structure du cœur même si on refuse ensuite — sans effet : un
	// cœur refusé n'appelle jamais context_reset.
	request := libretro.HandleSetHWRender(data)
	switch request.ContextType {
	case gl_context.ContextTypeOpenGLES2, gl_context.ContextTypeOpenGLES3:
	case gl_context.ContextTypeOpenGL, gl_context.ContextTypeOpenGLCore:
		if runtime.GOOS == "android" {
			log.Printf("[engine] rendu matériel refusé : OpenGL de bureau (type %d) demandé sur Android", request.ContextType)
			return false
		}
	default:
		// Vulkan, Direct3D… : aucun contexte pour eux.
		log.Printf("[engine] rendu matériel refusé : contexte de type %d non pris en charge", request.ContextType)
		return false
	}

	e.hw = &hwRequest{contextType: request.ContextType, major: request.VersionMajor, minor: request.VersionMinor}

	return true
}
