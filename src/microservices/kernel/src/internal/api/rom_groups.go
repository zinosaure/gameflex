package api

// LES DISQUES ET LA SÉRIE DE CHAQUE JEU DE LA LISTE (chantier 49) — posés sur
// GET /api/v1/retrogaming/{consoleId}/roms, quelle que soit la source (la
// bibliothèque, un disque interne, un ami). La règle est partagée avec le
// téléphone depuis le chantier 57 (`#716`) : gameflex/metadata, rom_groups.go.

import (
	"gameflex/kernel/internal/retro_files"
	"gameflex/metadata"
)

func annotateRoms(consoleID string, roms []retro_files.RomEntry) {
	groups := metadata.GroupRoms(consoleID, len(roms), func(index int) (string, string) {
		return roms[index].Reference, roms[index].Title
	})
	for index := range roms {
		roms[index].RomGroups = groups[index]
	}
}
