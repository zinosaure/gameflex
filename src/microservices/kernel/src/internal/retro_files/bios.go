package retro_files

// BIOS selection — some cores (PS1's pcsx_rearmed in particular) fall back
// to an imperfect HLE (High-Level Emulation) BIOS replacement whenever they
// can't find/pick a real BIOS dump, and HLE is well known to be
// incompatible with a meaningful number of real games. The flat BIOS_PATH
// root (see main.go/child.go's biosRoot) passed to libretro.InitEnvPaths as
// systemDir is an ES-DE/Batocera-style tree of PER-CONSOLE subfolders (e.g.
// {BIOS_PATH}/psx/scph5501.bin), not a flat pile of files at the root
// itself — before this file existed, EVERY console's systemDir was that
// flat root regardless, so a core scanning it directly (as pcsx_rearmed
// does) found nothing at all and silently fell back to HLE. Confirmed via
// direct inspection of a real deployment's BIOS_PATH (2026-08-01) — 24 real
// PS1 BIOS files sitting one level deeper than any core would ever look.
import (
	"os"
	"sort"

	"gameflex/consoles"
)

// consoles.BiosDir maps our own consoleId to the actual on-disk BIOS
// subfolder name under BIOS_PATH — deliberately a small, explicit table
// rather than assuming consoleId always matches the folder name (it
// usually doesn't: "ps1" here is "psx" on disk). Only consoles genuinely
// gated by picking a specific BIOS file are listed — everything else keeps
// passing BIOS_PATH's flat root straight through to InitEnvPaths,
// completely unchanged from before this file existed (zero regression risk
// for any console not listed here). PPSSPP in particular deliberately stays
// OUT of this table: it expects its own asset folder ({BIOS_PATH}/PPSSPP/,
// see the project README's BIOS note) directly at that flat root, and
// scoping its systemDir down to a subfolder would break that.
// La table elle-même vit dans shared/consoles depuis le 2026-09-01 : le
// téléphone en a besoin AUSSI (il range désormais ses BIOS par console, et
// un BIOS récupéré par fédération doit atterrir là où le core le cherche).
// Deux tables auraient fini par ne plus dire la même chose.
func biosDirFor(consoleID string) (string, bool) {
	dir := consoles.BiosDir(consoleID)

	return dir, dir != ""
}

// BiosEntry — un BIOS tel que l'écran d'administration le montre. La taille
// vient avec : sur un dump, elle est le premier signe qu'un fichier est
// tronqué, et une liste de noms seuls ne permet pas de le voir.
type BiosEntry struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	// IsFolder — un BIOS PS2 est un dossier (le .bin plus ses compagnons
	// .NVM/.MEC). La taille est alors celle de l'ensemble.
	IsFolder bool `json:"isFolder"`
}

// ListBiosEntries — les mêmes entrées que ListBiosFiles, avec leur taille.
func ListBiosEntries(biosRoot, consoleID string) ([]BiosEntry, error) {
	names, err := ListBiosFiles(biosRoot, consoleID)
	if err != nil {
		return nil, err
	}

	subdir, _ := biosDirFor(consoleID)
	entries := make([]BiosEntry, 0, len(names))
	for _, name := range names {
		entry := BiosEntry{Name: name}
		full, err := safeJoin(biosRoot, subdir, name)
		if err != nil {
			entries = append(entries, entry)

			continue
		}
		if info, err := os.Stat(full); err == nil {
			entry.IsFolder = info.IsDir()
			if info.IsDir() {
				entry.SizeBytes = folderSize(full)
			} else {
				entry.SizeBytes = info.Size()
			}
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// folderSize — la somme des fichiers d'un dossier, sans descendre plus bas
// que nécessaire : un BIOS tient en trois fichiers côte à côte.
func folderSize(dir string) int64 {
	items, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	var total int64
	for _, item := range items {
		if item.IsDir() {
			continue
		}
		if info, err := item.Info(); err == nil {
			total += info.Size()
		}
	}

	return total
}

// ListBiosFiles returns the available BIOS entry names for consoleID — an
// empty (non-nil) slice if this console has no selectable BIOS, either
// because it genuinely doesn't need one or because its bios/<consoleId>/
// folder doesn't exist. Never an error for "nothing to choose from", only for
// a real filesystem problem reading a folder that does exist.
//
// An entry is EITHER a single file (`scph5501.bin`) OR a directory holding
// one BIOS and its companion files (`scph10000/` → .bin + .NVM + .MEC).
// Both shapes are staged identically by resolveBiosDir — flattened into the
// same temp systemDir — so nothing downstream, and no core, ever sees the
// difference. Directories were skipped outright before (2026-08-12), which
// silently hid any folder-shaped dump from the picker.
//
// Les deux formes sont offertes pour TOUTES les consoles. La règle « la PS2 ne
// propose que des dossiers » a été retirée le 2026-09-08 — le volume des BIOS est
// en lecture seule, les compagnons vivent dans un cache côté runtime — et la
// table vide qui la portait encore est partie le 2026-09-14.
func ListBiosFiles(biosRoot, consoleID string) ([]string, error) {
	subdir, ok := biosDirFor(consoleID)
	if !ok {
		return []string{}, nil
	}
	dir, err := safeJoin(biosRoot, subdir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if IsHidden(e.Name()) {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	return files, nil
}

// ResolveBiosFile returns the absolute path of ONE listed BIOS entry, and
// whether it is a folder (chantier 6 — la surface fédérée doit pouvoir
// servir le fichier, pas seulement le nommer).
//
// Le nom accepté est exactement l'un de ceux que ListBiosFiles a rendus,
// et rien d'autre : on relit la liste plutôt que de faire confiance au
// paramètre d'URL. safeJoin refuserait déjà un ".." ; passer par la liste
// refuse EN PLUS un fichier bien réel mais qu'on n'offre pas — un fichier
// caché, par exemple.
func ResolveBiosFile(biosRoot, consoleID, name string) (path string, isDir bool, err error) {
	listed, err := ListBiosFiles(biosRoot, consoleID)
	if err != nil {
		return "", false, err
	}
	found := false
	for _, entry := range listed {
		if entry == name {
			found = true
			break
		}
	}
	if !found {
		return "", false, os.ErrNotExist
	}

	full, err := safeJoin(biosRoot, consoles.BiosDir(consoleID), name)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", false, err
	}
	return full, info.IsDir(), nil
}
