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
	// GPU — son nom complet quand on le connaît (« AMD Radeon 780M Graphics »,
	// gpuName ; le porteur, 2026-11-14 : « Dans GPU tu peux mettre le nom
	// complet comme AMD Radeon ... »), son fabricant sinon (« AMD », « Intel »,
	// « NVIDIA », « Adreno ») ; vide quand aucun GPU n'est utilisable.
	GPU string `json:"gpu,omitempty"`
	// VRAMBytes — la mémoire vidéo, quand le pilote la dit (AMD).
	VRAMBytes int64 `json:"vramBytes,omitempty"`
	// CPU — le nom du processeur (« AMD Ryzen 7 255 », cpuName ; le porteur,
	// 2026-11-14 : « tu peux aussi rajouter le nom du processus ? ») ; vide
	// quand la machine ne le dit pas.
	CPU string `json:"cpu,omitempty"`
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
		CPU:         cpuName(cpuModel("/proc/cpuinfo")),
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
	gpu, _, ok := SessionGPU("/sys", wanted)
	if !ok {
		return LocalResources("", 0)
	}

	return LocalResources(gpuName(gpu, GPUs("/sys"), cpuModel("/proc/cpuinfo")), gpu.VRAMBytes)
}

// gpuName — le nom complet d'un GPU : celui que son pilote donne ; sinon, pour
// la puce AMD intégrée, celui que Linux écrit dans le nom du processeur
// (« AMD Ryzen 7 255 w/ Radeon 780M Graphics » → « AMD Radeon 780M Graphics »)
// — seulement quand elle est le seul GPU AMD : à côté d'une carte AMD ajoutée
// (en OCuLink), rien ne dit laquelle est la puce. Son fabricant à défaut. La
// même lecture que la machine de la page Stockage (storage/listing,
// machine.go).
func gpuName(gpu GPU, all []GPU, cpu string) string {
	if gpu.Name != "" {
		if strings.HasPrefix(gpu.Name, gpu.Vendor) {
			return gpu.Name
		}

		return gpu.Vendor + " " + gpu.Name
	}
	amd := 0
	for _, other := range all {
		if other.Vendor == "AMD" {
			amd++
		}
	}
	if _, integrated, found := strings.Cut(cpu, " w/ "); found && gpu.Vendor == "AMD" && amd == 1 {
		return "AMD " + strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(integrated), "AMD "))
	}

	return gpu.Vendor
}

// cpuModel — le « model name » de /proc/cpuinfo, ou son « Hardware » (ce
// qu'un téléphone écrit à la place) ; vide illisible.
func cpuModel(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	hardware := ""
	for _, line := range strings.Split(string(content), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "model name":
			return strings.TrimSpace(value)
		case "Hardware":
			hardware = strings.TrimSpace(value)
		}
	}

	return hardware
}

// cpuName — le nom du processeur sans la puce graphique que Linux y joint
// (« AMD Ryzen 7 255 w/ Radeon 780M Graphics » → « AMD Ryzen 7 255 ») : elle a
// sa ligne à elle (gpuName).
func cpuName(model string) string {
	name, _, _ := strings.Cut(model, " w/ ")

	return strings.TrimSpace(name)
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
