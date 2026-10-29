package retro_files

import (
	"os"
	"path/filepath"
	"testing"
)

// LES FICHIERS D'UN ÉMULATEUR DE BUREAU (chantier 29) : sous une console qui
// tourne en conteneur, l'archive de l'éditeur, notre construction et les
// paquets du téléphone — ceux-ci reconnus comme tels. Rien d'autre : ni un
// transfert inachevé, ni un fichier caché, ni un nom qui sortirait du dossier.
func TestEmulatorFilesAreListedUnderTheirConsole(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ps5")
	if err := os.MkdirAll(filepath.Join(dir, "un-dossier"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"kyty-2026-10-23+box64-d653d0ebe-forky-r1-aarch64.zip",
		"kyty-2026-10-23+box64-d653d0ebe-forky-r1-x86_64.zip",
		"kyty_emulator-gameflex-KytyPS5-2026-10-23-6aaa533",
		"KytyPS5-2026-10-23-6aaa533-Linux-x86_64.tar.gz",
		"KytyPS5-2026-10-24-0000000-Linux-x86_64.tar.gz.part",
		".deleting-ancien",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	phone, server := 0, 0
	for _, file := range ListEmulatorFiles(root, "ps5") {
		if file.Phone {
			phone++
		} else {
			server++
		}
	}
	if phone != 2 || server != 2 {
		t.Fatalf("2 paquets du téléphone et 2 fichiers du serveur attendus, pas %d et %d", phone, server)
	}
	for _, name := range []string{"../ps4/x-aarch64.zip", "a/b.AppImage", "-x.zip", "x.AppImage.part", ".cache"} {
		if IsEmulatorFile("ps5", name) {
			t.Fatalf("%q ne doit pas passer pour un fichier d'émulateur", name)
		}
	}
	if IsEmulatorFile("nes", "fceumm_libretro.so") || ListEmulatorFiles(root, "nes") != nil {
		t.Fatal("une console rétro n'a pas d'émulateur de bureau")
	}
	if ListEmulatorFiles(root, "ps4") != nil {
		t.Fatal("une console sans dossier n'a pas de fichier")
	}
}
