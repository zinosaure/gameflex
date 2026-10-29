package api

// LES RESSOURCES INTERNES DANS L'API (chantier 23, #426) — les autres disques
// de jeux de cette machine, déclarés par un volume du compose
// (`<chemin>:/mnt/<nom>`, voir docker/intdisks.go).
//
// Trois règles, toutes exprimées sur la déclaration, jamais sur un nom :
//
//  1. UN JEU A LA MÊME RÉFÉRENCE PARTOUT : elle se dérive de sa console et de
//     son nom sur le disque (storage.RomID), quel que soit le disque. Favoris,
//     historique et sauvegardes le suivent donc d'un disque à l'autre.
//  2. IL SE JOUE DEPUIS LE PREMIER ENDROIT QUI L'A : la bibliothèque
//     principale, puis les disques internes dans l'ordre de leur nom. La
//     console (cœurs, BIOS, émulateur) est toujours celle de la bibliothèque
//     principale : c'est le droit qu'ont les ressources internes de « lancer
//     des jeux/consoles entre elles » (le porteur, 2026-10-22). Les cœurs et
//     les BIOS d'un disque s'importent d'abord (internal_disks_storage.go,
//     chantier 24).
//  3. UN DISQUE ABSENT EST SAUTÉ : il apparaît, sans jeux, et rien ne s'en
//     lance.
//
// Pour le propriétaire, chaque disque est une source à part dans les
// sélecteurs (`?disk=<nom>`, `disk:<nom>`). Pour un ami, la bibliothèque
// principale et les disques internes ne font qu'un, dans toutes les sections
// (le porteur : « qui pour lui apparaissent comme un seul »).

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/gamestatus"
	"gameflex/storage/listing"
)

// disks — les disques internes déclarés, dans l'ordre de leur nom. Un test
// les donne par `server.internalDisks` ; sinon c'est le manager qui les
// découvre (/mnt).
func (server *Server) disks(ctx context.Context) []docker.InternalDisk {
	if server.internalDisks != nil {
		return server.internalDisks(ctx)
	}
	if server.manager == nil {
		return []docker.InternalDisk{}
	}

	return server.manager.InternalDisks(ctx)
}

// presentDisks — les disques internes qui portent des jeux, dans l'ordre.
func (server *Server) presentDisks(ctx context.Context) []docker.InternalDisk {
	present := []docker.InternalDisk{}
	for _, disk := range server.disks(ctx) {
		if disk.Present {
			present = append(present, disk)
		}
	}

	return present
}

// findRom — où se trouve un jeu : la bibliothèque principale d'abord, puis
// les disques internes présents dans l'ordre de leur nom. `disk` est nil
// quand il est dans la bibliothèque principale.
func (server *Server) findRom(ctx context.Context, consoleID, reference string) (path string, isDir bool, disk *docker.InternalDisk, err error) {
	path, isDir, err = retro_files.ResolveCatalogRom(server.retroRomsRoot(), consoleID, reference)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return path, isDir, nil, err
	}
	for _, candidate := range server.presentDisks(ctx) {
		path, isDir, err = retro_files.ResolveCatalogRom(candidate.RomsDir(), consoleID, reference)
		if err == nil {
			found := candidate

			return path, isDir, &found, nil
		}
	}

	return "", false, nil, os.ErrNotExist
}

// diskNamed — le disque `name` s'il est déclaré ET présent ; sinon la
// réponse d'erreur est déjà écrite.
func (server *Server) diskNamed(w http.ResponseWriter, r *http.Request, name string) (docker.InternalDisk, bool) {
	declared := false
	for _, candidate := range server.disks(r.Context()) {
		if candidate.Name != name {
			continue
		}
		declared = true
		if candidate.Present {
			return candidate, true
		}
	}
	if declared {
		writeError(w, http.StatusServiceUnavailable, "the internal disk "+name+" is not there: it has no roms folder")
	} else {
		writeError(w, http.StatusNotFound, "no internal disk named "+name)
	}

	return docker.InternalDisk{}, false
}

// internalDiskView — un disque tel que la page le montre.
type internalDiskView struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
	// Consoles — le nombre de jeux par console, lu dans son catalogue.
	Consoles map[string]int `json:"consoles"`
	// ScannedAt — quand son catalogue a été construit.
	ScannedAt *time.Time `json:"scannedAt,omitempty"`
}

// handleInternalDisks — GET /api/v1/retrogaming/disks : les disques internes,
// dans l'ordre, avec leur état et leurs jeux par console. Ouvert à tous les
// comptes, comme la liste des jeux : c'est le sélecteur de la page des jeux
// qui s'en sert.
func (server *Server) handleInternalDisks(w http.ResponseWriter, r *http.Request) {
	views := []internalDiskView{}
	for _, disk := range server.disks(r.Context()) {
		view := internalDiskView{Name: disk.Name, Present: disk.Present, Consoles: map[string]int{}}
		if disk.Present {
			for _, summary := range retro_files.CountCatalog(disk.RomsDir()) {
				view.Consoles[summary.ID] = summary.Count
			}
			if at := retro_files.CatalogScannedAt(disk.RomsDir()); !at.IsZero() {
				view.ScannedAt = &at
			}
		}
		views = append(views, view)
	}

	writeJSON(w, http.StatusOK, views)
}

// handleInternalDiskRescan — POST /api/v1/retrogaming/disks/{name}/rescan :
// relire un disque en entier. Le catalogue se relit déjà de lui-même quand un
// dossier change ; ce bouton sert à ce qu'il ne voit pas (un fichier réécrit
// sur place).
func (server *Server) handleInternalDiskRescan(w http.ResponseWriter, r *http.Request) {
	disk, ok := server.diskNamed(w, r, chi.URLParam(r, "name"))
	if !ok {
		return
	}
	if err := retro_files.RescanCatalog(disk.RomsDir()); err != nil {
		writeError(w, http.StatusInternalServerError, "rescan failed: "+err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"name": disk.Name, "rescanned": true})
}

// serveDiskRoms — les jeux d'UN disque pour UNE console : ce que montre la
// liste quand son sélecteur désigne ce disque.
func (server *Server) serveDiskRoms(w http.ResponseWriter, r *http.Request, name, consoleID, query string) {
	disk, ok := server.diskNamed(w, r, name)
	if !ok {
		return
	}
	roms, err := retro_files.ListCatalog(disk.RomsDir(), consoleID, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}
	// Le statut d'un jeu de disque vit dans le dossier de ce serveur, sous
	// son identifiant (chantier 30) : le même jeu, où qu'il soit.
	statuses := server.consoleStatus(consoleID)
	for index := range roms {
		roms[index].Disk = disk.Name
		if roms[index].Files == nil {
			roms[index].Files = []string{}
		}
		roms[index].Status = statusView(consoleID, statuses, roms[index].Reference)
	}

	writeJSON(w, http.StatusOK, roms)
}

// mergedConsoleCounts — le nombre de jeux par console de la bibliothèque
// principale ET des disques internes : un jeu présent à deux endroits ne
// compte qu'une fois. C'est ce qui garde une console dans le menu quand ses
// jeux ne sont plus que sur un disque interne.
//
// UN JEU DE DISQUE SE COMPARE À CE QUE LA LISTE MONTRE — le listing de ce
// serveur, d'où viennent `local` et la liste elle-même —, jamais au disque.
// Le listing ne se relit qu'au parcours : des jeux retirés de la bibliothèque
// à la main y restent jusque-là. Comparés au disque, ils comptaient deux fois
// (mesuré le 2026-10-22 : 21 566 jeux NES annoncés pour 10 783).
func (server *Server) mergedConsoleCounts(ctx context.Context, local map[string]int) map[string]int {
	disks := server.presentDisks(ctx)
	if len(disks) == 0 {
		return local
	}
	merged := map[string]int{}
	seen := map[string]bool{}
	for consoleID, count := range local {
		merged[consoleID] = count
		for _, item := range server.selfItems(listing.Roms, consoleID) {
			seen[consoleID+"/"+item.ID] = true
		}
	}
	for _, disk := range disks {
		for _, summary := range retro_files.CountCatalog(disk.RomsDir()) {
			roms, err := retro_files.ListCatalog(disk.RomsDir(), summary.ID, "")
			if err != nil {
				continue
			}
			for _, rom := range roms {
				key := summary.ID + "/" + rom.Reference
				if seen[key] {
					continue
				}
				seen[key] = true
				merged[summary.ID]++
			}
		}
	}

	return merged
}

// withInternalDisks — le listing servi à un ami : la bibliothèque et les
// disques internes « qui pour lui apparaissent comme un seul » (le porteur).
// Un jeu présent à deux endroits n'y figure qu'une fois — sa référence est la
// même partout —, et l'ami le télécharge ou le joue là où il est
// (`findRom`). Les compteurs et la date suivent : un jeu ajouté sur un disque
// fait avancer la date, et l'ami recopie la liste.
func (server *Server) withInternalDisks(ctx context.Context, served *listing.Listing) {
	disks := server.presentDisks(ctx)
	if len(disks) == 0 {
		return
	}
	if served.Roms == nil {
		served.Roms = map[string][]listing.Item{}
	}
	seen := map[string]bool{}
	for consoleID, items := range served.Roms {
		for _, item := range items {
			seen[consoleID+"/"+item.ID] = true
		}
	}

	latest := served.Header.UpdatedAt
	grown := map[string]bool{}
	for _, disk := range disks {
		root := disk.RomsDir()
		for _, summary := range retro_files.CountCatalog(root) {
			roms, err := retro_files.ListCatalog(root, summary.ID, "")
			if err != nil {
				continue
			}
			reports := server.consoleStatus(summary.ID).Games
			for _, rom := range roms {
				key := summary.ID + "/" + rom.Reference
				if seen[key] {
					continue
				}
				seen[key] = true
				grown[summary.ID] = true
				served.Roms[summary.ID] = append(served.Roms[summary.ID], listing.Item{
					ID: rom.Reference, Name: rom.Title, Files: rom.Files, Dir: rom.Folder, Size: rom.SizeBytes,
					Reports: gamestatus.Public(reports[rom.Reference]),
				})
			}
		}
		if at := retro_files.CatalogScannedAt(root); at.After(latest) {
			latest = at
		}
	}

	counts := map[string]int{}
	for consoleID, items := range served.Roms {
		if grown[consoleID] {
			sort.SliceStable(items, func(left, right int) bool { return items[left].Name < items[right].Name })
		}
		counts[consoleID] = len(items)
	}
	if served.Header.Sections == nil {
		served.Header.Sections = map[string]listing.SectionInfo{}
	}
	section := served.Header.Sections[listing.Roms]
	section.Consoles = counts
	if latest.After(section.UpdatedAt) {
		section.UpdatedAt = latest
	}
	served.Header.Sections[listing.Roms] = section
	if latest.After(served.Header.UpdatedAt) {
		served.Header.UpdatedAt = latest
	}

	// Les autres sections (chantier 24) : un cœur, un BIOS, un correctif ou des
	// paramètres qu'un disque est seul à avoir s'ajoutent sous leur nom ; ce
	// que la bibliothèque a déjà passe devant.
	for _, name := range diskOnlySections {
		mergeDiskSection(served, name, disks)
	}
}

// diskOnlySections — ce qu'un disque apporte au-delà des jeux.
var diskOnlySections = []string{listing.Cores, listing.Bios, listing.Patches, listing.Params}

// mergeDiskSection — une section du listing servi, et ce que les disques
// ajoutent. Sa date avance avec le dernier changement d'un disque : l'ami
// recopie alors la liste.
func mergeDiskSection(served *listing.Listing, name string, disks []docker.InternalDisk) {
	items := map[string][]listing.Item{}
	seen := map[string]bool{}
	for consoleID, list := range served.Section(name) {
		items[consoleID] = append([]listing.Item{}, list...)
		for _, item := range list {
			seen[consoleID+"/"+item.Name] = true
		}
	}
	latest := served.Header.Sections[name].UpdatedAt
	for _, disk := range disks {
		for consoleID, list := range librarySection(diskRoots(disk), name) {
			for _, item := range list {
				key := consoleID + "/" + item.Name
				if seen[key] {
					continue
				}
				seen[key] = true
				items[consoleID] = append(items[consoleID], item)
			}
		}
		if at := sectionChangedAt(disk.Section(name)); at.After(latest) {
			latest = at
		}
	}
	served.SetSection(name, items, latest)
}

// sectionChangedAt — le dernier changement d'une section d'un disque : le plus
// récent de son dossier, de ses dossiers de console et de ce qu'ils
// contiennent. Un fichier ajouté, retiré ou réécrit le fait avancer. Rien
// n'est lu au-delà : un dossier de jeu ou de correctif compte pour un.
func sectionChangedAt(dir string) time.Time {
	var latest time.Time
	note := func(info os.FileInfo) {
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return latest
	}
	note(info)
	consoleDirs, _ := os.ReadDir(dir)
	for _, consoleDir := range consoleDirs {
		if info, err := consoleDir.Info(); err == nil {
			note(info)
		}
		if !consoleDir.IsDir() {
			continue
		}
		entries, _ := os.ReadDir(filepath.Join(dir, consoleDir.Name()))
		for _, entry := range entries {
			if info, err := entry.Info(); err == nil {
				note(info)
			}
		}
	}

	return latest.UTC()
}

// libraryRootsInOrder — où chercher un élément pour un ami : la bibliothèque,
// puis les disques internes présents, dans l'ordre de leur nom.
func (server *Server) libraryRootsInOrder(ctx context.Context) []libraryRoots {
	roots := []libraryRoots{server.selfRoots()}
	for _, disk := range server.presentDisks(ctx) {
		roots = append(roots, diskRoots(disk))
	}

	return roots
}

// disksUpdatedAt — la date la plus récente entre `at` et les disques internes
// (le catalogue de leurs jeux, et leurs autres sections) : ce que l'ami
// compare pour savoir s'il doit recopier.
func (server *Server) disksUpdatedAt(ctx context.Context, at time.Time) time.Time {
	for _, disk := range server.presentDisks(ctx) {
		if scanned := retro_files.CatalogScannedAt(disk.RomsDir()); scanned.After(at) {
			at = scanned
		}
		for _, name := range diskOnlySections {
			if changed := sectionChangedAt(disk.Section(name)); changed.After(at) {
				at = changed
			}
		}
	}

	return at
}
