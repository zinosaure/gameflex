package storage

import "testing"

// Le dossier fait partie de l'adresse pour WebDAV, et cette composition était
// recopiée (mal) à trois endroits. Les cas limites sont ceux qui l'ont fait
// diverger : des barres obliques en trop d'un côté, du blanc autour, un
// dossier vide qui doit rendre l'adresse INCHANGÉE.
func TestShareRoot(t *testing.T) {
	cas := []struct {
		nom    string
		base   string
		folder string
		veut   string
	}{
		{"dossier vide rend l'adresse telle quelle", "https://dav.test/partage", "", "https://dav.test/partage"},
		{"du blanc seul est un dossier vide", "https://dav.test/partage", "   ", "https://dav.test/partage"},
		{"un dossier simple s'ajoute", "https://dav.test/partage", "roms", "https://dav.test/partage/roms"},
		{"pas de barre en double", "https://dav.test/partage/", "roms", "https://dav.test/partage/roms"},
		{"les barres du dossier sont trimées", "https://dav.test/partage", "/roms/", "https://dav.test/partage/roms"},
		{"un chemin à plusieurs segments survit", "https://dav.test/p", "jeux/roms", "https://dav.test/p/jeux/roms"},
		{"le blanc autour du dossier ne compte pas", "https://dav.test/p", "  roms  ", "https://dav.test/p/roms"},
	}

	for _, c := range cas {
		if got := ShareRoot(c.base, c.folder); got != c.veut {
			t.Errorf("%s : ShareRoot(%q, %q) = %q, attendu %q", c.nom, c.base, c.folder, got, c.veut)
		}
	}
}
