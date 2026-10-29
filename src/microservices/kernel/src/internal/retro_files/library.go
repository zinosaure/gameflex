package retro_files

// « Ma bibliothèque » — ce qui est SUR LE DISQUE, lu comme on le lirait avec
// un explorateur de fichiers.
//
// C'est la différence avec le reste de ce paquet, qui répond « donne-moi les
// jeux de la PlayStation » : ici on part des DOSSIERS, et on dit ce qu'on y
// trouve. Un dossier dont le nom ne désigne aucune console reste donc
// affiché — c'est même la seule façon de découvrir qu'on a rangé quelque
// chose au mauvais endroit (demandé le 2026-09-01).
//
// La règle d'attribution diffère d'une section à l'autre, et ce n'est pas une
// incohérence :
//
//	bios   le NOM DU DOSSIER fait la console. Un BIOS n'a pas d'autre
//	       indice — son nom de fichier ne dit rien de qui le réclame.
//	cores  le REGISTRE fait la console, et lui seul : un core sert souvent
//	       plusieurs machines, et son emplacement n'en désigne aucune.
//
// Ce qui ne se rattache à rien sort avec un consoleId vide : l'appelant le
// range sous « Others » plutôt que de le taire.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gameflex/consoles"
)

// LibraryEntry — un élément du stockage, tel que l'écran le liste.
type LibraryEntry struct {
	// ConsoleID est le nom du dossier pour un BIOS, l'identifiant du
	// registre pour un core. Vide quand rien ne le rattache.
	ConsoleID string `json:"consoleId"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	IsFolder  bool   `json:"isFolder"`
	// Scoped — le fichier était-il rangé dans un sous-dossier ? Un core
	// posé à la racine reste parfaitement utilisable ; on le dit sans en
	// faire une erreur.
	Scoped bool `json:"scoped"`
}

// ListBiosTree — tout ce que contient le dossier BIOS, sous-dossier par
// sous-dossier, plus ce qui traîne à la racine.
func ListBiosTree(biosRoot string) ([]LibraryEntry, error) {
	entries, err := os.ReadDir(biosRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []LibraryEntry{}, nil
		}

		return nil, err
	}

	out := make([]LibraryEntry, 0, 64)
	for _, entry := range entries {
		if IsHidden(entry.Name()) {
			continue
		}

		// Un fichier à la racine n'appartient à aucune console : il sort
		// sans rattachement, et l'écran le montre sous « Others ».
		if !entry.IsDir() {
			if info, err := entry.Info(); err == nil {
				out = append(out, LibraryEntry{Name: entry.Name(), SizeBytes: info.Size()})
			}

			continue
		}

		folder := entry.Name()
		children, err := os.ReadDir(filepath.Join(biosRoot, folder))
		if err != nil {
			continue
		}
		for _, child := range children {
			if IsHidden(child.Name()) {
				continue
			}
			item := LibraryEntry{ConsoleID: folder, Name: child.Name(), Scoped: true}
			full := filepath.Join(biosRoot, folder, child.Name())
			if child.IsDir() {
				item.IsFolder = true
				item.SizeBytes = folderSize(full)
			} else if info, err := child.Info(); err == nil {
				item.SizeBytes = info.Size()
			}
			out = append(out, item)
		}
	}

	sortEntries(out)

	return out, nil
}

// ListCoreTree — tous les cores présents, attribués par le REGISTRE.
//
// Un core qui sert deux consoles sort DEUX fois, une par console : c'est ce
// qu'on veut voir en filtrant par console, et le fichier reste le même. Un
// core que le registre ne connaît pas sort une fois, sans rattachement.
func ListCoreTree(coresRoot string) ([]LibraryEntry, error) {
	found := map[string]fileInfo{}

	// La racine, puis chaque sous-dossier : les deux rangements coexistent,
	// et un core rangé par console l'emporte sur le même posé en vrac.
	if err := collectCores(coresRoot, "", found); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(coresRoot)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && !IsHidden(entry.Name()) {
				_ = collectCores(filepath.Join(coresRoot, entry.Name()), entry.Name(), found)
			}
		}
	}

	out := make([]LibraryEntry, 0, len(found))
	for file, info := range found {
		name := coreNameOf(file)
		owners := consoles.ConsolesForCore(name)
		if len(owners) == 0 {
			out = append(out, LibraryEntry{Name: name, SizeBytes: info.size, Scoped: info.scoped})

			continue
		}
		for _, consoleID := range owners {
			out = append(out, LibraryEntry{
				ConsoleID: consoleID, Name: name, SizeBytes: info.size, Scoped: info.scoped,
			})
		}
	}

	sortEntries(out)

	return out, nil
}

type fileInfo struct {
	size   int64
	scoped bool
}

func collectCores(dir, folder string, found map[string]fileInfo) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || IsHidden(entry.Name()) || !strings.HasSuffix(entry.Name(), ".so") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		existing, seen := found[entry.Name()]
		if seen && existing.scoped {
			continue // le rangé par console l'emporte
		}
		found[entry.Name()] = fileInfo{size: info.Size(), scoped: folder != ""}
	}

	return nil
}

// coreNameOf — « fceumm_libretro.so » → « fceumm ». Les variantes du
// buildbot (_android) sont retirées aussi : c'est le même core.
func coreNameOf(file string) string {
	name := strings.TrimSuffix(file, ".so")
	name = strings.TrimSuffix(name, "_libretro_android")

	return strings.TrimSuffix(name, "_libretro")
}

func sortEntries(entries []LibraryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ConsoleID != entries[j].ConsoleID {
			return entries[i].ConsoleID < entries[j].ConsoleID
		}

		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}
