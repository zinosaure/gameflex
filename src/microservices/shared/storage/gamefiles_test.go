package storage

import (
	"strings"
	"testing"
)

// Le cas qui a coûté le plus cher : un PS1 est un .cue et ses .bin, et
// n'emporter que le .cue donne deux kilo-octets qui ne démarrent rien.
func TestAFolderGameBringsAllItsFiles(t *testing.T) {
	source := newMemorySource("kDrive",
		"roms/ps1/[PS1] Ace Combat 2/Ace Combat 2.cue",
		"roms/ps1/[PS1] Ace Combat 2/Ace Combat 2 (Track 1).bin",
		"roms/ps1/[PS1] Ace Combat 2/Ace Combat 2 (Track 2).bin",
		"roms/ps1/[PS1] Crash/Crash.cue",
		"roms/ps1/[PS1] Crash/Crash.bin",
	)
	games, err := ScanConvention(source, "roms")
	if err != nil {
		t.Fatalf("scan : %v", err)
	}

	var ace Game
	for _, game := range games {
		if strings.Contains(game.Title, "Ace Combat") {
			ace = game
		}
	}
	if ace.Folder != "kDrive/roms/ps1/[PS1] Ace Combat 2" {
		t.Fatalf("le scanner n'a pas dit que ce jeu est son dossier : %+v", ace)
	}

	files, err := GameFiles(source, ace.Path, ace.Folder)
	if err != nil {
		t.Fatalf("GameFiles : %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("3 fichiers attendus (le .cue et ses deux .bin), %d : %+v", len(files), files)
	}
	// Et RIEN du jeu d'à côté : un dossier est un jeu, pas la console.
	for _, file := range files {
		if !strings.Contains(file.Path, "Ace Combat") {
			t.Fatalf("un fichier étranger au jeu : %q", file.Path)
		}
	}
}

// Une cartouche posée à même le dossier de sa console reste UN fichier : on
// ne doit surtout pas emporter les cent voisines.
func TestALooseGameBringsOnlyItself(t *testing.T) {
	source := newMemorySource("kDrive",
		"roms/nes/Mario.nes",
		"roms/nes/Zelda.nes",
		"roms/nes/Metroid.nes",
	)
	games, _ := ScanConvention(source, "roms")

	var mario Game
	for _, game := range games {
		if game.Title == "Mario" {
			mario = game
		}
	}
	if mario.Folder != "" {
		t.Fatalf("une cartouche seule n'est pas un dossier-jeu : %+v", mario)
	}

	files, err := GameFiles(source, mario.Path, mario.Folder)
	if err != nil {
		t.Fatalf("GameFiles : %v", err)
	}
	if len(files) != 1 || files[0].Name != "Mario.nes" {
		t.Fatalf("un seul fichier attendu, %+v", files)
	}
	// La taille vient du listing : sans elle, pas de barre de progression et
	// rien à comparer une fois la copie finie.
	if files[0].Size == 0 {
		t.Fatalf("taille perdue : %+v", files[0])
	}
}

// Un jeu rangé en sous-dossiers doit arriver AVEC ses sous-dossiers — c'est
// « Name » qui les porte, et c'est lui qui dit quoi créer à l'arrivée.
func TestSubFoldersSurviveTheCopy(t *testing.T) {
	source := newMemorySource("kDrive",
		"roms/psp/[PSP] Tekken 6/Tekken 6.iso",
		"roms/psp/[PSP] Tekken 6/media/cover.png",
	)
	games, _ := ScanConvention(source, "roms")

	files, err := GameFiles(source, games[0].Path, games[0].Folder)
	if err != nil {
		t.Fatalf("GameFiles : %v", err)
	}

	byName := map[string]bool{}
	for _, file := range files {
		byName[file.Name] = true
	}
	if !byName["Tekken 6.iso"] || !byName["media/cover.png"] {
		t.Fatalf("l'arborescence n'a pas survécu : %+v", files)
	}
}

// Un chemin qui désigne une AUTRE source ne doit rien rendre du tout.
func TestGameFilesStaysInsideItsSource(t *testing.T) {
	source := newMemorySource("kDrive", "roms/nes/Mario.nes")

	if _, err := GameFiles(source, "ailleurs/roms/nes/x.nes", "ailleurs/roms/nes"); err != ErrOutsideSource {
		t.Fatalf("erreur = %v, attendu ErrOutsideSource", err)
	}
}

// La garde qui compte : le dossier d'une console n'est le dossier d'AUCUN
// jeu, même quand il n'en contient qu'un. Sans elle, demander cette unique
// cartouche ferait descendre la console entière.
func TestAConsoleFolderIsNeverAGameFolder(t *testing.T) {
	source := newMemorySource("kDrive", "roms/nes/Mario.nes")

	folder, err := GameFolderOf(source, "nes", "kDrive/roms/nes/Mario.nes")
	if err != nil {
		t.Fatalf("GameFolderOf : %v", err)
	}
	if folder != "" {
		t.Fatalf("dossier = %q, attendu aucun", folder)
	}
}

// LE DOSSIER FAIT FOI, quoi qu'il contienne.
//
// La règle ne regarde que le chemin : sous roms/{console}/{dossier}/, le
// dossier EST le jeu et lui donne son nom — même s'il abrite plusieurs
// fichiers jouables (un jeu et son contenu additionnel, par exemple).
//
// Décision du 2026-09-02, et le catalogue applique exactement la même : c'est
// ce qui les rend d'accord. La version précédente listait le dossier pour
// compter, ce qui coûtait onze requêtes paginées avant le premier octet.
func TestAGroupingFolderIsTakenWhole(t *testing.T) {
	source := newMemorySource("kDrive",
		"roms/nes/USA/Mario.nes",
		"roms/nes/USA/Zelda.nes",
	)

	folder, err := GameFolderOf(source, "nes", "kDrive/roms/nes/USA/Mario.nes")
	if err != nil {
		t.Fatalf("GameFolderOf : %v", err)
	}
	if folder != "kDrive/roms/nes/USA" {
		t.Fatalf("dossier = %q", folder)
	}
}

// Au-delà d'un niveau, on REFUSE plutôt que de deviner : emporter le mauvais
// dossier est pire que ne rien emporter.
func TestATooDeepGameIsRejected(t *testing.T) {
	source := newMemorySource("kDrive", "roms/nes/USA/hacks/Mario.nes")

	if _, err := GameFolderOf(source, "nes", "kDrive/roms/nes/USA/hacks/Mario.nes"); err != ErrUnexpectedTree {
		t.Fatalf("erreur = %v, attendu ErrUnexpectedTree", err)
	}
}

// Et le cas utile : le dossier d'un jeu multi-fichiers est reconnu sans
// qu'on ait à croire ce que l'appelant raconte.
func TestGameFolderIsRecognisedFromTheSourceItself(t *testing.T) {
	source := newMemorySource("kDrive",
		"roms/ps1/[PS1] Ace Combat 2/Ace Combat 2.cue",
		"roms/ps1/[PS1] Ace Combat 2/Ace Combat 2.bin",
	)

	folder, err := GameFolderOf(source, "ps1", "kDrive/roms/ps1/[PS1] Ace Combat 2/Ace Combat 2.cue")
	if err != nil {
		t.Fatalf("GameFolderOf : %v", err)
	}
	if folder != "kDrive/roms/ps1/[PS1] Ace Combat 2" {
		t.Fatalf("dossier = %q", folder)
	}
}

// Le nom de fichier d'un jeu rapatrié : dérivé du titre, refusé s'il n'est pas
// un nom simple, et complété par l'extension du seul fichier annoncé — sans
// quoi un jeu arrive nu et aucun cœur ne le charge (leçon du 2026-09-02).
func TestRomFileNameDerivesAndRefuses(t *testing.T) {
	cases := []struct {
		titre   string
		files   []string
		attendu string
		erreur  bool
	}{
		{"GBA Serial Slave V0.96", []string{"GBA Serial Slave V0.96.zip"},
			"GBA Serial Slave V0.96.zip", false},
		{"Mario", []string{"Mario.nes"}, "Mario.nes", false},
		{"Mario.nes", []string{"Mario.nes"}, "Mario.nes", false},
		{"Ace Combat 2", []string{"a.cue", "a.bin"}, "Ace Combat 2", false},
		{"Sans fichiers", nil, "Sans fichiers", false},
		{"../../databases", []string{"x.nes"}, "", true},
		{"dossier/jeu", []string{"x.nes"}, "", true},
		{"", []string{"x.nes"}, "", true},
		{"..", []string{"x.nes"}, "", true},
	}

	for _, test := range cases {
		got, err := RomFileName(test.titre, test.files)
		if test.erreur {
			if err == nil {
				t.Errorf("RomFileName(%q) = %q, attendu une erreur", test.titre, got)
			}

			continue
		}
		if err != nil {
			t.Errorf("RomFileName(%q) = erreur %v, attendu %q", test.titre, err, test.attendu)

			continue
		}
		if got != test.attendu {
			t.Errorf("RomFileName(%q, %v) = %q, attendu %q",
				test.titre, test.files, got, test.attendu)
		}
	}
}
