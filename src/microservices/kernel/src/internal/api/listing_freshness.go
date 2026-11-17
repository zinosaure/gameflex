package api

// LA DATE DU LISTING, DE MÉMOIRE (journal 629 ; le porteur, 2026-11-15 :
// « Répondre de mémoire »). La règle est celle de listing.Freshness, que le
// kernel du téléphone suit aussi ; ici, ce que le serveur y branche :
//
//   - l'en-tête du listing de ce serveur, relu après chacune de ses écritures
//     (rememberSelfListing : rebuildSelfLocked, refreshConsoleRoms,
//     refreshConsoleStatuses, le démarrage) ;
//   - les disques présents, chaque fois que presentDisks les regarde ;
//   - leurs sections, relues au démarrage et par « Relire » (readDisks) ;
//   - la date du catalogue de chaque disque, que retro_files garde en mémoire
//     (CatalogLastScan) : un catalogue refait parce qu'une liste a vu un
//     dossier changer fait avancer la date sans que la question le relise.

import (
	"context"
	"time"

	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/listing"
)

// listingUpdatedAt — la date du listing servi à un ami, sans rien lire sur un
// disque ; faux tant que l'en-tête n'a jamais été lu.
func (server *Server) listingUpdatedAt() (time.Time, bool) {
	return server.freshness.UpdatedAt(func(_, dir string) time.Time {
		return retro_files.CatalogLastScan(docker.InternalDisk{Dir: dir}.RomsDir())
	})
}

// rememberSelfListing — le listing de ce serveur vient d'être écrit (ou existe
// déjà au démarrage) : sa date se relit, une fois.
func (server *Server) rememberSelfListing() {
	if header, err := listing.LoadHeader(listingCacheDir, listing.SelfKey); err == nil {
		server.freshness.Self(header.UpdatedAt)
	}
}

// readDisks — ces disques relus : leur catalogue s'il a changé, et le dernier
// changement de leurs autres sections. C'est ce que la question relisait à
// chaque fois (l'ancien disksUpdatedAt) ; il ne se fait plus qu'au démarrage
// et par « Relire ».
func (server *Server) readDisks(disks ...docker.InternalDisk) {
	for _, disk := range disks {
		retro_files.CatalogScannedAt(disk.RomsDir())
		var changed time.Time
		for _, name := range diskOnlySections {
			if at := sectionChangedAt(disk.Section(name)); at.After(changed) {
				changed = at
			}
		}
		server.freshness.Disk(disk.Name, changed)
	}
}

// startListingFreshness — au démarrage, en fond : l'en-tête et les disques une
// fois, pour que la première question ait déjà sa réponse.
func (server *Server) startListingFreshness() {
	server.rememberSelfListing()
	server.readDisks(server.presentDisks(context.Background())...)
}

// withFreshness — le listing servi porte la date que rend la question : sans
// elle, un disque parti (une date de MAINTENANT, qu'aucune section ne porte)
// ferait recopier l'ami à chaque question.
func (server *Server) withFreshness(served *listing.Listing) {
	if at, known := server.listingUpdatedAt(); known && at.After(served.Header.UpdatedAt) {
		served.Header.UpdatedAt = at
	}
}
