package logfile

// CE QUI S'ÉCRIT SUR LE DISQUE — un réglage, et plus une variable (le porteur,
// 2026-11-14 : « Il faut que dans logs on puisse avoir la possibilité de
// l'activer ou pas et aussi choisir les level à afficher si on active le log.
// Comme tu peux voir sinon, on a un log du kernel à 18mb »).
//
// Ces 18 Mo : 93 580 lignes « http » au niveau Info, 99,6 % du fichier — le
// journal d'accès, dont la sonde de santé toutes les 30 s. Les événements du
// jour tenaient en 500 lignes. D'où un interrupteur À PART pour les requêtes,
// qui ne suit pas les niveaux cochés : elles sont toutes au niveau Info, et
// cocher Info pour lire ce qui se passe ne doit pas rouvrir le robinet.
//
// `GAMEFLEX_FILE_LOGS_OFF`, qui éteignait tout d'un bloc et demandait de
// recréer le conteneur, est parti avec : le réglage vaut à la ligne suivante
// (Apply), et c'est le kernel qui le garde (`logs/settings.json`,
// kernel/internal/api/log_settings.go).
//
// LES FICHIERS SEULEMENT. La sortie standard (`docker logs`) ne change pas :
// c'est ce qu'on lit en développant, et un réglage de la page qui la ferait
// taire couperait l'outil qui sert à comprendre ce réglage.

import (
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"
)

// Settings — ce que le kernel écrit dans le dossier des journaux.
type Settings struct {
	// Kernel — le journal du jour du kernel (`logs/<jour>/kernel.log`).
	Kernel bool `json:"kernel"`
	// Levels — les niveaux qui s'y écrivent (LevelNames), Kernel allumé.
	Levels []string `json:"levels"`
	// HTTP — les lignes du journal d'accès (HTTPMessage), Kernel allumé,
	// quels que soient les niveaux cochés.
	HTTP bool `json:"http"`
	// Sessions — le journal de chaque partie
	// (`logs/<jour>/<heure>/<conteneur>.log`). Pas de niveaux : c'est la
	// sortie brute d'un émulateur, qui n'en a pas.
	Sessions bool `json:"sessions"`
	// Reports — le journal des signalements (`logs/reports.log`), ce que les
	// joueurs envoient d'un écran d'erreur avec « Signaler l'erreur » (le
	// porteur, 2026-11-14 : « Oui pour le reports.log »). Éteint, le kernel
	// refuse un signalement (ErrReportsOff) et ne déclare plus `reports` : le
	// bouton disparaît.
	Reports bool `json:"reports"`
}

// LevelNames — les niveaux qu'un journal peut écrire, dans l'ordre de slog.
var LevelNames = []string{"debug", "info", "warn", "error"}

// DefaultSettings — sans réglage enregistré : le kernel écrit ce qui se passe
// (Info, Warn, Error) mais pas chaque requête, chaque partie son journal, et
// les signalements.
//
// Allumé par défaut, comme avant : un serveur a un disque qu'on administre, et
// l'intérêt de ces fichiers est d'exister AVANT qu'on sache en avoir besoin.
// Les allumer après coup, c'est arriver trop tard.
func DefaultSettings() Settings {
	return Settings{Kernel: true, Levels: []string{"info", "warn", "error"}, Sessions: true, Reports: true}
}

// Normalized — chaque niveau une fois, dans l'ordre de LevelNames. Un niveau
// inconnu est une erreur : la route le refuse plutôt que d'enregistrer un
// réglage qui ne ferait rien.
func (settings Settings) Normalized() (Settings, error) {
	for _, name := range settings.Levels {
		if !slices.Contains(LevelNames, name) {
			return settings, fmt.Errorf("unknown log level %q", name)
		}
	}
	normalized := settings
	normalized.Levels = []string{}
	for _, name := range LevelNames {
		if slices.Contains(settings.Levels, name) {
			normalized.Levels = append(normalized.Levels, name)
		}
	}

	return normalized, nil
}

var (
	current atomic.Pointer[Settings]
	// generation — avance à chaque réglage appliqué et à chaque journal
	// supprimé : un fichier ouvert se rouvre à sa ligne suivante
	// (switchedFile), au lieu d'écrire dans un fichier qui n'est plus sur le
	// disque — éteint, supprimé pour faire de la place, rallumé.
	generation atomic.Uint64
)

func init() {
	defaults := DefaultSettings()
	current.Store(&defaults)
}

// Apply — le réglage que les journaux suivent, dès leur ligne suivante.
func Apply(settings Settings) {
	applied := settings
	applied.Levels = slices.Clone(settings.Levels)
	current.Store(&applied)
	generation.Add(1)
}

// Current — le réglage en vigueur.
func Current() Settings {
	settings := *current.Load()
	settings.Levels = slices.Clone(settings.Levels)

	return settings
}

// levelName — le nom du niveau d'une ligne, celui des cases à cocher.
func levelName(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "debug"
	case level < slog.LevelWarn:
		return "info"
	case level < slog.LevelError:
		return "warn"
	}

	return "error"
}

// writes — une ligne du kernel va-t-elle dans son fichier ?
func (settings *Settings) writes(level slog.Level, message string) bool {
	if !settings.Kernel {
		return false
	}

	if message == HTTPMessage {
		return settings.HTTP
	}

	return slices.Contains(settings.Levels, levelName(level))
}

// mayWrite — la même question avant de connaître le message
// (slog.Handler.Enabled) : une requête s'écrit au niveau Info.
func (settings *Settings) mayWrite(level slog.Level) bool {
	return settings.Kernel && (slices.Contains(settings.Levels, levelName(level)) || (settings.HTTP && level == slog.LevelInfo))
}
