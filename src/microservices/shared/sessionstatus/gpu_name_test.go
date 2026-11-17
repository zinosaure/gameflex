package sessionstatus

import "testing"

// LE NOM COMPLET DU GPU (le porteur, 2026-11-14 : « Dans GPU tu peux mettre le
// nom complet comme AMD Radeon ... ») : celui du pilote d'abord ; la puce AMD
// intégrée, seule de son fabricant, par le nom du processeur ; le fabricant à
// défaut.
func TestTheGPUIsNamedInFull(t *testing.T) {
	integrated := GPU{Vendor: "AMD", PCI: "0000:c4:00.0"}
	card := GPU{Vendor: "AMD", PCI: "0000:03:00.0", Name: "Radeon RX 7600"}
	cpu := "AMD Ryzen 7 255 w/ Radeon 780M Graphics"

	cases := []struct {
		name string
		gpu  GPU
		all  []GPU
		cpu  string
		want string
	}{
		{"la puce intégrée, seule", integrated, []GPU{integrated}, cpu, "AMD Radeon 780M Graphics"},
		{"le nom du pilote", card, []GPU{card}, cpu, "AMD Radeon RX 7600"},
		{"le nom du pilote, déjà préfixé", GPU{Vendor: "AMD", Name: "AMD Radeon RX 7600"}, nil, "", "AMD Radeon RX 7600"},
		{"deux GPU AMD : rien ne dit lequel est la puce", integrated, []GPU{card, integrated}, cpu, "AMD"},
		{"un processeur sans puce nommée", integrated, []GPU{integrated}, "AMD Ryzen 9 7950X 16-Core Processor", "AMD"},
		{"un GPU Intel ne prend pas le nom de la puce AMD", GPU{Vendor: "Intel"}, []GPU{{Vendor: "Intel"}}, cpu, "Intel"},
	}
	for _, c := range cases {
		if got := gpuName(c.gpu, c.all, c.cpu); got != c.want {
			t.Errorf("%s : %q, attendu %q", c.name, got, c.want)
		}
	}
}

// LE NOM DU PROCESSEUR, sans la puce graphique jointe (le porteur, 2026-11-14 :
// « tu peux aussi rajouter le nom du processus ? »).
func TestTheProcessorIsNamedWithoutItsGraphics(t *testing.T) {
	for model, want := range map[string]string{
		"AMD Ryzen 7 255 w/ Radeon 780M Graphics": "AMD Ryzen 7 255",
		"AMD Ryzen 9 7950X 16-Core Processor":     "AMD Ryzen 9 7950X 16-Core Processor",
		"":                                        "",
	} {
		if got := cpuName(model); got != want {
			t.Errorf("%q : %q, attendu %q", model, got, want)
		}
	}
}

