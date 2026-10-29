package listing

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LocalTree — une section rangée par console sur le disque de cette machine :
// sous `<root>/<consoleid>/`, chaque entrée est un élément. Les correctifs
// (`patches/` — d'ordinaire le dossier d'un jeu, qui porte ses packs :
// `wiiu/NewSuperMarioBrosU/CrashFix/`), les BIOS du téléphone (`bios/`). Un
// dossier voyage zippé ; sa taille est celle de son arborescence entière.
func LocalTree(root string) map[string][]Item {
	items := map[string][]Item{}
	consoles, err := os.ReadDir(root)
	if err != nil {
		return items
	}
	for _, console := range consoles {
		if !console.IsDir() || hidden(console.Name()) {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, console.Name()))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if hidden(entry.Name()) {
				continue
			}
			path := filepath.Join(root, console.Name(), entry.Name())
			item := Item{Name: entry.Name(), Dir: entry.IsDir(), Files: []string{}}
			if entry.IsDir() {
				children, _ := os.ReadDir(path)
				for _, child := range children {
					if !hidden(child.Name()) {
						item.Files = append(item.Files, child.Name())
					}
				}
				sort.Strings(item.Files)
				item.Size = treeSize(path)
			} else if info, err := entry.Info(); err == nil {
				item.Files = []string{entry.Name()}
				item.Size = info.Size()
			}
			items[console.Name()] = append(items[console.Name()], item)
		}
	}

	return items
}

// treeSize — le poids d'une arborescence ; un sous-dossier illisible compte
// pour zéro plutôt que de faire disparaître l'élément.
func treeSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != dir && hidden(entry.Name()) {
				return fs.SkipDir
			}

			return nil
		}
		if info, err := entry.Info(); err == nil && !hidden(entry.Name()) {
			total += info.Size()
		}

		return nil
	})

	return total
}

func hidden(name string) bool {
	return strings.HasPrefix(name, ".")
}
