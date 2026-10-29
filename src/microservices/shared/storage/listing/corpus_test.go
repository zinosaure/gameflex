package listing

import (
	"testing"
	"time"

	"gameflex/storage"
)

// S1 du banc d'essai, la moitié haute : ce qu'une ressource LISTE.
//
// Le corpus est un partage à la forme de GameFlex (bios/, cores/, roms/) :
// seuls ses jeux sont listés (#153), selon la convention — `roms/nes/USA/` est
// UN jeu (le dossier fait foi), et les seize .WAD de `bios/doom/` n'en sont
// AUCUN (signalé le 2026-09-02).
func TestCorpusListsExactlyTheReportedCounts(t *testing.T) {
	root := t.TempDir()
	if err := storage.WriteCorpus(root); err != nil {
		t.Fatalf("corpus : %v", err)
	}

	source, err := storage.NewLocalSource("cold", root)
	if err != nil {
		t.Fatalf("source : %v", err)
	}

	built, err := Build(source, time.Now())
	if err != nil {
		t.Fatalf("build : %v", err)
	}

	// doom = 0 jeu : une console vide n'hérite pas des .WAD de service, et
	// une ressource ne liste pas ses BIOS.
	if games := len(built.Roms["doom"]); games != 0 || len(built.Bios) != 0 || len(built.Cores) != 0 {
		t.Fatalf("doom a %d jeux, BIOS %+v, cœurs %+v", games, built.Bios, built.Cores)
	}

	// nes : les deux cartouches à plat, PLUS le dossier USA/ — un seul jeu,
	// nommé par le dossier.
	if games := len(built.Roms["nes"]); games != 3 {
		t.Fatalf("nes a %d jeux, attendu 3 (Mario, Zelda, USA) : %+v", games, built.Roms["nes"])
	}

	// ps1 : les deux jeux-dossiers, avec leur dossier — c'est lui qui dit au
	// transfert QUOI copier.
	if games := len(built.Roms["ps1"]); games != 2 {
		t.Fatalf("ps1 a %d jeux, attendu 2 : %+v", games, built.Roms["ps1"])
	}
	for _, game := range built.Roms["ps1"] {
		if game.Folder == "" || !game.Dir || game.ID == "" {
			t.Fatalf("un PS1 sans dossier ou sans identité : %+v", game)
		}
	}

	// psp et ps2 : un jeu-dossier chacun.
	if games := len(built.Roms["psp"]); games != 1 {
		t.Fatalf("psp a %d jeux, attendu 1", games)
	}
	if games := len(built.Roms["ps2"]); games != 1 {
		t.Fatalf("ps2 a %d jeux, attendu 1", games)
	}

	// nds : la ROM qui EST un zip, intacte comme entrée.
	if games := len(built.Roms["nds"]); games != 1 || built.Roms["nds"][0].Name != "Pokemon Platinum (Europe)" {
		t.Fatalf("nds = %+v, attendu la ROM zippée", built.Roms["nds"])
	}

	// snes : les deux jeux aux noms difficiles ; le .DS_Store et le dossier
	// de service n'existent nulle part.
	if games := len(built.Roms["snes"]); games != 2 {
		t.Fatalf("snes a %d jeux, attendu 2 : %+v", games, built.Roms["snes"])
	}

	// ps3 : le gros fichier, avec sa taille de dossier.
	if games := len(built.Roms["ps3"]); games != 1 || built.Roms["ps3"][0].Size != 2684354560 {
		t.Fatalf("ps3 = %+v, attendu le 2,5 Go", built.Roms["ps3"])
	}
}
