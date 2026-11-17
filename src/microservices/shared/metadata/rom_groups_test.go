package metadata

import (
	"strconv"
	"testing"
)

// LES DISQUES D'UN JEU ET SA SÉRIE, d'après les seuls noms d'une liste : la
// règle du serveur ET du téléphone (chantier 57, `#716`). Les quatre disques
// de FF IX, listés dans le désordre, forment un groupe rangé par numéro ; ceux
// de C&C sont nommés ; un jeu dont un seul disque est listé n'a pas de groupe.
// La série qu'une fiche a donnée passe devant le nom ; « Final Fantasy
// Origins » rejoint « Final Fantasy », « Crash Team Racing » reste seul.
func TestGroupRomsGroupsDiscsAndSeries(t *testing.T) {
	names := []string{
		"Final Fantasy IX [F] [Disc3of4][SLES-22966]",
		"Final Fantasy IX [F] [Disc1of4][SLES-02966]",
		"Final Fantasy IX [F] [Disc4of4][SLES-32966]",
		"Final Fantasy IX [F] [Disc2of4][SLES-12966]",
		"Command & Conquer [F] [NOD Disc][SLES-10531]",
		"Command & Conquer [F] [GDI Disc][SLES-00531]",
		"Final Fantasy Origins - Final Fantasy I [E] [SLES-04034]",
		"Parasite Eve II [F] [Disc1of2][SLES-02559]",
		"Crash Team Racing [E-F-G-I-N-S] [SCES-02105] [Patched]",
	}
	// La fiche du disque GDI a déjà été ouverte : IGDB lui a donné sa série.
	rememberSeries("groups-test", "ref-5", "C&C", sourceIGDB)
	t.Cleanup(func() { forgetSeries("groups-test", "ref-5") })

	groups := GroupRoms("groups-test", len(names), func(index int) (string, string) {
		return "ref-" + strconv.Itoa(index), names[index]
	})
	if len(groups) != len(names) {
		t.Fatalf("%d groupes pour %d jeux", len(groups), len(names))
	}

	for index, disc := range map[int]int{1: 1, 3: 2, 0: 3, 2: 4} {
		if group := groups[index]; group.DiscGroup != "Final Fantasy IX [F]" || group.Disc != disc || group.DiscCount != 4 || group.DiscLabel != "" {
			t.Errorf("FF IX, disque %d : %+v", disc, group)
		}
	}
	if gdi, nod := groups[5], groups[4]; gdi.DiscGroup != "Command & Conquer [F]" || gdi.Disc != 1 || gdi.DiscLabel != "GDI" ||
		nod.Disc != 2 || nod.DiscLabel != "NOD" || nod.DiscCount != 2 {
		t.Errorf("C&C : %+v / %+v", gdi, nod)
	}
	if parasite := groups[7]; parasite.DiscGroup != "" || parasite.Disc != 0 || parasite.DiscCount != 0 {
		t.Errorf("un seul disque listé forme un groupe : %+v", parasite)
	}

	for index, want := range map[int]RomGroups{
		0: {Series: "Final Fantasy", SeriesSource: "name"},
		6: {Series: "Final Fantasy", SeriesSource: "name"},
		5: {Series: "C&C", SeriesSource: sourceIGDB},
		4: {Series: "Command & Conquer", SeriesSource: "name"},
		7: {Series: "Parasite Eve", SeriesSource: "name"},
		8: {Series: "Crash Team Racing", SeriesSource: "name"},
	} {
		if group := groups[index]; group.Series != want.Series || group.SeriesSource != want.SeriesSource {
			t.Errorf("%s : série %q (%q), attendue %q (%q)", names[index], group.Series, group.SeriesSource, want.Series, want.SeriesSource)
		}
	}
}
