package consoles

import (
	"reflect"
	"testing"
)

// La règle de la page (`missingRequiredBios`), pour les deux kernels : un
// requis suffit, sans la casse ; un facultatif n'arrête rien (#393).
func TestMissingRequiredBios(t *testing.T) {
	cases := []struct {
		name    string
		console string
		present []string
		want    []string
	}{
		{"aucun requis présent", "saturn", nil, BiosRequiredFor("saturn")},
		{"un seul des requis suffit", "saturn", []string{"MPR-17933.BIN"}, nil},
		{"un facultatif n'est pas un requis", "saturn", []string{"autre.bin"}, BiosRequiredFor("saturn")},
		{"des facultatifs seulement : rien n'arrête", "ps1", nil, nil},
		{"une console sans BIOS", "nes", nil, nil},
		{"une console inconnue", "pas-une-console", nil, nil},
		{"les clés de la Switch", "switch", []string{"title.keys"}, []string{"prod.keys"}},
	}
	for _, c := range cases {
		if got := MissingRequiredBios(c.console, c.present); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s : MissingRequiredBios(%q, %v) = %v, attendu %v", c.name, c.console, c.present, got, c.want)
		}
	}
}
