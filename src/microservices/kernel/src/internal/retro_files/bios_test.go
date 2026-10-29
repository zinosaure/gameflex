package retro_files

import (
	"os"
	"path/filepath"
	"testing"
)

// Ces cas vivaient côté runtime, sur une copie de ListBiosFiles que plus rien
// n'appelait : le sélecteur de BIOS est servi par le kernel, dont la copie
// n'avait aucun test. La copie morte est partie le 2026-09-14 ; la couverture
// suit le code qui tourne.

func testWriteBios(t *testing.T, root string, parts ...string) {
	t.Helper()
	path := filepath.Join(append([]string{root}, parts...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// LE DOSSIER EST L'IDENTIFIANT DE LA CONSOLE (registre, 2026-09-01). La copie du
// runtime testait `psx/` — l'ancien nom — et elle passait : un test peut figer
// une table périmée aussi solidement qu'il protège une règle.
func TestListBiosFilesReadsTheConsoleIdFolder(t *testing.T) {
	root := t.TempDir()
	testWriteBios(t, root, "ps1", "scph5501.bin")
	testWriteBios(t, root, "ps1", "scph1001.bin")
	testWriteBios(t, root, "psx", "scph7001.bin") // l'ancien nom : ne doit rien offrir

	files, err := ListBiosFiles(root, "ps1")
	if err != nil {
		t.Fatalf("ListBiosFiles(ps1) : %v", err)
	}
	want := []string{"scph1001.bin", "scph5501.bin"}
	if len(files) != len(want) || files[0] != want[0] || files[1] != want[1] {
		t.Errorf("ListBiosFiles(ps1) = %v, want %v", files, want)
	}
}

// Les deux formes — un .bin nu, ou un dossier qui tient un dump et ses
// compagnons — sont offertes, triées, et un fichier caché ne l'est jamais.
func TestListBiosFilesOffersBothShapesSortedWithoutHiddenFiles(t *testing.T) {
	root := t.TempDir()
	testWriteBios(t, root, "ps2", "scph10000", "scph10000.bin")
	testWriteBios(t, root, "ps2", "ps2-0220e.bin")
	testWriteBios(t, root, "ps2", ".DS_Store")

	files, err := ListBiosFiles(root, "ps2")
	if err != nil {
		t.Fatalf("ListBiosFiles(ps2) : %v", err)
	}
	want := []string{"ps2-0220e.bin", "scph10000"}
	if len(files) != len(want) || files[0] != want[0] || files[1] != want[1] {
		t.Errorf("ListBiosFiles(ps2) = %v, want %v", files, want)
	}
}

// « Rien à choisir » n'est pas une erreur : une liste vide, jamais nil.
func TestListBiosFilesWithoutAFolderIsAnEmptyList(t *testing.T) {
	files, err := ListBiosFiles(t.TempDir(), "nes")
	if err != nil {
		t.Fatalf("ListBiosFiles(nes) : %v", err)
	}
	if files == nil || len(files) != 0 {
		t.Errorf("ListBiosFiles(nes) = %#v, want une liste vide non nil", files)
	}
}

// ResolveBiosFile ne sert QUE ce que ListBiosFiles offre : un nom caché, une
// traversée ou un absent sont refusés de la même façon.
func TestResolveBiosFileServesOnlyWhatIsOffered(t *testing.T) {
	root := t.TempDir()
	testWriteBios(t, root, "ps1", "scph5501.bin")
	testWriteBios(t, root, "ps1", ".hidden.bin")

	path, isDir, err := ResolveBiosFile(root, "ps1", "scph5501.bin")
	if err != nil || isDir || path != filepath.Join(root, "ps1", "scph5501.bin") {
		t.Errorf("ResolveBiosFile(scph5501.bin) = (%q, %v, %v)", path, isDir, err)
	}
	for _, name := range []string{".hidden.bin", "../ps1/scph5501.bin", "absent.bin"} {
		if _, _, err := ResolveBiosFile(root, "ps1", name); err == nil {
			t.Errorf("ResolveBiosFile(%q) a servi un nom que la liste n'offre pas", name)
		}
	}
}
