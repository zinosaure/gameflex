package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// LA DÉCOUVERTE DES DISQUES INTERNES (chantiers 23 et 24) : chaque dossier de
// /mnt au nom utilisable, dans l'ordre des noms ; présent s'il apporte une
// section (des jeux, ou seulement des BIOS…) ET a un chemin sur l'hôte (sans
// lui, rien ne peut le monter dans une partie).
func TestInternalDisksAreTheFoldersOfMntInNameOrder(t *testing.T) {
	root := t.TempDir()
	previous := internalDisksDir
	internalDisksDir = root
	t.Cleanup(func() { internalDisksDir = previous })

	for _, dir := range []string{"ssd/roms/nes", "usb/saves/gino", "orphelin/roms", "nom invalide/roms", "firmwares/bios/ps2"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	selfMountsMu.Lock()
	previousMounts, previousTried := selfMounts, selfMountsTriedAt
	selfMounts = map[string]string{
		filepath.Join(root, "ssd"):       "/run/media/disque/jeux",
		filepath.Join(root, "usb"):       "/media/usb",
		filepath.Join(root, "firmwares"): "/media/firmwares",
	}
	selfMountsTriedAt = time.Now()
	selfMountsMu.Unlock()
	t.Cleanup(func() {
		selfMountsMu.Lock()
		selfMounts, selfMountsTriedAt = previousMounts, previousTried
		selfMountsMu.Unlock()
	})

	disks := (&Client{}).InternalDisks(context.Background())
	if len(disks) != 4 {
		t.Fatalf("4 disques attendus (firmwares, orphelin, ssd, usb), %d : %+v", len(disks), disks)
	}
	want := []struct {
		name    string
		present bool
		host    string
	}{
		{"firmwares", true, "/media/firmwares"},
		{"orphelin", false, ""},
		{"ssd", true, "/run/media/disque/jeux"},
		{"usb", false, "/media/usb"},
	}
	for index, expected := range want {
		disk := disks[index]
		if disk.Name != expected.name || disk.Present != expected.present || disk.HostDir != expected.host {
			t.Fatalf("disque %d = %+v, attendu %+v", index, disk, expected)
		}
	}
	if disks[2].RomsHostDir() != "/run/media/disque/jeux/roms" {
		t.Fatalf("dossier de jeux sur l'hôte : %s", disks[2].RomsHostDir())
	}
}
