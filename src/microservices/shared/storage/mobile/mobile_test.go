package mobile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ce paquet est la SEULE porte entre l'application Android et les règles de
// scan, et il n'avait aucun test. Ce qu'on vérifie ici n'est pas le scan
// lui-même (il est testé dans `gameflex/storage`) mais le contrat du pont :
// ce qui entre, ce qui sort, et ce qui se perd en route.

// LA BIBLIOTHÈQUE DU TÉLÉPHONE suit la convention (chantier 15) : une source
// posée sur le dossier des jeux, `<consoleid>/<jeu>` dessous — l'identifiant
// du registre, tel quel.
func TestScanReadsTheConvention(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"nes/Metroid.nes", "psx/Crash.cue", "ps1/[PS1] Ace/Ace.cue"} {
		target := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entree, _ := json.Marshal([]SourceSpec{{Name: "appareil", Kind: "local", Root: root}})
	sortie, err := Scan(string(entree))
	if err != nil {
		t.Fatalf("scan : %v", err)
	}
	var jeux []struct{ Console, Title, Path, Folder string }
	if err := json.Unmarshal([]byte(sortie), &jeux); err != nil || len(jeux) != 2 {
		t.Fatalf("jeux = %s (%v)", sortie, err)
	}
	if jeux[0].Path != "appareil/nes/Metroid.nes" || jeux[1].Title != "[PS1] Ace" || jeux[1].Path != "appareil/ps1/[PS1] Ace/Ace.cue" {
		t.Fatalf("jeux = %+v", jeux)
	}
}

// UN PARTAGE N'EST PAS UNE SOURCE DE BIBLIOTHÈQUE (chantier 14) : il se dit,
// avec son nom.
func TestScanRefusesARemoteKind(t *testing.T) {
	for _, kind := range []string{"webdav", "kdrive", "smb"} {
		entree, _ := json.Marshal([]SourceSpec{{Name: "drive", Kind: kind, BaseURL: "http://x"}})
		if _, err := Scan(string(entree)); err == nil || !strings.Contains(err.Error(), kind) {
			t.Errorf("%s : %v, attendu une erreur qui le nomme", kind, err)
		}
	}
}

// « Aucun jeu » et « je n'ai pas pu lire » sont deux réponses différentes.
// Une liste vide doit sortir en `[]` et jamais en `null` : le commentaire de
// Scan cite le « Value null … cannot be converted » que ça coûtait côté
// Kotlin, et c'est exactement le genre de régression qu'un test fige.
func TestScanRendersAnEmptyLibraryAsAnArray(t *testing.T) {
	sortie, err := Scan("[]")
	if err != nil {
		t.Fatalf("scan : %v", err)
	}
	if sortie != "[]" {
		t.Fatalf("sortie = %q, attendu \"[]\"", sortie)
	}

	var jeux []map[string]any
	if err := json.Unmarshal([]byte(sortie), &jeux); err != nil {
		t.Fatalf("la sortie n'est pas du JSON valide : %v", err)
	}
}

// Du JSON illisible revient en ERREUR, jamais en bibliothèque vide : une
// bibliothèque vide sans raison affichée est le pire des deux mondes.
func TestScanRefusesUnreadableInput(t *testing.T) {
	for _, entree := range []string{"", "{", "pas du json", "{\"name\":\"x\"}"} {
		if _, err := Scan(entree); err == nil {
			t.Errorf("Scan(%q) n'a pas rendu d'erreur", entree)
		}
	}
}

// Un type inconnu se DIT, avec son nom : c'est la seule façon de corriger une
// faute de frappe côté application sans lire le Go.
func TestScanNamesAnUnknownKind(t *testing.T) {
	entree, _ := json.Marshal([]SourceSpec{{Name: "drive", Kind: "ftp"}})
	_, err := Scan(string(entree))
	if err == nil {
		t.Fatal("un type inconnu doit rendre une erreur")
	}
	if !strings.Contains(err.Error(), "ftp") || !strings.Contains(err.Error(), "drive") {
		t.Fatalf("erreur = %q, elle doit nommer la source ET le type", err)
	}
}

// UN DOSSIER ABSENT N'EST PAS UNE PANNE, et ce test fige le contraire de ce
// que j'avais supposé en l'écrivant la première fois.
//
// Le dépôt le dit à trois endroits : `NewLocalSource` n'échoue que sur un nom
// vide, `LocalSource.List` porte « un dossier disparu n'est pas une panne de
// scan » avec le `os.IsNotExist` qui va avec, et `local_test.go` l'assert
// déjà. La raison est bonne : une bibliothèque doit survivre à un dossier
// renommé, sans quoi un seul montage cassé emporterait tout le scan.
//
// Ce qu'on vérifie ici, c'est que le PONT ne réintroduit pas la sévérité que
// la couche du dessous a écartée — et qu'il rend bien `[]`, pas `null`.
func TestScanSurvivesAMissingLocalRoot(t *testing.T) {
	entree, _ := json.Marshal([]SourceSpec{{Name: "local", Kind: "local", Root: "/nulle/part/vraiment"}})
	sortie, err := Scan(string(entree))
	if err != nil {
		t.Fatalf("un dossier absent ne doit pas casser le scan : %v", err)
	}
	if sortie != "[]" {
		t.Fatalf("sortie = %q, attendu \"[]\"", sortie)
	}
}

// Un nom de montage VIDE, en revanche, est une erreur de l'appelant : c'est le
// premier segment de tout chemin virtuel, et sans lui rien n'est adressable.
func TestScanRefusesAnEmptyMountName(t *testing.T) {
	entree, _ := json.Marshal([]SourceSpec{{Name: "", Kind: "local", Root: "/tmp"}})
	if _, err := Scan(string(entree)); err == nil {
		t.Fatal("un nom de montage vide doit rendre une erreur")
	}
}

// Consoles() sert à grouper la bibliothèque : elle doit rendre un tableau JSON
// non vide, sinon l'application n'a aucun nom de section à afficher.
func TestConsolesRendersTheRegistry(t *testing.T) {
	sortie, err := Consoles()
	if err != nil {
		t.Fatalf("consoles : %v", err)
	}

	var consoles []map[string]any
	if err := json.Unmarshal([]byte(sortie), &consoles); err != nil {
		t.Fatalf("sortie illisible : %v", err)
	}
	if len(consoles) == 0 {
		t.Fatal("le registre des consoles est vide")
	}
}
