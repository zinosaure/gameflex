package retrogaming

import "testing"

// stateFilePath is shared by saveStatePath/screenshotPath (a live
// emulationSession's own methods) AND the standalone HTTP routes
// (handlers.go's SaveStateScreenshot/ListSaveSlots, which have no live
// session to read consoleID/reference/username off of) — both must derive
// the EXACT same path for the same (username, consoleID, reference, slot),
// or those routes would 404/mismatch against a real save.

func TestStateFilePathMatchesForSameInputs(t *testing.T) {
	a := stateFilePath("/saves", "alice", "nes", "Super Mario Bros.", 1, ".state")
	b := stateFilePath("/saves", "alice", "nes", "Super Mario Bros.", 1, ".state")
	if a != b {
		t.Errorf("stateFilePath is not deterministic: %q != %q", a, b)
	}
}

func TestStateFilePathReplacesUnsafeCharacters(t *testing.T) {
	path := stateFilePath("/saves", "alice", "nes", "Some/Game\\Name", 1, ".state")
	if want := "/saves/alice/states/nes/Some_Game_Name_slot1.state"; path != want {
		t.Errorf("stateFilePath(...) = %q, want %q", path, want)
	}
}

func TestStateFilePathScopesByExtension(t *testing.T) {
	statePath := stateFilePath("/saves", "alice", "nes", "Metroid", 1, ".state")
	screenshotPath := stateFilePath("/saves", "alice", "nes", "Metroid", 1, ".png")
	if statePath == screenshotPath {
		t.Errorf("save-state and screenshot paths collided: %q", statePath)
	}
	if want := "/saves/alice/states/nes/Metroid_slot1.png"; screenshotPath != want {
		t.Errorf("screenshot path = %q, want %q", screenshotPath, want)
	}
}

func TestStateFilePathScopesBySlot(t *testing.T) {
	slot0 := stateFilePath("/saves", "alice", "nes", "Metroid", 0, ".state")
	slot1 := stateFilePath("/saves", "alice", "nes", "Metroid", 1, ".state")
	if slot0 == slot1 {
		t.Errorf("different slots collided on the same path: %q", slot0)
	}
	if want := "/saves/alice/states/nes/Metroid_slot0.state"; slot0 != want {
		t.Errorf("slot 0 path = %q, want %q", slot0, want)
	}
}
