package storage

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// isoSector — la taille d'un secteur ISO 9660.
const isoSector = 2048

// isoFileLimit — ce qu'on accepte de lire d'un fichier de l'image : un
// PARAM.SFO, une icône, un fond. Rien qui demande de charger un jeu.
const isoFileLimit = 32 << 20

// GameISO — l'image disque d'un jeu rangé en `path` : le fichier lui-même
// s'il finit en `.iso`, sinon le premier `.iso` de son dossier (un jeu PS3
// est rangé dans un dossier à son nom, son `.iso` dedans). "" sans image.
func GameISO(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Ext(path), ".iso") {
			return path
		}
		path = filepath.Dir(path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".iso") {
			return filepath.Join(path, entry.Name())
		}
	}

	return ""
}

// ISOFile — le contenu du fichier `name` (« PS3_GAME/ICON0.PNG ») d'une image
// ISO 9660, sans la monter. La casse et le « ;1 » des noms sont ignorés.
// os.ErrNotExist s'il n'y est pas. Sur un disque PS3, le système de fichiers
// et `PS3_GAME/` sont en clair même quand le jeu est chiffré.
func ISOFile(isoPath, name string) ([]byte, error) {
	file, err := os.Open(isoPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	descriptor := make([]byte, isoSector)
	if _, err := file.ReadAt(descriptor, 16*isoSector); err != nil {
		return nil, err
	}
	if descriptor[0] != 1 || string(descriptor[1:6]) != "CD001" {
		return nil, errors.New("not an ISO 9660 image")
	}
	extent, size := isoExtent(descriptor[156:190])
	parts := strings.Split(strings.Trim(name, "/"), "/")
	for index, part := range parts {
		found := false
		records, err := readExtent(file, extent, size)
		if err != nil {
			return nil, err
		}
		for at := 0; at < len(records); {
			length := int(records[at])
			if length == 0 {
				at = (at/isoSector + 1) * isoSector
				continue
			}
			if at+length > len(records) || length < 34 {
				break
			}
			record := records[at : at+length]
			at += length
			nameLength := int(record[32])
			if 33+nameLength > len(record) {
				continue
			}
			entryName, _, _ := strings.Cut(string(record[33:33+nameLength]), ";")
			if !strings.EqualFold(entryName, part) {
				continue
			}
			isDirectory := record[25]&2 != 0
			if isDirectory != (index < len(parts)-1) {
				continue
			}
			extent, size = isoExtent(record)
			found = true
			break
		}
		if !found {
			return nil, os.ErrNotExist
		}
	}
	if size > isoFileLimit {
		return nil, errors.New("file too large")
	}

	return readExtent(file, extent, size)
}

// isoExtent — l'emplacement (en secteurs) et la taille d'un enregistrement
// de répertoire, dans leur moitié petit-boutiste.
func isoExtent(record []byte) (int64, int64) {
	return int64(binary.LittleEndian.Uint32(record[2:6])), int64(binary.LittleEndian.Uint32(record[10:14]))
}

func readExtent(file *os.File, extent, size int64) ([]byte, error) {
	if size > isoFileLimit {
		return nil, errors.New("directory too large")
	}
	content := make([]byte, size)
	if _, err := file.ReadAt(content, extent*isoSector); err != nil && !(errors.Is(err, io.EOF) && size == 0) {
		return nil, err
	}

	return content, nil
}
