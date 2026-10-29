package listing

// LA MACHINE QUI SERT UN LISTING (chantier 15) : « tu peux rajouter des infos
// […] info du PC (RAM, VRAM, etc.) pour que les autres serveurs fédérés
// sachent » (le porteur, 2026-10-13). Un ami qui choisit où lancer une PS3
// sait ainsi sur quoi elle tournerait.
//
// Lu sur la machine même, par ce que Linux expose à tout processus — `/proc`
// et `/sys/class/drm` — : un conteneur les voit aussi. Ce qui ne se lit pas
// reste vide plutôt que deviné.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Machine — ce sur quoi tourne un serveur.
type Machine struct {
	OS       string `json:"os"`
	CPU      string `json:"cpu,omitempty"`
	Cores    int    `json:"cores"`
	RAMBytes int64  `json:"ramBytes,omitempty"`
	GPUs     []GPU  `json:"gpus,omitempty"`
	// Device — un téléphone : son modèle (l'application le dit, Linux non).
	Device string `json:"device,omitempty"`
}

// GPU — une carte graphique, et sa mémoire quand le pilote la dit (amdgpu).
// Une puce intégrée partage la mémoire du système, qui est sa seule limite.
type GPU struct {
	Vendor    string `json:"vendor,omitempty"`
	Name      string `json:"name,omitempty"`
	Driver    string `json:"driver,omitempty"`
	VRAMBytes int64  `json:"vramBytes,omitempty"`
}

// ReadMachine — la machine qui exécute ce code.
func ReadMachine() Machine {
	return readMachine("/")
}

// gpuVendors — les fabricants, par leur identifiant PCI.
var gpuVendors = map[string]string{"0x1002": "AMD", "0x10de": "NVIDIA", "0x8086": "Intel"}

var cardName = regexp.MustCompile(`^card[0-9]+$`)

func readMachine(root string) Machine {
	machine := Machine{OS: runtime.GOOS, Cores: runtime.NumCPU()}
	if release := readTrimmed(filepath.Join(root, "proc/sys/kernel/osrelease")); release != "" {
		machine.OS += " " + release
	}

	processors := 0
	for _, line := range strings.Split(readTrimmed(filepath.Join(root, "proc/cpuinfo")), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "processor":
			processors++
		case "model name", "Hardware":
			// « Hardware » est ce qu'un téléphone écrit à la place.
			if machine.CPU == "" {
				machine.CPU = value
			}
		}
	}
	if processors > 0 {
		machine.Cores = processors
	}

	for _, line := range strings.Split(readTrimmed(filepath.Join(root, "proc/meminfo")), "\n") {
		if rest, found := strings.CutPrefix(line, "MemTotal:"); found {
			if kilobytes, err := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "kB")), 10, 64); err == nil {
				machine.RAMBytes = kilobytes * 1024
			}
		}
	}

	cards, _ := os.ReadDir(filepath.Join(root, "sys/class/drm"))
	for _, card := range cards {
		if !cardName.MatchString(card.Name()) {
			continue // card1-DP-1 est une sortie, pas une carte
		}
		device := filepath.Join(root, "sys/class/drm", card.Name(), "device")
		vendor := gpuVendors[readTrimmed(filepath.Join(device, "vendor"))]
		if vendor == "" {
			continue
		}
		gpu := GPU{Vendor: vendor}
		if driver, err := os.Readlink(filepath.Join(device, "driver")); err == nil {
			gpu.Driver = filepath.Base(driver)
		}
		if vram, err := strconv.ParseInt(readTrimmed(filepath.Join(device, "mem_info_vram_total")), 10, 64); err == nil {
			gpu.VRAMBytes = vram
		}
		machine.GPUs = append(machine.GPUs, gpu)
	}

	// UNE PUCE INTÉGRÉE porte son nom dans celui du processeur : « AMD
	// Ryzen 7 255 w/ Radeon 780M Graphics ». C'est le seul endroit où Linux
	// le donne sans base de noms PCI. Seulement quand l'AMD est seul : à côté
	// d'une carte AMD ajoutée (en OCuLink), rien ne dit laquelle est la puce,
	// et les deux portaient son nom (chantier 39).
	if _, integrated, found := strings.Cut(machine.CPU, " w/ "); found {
		amd := -1
		for i, gpu := range machine.GPUs {
			if gpu.Vendor == "AMD" {
				if amd >= 0 {
					amd = -1
					break
				}
				amd = i
			}
		}
		if amd >= 0 {
			machine.GPUs[amd].Name = integrated
		}
	}

	// UN ADRENO (les téléphones Snapdragon) se nomme dans son pilote, KGSL :
	// « Adreno830v2 ». Sa mémoire est celle du téléphone — pas de mémoire
	// graphique à part.
	if model := readTrimmed(filepath.Join(root, "sys/class/kgsl/kgsl-3d0/gpu_model")); model != "" {
		machine.GPUs = append(machine.GPUs, GPU{Vendor: "Qualcomm", Name: adrenoName(model), Driver: "kgsl"})
	}

	return machine
}

var adrenoModel = regexp.MustCompile(`^Adreno\s*([0-9]+)`)

// adrenoName — « Adreno830v2 » devient « Adreno 830 » ; un autre nom reste
// tel quel.
func adrenoName(model string) string {
	if match := adrenoModel.FindStringSubmatch(model); match != nil {
		return "Adreno " + match[1]
	}

	return model
}

func readTrimmed(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(content))
}
