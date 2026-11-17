package metadata

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gameflex/storage"
)

// switchPackageWithTicket — un `.nsp` (PFS0) qui ne range que le ticket de ce jeu : son identifiant se lit sans clé
// (storage.GameTitleID), comme celui des vrais paquets de Gamebox.
func switchPackageWithTicket(titleID string) []byte {
	names := append([]byte(strings.ToLower(titleID)+"0000000000000005.tik"), 0)
	header := make([]byte, 0x10+0x18)
	copy(header, "PFS0")
	binary.LittleEndian.PutUint32(header[4:], 1)
	binary.LittleEndian.PutUint32(header[8:], uint32(len(names)))
	binary.LittleEndian.PutUint64(header[0x10+8:], 0x2C0)

	return append(append(header, names...), make([]byte, 0x2C0)...)
}

// UN JEU SWITCH DONT L'IDENTIFIANT SE LIT DÉSORMAIS GARDE SES MÉTADONNÉES (2026-11-15) : « Crash Bandicoot.xci » avait
// son dossier sous sa référence ; à la première lecture de sa fiche, ce dossier devient celui de son identifiant, tout
// ce qu'il garde avec lui, et la fiche dit cet identifiant — celui de ses sauvegardes. Relue, rien ne bouge. Un dossier
// qui porte déjà l'identifiant n'est jamais écrasé : il sert, et celui de la référence reste tel quel.
func TestASwitchGameKeepsItsMetadataUnderItsTitleID(t *testing.T) {
	root := t.TempDir()
	roots := Roots{Server: root, Game: root}
	switchDir := filepath.Join(root, "metadata", "switch")

	crash := filepath.Join(root, "roms", "switch", "Crash Bandicoot")
	writeTestFile(t, filepath.Join(crash, "Crash Bandicoot.nsp"), string(switchPackageWithTicket("0100D1B006744000")))
	reference := storage.RomID("switch", "Crash Bandicoot")
	writeTestFile(t, filepath.Join(switchDir, reference, "manual", "source.json"), `{"title":"Crash Bandicoot N. Sane Trilogy"}`)
	writeTestFile(t, filepath.Join(switchDir, reference, "igdb", "cover.jpg"), "la jaquette")

	info := Info(context.Background(), roots, Accounts{}, "switch", crash, reference)
	if info.GameID != "0100D1B006744000" || info.Title != "Crash Bandicoot N. Sane Trilogy" {
		t.Fatalf("la fiche : %+v", info)
	}
	if _, err := os.Stat(filepath.Join(switchDir, reference)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("le dossier de la référence est resté : %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(switchDir, "0100D1B006744000", "igdb", "cover.jpg")); err != nil || string(content) != "la jaquette" {
		t.Fatalf("la jaquette sous l'identifiant : %q, %v", content, err)
	}
	if again := Info(context.Background(), roots, Accounts{}, "switch", crash, reference); again.GameID != info.GameID || again.Title != info.Title {
		t.Fatalf("relue : %+v", again)
	}

	odyssey := filepath.Join(root, "roms", "switch", "Super Mario Odyssey")
	writeTestFile(t, filepath.Join(odyssey, "Super Mario Odyssey.nsp"), string(switchPackageWithTicket("0100000000010000")))
	odysseyReference := storage.RomID("switch", "Super Mario Odyssey")
	writeTestFile(t, filepath.Join(switchDir, odysseyReference, "manual", "source.json"), `{"title":"L'ancien"}`)
	writeTestFile(t, filepath.Join(switchDir, "0100000000010000", "manual", "source.json"), `{"title":"Super Mario Odyssey"}`)
	if got := Info(context.Background(), roots, Accounts{}, "switch", odyssey, odysseyReference); got.GameID != "0100000000010000" || got.Title != "Super Mario Odyssey" {
		t.Fatalf("l'identifiant a déjà son dossier : %+v", got)
	}
	if content, err := os.ReadFile(filepath.Join(switchDir, odysseyReference, "manual", "source.json")); err != nil || !strings.Contains(string(content), "L'ancien") {
		t.Fatalf("le dossier de la référence a changé : %q, %v", content, err)
	}
	if moved, err := MoveReferenceFolder(root, "switch", odysseyReference, "0100000000010000"); moved || !errors.Is(err, ErrGameFolderTaken) {
		t.Fatalf("écraser : %v, %v", moved, err)
	}
	if moved, err := MoveReferenceFolder(root, "switch", odysseyReference, "../../roms"); moved || !errors.Is(err, ErrNotAGameFolder) {
		t.Fatalf("un gameid piégé : %v, %v", moved, err)
	}
}
