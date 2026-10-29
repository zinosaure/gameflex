package storage

import "testing"

// LES JEUX DE LA BIBLIOTHÈQUE, tels qu'ils sont rangés (relevé du 2026-10-27) :
// chacun doit être reconnu par le scan, et un jeu sans marqueur ne l'est pas.
func TestGameEngineOfTheLibrary(t *testing.T) {
	games := titlesOf(scanOf(t, newMemorySource("gamebox",
		"roms/ps4/Kingdom Hearts - Melody of Memory/eboot.bin",
		"roms/ps4/Kingdom Hearts - Melody of Memory/archive.psarc",
		"roms/ps4/Kingdom Hearts - Melody of Memory/Media/boot.config",
		"roms/ps4/Kingdom Hearts - Melody of Memory/Media/globalgamemanagers",
		"roms/ps4/Kingdom Hearts III/eboot.bin",
		"roms/ps4/Kingdom Hearts III/ue4commandline.txt",
		"roms/ps4/Kingdom Hearts III/tresgame/Content/x.pak",
		"roms/ps4/FIFA 22/eboot.bin",
		"roms/ps4/FIFA 22/package.mft",
		"roms/ps4/FIFA 22/Engine.Render.Core2.PlatformPs4.prx",
		"roms/ps4/FIFA 22/Data/layout.toc",
		"roms/ps4/Grand Theft Auto V/eboot.bin",
		"roms/ps4/Grand Theft Auto V/common.rpf",
		"roms/ps4/Sonic Mania Plus/eboot.bin",
		"roms/ps4/Sonic Mania Plus/data.rsdk",
		"roms/ps5/Black Myth Wukong/eboot.bin",
		"roms/ps5/Black Myth Wukong/uecommandline.txt",
		"roms/ps5/Richman 11/eboot.bin",
		"roms/ps5/Richman 11/Media/boot.config",
		"roms/ps5/Final Fantasy VII Rebirth/Final Fantasy VII Rebirth.zar",
	)))

	for title, want := range map[string]string{
		"Kingdom Hearts - Melody of Memory": EngineUnity,
		"Kingdom Hearts III":                EngineUnreal,
		"FIFA 22":                           EngineFrostbite,
		"Grand Theft Auto V":                EngineRage,
		"Sonic Mania Plus":                  "",
		"Black Myth Wukong":                 EngineUnreal,
		"Richman 11":                        EngineUnity,
		// Emballé : son moteur ne se lit qu'au lancement.
		"Final Fantasy VII Rebirth": "",
	} {
		game, found := games[title]
		if !found {
			t.Fatalf("%s n'est pas listé", title)
		}
		if game.Engine != want {
			t.Errorf("%s : moteur %q, attendu %q", title, game.Engine, want)
		}
	}
}

// Un seul marqueur de Frostbite ne suffit pas : un `package.mft` ou un dossier
// `Data/` se trouvent ailleurs.
func TestFrostbiteNeedsAllItsMarkers(t *testing.T) {
	root := []Entry{{Name: "eboot.bin"}, {Name: "package.mft"}, {Name: "Data", Dir: true}}
	if engine := GameEngine("ps4", root, nil); engine != "" {
		t.Errorf("sans module Engine.*.prx : %q", engine)
	}
}

// Les consoles sans moteur reconnaissable ne coûtent rien : `list` n'est pas
// appelée, et la PS3 (des `.iso`) n'a pas de moteur au scan.
func TestGameEngineOnlyForPs4AndPs5(t *testing.T) {
	root := []Entry{{Name: "ue4commandline.txt"}, {Name: "Media", Dir: true}}
	listed := false
	list := func(string) ([]Entry, error) {
		listed = true

		return nil, nil
	}
	for _, console := range []string{"ps3", "switch", "nes"} {
		if engine := GameEngine(console, root, list); engine != "" {
			t.Errorf("%s : %q", console, engine)
		}
	}
	if listed {
		t.Error("un sous-dossier a été listé pour une console sans moteur")
	}
}
