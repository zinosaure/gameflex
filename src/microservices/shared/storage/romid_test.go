package storage

import "testing"

// L'IDENTITÉ EST CELLE DU SERVEUR, au caractère près : ces valeurs sont celles
// que `retro_files.deriveRomID` du kernel donne pour les mêmes entrées. Les
// changer, c'est perdre le lien entre un jeu, ses sauvegardes et son historique.
func TestRomIDIsTheServersIdentity(t *testing.T) {
	for _, test := range []struct{ console, entry, want string }{
		{"nes", "Metroid.nes", "4f9523a0-83b6-5141-a5a1-63a2d9b490e9"},
		{"ps1", "[PS1] Ace Combat 2 [F] [SCES-00901]", "c0757d8e-9e41-5234-9f34-55a341e41e8b"},
	} {
		if got := RomID(test.console, test.entry); got != test.want {
			t.Errorf("RomID(%s, %s) = %s", test.console, test.entry, got)
		}
		if !IsRomID(test.want) {
			t.Errorf("IsRomID(%s) = faux", test.want)
		}
	}
	if RomID("nes", "Metroid.nes") == RomID("nes", "Metroid.NES") {
		t.Error("un nom écrit autrement doit être un autre jeu")
	}
	if RomID("nes", "Metroid.nes") == RomID("snes", "Metroid.nes") {
		t.Error("la console fait partie de l'identité")
	}
	for _, other := range []string{"", "keytest", "srv3:4f9523a0-83b6-5141-a5a1-63a2d9b490e9", "appareil/nes/Metroid.nes",
		"{4f9523a0-83b6-5141-a5a1-63a2d9b490e9}", "urn:uuid:4f9523a0-83b6-5141-a5a1-63a2d9b490e9"} {
		if IsRomID(other) {
			t.Errorf("IsRomID(%q) = vrai", other)
		}
	}
}

// L'ENTRÉE D'UN JEU SCANNÉ : le fichier libre, ou le dossier d'un jeu-dossier
// — jamais le fichier qui le lance —, sous le dossier de SA console, où qu'il
// soit dans le chemin.
func TestGameEntryIsTheEntryUnderTheConsole(t *testing.T) {
	for _, test := range []struct {
		game Game
		want string
	}{
		{Game{Console: "nes", Path: "appareil/nes/Metroid.nes"}, "Metroid.nes"},
		{Game{Console: "ps1", Path: "appareil/ps1/[PS1] Ace Combat 2/Ace Combat 2.cue", Folder: "appareil/ps1/[PS1] Ace Combat 2"}, "[PS1] Ace Combat 2"},
		// Sans son `Folder` (une référence d'autrefois) : le même dossier.
		{Game{Console: "ps1", Path: "appareil/ps1/[PS1] Ace Combat 2/Ace Combat 2.cue"}, "[PS1] Ace Combat 2"},
		{Game{Console: "gba", Path: "appareil/gba/007/sous/007.gba"}, "007"},
		// Une ressource distante : sa racine porte roms/.
		{Game{Console: "nes", Path: "kdrive/roms/nes/Metroid.nes"}, "Metroid.nes"},
	} {
		if got := GameEntry(test.game); got != test.want {
			t.Errorf("GameEntry(%s) = %q, attendu %q", test.game.Path, got, test.want)
		}
	}
	if GameID(Game{Console: "nes", Path: "appareil/nes/Metroid.nes"}) != "4f9523a0-83b6-5141-a5a1-63a2d9b490e9" {
		t.Error("le Metroid du téléphone n'est pas celui du serveur")
	}
}
