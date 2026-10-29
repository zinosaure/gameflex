package sessionstatus

// LES RESSOURCES D'UNE MACHINE, POUR SES ÉMULATEURS DE BUREAU (2026-10-23).
//
// Une PS3, une Switch ou une Xbox tournent sur le GPU de la machine qui les
// lance — et sans lui, sur le seul processeur, bien trop lent pour jouer. La
// page le dit avant la partie, pour la source choisie : ici, ou chez un
// serveur fédéré, dont les ressources ne sont pas les nôtres (le porteur :
// « quand on est sur un serveur fédéré, ce n'est pas les mêmes ressources
// qui sont utilisées »), et le panneau Performance les montre pendant la
// partie (Status.Host). Chaque machine les déclare dans /api/v1/capabilities
// (`resources`) ; un kernel les relaie d'un pair (remote.PeerResources).

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Resources — ce qu'une machine offre à ses émulateurs de bureau.
type Resources struct {
	// GPU — son fabricant (« AMD », « Intel », « NVIDIA », « Adreno ») ; vide
	// quand aucun GPU n'est utilisable.
	GPU string `json:"gpu,omitempty"`
	// VRAMBytes — la mémoire vidéo, quand le pilote la dit (AMD).
	VRAMBytes int64 `json:"vramBytes,omitempty"`
	// CPUThreads — les fils d'exécution du processeur.
	CPUThreads int `json:"cpuThreads,omitempty"`
	// MemoryBytes — la mémoire vive de la machine.
	MemoryBytes int64 `json:"memoryBytes,omitempty"`
}

// LocalResources — le processeur et la mémoire de cette machine, avec le GPU
// que l'appelant sait (SessionGPU au serveur, l'Adreno au téléphone).
func LocalResources(gpu string, vramBytes int64) Resources {
	return Resources{
		GPU:         gpu,
		VRAMBytes:   vramBytes,
		CPUThreads:  runtime.NumCPU(),
		MemoryBytes: memTotal("/proc/meminfo"),
	}
}

// drmVendors — les fabricants par identifiant PCI.
var drmVendors = map[string]string{"0x1002": "AMD", "0x8086": "Intel", "0x10de": "NVIDIA"}

// ServerResources — les ressources d'un serveur : le GPU que ses sessions
// reçoivent (SessionGPU, `wanted` étant GAMEFLEX_GPU), lu dans le sysfs de
// l'hôte, que le kernel voit.
func ServerResources(wanted string) Resources {
	gpu, _, _ := SessionGPU("/sys", wanted)

	return LocalResources(gpu.Vendor, gpu.VRAMBytes)
}

// memTotal — « MemTotal » de /proc/meminfo, en octets ; 0 illisible.
func memTotal(path string) int64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kilobytes, _ := strconv.ParseInt(fields[1], 10, 64)

			return kilobytes * 1024
		}
	}

	return 0
}
