package metadata

// L'ARCHIVE DES MÉTADONNÉES D'UN JEU (le porteur, 2026-11-16, le Catalogue des Réglages : « dans une section
// téléchargement on pour télécharger le jeu, télécharger le metadata […] Les téléchargement sont par navigateur ») : son
// dossier `metadata/<console>/<gameid>/`, tel qu'il est — ses sources, leurs images, ses corrections —, dans une archive
// .zip que le NAVIGATEUR télécharge. Ses entrées gardent ce chemin : décompressée à la racine d'un dossier GameFlex
// (celui du serveur, ou un disque), l'archive remet le dossier à sa place. Le kernel du serveur la sert (pour un de ses
// jeux, et à un pair) ; celui du téléphone la servira avec ses métadonnées (chantier 57, P4).

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// GameArchiveFolder — le dossier des métadonnées d'un jeu sous `gameRoot` (Roots.Game, gameDirectory : le seul point où
// il se compose) et son gameid ; os.ErrNotExist quand le jeu n'en a pas encore : aucune de ses fiches n'a été montrée,
// ou aucune source ne le connaît.
func GameArchiveFolder(gameRoot, consoleID, gamePath, reference string) (folder, gameID string, err error) {
	folder = gameDirectory(gameRoot, consoleID, gamePath, reference)
	info, err := os.Stat(folder)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", os.ErrNotExist
	}

	return folder, filepath.Base(folder), nil
}

// WriteGameArchive — l'archive de `folder` (GameArchiveFolder), écrite dans `w` à mesure qu'elle se lit. N'y entrent que
// ses fichiers ordinaires : ni un fichier caché (un marqueur), ni un lien — un lien posé dans le dossier d'un disque ne
// fait pas sortir un fichier d'ailleurs. Un fichier illisible est sauté. Ses entrées : `metadata/<console>/<gameid>/…`.
// Une erreur d'écriture arrête tout : le navigateur a abandonné.
func WriteGameArchive(w io.Writer, folder, consoleID, gameID string) error {
	archive := zip.NewWriter(w)
	prefix := path.Join("metadata", consoleID, gameID)
	walked := filepath.WalkDir(folder, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if current != folder && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}

			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(folder, current)
		if err != nil {
			return nil
		}

		return addArchiveFile(archive, current, path.Join(prefix, filepath.ToSlash(relative)))
	})
	if walked != nil {
		archive.Close()

		return walked
	}

	return archive.Close()
}

// addArchiveFile — un fichier du dossier, sous `name`, avec sa date ; illisible, il est sauté.
func addArchiveFile(archive *zip.Writer, file, name string) error {
	source, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return nil
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return nil
	}
	header.Name, header.Method = name, zip.Deflate
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, source)

	return err
}
