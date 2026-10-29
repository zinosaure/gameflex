package storage

// LE MOTEUR D'UN JEU (chantier 36, 2026-10-27) — reconnu à ses fichiers, pour
// régler l'émulateur selon lui AVANT de lancer : « il faut qu'on sache avant de
// lancer le jeu c'est quoi le moteur pour adapter le réglage » (le porteur).
//
// Un même émulateur ne rencontre pas les mêmes défauts d'un moteur à l'autre :
// Unity arrête ses fils par des signaux à chaque ramasse-miettes (#510),
// Frostbite exige une page à 1 008 Gio qu'un téléphone n'a pas (#513).
//
// Une seule règle pour le serveur et le téléphone : le catalogue du kernel, le
// scan des sources et le lanceur (pour un jeu emballé, une fois déballé)
// l'appellent tous ici. Elle ne lit aucun octet de jeu, elle regarde des noms —
// la racine du jeu, et au plus un sous-dossier (`Media/` pour Unity).

import (
	"os"
	"path/filepath"
	"strings"
)

// Les moteurs reconnus. Un jeu sans marqueur n'en a pas (""), et garde les
// réglages communs de son émulateur.
const (
	EngineUnity     = "unity"
	EngineUnreal    = "unreal"
	EngineFrostbite = "frostbite"
	EngineRage      = "rage"
)

// engineConsoles — les consoles dont les jeux ont un moteur reconnaissable dans
// leurs fichiers. Pas la PS3 : ses jeux sont des `.iso`, et son moteur ne se
// lit qu'une fois l'image montée, au lancement.
var engineConsoles = map[string]bool{"ps4": true, "ps5": true}

// GameEngine — le moteur du jeu-dossier dont `root` est la racine, ou "".
// `list` liste un sous-dossier du jeu (chemin relatif à sa racine) ; elle
// n'est appelée que pour un jeu qui a un dossier `Media/`.
//
// Les marqueurs, relevés sur la bibliothèque le 2026-10-27 :
//
//	Unity      Media/boot.config (ou globalgamemanagers, data.unity3d)  Melody of Memory, Richman 11
//	Unreal     ue4commandline.txt, uecommandline.txt                      FF7 Remake, KH III, Wukong, Tekken 8
//	Frostbite  package.mft + Data/ + Engine.*.prx                         FIFA 22, FC 27
//	RAGE       *.rpf                                                      GTA V
func GameEngine(consoleID string, root []Entry, list func(dir string) ([]Entry, error)) string {
	if !engineConsoles[consoleID] {
		return ""
	}
	var hasMedia, hasData, hasManifest, hasEngineModule bool
	for _, entry := range root {
		name := strings.ToLower(entry.Name)
		if entry.Dir {
			hasMedia = hasMedia || name == "media"
			hasData = hasData || name == "data"

			continue
		}
		switch {
		case name == "ue4commandline.txt" || name == "uecommandline.txt":
			return EngineUnreal
		case strings.HasSuffix(name, ".rpf"):
			return EngineRage
		case name == "package.mft":
			hasManifest = true
		case strings.HasPrefix(name, "engine.") && strings.HasSuffix(name, ".prx"):
			hasEngineModule = true
		}
	}

	if hasManifest && hasData && hasEngineModule {
		return EngineFrostbite
	}

	if hasMedia && list != nil {
		media, err := list(mediaDir(root))
		if err != nil {
			return ""
		}
		for _, entry := range media {
			switch strings.ToLower(entry.Name) {
			case "boot.config", "globalgamemanagers", "data.unity3d":
				if !entry.Dir {
					return EngineUnity
				}
			}
		}
	}

	return ""
}

// mediaDir — le nom du dossier `Media` tel qu'il est écrit sur le disque : une
// source sensible à la casse ne le trouverait pas sous un autre.
func mediaDir(root []Entry) string {
	for _, entry := range root {
		if entry.Dir && strings.EqualFold(entry.Name, "media") {
			return entry.Name
		}
	}

	return "Media"
}

// GameEngineAt — le moteur du jeu-dossier `dir`, lu sur un disque local : le
// lanceur du serveur et celui du téléphone, au moment de lancer (chantier 36).
// "" s'il ne se lit pas.
func GameEngineAt(consoleID, dir string) string {
	entriesOf := func(folder string) ([]Entry, error) {
		items, err := os.ReadDir(folder)
		if err != nil {
			return nil, err
		}
		entries := make([]Entry, 0, len(items))
		for _, item := range items {
			entries = append(entries, Entry{Name: item.Name(), Dir: item.IsDir()})
		}

		return entries, nil
	}
	root, err := entriesOf(dir)
	if err != nil {
		return ""
	}

	return GameEngine(consoleID, root, func(sub string) ([]Entry, error) {
		return entriesOf(filepath.Join(dir, sub))
	})
}
