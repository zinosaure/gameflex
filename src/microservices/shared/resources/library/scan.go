package library

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gameflex/consoles"
	"gameflex/storage"
	"gameflex/storage/listing"
)

// LE PARCOURS DES SECTIONS, SUR N'IMPORTE QUELLE SOURCE (chantier 61 : « hormis les jeux tout le reste n'est accessible
// que par disque interne », le porteur, 2026-11-18). Un disque (Local) comme un partage distant (Samba, WebDAV, kDrive,
// FTP, SFTP) sait lister un dossier (storage.Source) : c'est tout ce qu'il faut pour trouver ses cœurs, ses émulateurs,
// ses BIOS et ses correctifs, rangés comme chez GameFlex (cores.go, bios.go, listing.LocalTree). Chaque élément trouvé
// garde son emplacement (Locator), d'où le service le servira.

// Locator — où est un élément dans sa source.
type Locator struct {
	// Path — le fichier, ou le dossier (Dir) qui EST l'élément, relatif à la source.
	Path string `json:"path"`
	Dir  bool   `json:"dir,omitempty"`
	// Android — pour un cœur, sa variante Android, relative à la source.
	Android string `json:"android,omitempty"`
	Size    int64  `json:"size"`
}

// Found — ce qu'une section d'une source porte, par console, et où.
type Found struct {
	Items map[string][]listing.Item `json:"items"`
	Where map[string][]Locator      `json:"where"`
}

func newFound() Found {
	return Found{Items: map[string][]listing.Item{}, Where: map[string][]Locator{}}
}

func (found *Found) add(console string, item listing.Item, where Locator) {
	found.Items[console] = append(found.Items[console], item)
	found.Where[console] = append(found.Where[console], where)
}

func entriesByName(entries []storage.Entry) map[string]storage.Entry {
	byName := map[string]storage.Entry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}

	return byName
}

// ScanCores — les cœurs libretro déclarés (rangés sous leur console, ou à plat), leur variante Android, et les
// émulateurs et paquets du téléphone des consoles récentes : listés comme librarySection les liste chez GameFlex.
func ScanCores(source storage.Source) Found {
	found := newFound()
	flat, _ := source.List("cores")
	flatByName := entriesByName(flat)
	for _, console := range consoles.All() {
		dir := path.Join("cores", console.ID)
		entries, _ := source.List(dir)
		byName := entriesByName(entries)
		for _, core := range console.Cores {
			if strings.Contains(core, " ") {
				continue // un émulateur en conteneur, pas un cœur libretro
			}
			file := CoreFileName(core)
			where := Locator{}
			variants := map[string]int64{}
			if entry, ok := byName[file]; ok && !entry.Dir {
				where.Path, where.Size = path.Join(dir, file), entry.Size
				variants["linux"] = entry.Size
			} else if entry, ok := flatByName[file]; ok && !entry.Dir {
				// Un cœur à plat compte pour le serveur, pas pour le listing (librarySection ne garde que ceux rangés
				// sous leur console ou qui ont une variante Android).
				where.Path, where.Size = path.Join("cores", file), entry.Size
			}
			if entry, ok := byName[AndroidCoreFileName(core)]; ok && !entry.Dir {
				where.Android = path.Join(dir, entry.Name)
				variants["android"] = entry.Size
			}
			if len(variants) == 0 {
				continue
			}
			found.add(console.ID, listing.Item{Name: core, Size: where.Size, Variants: variants}, where)
		}
		if !HasDesktopEmulator(console.ID) {
			continue
		}
		for _, entry := range entries {
			if entry.Dir || !IsEmulatorFile(console.ID, entry.Name) {
				continue
			}
			variant := "emulator"
			if IsPhoneBundle(entry.Name) {
				variant = "bundle"
			}
			found.add(console.ID, listing.Item{Name: entry.Name, Size: entry.Size, Variants: map[string]int64{variant: entry.Size}},
				Locator{Path: path.Join(dir, entry.Name), Size: entry.Size})
		}
	}

	return found
}

// ScanBios — les BIOS de chaque console, sous `bios/<console>/` (consoles.BiosDir) : un fichier, ou un dossier (sa
// taille est celle de sa racine, comme chez GameFlex).
func ScanBios(source storage.Source) Found {
	found := newFound()
	for _, console := range consoles.All() {
		subdir, ok := biosDirFor(console.ID)
		if !ok {
			continue
		}
		dir := path.Join("bios", subdir)
		entries, err := source.List(dir)
		if err != nil {
			continue
		}
		sortEntries(entries)
		for _, entry := range entries {
			if IsHidden(entry.Name) {
				continue
			}
			size := entry.Size
			if entry.Dir {
				size = rootSize(source, path.Join(dir, entry.Name))
			}
			found.add(console.ID, listing.Item{Name: entry.Name, Dir: entry.Dir, Size: size},
				Locator{Path: path.Join(dir, entry.Name), Dir: entry.Dir, Size: size})
		}
	}

	return found
}

// ScanPatches — les correctifs, `patches/<console>/<nom>` : un dossier (ses fichiers de tête nommés, sa taille entière)
// ou un fichier, comme listing.LocalTree.
func ScanPatches(source storage.Source) Found {
	found := newFound()
	consoleDirs, err := source.List("patches")
	if err != nil {
		return found
	}
	for _, console := range consoleDirs {
		if !console.Dir || IsHidden(console.Name) {
			continue
		}
		dir := path.Join("patches", console.Name)
		entries, _ := source.List(dir)
		sortEntries(entries)
		for _, entry := range entries {
			if IsHidden(entry.Name) {
				continue
			}
			item := listing.Item{Name: entry.Name, Dir: entry.Dir, Files: []string{}}
			where := Locator{Path: path.Join(dir, entry.Name), Dir: entry.Dir}
			if entry.Dir {
				children, _ := source.List(where.Path)
				for _, child := range children {
					if !IsHidden(child.Name) {
						item.Files = append(item.Files, child.Name)
					}
				}
				sort.Strings(item.Files)
				item.Size = storage.TreeSize(source, where.Path)
			} else {
				item.Files = []string{entry.Name}
				item.Size = entry.Size
			}
			where.Size = item.Size
			found.add(console.Name, item, where)
		}
	}

	return found
}

func sortEntries(entries []storage.Entry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
}

// rootSize — le poids des fichiers de tête d'un dossier.
func rootSize(source storage.Source, dir string) int64 {
	entries, _ := source.List(dir)
	var total int64
	for _, entry := range entries {
		if !entry.Dir {
			total += entry.Size
		}
	}

	return total
}

// MirrorParams — recopie le dossier `params/` d'une source (de petits fichiers JSON) dans `target`, d'où le code des
// paramètres de GameFlex (gameflex/emuparams, qui lit un disque) les lit ; rend faux si la source n'en a pas. Le miroir
// précédent part : un paramètre retiré de la source disparaît aussi.
func MirrorParams(source storage.Source, target string) (bool, error) {
	if err := os.RemoveAll(target); err != nil {
		return false, err
	}
	opener, ok := source.(storage.Opener)
	if !ok {
		return false, nil
	}
	if entries, err := source.List("params"); err != nil || len(entries) == 0 {
		return false, nil
	}
	var copyDir func(dir string) error
	copyDir = func(dir string) error {
		entries, err := source.List(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := path.Join(dir, entry.Name)
			if IsHidden(entry.Name) {
				continue
			}
			if entry.Dir {
				if err := copyDir(child); err != nil {
					return err
				}

				continue
			}
			if !strings.HasSuffix(entry.Name, ".json") || entry.Size > 4<<20 {
				continue
			}
			local := filepath.Join(target, filepath.FromSlash(child))
			if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
				return err
			}
			in, err := opener.Open(child)
			if err != nil {
				return err
			}
			out, err := os.Create(local)
			if err != nil {
				in.Close()

				return err
			}
			_, err = io.Copy(out, in)
			in.Close()
			out.Close()
			if err != nil {
				return err
			}
		}

		return nil
	}

	return true, copyDir("params")
}
