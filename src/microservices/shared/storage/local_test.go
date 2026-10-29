package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// Le fournisseur local contre un VRAI arbre de fichiers : le reste des tests
// passe par une source en mémoire, et une source qui ne lit jamais un disque
// ne prouve rien sur celle qui le fera.
func TestLocalSourceScansARealTree(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{
		"roms/gba/Metroid Fusion.gba",
		"roms/ps1/Tekken 3.chd",
		"roms/ps1/notes.txt", // extension absente de ps1 : écartée
		"roms/.hidden/x.gba", // dossier caché : jamais visité
	} {
		full := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("rom"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	source, err := NewLocalSource("interne", root)
	if err != nil {
		t.Fatalf("source : %v", err)
	}
	games := scanOf(t, source)

	if len(games) != 2 {
		t.Fatalf("2 jeux attendus, %d : %+v", len(games), games)
	}
	for _, game := range games {
		if game.Size == 0 {
			t.Errorf("taille non lue pour %q", game.Title)
		}
	}
}

// Un chemin qui tente de remonter au-dessus de la racine montée doit rester
// dans la source. Une source qui fuit est une source qui expose tout le
// téléphone.
func TestLocalSourceStaysInsideItsRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "..", "dehors")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outside)
	if err := os.WriteFile(filepath.Join(outside, "secret.gba"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	source, _ := NewLocalSource("interne", filepath.Join(root, "roms"))
	entries, err := source.List("../../dehors")
	if err != ErrOutsideSource {
		t.Fatalf("la sortie de racine doit être refusée explicitement, err=%v", err)
	}
	for _, entry := range entries {
		if entry.Name == "secret.gba" {
			t.Fatal("la source a lu au-dessus de sa racine")
		}
	}
}

func TestLocalSourceOnAMissingFolderIsNotAFailure(t *testing.T) {
	source, _ := NewLocalSource("interne", filepath.Join(t.TempDir(), "jamais-créé"))

	entries, err := source.List("")
	if err != nil {
		t.Fatalf("un dossier absent ne doit pas casser le scan : %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("rien n'était attendu : %+v", entries)
	}
}
