package storage

import (
	"os"
	"testing"
)

// Le corpus, scanné en source locale : les formes qu'il contient doivent
// ressortir avec les bons noms, dossiers et tailles. C'est la moitié basse
// du scénario S1 — la moitié haute (bios/ et cores/ exclus des jeux) vit
// dans listing/corpus_test.go, parce que le filtre vit dans Build.
func TestCorpusScanKeepsEveryShape(t *testing.T) {
	root := t.TempDir()
	if err := WriteCorpus(root); err != nil {
		t.Fatalf("corpus : %v", err)
	}

	source, err := NewLocalSource("cold", root)
	if err != nil {
		t.Fatalf("source : %v", err)
	}

	games, err := ScanConvention(source, "roms")
	if err != nil {
		t.Fatalf("scan : %v", err)
	}

	byTitle := map[string]Game{}
	for _, game := range games {
		byTitle[game.Title] = game
	}

	// LE JEU-DOSSIER, la panne la plus chère : le scanner doit le nommer par
	// son dossier, porter son dossier, et annoncer la taille du DOSSIER (un
	// .cue pèse 75 octets ; ce jeu en pèse 554 Mio).
	ace, ok := byTitle["[PS1] Ace Combat 2 [F] [SCES-00901]"]
	if !ok {
		t.Fatalf("Ace Combat 2 manque : %+v", byTitle)
	}
	if ace.Console != "ps1" || ace.Folder != "cold/roms/ps1/[PS1] Ace Combat 2 [F] [SCES-00901]" {
		t.Fatalf("Ace Combat 2 mal lu : %+v", ace)
	}
	if ace.Size != 75+581734272 {
		t.Fatalf("taille = %d, attendu 75 + le .bin", ace.Size)
	}

	// LE MULTI-DISQUES : le .m3u gagne sur les .cue, et le dossier entier ne
	// fait qu'UN jeu.
	final, ok := byTitle["[PS1] Final Fantasy IX [F]"]
	if !ok || final.Size == 64 {
		t.Fatalf("Final Fantasy IX mal lu : %+v", final)
	}

	// LE DOSSIER SOUS LA CONSOLE est UN jeu, quoi qu'il contienne : USA/
	// porte deux cartouches, et ne fait qu'un jeu nommé par le dossier —
	// décision 2026-09-02 (scanner.go). La taille est celle des deux.
	usa, ok := byTitle["USA"]
	if !ok {
		t.Fatalf("USA manque : %+v", byTitle)
	}
	if usa.Console != "nes" || usa.Folder != "cold/roms/nes/USA" {
		t.Fatalf("USA mal lu : %+v", usa)
	}
	if usa.Size != 40960+40960 {
		t.Fatalf("taille USA = %d, attendu les deux cartouches", usa.Size)
	}

	// Le jeu-dossier AVEC un sous-dossier : media/ n'est pas compté dans la
	// taille annoncée par le scanner — elle ne voit que les fichiers du
	// dossier lui-même. Le transfert, lui, emportera tout (GameFiles).
	slug, ok := byTitle["[PS2] Metal Slug 4"]
	if !ok || slug.Size != 75+209715200 {
		t.Fatalf("Metal Slug 4 mal lu : %+v", slug)
	}

	// La ROM qui EST un zip : le .zip reste un jeu comme les autres, sous
	// son extension — jamais ouvert, jamais deviné.
	if _, ok := byTitle["Pokemon Platinum (Europe)"]; !ok {
		t.Fatalf("la ROM NDS manque : %+v", byTitle)
	}

	// Les noms difficiles : accents, #, &, crochets. Ils se listent tels
	// quels — le caractère d'URL n'est pas le problème du scanner.
	if _, ok := byTitle["Chrono Trigger (é, à, ç)"]; !ok {
		t.Fatalf("le jeu à accents manque : %+v", byTitle)
	}
	if _, ok := byTitle["Super Mario World #1 & 2 [!]"]; !ok {
		t.Fatalf("le jeu à caractères d'URL manque : %+v", byTitle)
	}

	// Les fichiers cachés et les dossiers de service sont sautés sans un mot.
	for _, game := range games {
		if game.Title == ".DS_Store" || game.Title == "System Volume Information" {
			t.Fatalf("le bruit système est remonté en jeu : %+v", game)
		}
	}

	// Le gros fichier : un 2,5 Go clairsemé se scanne comme un autre.
	gros, ok := byTitle["[PS3] Gros Jeu"]
	if !ok || gros.Size != 2684354560 {
		t.Fatalf("le gros jeu mal lu : %+v", gros)
	}
}

// Le corpus est reproductible : deux générations posent les mêmes tailles —
// le banc compare des octets, jamais des souvenirs.
func TestCorpusIsReproducible(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	if err := WriteCorpus(first); err != nil {
		t.Fatalf("première génération : %v", err)
	}
	if err := WriteCorpus(second); err != nil {
		t.Fatalf("seconde génération : %v", err)
	}

	compare := func(a, b string, t *testing.T) {
		t.Helper()

		infoA, errA := os.Stat(a)
		infoB, errB := os.Stat(b)
		if errA != nil || errB != nil || infoA.Size() != infoB.Size() {
			t.Fatalf("%s et %s diffèrent : %v / %v", a, b, errA, errB)
		}
	}

	compare(first+"/roms/ps1/[PS1] Ace Combat 2 [F] [SCES-00901]/Ace Combat 2.bin",
		second+"/roms/ps1/[PS1] Ace Combat 2 [F] [SCES-00901]/Ace Combat 2.bin", t)
	compare(first+"/roms/ps3/[PS3] Gros Jeu/gros.iso",
		second+"/roms/ps3/[PS3] Gros Jeu/gros.iso", t)
}
