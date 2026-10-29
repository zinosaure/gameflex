package sessionstatus

// LA CHALEUR ET LA BATTERIE DE LA MACHINE QUI JOUE (le porteur, 2026-10-28) :
// « la température et la batterie (100 % quand c'est serveur) », et entre
// parenthèses l'écart depuis le début de la partie — « commencé à 20°, 34°
// vingt minutes plus tard : (+14°) ». Les mêmes écarts pour le processeur, la
// mémoire et la mémoire vidéo.
//
// La machine est celle qui fait tourner la partie, comme Status.Host : le
// serveur, ou le téléphone quand c'est lui qui joue (il donne ses propres
// chiffres, voir le kernel du téléphone).

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Baseline — la première mesure d'une partie, champ par champ. Mêmes noms que
// Status : la page soustrait l'une de l'autre.
type Baseline struct {
	TemperatureCelsius *float64 `json:"temperatureCelsius,omitempty"`
	BatteryPercent     *float64 `json:"batteryPercent,omitempty"`
	CPUPercent         *float64 `json:"cpuPercent,omitempty"`
	RSSBytes           int64    `json:"rssBytes,omitempty"`
	VRAMUsedBytes      int64    `json:"vramUsedBytes,omitempty"`
}

// Baselines — la première mesure de chaque partie en cours.
//
// GARDÉE PAR LE SERVEUR, ET NON PAR LA PAGE : tous ceux qui regardent voient
// le même écart, et recharger la page ne le remet pas à zéro. Champ par champ,
// parce qu'une mesure n'existe pas toujours dès le premier relevé — la
// cadence d'un GPU arrive après son premier échantillon : son départ est le
// premier relevé qui la porte.
//
// Rien n'est relevé pour personne (CODE_CONVENTIONS) : la première mesure est
// celle du premier abonné de la partie — la page la suit dès qu'elle joue sur
// un téléphone, pour ses pastilles.
type Baselines struct {
	mu    sync.Mutex
	first map[string]*Baseline
}

// Stamp complète le départ de la partie avec ce que ce relevé mesure pour la
// première fois, et le joint au relevé (Status.Start).
func (baselines *Baselines) Stamp(sessionID string, status *Status) {
	baselines.mu.Lock()
	defer baselines.mu.Unlock()

	if baselines.first == nil {
		baselines.first = map[string]*Baseline{}
	}
	start := baselines.first[sessionID]
	if start == nil {
		start = &Baseline{}
		baselines.first[sessionID] = start
	}
	if start.TemperatureCelsius == nil {
		start.TemperatureCelsius = status.TemperatureCelsius
	}
	if start.BatteryPercent == nil {
		start.BatteryPercent = status.BatteryPercent
	}
	if start.CPUPercent == nil {
		start.CPUPercent = status.CPUPercent
	}
	if start.RSSBytes == 0 {
		start.RSSBytes = status.RSSBytes
	}
	if start.VRAMUsedBytes == 0 {
		start.VRAMUsedBytes = status.VRAMUsedBytes
	}

	snapshot := *start
	status.Start = &snapshot
}

// Forget oublie le départ d'une partie terminée.
func (baselines *Baselines) Forget(sessionID string) {
	baselines.mu.Lock()
	defer baselines.mu.Unlock()

	delete(baselines.first, sessionID)
}

// ServerPower — la température et la batterie d'un serveur, lues dans le
// sysfs de l'hôte, que les conteneurs voient.
func ServerPower(status *Status) {
	if celsius, found := CPUTemperature("/sys"); found {
		status.TemperatureCelsius = &celsius
	}
	battery := BatteryPercent("/sys")
	status.BatteryPercent = &battery
}

// cpuSensors — les capteurs du processeur, par nom de pilote hwmon, et
// l'étiquette à préférer : la puce entière, pas un cœur. Tdie avant Tctl chez
// AMD — Tctl porte un décalage sur certains Ryzen.
var cpuSensors = []struct{ driver, labels string }{
	{"k10temp", "Tdie,Tctl"},
	{"zenpower", "Tdie,Tctl"},
	{"coretemp", "Package id 0"},
	{"cpu_thermal", ""},
}

// CPUTemperature — la température du processeur, en °C, lue dans hwmon.
// Faux quand aucun capteur connu n'est lisible (une machine virtuelle, un
// conteneur sans /sys).
func CPUTemperature(sysRoot string) (float64, bool) {
	monitors, _ := filepath.Glob(filepath.Join(sysRoot, "class/hwmon/hwmon*"))
	for _, sensor := range cpuSensors {
		for _, monitor := range monitors {
			if readTrimmed(filepath.Join(monitor, "name")) != sensor.driver {
				continue
			}
			if celsius, found := labelledTemperature(monitor, sensor.labels); found {
				return celsius, true
			}
		}
	}

	return 0, false
}

// labelledTemperature — la première étiquette de la liste qui existe, ou le
// premier capteur quand la liste est vide.
func labelledTemperature(monitor, labels string) (float64, bool) {
	inputs, _ := filepath.Glob(filepath.Join(monitor, "temp*_input"))
	if labels == "" {
		for _, input := range inputs {
			if celsius, found := milliCelsius(input); found {
				return celsius, true
			}
		}

		return 0, false
	}
	for _, label := range strings.Split(labels, ",") {
		for _, input := range inputs {
			if readTrimmed(strings.TrimSuffix(input, "_input")+"_label") != label {
				continue
			}
			if celsius, found := milliCelsius(input); found {
				return celsius, true
			}
		}
	}

	return 0, false
}

// BatteryPercent — la charge de la batterie de la machine, ou 100 pour une
// machine sur secteur, qui n'en a pas. La batterie d'un PÉRIPHÉRIQUE (une
// manette, une souris : `scope` vaut « Device ») n'est pas celle de la
// machine.
func BatteryPercent(sysRoot string) float64 {
	supplies, _ := filepath.Glob(filepath.Join(sysRoot, "class/power_supply/*"))
	for _, supply := range supplies {
		if readTrimmed(filepath.Join(supply, "type")) != "Battery" ||
			readTrimmed(filepath.Join(supply, "scope")) == "Device" {
			continue
		}
		if capacity, err := strconv.ParseFloat(readTrimmed(filepath.Join(supply, "capacity")), 64); err == nil {
			return capacity
		}
	}

	return 100
}

func milliCelsius(path string) (float64, bool) {
	value, err := strconv.ParseFloat(readTrimmed(path), 64)
	if err != nil || value <= 0 {
		return 0, false
	}

	return value / 1000, true
}

func readTrimmed(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(raw))
}
