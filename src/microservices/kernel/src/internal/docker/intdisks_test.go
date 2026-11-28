package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// LA DÉCOUVERTE DES DISQUES INTERNES (chantiers 23, 24 et 86) : chaque point de montage du dossier des disques (/mnt,
// monté depuis l'hôte en `rslave`) au nom utilisable, dans l'ordre des noms ; son dossier `gameflex/` s'il en a un,
// sinon sa racine ; présent s'il apporte une section (des jeux, ou seulement des BIOS…), s'il a un chemin sur l'hôte
// (sans lui, rien ne peut le monter dans une partie), et s'il n'est pas coupé.
func TestInternalDisksAreTheFoldersOfMntInNameOrder(t *testing.T) {
	root := t.TempDir()
	previous := internalDisksDir
	internalDisksDir = root
	t.Cleanup(func() { internalDisksDir = previous })
	state := t.TempDir()
	previousState := disksStateDir
	disksStateDir = state
	t.Cleanup(func() { disksStateDir = previousState })

	for _, dir := range []string{"ssd/roms/nes", "usb/saves/gino", "Gamebox/gameflex/roms/ps4", "Gamebox/github", "nom invalide/roms", "firmwares/bios/ps2", "coupe/roms/snes"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetDiskEnabled("coupe", false); err != nil {
		t.Fatal(err)
	}

	selfMountsMu.Lock()
	previousMounts, previousTried := selfMounts, selfMountsTriedAt
	selfMounts = map[string]string{filepath.Clean(root): "/mnt/gameflex"}
	selfMountsTriedAt = time.Now()
	selfMountsMu.Unlock()
	t.Cleanup(func() {
		selfMountsMu.Lock()
		selfMounts, selfMountsTriedAt = previousMounts, previousTried
		selfMountsMu.Unlock()
	})

	disks := (&Client{}).InternalDisks(context.Background())
	want := []struct {
		name    string
		present bool
		enabled bool
		host    string
	}{
		{"Gamebox", true, true, "/mnt/gameflex/Gamebox/gameflex"},
		{"coupe", false, false, "/mnt/gameflex/coupe"},
		{"firmwares", true, true, "/mnt/gameflex/firmwares"},
		{"ssd", true, true, "/mnt/gameflex/ssd"},
		{"usb", false, true, "/mnt/gameflex/usb"},
	}
	if len(disks) != len(want) {
		t.Fatalf("%d disques attendus, %d : %+v", len(want), len(disks), disks)
	}
	for index, expected := range want {
		disk := disks[index]
		if disk.Name != expected.name || disk.Present != expected.present || disk.Enabled != expected.enabled || disk.HostDir != expected.host {
			t.Fatalf("disque %d = %+v, attendu %+v", index, disk, expected)
		}
	}
	if disks[0].RomsDir() != filepath.Join(root, "Gamebox", "gameflex", "roms") {
		t.Fatalf("dossier de jeux de Gamebox : %s", disks[0].RomsDir())
	}

	// Réactivé, il revient.
	if err := SetDiskEnabled("coupe", true); err != nil {
		t.Fatal(err)
	}
	if again := (&Client{}).InternalDisks(context.Background()); !again[1].Present || !again[1].Enabled {
		t.Fatalf("réactivé, le disque coupe devait revenir : %+v", again[1])
	}
	if disks[3].RomsHostDir() != "/mnt/gameflex/ssd/roms" {
		t.Fatalf("dossier de jeux sur l'hôte : %s", disks[3].RomsHostDir())
	}
}
