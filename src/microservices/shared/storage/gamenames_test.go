package storage

import "testing"

// LES DISQUES : les noms des dossiers PS1 de la dev, et ceux des jeux complets
// de No-Intro et Redump. Le groupe perd la mention du disque et le numéro de
// série, qui changent d'un disque à l'autre ; il garde le reste.
func TestDiscOf(t *testing.T) {
	for name, want := range map[string]Disc{
		"[PS1] Final Fantasy IX [F] [Disc1of4][SLES-02966]":              {Group: "[PS1] Final Fantasy IX [F]", Number: 1},
		"[PS1] Final Fantasy IX [F] [Disc4of4][SLES-32966]":              {Group: "[PS1] Final Fantasy IX [F]", Number: 4},
		"[PS1] Command & Conquer [F] [GDI Disc][SLES-00531]":             {Group: "[PS1] Command & Conquer [F]", Label: "GDI"},
		"[PS1] Command & Conquer [F] [NOD Disc][SLES-10531]":             {Group: "[PS1] Command & Conquer [F]", Label: "NOD"},
		"[PS1] Parasite Eve II [F] [Disc2of2][SLES-12559]":               {Group: "[PS1] Parasite Eve II [F]", Number: 2},
		"Final Fantasy VII (Europe) (Disc 1)":                            {Group: "Final Fantasy VII (Europe)", Number: 1},
		"Metal Gear Solid (USA) (Disc 2 of 2) (Rev 1)":                   {Group: "Metal Gear Solid (USA) (Rev 1)", Number: 2},
		"Maniac Mansion (Disk A)":                                        {Group: "Maniac Mansion", Number: 1, Label: "A"},
		"Zork (Side B)":                                                  {Group: "Zork", Number: 2, Label: "B"},
		"Zelda no Densetsu - The Hyrule Fantasy (Japan) (Disk 1 Side B)": {Group: "Zelda no Densetsu - The Hyrule Fantasy (Japan)", Number: 5, Label: "1B"},
	} {
		got, found := DiscOf(name)
		if !found || got != want {
			t.Errorf("DiscOf(%q) = %+v %v, want %+v", name, got, found, want)
		}
	}

	for _, name := range []string{
		"[PS1] Final Fantasy Anthology - Final Fantasy IV [E] [SCES-03840]",
		"[PS1] Crash Team Racing [E-F-G-I-N-S] [SCES-02105] [Patched]",
		"Ike Ike! Nekketsu Hockey Bu (J) [T+Eng.99_Disconnected]",
		"Micro Machines (USA, Europe) (MDMM ACD3)",
		"Disco Elysium",
	} {
		if got, found := DiscOf(name); found {
			t.Errorf("DiscOf(%q) = %+v, want no disc", name, got)
		}
	}
}

func TestSerialsIn(t *testing.T) {
	if got := SerialsIn("[PS1] Command & Conquer [F] [GDI Disc][SLES-00531] SLUS_012.72"); len(got) != 2 || got[0] != "SLES00531" || got[1] != "SLUS01272" {
		t.Fatalf("SerialsIn = %v", got)
	}
	if got := SerialsIn("[PS1] Tekken 3 [E] FIFA 99"); len(got) != 0 {
		t.Fatalf("SerialsIn = %v", got)
	}
}

// LA SÉRIE QUE DIT LE NOM, puis le rattachement à une série plus courte de
// la même liste.
func TestSeriesFromName(t *testing.T) {
	for name, want := range map[string]string{
		"[PS1] Final Fantasy IX [F] [Disc1of4][SLES-02966]":                 "Final Fantasy",
		"[PS1] Final Fantasy Anthology - Final Fantasy IV [E] [SCES-03840]": "Final Fantasy Anthology",
		"[PS1] Final Fantasy Origins - Final Fantasy I [E] [SLES-04034]":    "Final Fantasy Origins",
		"[PS1] Command & Conquer [F] [GDI Disc][SLES-00531]":                "Command & Conquer",
		"[PS1] Parasite Eve II [F] [Disc1of2][SLES-02559]":                  "Parasite Eve",
		"[PS1] Tekken 3 [E] [SCES-01237]":                                   "Tekken",
		"[PS1] Tekken [E] [SCES-00005]":                                     "Tekken",
		"[PS1] Ace Combat 3 - Electrosphere [E-F-G-I-S] [SCES-02066]":       "Ace Combat",
		"[PS1] Gran Turismo 2 - GT Mode [E-F-G-I-S] [SCES-12380]":           "Gran Turismo",
		"[PS1] Crash Team Racing [E-F-G-I-N-S] [SCES-02105] [Patched]":      "Crash Team Racing",
		"Legend of Zelda, The (USA)":                                        "The Legend of Zelda",
		"Mega Man 2 (U)":                                                    "Mega Man",
		"Final Fantasy: Crystal Chronicles (Europe)":                        "Final Fantasy",
		"1942 (Japan, USA) (En)":                                            "1942",
	} {
		if got := SeriesFromName(name); got != want {
			t.Errorf("SeriesFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFoldSeries(t *testing.T) {
	known := map[string]string{
		"final fantasy": "Final Fantasy", "final fantasy anthology": "Final Fantasy Anthology",
		"final fantasy origins": "Final Fantasy Origins", "crash team racing": "Crash Team Racing",
		"crash bandicoot": "Crash Bandicoot", "tekken": "Tekken",
	}
	for series, want := range map[string]string{
		"Final Fantasy Anthology": "Final Fantasy",
		"Final Fantasy Origins":   "Final Fantasy",
		"Final Fantasy":           "Final Fantasy",
		"Crash Team Racing":       "Crash Team Racing",
		"Tekken Tag Tournament":   "Tekken",
	} {
		if got := FoldSeries(series, known); got != want {
			t.Errorf("FoldSeries(%q) = %q, want %q", series, got, want)
		}
	}
}
