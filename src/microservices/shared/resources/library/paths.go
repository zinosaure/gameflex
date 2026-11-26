// Repris de kernel/src/internal/retro_files/scanner.go (chantier 61) : ce que bios.go et cores.go y prenaient.

package library

import "strings"

// IsHidden — un nom caché (un point devant) n'est jamais un élément.
func IsHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}
