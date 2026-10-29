package retro_files

import (
	"os"
	"path/filepath"
	"testing"
)

// Ces cas vivaient côté runtime, sur une copie de saves.go que plus rien
// n'appelait. La liste des sauvegardes est servie par le kernel
// (GET /api/v1/retrogaming/savestate/user), dont la copie n'avait, elle, aucun
// test. Portés le 2026-09-14, sur la signature du kernel : ici stateFilePath
// rend une erreur, parce que la référence arrive d'une requête.

func testSavePath(t *testing.T, savesRoot, username, consoleID, reference string, slot int, ext string) string {
	t.Helper()
	path, err := stateFilePath(savesRoot, username, consoleID, reference, slot, ext)
	if err != nil {
		t.Fatalf("stateFilePath : %v", err)
	}
	return path
}

func testWriteSave(t *testing.T, savesRoot, username, consoleID, reference string, slot int, ext string) {
	t.Helper()
	path := testSavePath(t, savesRoot, username, consoleID, reference, slot, ext)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll : %v", err)
	}
	if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
		t.Fatalf("WriteFile : %v", err)
	}
}

func TestListUserSaveStatesEmptyWhenNoSavesDir(t *testing.T) {
	resetCatalog()
	entries, err := ListUserSaveStates("", t.TempDir(), "alice")
	if err != nil {
		t.Fatalf("ListUserSaveStates : %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("attendu aucune entrée, obtenu %d", len(entries))
	}
}

func TestListUserSaveStatesAcrossGamesAndConsoles(t *testing.T) {
	resetCatalog()
	savesRoot := t.TempDir()
	metroid, ff7 := deriveRomID("nes", "Metroid"), deriveRomID("ps1", "Final Fantasy VII")
	testWriteSave(t, savesRoot, "alice", "nes", metroid, 0, ".state")
	testWriteSave(t, savesRoot, "alice", "nes", metroid, 1, ".state")
	testWriteSave(t, savesRoot, "alice", "ps1", ff7, 0, ".state")
	// Un autre utilisateur : rien ne doit fuiter dans la liste d'alice.
	testWriteSave(t, savesRoot, "bob", "nes", metroid, 0, ".state")

	entries, err := ListUserSaveStates("", savesRoot, "alice")
	if err != nil {
		t.Fatalf("ListUserSaveStates : %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("attendu 3 entrées, obtenu %d : %+v", len(entries), entries)
	}
	for _, e := range entries {
		// Aucun catalogue joignable : le titre retombe sur la référence plutôt
		// que de revenir vide.
		if e.Title == "" || e.Title != e.Reference {
			t.Errorf("entrée %+v : le titre doit retomber sur la référence", e)
		}
	}
}

func TestListUserSaveStatesIgnoresNonStateFiles(t *testing.T) {
	resetCatalog()
	savesRoot := t.TempDir()
	metroid := deriveRomID("nes", "Metroid")
	testWriteSave(t, savesRoot, "alice", "nes", metroid, 0, ".state")
	// La capture posée à côté ne compte pas pour une seconde entrée.
	testWriteSave(t, savesRoot, "alice", "nes", metroid, 0, ".png")

	entries, err := ListUserSaveStates("", savesRoot, "alice")
	if err != nil {
		t.Fatalf("ListUserSaveStates : %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("attendu 1 entrée, obtenu %d : %+v", len(entries), entries)
	}
}

func TestRemoveSaveFilesDeletesStateAndScreenshot(t *testing.T) {
	savesRoot := t.TempDir()
	metroid := deriveRomID("nes", "Metroid")
	testWriteSave(t, savesRoot, "alice", "nes", metroid, 2, ".state")
	testWriteSave(t, savesRoot, "alice", "nes", metroid, 2, ".png")

	if err := removeSaveFiles(savesRoot, "alice", "nes", metroid, 2); err != nil {
		t.Fatalf("removeSaveFiles : %v", err)
	}
	for _, ext := range []string{".state", ".png"} {
		if _, err := os.Stat(testSavePath(t, savesRoot, "alice", "nes", metroid, 2, ext)); !os.IsNotExist(err) {
			t.Errorf("%s toujours présent (stat : %v)", ext, err)
		}
	}
}

func TestRemoveSaveFilesMissingSlotIsNotAnError(t *testing.T) {
	if err := removeSaveFiles(t.TempDir(), "alice", "nes", deriveRomID("nes", "Metroid"), 5); err != nil {
		t.Errorf("retirer un emplacement jamais écrit doit être sans effet, obtenu : %v", err)
	}
}

// La console vient de la query : elle ne doit pas faire redescendre le chemin
// chez un autre compte (revue de sécurité du 2026-10-28 — un joueur lisait,
// écrasait et effaçait les sauvegardes de Bob avec `../../bob/states/gba`).
func TestStateFilePathStaysUnderTheAccount(t *testing.T) {
	savesRoot := t.TempDir()
	for _, consoleID := range []string{"../../bob/states/gba", "../bob", "..", "gba/../../bob", "inconnue", ""} {
		if path, err := stateFilePath(savesRoot, "alice", consoleID, "ref", 0, ".state"); err == nil {
			t.Errorf("console %q : %s accepté, refus attendu", consoleID, path)
		}
	}
	for _, username := range []string{"../bob", "..", "a/b", ""} {
		if path, err := stateFilePath(savesRoot, username, "gba", "ref", 0, ".state"); err == nil {
			t.Errorf("compte %q : %s accepté, refus attendu", username, path)
		}
	}

	path := testSavePath(t, savesRoot, "alice", "gba", "ref", 2, ".png")
	if want := filepath.Join(savesRoot, "alice", "states", "gba", "ref_slot2.png"); path != want {
		t.Errorf("chemin %s, attendu %s", path, want)
	}
}
