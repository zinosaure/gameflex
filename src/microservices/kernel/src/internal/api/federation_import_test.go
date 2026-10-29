package api

import "testing"

// CE QUE L'ON FAIT D'UNE ARCHIVE — la règle que j'avais écrite à l'envers.
//
// Premier jet : décider sur la signature du flux (`PK\x03\x04`). Faux pour un
// jeu, et mesuré sur la vraie bibliothèque le 2026-09-17 : `roms/gba` est à
// 1666/1666 en `.zip`, `roms/nds` à 7635/7635 — un `.zip` EST la forme normale
// d'une ROM, que les cœurs libretro lisent telle quelle. La signature ne
// distingue donc pas « ce jeu est une archive » de « ce dossier a été zippé à
// la volée » : les deux commencent pareil. Seul le nombre de fichiers annoncé
// par le catalogue tranche.
//
// Les trois formes existent réellement côte à côte dans la même bibliothèque :
// fichier nu (`roms/nes`, 10785 éléments, aucun zip), `.zip` d'un seul fichier
// (gba, nds, megadrive) et dossier (`roms/ps1`, 29 éléments, tous des dossiers).
func TestZipPolicyForCoversTheThreeRealShapes(t *testing.T) {
	cases := []struct {
		nom     string
		section string
		name    string
		files   []string
		folder  bool
		attendu zipPolicy
	}{
		{"un cœur refuse une archive", "cores", "fceumm", nil, false, zipRefuse},
		{"un cœur refuse même avec des fichiers", "cores", "fceumm", []string{"a", "b"}, false, zipRefuse},
		{"une ROM d'un seul fichier garde son .zip", "roms", "jeu.zip", []string{"jeu.zip"}, false, zipKeep},
		{"une ROM sans liste annoncée garde aussi", "roms", "jeu.zip", nil, false, zipKeep},
		{"une ROM multi-fichiers se déplie", "roms", "jeu", []string{"jeu.cue", "jeu.bin"}, false, zipExpand},
		// UN DOSSIER D'UN SEUL FICHIER EST UN DOSSIER. Mesuré le 2026-10-10 :
		// `roms/psp` compte 26 dossiers, et les 26 n'ont qu'un `.iso` à leur
		// racine (11 sur 13 en PS2). Le pair les sert zippés ; compter les
		// fichiers les prenait pour des ROMs nues, et l'archive était rangée
		// sous le nom de l'`.iso`.
		{"un jeu PSP en dossier d'un seul .iso se déplie", "roms", "[PSP] Crisis Core", []string{"[PSP] Crisis Core.iso"}, true, zipExpand},
		// LE BIOS DE LA NEO-GEO EST UNE ARCHIVE, et le registre l'attend ainsi
		// (`neogeo.zip`). Le déplier donnerait un dossier que le cœur ne
		// trouverait pas — même famille d'erreur que pour les ROMs zippées.
		{"un BIOS déclaré en .zip se garde", "bios", "neogeo.zip", nil, false, zipKeep},
		{"un BIOS déclaré en .ZIP aussi", "bios", "NEOGEO.ZIP", nil, false, zipKeep},
		{"un dump de BIOS en dossier se déplie", "bios", "scph10000", nil, false, zipExpand},
		{"un BIOS d'un seul fichier .bin se déplie (rien à déplier)", "bios", "dc_boot.bin", nil, false, zipExpand},
	}

	for _, test := range cases {
		if got := zipPolicyFor(test.section, test.name, test.files, test.folder); got != test.attendu {
			t.Errorf("%s : zipPolicyFor(%q, %q, %v, %v) = %d, attendu %d",
				test.nom, test.section, test.name, test.files, test.folder, got, test.attendu)
		}
	}
}
