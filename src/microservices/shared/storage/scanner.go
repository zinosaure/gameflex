package storage

import (
	"log/slog"
	"sort"
	"strings"

	"gameflex/consoles"
)

// Game est un jeu trouvé : un fichier, et la console à laquelle il appartient.
type Game struct {
	// Console est l'identifiant du registre partagé ("ps1", "gba"…).
	Console string
	// Title est le nom du fichier sans son extension — pas une métadonnée,
	// juste ce que l'utilisateur a écrit en rangeant ses jeux. C'est
	// délibéré : personne n'a besoin d'une jaquette pour reconnaître un jeu
	// qu'il a copié lui-même.
	Title string
	// Path est le chemin virtuel complet, source comprise.
	Path string
	// Folder est le dossier virtuel qui EST ce jeu, quand il en a un.
	//
	// Un jeu n'est pas toujours un fichier : un PS1 est un .cue et ses .bin,
	// un multi-disques un .m3u et quatre paires. Path ne désigne que celui
	// qui SE LANCE — le copier seul donnait 2 Ko qui ne démarrent rien
	// (signalé le 2026-09-02). Folder dit où sont les autres.
	//
	// Vide pour un jeu qui vit à même le dossier de sa console : là, le jeu
	// EST le fichier, et rien d'autre ne lui appartient.
	//
	// Pour un jeu d'un PAIR, Folder vaut Path quand le pair l'annonce en
	// dossier : c'est alors le jeu entier qu'on désigne, et il voyage zippé.
	Folder string
	// Files — les fichiers à la racine du jeu : le fichier lui-même pour un
	// jeu libre, ce qu'on voit en ouvrant le dossier pour un jeu-dossier.
	//
	// C'est ce qui dit l'extension d'un jeu qu'un autre rangera chez lui —
	// sans elle, aucun cœur ne le reconnaît (N64 fédéré « ROM not found »,
	// 2026-10-10) —, et ce que le listing publie (chantier 15).
	Files []string
	// Source est le nom du montage d'où il vient — pour dire à l'utilisateur
	// que ce jeu est sur son NAS et pas sur son téléphone.
	Source string
	// Kind est la NATURE de cette origine (local, smb, webdav, server).
	// Source seul ne suffit pas : « salon » ne dit pas si c'est un partage
	// ou la machine d'un ami, et les deux ne se lancent pas pareil.
	Kind string
	// Identity — pour un serveur distant, sous quel nom il nous connaît
	// (le label du jeton). Vide partout ailleurs.
	Identity string
	Size     int64
	// Engine — le moteur du jeu (GameEngine), "" s'il n'est pas reconnu ou
	// pas lisible ici (un jeu emballé). Chantier 36.
	Engine string
}

// ScanConvention rend les jeux d'une source rangée selon LA convention de
// GameFlex (chantier 15, exigée par le porteur le 2026-10-13) :
//
//	<base>/<consoleid>/<jeu>.<ext>                 un jeu, nommé par son fichier
//	<base>/<consoleid>/<dossier>/<fichiers du jeu> un jeu, nommé par son dossier
//
// `<consoleid>` est un identifiant du REGISTRE, tel quel : ni alias (« psx »),
// ni majuscules, ni dossier de console à une autre profondeur. Une ressource
// distante rangée autrement ne montre rien — c'est la règle, et elle fait
// qu'un jeu porte le même UUID partout (« console/nom sur le disque »).
// `base` vaut « roms » pour une ressource distante (sa racine porte roms/,
// bios/…), "" pour une source déjà posée sur le dossier des jeux (le
// téléphone).
//
// LES RÈGLES SONT CELLES DU CATALOGUE DU SERVEUR (retro_files) : un dossier
// sous une console est UN jeu quoi qu'il contienne, un fichier libre l'est si
// la console accepte son extension. Deux parcours qui ne listent pas la même
// chose, c'est un jeu listé ici qui n'existe pas là-bas.
//
// Trié par console puis par titre. La racine illisible est une erreur ; un
// dossier de console illisible est sauté, et dit au journal.
func ScanConvention(source Source, base string) ([]Game, error) {
	entries, err := source.List(base)
	if err != nil {
		return nil, err
	}

	kind := KindOf(source)
	var found []Game
	for _, entry := range entries {
		if !entry.Dir || isHidden(entry.Name) || !consoles.Exists(entry.Name) {
			continue
		}
		consoleID := entry.Name
		consoleDir := joinPath(base, consoleID)
		children, err := source.List(consoleDir)
		if err != nil {
			slog.Warn("scan: dossier de console sauté", "source", source.Name(), "dir", consoleDir, "error", err)

			continue
		}
		for _, child := range children {
			if isHidden(child.Name) {
				continue
			}
			if child.Dir {
				found = append(found, folderGame(source, kind, consoleID, joinPath(consoleDir, child.Name)))

				continue
			}
			if !consoles.Accepts(consoleID, child.Name) {
				continue // un .txt, un .sav, un .mp3 n'est pas un jeu
			}
			found = append(found, Game{
				Console: consoleID,
				Title:   titleOf(child.Name),
				Path:    joinPath(source.Name(), consoleDir, child.Name),
				Files:   []string{child.Name},
				Source:  source.Name(),
				Kind:    kind,
				Size:    child.Size,
			})
		}
	}

	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Console != found[j].Console {
			return found[i].Console < found[j].Console
		}

		return strings.ToLower(found[i].Title) < strings.ToLower(found[j].Title)
	})

	return found, nil
}

// folderGame — un dossier sous une console, qui EST un jeu et porte son nom.
//
// Ce qui se lance est le fichier le plus haut de sa racine (un .m3u avant un
// .cue, un .cue avant un .bin, voir layerOf) parmi ceux que la console
// accepte ; sans aucun, le dossier lui-même — un invité de bureau sait ouvrir
// un dossier (GlibcGuest). La taille est celle de sa racine : en descendre
// les sous-dossiers coûterait un aller-retour chacun sur un partage.
func folderGame(source Source, kind, consoleID, dir string) Game {
	game := Game{
		Console: consoleID,
		// LE NOM DU DOSSIER, TEL QUEL : titleOf retirerait une « extension »
		// qu'un dossier n'a pas — « Kingdom Hearts - HD 1.5 ReMIX » devenait
		// « Kingdom Hearts - HD 1 » (2026-09-02).
		Title:  baseName(dir),
		Path:   joinPath(source.Name(), dir),
		Folder: joinPath(source.Name(), dir),
		Files:  []string{},
		Source: source.Name(),
		Kind:   kind,
	}

	entries, err := source.List(dir)
	if err != nil {
		slog.Warn("scan: dossier de jeu illisible", "source", source.Name(), "dir", dir, "error", err)

		return game
	}
	launch := ""
	for _, entry := range entries {
		if entry.Dir || isHidden(entry.Name) {
			continue
		}
		game.Files = append(game.Files, entry.Name)
		game.Size += entry.Size
		if !consoles.Accepts(consoleID, entry.Name) {
			continue
		}
		if launch == "" || layerOf(entry.Name) > layerOf(launch) ||
			(layerOf(entry.Name) == layerOf(launch) && strings.ToLower(entry.Name) < strings.ToLower(launch)) {
			launch = entry.Name
		}
	}
	sort.Strings(game.Files)
	if launch != "" {
		game.Path = joinPath(source.Name(), dir, launch)
	}
	game.Engine = GameEngine(consoleID, entries, func(sub string) ([]Entry, error) {
		return source.List(joinPath(dir, sub))
	})

	return game
}

// baseName — le dernier segment d'un chemin de source (toujours des « / »,
// quelle que soit la plateforme : ce sont des chemins logiques, pas des
// chemins de système de fichiers).
func baseName(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	if cut := strings.LastIndex(trimmed, "/"); cut >= 0 {
		return trimmed[cut+1:]
	}

	return trimmed
}

// isHidden — les fichiers en point, et les dossiers de service des systèmes
// qui se croient obligés d'en semer partout.
func isHidden(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "system volume information", "$recycle.bin", "lost+found", "@eadir":
		return true
	}

	return false
}

// layerOf classe un fichier par ce qu'il DÉCRIT, du plus haut au plus bas.
//
// C'est la hiérarchie que les émulateurs suivent eux-mêmes : on charge un
// .m3u s'il y en a un, sinon un .cue, et jamais le .bin qu'ils désignent.
// Reproduire cet ordre ici, c'est présenter la bibliothèque telle qu'elle se
// JOUE, pas telle qu'elle est stockée.
func layerOf(filename string) int {
	dot := strings.LastIndexByte(filename, '.')
	if dot < 0 {
		return 1
	}

	switch strings.ToLower(filename[dot+1:]) {
	case "m3u":
		return 3 // une liste de disques : LE jeu, pour un multi-disques
	case "cue", "gdi", "ccd", "mds", "pbp":
		return 2 // un descripteur de pistes : le jeu, pas ses données
	}

	return 1
}

func titleOf(filename string) string {
	if dot := strings.LastIndexByte(filename, '.'); dot > 0 {
		return filename[:dot]
	}

	return filename
}
