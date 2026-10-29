// Statut de session streamex — voir sessionstatus.Status pour la forme, et
// manager.StreamexStatus pour ce que le kernel y ajoute (l'heure de démarrage
// et la mémoire vue par Docker, que l'enfant ne peut pas connaître).
package docker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"gameflex/sessionstatus"
)

// streamexChildStatus — ce que le conteneur streamex publie sur /status. On
// ne déclare que ce qu'on lit : le reste de sa charge utile (slots, auth,
// jeu en cours…) sert à d'autres appelants et n'a rien à faire dans le
// panneau.
type streamexChildStatus struct {
	Players    int   `json:"players"`
	Spectators int   `json:"spectators"`
	RSSBytes   int64 `json:"rss_bytes"`
	// Emulator — ce que l'image déclare (/opt/emulator.txt).
	Emulator string `json:"emulator"`

	CPU struct {
		EmulatorPercent *float64 `json:"emulator_percent"`
		// EmulatorRSSBytes voyage avec le CPU du même processus : c'est la
		// mémoire de l'ÉMULATEUR, à distinguer de rss_bytes qui est celle du
		// conteneur entier. Le panneau montre les deux — ce que la machine
		// paye, et ce que l'émulateur prend.
		EmulatorRSSBytes int64    `json:"emulator_rss_bytes"`
		ContainerPercent *float64 `json:"container_percent"`
	} `json:"cpu"`
	FPS struct {
		// `emulator_window` est la cadence de l'ÉMULATEUR, que streamex lit dans
		// le titre de sa fenêtre (RPCS3, Cemu, Kyty) ou dans sa barre d'état par
		// le bus d'accessibilité (Eden). Absente, le panneau affiche un tiret
		// — voir StatusStreamex.
		EmulatorWindow *float64 `json:"emulator_window"`
	} `json:"fps"`
	GPU struct {
		BusyPercent    *float64 `json:"busy_percent"`
		VRAMUsedBytes  int64    `json:"vram_used_bytes"`
		VRAMTotalBytes int64    `json:"vram_total_bytes"`
	} `json:"gpu"`
	Rates struct {
		EmulatorDiskReadBytesPerSec *float64 `json:"emulator_disk_read_bytes_per_sec"`
	} `json:"rates"`
}

// StatusStreamex interroge le /status du conteneur et le traduit dans la
// forme partagée.
func (dockerClient *Client) StatusStreamex(host string, port int) (sessionstatus.Status, error) {
	var out sessionstatus.Status

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/status", host, port), nil)
	if err != nil {
		return out, err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return out, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return out, fmt.Errorf("status endpoint returned %s", response.Status)
	}

	var child streamexChildStatus
	if err := json.NewDecoder(response.Body).Decode(&child); err != nil {
		return out, err
	}

	out.Players = child.Players
	out.Spectators = child.Spectators
	out.Emulator = child.Emulator
	// streamex ne connaît PAS le RSS de l'émulateur seul : son `rss_bytes` est
	// déjà celui du cgroup, donc du conteneur. On le range là où il est vrai
	// plutôt que sous « RAM émulateur », où il mentirait.
	out.ContainerRSSBytes = child.RSSBytes
	out.RSSBytes = child.CPU.EmulatorRSSBytes
	out.CPUPercent = child.CPU.EmulatorPercent
	if out.CPUPercent == nil {
		out.CPUPercent = child.CPU.ContainerPercent
	}
	// La cadence de l'émulateur, ou rien. Se rabattre sur celle du flux
	// (`encoded`) remplissait « Emulation FPS » avec le 60 de l'encodeur, déjà
	// affiché sous « FPS » : le 2026-09-11, Zelda BotW tournait à 17 FPS dans
	// Citron, qui n'écrit pas sa cadence dans son titre, et le panneau disait 61.
	// Deux mesures sous le même nom, c'est pire qu'un tiret.
	if child.FPS.EmulatorWindow != nil {
		out.EmulationFPS = *child.FPS.EmulatorWindow
	}
	out.GPUBusyPercent = child.GPU.BusyPercent
	out.VRAMUsedBytes = child.GPU.VRAMUsedBytes
	out.VRAMTotalBytes = child.GPU.VRAMTotalBytes
	out.DiskReadBytesPerSec = child.Rates.EmulatorDiskReadBytesPerSec

	return out, nil
}
