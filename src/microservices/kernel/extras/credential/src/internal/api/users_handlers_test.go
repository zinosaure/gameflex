package api

import (
	"strings"
	"testing"
)

// UN NOM DE COMPTE DEVIENT UN DOSSIER (saves/<nom>) : ni `/`, ni `..`, et le
// motif est celui que le kernel accepte pour lancer une partie (#601).
func TestAUsernameIsAFolderName(t *testing.T) {
	for _, name := range []string{"gino", "claude", "marc.dupont", "a_b-c", "abc", strings.Repeat("a", 32)} {
		if !usernamePattern.MatchString(name) {
			t.Errorf("%q refusé", name)
		}
	}
	for _, name := range []string{"..", "...a/", "../bob", "a/b", `a\b`, "ab", "Gino D", "élodie", strings.Repeat("a", 33), ""} {
		if usernamePattern.MatchString(name) {
			t.Errorf("%q accepté", name)
		}
	}
}
