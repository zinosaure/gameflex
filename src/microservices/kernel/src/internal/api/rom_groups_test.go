package api

import (
	"testing"
)

// UNE FICHE PAR JEU, LES JEUX PAR SÉRIE : la liste PS1 telle que les dossiers
// de la dev la donnent. Les quatre disques de FF IX forment un groupe, dans
// l'ordre, ceux de C&C sont nommés ; un jeu à un seul disque listé n'a pas de
// groupe. « Final Fantasy Anthology » et « Origins » rejoignent « Final
// Fantasy », « Crash Team Racing » reste seul.
func TestTheListingGroupsDiscsAndSeries(t *testing.T) {
	folders := []string{
		"[PS1] Final Fantasy IX [F] [Disc3of4][SLES-22966]",
		"[PS1] Final Fantasy IX [F] [Disc1of4][SLES-02966]",
		"[PS1] Final Fantasy IX [F] [Disc4of4][SLES-32966]",
		"[PS1] Final Fantasy IX [F] [Disc2of4][SLES-12966]",
		"[PS1] Command & Conquer [F] [NOD Disc][SLES-10531]",
		"[PS1] Command & Conquer [F] [GDI Disc][SLES-00531]",
		"[PS1] Final Fantasy Anthology - Final Fantasy IV [E] [SCES-03840]",
		"[PS1] Final Fantasy Origins - Final Fantasy I [E] [SLES-04034]",
		"[PS1] Parasite Eve II [F] [Disc1of2][SLES-02559]",
		"[PS1] Tekken 3 [E] [SCES-01237]",
		"[PS1] Tekken [E] [SCES-00005]",
		"[PS1] Crash Team Racing [E-F-G-I-N-S] [SCES-02105] [Patched]",
	}
	files := []string{}
	for _, folder := range folders {
		files = append(files, "roms/ps1/"+folder+"/game.cue")
	}
	server, _ := listingServer(t, files...)

	byTitle := map[string]map[string]any{}
	for _, rom := range listedRoms(t, server, "ps1") {
		byTitle[rom["title"].(string)] = rom
	}
	if len(byTitle) != len(folders) {
		t.Fatalf("%d jeux listés, %d attendus : %v", len(byTitle), len(folders), byTitle)
	}

	// Listés dans le désordre, rangés par leur numéro.
	for title, disc := range map[string]float64{folders[1]: 1, folders[3]: 2, folders[0]: 3, folders[2]: 4} {
		rom := byTitle[title]
		if rom["discGroup"] != "[PS1] Final Fantasy IX [F]" || rom["disc"] != disc || rom["discCount"] != float64(4) || rom["discLabel"] != nil {
			t.Fatalf("FF IX, disque %v : %v", disc, rom)
		}
	}
	gdi, nod := byTitle[folders[5]], byTitle[folders[4]]
	if gdi["discGroup"] != "[PS1] Command & Conquer [F]" || gdi["disc"] != float64(1) || gdi["discLabel"] != "GDI" ||
		nod["disc"] != float64(2) || nod["discLabel"] != "NOD" || nod["discCount"] != float64(2) {
		t.Fatalf("C&C : %v / %v", gdi, nod)
	}
	if parasite := byTitle[folders[8]]; parasite["discGroup"] != nil || parasite["disc"] != nil {
		t.Fatalf("un seul disque listé forme un groupe : %v", parasite)
	}

	for title, want := range map[string]string{
		folders[0]: "Final Fantasy", folders[6]: "Final Fantasy", folders[7]: "Final Fantasy",
		folders[4]: "Command & Conquer", folders[8]: "Parasite Eve",
		folders[9]: "Tekken", folders[10]: "Tekken", folders[11]: "Crash Team Racing",
	} {
		if rom := byTitle[title]; rom["series"] != want || rom["seriesSource"] != "name" {
			t.Errorf("%s : série %v (%v), attendue %s", title, rom["series"], rom["seriesSource"], want)
		}
	}
}
