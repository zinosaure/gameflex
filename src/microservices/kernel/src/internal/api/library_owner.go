package api

import (
	"fmt"
	"os"
	"path/filepath"

	"gameflex/kernel/internal/config"
	"gameflex/storage"
	"gameflex/storage/installs"
)

// CE QUE LE NOYAU ÉCRIT DANS LA BIBLIOTHÈQUE APPARTIENT À SON PROPRIÉTAIRE
// (#146) : storage.AdoptParentOwner, partagé depuis le 2026-11-13 avec les
// métadonnées, qui écrivent aussi sur les disques internes.

// EnsureStorageFolders — le rangement du dossier (chantier 24,
// config.StorageFolders), créé s'il manque : une partie monte ses sections, et
// Docker refuse un montage dont la source n'existe pas. Ce que le noyau crée
// là appartient au propriétaire du dossier — à root, un `bios/` ne se
// remplirait plus à la main (vécu le 2026-10-22 sur les deux piles).
func EnsureStorageFolders(root string) error {
	for _, folder := range config.StorageFolders {
		dir := filepath.Join(root, folder)
		if _, err := os.Stat(dir); err == nil {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("%s: %w", folder, err)
		}
		storage.AdoptParentOwner(dir)
	}
	// Les effacements du cache qu'un arrêt a interrompus (#448), repris en fond.
	go installs.Purge(root)

	return nil
}
