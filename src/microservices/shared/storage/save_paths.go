// LE RANGEMENT DES SAUVEGARDES, LE MÊME SUR LES DEUX HÔTES (2026-10-12).
//
//	saves/<compte>/states/<console>/<jeu>_slot<N>.state   (et sa vignette <jeu>_slot<N>.png)
//	saves/<compte>/sram/<console>/<jeu>.srm
//	saves/<compte>/cores/<cœur>/                          ce que le cœur écrit lui-même
//
// C'est la disposition du serveur (retro_files, et le runtime retrogaming :
// stateFilePath, sramFilePath, coreSaveDir). Le téléphone rangeait ses états à
// la racine de saves/, sa SRAM dans saves/sram/, et laissait les cœurs écrire
// dans le stockage PRIVÉ de l'application — invisible, et perdu à la
// désinstallation. « Les saves sur Android sont mal rangées, sous mon compte
// il y a rien […] ça ne suit pas comme sur webapp » (le porteur). La règle vit
// ici pour ne plus s'écrire deux fois.
//
// LE NOM DU JEU est la clé que l'hôte lui donne ; les caractères qu'un
// système de fichiers refuse — Android suit les règles de FAT dans son
// stockage partagé — deviennent « _ ». Une référence du serveur (un UUID) n'en
// contient aucun.
package storage

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var unsafeSaveName = strings.NewReplacer(
	`\`, "_", "/", "_", ":", "_", "*", "_", "?", "_", `"`, "_", "<", "_", ">", "_", "|", "_",
)

// SafeSaveName — une clé de jeu, telle qu'un nom de fichier la porte.
func SafeSaveName(game string) string {
	return unsafeSaveName.Replace(game)
}

// saveJoin — un chemin sous savesRoot, sans qu'un segment venu d'une requête
// (le compte, la console) puisse en sortir : « ../databases » ne passe pas.
func saveJoin(savesRoot string, segments ...string) (string, error) {
	if strings.TrimSpace(savesRoot) == "" {
		return "", fmt.Errorf("aucun dossier de sauvegardes")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, `/\`) {
			return "", fmt.Errorf("segment de sauvegarde invalide : %q", segment)
		}
	}

	return filepath.Join(append([]string{savesRoot}, segments...)...), nil
}

// UserSavesDir — le dossier d'un compte : saves/<compte>.
func UserSavesDir(savesRoot, username string) (string, error) {
	return saveJoin(savesRoot, username)
}

// SaveStatePath — l'emplacement N d'un jeu, pour ce compte. ext vaut
// « .state », ou « .png » pour sa vignette — les noms du serveur.
func SaveStatePath(savesRoot, username, consoleID, game string, slot int, ext string) (string, error) {
	return saveJoin(savesRoot, username, "states", consoleID, fmt.Sprintf("%s_slot%d%s", SafeSaveName(game), slot, ext))
}

// SaveRAMPath — la sauvegarde de cartouche d'un jeu, pour ce compte.
func SaveRAMPath(savesRoot, username, consoleID, game string) (string, error) {
	return saveJoin(savesRoot, username, "sram", consoleID, SafeSaveName(game)+".srm")
}

// CoreSaveDir — le dossier qu'un CŒUR écrit lui-même (GET_SAVE_DIRECTORY) :
// keyé par cœur et non par console, parce qu'un même émulateur sert parfois
// deux consoles et range comme il l'entend (Dolphin, gc et wii).
func CoreSaveDir(savesRoot, username, core string) (string, error) {
	return saveJoin(savesRoot, username, "cores", core)
}

var saveStateName = regexp.MustCompile(`^(.*)_slot(\d+)\.state$`)

// ParseSaveStateName — (clé du jeu, emplacement) d'un nom d'état, à rebours
// de SaveStatePath ; faux pour tout autre fichier (une vignette, un .srm).
func ParseSaveStateName(name string) (game string, slot int, ok bool) {
	match := saveStateName.FindStringSubmatch(name)
	if match == nil {
		return "", 0, false
	}
	if _, err := fmt.Sscanf(match[2], "%d", &slot); err != nil {
		return "", 0, false
	}

	return match[1], slot, true
}
