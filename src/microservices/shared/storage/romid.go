package storage

// L'IDENTITÉ D'UN JEU — un UUID de « console/nom sur le disque » (chantier 14,
// 2026-10-12 : « l'UUID devient la référence partout », téléphone compris).
//
// Le même jeu, rangé sous le même nom de fichier sur deux machines, porte le
// même identifiant : c'est ce qui permet de dire qu'un jeu d'un pair est déjà
// dans la bibliothèque d'ici. Un nom écrit autrement est un autre jeu — « le
// nom + l'extension n'est pas pareil » (le porteur).

import (
	"strings"

	"github.com/google/uuid"
)

// romNamespace — fixe, sans autre sens que d'être constant : changé, tous les
// identifiants changeraient avec lui. C'est celui du kernel du serveur et de
// l'enfant rétro depuis toujours.
var romNamespace = uuid.MustParse("2f6b8f2e-8f0a-4e9d-9a4f-6a2f8b6f2f20")

// RomID — l'identifiant d'un jeu : UUID v5 de « console/entrée ». `entry` est
// le nom SUR LE DISQUE de l'entrée sous `roms/<console>/` — le fichier,
// extension comprise, ou le DOSSIER d'un jeu-dossier —, pas le titre affiché.
func RomID(consoleID, entry string) string {
	return uuid.NewSHA1(romNamespace, []byte(consoleID+"/"+entry)).String()
}

// IsRomID — la référence a-t-elle la forme d'un identifiant de jeu ? Ce qui ne
// l'a pas n'est pas un jeu de la bibliothèque : un identifiant fédéré
// (« srv3:… »), un chemin, ou ce qu'un hôte démarre sans fichier (keytest).
func IsRomID(reference string) bool {
	_, err := uuid.Parse(reference)

	return err == nil && len(reference) == 36
}

// GameEntry — l'entrée d'un jeu sous le dossier de SA console : le segment qui
// suit, dans son chemin virtuel, le dossier qui porte l'identifiant de cette
// console (la convention, chantier 15 : `roms/<consoleid>/<entrée>`). Un
// fichier libre y a son nom, un jeu-dossier son dossier — la même entrée que
// celle que le serveur lit dans `roms/<console>/`. Le chemin du fichier qui SE
// LANCE suffit : un jeu-dossier se reconnaît sans son `Folder`, ce qui permet
// de dériver l'identifiant d'une référence d'autrefois, faite de ce seul
// chemin.
func GameEntry(game Game) string {
	segments := strings.Split(strings.Trim(game.Path, "/"), "/")
	for index := len(segments) - 2; index >= 0; index-- {
		if segments[index] == game.Console {
			return segments[index+1]
		}
	}

	return baseName(game.Path)
}

// GameID — l'identifiant d'un jeu scanné.
func GameID(game Game) string {
	return RomID(game.Console, GameEntry(game))
}
