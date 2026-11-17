package storage

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// isoRecord — un enregistrement de répertoire ISO 9660.
func isoRecord(name string, extent, size int, directory bool) []byte {
	length := 33 + len(name)
	if length%2 == 1 {
		length++
	}
	record := make([]byte, length)
	record[0] = byte(length)
	binary.LittleEndian.PutUint32(record[2:], uint32(extent))
	binary.BigEndian.PutUint32(record[6:], uint32(extent))
	binary.LittleEndian.PutUint32(record[10:], uint32(size))
	binary.BigEndian.PutUint32(record[14:], uint32(size))
	if directory {
		record[25] = 2
	}
	record[32] = byte(len(name))
	copy(record[33:], name)

	return record
}

// fakePS3ISO — une image ISO 9660 fabriquée : `PS3_GAME/` et ses fichiers,
// un par secteur à partir du 20. Aucune donnée de jeu dans le dépôt.
func fakePS3ISO(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	image := make([]byte, 40*isoSector)
	descriptor := image[16*isoSector:]
	descriptor[0] = 1
	copy(descriptor[1:], "CD001")
	copy(descriptor[156:], isoRecord("\x00", 18, isoSector, true))

	root := image[18*isoSector:]
	at := copy(root, isoRecord("\x00", 18, isoSector, true))
	at += copy(root[at:], isoRecord("\x01", 18, isoSector, true))
	copy(root[at:], isoRecord("PS3_GAME", 19, isoSector, true))

	game := image[19*isoSector:]
	at = copy(game, isoRecord("\x00", 19, isoSector, true))
	at += copy(game[at:], isoRecord("\x01", 18, isoSector, true))
	sector := 20
	for name, content := range files {
		at += copy(game[at:], isoRecord(name+";1", sector, len(content), false))
		copy(image[sector*isoSector:], content)
		sector++
	}
	writeFile(t, path, image)
}

func TestAFileIsReadFromAnISOWithoutMountingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Un jeu", "Un jeu (Europe).iso")
	fakePS3ISO(t, path, map[string][]byte{"ICON0.PNG": []byte("icône"), "PARAM.SFO": sfoWith("TITLE_ID", "BLES00783")})

	content, err := ISOFile(path, "ps3_game/icon0.png")
	if err != nil || string(content) != "icône" {
		t.Fatalf("content = %q, err = %v", content, err)
	}
	if _, err := ISOFile(path, "PS3_GAME/PIC1.PNG"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent file: err = %v", err)
	}
	if _, err := ISOFile(path, "PS3_GAME"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a directory is not a file: err = %v", err)
	}

	// Le dossier du jeu, ou son image : le même identifiant.
	if got := GameTitleID(filepath.Dir(path)); got != "BLES00783" {
		t.Fatalf("folder: %q", got)
	}
	if got := GameTitleID(path); got != "BLES00783" {
		t.Fatalf("image: %q", got)
	}
}

func TestSomethingElseIsNotAnISO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "faux.iso")
	writeFile(t, path, make([]byte, 20*isoSector))
	if _, err := ISOFile(path, "PS3_GAME/PARAM.SFO"); err == nil {
		t.Fatal("an empty file was read as an ISO")
	}
	if got := GameTitleID(path); got != "" {
		t.Fatalf("title = %q", got)
	}
}
