package retro_files

import (
	"os"
	"path/filepath"
	"testing"
)

// Les catalogues vivent au niveau du paquet, un par racine : chaque test part
// d'un état neuf, sinon le premier scan d'un test décide de ce que voit le
// suivant.
func resetCatalog() {
	catalogsMu.Lock()
	catalogs = map[string]*catalogState{}
	catalogsMu.Unlock()
}

func writeGameFolder(t *testing.T, root, consoleID, title, file string) {
	t.Helper()
	dir := filepath.Join(root, consoleID, title)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
}

// Le vrai bug du 2026-09-10 : le catalogue n'était scanné qu'une fois par vie
// du processus, donc un jeu déposé après le démarrage n'apparaissait jamais
// dans la liste — alors que le compteur de la console, lui, le voyait.
// LE COMPTEUR D'UNE CONSOLE EST LA LONGUEUR DE SA LISTE (#107). Un fichier
// libre que la console refuse (`notes.txt` pour gbc) n'est pas un jeu : la
// liste l'écarte, et le compteur ne doit pas l'annoncer. L'ancien compteur
// relisait le disque et comptait tout — mesuré sur la pile de dev : 521 jeux
// gbc annoncés pour 494 listés.
func TestTheConsoleCountIsTheLengthOfItsList(t *testing.T) {
	resetCatalog()
	root := t.TempDir()
	consoleDir := filepath.Join(root, "gbc")
	if err := os.MkdirAll(consoleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Tetris.gbc", "Pokemon Crystal.gbc", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(consoleDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	listed, err := ListCatalog(root, "gbc", "")
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}

	counted := -1
	for _, summary := range CountCatalog(root) {
		if summary.ID == "gbc" {
			counted = summary.Count
		}
	}
	if len(listed) != 2 || counted != len(listed) {
		t.Fatalf("compteur gbc = %d pour %d jeu(x) listé(s), attendu 2 et 2", counted, len(listed))
	}
}

func TestAGameAddedAfterTheFirstScanShowsUp(t *testing.T) {
	resetCatalog()
	root := t.TempDir()
	writeGameFolder(t, root, "ps5", "Premier jeu", "eboot.bin")

	first, err := ListCatalog(root, "ps5", "")
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("attendu 1 jeu au premier scan, obtenu %d", len(first))
	}

	writeGameFolder(t, root, "ps5", "Deuxième jeu", "eboot.bin")

	second, err := ListCatalog(root, "ps5", "")
	if err != nil {
		t.Fatalf("ListCatalog après ajout: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("le jeu ajouté après le premier scan reste invisible : %d jeu(x)", len(second))
	}
}

// Une console entière qui apparaît compte aussi : c'est la racine des roms
// qui change, pas un dossier de console déjà connu.
func TestAConsoleAddedAfterTheFirstScanShowsUp(t *testing.T) {
	resetCatalog()
	root := t.TempDir()
	writeGameFolder(t, root, "ps4", "Un jeu ps4", "game.pkg")

	if _, err := ListCatalog(root, "ps4", ""); err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if games, _ := ListCatalog(root, "ps5", ""); len(games) != 0 {
		t.Fatalf("ps5 ne devrait rien avoir encore, obtenu %d", len(games))
	}

	writeGameFolder(t, root, "ps5", "Richman 11", "eboot.bin")

	games, err := ListCatalog(root, "ps5", "")
	if err != nil {
		t.Fatalf("ListCatalog ps5: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("la console apparue après le premier scan reste vide : %d jeu(x)", len(games))
	}
}

// Un fichier ajouté DANS un jeu-dossier ne change que la date de ce dossier —
// ni celle de la console, ni celle de la racine. C'est pour ça que chaque
// jeu-dossier est relevé séparément.
func TestAFileAddedInsideAGameFolderIsSeen(t *testing.T) {
	resetCatalog()
	root := t.TempDir()
	writeGameFolder(t, root, "ps5", "Richman 11", "eboot.bin")

	first, err := ListCatalog(root, "ps5", "")
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(first[0].Files) != 1 {
		t.Fatalf("attendu 1 fichier, obtenu %d", len(first[0].Files))
	}

	extra := filepath.Join(root, "ps5", "Richman 11", "param.json")
	if err := os.WriteFile(extra, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	second, err := ListCatalog(root, "ps5", "")
	if err != nil {
		t.Fatalf("ListCatalog après ajout: %v", err)
	}
	if len(second[0].Files) != 2 {
		t.Fatalf("le fichier ajouté dans le jeu reste invisible : %v", second[0].Files)
	}
}

// Rien n'a bougé : le catalogue ne doit PAS se reconstruire (c'est tout
// l'intérêt du cache — le rendre à nouveau exact ne doit pas le rendre
// inutile).
func TestAnUnchangedLibraryIsNotRescanned(t *testing.T) {
	resetCatalog()
	root := t.TempDir()
	writeGameFolder(t, root, "ps5", "Richman 11", "eboot.bin")

	if _, err := ListCatalog(root, "ps5", ""); err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	catalog := catalogFor(root)
	catalog.mu.RLock()
	firstScan := catalog.lastScan
	catalog.mu.RUnlock()

	if _, err := ListCatalog(root, "ps5", ""); err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	catalog.mu.RLock()
	secondScan := catalog.lastScan
	catalog.mu.RUnlock()

	if !secondScan.Equal(firstScan) {
		t.Fatalf("bibliothèque inchangée et pourtant rescannée (%v → %v)", firstScan, secondScan)
	}
}

// La référence d'un jeu ne doit pas changer d'un scan à l'autre : les favoris
// et l'historique la stockent (deriveRomID est déterministe — ce test est là
// pour que le rescan automatique ne le remette pas en cause par accident).
func TestReferencesSurviveARescan(t *testing.T) {
	resetCatalog()
	root := t.TempDir()
	writeGameFolder(t, root, "ps5", "Richman 11", "eboot.bin")

	before, err := ListCatalog(root, "ps5", "")
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	writeGameFolder(t, root, "ps5", "Un autre jeu", "eboot.bin")
	after, err := ListCatalog(root, "ps5", "")
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}

	var found string
	for _, game := range after {
		if game.Title == before[0].Title {
			found = game.Reference
		}
	}
	if found != before[0].Reference {
		t.Fatalf("la référence a changé au rescan : %q → %q", before[0].Reference, found)
	}
}

// Un jeu ARBORESCENT (le cas PS5) : la taille annoncée doit être celle du jeu
// entier, pas seulement de sa racine. Avant, la bibliothèque affichait 30 Mo
// pour un jeu de 3,2 Go.
func TestAGameSizeCountsItsSubfolders(t *testing.T) {
	resetCatalog()
	root := t.TempDir()
	game := filepath.Join(root, "ps5", "Richman 11")
	if err := os.MkdirAll(filepath.Join(game, "Media", "Modules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(game, "eboot.bin"), make([]byte, 100), 0o644); err != nil {
		t.Fatalf("write eboot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(game, "Media", "level0"), make([]byte, 4000), 0o644); err != nil {
		t.Fatalf("write level0: %v", err)
	}
	if err := os.WriteFile(filepath.Join(game, "Media", "Modules", "PS5Util.prx"), make([]byte, 900), 0o644); err != nil {
		t.Fatalf("write prx: %v", err)
	}

	games, err := ListCatalog(root, "ps5", "")
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("attendu 1 jeu, obtenu %d", len(games))
	}
	if games[0].SizeBytes != 5000 {
		t.Fatalf("taille attendue 5000 (racine + sous-dossiers), obtenue %d", games[0].SizeBytes)
	}
	// Files reste la racine : c'est ce qu'on voit en ouvrant le dossier.
	if len(games[0].Files) != 1 || games[0].Files[0] != "eboot.bin" {
		t.Fatalf("Files devrait ne lister que la racine, obtenu %v", games[0].Files)
	}
}

// LA MÊME RÉFÉRENCE PARTOUT — ou un jeu listé ne se lance pas.
//
// Le kernel donne cette référence au navigateur ; le runtime rétro la reçoit au
// lancement et la retrouve avec SA copie de la dérivation
// (runtimes/retrogaming, catalog.go), et internal/catalog l'accepte aussi comme
// identifiant. Trois copies d'un calcul qui doivent rendre la même chaîne, dans
// trois paquets qui ne peuvent pas s'importer : la valeur est figée à
// l'identique dans chacun, pour qu'une dérive casse le test du paquet qui a
// dérivé plutôt que chaque lancement. L'EXTENSION COMPTE : depuis le
// 2026-09-18 la dérivation porte sur le nom SUR LE DISQUE, donc sur
// « Super Mario Bros..nes » et non sur le titre « Super Mario Bros. ».
func TestReferencesMatchTheSharedGoldenValue(t *testing.T) {
	const golden = "a16f8b20-e5e2-5991-a712-a4d77ef85a7e"
	if got := deriveRomID("nes", "Super Mario Bros..nes"); got != golden {
		t.Fatalf("deriveRomID(nes, Super Mario Bros..nes) = %s, want %s", got, golden)
	}

	resetCatalog()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nes", "Super Mario Bros..nes"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	roms, err := ListCatalog(root, "nes", "")
	if err != nil || len(roms) != 1 || roms[0].Reference != golden {
		t.Errorf("ListCatalog(nes) = %+v, %v — want une seule entrée de référence %s", roms, err, golden)
	}
}

// DEUX RACINES, DEUX CATALOGUES (chantier 23). La bibliothèque principale et un
// disque interne se lisent l'un après l'autre : chacune garde ses jeux, la
// même référence se résout dans chacune vers SON fichier, et un jeu ajouté sur
// l'une n'apparaît pas dans l'autre.
func TestEachRootKeepsItsOwnCatalog(t *testing.T) {
	resetCatalog()
	local, disk := t.TempDir(), t.TempDir()
	writeGameFolder(t, local, "ps5", "Richman 11", "eboot.bin")
	writeGameFolder(t, disk, "ps5", "Richman 11", "eboot.bin")
	writeGameFolder(t, disk, "ps5", "Astro Bot", "eboot.bin")

	localGames, _ := ListCatalog(local, "ps5", "")
	diskGames, _ := ListCatalog(disk, "ps5", "")
	if len(localGames) != 1 || len(diskGames) != 2 {
		t.Fatalf("jeux : local %d (1 attendu), disque %d (2 attendus)", len(localGames), len(diskGames))
	}

	reference := localGames[0].Reference
	localPath, _, err := ResolveCatalogRom(local, "ps5", reference)
	if err != nil {
		t.Fatalf("résolution locale : %v", err)
	}
	diskPath, _, err := ResolveCatalogRom(disk, "ps5", reference)
	if err != nil {
		t.Fatalf("résolution sur le disque : %v", err)
	}
	if localPath != filepath.Join(local, "ps5", "Richman 11") || diskPath != filepath.Join(disk, "ps5", "Richman 11") {
		t.Fatalf("chaque racine doit rendre SON dossier : %s, %s", localPath, diskPath)
	}

	writeGameFolder(t, disk, "ps5", "Demon's Souls", "eboot.bin")
	if games, _ := ListCatalog(local, "ps5", ""); len(games) != 1 {
		t.Fatalf("un jeu ajouté sur le disque apparaît dans le local : %d jeux", len(games))
	}
	if games, _ := ListCatalog(disk, "ps5", ""); len(games) != 3 {
		t.Fatalf("le jeu ajouté sur le disque n'y apparaît pas : %d jeux", len(games))
	}
}

// LE MOTEUR D'UN JEU (chantier 36) : le catalogue le pose au scan, depuis la
// racine du jeu et son dossier `Media/` — la règle partagée avec le téléphone
// (storage.GameEngine). Un jeu sans marqueur n'en a pas.
func TestTheCatalogKnowsAGameEngine(t *testing.T) {
	resetCatalog()
	root := t.TempDir()
	writeGameFolder(t, root, "ps4", "Melody of Memory", "eboot.bin")
	writeGameFolder(t, root, "ps4", filepath.Join("Melody of Memory", "Media"), "boot.config")
	writeGameFolder(t, root, "ps4", "FIFA 22", "package.mft")
	writeGameFolder(t, root, "ps4", "FIFA 22", "Engine.Render.Core2.PlatformPs4.prx")
	writeGameFolder(t, root, "ps4", filepath.Join("FIFA 22", "Data"), "layout.toc")
	writeGameFolder(t, root, "ps4", "Sonic Mania Plus", "data.rsdk")

	listed, err := ListCatalog(root, "ps4", "")
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	engines := map[string]string{}
	for _, rom := range listed {
		engines[rom.Title] = rom.Engine
	}
	want := map[string]string{"Melody of Memory": "unity", "FIFA 22": "frostbite", "Sonic Mania Plus": ""}
	for title, engine := range want {
		if got, found := engines[title]; !found || got != engine {
			t.Errorf("%s : moteur %q (listé : %v), attendu %q", title, got, found, engine)
		}
	}
}
