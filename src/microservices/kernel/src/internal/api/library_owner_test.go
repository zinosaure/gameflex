package api

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"gameflex/kernel/internal/config"
)

// LE RANGEMENT DU DOSSIER, CRÉÉ POUR SON PROPRIÉTAIRE (chantier 24) : ce qui
// manque est créé et lui appartient ; ce qui existait déjà n'est pas touché.
func TestTheStorageFoldersAreCreatedForTheFolderOwner(t *testing.T) {
	root := t.TempDir()
	if err := os.Chown(root, 4242, 4242); err != nil {
		t.Skip("changer de propriétaire demande root : ", err)
	}
	if err := os.Mkdir(filepath.Join(root, "roms"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := ownerOf(t, filepath.Join(root, "roms"))

	if err := EnsureStorageFolders(root); err != nil {
		t.Fatal(err)
	}
	for _, folder := range config.StorageFolders {
		want := uint32(4242)
		if folder == "roms" {
			want = before
		}
		if got := ownerOf(t, filepath.Join(root, folder)); got != want {
			t.Fatalf("%s appartient à %d, attendu %d", folder, got, want)
		}
	}
}

func ownerOf(t *testing.T, path string) uint32 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("%s : %v", path, err)
	}

	return info.Sys().(*syscall.Stat_t).Uid
}
