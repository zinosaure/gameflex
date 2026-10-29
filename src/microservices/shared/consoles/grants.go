package consoles

import (
	"fmt"
	"sort"
	"strings"
)

// LES CONSOLES D'UN COMPTE (chantier 20, le porteur, 2026-10-21) — « tous les
// rétro gaming, puis le choix des consoles streamex et remoteplay », pour
// « gérer les ressources ». Le compte porte ses droits ; son jeton de
// fédération les suit.
//
// Une liste de droits, rangée en une chaîne (virgules) dans le compte :
//   - GrantAll ("*") : tout, consoles à venir comprises. Le défaut d'un compte,
//     et toujours le cas d'un administrateur ;
//   - GrantRetro ("retro") : toutes les consoles du rétro, d'un seul droit ;
//   - l'identifiant d'une console streamex ou Remote Play (`ps3`, `rp-ps5`…).
//
// Vide : aucune console. Le compte ne joue nulle part ; son jeton n'ouvre que
// le catalogue (voir, télécharger).
const (
	GrantAll   = "*"
	GrantRetro = "retro"
)

// ParseGrants — la liste demandée, vérifiée, dédoublonnée et remise dans un
// ordre stable. Un droit inconnu est REFUSÉ plutôt qu'ignoré : une faute de
// frappe ouvrirait moins que ce que l'administrateur croit, et rien ne le lui
// dirait. `*` l'emporte sur tout le reste.
func ParseGrants(asked []string) (string, error) {
	granted := map[string]bool{}
	for _, one := range asked {
		grant := strings.TrimSpace(one)
		switch {
		case grant == "":
			continue
		case grant == GrantAll:
			return GrantAll, nil
		case grant == GrantRetro:
		case grantable(grant):
		default:
			return "", fmt.Errorf("unknown console %q", grant)
		}
		granted[grant] = true
	}

	list := make([]string, 0, len(granted))
	for grant := range granted {
		list = append(list, grant)
	}
	sort.Strings(list)

	return strings.Join(list, ","), nil
}

// grantable — une console qui se donne une par une : streamex ou Remote Play.
// Celles du rétro ne se donnent qu'ensemble (GrantRetro).
func grantable(id string) bool {
	for _, console := range registry {
		if console.ID == id {
			return console.Method == MethodStreamex || console.Method == MethodRemotePlay
		}
	}

	return false
}

// SplitGrants — la chaîne rangée, en liste. Vide : aucune console.
func SplitGrants(stored string) []string {
	list := []string{}
	for _, one := range strings.Split(stored, ",") {
		if one = strings.TrimSpace(one); one != "" {
			list = append(list, one)
		}
	}

	return list
}

// Allows — ces droits ouvrent-ils cette console ? `consoleID` est l'identifiant
// du registre (`nes`, `ps3`, `rp-ps5`…).
func Allows(grants []string, consoleID string) bool {
	retro := false
	for _, console := range registry {
		if console.ID == consoleID {
			retro = console.Method == MethodRetroGaming
			break
		}
	}
	for _, grant := range grants {
		if grant == GrantAll || grant == consoleID || (grant == GrantRetro && retro) {
			return true
		}
	}

	return false
}

// AllowsAny — ces droits ouvrent-ils au moins une console ? Sinon le compte ne
// joue nulle part, et son jeton n'ouvre que le catalogue.
func AllowsAny(grants []string) bool {
	return len(grants) > 0
}
