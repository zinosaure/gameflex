package sessionstatus

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeGPU pose dans un sysfs factice une carte comme Linux la montre :
// class/drm/<card>/device, lien vers le périphérique PCI, qui porte son
// fabricant, sa mémoire vidéo et ses nœuds (drm/<card>, drm/<render>).
func fakeGPU(t *testing.T, root, card, pci, vendor, render string, vram string) {
	t.Helper()
	device := filepath.Join(root, "devices", pci)
	os.MkdirAll(filepath.Join(device, "drm", card), 0o755)
	if render != "" {
		os.MkdirAll(filepath.Join(device, "drm", render), 0o755)
		os.MkdirAll(filepath.Join(root, "class/drm", render), 0o755)
		os.Symlink(filepath.Join("../../../devices", pci), filepath.Join(root, "class/drm", render, "device"))
	}
	os.WriteFile(filepath.Join(device, "vendor"), []byte(vendor+"\n"), 0o644)
	if vram != "" {
		os.WriteFile(filepath.Join(device, "mem_info_vram_total"), []byte(vram+"\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(root, "class/drm", card), 0o755)
	if err := os.Symlink(filepath.Join("../../../devices", pci), filepath.Join(root, "class/drm", card, "device")); err != nil {
		t.Fatal(err)
	}
}

// LES CARTES DE L'HÔTE, dans l'ordre PCI — pas celui des numéros de card —,
// avec leurs deux nœuds ; une sortie (card1-DP-1) et une carte de fabricant
// inconnu n'en sont pas.
func TestGPUsInPCIOrderWithTheirNodes(t *testing.T) {
	root := t.TempDir()
	fakeGPU(t, root, "card1", "0000:c4:00.0", "0x1002", "renderD128", "8589934592")
	fakeGPU(t, root, "card0", "0000:03:00.0", "0x10de", "renderD129", "")
	fakeGPU(t, root, "card2", "0000:07:00.0", "0x1234", "renderD130", "")
	os.MkdirAll(filepath.Join(root, "class/drm/card1-DP-1"), 0o755)

	gpus := GPUs(root)
	if len(gpus) != 2 {
		t.Fatalf("%d cartes : %+v", len(gpus), gpus)
	}
	want := []GPU{
		{PCI: "0000:03:00.0", Vendor: "NVIDIA", Card: "card0", Render: "renderD129"},
		{PCI: "0000:c4:00.0", Vendor: "AMD", Card: "card1", Render: "renderD128", VRAMBytes: 8589934592},
	}
	for i := range want {
		if gpus[i] != want[i] {
			t.Errorf("carte %d : %+v, attendu %+v", i, gpus[i], want[i])
		}
	}
}

// LE GPU DES PARTIES est celui que GAMEFLEX_GPU nomme, avec ou sans le
// domaine PCI ; déclaré mais absent, ou pas déclaré, c'est le premier.
func TestSessionGPUTakesTheDeclaredOne(t *testing.T) {
	root := t.TempDir()
	fakeGPU(t, root, "card1", "0000:c4:00.0", "0x1002", "renderD128", "")
	fakeGPU(t, root, "card0", "0000:03:00.0", "0x1002", "renderD129", "")

	for _, test := range []struct {
		wanted   string
		pci      string
		declared bool
	}{
		{"0000:c4:00.0", "0000:c4:00.0", true},
		{" C4:00.0 ", "0000:c4:00.0", true},
		{"0000:05:00.0", "0000:03:00.0", false},
		{"", "0000:03:00.0", false},
	} {
		gpu, declared, ok := SessionGPU(root, test.wanted)
		if !ok || gpu.PCI != test.pci || declared != test.declared {
			t.Errorf("%q : %s déclaré=%v ok=%v, attendu %s déclaré=%v", test.wanted, gpu.PCI, declared, ok, test.pci, test.declared)
		}
	}
	if _, _, ok := SessionGPU(t.TempDir(), ""); ok {
		t.Error("sans carte : un GPU")
	}
}

// LE PANNEAU DE PERFORMANCES lit le GPU que le conteneur a reçu — son nœud
// de rendu dans /dev/dri —, jamais une carte voisine de l'hôte.
func TestGPUUsageReadsTheReceivedNode(t *testing.T) {
	sys, dev := t.TempDir(), t.TempDir()
	fakeGPU(t, sys, "card1", "0000:c4:00.0", "0x1002", "renderD128", "")
	fakeGPU(t, sys, "card0", "0000:03:00.0", "0x1002", "renderD129", "")
	write := func(pci, name, content string) {
		os.WriteFile(filepath.Join(sys, "devices", pci, name), []byte(content+"\n"), 0o644)
	}
	write("0000:c4:00.0", "gpu_busy_percent", "99")
	write("0000:03:00.0", "gpu_busy_percent", "37")
	write("0000:03:00.0", "mem_info_vram_total", "8589934592")
	write("0000:03:00.0", "mem_info_vram_used", "1073741824")
	os.MkdirAll(filepath.Join(dev, "dri"), 0o755)
	os.WriteFile(filepath.Join(dev, "dri/renderD129"), nil, 0o644)

	usage := readGPUUsage(dev, sys)
	if usage["busy_percent"] != 37 || usage["vram_total_bytes"] != uint64(8589934592) || usage["vram_used_bytes"] != uint64(1073741824) {
		t.Fatalf("%v", usage)
	}
	if usage := readGPUUsage(t.TempDir(), sys); len(usage) != 0 {
		t.Fatalf("sans GPU reçu : %v", usage)
	}
}
