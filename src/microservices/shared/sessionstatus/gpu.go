package sessionstatus

// LE GPU DES PARTIES (chantier 39, 2026-10-29). Un serveur peut en avoir
// plusieurs — le Radeon intégré et une carte ajoutée en OCuLink, par
// exemple. Les parties n'en reçoivent qu'un : celui que l'installation
// déclare (GAMEFLEX_GPU, son adresse PCI), le premier dans l'ordre PCI à
// défaut. Le kernel ne donne au conteneur que ses deux nœuds de /dev/dri :
// l'émulateur, Vulkan, l'encodeur et le panneau de performances n'en voient
// pas d'autre.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// GPU — une carte de l'hôte, telle que /sys/class/drm la décrit.
type GPU struct {
	// PCI — son adresse, « 0000:c4:00.0 » : ce que GAMEFLEX_GPU nomme. Elle ne
	// bouge pas d'un démarrage à l'autre ; les numéros de « card », si.
	PCI string
	// Vendor — « AMD », « Intel », « NVIDIA ».
	Vendor string
	// Card, Render — ses nœuds dans /dev/dri (« card1 », « renderD128 ») ;
	// Render vide quand le pilote n'en donne pas.
	Card   string
	Render string
	// VRAMBytes — sa mémoire vidéo, quand le pilote la dit (amdgpu).
	VRAMBytes int64
}

var drmCard = regexp.MustCompile(`^card[0-9]+$`)

// GPUs — les cartes de fabricant connu de l'hôte, dans l'ordre PCI.
func GPUs(sysRoot string) []GPU {
	entries, _ := os.ReadDir(filepath.Join(sysRoot, "class/drm"))
	var gpus []GPU
	for _, entry := range entries {
		if !drmCard.MatchString(entry.Name()) {
			continue // card1-DP-1 est une sortie, pas une carte
		}
		device := filepath.Join(sysRoot, "class/drm", entry.Name(), "device")
		vendor := drmVendors[readTrimmed(filepath.Join(device, "vendor"))]
		if vendor == "" {
			continue
		}
		gpu := GPU{Vendor: vendor, Card: entry.Name()}
		if link, err := os.Readlink(device); err == nil {
			gpu.PCI = filepath.Base(link)
		}
		nodes, _ := os.ReadDir(filepath.Join(device, "drm"))
		for _, node := range nodes {
			if strings.HasPrefix(node.Name(), "renderD") {
				gpu.Render = node.Name()
			}
		}
		gpu.VRAMBytes, _ = strconv.ParseInt(readTrimmed(filepath.Join(device, "mem_info_vram_total")), 10, 64)
		gpus = append(gpus, gpu)
	}
	sort.SliceStable(gpus, func(i, j int) bool { return gpus[i].PCI < gpus[j].PCI })

	return gpus
}

// SessionGPU — le GPU des parties : celui dont `wanted` est l'adresse PCI
// (« 0000:c4:00.0 », ou « c4:00.0 »), le premier sinon. `declared` dit si
// `wanted` a été trouvé : une carte déclarée mais absente (en OCuLink,
// éteinte au démarrage) laisse les parties sur le premier GPU, et le kernel
// le dit au journal. `ok` faux : aucun GPU.
func SessionGPU(sysRoot, wanted string) (gpu GPU, declared, ok bool) {
	gpus := GPUs(sysRoot)
	if len(gpus) == 0 {
		return GPU{}, false, false
	}
	if wanted = strings.ToLower(strings.TrimSpace(wanted)); wanted != "" {
		if strings.Count(wanted, ":") == 1 {
			wanted = "0000:" + wanted
		}
		for _, gpu := range gpus {
			if gpu.PCI == wanted {
				return gpu, true, true
			}
		}
	}

	return gpus[0], false, true
}

// GPUUsage — l'occupation et la mémoire vidéo du GPU que CE conteneur a reçu,
// quand le pilote les dit (amdgpu) ; vide sinon, et la page montre « — ».
func GPUUsage() map[string]any {
	return readGPUUsage("/dev", "/sys")
}

// readGPUUsage — le GPU reçu se reconnaît à son nœud de rendu dans /dev/dri,
// le seul que le kernel donne (SessionGPU) ; ses chiffres sont dans le sysfs
// de l'hôte, sous le même nom. Une carte voisine n'y entre plus.
func readGPUUsage(devRoot, sysRoot string) map[string]any {
	out := map[string]any{}
	nodes, _ := filepath.Glob(filepath.Join(devRoot, "dri/renderD*"))
	if len(nodes) == 0 {
		return out
	}
	sort.Strings(nodes)
	device := filepath.Join(sysRoot, "class/drm", filepath.Base(nodes[0]), "device")
	if busy, err := strconv.Atoi(readTrimmed(filepath.Join(device, "gpu_busy_percent"))); err == nil {
		out["busy_percent"] = busy
	}
	total, errTotal := strconv.ParseUint(readTrimmed(filepath.Join(device, "mem_info_vram_total")), 10, 64)
	used, errUsed := strconv.ParseUint(readTrimmed(filepath.Join(device, "mem_info_vram_used")), 10, 64)
	if errTotal == nil && errUsed == nil {
		out["vram_total_bytes"] = total
		out["vram_used_bytes"] = used
	}

	return out
}
