// Package installs — ce que le premier lancement d'un jeu installe, une fois
// pour tous les comptes (chantier 22), rangé dans le cache du dossier :
// `cache/installs/<console>/…` (#448, le porteur, 2026-10-24 : « ce serait pas
// mieux de mettre le gameflex/installs dans gameflex/cache/installs ? […] ça
// évitera de grossir le stockage sans savoir d'où ça vient »).
//
// Tout s'y refait : un jeu PS4 se redéballe depuis son .pkg, le firmware de la
// PS3 se réinstalle, un jeu PS3 réinstalle ses données. Le serveur et le
// téléphone rangent ici de la même façon, et l'écran Réglages › Stockage ›
// Cache le montre et le vide, entrée par entrée.
package installs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Dir — `<root>/cache/installs/<console>[/<sous-chemin>…]`.
func Dir(root, consoleID string, sub ...string) string {
	return filepath.Join(append([]string{root, "cache", "installs", consoleID}, sub...)...)
}

// Les natures d'une entrée, ce que l'écran en dit.
const (
	KindUnpacked    = "unpacked"    // un jeu PS4 déballé de son .pkg
	KindFirmware    = "firmware"    // le firmware de la PS3 (dev_flash)
	KindGameData    = "game-data"   // un jeu PS3 installé, ou ses données installées par lui
	KindCompilation = "compilation" // les modules compilés par RPCS3
	KindShaders     = "shaders"     // le cache de shaders de Mesa
	KindOther       = "other"
)

// Entry — une entrée du cache d'une console.
type Entry struct {
	// Name — son chemin sous `cache/installs/<console>/`, tel que la suppression
	// le reçoit (`dev_flash`, `game/NPUB30423`, le nom d'un .pkg sans extension).
	Name string `json:"name"`
	// Label — ce qu'on en lit : le titre d'un jeu PS3 (son PARAM.SFO), sinon le nom.
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"sizeBytes"`
}

// Console — les entrées d'une console, et ce qu'elles pèsent ensemble.
type Console struct {
	ConsoleID string  `json:"consoleId"`
	SizeBytes int64   `json:"sizeBytes"`
	Entries   []Entry `json:"entries"`
}

// List — tout ce que `cache/installs/` contient, console par console. Un
// dossier absent n'est pas une erreur : rien n'a encore été installé.
func List(root string) ([]Console, error) {
	base := filepath.Join(root, "cache", "installs")
	consoles, err := os.ReadDir(base)
	if errors.Is(err, fs.ErrNotExist) {
		return []Console{}, nil
	}
	if err != nil {
		return nil, err
	}

	listed := []Console{}
	for _, console := range consoles {
		if !console.IsDir() || hidden(console.Name()) {
			continue
		}
		entries := entriesOf(base, console.Name())
		if len(entries) == 0 {
			continue
		}
		item := Console{ConsoleID: console.Name(), Entries: entries}
		for _, entry := range entries {
			item.SizeBytes += entry.SizeBytes
		}
		listed = append(listed, item)
	}
	sort.Slice(listed, func(i, j int) bool { return listed[i].ConsoleID < listed[j].ConsoleID })

	return listed, nil
}

// entriesOf — les entrées d'une console. La PS3 range par usage (`dev_flash`,
// `game/<jeu>`, `cache`, `mesa_shader_cache`) ; ailleurs, chaque dossier est un
// jeu (la PS4 : `<nom du .pkg>`). Les verrous et les déballages inachevés
// (`.lock`, `.partial`) ne sont pas des entrées : ils partent avec la leur.
func entriesOf(base, consoleID string) []Entry {
	dir := filepath.Join(base, consoleID)
	children, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	entries := []Entry{}
	add := func(name, kind, label string) {
		size := treeSize(filepath.Join(dir, name))
		if size == 0 {
			return // un dossier vide n'occupe rien : rien à montrer
		}
		if label == "" {
			label = name
		}
		entries = append(entries, Entry{Name: name, Label: label, Kind: kind, SizeBytes: size})
	}
	for _, child := range children {
		name := child.Name()
		if !child.IsDir() || hidden(name) || strings.HasSuffix(name, ".partial") {
			continue
		}
		if consoleID != "ps3" {
			kind := KindOther
			if consoleID == "ps4" {
				kind = KindUnpacked
			}
			add(name, kind, "")

			continue
		}
		switch name {
		case "dev_flash":
			add(name, KindFirmware, "")
		case "cache":
			add(name, KindCompilation, "")
		case "mesa_shader_cache":
			add(name, KindShaders, "")
		case "game":
			games, _ := os.ReadDir(filepath.Join(dir, name))
			for _, game := range games {
				if game.IsDir() && !hidden(game.Name()) && !strings.HasPrefix(game.Name(), "＄") {
					add("game/"+game.Name(), KindGameData, SFOTitle(filepath.Join(dir, name, game.Name(), "PARAM.SFO")))
				}
			}
		default:
			add(name, KindOther, "")
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	return entries
}

// ErrUnknown — le nom ne désigne aucune entrée de ce cache.
var ErrUnknown = errors.New("entrée inconnue")

// Remove — retire une entrée. Le nom doit être l'une de celles que List rend :
// rien d'autre ne s'efface par ici, et rien hors du dossier. L'entrée est
// d'abord renommée en dossier caché (instantané, elle quitte la liste), puis
// effacée en fond : un jeu déballé pèse jusqu'à 145 Go. Son verrou et un
// déballage inachevé partent avec elle.
func Remove(root, consoleID, name string) error {
	consoles, err := List(root)
	if err != nil {
		return err
	}
	known := false
	for _, console := range consoles {
		if console.ConsoleID != consoleID {
			continue
		}
		for _, entry := range console.Entries {
			if entry.Name == name {
				known = true
			}
		}
	}
	if !known {
		return ErrUnknown
	}

	path := Dir(root, consoleID, filepath.FromSlash(name))
	doomed := filepath.Join(filepath.Dir(path), fmt.Sprintf(".deleting-%d-%s", time.Now().UnixNano(), filepath.Base(path)))
	if err := os.Rename(path, doomed); err != nil {
		// Un renommage refusé (le stockage partagé d'Android, chantier 22) :
		// effacer sur place, en attendant la fin.
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	} else {
		go os.RemoveAll(doomed) //nolint:errcheck
	}
	for _, sibling := range []string{path + ".lock", path + ".partial"} {
		_ = os.RemoveAll(sibling)
	}

	return nil
}

// Purge — les effacements qu'un arrêt a interrompus (`.deleting-…`), à reprendre.
func Purge(root string) {
	_ = filepath.WalkDir(filepath.Join(root, "cache", "installs"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil //nolint:nilerr
		}
		if strings.HasPrefix(entry.Name(), ".deleting-") {
			_ = os.RemoveAll(path)

			return fs.SkipDir
		}

		return nil
	})
}

func hidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

// treeSize — ce que pèse un dossier, fichiers réguliers seulement : un lien ne
// compte pas (le téléphone en pose vers ce dossier, pas depuis lui).
func treeSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil //nolint:nilerr
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}

		return nil
	})

	return total
}

// SFOTitle — le TITLE d'un PARAM.SFO (PS3, et le param.sfo d'une sauvegarde PS4), vide s'il
// n'y en a pas.
func SFOTitle(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 20 || !bytes.Equal(data[:4], []byte("\x00PSF")) {
		return ""
	}
	keys := binary.LittleEndian.Uint32(data[8:12])
	values := binary.LittleEndian.Uint32(data[12:16])
	count := binary.LittleEndian.Uint32(data[16:20])
	for index := uint32(0); index < count; index++ {
		at := 20 + 16*index
		if int(at+16) > len(data) {
			break
		}
		keyOffset := uint32(binary.LittleEndian.Uint16(data[at : at+2]))
		length := binary.LittleEndian.Uint32(data[at+4 : at+8])
		valueOffset := binary.LittleEndian.Uint32(data[at+12 : at+16])
		start := keys + keyOffset
		if int(start) >= len(data) {
			continue
		}
		end := bytes.IndexByte(data[start:], 0)
		if end < 0 || string(data[start:start+uint32(end)]) != "TITLE" {
			continue
		}
		from, to := values+valueOffset, values+valueOffset+length
		if int(to) > len(data) {
			return ""
		}

		return strings.TrimSpace(string(bytes.TrimRight(data[from:to], "\x00")))
	}

	return ""
}
