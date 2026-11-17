package emuparams

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LA HAUTEUR QUE L'ÉMULATEUR REND — chantier 58 (le porteur, 2026-11-16 :
// « On ne propose des adaptations de flux que si la console le propose aussi.
// Mais attention, ça ne doit pas être quelque chose de fixe à la console mais
// plutôt lié aux réglages de la console »).
//
// Lue après Apply, dans les réglages EN VIGUEUR : le défaut du lanceur, le
// paramètre de l'utilisateur, le correctif du jeu par-dessus. Elle borne les
// paliers d'une partie (consoles.TierHeights) : un palier plus haut que ce que
// l'émulateur rend ne serait qu'un agrandissement. 0 quand rien ne la dit
// (shadPS4, Cemu, Xenia : le jeu rend sa propre taille) — la partie la prend
// alors pour celle de son flux, sans rien inventer au-dessus.
//
// Kyty dit aussi la taille qu'il rend vraiment, jeu par jeu, dans le titre de
// sa fenêtre (« render: 960x540 ») : streamex la préfère quand elle y est.
func RenderHeight(emulator, configHome string) int {
	w, known := writers[emulator]
	if !known {
		return 0
	}
	switch emulator {
	case "kyty":
		return kytyRenderHeight(Environment(emulator, configHome))
	}
	base := configHome
	if w.data {
		base = filepath.Join(filepath.Dir(configHome), ".local", "share")
	}
	content, err := os.ReadFile(filepath.Join(base, w.file))
	if err != nil {
		return 0
	}
	lines := strings.Split(string(content), "\n")
	switch emulator {
	case "citron", "eden", "lichii":
		return switchRenderHeight(emulator, lines)
	case "rpcs3":
		return rpcs3RenderHeight(lines)
	case "xemu":
		return xemuRenderHeight(lines)
	}

	return 0
}

// switchRenderHeight — le palier de `resolution_setup` (son numéro dans la liste
// de l'émulateur, voir EdenResolution et CitronResolution) appliqué à la sortie
// de la console : 1080 lignes docké, 720 en portable.
func switchRenderHeight(emulator string, lines []string) int {
	setups, factors := []int{1, 2, 3, 4, 5, 6, 7}, []float64{0.5, 0.75, 1, 1.25, 1.5, 2, 3}
	if emulator == "citron" {
		setups, factors = []int{1, 2, 3, 12, 4, 13, 5, 6}, []float64{0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 3}
	}
	setup, ok := qtINIRead(lines, "Renderer", "resolution_setup")
	if !ok {
		return 0
	}
	base := 1080.0
	if docked, ok := qtINIRead(lines, "System", "use_docked_mode"); ok && docked == "0" {
		base = 720
	}
	number, err := strconv.Atoi(setup)
	if err != nil {
		return 0
	}
	for i, candidate := range setups {
		if candidate == number {
			return int(math.Round(factors[i] * base))
		}
	}

	return 0
}

// rpcs3RenderHeight — `Video: Resolution Scale` (un pourcentage de la sortie
// 1280×720 de RPCS3, voir RenderScale).
func rpcs3RenderHeight(lines []string) int {
	inVideo := false
	for _, line := range lines {
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			inVideo = strings.TrimSpace(line) == "Video:"

			continue
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if inVideo && ok && key == "Resolution Scale" {
			percent, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return 0
			}

			return 720 * percent / 100
		}
	}

	return 0
}

// xemuRenderHeight — `[display.quality] surface_scale`, un multiple des 480
// lignes de la Xbox (voir XemuSurfaceScale).
func xemuRenderHeight(lines []string) int {
	from, to, ok := tomlTable(lines, "display.quality")
	if !ok {
		return 0
	}
	for _, line := range lines[from:to] {
		key, value, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(key) == "surface_scale" {
			scale, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return 0
			}

			return 480 * scale
		}
	}

	return 0
}

// kytyRenderHeight — la plus petite de la sortie (`KYTY_OUTPUT_RESOLUTION`) et
// de la hauteur des jeux Unreal (`KYTY_UE_RENDER_HEIGHT`). Celle-ci ne vaut que
// pour un jeu Unreal ; la prendre pour borne ne propose jamais un palier que le
// jeu ne rend pas. Le titre de Kyty, lu par streamex, dit la vraie.
func kytyRenderHeight(environment []string) int {
	height := 0
	for _, variable := range environment {
		name, value, _ := strings.Cut(variable, "=")
		if name != "KYTY_OUTPUT_RESOLUTION" && name != "KYTY_UE_RENDER_HEIGHT" {
			continue
		}
		number, err := strconv.Atoi(value)
		if err != nil || number <= 0 {
			continue
		}
		if height == 0 || number < height {
			height = number
		}
	}

	return height
}

// qtINIRead — la valeur d'une clé d'une section d'un qt-config.ini.
func qtINIRead(lines []string, section, key string) (string, bool) {
	from, to, ok := qtINISection(lines, section)
	if !ok {
		return "", false
	}
	for _, line := range lines[from:to] {
		name, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && name == key {
			return strings.TrimSpace(value), true
		}
	}

	return "", false
}
