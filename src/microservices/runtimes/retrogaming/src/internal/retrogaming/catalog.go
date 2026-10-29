package retrogaming

// The ROM catalog — an in-memory cache on top of scanner.go's raw disk
// scanning, giving every game a STABLE reference instead of the raw
// filename ScanConsole/ResolveRom used to hand out directly. A raw filename
// as the identifier used in URLs (ServeRom) and history was
// fragile (percent-encoding gymnastics for special characters — see
// urlParam's doc) and coupled those to the exact on-disk name forever.
//
// deriveRomID is DETERMINISTIC (UUID v5, hashed from consoleID + the entry's
// NAME ON DISK) rather than random: the same file always derives the same
// id across rescans, so a rescan (e.g. after adding new ROMs) doesn't
// invalidate existing history/session references — only an
// actual rename or delete does, exactly like the old raw-filename
// references would have.
//
// LE NOM SUR LE DISQUE, ET NON LE TITRE AFFICHÉ (2026-09-18) — la même règle
// que le kernel, mot pour mot : c'est CE scan-ci qui retrouve un jeu au
// lancement, et deux dérivations qui divergent, c'est un jeu listé qui ne se
// lance pas. Voir `kernel/.../retro_files/catalog.go`.
//
// Unlike scanner.go's functions (still called fresh on every request, for
// ScanConsoles' folder counts, which don't need stable ids at all), this
// cache is only rebuilt on an explicit RescanCatalog call — the first one
// lazily on first use, subsequent ones via the admin rescan route
// (handlers.go's RescanRoms).
import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gameflex/consoles"
	"github.com/google/uuid"
)

// romNamespace is an arbitrary fixed UUID used only as the namespace input
// to each ROM's deterministic id below — has no meaning of its own, just
// needs to stay constant across restarts.
var romNamespace = uuid.MustParse("2f6b8f2e-8f0a-4e9d-9a4f-6a2f8b6f2f20")

func deriveRomID(consoleID, entryName string) string {
	return uuid.NewSHA1(romNamespace, []byte(consoleID+"/"+entryName)).String()
}

// catalogEntry is the cached, per-game record — RomEntry (scanner.go) is the
// JSON shape ListRoms/ServeRom expose, derived from this one request at a
// time rather than stored directly, so the wire format can keep Reference
// as the field name (now holding ID, not Title) without this package's
// internals also being named that confusingly.
type catalogEntry struct {
	ID        string
	ConsoleID string
	Title     string // human display name — what Reference used to BE
	// Path — le nom de l'entrée SUR LE DISQUE (dossier, ou fichier AVEC son
	// extension) : c'est lui qui dérive l'identifiant, et non `Title`.
	Path      string
	Files     []string
	SizeBytes int64
	// IsDir — whether this entry is a FOLDER (Files lives inside
	// {consoleDir}/{Title}/) or a loose file directly inside {consoleDir}
	// (Files[0] IS the file, joined straight onto consoleDir). Set by
	// scanConsoleEntries, read by ResolveCatalogRom — len(Files) alone
	// can't distinguish these two cases: a folder containing exactly ONE
	// file (a common ROM layout — one .iso per named folder) produces the
	// exact same len(Files)==1 a genuinely loose file does, which used to
	// make ResolveCatalogRom join Files[0] onto consoleDir directly for
	// BOTH cases — wrong (and a real "file not found" bug) for the folder
	// one, since that file actually lives one directory deeper.
	IsDir bool
}

type catalogState struct {
	mu        sync.RWMutex
	byID      map[string]catalogEntry
	byConsole map[string][]string // consoleID -> ids, in Title-sorted order
	scanned   bool
	lastScan  time.Time
}

var catalog = &catalogState{
	byID:      make(map[string]catalogEntry),
	byConsole: make(map[string][]string),
}

// RescanCatalog rebuilds the entire in-memory catalog from disk — the only
// place a full directory walk happens (aside from ScanConsoles' own folder
// counts, which are unrelated and stay request-time). Called lazily on
// first use (ensureCatalogScanned) and explicitly by the admin rescan route.
func RescanCatalog(root string) error {
	consoleSummaries, err := ScanConsoles(root)
	if err != nil {
		return err
	}

	byID := make(map[string]catalogEntry, len(catalog.byID))
	byConsole := make(map[string][]string, len(consoleSummaries))

	for _, cs := range consoleSummaries {
		entries, err := scanConsoleEntries(root, cs.ID)
		if err != nil {
			continue
		}
		ids := make([]string, 0, len(entries))
		for _, e := range entries {
			e.ID = deriveRomID(cs.ID, e.Path)
			e.ConsoleID = cs.ID
			byID[e.ID] = e
			ids = append(ids, e.ID)
		}
		byConsole[cs.ID] = ids
	}

	catalog.mu.Lock()
	catalog.byID = byID
	catalog.byConsole = byConsole
	catalog.scanned = true
	catalog.lastScan = time.Now()
	catalog.mu.Unlock()
	return nil
}

// ensureCatalogScanned scans once, lazily, on whichever request happens to
// need the catalog first — everything after that reads the cache until an
// explicit rescan. Best-effort: a scan error just leaves the catalog empty
// (same observable behavior scanner.go's functions always had for a missing/
// unreadable root) rather than failing the triggering request.
func ensureCatalogScanned(root string) {
	catalog.mu.RLock()
	scanned := catalog.scanned
	catalog.mu.RUnlock()
	if scanned {
		return
	}
	RescanCatalog(root)
}

// ResolveCatalogRom translates a catalog id back to its on-disk path — used by
// resolveGameFile (stream_session.go), the only thing the runtime asks its
// catalog. Directory case (multi-file game): Files holds every file inside
// consoleID/Title/.
func ResolveCatalogRom(root, consoleID, id string) (path string, isDir bool, err error) {
	ensureCatalogScanned(root)

	catalog.mu.RLock()
	e, ok := catalog.byID[id]
	catalog.mu.RUnlock()
	if !ok || e.ConsoleID != consoleID {
		return "", false, os.ErrNotExist
	}

	consoleDir, err := safeJoin(root, consoleID)
	if err != nil {
		return "", false, err
	}

	if e.IsDir {
		// Folder-based game (one file or several) — Title IS the folder
		// name (see scanConsoleEntries), Files live inside it.
		folderPath, err := safeJoin(root, consoleID, e.Title)
		if err != nil {
			return "", false, err
		}
		return folderPath, true, nil
	}
	if len(e.Files) == 1 {
		return filepath.Join(consoleDir, e.Files[0]), false, nil
	}
	return "", false, os.ErrNotExist
}

// scanConsoleEntries lists every game of one console — a loose file, or a
// folder holding a multi-file game — as catalogEntry (ID/ConsoleID filled in
// by the caller). It must derive titles EXACTLY like the kernel's retro_files
// copy: the title is what the reference is computed from, and it's the kernel
// that hands the reference out (see TestReferencesMatchTheSharedGoldenValue).
func scanConsoleEntries(root, consoleID string) ([]catalogEntry, error) {
	consoleDir, err := safeJoin(root, consoleID)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(consoleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []catalogEntry{}, nil
		}
		return nil, err
	}

	out := make([]catalogEntry, 0, len(entries))
	for _, entry := range entries {
		if isHidden(entry.Name()) {
			continue
		}
		if entry.IsDir() {
			rom, err := scanGameFolder(consoleDir, entry.Name())
			if err != nil {
				continue
			}
			out = append(out, catalogEntry{Title: rom.Reference, Path: entry.Name(), Files: rom.Files, SizeBytes: rom.SizeBytes, IsDir: true})
			continue
		}
		// UN FICHIER QUE LA CONSOLE N'ACCEPTE PAS N'EST PAS UN JEU.
		//
		// LE MÊME FILTRE QUE LE KERNEL, AU MÊME ENDROIT, et ce n'est pas une
		// duplication de confort : c'est CE scan-ci qui retrouve un jeu au
		// lancement, et le kernel qui donne la référence au navigateur. Deux
		// scans qui ne listent pas la même chose, c'est un jeu listé qui ne se
		// lance pas — la panne même que le test doré partagé surveille.
		//
		// Voir `kernel/.../retro_files/catalog.go` pour ce qui a été mesuré le
		// 2026-09-18 : 39 fichiers listés comme des jeux, et 14 titres `gbc`
		// effondrés sur une seule référence dont le gagnant était un `.sav` ou
		// un `.ogg`.
		if !consoles.Accepts(consoleID, entry.Name()) {
			continue
		}
		title := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, catalogEntry{Title: title, Path: entry.Name(), Files: []string{entry.Name()}, SizeBytes: info.Size(), IsDir: false})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}
