package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

// AdoptParentOwner — CE QUE LE NOYAU ÉCRIT DANS LA BIBLIOTHÈQUE APPARTIENT À
// SON PROPRIÉTAIRE (#146), et sur un disque interne au propriétaire du disque
// (les métadonnées d'un jeu de disque, chantier 49).
//
// Le noyau tourne en root et le dossier lui est ouvert en écriture
// depuis le 2026-09-09 : tout ce qu'il crée est donc `root:root`. Mesuré le
// 2026-09-17 — `cores/atari5200` sur la dev, et six dossiers sur la release
// (`n64 nes ps1 ps2 psp wii`), là où `bios/` et `roms/`, créés par un humain,
// lui appartiennent.
//
// LE TORT N'EST PAS POUR LE NOYAU, qui peut tout écrire, mais pour la PERSONNE :
// dans un dossier `root:root 755` elle ne peut plus ni remplacer ni supprimer un
// cœur — précisément le remède de #142 et #143. On lui rendait un dossier
// qu'elle ne pouvait plus gérer.
//
// La règle retenue : ce qu'on crée prend l'appartenance de son PARENT. Ni une
// identité configurée (il faudrait la tenir à jour, et elle mentirait le jour
// où la bibliothèque change de main), ni un mode 777 (qui ouvre à tout le
// monde) : le parent EST la réponse, puisque c'est la bibliothèque de
// quelqu'un.
//
// UN ÉCHEC EST IGNORÉ, volontairement. Sur le kernel du téléphone le processus
// n'est pas root, `chown` y est refusé (EPERM) — et il n'y a rien à corriger là,
// puisque le fichier appartient déjà à celui qui l'a écrit. Échouer bruyamment
// transformerait un non-problème en erreur de téléchargement.
func AdoptParentOwner(path string) {
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return
	}
	wanted, ok := parent.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}

	// Déjà au bon propriétaire : ne pas appeler `chown` pour rien, et ne pas
	// journaliser un échec qui n'aurait aucun sens sur un système sans droits.
	if current, err := os.Stat(path); err == nil {
		if mine, ok := current.Sys().(*syscall.Stat_t); ok &&
			mine.Uid == wanted.Uid && mine.Gid == wanted.Gid {
			return
		}
	}

	if err := os.Chown(path, int(wanted.Uid), int(wanted.Gid)); err != nil {
		slog.Debug("appartenance laissée telle quelle", "path", path, "error", err)
	}
}
