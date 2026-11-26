package metadata

// LE NOM D'UNE SAUVEGARDE EST CELUI DE SON JEU (le porteur, 2026-11-16 : « Oui il faut les nom des jeux et la console
// dans mes sauvegardes »). Une console récente range la sauvegarde d'un jeu sous son identifiant de titre (saves.Games :
// `01000EA014150000`, CUSA07010, PPSA10594, BLES00783) — le `<gameid>` que GameKey lit dans le jeu même (son paquet
// Switch, son `sce_sys/`, l'ISO d'un PS3). La sauvegarde prend donc le titre du jeu de la bibliothèque qui porte ce
// gameid ; sinon celui qu'elle se donne (le « maintitle » d'une PS4, le PARAM.SFO d'une PS3, le meta.xml d'une Wii U) ;
// son identifiant en dernier, que la page montre alors.
//
// RIEN NE SE RELIT À CHAQUE REQUÊTE : GameKey garde le gameid de chaque jeu par fichier, taille et date — un `stat` par
// jeu, des seules consoles qui ont une sauvegarde. Seule la première demande après un démarrage lit des en-têtes
// (quelques Ko par paquet Switch : 0,64 s pour les 34 jeux Switch de Gamebox, mesuré le 2026-11-16 ; 0,2 ms ensuite) —
// et les jaquettes de la bibliothèque les ont souvent déjà lus.
//
// LE NOM SE GARDE AVEC LA SAUVEGARDE (chantier 76) : ce que la bibliothèque nomme s'écrit dans `titles.json` du
// rangement (saves.Remember), avec la référence et le chemin du jeu ; une sauvegarde dont le jeu est parti garde ainsi
// son nom. L'ordre : la bibliothèque, puis `titles.json` (ce que la bibliothèque disait), puis ce que la sauvegarde dit
// d'elle-même, puis son identifiant.
//
// Chaque kernel donne ses jeux (le serveur : son catalogue, puis ses disques ; le téléphone : sa bibliothèque, puis les
// siens) : la règle, elle, est ici, une fois.

import "gameflex/storage/saves"

// LibraryGame — un jeu d'une console, tel qu'un kernel le connaît : le chemin que GameKey lit (le dossier qui est ce
// jeu quand il en a un, sinon son fichier), son chemin depuis le dossier des jeux (« ps5/Astro Bot », celui que garde
// `titles.json`), sa référence (le repli de GameKey, celui de ses métadonnées), son titre, et son disque interne ("" :
// la bibliothèque).
type LibraryGame struct {
	Path      string
	RelPath   string
	Reference string
	Title     string
	Disk      string
}

// NameSaves — chaque sauvegarde du rangement `tree` avec le titre, la référence, le disque et le chemin du jeu de
// `library` qui porte son identifiant ; le premier l'emporte (la bibliothèque, puis les disques, dans l'ordre que le
// kernel les donne). Sans jeu, ce que `titles.json` en a gardé ; sans rien, elle garde son titre. `library` n'est
// demandée qu'une fois par console qui a une sauvegarde — jamais pour la Xbox, dont la sauvegarde est le disque de
// toute la console. La liste ne tombe pas pour un nom qu'elle n'a pas pu garder.
func NameSaves(tree string, games []saves.Game, library func(consoleID string) []LibraryGame) []saves.Game {
	byConsole := map[string]map[string]LibraryGame{}
	known := saves.Known(tree)
	named := []saves.Named{}

	for index, game := range games {
		if game.ID == saves.XboxDisk {
			continue
		}
		found, read := byConsole[game.Console]

		if !read {
			found = map[string]LibraryGame{}

			for _, entry := range library(game.Console) {
				// La référence en repli, comme chaque appel de GameKey : le gameid gardé pour ce fichier est celui
				// que ses métadonnées lisent aussi.
				key := GameKey(entry.Path, entry.Reference)

				if _, taken := found[key]; !taken && entry.Title != "" {
					found[key] = entry
				}
			}
			byConsole[game.Console] = found
		}

		if entry, here := found[game.ID]; here {
			games[index].Title, games[index].Reference, games[index].Disk, games[index].Path = entry.Title, entry.Reference, entry.Disk, entry.RelPath
			named = append(named, saves.Named{Console: game.Console, Save: game.ID,
				Game: saves.KnownGame{Title: entry.Title, ID: entry.Reference, Path: entry.RelPath}})
		} else if kept, ok := known(game.Console, game.ID); ok {
			games[index].Title, games[index].Path = kept.Title, kept.Path
		}
	}
	_ = saves.Remember(tree, named...)

	return games
}
