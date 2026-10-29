// Package retrogaming scans a ROMs folder tree and exposes it as metadata —
// no server-side emulation. The frontend decides how to play a game (WASM
// core via EmulatorJS for retro systems, native streaming for "nextgen"
// systems) once it has the file(s) served by this package.
package retro_files

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gameflex/storage"
	"gameflex/storage/gamestatus"
)

// ConsoleSummary is one top-level folder under the ROMs root (its name is
// the console id, matching gameflex-consoles' registry).
//
// Le compte ne fait plus le menu, qui liste toutes les consoles actives
// (2026-10-11) : il dit aux écrans de stockage quelles consoles ont des jeux
// (`activeConsoleIds`). `FederatedCount`, qui ne servait qu'au menu, est
// parti avec lui.
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
	// Folder dit que ce jeu est un DOSSIER, pas un fichier.
	//
	// `Files` ne le dit pas : il ne liste que la racine, et un jeu PSP ou PS2
	// rangé dans son dossier n'y a qu'un `.iso` — ce que donne aussi un
	// fichier libre. Les deux ne voyagent pourtant pas pareil : un pair sert
	// un dossier ZIPPÉ (`serveDirectoryAsZip`), et c'est ce champ, publié dans
	// son catalogue, qui dit à l'autre s'il doit le déplier (2026-10-10).
	Folder bool `json:"folder,omitempty"`

	// ── Origine (chantier 6) ────────────────────────────────────────
	// Vides pour un jeu d'ici, et c'est le cas par défaut : un jeu sans
	// mention est un jeu local. Remplis pour un jeu qui vient d'un autre
	// serveur, afin que la liste puisse le dire — et surtout pour que
	// personne n'essaie de le lancer comme s'il était là.

	// Source est le nom à afficher : le hostname du serveur distant.
	Source string `json:"source,omitempty"`
	// Kind est la nature de cette origine (voir storage.Kind*).
	Kind string `json:"kind,omitempty"`
	// Identity est le nom du jeton, donc sous quel nom CE serveur nous
	// connaît. Sert à reconnaître quel accès fait apparaître ce jeu.
	Identity string `json:"identity,omitempty"`
	// ServerID désigne l'entrée du registre d'où il vient.
	ServerID int64 `json:"serverId,omitempty"`
	// MountID désigne le PARTAGE MONTÉ d'où il vient — le pendant de
	// ServerID pour une ressource distante (SMB, WebDAV, kDrive).
	//
	// Il existe parce que `Kind` seul ne suffit pas : il sait dire « ça vient
	// d'un kDrive » sans jamais dire DUQUEL, et deux kDrive enregistrés
	// étaient donc indistinguables côté page (#163). L'identifiant était bien
	// là, mais seulement enfoui dans la référence (`mnt<id>:<chemin>`), que la
	// page ne décode jamais — mesuré le 2026-09-17 : `grep mnt` sur toute la
	// webapp ne rendait rien.
	//
	// Un champ plutôt qu'une chaîne à découper, pour suivre ce que ServerID
	// fait déjà juste au-dessus : la référence reste un LOCALISATEUR opaque
	// (elle sert à aller lire le fichier), l'origine se déclare.
	MountID int64 `json:"mountId,omitempty"`
	// Disk — le disque interne où est ce jeu (chantier 23), vide pour la
	// bibliothèque principale. Un jeu de disque interne est un jeu D'ICI : il
	// n'a ni Kind ni Source, il se joue ici comme les autres ; ce champ ne
	// sert qu'à dire où il est.
	Disk string `json:"disk,omitempty"`

	// Status — le statut du jeu (chantier 30) : le badge de la liste et le
	// détail de la fiche, calculé par gamestatus.Resolve. Absent pour un jeu
	// d'une ressource distante, que personne ne signale.
	Status *gamestatus.View `json:"status,omitempty"`
	// Engine — le moteur du jeu (storage.GameEngine, chantier 36), absent
	// s'il n'est pas reconnu.
	Engine string `json:"engine,omitempty"`
}

func IsHidden(name string) bool {
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
		if !entry.IsDir() || IsHidden(entry.Name()) {
			continue
		}
		children, err := os.ReadDir(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		count := 0
		for _, c := range children {
			if !IsHidden(c.Name()) {
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
func scanGameFolder(consoleID, consoleDir, folderName string) (RomEntry, error) {
	folderPath := filepath.Join(consoleDir, folderName)
	files, err := os.ReadDir(folderPath)
	if err != nil {
		return RomEntry{}, err
	}

	// Files starts empty, not nil: an EMPTY folder still serializes as
	// "files": [] — a null list crashes the game list (the bench's
	// "System Volume Information" trap, 2026-09-03).
	rom := RomEntry{Reference: folderName, Files: []string{}}
	for _, f := range files {
		if IsHidden(f.Name()) {
			continue
		}
		// UN JEU PEUT ÊTRE ARBORESCENT. Le premier cas réel est un jeu PS5
		// (2026-09-10) : `Media/`, `sce_sys/` et `sce_module/` portent
		// l'essentiel des 3,2 Go, la racine ne tient que l'`eboot.bin`.
		//
		// `Files` ne liste toujours QUE la racine — c'est ce qu'on voit en
		// ouvrant le dossier, ce qui nomme le téléchargement, et ce qui
		// décide de l'icône « un fichier / une collection ». Mais la TAILLE
		// doit être celle du jeu entier, sinon la bibliothèque annonce
		// 30 Mo pour 3,2 Go.
		if f.IsDir() {
			rom.SizeBytes += treeSize(filepath.Join(folderPath, f.Name()))
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
	rom.Engine = storage.GameEngine(consoleID, storageEntries(files), func(sub string) ([]storage.Entry, error) {
		entries, err := os.ReadDir(filepath.Join(folderPath, sub))

		return storageEntries(entries), err
	})

	return rom, nil
}

// storageEntries — un dossier lu sur le disque, à la forme que la règle
// partagée du moteur lit (storage.Entry : le nom, et dossier ou non).
func storageEntries(entries []os.DirEntry) []storage.Entry {
	out := make([]storage.Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, storage.Entry{Name: entry.Name(), Dir: entry.IsDir()})
	}

	return out
}

// treeSize additionne les fichiers d'une arborescence. Best-effort : un
// sous-dossier illisible compte pour zéro plutôt que de faire disparaître le
// jeu de la liste — une taille approximative reste jouable, une entrée
// absente non.
func treeSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if IsHidden(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if IsHidden(entry.Name()) {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
