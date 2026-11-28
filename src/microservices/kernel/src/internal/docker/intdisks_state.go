package docker

// COUPER UN DISQUE INTERNE, LE DÉBRANCHER, LE REBRANCHER (chantier 86, `#813`).
//
//   - Coupé (`databases/disks.json` du dossier du serveur), un disque se lit comme absent : ses jeux, BIOS, cœurs et
//     correctifs sortent des listes et rien ne s'en lance. GameFlex ne le touche plus : il peut être démonté sur l'hôte.
//   - Une partie en cours qui monte un de ses dossiers le tient (un démontage serait refusé, « busy ») : on ne le coupe
//     pas sous elle (SessionsUsing).
//   - Les montages de l'hôte se suivent sans sondage : le noyau réveille qui attend sur /proc/self/mountinfo à chaque
//     changement de la table des montages de ce conteneur (POLLPRI) — le dossier des disques est monté en `rslave`,
//     un disque branché ou débranché y change donc la table (WatchMounts).

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"golang.org/x/sys/unix"

	"gameflex/kernel/internal/config"
	"gameflex/storage"
)

var disksStateMu sync.Mutex

// disksStateDir — le dossier du serveur ; un test le pointe ailleurs.
var disksStateDir = config.StorageDir

type disksState struct {
	// Disabled — les disques coupés, par leur nom.
	Disabled []string `json:"disabled"`
}

func disksStatePath() string {
	return filepath.Join(disksStateDir, "databases", "disks.json")
}

func readDisksState() disksState {
	state := disksState{}
	raw, err := os.ReadFile(disksStatePath())
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("internal disks: state unreadable, every disk is enabled", "error", err)
		}

		return state
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		slog.Warn("internal disks: state unreadable, every disk is enabled", "error", err)

		return disksState{}
	}

	return state
}

// disabledDisks — les disques coupés.
func disabledDisks() map[string]bool {
	disksStateMu.Lock()
	defer disksStateMu.Unlock()

	disabled := map[string]bool{}
	for _, name := range readDisksState().Disabled {
		disabled[name] = true
	}

	return disabled
}

// SetDiskEnabled — couper ou réactiver le disque `name` ; gardé au redémarrage.
func SetDiskEnabled(name string, enabled bool) error {
	disksStateMu.Lock()
	defer disksStateMu.Unlock()

	state := readDisksState()
	kept := []string{}
	for _, disabled := range state.Disabled {
		if disabled != name {
			kept = append(kept, disabled)
		}
	}
	if !enabled {
		kept = append(kept, name)
	}
	sort.Strings(kept)
	state.Disabled = kept

	path := disksStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".part"
	if err := os.WriteFile(temporary, encoded, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	storage.AdoptParentOwner(path)

	return nil
}

// SessionsUsing — les jeux des parties en cours qui montent un dossier du disque (sa source sur l'hôte) : tant qu'il y
// en a, on ne le coupe pas.
func (dockerClient *Client) SessionsUsing(ctx context.Context, disk InternalDisk) ([]string, error) {
	if disk.HostDir == "" || dockerClient.dockerClient == nil {
		return nil, nil
	}
	list, err := dockerClient.dockerClient.ContainerList(ctx, container.ListOptions{Filters: dockerClient.sessionFilters()})
	if err != nil {
		return nil, err
	}
	root := filepath.Clean(disk.HostDir) + string(filepath.Separator)
	games := []string{}
	for _, listed := range list {
		for _, point := range listed.Mounts {
			if strings.HasPrefix(filepath.Clean(point.Source)+string(filepath.Separator), root) {
				games = append(games, listed.Labels[labelGame])

				break
			}
		}
	}

	return games, nil
}

// WatchMounts — `onChange` à chaque changement de la table des montages de ce conteneur, jusqu'à la fin de `ctx`. Le
// noyau signale un changement de /proc/self/mountinfo par POLLPRI : aucune relecture à intervalle.
func WatchMounts(ctx context.Context, onChange func()) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		slog.Warn("internal disks: the mount table cannot be followed", "error", err)

		return
	}
	go func() {
		defer file.Close()
		fds := []unix.PollFd{{Fd: int32(file.Fd()), Events: unix.POLLPRI | unix.POLLERR}}
		for ctx.Err() == nil {
			// Une poignée de secondes au plus par attente, pour voir la fin de `ctx` : ce n'est pas une relecture, le
			// réveil vient du noyau.
			count, err := unix.Poll(fds, 5000)
			if err != nil {
				if errors.Is(err, unix.EINTR) {
					continue
				}
				slog.Warn("internal disks: following the mount table stopped", "error", err)

				return
			}
			// Le noyau retient le changement signalé à cette attente : la suivante ne le redonne pas.
			if count > 0 && fds[0].Revents&(unix.POLLPRI|unix.POLLERR) != 0 {
				onChange()
			}
		}
	}()
}
