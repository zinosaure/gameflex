package metadata

import (
	"path/filepath"
	"testing"

	"gameflex/storage/saves"
)

// savedPS5Game — un jeu PS5 déballé sous `root`, son identifiant dans `sce_sys/param.json`.
func savedPS5Game(t *testing.T, root, name, titleID string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	writeTestFile(t, filepath.Join(dir, "sce_sys", "param.json"), `{"titleId":"`+titleID+`"}`)

	return dir
}

// LA SAUVEGARDE PREND LE TITRE DU JEU QUI PORTE SON IDENTIFIANT : la bibliothèque d'abord, puis les disques ; sans
// jeu, elle garde le sien ; la Xbox (un disque pour toute la console) ne demande rien ; chaque console n'est demandée
// qu'une fois.
func TestNameSavesUsesTheGameOfTheLibrary(t *testing.T) {
	library, disk := t.TempDir(), t.TempDir()
	here := savedPS5Game(t, library, "Astro Bot", "PPSA12345")
	alsoOnDisk := savedPS5Game(t, disk, "ASTRO BOT (copie)", "PPSA12345")
	onDisk := savedPS5Game(t, disk, "Kena", "PPSA00042")
	untitled := savedPS5Game(t, disk, "Sans titre", "PPSA00077")
	asked := map[string]int{}
	gamesOf := func(consoleID string) []LibraryGame {
		asked[consoleID]++
		if consoleID != "ps5" {
			return nil
		}

		return []LibraryGame{
			{Path: here, RelPath: "ps5/Astro Bot", Reference: "ref-astro", Title: "Astro Bot"},
			{Path: alsoOnDisk, RelPath: "ps5/ASTRO BOT (copie)", Reference: "ref-astro-disk", Title: "ASTRO BOT (copie)", Disk: "Gamebox"},
			{Path: onDisk, RelPath: "ps5/Kena", Reference: "ref-kena", Title: "Kena", Disk: "Gamebox"},
			{Path: untitled, Reference: "ref-untitled"},
		}
	}

	tree := t.TempDir()
	// Une sauvegarde dont le jeu est parti, nommée par un listage d'avant.
	if err := saves.Remember(tree, saves.Named{Console: "ps5", Save: "PPSA99999",
		Game: saves.KnownGame{Title: "Returnal", ID: "ref-returnal", Path: "ps5/Returnal"}}); err != nil {
		t.Fatal(err)
	}
	named := NameSaves(tree, []saves.Game{
		{Console: "ps5", ID: "PPSA12345"},
		{Console: "ps5", ID: "PPSA00042", Title: "Kena: Bridge of Spirits"},
		{Console: "ps5", ID: "PPSA00077"},
		{Console: "ps5", ID: "PPSA99999"},
		{Console: "wiiu", ID: "10101e00", Title: "New SUPER MARIO BROS. U"},
		{Console: "xbox", ID: saves.XboxDisk},
	}, gamesOf)

	want := []saves.Game{
		{Console: "ps5", ID: "PPSA12345", Title: "Astro Bot", Reference: "ref-astro", Path: "ps5/Astro Bot"},
		{Console: "ps5", ID: "PPSA00042", Title: "Kena", Reference: "ref-kena", Disk: "Gamebox", Path: "ps5/Kena"},
		{Console: "ps5", ID: "PPSA00077"},
		// Le jeu est parti : son nom et son chemin restent, pas sa référence (sa fiche n'existe plus).
		{Console: "ps5", ID: "PPSA99999", Title: "Returnal", Path: "ps5/Returnal"},
		{Console: "wiiu", ID: "10101e00", Title: "New SUPER MARIO BROS. U"},
		{Console: "xbox", ID: saves.XboxDisk},
	}
	for index, game := range named {
		if game != want[index] {
			t.Errorf("%s/%s : %+v, attendu %+v", game.Console, game.ID, game, want[index])
		}
	}
	if asked["ps5"] != 1 || asked["wiiu"] != 1 || asked["xbox"] != 0 {
		t.Fatalf("consoles demandées : %v", asked)
	}
	// CE QUE LA BIBLIOTHÈQUE A NOMMÉ EST GARDÉ (chantier 76).
	if game, _ := saves.Known(tree)("ps5", "PPSA00042"); game != (saves.KnownGame{Title: "Kena", ID: "ref-kena", Path: "ps5/Kena"}) {
		t.Fatalf("titles.json : %+v", game)
	}
}
