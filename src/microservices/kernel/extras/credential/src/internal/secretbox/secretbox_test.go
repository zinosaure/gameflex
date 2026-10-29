package secretbox

import (
	"strings"
	"testing"
)

// UN IDENTIFIANT SCELLÉ NE SE LIT QU'AVEC LE MÊME SECRET, et rien de lui
// n'apparaît en clair dans ce qui est rangé.
func TestASealedCredentialOpensOnlyWithItsSecret(t *testing.T) {
	stored, err := Seal("le-secret-de-gameflex", "mot de passe du partage")
	if err != nil {
		t.Fatal(err)
	}
	if !Sealed(stored) || strings.Contains(stored, "partage") {
		t.Fatalf("rangé : %q", stored)
	}
	if plain, err := Open("le-secret-de-gameflex", stored); err != nil || plain != "mot de passe du partage" {
		t.Fatalf("ouvert : %q %v", plain, err)
	}
	if _, err := Open("un-autre-secret", stored); err == nil {
		t.Fatal("un autre secret ouvre l'identifiant")
	}
	if _, err := Open("le-secret-de-gameflex", "en clair"); err == nil {
		t.Fatal("une valeur non scellée est lue")
	}
	if _, err := Seal("", "x"); err == nil {
		t.Fatal("sceller sans secret doit échouer")
	}
	if stored, err := Seal("", ""); err != nil || stored != "" {
		t.Fatalf("un mot de passe vide (partage invité) : %q %v", stored, err)
	}
}
