// Package profiles carries the console pad profiles (profile.json) served
// by the streamex session containers. Streamex main no longer embeds any
// button knowledge ("streamex ne connaît plus aucune console") — the core
// binary's /profile.json falls back to a bare {"console","keylab"} and the
// GameFlex on-screen pads (gameflex/streamex/components/*.jsx) would hide.
//
// These profiles are the exact ones the previous streamex branch
// (feature/key-test) embedded in its binary, keyed by OUR console id. They
// are written once at startup into GAMEFLEX_PROFILES_HOST_DIR (host-side) and
// bind-mounted read-only into each session container at /opt/profile.json.
//
// QUELLES CONSOLES ? LE REGISTRE LE DIT — pas une liste écrite ici.
//
// C'était une liste en dur (`switch/ps3/ps4/xbox/keytest`), et rien ne
// rappelait de l'allonger : une console streamex ajoutée au registre partait
// donc sans profil, et sa première partie mourait sur une erreur brute du
// démon Docker (« bind source path does not exist: …/profiles/ps5.json »),
// qui ne nomme ni la console ni ce qui manque. Arrivé le 2026-09-10 avec la
// PS5 ; xbox360 et xboxone attendaient le même sort le jour où on les
// activerait.
//
// Il n'y a que QUATRE profils de boutons (switch/ps3/xbox/keytest) pour huit
// consoles : plusieurs manettes sont la même. Ce qui distingue les fichiers
// écrits, c'est leur champ `console` — estampillé par withConsoleID, sinon
// ps4.json annonçait « ps3 » (c'était le cas) et wiiu.json devait être
// corrigé à la main pour dire « wiiu » (c'était le cas aussi).
package profiles

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gameflex/consoles"
)

//go:embed switch.json
var switchProfile string

//go:embed ps3.json
var ps3Profile string

//go:embed xbox.json
var xboxProfile string

//go:embed keytest.json
var keytestProfile string

// ForConsole returns the pad profile for a console id, son champ `console`
// estampillé au nom de CETTE console.
func ForConsole(console string) string {
	return withConsoleID(baseProfile(console), console)
}

// baseProfile — quelle manette, parmi les quatre qu'on sait dessiner.
//
// La PS4 et la PS5 reprennent celle de la PS3 : c'est la même DualShock/
// DualSense (seules SHARE/CREATE et OPTIONS remplacent SELECT/START, et le
// pavé tactile n'est pas encore une surface qu'une manette à l'écran sait
// offrir — voir docs/TODO.md). La Wii U reprend celle de la Switch, et les
// trois Xbox la même. keytest n'est pas une console mais un banc d'essai de
// touches, avec son propre profil.
func baseProfile(console string) string {
	switch console {
	case "ps3", "ps4", "ps5":
		return ps3Profile
	case "xbox", "xbox360", "xboxone":
		return xboxProfile
	case "keytest":
		return keytestProfile
	default:
		return switchProfile
	}
}

// consoleField repère le champ `console` du profil, quel que soit son
// espacement — les fichiers embarqués sont indentés, pas compacts.
var consoleField = regexp.MustCompile(`"console"(\s*):(\s*)"[^"]*"`)

// withConsoleID estampille le champ `console` du profil.
//
// Une réécriture de texte, pas un décodage JSON : passer par
// encoding/json réordonnerait les clés et réindenterait tout le fichier, ce
// qui rendrait illisible le diff de fichiers qui sont, eux, suivis par git.
// Seule la PREMIÈRE occurrence est remplacée — le champ est au premier
// niveau, les boutons n'en ont pas.
//
// Un identifiant qui contiendrait un guillemet casserait le JSON produit :
// on renvoie alors le profil inchangé plutôt qu'un fichier illisible. Les
// identifiants viennent du registre, donc ça ne devrait jamais arriver.
func withConsoleID(profile, console string) string {
	if strings.ContainsAny(console, `"\`) {
		return profile
	}
	match := consoleField.FindStringSubmatch(profile)
	if match == nil {
		return profile
	}
	stamped := `"console"` + match[1] + ":" + match[2] + `"` + console + `"`

	return strings.Replace(profile, match[0], stamped, 1)
}

// Consoles — les consoles qui ont besoin d'un profil : toutes celles que le
// registre déclare streamex, désactivées comprises (en écrire un pour une
// console éteinte ne coûte rien, et lui en faire manquer un le jour où on
// l'allume coûte une partie), plus keytest, qui n'est pas dans le registre.
func Consoles() []string {
	out := []string{"keytest"}
	for _, console := range consoles.All() {
		if console.Method == consoles.MethodStreamex {
			out = append(out, console.ID)
		}
	}

	return out
}

// WriteAll writes every console profile into hostDir (idempotent — existing
// files with the same content are left untouched). hostDir is a HOST-side
// path resolved by the Docker daemon for the session mounts; this service
// reaches it through its own bind mount of the same folder.
func WriteAll(hostDir string) error {
	if hostDir == "" {
		return nil
	}
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		return err
	}
	written := Consoles()
	for _, console := range written {
		target := filepath.Join(hostDir, console+".json")
		if err := os.WriteFile(target, []byte(ForConsole(console)), 0o644); err != nil {
			return err
		}
	}
	slog.Info("console pad profiles written", "dir", hostDir, "consoles", written)
	return nil
}
