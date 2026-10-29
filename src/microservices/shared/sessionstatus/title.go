package sessionstatus

import (
	"strconv"
	"strings"
)

// TitleFPS lit la cadence qu'un émulateur de bureau écrit lui-même dans le
// titre de sa fenêtre, où qu'elle s'y trouve et quelle que soit la casse
// (titres relevés le 2026-09-11) :
//
//	RPCS3  « FPS: 30.02 | Vulkan | … »
//	Cemu   « Cemu 2.6 - FPS: 60.10 [Vulkan] [Generic] [TitleId: …] … »
//	Kyty   « [Official build KytyPS5-…] Richman 11, …, frame: 2336, fps: 40 »
//	Eden   « eden | … | Hogwarts Legacy | … | FPS: 24.6 | Speed: 82% | Frame: 40.7 ms »
//	       — notre construction du téléphone seulement (tools/eden, patch 0003)
//
// La nocturne d'Eden du serveur, xemu et shadPS4 n'y mettent pas la leur : pas
// de valeur, et le panneau affiche un tiret plutôt qu'un chiffre venu d'ailleurs.
//
// UNE SEULE LECTURE POUR LE SERVEUR ET LE TÉLÉPHONE (2026-10-11). Elle vivait
// dans streamex ; le téléphone, qui a besoin de la même — en 360p à 30 images
// par seconde, sa capture plafonnait « Emulation FPS » à 30 —, la prend ici
// plutôt que d'en écrire une seconde qui dériverait.
func TitleFPS(title string) (float64, bool) {
	const label = "fps:"
	for i := 0; i+len(label) <= len(title); i++ {
		if !strings.EqualFold(title[i:i+len(label)], label) {
			continue
		}
		fields := strings.Fields(title[i+len(label):])
		if len(fields) == 0 {
			return 0, false
		}
		if v, err := strconv.ParseFloat(strings.TrimRight(fields[0], ",|"), 64); err == nil && v > 0 {
			return v, true
		}
	}

	return 0, false
}
