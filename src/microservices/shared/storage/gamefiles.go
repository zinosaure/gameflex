package storage

// DE QUOI UN JEU EST FAIT.
//
// Le scanner désigne UN fichier par jeu : celui qui se lance. C'est ce qu'il
// faut pour afficher une bibliothèque, et c'est faux dès qu'on veut EMPORTER
// le jeu — un PS1 est un .cue et ses .bin, un PSP un .iso rangé dans son
// dossier, un multi-disques un .m3u et quatre paires. Copier le seul fichier
// désigné donnait 2 Ko qui ne démarrent rien, et un « Tekken 6.iso » posé à
// plat sous un nom que le catalogue ne reconnaissait plus (2026-09-02).
//
// Ce fichier répond à la question que personne ne posait : « et les autres ? »

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// ErrUnexpectedTree — le jeu n'est pas rangé comme on l'attend, et on refuse
// de deviner. Emporter le mauvais dossier est pire que ne rien emporter.
var ErrUnexpectedTree = errors.New("storage: arborescence de jeu inattendue")

// maxGameDepth — jusqu'où on descend dans le dossier d'un jeu.
//
// Un jeu rangé va rarement plus loin que deux niveaux (disc1/, media/). La
// borne n'est pas une optimisation : c'est ce qui garantit qu'un partage
// distant mal rangé — ou un lien qui boucle — ne fasse pas descendre la
// moitié d'un drive au motif qu'on a demandé un jeu.
const maxGameDepth = 8

// File — un fichier à copier.
type File struct {
	// Name est le chemin RELATIF au jeu : "Tekken 6.iso", ou
	// "disc1/track01.bin" quand le jeu a des sous-dossiers. C'est lui qui
	// dit à celui qui copie quels dossiers créer.
	Name string
	// Path est le chemin virtuel complet, à redonner tel quel à la source.
	Path string
	Size int64
}

// GameFolderOf — le dossier qui EST ce jeu, ou "" si le jeu est son fichier.
//
// LA FORME ATTENDUE, et rien d'autre :
//
//	roms/{console}/jeu.ext                 → un fichier, et lui seul
//	roms/{console}/{dossier}/jeu.ext …     → le dossier entier
//	roms/{console}/{a}/{b}/jeu.ext         → refusé
//
// La question se répond donc SUR LE CHEMIN : le dossier qui contient le
// fichier porte-t-il un nom de console ? Si oui, le jeu est ce fichier. Sinon
// c'est un dossier de jeu, et son parent doit être la console — sans quoi
// l'arborescence n'est pas celle qu'on attend et on ne devine pas.
//
// Aucun aller-retour réseau. La version précédente listait le dossier parent
// pour compter les jeux qu'il contient : juste, mais payé en 11 requêtes
// paginées sur un dossier NES de 10 785 entrées, avant même le premier octet
// (mesuré le 2026-09-02). Le catalogue, lui, tranche déjà pendant le
// parcours — c'est lui qu'on lit en premier ; ceci n'est que le repli.
func GameFolderOf(source Source, console, gamePath string) (string, error) {
	inside, ok := insideSource(source.Name(), gamePath)
	if !ok {
		return "", ErrOutsideSource
	}

	parent := parentOf(inside)
	if parent == "" {
		return "", ErrUnexpectedTree // un fichier à la racine du partage
	}

	// Le dossier de la console — son identifiant tel quel, la convention
	// (chantier 15) : le jeu est son fichier.
	if baseName(parent) == console {
		return "", nil
	}

	// Sinon c'est un dossier de jeu — et on ne remonte QU'UNE fois.
	grandParent := parentOf(parent)
	if grandParent == "" {
		return "", ErrUnexpectedTree
	}
	if baseName(grandParent) != console {
		// Deux niveaux de dossiers ou plus : ce n'est pas la forme attendue,
		// et deviner ici reviendrait à emporter n'importe quoi.
		return "", ErrUnexpectedTree
	}

	return joinPath(source.Name(), parent), nil
}

// GameFiles rend tout ce qu'il faut copier pour s'approprier un jeu.
//
// `folder` est le Folder du jeu tel que le scanner l'a rapporté : vide quand
// le jeu est un fichier seul, et c'est alors ce fichier-là qu'on rend, avec
// sa taille — un seul aller-retour sert donc les deux cas.
//
// Une copie CONFORME : ce qui est dans le dossier y reste, aux mêmes places.
// Pas d'archive au milieu — un .zip qui est vraiment le jeu (les ROMs NDS le
// sont) doit arriver zippé, et un dossier qu'on aurait zippé pour le
// transport devait être ouvert à l'arrivée par quelqu'un qui n'a aucun moyen
// de distinguer les deux.
func GameFiles(source Source, gamePath, folder string) ([]File, error) {
	if folder == "" {
		return loneFile(source, gamePath)
	}

	inside, ok := insideSource(source.Name(), folder)
	if !ok {
		return nil, ErrOutsideSource
	}

	var files []File
	if err := walkGame(source, folder, inside, "", 0, &files); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		// Le dossier est vide ou illisible : on ne rend pas « rien » en
		// silence, on retombe sur le fichier que le scanner désignait. Mieux
		// vaut le jeu incomplet qu'on avait avant que pas de jeu du tout.
		return loneFile(source, gamePath)
	}

	sort.SliceStable(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return files, nil
}

// loneFile — le jeu est son fichier. On liste quand même son dossier, pour
// en connaître la TAILLE : sans elle il n'y a pas de barre de progression, ni
// rien à comparer une fois la copie finie.
func loneFile(source Source, gamePath string) ([]File, error) {
	name := baseName(gamePath)
	file := File{Name: name, Path: gamePath}

	parent := parentOf(gamePath)
	if inside, ok := insideSource(source.Name(), parent); ok {
		if entries, err := source.List(inside); err == nil {
			for _, entry := range entries {
				if !entry.Dir && entry.Name == name {
					file.Size = entry.Size

					break
				}
			}
		}
	}

	return []File{file}, nil
}

func walkGame(source Source, virtualDir, insideDir, prefix string, depth int, out *[]File) error {
	if depth > maxGameDepth {
		return nil
	}

	entries, err := source.List(insideDir)
	if err != nil {
		if depth == 0 {
			return err
		}

		return nil // une branche illisible ne perd pas le reste du jeu
	}

	for _, entry := range entries {
		if isHidden(entry.Name) {
			continue
		}
		if entry.Dir {
			if err := walkGame(source,
				joinPath(virtualDir, entry.Name),
				joinPath(insideDir, entry.Name),
				joinPath(prefix, entry.Name),
				depth+1, out); err != nil {
				return err
			}

			continue
		}
		*out = append(*out, File{
			Name: joinPath(prefix, entry.Name),
			Path: joinPath(virtualDir, entry.Name),
			Size: entry.Size,
		})
	}

	return nil
}

// insideSource traduit un chemin virtuel en chemin propre à la source, et dit
// s'il lui appartient vraiment. Sans cette vérification, un chemin venu du
// réseau désignerait le contenu d'une AUTRE source montée.
func insideSource(name, virtual string) (string, bool) {
	first, rest := splitPath(virtual)
	if first != name {
		return "", false
	}

	return rest, true
}

// parentOf — le dossier d'un chemin virtuel, chaîne vide s'il n'en a pas.
func parentOf(path string) string {
	clean := trimPath(path)
	if cut := strings.LastIndexByte(clean, '/'); cut >= 0 {
		return clean[:cut]
	}

	return ""
}

// RomFileName dérive le nom de fichier d'un jeu rapatrié, et refuse tout ce
// qui n'est pas un nom simple.
//
// PARTAGÉE PAR LES DEUX KERNELS (2026-10-10) : le serveur et le téléphone
// rapatrient un jeu d'un pair, et un nom dérivé de deux façons finirait par
// ne plus désigner le même fichier.
//
// LE TITRE VIENT D'UNE PAGE, et il sert à écrire sur le disque : « ../../
// databases » y passerait sans cette garde (même raisonnement que
// `saveUserDir`). On ne le corrige pas, on le refuse — deviner ce qu'un nom
// piégé voulait dire est exactement la mauvaise réponse.
//
// L'EXTENSION EST PRISE SUR LE PREMIER FICHIER, quand le jeu en a un seul :
// un scanner reconnaît un jeu par son extension, et un fichier qui arrive nu
// ne se lancerait jamais. Un jeu multi-fichiers, lui, arrive zippé et devient
// un dossier — il n'a donc pas d'extension à porter, et le titre suffit.
func RomFileName(title string, files []string) (string, error) {
	clean := strings.TrimSpace(title)
	if clean == "" {
		return "", errors.New("title est requis pour un jeu")
	}
	if clean != filepath.Base(clean) || clean == "." || clean == ".." ||
		strings.ContainsAny(clean, `/\`) {
		return "", errors.New("ce titre ne peut pas servir de nom de fichier")
	}

	if len(files) != 1 {
		return clean, nil
	}

	extension := filepath.Ext(filepath.Base(strings.TrimSpace(files[0])))
	if extension == "" || strings.ContainsAny(extension, `/\`) {
		return clean, nil
	}
	if strings.HasSuffix(strings.ToLower(clean), strings.ToLower(extension)) {
		return clean, nil
	}

	return clean + extension, nil
}
