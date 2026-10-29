package sessionstatus

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeSys(t *testing.T, files map[string]string) string {
	root := t.TempDir()
	for path, content := range files {
		full := filepath.Join(root, path)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(content), 0o644)
	}

	return root
}

// LA PUCE, PAS UN AUTRE CAPTEUR : hwmon mêle la carte mère, le disque, la
// carte réseau. Chez AMD, Tdie avant Tctl (décalé sur certains Ryzen) ;
// chez Intel, le « Package id 0 » plutôt qu'un cœur.
func TestCPUTemperatureReadsTheProcessorPackage(t *testing.T) {
	root := fakeSys(t, map[string]string{
		"class/hwmon/hwmon0/name":        "nvme\n",
		"class/hwmon/hwmon0/temp1_input": "27850\n",
		"class/hwmon/hwmon1/name":        "k10temp\n",
		"class/hwmon/hwmon1/temp1_input": "55500\n",
		"class/hwmon/hwmon1/temp1_label": "Tctl\n",
		"class/hwmon/hwmon1/temp2_input": "45500\n",
		"class/hwmon/hwmon1/temp2_label": "Tdie\n",
	})
	if celsius, found := CPUTemperature(root); !found || celsius != 45.5 {
		t.Fatalf("AMD : %v %v", celsius, found)
	}

	root = fakeSys(t, map[string]string{
		"class/hwmon/hwmon3/name":        "coretemp\n",
		"class/hwmon/hwmon3/temp2_input": "61000\n",
		"class/hwmon/hwmon3/temp2_label": "Core 0\n",
		"class/hwmon/hwmon3/temp1_input": "58000\n",
		"class/hwmon/hwmon3/temp1_label": "Package id 0\n",
	})
	if celsius, found := CPUTemperature(root); !found || celsius != 58 {
		t.Fatalf("Intel : %v %v", celsius, found)
	}

	if _, found := CPUTemperature(fakeSys(t, map[string]string{"class/hwmon/hwmon0/name": "nvme\n"})); found {
		t.Fatal("sans capteur du processeur, pas de température")
	}
}

// UN SERVEUR SUR SECTEUR EST À 100 %, un portable dit sa batterie — et la
// batterie d'une manette branchée au serveur n'est pas la sienne.
func TestBatteryPercentIsTheMachines(t *testing.T) {
	if percent := BatteryPercent(t.TempDir()); percent != 100 {
		t.Fatalf("secteur : %v", percent)
	}

	root := fakeSys(t, map[string]string{
		"class/power_supply/ps-controller-battery-01/type":     "Battery\n",
		"class/power_supply/ps-controller-battery-01/scope":    "Device\n",
		"class/power_supply/ps-controller-battery-01/capacity": "20\n",
		"class/power_supply/AC/type":                           "Mains\n",
	})
	if percent := BatteryPercent(root); percent != 100 {
		t.Fatalf("la manette compte : %v", percent)
	}

	root = fakeSys(t, map[string]string{
		"class/power_supply/BAT0/type":     "Battery\n",
		"class/power_supply/BAT0/capacity": "37\n",
	})
	if percent := BatteryPercent(root); percent != 37 {
		t.Fatalf("portable : %v", percent)
	}
}

// LE DÉPART EST LA PREMIÈRE MESURE, CHAMP PAR CHAMP : une mesure absente du
// premier relevé prend le départ du premier relevé qui la porte, et un départ
// pris ne bouge plus. Une partie finie est oubliée.
func TestBaselinesKeepTheFirstReadingOfEachField(t *testing.T) {
	var baselines Baselines

	first := Status{TemperatureCelsius: Percent(20), BatteryPercent: Percent(50), RSSBytes: 100}
	baselines.Stamp("a", &first)
	if first.Start == nil || *first.Start.TemperatureCelsius != 20 || first.Start.VRAMUsedBytes != 0 {
		t.Fatalf("premier relevé : %+v", first.Start)
	}

	later := Status{TemperatureCelsius: Percent(34), BatteryPercent: Percent(37), RSSBytes: 900, VRAMUsedBytes: 2048}
	baselines.Stamp("a", &later)
	if *later.Start.TemperatureCelsius != 20 || *later.Start.BatteryPercent != 50 ||
		later.Start.RSSBytes != 100 || later.Start.VRAMUsedBytes != 2048 {
		t.Fatalf("plus tard : %+v", later.Start)
	}

	other := Status{TemperatureCelsius: Percent(40)}
	baselines.Stamp("b", &other)
	if *other.Start.TemperatureCelsius != 40 {
		t.Fatalf("une autre partie : %+v", other.Start)
	}

	baselines.Forget("a")
	again := Status{TemperatureCelsius: Percent(30)}
	baselines.Stamp("a", &again)
	if *again.Start.TemperatureCelsius != 30 {
		t.Fatalf("oubliée : %+v", again.Start)
	}
}
