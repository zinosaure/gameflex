package retrogaming

import (
	"os"
	"path/filepath"
	"testing"

	"gameflex/consoles"
)

// RIEN À METTRE EN SCÈNE : ON REND LA RACINE.
//
// Le cas est réel — le dossier de la console n'existe pas — et le repli est
// voulu : fabriquer un dossier temporaire vide ne ferait que déplacer le
// message d'erreur du cœur.
//
// Ce test s'appelait « NoChoiceKeepsFlatRootUnchanged » et affirmait qu'une PS1
// sans choix garde la racine. C'est faux depuis #144 dès que son dossier
// contient de vrais dumps — et il passait alors pour la MAUVAISE raison, sa
// racine n'existant pas. Renommé sur ce qu'il vérifie vraiment.
func TestResolveBiosDirFallsBackToTheRootWhenNothingIsStageable(t *testing.T) {
	root := "/some/flat/bios/root"
	saves := t.TempDir()

	dir, tempDir, _, err := resolveBiosDir(root, saves, "ps1", "")
	if err != nil {
		t.Fatalf("resolveBiosDir failed: %v", err)
	}
	if dir != root {
		t.Errorf("dir = %q, want %q (la racine, faute de quoi mettre en scène)", dir, root)
	}
	if tempDir != "" {
		t.Errorf("tempDir = %q, want empty (rien à nettoyer)", tempDir)
	}
}

// LE DOSSIER EST L'IDENTIFIANT DE LA CONSOLE (registre, 2026-09-01), et le
// disque le confirme : `bios/ps1/`, pas de `bios/psx/`. Ce test créait `psx/`
// — le nom d'avant — et il passait, parce que la table du runtime disait
// encore psx : il figeait la table au lieu du disque. Le sélecteur (kernel)
// offrait les BIOS de `ps1/`, et le runtime les cherchait ailleurs.
func TestResolveBiosDirCopiesOnlyChosenFile(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	_ = saves
	ps1Dir := filepath.Join(root, "ps1")
	if err := os.MkdirAll(ps1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ps1Dir, "scph5501.bin"), []byte("chosen"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ps1Dir, "scph1001.bin"), []byte("not chosen"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, tempDir, _, err := resolveBiosDir(root, saves, "ps1", "scph5501.bin")
	if err != nil {
		t.Fatalf("resolveBiosDir failed: %v", err)
	}
	if dir == "" || dir != tempDir {
		t.Fatalf("resolveBiosDir returned dir=%q tempDir=%q, want a matching non-empty temp dir", dir, tempDir)
	}
	defer os.RemoveAll(tempDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read resolved dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "scph5501.bin" {
		t.Errorf("resolved dir contains %v, want exactly [scph5501.bin] — every other BIOS variant must stay invisible to the core", entries)
	}
	data, err := os.ReadFile(filepath.Join(dir, "scph5501.bin"))
	if err != nil || string(data) != "chosen" {
		t.Errorf("copied file content = %q, err=%v, want %q", data, err, "chosen")
	}
}

func TestResolveBiosDirMissingFileErrors(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	_ = saves
	if err := os.MkdirAll(filepath.Join(root, "ps1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := resolveBiosDir(root, saves, "ps1", "does-not-exist.bin"); err == nil {
		t.Error("resolveBiosDir with a nonexistent bios filename = nil error, want an error")
	}
}

// A folder-shaped dump (scph10000/ holding .bin + .NVM + .MEC, a real shape
// found on this deployment) must stage every file it contains, flat, and
// must ALSO reach the per-core mirror subdirectory — pcsx2 only ever looks
// there, so a companion staged at the root alone would be invisible to it.
func TestResolveBiosDirStagesFolderEntryFlat(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	_ = saves
	entry := filepath.Join(root, "ps2", "scph10000")
	if err := os.MkdirAll(entry, 0o755); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"scph10000.bin": "rom",
		"scph10000.NVM": "nvram",
		"scph10000.MEC": "mec",
	}
	for name, content := range want {
		if err := os.WriteFile(filepath.Join(entry, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dir, tempDir, _, err := resolveBiosDir(root, saves, "ps2", "scph10000")
	if err != nil {
		t.Fatalf("resolveBiosDir failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	for _, base := range []string{dir, filepath.Join(dir, "pcsx2", "bios")} {
		for name, content := range want {
			data, err := os.ReadFile(filepath.Join(base, name))
			if err != nil {
				t.Errorf("%s/%s not staged: %v", base, name, err)
				continue
			}
			if string(data) != content {
				t.Errorf("%s/%s = %q, want %q", base, name, data, content)
			}
		}
	}
}

// The core creates its .NVM/.MEC in the mirror it loaded the BIOS from;
// those must land on disk so the next session reuses them, while anything
// already on disk stays untouched.
func TestPersistBiosCompanionsCreatesOnlyMissingFiles(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	_ = saves
	ps2 := filepath.Join(root, "ps2")
	if err := os.MkdirAll(ps2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ps2, "bios.bin"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	mirror := filepath.Join(tempDir, "pcsx2", "bios")
	if err := os.MkdirAll(mirror, 0o755); err != nil {
		t.Fatal(err)
	}
	// What a session's temp dir looks like at teardown: the BIOS as staged
	// (byte-identical), plus companions the core generated.
	for name, content := range map[string]string{
		"bios.bin": "rewritten-by-core",
		"bios.NVM": "nvram",
		"bios.MEC": "mec",
	} {
		if err := os.WriteFile(filepath.Join(mirror, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	persistBiosCompanions(root, saves, "ps2", "bios.bin", tempDir)

	for name, want := range map[string]string{
		"bios.bin": "original", // never overwritten
		"bios.NVM": "nvram",
		"bios.MEC": "mec",
	} {
		data, err := os.ReadFile(filepath.Join(ps2, name))
		if err != nil {
			t.Errorf("%s missing on disk: %v", name, err)
			continue
		}
		if string(data) != want {
			t.Errorf("%s = %q, want %q", name, data, want)
		}
	}
}

// Same, for a folder-shaped entry: companions belong INSIDE the folder, not
// beside it, or they'd drift away from the BIOS they describe.
func TestPersistBiosCompanionsWritesInsideFolderEntry(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	_ = saves
	entry := filepath.Join(root, "ps2", "scph10000")
	if err := os.MkdirAll(entry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry, "scph10000.bin"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	mirror := filepath.Join(tempDir, "pcsx2", "bios")
	if err := os.MkdirAll(mirror, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mirror, "scph10000.NVM"), []byte("nvram"), 0o644); err != nil {
		t.Fatal(err)
	}

	persistBiosCompanions(root, saves, "ps2", "scph10000", tempDir)

	if _, err := os.Stat(filepath.Join(entry, "scph10000.NVM")); err != nil {
		t.Errorf("companion not written inside the folder entry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "ps2", "scph10000.NVM")); err == nil {
		t.Error("companion written BESIDE the folder entry — it belongs inside it")
	}
}

// A folder holding only its .bin — no companions yet — must still be a
// usable entry, otherwise a newly added dump could never be launched and so
// could never gain the companions that would make it launchable.
func TestResolveBiosDirAcceptsFolderWithoutCompanions(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	_ = saves
	entry := filepath.Join(root, "ps2", "ps2-0220e")
	if err := os.MkdirAll(entry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry, "ps2-0220e.bin"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, tempDir, _, err := resolveBiosDir(root, saves, "ps2", "ps2-0220e")
	if err != nil {
		t.Fatalf("resolveBiosDir on a companion-less folder failed: %v", err)
	}
	defer os.RemoveAll(tempDir)
	if _, err := os.Stat(filepath.Join(dir, "pcsx2", "bios", "ps2-0220e.bin")); err != nil {
		t.Errorf("bios not staged into the mirror: %v", err)
	}
}

func TestResolveBiosDirStagesABareBinChoiceAndItsMirror(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	_ = saves
	ps2 := filepath.Join(root, "ps2")
	if err := os.MkdirAll(ps2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ps2, "ps2-0220e.bin"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}

	// UN .bin NU EST UN CHOIX VALABLE, depuis le 2026-09-08.
	//
	// Ce test attendait exactement l'inverse. La règle « un BIOS PS2 est un
	// dossier » supposait qu'on puisse écrire à côté du dump pour y créer ses
	// compagnons ; le volume des BIOS est monté en lecture seule, et la seule
	// chose que la règle produisait était de cacher du sélecteur la forme
	// réellement présente chez les gens.
	dir, tempDir, _, err := resolveBiosDir(root, saves, "ps2", "ps2-0220e.bin")
	if err != nil {
		t.Fatalf("resolveBiosDir(.bin nu) = %v, want no error", err)
	}
	if tempDir == "" {
		t.Fatal("un choix explicite doit être mis en scène dans un dossier temporaire")
	}
	defer os.RemoveAll(tempDir)
	if _, err := os.Stat(filepath.Join(dir, "ps2-0220e.bin")); err != nil {
		t.Errorf("le .bin choisi n'est pas dans le dossier de session : %v", err)
	}
	// Et dans le miroir que LRPS2 est le seul à lire.
	if _, err := os.Stat(filepath.Join(dir, "pcsx2", "bios", "ps2-0220e.bin")); err != nil {
		t.Errorf("le .bin choisi n'est pas dans le miroir pcsx2/bios : %v", err)
	}
}

// LE VOLUME DES BIOS EST EN LECTURE SEULE — le cas réel, et celui qui rendait
// la PS2 injouable.
//
// L'émulateur écrit ses .NVM/.MEC dans le dossier de session ; on ne peut pas
// les recopier à côté du dump. Ils doivent alors atterrir dans un cache
// inscriptible, et revenir tout seuls à la partie suivante — sans quoi une PS2
// repart chaque fois avec sa langue, son horloge et sa région d'usine.
func TestBiosCompanionsSurviveAReadOnlyBiosVolume(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	ps2 := filepath.Join(root, "ps2")
	if err := os.MkdirAll(ps2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ps2, "ps2-0220e.bin"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Première partie : le cœur écrit son compagnon dans le dossier de session.
	first, tempDir, _, err := resolveBiosDir(root, saves, "ps2", "ps2-0220e.bin")
	if err != nil {
		t.Fatalf("resolveBiosDir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	if err := os.WriteFile(filepath.Join(first, "ps2-0220e.NVM"), []byte("region"), 0o644); err != nil {
		t.Fatal(err)
	}

	// LA DESTINATION N'ACCEPTE PLUS L'ÉCRITURE.
	//
	// On la fait DISPARAÎTRE plutôt que de la passer en lecture seule : un
	// `chmod` ne dit rien à root, qui passe outre, et les tests tournent en
	// root dans le conteneur — l'assertion aurait été vide de sens. Ce qu'on
	// produit ici est la même CONDITION vue du code : la sonde d'écriture
	// échoue.
	//
	// CE QUE CE TEST NE PROUVE PAS, et qui a été vérifié à part : que la sonde
	// échoue bien sur un montage en LECTURE SEULE, qui est ce que la production
	// a vraiment (kernel/internal/docker/docker.go monte la racine froide —
	// roms, cores, bios — avec ReadOnly: true). Mesuré le 2026-09-08, en root,
	// dans un conteneur :
	//
	//     writable(/ro-mount) = false      // -v dir:/ro-mount:ro
	//     writable(/rw-mount) = true       // -v dir:/rw-mount
	//
	// À rejouer avec `docker run --rm -v <dir>:/ro:ro golang:1.26-alpine` si
	// l'on doute un jour de la sonde elle-même.
	if err := os.RemoveAll(ps2); err != nil {
		t.Fatal(err)
	}

	persistBiosCompanions(root, saves, "ps2", "ps2-0220e.bin", tempDir)

	cached := filepath.Join(biosCompanionCache(saves, "ps2", "ps2-0220e.bin"), "ps2-0220e.NVM")
	if _, err := os.Stat(cached); err != nil {
		t.Fatalf("le compagnon n'a pas été mis en cache : %v", err)
	}
	// Partie suivante : il revient tout seul, dans la racine ET dans le miroir.
	if err := os.MkdirAll(ps2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ps2, "ps2-0220e.bin"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, tempDir2, _, err := resolveBiosDir(root, saves, "ps2", "ps2-0220e.bin")
	if err != nil {
		t.Fatalf("resolveBiosDir (2e partie) : %v", err)
	}
	defer os.RemoveAll(tempDir2)
	for _, path := range []string{
		filepath.Join(second, "ps2-0220e.NVM"),
		filepath.Join(second, "pcsx2", "bios", "ps2-0220e.NVM"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("le compagnon n'est pas revenu dans %s : %v", path, err)
		}
	}
}

// SANS CHOIX EXPLICITE, LA PS2 DOIT QUAND MÊME AVOIR SON MIROIR.
//
// C'est la panne relevée sur une vraie session le 2026-09-08 : LRPS2 ne lit
// QUE `<systemDir>/pcsx2/bios`, et sans mise en scène ce dossier n'existe
// nulle part — le cœur cherchait dans `<racine des BIOS>/pcsx2/bios` et
// refusait le jeu. La console était injouable tant qu'on n'avait pas choisi
// soi-même, sans que rien ne l'explique.
func TestResolveBiosDirStagesEverythingWhenNothingWasChosen(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	ps2 := filepath.Join(root, "ps2")
	if err := os.MkdirAll(filepath.Join(ps2, "scph10000"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ps2, "ps2-0220e.bin"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Un dump rangé en dossier : ses fichiers doivent arriver à plat, au même
	// niveau que les .bin nus.
	if err := os.WriteFile(filepath.Join(ps2, "scph10000", "scph10000.bin"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, tempDir, _, err := resolveBiosDir(root, saves, "ps2", "")
	if err != nil {
		t.Fatalf("resolveBiosDir(sans choix) = %v", err)
	}
	if tempDir == "" {
		t.Fatal("une console dont le cœur ne lit que le miroir doit être mise en scène même sans choix")
	}
	defer os.RemoveAll(tempDir)

	for _, name := range []string{"ps2-0220e.bin", "scph10000.bin"} {
		if _, err := os.Stat(filepath.Join(dir, "pcsx2", "bios", name)); err != nil {
			t.Errorf("%s absent du miroir pcsx2/bios : %v", name, err)
		}
	}
}

// UNE CONSOLE QUI NE DÉCLARE AUCUN BIOS garde le raccourci : pas de choix, pas
// de mise en scène, la racine telle quelle.
//
// Le test disait « une console SANS miroir garde le raccourci » et prenait la
// PS1 pour exemple — ce qui n'est plus vrai (#144) : la PS1 déclare trois
// dumps, donc elle est mise en scène. Ce qui garantit qu'on n'a rien changé
// pour le reste du catalogue, c'est l'absence de BIOS DÉCLARÉS, pas l'absence
// de miroir. La console est cherchée dans le registre pour que ce test suive
// ses évolutions au lieu de figer un identifiant.
func TestResolveBiosDirSkipsStagingForAConsoleWithoutDeclaredBios(t *testing.T) {
	var consoleID string
	for _, console := range consoles.All() {
		_, mirrored := biosMirrorSubdir[console.ID]
		if !mirrored && len(consoles.BiosFor(console.ID)) == 0 {
			consoleID = console.ID

			break
		}
	}
	if consoleID == "" {
		t.Skip("toutes les consoles du registre déclarent un BIOS ou un miroir")
	}

	root := t.TempDir()
	saves := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, consoleID), 0o755); err != nil {
		t.Fatal(err)
	}

	dir, tempDir, _, err := resolveBiosDir(root, saves, consoleID, "")
	if err != nil {
		t.Fatalf("resolveBiosDir(%s, sans choix) = %v", consoleID, err)
	}
	if dir != root || tempDir != "" {
		t.Errorf("resolveBiosDir(%s, sans choix) = (%q, %q), want (%q, \"\")",
			consoleID, dir, tempDir, root)
	}
}

// LA PS1 TOURNAIT SUR SON BIOS ÉMULÉ ALORS QUE LES VRAIS ÉTAIENT LÀ (#144).
//
// Sans choix explicite, `resolveBiosDir` rendait la RACINE des BIOS — qui ne
// contient que des dossiers de console, pas un seul fichier (mesuré le
// 2026-09-17 sur la vraie bibliothèque). `pcsx_rearmed` la balayait, n'y
// trouvait rien, écrivait « No BIOS files found. » et se rabattait sur son HLE
// EN SILENCE. Les jeux tournaient quand même, mais le HLE casse une partie des
// titres et rien ne le disait.
//
// C'est l'assertion qui manquait : les deux tests voisins passaient sans avoir
// un seul fichier sur le disque, donc sans jamais exercer ce chemin.
func TestResolveBiosDirStagesADeclaredConsoleWhenNothingWasChosen(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	ps1 := filepath.Join(root, "ps1")
	if err := os.MkdirAll(ps1, 0o755); err != nil {
		t.Fatal(err)
	}
	dumps := map[string]string{
		"scph5501.bin": "usa",
		"scph1001.bin": "vieux",
	}
	for name, content := range dumps {
		if err := os.WriteFile(filepath.Join(ps1, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dir, tempDir, chosen, err := resolveBiosDir(root, saves, "ps1", "")
	if err != nil {
		t.Fatalf("resolveBiosDir(ps1, sans choix) = %v", err)
	}
	if tempDir == "" {
		t.Fatal("une console qui déclare des BIOS doit être mise en scène même sans choix : " +
			"sinon le cœur balaie une racine qui n'a que des dossiers")
	}
	defer os.RemoveAll(tempDir)

	for name, content := range dumps {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s absent du dossier de session : %v", name, err)

			continue
		}
		if string(data) != content {
			t.Errorf("%s = %q, want %q", name, data, content)
		}
	}
	// Et un BIOS est DÉSIGNÉ : certains cœurs ne choisissent pas tout seuls.
	if chosen == "" {
		t.Error("aucun BIOS désigné alors que deux étaient disponibles")
	}
}

// UN CŒUR QUI NE CHOISIT PAS DOIT SE VOIR DÉSIGNER UN BIOS.
//
// pcsx2 énumère les dumps qu'il voit puis refuse le jeu quand même : il attend
// que l'option `pcsx2_bios` nomme celui à prendre. Sans choix explicite,
// personne ne la remplissait — « Could not find any valid PS2 BIOS File »
// s'affichait alors que trois venaient d'être trouvés (2026-09-08).
func TestResolveBiosDirNamesABiosWhenNothingWasChosen(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	ps2 := filepath.Join(root, "ps2")
	if err := os.MkdirAll(ps2, 0o755); err != nil {
		t.Fatal(err)
	}
	// Volontairement dans le désordre, et avec un intrus qui n'est pas un BIOS.
	for _, name := range []string{"ps2-0230j.bin", "notes.txt", "ps2-0220e.bin"} {
		if err := os.WriteFile(filepath.Join(ps2, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, tempDir, chosen, err := resolveBiosDir(root, saves, "ps2", "")
	if err != nil {
		t.Fatalf("resolveBiosDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Le plus RÉCENT : les dumps se nomment par version, donc le dernier dans
	// l.ordre alphabétique. Un BIOS récent joue les jeux anciens, l.inverse est
	// faux.
	if chosen != "ps2-0230j.bin" {
		t.Errorf("resolveBiosDir a désigné %q, want le plus récent (ps2-0230j.bin)", chosen)
	}
}

// Un choix explicite reste le choix, évidemment.
func TestResolveBiosDirKeepsTheExplicitChoice(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	ps2 := filepath.Join(root, "ps2")
	if err := os.MkdirAll(ps2, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ps2-0220e.bin", "ps2-0230j.bin"} {
		if err := os.WriteFile(filepath.Join(ps2, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, tempDir, chosen, err := resolveBiosDir(root, saves, "ps2", "ps2-0230j.bin")
	if err != nil {
		t.Fatalf("resolveBiosDir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	if chosen != "ps2-0230j.bin" {
		t.Errorf("resolveBiosDir a désigné %q, want le choix explicite", chosen)
	}
}

// LA DREAMCAST N'AVAIT JAMAIS SON MIROIR — mesuré le 2026-09-14.
//
// b6206ff1 a ajouté `"dreamcast": "dc"` à biosMirrorSubdir, parce que flycast
// ne lit QUE `<systemDir>/dc`. Mais la console manquait à la table des
// dossiers, et resolveBiosDir rendait la racine avant même d'atteindre le
// miroir : le code écrit pour la Dreamcast ne s'exécutait pour aucune partie.
func TestResolveBiosDirStagesTheDreamcastMirrorWhenNothingWasChosen(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	dreamcast := filepath.Join(root, "dreamcast")
	if err := os.MkdirAll(dreamcast, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dc_boot.bin", "dc_flash.bin"} {
		if err := os.WriteFile(filepath.Join(dreamcast, name), []byte("rom"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dir, tempDir, _, err := resolveBiosDir(root, saves, "dreamcast", "")
	if err != nil {
		t.Fatalf("resolveBiosDir(dreamcast, sans choix) = %v", err)
	}
	if tempDir == "" {
		t.Fatal("flycast ne lit que <systemDir>/dc : sans mise en scène, il ne trouve aucun BIOS")
	}
	defer os.RemoveAll(tempDir)
	for _, name := range []string{"dc_boot.bin", "dc_flash.bin"} {
		if _, err := os.Stat(filepath.Join(dir, "dc", name)); err != nil {
			t.Errorf("%s absent du miroir dc : %v", name, err)
		}
	}
}

// Une console HORS registre n'a rien à choisir : un bios est ignoré, sans
// erreur — la racine telle quelle, comme avant que le choix existe.
func TestResolveBiosDirUnknownConsoleIgnoresChoice(t *testing.T) {
	root := "/some/flat/bios/root"
	dir, tempDir, _, err := resolveBiosDir(root, t.TempDir(), "pas-une-console", "some-file.bin")
	if err != nil {
		t.Fatalf("resolveBiosDir failed: %v", err)
	}
	if dir != root || tempDir != "" {
		t.Errorf("resolveBiosDir(console inconnue, choix) = (%q, %q), want (%q, \"\")", dir, tempDir, root)
	}
}

// UN CHOIX EST HONORÉ POUR TOUTE CONSOLE DU REGISTRE — plus seulement PS1/PS2.
//
// Le test d'avant disait l'inverse : « une console absente de la table ignore
// le choix ». Or le sélecteur du kernel offre les BIOS de `bios/<console>/` pour
// n'importe quelle console : l'utilisateur choisissait, et le runtime jetait le
// choix sans rien dire. La table n'avait que deux entrées.
func TestResolveBiosDirHonorsAChoiceForEveryRegistryConsole(t *testing.T) {
	root := t.TempDir()
	saves := t.TempDir()
	saturn := filepath.Join(root, "saturn")
	if err := os.MkdirAll(saturn, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sega_101.bin", "mpr-17933.bin"} {
		if err := os.WriteFile(filepath.Join(saturn, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dir, tempDir, chosen, err := resolveBiosDir(root, saves, "saturn", "sega_101.bin")
	if err != nil {
		t.Fatalf("resolveBiosDir(saturn, choix) = %v", err)
	}
	if tempDir == "" {
		t.Fatal("un choix explicite doit être mis en scène, quelle que soit la console du registre")
	}
	defer os.RemoveAll(tempDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "sega_101.bin" || chosen != "sega_101.bin" {
		t.Errorf("mis en scène : %v (choisi %q), want exactement [sega_101.bin]", entries, chosen)
	}
}
