package library

import (
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gameflex/consoles"
	"gameflex/federation/internal/store"
	"gameflex/storage"
)

// LES RESSOURCES INTERNES DU SERVICE (chantier 61 ; le porteur, 2026-11-18 : « Gameflex-Federation prends aussi en
// compte les ressources internes (équivalent des disques externes qu'on rajoute dans gameflex actuellement) ») : un
// disque monté dans le conteneur, `<chemin>:/mnt/<nom>`, rangé comme une bibliothèque GameFlex — `roms/`, `bios/`,
// `cores/`, `patches/`, les paramètres (`params/`) et `metadata/`. Comme chez GameFlex (chantiers 23 et 24) : chaque
// dossier sous /mnt dont le nom est utilisable est un disque, présent s'il porte au moins une section ; pour un appareil
// relié, tous les disques et toutes les ressources ne font qu'un.

// Disk — un disque monté.
type Disk struct {
	Name    string `json:"name"`
	Dir     string `json:"-"`
	Present bool   `json:"present"`
}

// diskName — un nom utilisable tel quel dans une adresse et un chemin, la règle de GameFlex.
var diskName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Folders — les sections qu'un disque peut porter.
var Folders = []string{"roms", "bios", "cores", "patches", "params", "metadata"}

// Section — le dossier d'une section de ce disque.
func (disk Disk) Section(name string) string { return filepath.Join(disk.Dir, name) }

// ID — l'identifiant du disque dans le catalogue : négatif, dérivé de son nom, pour ne jamais croiser celui d'une
// ressource enregistrée (positif). Le même nom retrouve le même identifiant d'un démarrage à l'autre.
func (disk Disk) ID() int64 {
	sum := fnv.New64a()
	sum.Write([]byte(disk.Name))

	return -int64(sum.Sum64()>>1) - 1
}

// Resource — le disque vu comme une ressource du catalogue (genre « disk », son dossier pour adresse).
func (disk Disk) Resource() store.Resource {
	return store.Resource{ID: disk.ID(), Name: disk.Name, Kind: "disk", Address: disk.Dir}
}

// Disks — les disques montés sous `root`, dans l'ordre de leur nom.
func Disks(root string) []Disk {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	disks := []Disk{}
	for _, entry := range entries {
		if !entry.IsDir() || !diskName.MatchString(entry.Name()) {
			continue
		}
		disk := Disk{Name: entry.Name(), Dir: filepath.Join(root, entry.Name())}
		for _, folder := range Folders {
			if info, err := os.Stat(disk.Section(folder)); err == nil && info.IsDir() {
				disk.Present = true

				break
			}
		}
		disks = append(disks, disk)
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].Name < disks[j].Name })

	return disks
}

// Present — les disques qui portent au moins une section.
func Present(disks []Disk) []Disk {
	kept := []Disk{}
	for _, disk := range disks {
		if disk.Present {
			kept = append(kept, disk)
		}
	}

	return kept
}

// CoreBelongsTo — ce cœur est-il déclaré pour cette console ? Repris de GameFlex (storage_admin_api.go).
func CoreBelongsTo(consoleID, core string) bool {
	if consoleID == "" || core == "" {
		return false
	}
	for _, declared := range consoles.CoresFor(consoleID) {
		if declared == core {
			return true
		}
	}

	return false
}

// Local — la source locale de GameFlex (storage.LocalSource, qui liste), et la lecture d'un fichier, sans sortir du
// dossier du disque : un jeu, un BIOS ou un correctif en dossier voyage zippé, fichier par fichier.
type Local struct {
	*storage.LocalSource
	root string
}

// NewLocal — la source du dossier `root`, sous le nom `name`.
func NewLocal(name, root string) (*Local, error) {
	source, err := storage.NewLocalSource(name, root)
	if err != nil {
		return nil, err
	}

	return &Local{LocalSource: source, root: root}, nil
}

// Open — le contenu d'un fichier, chemin relatif au disque.
func (local *Local) Open(path string) (io.ReadCloser, error) {
	full, err := safeJoin(local.root, filepath.FromSlash(path))
	if err != nil {
		return nil, err
	}

	return os.Open(full)
}
