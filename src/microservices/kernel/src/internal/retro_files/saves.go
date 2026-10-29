package retro_files

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// slotFilePattern matches stateFilePath's own naming scheme
// ({safeName}_slot{N}.state) in reverse, to recover (reference, slot) from a
// bare filename when walking a user's saves directory — see
// ListUserSaveStates.
var slotFilePattern = regexp.MustCompile(`^(.*)_slot(\d+)\.state$`)

// userSaveEntry is one save slot, as listed by ListUserSaveStates — flat (not
// pre-grouped by game), the frontend's profile page groups by
// ConsoleID+Reference itself for display.
type userSaveEntry struct {
	ConsoleID string `json:"consoleId"`
	Reference string `json:"reference"`
	// Title — the catalog's human display name for Reference (see
	// catalog.go's deriveRomID doc: Reference is a UUID, not something to
	// show a user directly). Falls back to Reference itself if the catalog
	// has no matching entry (game removed/renamed since the save was made).
	Title     string    `json:"title"`
	Slot      int       `json:"slot"`
	Timestamp time.Time `json:"timestamp"`
}

// lookupCatalogTitle resolves a catalog id (RomEntry.Reference/Title's own
// id, see catalog.go) back to its human display title — same
// ensureCatalogScanned/RLock pattern as ListCatalog/ResolveCatalogRom.
func lookupCatalogTitle(root, id string) (string, bool) {
	catalog := ensureCatalogScanned(root)
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	e, ok := catalog.byID[id]
	return e.Title, ok
}

// ListUserSaveStates walks {savesRoot}/{username}/states/{consoleId}/*.state
// and returns every save slot the user has, across every game/console —
// backs the profile page's save manager (unlike ListSaveSlots, which is
// scoped to one already-known game). romsRoot is only used to resolve each
// entry's display Title via the catalog; a missing/unscanned catalog just
// falls back to the raw reference, it never fails this call.
//
// `otherRoots` — les dossiers de jeux des disques internes (chantier 23) : le
// titre d'un jeu qui n'est que sur l'un d'eux s'y cherche après la
// bibliothèque.
func ListUserSaveStates(romsRoot, savesRoot, username string, otherRoots ...string) ([]userSaveEntry, error) {
	base := filepath.Join(savesRoot, username, "states")
	consoleDirs, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return []userSaveEntry{}, nil
		}
		return nil, err
	}

	var out []userSaveEntry
	for _, cd := range consoleDirs {
		if !cd.IsDir() {
			continue
		}
		consoleID := cd.Name()
		files, err := os.ReadDir(filepath.Join(base, consoleID))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			m := slotFilePattern.FindStringSubmatch(f.Name())
			if m == nil {
				continue
			}
			slot, err := strconv.Atoi(m[2])
			if err != nil {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			reference := m[1]
			title, ok := lookupCatalogTitle(romsRoot, reference)
			for _, root := range otherRoots {
				if ok {
					break
				}
				title, ok = lookupCatalogTitle(root, reference)
			}
			if !ok {
				title = reference
			}
			out = append(out, userSaveEntry{
				ConsoleID: consoleID,
				Reference: reference,
				Title:     title,
				Slot:      slot,
				Timestamp: info.ModTime(),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ConsoleID != out[j].ConsoleID {
			return out[i].ConsoleID < out[j].ConsoleID
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].Slot < out[j].Slot
	})
	return out, nil
}

// removeSaveFiles deletes one slot's .state and its sibling .png screenshot
// (see captureScreenshot) — missing files are not an error, both save-slot
// deletion and whole-game deletion (handlers.go's DeleteSaveSlot/
// DeleteGameSaves) call this per-slot regardless of whether that particular
// slot actually holds anything.
func removeSaveFiles(savesRoot, username, consoleID, reference string, slot int) error {
	for _, ext := range []string{".state", ".png"} {
		path, err := stateFilePath(savesRoot, username, consoleID, reference, slot, ext)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
