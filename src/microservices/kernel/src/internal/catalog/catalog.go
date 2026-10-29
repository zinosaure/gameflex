// Package catalog scans the console games volume (.xci/.nsp) with a small
// cache, mirroring the reference orchestrator of the DockGaming POC. Game
// identifiers are deterministic, and the retro catalog's own references
// (UUIDv5 of the console and the ON-DISK NAME, extension included — not the
// title, since #167) resolve here too — the frontend can pass either
// the title it displays or that reference.
package catalog

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"gameflex/consoles"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gameflex/storage"
)

type Game struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Folder  string `json:"folder"`
	RelPath string `json:"rel_path"`
}

type Catalog struct {
	console  string
	gamesDir string
	scan     func(console, gamesDir string) []Game

	mu          sync.Mutex
	cachedAt    time.Time
	cachedGames []Game
}

const cacheTTL = 5 * time.Second

// New builds the catalog for one console volume. All three consoles share
// the same scan — the retrogaming-style layout (files and folder-format
// dumps side by side, some nested deeper than others) is identical, only
// the volume path differs.
func New(console, gamesDir string) *Catalog {
	return &Catalog{console: console, gamesDir: gamesDir, scan: scan}
}

// NewKeyTest builds the keytest catalog: ONE fixed entry — the console has
// no games, the adapter ignores the path (see the streamex keytest console).
func NewKeyTest() *Catalog {
	return &Catalog{console: "keytest", scan: func(_, _ string) []Game {
		return []Game{{ID: "keytest", Title: "Key test", Folder: "", RelPath: "keytest"}}
	}}
}

func (catalog *Catalog) List() []Game {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()

	if time.Since(catalog.cachedAt) > cacheTTL {
		catalog.cachedGames = catalog.scan(catalog.console, catalog.gamesDir)
		catalog.cachedAt = time.Now()
	}

	return catalog.cachedGames
}

// Resolve finds a game from whatever the frontend holds: the display title
// (folder name or file stem), the retro catalog's reference, or this
// catalog's own id. Matching is case-insensitive.
// Path — où le kernel voit ce jeu : son dossier, ou son fichier.
func (catalog *Catalog) Path(game Game) string {
	return filepath.Join(catalog.gamesDir, game.RelPath)
}

func (catalog *Catalog) Resolve(identifier string) (Game, error) {
	needle := strings.ToLower(strings.TrimSpace(identifier))
	if needle == "" {
		return Game{}, fmt.Errorf("game is required")
	}

	for _, game := range catalog.List() {
		for _, candidate := range catalog.candidatesFor(game) {
			if candidate == needle {
				return game, nil
			}
		}
	}

	return Game{}, fmt.Errorf("game not found: %s", identifier)
}

func (catalog *Catalog) candidatesFor(game Game) []string {
	// UNE SEULE FORME DÉRIVÉE, celle du nom sur le disque (2026-09-18).
	//
	// Il y en avait deux — le titre et le dossier — parce que l'identifiant
	// rétro dérivait du TITRE, lequel perd l'extension d'un fichier libre. Il
	// dérive désormais du nom de l'entrée, et pour un dump en dossier
	// `Base(RelPath)` EST le nom du dossier : la seconde forme n'apporte donc
	// plus rien. Les trois premières restent, elles : ce sont des
	// identifiants en clair, que des hôtes envoient tels quels.
	return []string{
		strings.ToLower(game.Title),
		strings.ToLower(game.Folder),
		strings.ToLower(game.ID),
		retroReference(catalog.console, filepath.Base(game.RelPath)),
	}
}

// retroReference — the retro catalog's reference, by the rule both kernels
// share (`storage.RomID`, chantier 14): the same namespace is what makes a
// reference minted by the retro scanner resolve here without any shared
// database between the two services.
func retroReference(consoleID, entryName string) string {
	return storage.RomID(consoleID, entryName)
}

// scan is shared by switch/ps3/xbox: files with a known game extension
// anywhere under the volume, plus folder-format dumps (a folder holding an
// EBOOT.BIN or at least one .iso is its own game) — the same mixed layout
// the retrogaming roms use.
func scan(console, gamesDir string) []Game {
	games := make([]Game, 0)
	seenDirs := make(map[string]bool)

	err := filepath.WalkDir(gamesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			// UN DOSSIER CACHÉ N'EST PAS UN JEU, comme pour la liste
			// (storage.ScanConvention) : un `.extraction` en cours, plein d'un
			// jeu PS5 qui s'archive (#431), y passait pour un jeu de plus.
			if path != gamesDir && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}

		relativePath, err := filepath.Rel(gamesDir, path)
		if err != nil || relativePath == "." {
			return nil
		}

		name := entry.Name()

		// Folder-format dumps: EBOOT.BIN (PS3) or a redump-style folder
		// holding its .iso files (Xbox) is listed as ONE game.
		if strings.EqualFold(name, "EBOOT.BIN") || strings.EqualFold(filepath.Ext(name), ".iso") {
			parent := filepath.Dir(relativePath)
			if parent != "." && !seenDirs[parent] {
				seenDirs[parent] = true
				games = append(games, Game{
					ID:      shortID(parent),
					Title:   filepath.Base(parent),
					Folder:  filepath.Base(parent),
					RelPath: parent,
				})
			}
			if strings.EqualFold(name, "EBOOT.BIN") {
				return nil
			}
		}

		if !isGameFile(console, name) {
			return nil
		}

		games = append(games, Game{
			ID:      shortID(relativePath),
			Title:   strings.TrimSuffix(name, filepath.Ext(name)),
			Folder:  filepath.Base(filepath.Dir(path)),
			RelPath: relativePath,
		})
		return nil
	})
	if err != nil {
		return games
	}

	sort.Slice(games, func(i, j int) bool { return games[i].RelPath < games[j].RelPath })
	return games
}

// isGameFile — LE REGISTRE D'ABORD, la liste en dur seulement à défaut.
//
// Cette liste était la seule vérité, et elle ne connaissait que quatre
// extensions. Une Wii U dont les jeux sont en .wux répondait donc « game not
// found » alors que le catalogue balayait le bon dossier : rien ne disait que
// le refus venait d'ici (2026-09-08). Le registre partagé porte déjà les
// extensions par console — quand il en déclare, ce sont elles qui décident.
//
// Le repli n'est pas de la timidité : switch/ps3/xbox n'en déclarent
// aucune aujourd'hui, et consoles.Accepts() traite « aucune extension
// déclarée » comme « tout est accepté » — ce qui, ici, listerait le
// README.TXT posé à côté d'un jeu. Elles gardent donc exactement le
// comportement qu'elles avaient, jusqu'au jour où on leur écrira leurs
// extensions.
func isGameFile(console, name string) bool {
	if len(consoles.ExtensionsFor(console)) > 0 {
		return consoles.Accepts(console, name)
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	for _, known := range fallbackExtensions {
		if ext == known {
			return true
		}
	}
	return false
}

// fallbackExtensions — les formats des consoles qui n'en déclarent pas au
// registre (voir isGameFile).
var fallbackExtensions = []string{"xci", "nsp", "iso", "pkg"}

// Formats — les fichiers que le catalogue lance pour cette console, à dire à
// qui lance un dossier qui n'en contient aucun : « .elf, .zar ».
func Formats(console string) string {
	extensions := consoles.ExtensionsFor(console)
	if len(extensions) == 0 {
		extensions = fallbackExtensions
	}

	return "." + strings.Join(extensions, ", .")
}

// HoldsFolder — un dossier de ce nom (sans casse) est-il rangé à la racine de
// la console ? Quand Resolve échoue, il distingue le jeu absent du jeu présent
// dont aucun fichier ne se lance — un paquet PS5, que Kyty ne lit pas (#431).
func (catalog *Catalog) HoldsFolder(identifier string) bool {
	entries, err := os.ReadDir(catalog.gamesDir)
	if err != nil {
		return false
	}
	name := strings.TrimSpace(identifier)
	for _, entry := range entries {
		if entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			return true
		}
	}

	return false
}

// shortID is deterministic (first 8 hex chars of the sha1 of the relative
// path) so a launch request survives a cache refresh — same scheme as the
// POC's reference orchestrator.
func shortID(relativePath string) string {
	sum := sha1.Sum([]byte(relativePath))
	return hex.EncodeToString(sum[:4])
}
