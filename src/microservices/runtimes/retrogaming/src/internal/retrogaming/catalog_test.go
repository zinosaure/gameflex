package retrogaming

import (
	"os"
	"path/filepath"
	"testing"
)

// deriveRomID must be deterministic (the same console and the same ON-DISK
// NAME always yield the same id across restarts/rescans, see catalog.go's own
// doc — history/session references would otherwise all go stale on
// every rescan) and must not collide between different (consoleID, entryName)
// pairs.
//
// LE SECOND ARGUMENT EST LE NOM SUR LE DISQUE, extension comprise, et non le
// titre affiché : c'est ce qu'a changé #167. Ce commentaire, ces noms de test
// et ces fixtures disaient « title » — vocabulaire périmé, corrigé le
// 2026-09-18 pour que le test documente sa vraie entrée. Le comportement
// vérifié, lui, ne change pas : deux chaînes identiques donnent le même UUID.

func TestDeriveRomIDIsDeterministic(t *testing.T) {
	a := deriveRomID("nes", "Super Mario Bros..nes")
	b := deriveRomID("nes", "Super Mario Bros..nes")
	if a != b {
		t.Errorf("deriveRomID is not deterministic: %q != %q for the same input", a, b)
	}
}

func TestDeriveRomIDDiffersByName(t *testing.T) {
	a := deriveRomID("nes", "Super Mario Bros..nes")
	b := deriveRomID("nes", "Super Mario Bros. 2.nes")
	if a == b {
		t.Errorf("deriveRomID collision: different on-disk names on the same console produced the same id %q", a)
	}
}

func TestDeriveRomIDDiffersByConsole(t *testing.T) {
	// Same title on two different consoles (e.g. a game re-released across
	// generations) must NOT collide — ResolveCatalogRom keys strictly by id,
	// and cross-console collisions would let one console's rescan silently
	// resolve to another's file.
	a := deriveRomID("nes", "Tetris")
	b := deriveRomID("gbc", "Tetris")
	if a == b {
		t.Errorf("deriveRomID collision: same title on different consoles produced the same id %q", a)
	}
}

// A folder containing exactly ONE file (a common ROM layout — one .iso per
// named folder, e.g. PSP games) must still resolve as a DIRECTORY
// (IsDir=true, file joined onto {consoleDir}/{Title}/), not be confused
// with a genuinely loose file sitting directly in consoleDir — both used to
// produce len(Files)==1, which crashed ResolveCatalogRom into joining the
// inner filename straight onto consoleDir (missing the folder segment) —
// the real "Failed to read ROM: ... no such file or directory" bug this
// guards against, reproduced with a real PSP library folder in production.
func TestResolveCatalogRomSingleFileFolder(t *testing.T) {
	root := t.TempDir()
	consoleDir := filepath.Join(root, "psp")
	gameDir := filepath.Join(consoleDir, "Some Game [X]")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	isoPath := filepath.Join(gameDir, "Some Game [X].iso")
	if err := os.WriteFile(isoPath, []byte("fake iso data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RescanCatalog(root); err != nil {
		t.Fatalf("RescanCatalog failed: %v", err)
	}

	// La référence que le kernel aurait donnée : console + nom sur le disque —
	// ici le nom du DOSSIER, qu'aucune extension ne suit. C'est pourquoi un
	// jeu-dossier garde son identifiant à travers le changement du 2026-09-18.
	path, isDir, err := ResolveCatalogRom(root, "psp", deriveRomID("psp", "Some Game [X]"))
	if err != nil {
		t.Fatalf("ResolveCatalogRom failed: %v", err)
	}
	if !isDir {
		t.Errorf("isDir = false, want true — a single-file folder must still resolve as a directory")
	}
	if want := gameDir; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	// Confirm the resolved path actually leads to a real, readable file —
	// exactly what handleStart's resolveGameFile (stream_session.go) does
	// next with this same isDir=true result.
	if _, err := os.Stat(filepath.Join(path, "Some Game [X].iso")); err != nil {
		t.Errorf("resolved directory does not contain the expected file: %v", err)
	}
}

// A genuinely loose file directly in consoleDir (no wrapping folder) must
// keep resolving as a plain file, not get misidentified as a directory.
func TestResolveCatalogRomLooseFile(t *testing.T) {
	root := t.TempDir()
	consoleDir := filepath.Join(root, "nes")
	if err := os.MkdirAll(consoleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	romPath := filepath.Join(consoleDir, "Loose Game.nes")
	if err := os.WriteFile(romPath, []byte("fake rom data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RescanCatalog(root); err != nil {
		t.Fatalf("RescanCatalog failed: %v", err)
	}

	// La référence que le kernel aurait donnée : console + nom sur le disque,
	// extension comprise puisque le jeu est un fichier libre.
	path, isDir, err := ResolveCatalogRom(root, "nes", deriveRomID("nes", "Loose Game.nes"))
	if err != nil {
		t.Fatalf("ResolveCatalogRom failed: %v", err)
	}
	if isDir {
		t.Errorf("isDir = true, want false — a loose file must not be treated as a directory")
	}
	if want := romPath; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

// LA MÊME RÉFÉRENCE QUE LE KERNEL — ou un jeu listé ne se lance pas.
//
// Le kernel donne la référence au navigateur (retro_files) ; ce catalogue la
// retrouve au lancement avec sa propre copie de la dérivation. La valeur est
// figée à l'identique dans les trois paquets qui la calculent — voir
// retro_files.TestReferencesMatchTheSharedGoldenValue. L'EXTENSION COMPTE :
// depuis le 2026-09-18 la dérivation porte sur le nom SUR LE DISQUE.
func TestReferencesMatchTheSharedGoldenValue(t *testing.T) {
	const golden = "a16f8b20-e5e2-5991-a712-a4d77ef85a7e"
	if got := deriveRomID("nes", "Super Mario Bros..nes"); got != golden {
		t.Fatalf("deriveRomID(nes, Super Mario Bros..nes) = %s, want %s", got, golden)
	}

	root := t.TempDir()
	romPath := filepath.Join(root, "nes", "Super Mario Bros..nes")
	if err := os.MkdirAll(filepath.Dir(romPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(romPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RescanCatalog(root); err != nil {
		t.Fatalf("RescanCatalog : %v", err)
	}
	path, isDir, err := ResolveCatalogRom(root, "nes", golden)
	if err != nil || isDir || path != romPath {
		t.Errorf("ResolveCatalogRom(nes, %s) = (%q, %v, %v), want (%q, false, nil)", golden, path, isDir, err, romPath)
	}
}
