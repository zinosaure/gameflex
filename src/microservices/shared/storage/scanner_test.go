package storage

import (
	"errors"
	"testing"
)

// scanOf — le parcours d'une ressource distante : sa racine porte roms/.
func scanOf(t *testing.T, source Source) []Game {
	t.Helper()

	games, err := ScanConvention(source, "roms")
	if err != nil {
		t.Fatalf("scan : %v", err)
	}

	return games
}

func titlesOf(games []Game) map[string]Game {
	byTitle := map[string]Game{}
	for _, game := range games {
		byTitle[game.Title] = game
	}

	return byTitle
}

// LA CONVENTION, ET ELLE SEULE (chantier 15, le porteur, 2026-10-13) :
// roms/<consoleid>/<jeu>.<ext> ou roms/<consoleid>/<dossier>/… — l'identifiant
// du registre tel quel. Ni alias, ni majuscules, ni console rangée ailleurs.
func TestOnlyTheConventionIsRead(t *testing.T) {
	games := scanOf(t, newMemorySource("smb1",
		"roms/nes/Metroid.nes",
		"roms/psx/Crash.cue",            // un alias : ce n'est pas un identifiant
		"roms/PS2/Shadow.iso",           // en majuscules non plus
		"jeux/ps1/Metal Gear Solid.chd", // hors de roms/
		"roms/Metroid Prime.iso",        // à même roms/, sans console
		"roms/inconnue/x.bin",           // pas une console du registre
	))

	if len(games) != 1 || games[0].Title != "Metroid" || games[0].Console != "nes" {
		t.Fatalf("seul roms/nes/Metroid.nes suit la convention : %+v", games)
	}
	// Le chemin porte la source : c'est ce qui permet de le relire plus tard.
	if games[0].Path != "smb1/roms/nes/Metroid.nes" || games[0].Source != "smb1" ||
		len(games[0].Files) != 1 || games[0].Files[0] != "Metroid.nes" {
		t.Fatalf("jeu = %+v", games[0])
	}
}

// LE TÉLÉPHONE pose sa source sur le dossier des jeux même : la base est vide.
func TestASourceRootedOnTheGamesFolder(t *testing.T) {
	games, err := ScanConvention(newMemorySource("appareil", "nes/Metroid.nes", "roms/nes/Zelda.nes"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 || games[0].Path != "appareil/nes/Metroid.nes" {
		t.Fatalf("jeux = %+v", games)
	}
}

// UN FICHIER LIBRE N'EST UN JEU QUE SI SA CONSOLE L'ACCEPTE — la règle du
// catalogue du serveur, où 39 .mp3, .sav et .txt passaient pour des jeux
// (2026-09-18). Les archives passent partout : on ne les ouvre pas.
func TestALooseFileNeedsAnExtensionItsConsoleAccepts(t *testing.T) {
	byTitle := titlesOf(scanOf(t, newMemorySource("smb1",
		"roms/gbc/Pokemon Jaune.gbc",
		"roms/gbc/Pokemon Jaune.sav",
		"roms/gbc/notes.txt",
		"roms/snes/Chrono Trigger.zip",
	)))

	if len(byTitle) != 2 || byTitle["Pokemon Jaune"].Console != "gbc" || byTitle["Chrono Trigger"].Console != "snes" {
		t.Fatalf("jeux = %+v", byTitle)
	}
}

// Une console sans extensions déclarées (scummvm : un jeu est un dossier) ne
// doit pas devenir un refus global.
func TestConsoleWithoutExtensionsAcceptsEverything(t *testing.T) {
	games := scanOf(t, newMemorySource("smb1", "roms/scummvm/MONKEY.000"))

	if len(games) != 1 || games[0].Console != "scummvm" {
		t.Fatalf("l'absence de second signal ne doit pas refuser : %+v", games)
	}
}

func TestHiddenAndSystemJunkIsSkipped(t *testing.T) {
	games := scanOf(t, newMemorySource("smb1",
		"roms/gba/.DS_Store",
		"roms/gba/@eaDir/Zelda.gba",
		"roms/gba/System Volume Information/x.gba",
		"roms/.cache/gba/y.gba",
		"roms/gba/Zelda.gba",
	))

	if len(games) != 1 || games[0].Title != "Zelda" {
		t.Fatalf("seul Zelda devait rester : %+v", games)
	}
}

// UN DOSSIER SOUS UNE CONSOLE EST UN JEU, et porte son nom — quoi qu'il
// contienne, comme au serveur (décision du 2026-09-02). Ce qui se lance est le
// plus haut de sa racine : le .m3u d'un multi-disques avant ses .cue, un .cue
// avant ses .bin. Sa taille est celle de sa racine, ses fichiers aussi.
func TestAFolderUnderAConsoleIsOneGame(t *testing.T) {
	source := newMemorySource("smb1",
		"roms/ps1/[PS1] Ace Combat 2/Ace Combat 2.cue",
		"roms/ps1/[PS1] Ace Combat 2/Ace Combat 2.bin",
		"roms/ps1/[PS1] Final Fantasy IX/FF9.m3u",
		"roms/ps1/[PS1] Final Fantasy IX/FF9 (Disc 1).cue",
		"roms/ps1/[PS1] Final Fantasy IX/FF9 (Disc 1).bin",
		"roms/ps2/[PS2] Metal Slug 4/jeu.iso",
		"roms/ps2/[PS2] Metal Slug 4/extras/manuel.pdf",
	)
	byTitle := titlesOf(scanOf(t, source))

	if len(byTitle) != 3 {
		t.Fatalf("trois jeux-dossiers attendus : %+v", byTitle)
	}
	ace := byTitle["[PS1] Ace Combat 2"]
	if ace.Path != "smb1/roms/ps1/[PS1] Ace Combat 2/Ace Combat 2.cue" ||
		ace.Folder != "smb1/roms/ps1/[PS1] Ace Combat 2" || len(ace.Files) != 2 || ace.Size != 2048 {
		t.Fatalf("Ace Combat 2 = %+v", ace)
	}
	if got := byTitle["[PS1] Final Fantasy IX"].Path; got != "smb1/roms/ps1/[PS1] Final Fantasy IX/FF9.m3u" {
		t.Fatalf("le .m3u doit gagner : %q", got)
	}
	// Un sous-dossier fait partie du jeu ; seule la racine est listée.
	if slug := byTitle["[PS2] Metal Slug 4"]; len(slug.Files) != 1 || slug.Files[0] != "jeu.iso" {
		t.Fatalf("Metal Slug 4 = %+v", slug)
	}
}

// LE NOM D'UN DOSSIER N'A PAS D'EXTENSION À RETIRER : « Kingdom Hearts - HD
// 1.5 ReMIX » devenait « Kingdom Hearts - HD 1 » (2026-09-02).
func TestAFolderNameKeepsItsDots(t *testing.T) {
	byTitle := titlesOf(scanOf(t, newMemorySource("smb1",
		"roms/ps3/Marvel vs. Capcom 2 - New Age of Heroes (Europe)/game.iso",
		"roms/ps3/Kingdom Hearts - HD 1.5 ReMIX (Europe)/game.iso",
	)))

	if _, ok := byTitle["Marvel vs. Capcom 2 - New Age of Heroes (Europe)"]; !ok {
		t.Fatalf("les points du nom de dossier doivent survivre : %+v", byTitle)
	}
	if _, ok := byTitle["Kingdom Hearts - HD 1.5 ReMIX (Europe)"]; !ok {
		t.Fatalf("les points du nom de dossier doivent survivre : %+v", byTitle)
	}
}

// UN JEU-DOSSIER SANS RIEN À LANCER À SA RACINE (un jeu PS3 en dossier) reste
// un jeu : c'est le dossier qui se désigne.
func TestAFolderWithoutALaunchFileDesignatesItself(t *testing.T) {
	games := scanOf(t, newMemorySource("smb1", "roms/ps3/Uncharted/PS3_GAME/USRDIR/EBOOT.BIN"))

	if len(games) != 1 || games[0].Path != "smb1/roms/ps3/Uncharted" || games[0].Folder != games[0].Path {
		t.Fatalf("jeu = %+v", games)
	}
}

// UN JEU PS4 DÉBALLÉ se lance par son dossier, pas par son eboot.bin : le
// téléphone refusait ce fichier (« shadps4 ne lance pas un fichier .bin », S25,
// 2026-10-24). Celui qui garde son .pkg se lance par lui, et un verrou copié avec
// un déballé n'est pas un jeu.
func TestAnUnpackedPS4GameLaunchesByItsFolder(t *testing.T) {
	byTitle := titlesOf(scanOf(t, newMemorySource("usb",
		"roms/ps4/Kingdom Hearts III/eboot.bin",
		"roms/ps4/Kingdom Hearts III/sce_sys/param.sfo",
		"roms/ps4/Sonic Mania Plus/Sonic.Mania.Plus.pkg",
		"roms/ps4/Kingdom Hearts III.lock",
	)))

	if len(byTitle) != 2 {
		t.Fatalf("deux jeux attendus, le verrou n'en est pas un : %+v", byTitle)
	}
	if got := byTitle["Kingdom Hearts III"].Path; got != "usb/roms/ps4/Kingdom Hearts III" {
		t.Fatalf("le déballé doit se lancer par son dossier : %q", got)
	}
	if got := byTitle["Sonic Mania Plus"].Path; got != "usb/roms/ps4/Sonic Mania Plus/Sonic.Mania.Plus.pkg" {
		t.Fatalf("le .pkg doit rester ce qui se lance : %q", got)
	}
}

type refusingSource struct {
	Source
	refused map[string]bool
}

func (s refusingSource) List(dir string) ([]Entry, error) {
	if s.refused[trimPath(dir)] {
		return nil, errors.New("accès refusé")
	}

	return s.Source.List(dir)
}

// Un dossier de console illisible ne doit pas emporter le reste de la source
// (2026-09-02) ; la RACINE, elle, est une vraie panne : ne pas l'atteindre,
// c'est ne pas avoir la source du tout.
func TestAnUnreadableConsoleIsSkippedButNotTheRoot(t *testing.T) {
	inner := newMemorySource("drive", "roms/nes/Metroid.nes", "roms/gba/Zelda.gba")
	games, err := ScanConvention(refusingSource{Source: inner, refused: map[string]bool{"roms/gba": true}}, "roms")
	if err != nil || len(games) != 1 || games[0].Title != "Metroid" {
		t.Fatalf("le jeu lisible a disparu avec le dossier illisible : %+v (%v)", games, err)
	}

	if _, err := ScanConvention(refusingSource{Source: inner, refused: map[string]bool{"roms": true}}, "roms"); err == nil {
		t.Fatal("une racine illisible doit remonter une erreur")
	}
}

// Trié par console, puis par titre sans tenir compte de la casse.
func TestGamesAreSortedByConsoleThenTitle(t *testing.T) {
	games := scanOf(t, newMemorySource("smb1",
		"roms/snes/zelda.sfc", "roms/nes/Metroid.nes", "roms/snes/Aladdin.sfc",
	))

	if len(games) != 3 || games[0].Console != "nes" || games[1].Title != "Aladdin" || games[2].Title != "zelda" {
		t.Fatalf("ordre = %+v", games)
	}
}
