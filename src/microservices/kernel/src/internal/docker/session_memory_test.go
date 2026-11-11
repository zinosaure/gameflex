package docker

import "testing"

// LA PS5 A 20 GO : Kyty tient les 12,5 Go d'un jeu PS5 dans un seul bloc, à
// côté de la capture et de l'encodage, et à 12 Go le conteneur touchait son
// plafond (Expedition 33, 2026-10-30). LA SWITCH 2 AUSSI : à 12 Go, Lichii
// était tué par le noyau dès la forêt Kokiri de *Zelda Ocarina of Time*
// (2026-11-11, #644). Les autres consoles gardent les 12 Go mesurés pour RPCS3.
func TestSessionMemoryLimitGivesThePs5ItsWholeMemory(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)
	for console, want := range map[string]int64{"ps5": 20 * gib, "switch2": 20 * gib, "ps4": 12 * gib, "ps3": 12 * gib, "switch": 12 * gib} {
		if got := sessionMemoryLimit(console); got != want {
			t.Errorf("%s : %d Go, attendu %d", console, got/gib, want/gib)
		}
	}
}
