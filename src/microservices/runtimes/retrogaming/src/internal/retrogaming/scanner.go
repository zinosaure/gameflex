// Package retrogaming scans a ROMs folder tree and exposes it as metadata —
// no server-side emulation. The frontend decides how to play a game (WASM
// core via EmulatorJS for retro systems, native streaming for "nextgen"
// systems) once it has the file(s) served by this package.
package retrogaming

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ConsoleSummary is one top-level folder under the ROMs root (its name is
// the console id, matching gameflex-consoles' registry).
type ConsoleSummary struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// RomEntry is one playable game under a console folder. A game that ships as
// multiple files (e.g. a .cue + several .bin tracks) is stored as its own
// subfolder — in that case Title is the FOLDER name, not any file inside
// it, per the convention this package was built around.
//
// Reference is the catalog's stable UUID (see catalog.go's deriveRomID) —
// what gets passed back to ServeRom/handleStart/history as the
// identifier. Title is the human-readable display name (what Reference
// itself used to BE, before the UUID switch) — added so the frontend still
// has something to show a user besides a raw UUID.
type RomEntry struct {
	Reference string   `json:"reference"`
	Title     string   `json:"title"`
	Files     []string `json:"files"`
	SizeBytes int64    `json:"sizeBytes"`
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

// safeJoin joins root with the given untrusted path segments and rejects
// anything that would escape root (path traversal via ".." or an absolute
// segment) — segments come straight from URL parameters.
func safeJoin(root string, segments ...string) (string, error) {
	joined := filepath.Join(append([]string{root}, segments...)...)
	cleanRoot := filepath.Clean(root)
	if joined != cleanRoot && !strings.HasPrefix(joined, cleanRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path")
	}
	return joined, nil
}

// ScanConsoles lists the console folders present under root and how many
// games (files or subfolders) each one contains.
func ScanConsoles(root string) ([]ConsoleSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []ConsoleSummary{}, nil
		}
		return nil, err
	}

	summaries := make([]ConsoleSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || isHidden(entry.Name()) {
			continue
		}
		children, err := os.ReadDir(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		count := 0
		for _, c := range children {
			if !isHidden(c.Name()) {
				count++
			}
		}
		summaries = append(summaries, ConsoleSummary{ID: entry.Name(), Count: count})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}

// scanGameFolder reads one multi-file game's folder (e.g. a .cue + several
// .bin tracks) — the RomEntry's Reference field is repurposed here as a
// plain scratch holder for the folder name; catalog.go's scanConsoleEntries
// (the only caller) reads it back out into Title, deriving the real
// (Reference =) id itself afterward.
func scanGameFolder(consoleDir, folderName string) (RomEntry, error) {
	folderPath := filepath.Join(consoleDir, folderName)
	files, err := os.ReadDir(folderPath)
	if err != nil {
		return RomEntry{}, err
	}

	rom := RomEntry{Reference: folderName}
	for _, f := range files {
		if f.IsDir() || isHidden(f.Name()) {
			continue
		}
		info, err := f.Info()
		if err != nil {
			continue
		}
		rom.Files = append(rom.Files, f.Name())
		rom.SizeBytes += info.Size()
	}
	sort.Strings(rom.Files)
	return rom, nil
}
