package installs

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// write — un fichier de `size` octets, ses dossiers compris.
func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sfo — un PARAM.SFO minimal, avec son seul TITLE.
func sfo(title string) []byte {
	key := []byte("TITLE\x00")
	value := append([]byte(title), 0)
	keys := uint32(20 + 16)
	values := keys + uint32(len(key))
	header := make([]byte, 20)
	copy(header, "\x00PSF")
	binary.LittleEndian.PutUint32(header[4:], 0x101)
	binary.LittleEndian.PutUint32(header[8:], keys)
	binary.LittleEndian.PutUint32(header[12:], values)
	binary.LittleEndian.PutUint32(header[16:], 1)
	index := make([]byte, 16)
	binary.LittleEndian.PutUint16(index[0:], 0)
	binary.LittleEndian.PutUint16(index[2:], 0x0204)
	binary.LittleEndian.PutUint32(index[4:], uint32(len(value)))
	binary.LittleEndian.PutUint32(index[8:], uint32(len(value)))
	binary.LittleEndian.PutUint32(index[12:], 0)

	return append(append(append(header, index...), key...), value...)
}

// CE QUE LE CACHE CONTIENT, PAR NATURE : la PS4 par jeu déballé, la PS3 par
// usage et par jeu (son titre lu dans son PARAM.SFO) ; ni les verrous, ni les
// déballages inachevés, ni un dossier vide.
func TestTheCacheIsListedByKind(t *testing.T) {
	root := t.TempDir()
	write(t, Dir(root, "ps4", "Sonic", "eboot.bin"), 100)
	write(t, Dir(root, "ps4", "Sonic.lock"), 0)
	write(t, Dir(root, "ps4", "FF7.partial", "eboot.bin"), 50)
	write(t, Dir(root, "ps3", "dev_flash", "vsh", "module", "vsh.self"), 30)
	write(t, Dir(root, "ps3", "game", "NPUB30423", "USRDIR", "EBOOT.BIN"), 20)
	if err := os.WriteFile(Dir(root, "ps3", "game", "NPUB30423", "PARAM.SFO"), sfo("AC Brotherhood"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(Dir(root, "ps3", "cache"), 0o755); err != nil {
		t.Fatal(err)
	}

	consoles, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Entry{}
	for _, console := range consoles {
		for _, entry := range console.Entries {
			byName[console.ConsoleID+"/"+entry.Name] = entry
		}
	}
	if len(byName) != 3 {
		t.Fatalf("trois entrées attendues : %+v", byName)
	}
	if entry := byName["ps4/Sonic"]; entry.Kind != KindUnpacked || entry.SizeBytes != 100 {
		t.Fatalf("Sonic déballé : %+v", entry)
	}
	if entry := byName["ps3/dev_flash"]; entry.Kind != KindFirmware {
		t.Fatalf("le firmware : %+v", entry)
	}
	if entry := byName["ps3/game/NPUB30423"]; entry.Kind != KindGameData || entry.Label != "AC Brotherhood" {
		t.Fatalf("le jeu installé, sous son titre : %+v", entry)
	}
}

// LA SUPPRESSION ne prend qu'une entrée de la liste, emporte son verrou et son
// déballage inachevé, et rend la main tout de suite.
func TestRemoveTakesOnlyAListedEntry(t *testing.T) {
	root := t.TempDir()
	write(t, Dir(root, "ps4", "Sonic", "eboot.bin"), 100)
	write(t, Dir(root, "ps4", "Sonic.lock"), 0)
	write(t, Dir(root, "ps4", "Sonic.partial", "x"), 1)
	write(t, filepath.Join(root, "roms", "ps4", "Sonic", "Sonic.pkg"), 10)

	for _, name := range []string{"../../roms/ps4/Sonic", "Sonic.lock", "Inconnu", ""} {
		if err := Remove(root, "ps4", name); !errors.Is(err, ErrUnknown) {
			t.Fatalf("%q : %v, attendu ErrUnknown", name, err)
		}
	}
	if err := Remove(root, "ps4", "Sonic"); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"Sonic", "Sonic.lock", "Sonic.partial"} {
		if _, err := os.Stat(Dir(root, "ps4", gone)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s est resté", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "roms", "ps4", "Sonic", "Sonic.pkg")); err != nil {
		t.Fatal("le .pkg de la bibliothèque a été touché")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		left, _ := filepath.Glob(Dir(root, "ps4", ".deleting-*"))
		if len(left) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("l'effacement en fond n'a pas fini : %v", left)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if consoles, _ := List(root); len(consoles) != 0 {
		t.Fatalf("le cache devrait être vide : %+v", consoles)
	}
}
