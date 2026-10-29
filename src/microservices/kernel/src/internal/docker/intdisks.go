package docker

// LES RESSOURCES INTERNES (chantier 23, #426) — les autres disques de jeux de
// cette machine.
//
// Le porteur, 2026-10-22 : « les alt disques ne doivent pas être considérés
// comme ressources distantes mais ressources internes. Les ressources internes
// ont le droit de lancer des jeux/consoles entre eux. »
//
// UN DISQUE SE DÉCLARE PAR UN VOLUME DU COMPOSE, et rien d'autre :
//
//	volumes:
//	  - /un/disque/jeux:/mnt/<nom>
//
// Le kernel les découvre en lisant /mnt. Pour monter un disque dans le
// conteneur d'une partie, il lui faut pourtant son chemin SUR L'HÔTE — c'est
// le démon Docker qui résout une source de montage, pas ce conteneur. Ce
// chemin, il le lit sur son propre conteneur, par l'API Docker qu'il a déjà :
// aucune variable à tenir à jour à côté du volume.
//
// GÉNÉRIQUE : aucun nom de disque n'est écrit ici, l'ordre est celui des
// noms, et un disque n'est qu'un dossier rangé comme la bibliothèque.

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gameflex/kernel/internal/config"
)

// InternalDisk — un disque interne tel que le kernel le voit.
type InternalDisk struct {
	// Name — le nom du dossier sous /mnt : celui que l'installation a choisi.
	Name string `json:"name"`
	// Dir — son chemin dans le kernel (/mnt/<nom>).
	Dir string `json:"-"`
	// HostDir — son chemin sur l'hôte, lu sur le conteneur du kernel. Vide
	// quand le kernel ne tourne pas dans un conteneur, ou que ce dossier n'y
	// est pas un montage : le disque ne peut alors pas être monté dans une
	// partie.
	HostDir string `json:"-"`
	// Present — le disque apporte au moins une section (config.DiskFolders :
	// jeux, BIOS, cœurs, paramètres, correctifs) et peut être monté dans une
	// partie. Un disque débranché au démarrage de la pile laisse un dossier
	// vide : il apparaît, absent, et rien ne s'en lit.
	Present bool `json:"present"`
}

// Section — une section de ce disque (`cores`, `bios`…), dans le kernel.
func (disk InternalDisk) Section(name string) string {
	return filepath.Join(disk.Dir, name)
}

// RomsDir — le dossier des jeux de ce disque, dans le kernel.
func (disk InternalDisk) RomsDir() string {
	return filepath.Join(disk.Dir, "roms")
}

// RomsHostDir — le même, sur l'hôte : la source d'un montage de partie.
func (disk InternalDisk) RomsHostDir() string {
	return filepath.Join(disk.HostDir, "roms")
}

// diskName — un nom de disque utilisable tel quel dans une adresse, une
// étiquette de conteneur et un chemin : lettres, chiffres, point, tiret,
// souligné. Un dossier qui n'y répond pas n'est pas un disque.
var diskName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// internalDisksDir — /mnt ; un test le pointe ailleurs.
var internalDisksDir = config.InternalDisksDir

// InternalDisks — les disques internes, dans l'ordre de leur nom.
func (dockerClient *Client) InternalDisks(ctx context.Context) []InternalDisk {
	entries, err := os.ReadDir(internalDisksDir)
	if err != nil {
		return []InternalDisk{}
	}
	sources := dockerClient.selfMountSources(ctx)

	disks := []InternalDisk{}
	for _, entry := range entries {
		if !entry.IsDir() || !diskName.MatchString(entry.Name()) {
			continue
		}
		dir := filepath.Join(internalDisksDir, entry.Name())
		disk := InternalDisk{Name: entry.Name(), Dir: dir, HostDir: sources[dir]}
		disk.Present = disk.HostDir != "" && disk.bringsAnything()
		disks = append(disks, disk)
	}

	return disks
}

// bringsAnything — le disque a-t-il au moins une des sections qu'un disque
// apporte ?
func (disk InternalDisk) bringsAnything() bool {
	for _, folder := range config.DiskFolders {
		if info, err := os.Stat(disk.Section(folder)); err == nil && info.IsDir() {
			return true
		}
	}

	return false
}

// InternalDisk — le disque `name`, s'il est déclaré.
func (dockerClient *Client) InternalDisk(ctx context.Context, name string) (InternalDisk, bool) {
	for _, disk := range dockerClient.InternalDisks(ctx) {
		if disk.Name == name {
			return disk, true
		}
	}

	return InternalDisk{}, false
}

// LES MONTAGES DU KERNEL LUI-MÊME, gardés : ils ne changent qu'avec le
// conteneur, qui redémarre le processus. Une lecture ratée (démon lent au
// démarrage) se retente au plus toutes les trente secondes.
var (
	selfMountsMu      sync.Mutex
	selfMounts        map[string]string
	selfMountsTriedAt time.Time
)

// selfMountSources — destination → source des montages du conteneur du
// kernel. Vide hors conteneur.
func (dockerClient *Client) selfMountSources(ctx context.Context) map[string]string {
	selfMountsMu.Lock()
	defer selfMountsMu.Unlock()

	if selfMounts != nil {
		return selfMounts
	}
	if time.Since(selfMountsTriedAt) < 30*time.Second {
		return map[string]string{}
	}
	selfMountsTriedAt = time.Now()

	id := selfContainerID()
	if id == "" {
		return map[string]string{}
	}
	inspected, err := dockerClient.dockerClient.ContainerInspect(ctx, id)
	if err != nil {
		slog.Warn("internal disks: cannot read the kernel's own mounts", "container", id, "error", err)

		return map[string]string{}
	}
	sources := map[string]string{}
	for _, point := range inspected.Mounts {
		sources[filepath.Clean(point.Destination)] = point.Source
	}
	selfMounts = sources

	return selfMounts
}

// containerInMountinfo — l'identifiant d'un conteneur, tel que Docker le
// laisse dans les montages de /etc/hostname et /etc/hosts.
var containerInMountinfo = regexp.MustCompile(`/containers/([0-9a-f]{64})/`)

// selfContainerID — l'identifiant du conteneur où tourne ce processus. Le nom
// d'hôte n'en est que le début, et seulement si le compose ne le change pas :
// /proc/self/mountinfo, lui, porte l'identifiant entier.
func selfContainerID() string {
	if raw, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		if match := containerInMountinfo.FindSubmatch(raw); match != nil {
			return string(match[1])
		}
	}
	if hostname, err := os.Hostname(); err == nil {
		return strings.TrimSpace(hostname)
	}

	return ""
}
