package metadata

import (
	"os"
	"path/filepath"
	"testing"
)

// L'EMPREINTE CONNUE, SANS LIRE LE JEU (chantier 53) : KnownGameKey ne rend que ce que GameKey a déjà gardé pour ce
// fichier tel qu'il est ; réécrit (une autre taille), il ne la connaît plus, jusqu'à ce que GameKey le relise. Un jeu
// qui ne se lit plus a son repli, comme chez GameKey.
func TestKnownGameKeyIsOnlyWhatIsInMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Jeu.nes")
	if err := os.WriteFile(path, []byte("le contenu"), 0o644); err != nil {
		t.Fatal(err)
	}
	if key, known := KnownGameKey(path, "repli"); known {
		t.Fatalf("jamais lu : %q connu", key)
	}
	read := GameKey(path, "repli")
	if key, known := KnownGameKey(path, "repli"); !known || key != read {
		t.Fatalf("lu : %q (connu %v), GameKey %q", key, known, read)
	}

	if err := os.WriteFile(path, []byte("un autre contenu, plus long"), 0o644); err != nil {
		t.Fatal(err)
	}
	if key, known := KnownGameKey(path, "repli"); known {
		t.Fatalf("réécrit : l'empreinte d'avant %q passe pour connue", key)
	}
	if again := GameKey(path, "repli"); again == read {
		t.Fatalf("réécrit : la même empreinte %q", again)
	}

	if key, known := KnownGameKey(filepath.Join(t.TempDir(), "absent.nes"), "repli"); !known || key != "repli" {
		t.Fatalf("absent : %q (connu %v)", key, known)
	}
}
