// Repris de kernel/src/internal/retro_files/scanner.go (chantier 61) : ce que bios.go et cores.go y prenaient.

package library

import (
	"fmt"
	"path/filepath"
	"strings"
)

// IsHidden — un nom caché (un point devant) n'est jamais un élément.
func IsHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

// safeJoin joins root with the given untrusted path segments and rejects
// anything that would escape root (path traversal via ".." or an absolute
// segment) — segments come straight from URL parameters.
func safeJoin(root string, segments ...string) (string, error) {
	joined := filepath.Join(append([]string{root}, segments...)...)
	cleanRoot := filepath.Clean(root)
	if joined != cleanRoot && !strings.HasPrefix(joined, cleanRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path")
	}
	return joined, nil
}
