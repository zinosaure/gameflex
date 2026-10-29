package storage

import (
	"os"
	"path/filepath"
	"strings"
)

// LocalSource — un dossier de la machine. Le fournisseur par défaut : sur le
// serveur, les jeux sont là ; sur un téléphone, c'est le stockage interne.
//
// C'est aussi le seul que le kernel utilise aujourd'hui, et c'est très bien :
// faire parler SMB à une machine pour lire son propre disque n'ajoute qu'une
// couche. La capacité existe pour ceux dont les fichiers sont ailleurs.
type LocalSource struct {
	name string
	root string
}

// NewLocalSource monte un dossier sous un nom de source.
func NewLocalSource(name, root string) (*LocalSource, error) {
	if name == "" {
		return nil, ErrEmptyName
	}

	return &LocalSource{name: name, root: root}, nil
}

func (s *LocalSource) Name() string { return s.name }

func (s *LocalSource) List(dir string) ([]Entry, error) {
	// filepath.Join NETTOIE le chemin, il ne l'ENFERME pas — c'est la
	// confusion que ce commentaire affirmait avant qu'un test ne la démente :
	// Join("/jeux/roms", "../../dehors") vaut "/dehors", proprement écrit et
	// parfaitement hors de la source. Une source qui fuit expose tout
	// l'appareil, alors la confinement est vérifié, pas supposé.
	target := filepath.Join(s.root, filepath.FromSlash(trimPath(dir)))

	inside, err := filepath.Rel(s.root, target)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return nil, ErrOutsideSource
	}

	items, err := os.ReadDir(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // un dossier disparu n'est pas une panne de scan
		}

		return nil, err
	}

	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		entry := Entry{Name: item.Name(), Dir: item.IsDir()}
		if info, err := item.Info(); err == nil {
			entry.Size = info.Size()
		}
		entries = append(entries, entry)
	}

	return entries, nil
}
