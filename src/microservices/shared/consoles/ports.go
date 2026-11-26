package consoles

import "sort"

// LES PORTS PC (chantier 40, #609 ; chantiers 72 et 77).
//
// Un port est un jeu recompilé pour le PC par sa communauté, construit chez le
// joueur depuis SA copie : la console `ports` n'a donc pas de dossier à elle,
// ses jeux vivent chez la console d'origine (`roms/ps5`). Un port ne connaît
// qu'une version précise d'un jeu, reconnue à son identifiant et à sa version
// (AstroHybrid : Astro Bot PPSA21567 01.019.000).
//
// PAS DE JEUX RÉTRO (le porteur, 2026-11-23 : « On ne va pas recompiler les
// jeux rétro, les émulateurs sont déjà très bien ») : les ports sont ceux des
// PS3, PS4 et PS5. Zelda 64: Recompiled, le pilote N64 du chantier 40, et la
// reconnaissance d'une ROM par sa taille et son SHA-1 sont retirés.

// PortsID — l'identifiant de la console des ports.
const PortsID = "ports"

// PortTitle — un jeu-dossier qu'un port accepte (une copie PS5 : `sce_sys/param.json`) : son identifiant et sa version.
// Un port recompilé ne connaît qu'une version précise du jeu (AstroHybrid : Astro Bot 01.019.000).
type PortTitle struct {
	TitleID string
	Version string
}

// Port — un port : son cœur dans la console `ports` (le nom du registre), la
// console dont il lit les jeux, et ceux qu'il accepte (Titles).
//
// `Pad` : la console dont le port lit la manette — son profil de boutons, et le dessin à l'écran (Astro Bot, un jeu PS5,
// se joue à la DualSense). Vide : la manette des ports, celle de la Xbox.
type Port struct {
	Core   string
	From   string
	Titles []PortTitle
	Pad    string
}

var ports = []Port{
	// AstroHybrid (chantier 72, à l'essai sur la dev) : Astro Bot relié en programme Linux par AnyPS5, rendu par la
	// couche GPU de KytyPS5 ; il se construit à la première partie, depuis la copie du joueur. La version éprouvée.
	{Core: "AstroHybrid (streamex)", From: "ps5", Pad: "ps5", Titles: []PortTitle{
		{TitleID: "PPSA21567", Version: "01.019.000"},
	}},
}

// Ports — les ports connus, dans l'ordre du registre.
func Ports() []Port {
	return append([]Port(nil), ports...)
}

// PortSources — les consoles dont des ports lisent les jeux, triées.
func PortSources() []string {
	seen := map[string]bool{}
	sources := []string{}
	for _, port := range ports {
		if !seen[port.From] {
			seen[port.From] = true
			sources = append(sources, port.From)
		}
	}
	sort.Strings(sources)

	return sources
}

// PortByCore — le port de ce cœur.
func PortByCore(core string) (Port, bool) {
	for _, port := range ports {
		if port.Core == core {
			return port, true
		}
	}

	return Port{}, false
}

// PadOf — la console dont une partie de `console` lit la manette, avec ce cœur : celle que le port déclare, sinon la
// console elle-même.
func PadOf(console, core string) string {
	if console == PortsID {
		if port, ok := PortByCore(core); ok && port.Pad != "" {
			return port.Pad
		}
	}

	return console
}
