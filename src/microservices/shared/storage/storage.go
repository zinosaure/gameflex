// Package storage est la bibliothèque de jeux : plusieurs sources composées
// en un seul arbre, et un scanner qui en tire une liste de jeux classés par
// console.
//
// Pourquoi ça n'existe pas déjà : côté serveur, les jeux sont sur la machine
// et le scanner lit un dossier. Sur un téléphone, ils sont ailleurs — un NAS,
// un partage WebDAV, un dossier choisi via le sélecteur d'Android — et
// souvent à plusieurs endroits à la fois.
//
// D'où la forme retenue, celle d'un montage Unix : chaque source est montée
// sous un nom, et le scanner ne connaît QUE la racine composée.
//
//	/smb1/jeux/ps1
//	/smb2/games/roms/PS1
//	/webdav/myroms/gba
//	/saf/roms/PS2
//
// Le paquet est délibérément PUR : pas de réseau, pas d'Android, aucune
// horloge. Une source est une interface de trois méthodes, ce qui rend le
// scanner testable sans monter quoi que ce soit — et permet au même code de
// servir le kernel (Go) et le téléphone (gomobile).
package storage

import (
	"errors"
	"io"
)

var (
	// ErrEmptyName — un montage sans nom n'a pas d'adresse.
	ErrEmptyName = errors.New("une source a besoin d'un nom")
	// ErrOutsideSource — le chemin demandé sort de la racine montée. Une
	// source doit rester une source, quoi qu'on lui passe.
	ErrOutsideSource = errors.New("chemin hors de la source")
)

// Entry est un fichier ou un dossier, tel qu'une source le rapporte.
type Entry struct {
	// Name est le nom seul, sans chemin — "Zelda.sfc", "ps1".
	Name string
	// Dir distingue un dossier d'un fichier. Le scanner descend dans les
	// premiers et juge les seconds.
	Dir bool
	// Size en octets, 0 pour un dossier. Informatif : rien n'en dépend.
	Size int64
}

// ProbeResult — ce qu'on a vu en regardant une source distante.
//
// Pas un simple booléen : « joignable » ne dit pas si on a mis le doigt au
// bon endroit. Qui vient d'ajouter un partage veut savoir qu'on l'atteint ET
// qu'il y a quelque chose à y lire — un dossier vide ou mal nommé se lit
// autrement qu'un mot de passe refusé, et se corrige autrement.
//
// Ici et non dans chaque source : les deux (WebDAV, kDrive) répondent la même
// chose, et l'écran qui l'affiche n'a pas à savoir laquelle lui parle.
type ProbeResult struct {
	// Folders et Files, tels quels, à la profondeur demandée.
	Folders []string `json:"folders"`
	Files   int      `json:"files"`
}

// Source est une origine de fichiers — un disque local, un partage SMB, un
// serveur WebDAV, un arbre SAF côté Android.
//
// Trois méthodes, et c'est voulu : tout ce que le scanner sait faire, c'est
// lister un dossier. Ni recherche, ni métadonnées, ni téléchargement — une
// source qui n'expose que ça peut être écrite en une heure, y compris en
// Kotlin de l'autre côté d'un pont gomobile.
type Source interface {
	// Name est le nom de montage : le premier segment du chemin virtuel.
	Name() string
	// List renvoie le contenu d'un dossier, chemin relatif À LA SOURCE
	// ("" pour sa racine). L'ordre n'a pas d'importance : le scanner trie.
	List(dir string) ([]Entry, error)
}

// Opener est une source dont on peut LIRE les fichiers, et pas seulement
// les nommer.
//
// Interface à part, et optionnelle, pour une raison précise : le scanner
// n'en a pas besoin — il ne lit jamais un octet de jeu, il regarde des noms.
// La garder hors de Source laisse écrire une source qui ne sait que lister
// (un catalogue distant, un arbre SAF côté Android) sans lui inventer un
// Open() qui renverrait toujours une erreur.
//
// Ce dont ELLE a besoin, c'est de C6-P4 : s'approprier un jeu distant, que
// ce soit dans le cache ou dans le catalogue, suppose de le télécharger.
type Opener interface {
	// Open rend le contenu d'un fichier, chemin relatif À LA SOURCE.
	// L'appelant ferme.
	Open(path string) (io.ReadCloser, error)
}

// Kinded est une source qui sait dire DE QUELLE NATURE elle est — un
// partage SMB, un serveur GameFlex, du WebDAV. Sert à choisir une icône et
// un mot dans la liste des jeux, rien de plus.
//
// Optionnelle, et son absence veut dire KindLocal : le disque de la machine
// est le cas par défaut, et une source écrite avant cette interface n'a pas
// à mentir sur ce qu'elle est.
type Kinded interface {
	Kind() string
}

// Les natures connues. Ce sont des mots destinés à être AFFICHÉS à côté
// d'un jeu, pas des identifiants de protocole.
const (
	KindLocal  = "local"
	KindSMB    = "smb"
	KindWebDAV = "webdav"
	KindServer = "server"
)

// KindOf — la nature d'une source, KindLocal à défaut.
func KindOf(source Source) string {
	if kinded, ok := source.(Kinded); ok {
		return kinded.Kind()
	}

	return KindLocal
}
