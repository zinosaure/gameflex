package config

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

// LES RÉGLAGES DU SERVEUR QUI CHANGENT SANS LE RELANCER (chantier 81 ; le porteur, 2026-11-25 : « On peut mettre les
// varenv dans une section "Serveur config" dans le settings pour les administrateurs ? »). Ils ont quitté le compose
// (GAMEFLEX_SERVER_NAME, GAMEFLEX_MAX_PLAYERS, GAMEFLEX_CONSOLE_LIMITS, GAMEFLEX_GPU ; GAMEFLEX_MIRROR_URL, lui, est
// supprimé : on n'utilise plus de miroir, le porteur, 2026-11-25) : un fichier
// du dossier du serveur les garde (api/server_config.go), Réglages › Configuration du serveur les change, et chacun de
// leurs lecteurs relit la valeur au moment où il s'en sert (CurrentLive) — la prochaine partie, la prochaine réponse.
// Rien de réglé : DefaultLive. Le compose ne garde que ce qui ne peut pas changer en marche (ports, adresse, chemins,
// images, l'interface ICE du gateway).
type Live struct {
	// ServerName — le nom sous lequel ce serveur s'annonce aux autres (la fédération) ; vide, son adresse publique.
	ServerName string `json:"serverName"`
	// Players — les manettes d'une partie, par console Streamex (le porteur, 2026-11-25 : « Le champ "Controllers per
	// game" c'est aussi par console ») ; une console absente garde les places du registre (consoles.MaxPlayers). Les
	// consoles rétro gardent les leurs, fixes.
	Players map[string]int `json:"players"`
	// ConsoleLimits — les parties en même temps, par console (un sélecteur par console Streamex dans la page, le
	// porteur, 2026-11-25) ; une console absente n'a pas de limite.
	ConsoleLimits map[string]int `json:"consoleLimits"`
	// GPU — l'adresse PCI du GPU des parties (« 0000:c4:00.0 ») ; vide, le premier.
	GPU string `json:"gpu"`
}

// MaxConsoleLimit — au-delà, la machine ne suit pas : le sélecteur de la page s'arrête là.
const MaxConsoleLimit = 8

// MaxPlayers — les manettes d'une partie, au plus (la Switch en a huit) ; DefaultSeats, celles d'une console que le
// registre ne connaît pas.
const (
	MaxPlayers   = 8
	DefaultSeats = 4
)

// DefaultLive — une partie à la fois pour chaque console récente (Streamex).
func DefaultLive() Live {
	limits := map[string]int{}
	for _, console := range []string{"ps3", "ps4", "ps5", "wiiu", "switch", "switch2", "xbox", "xbox360", "ports"} {
		limits[console] = 1
	}

	return Live{Players: map[string]int{}, ConsoleLimits: limits}
}

var (
	pciAddress    = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$`)
	consoleID     = regexp.MustCompile(`^[a-z0-9_-]+$`)
	serverNameMax = 40
)

// Normalized — les valeurs nettoyées, ou ce qui ne va pas (dit à la page).
func (live Live) Normalized() (Live, error) {
	live.ServerName = strings.TrimSpace(live.ServerName)
	live.GPU = strings.TrimSpace(live.GPU)
	if utf8.RuneCountInString(live.ServerName) > serverNameMax {
		return live, fmt.Errorf("serverName: %d characters at most", serverNameMax)
	}
	players := map[string]int{}
	for console, count := range live.Players {
		if !consoleID.MatchString(console) || count < 1 || count > MaxPlayers {
			return live, fmt.Errorf("players: %s between 1 and %d", console, MaxPlayers)
		}
		players[console] = count
	}
	live.Players = players
	// Une nouvelle table : celle reçue n'est jamais modifiée (elle peut être celle en vigueur). Zéro : pas de limite.
	limits := map[string]int{}
	for console, maximum := range live.ConsoleLimits {
		if !consoleID.MatchString(console) || maximum < 0 || maximum > MaxConsoleLimit {
			return live, fmt.Errorf("consoleLimits: %s between 0 and %d", console, MaxConsoleLimit)
		}
		if maximum > 0 {
			limits[console] = maximum
		}
	}
	live.ConsoleLimits = limits
	if live.GPU != "" && !pciAddress.MatchString(live.GPU) {
		return live, fmt.Errorf("gpu: a PCI address such as 0000:c4:00.0")
	}

	return live, nil
}

// Limits — les parties en même temps par console ; une console absente n'a pas de limite.
func (live Live) Limits() map[string]int {
	return live.ConsoleLimits
}

var (
	liveMu  sync.RWMutex
	current = DefaultLive()
)

// CurrentLive — les réglages en vigueur, lus à chaque usage.
func CurrentLive() Live {
	liveMu.RLock()
	defer liveMu.RUnlock()

	return current
}

// ApplyLive — de nouveaux réglages, en vigueur aussitôt (au démarrage, à leur enregistrement).
func ApplyLive(live Live) {
	liveMu.Lock()
	defer liveMu.Unlock()
	current = live
}
