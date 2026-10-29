package api

// CE QU'UN DISQUE INTERNE APPORTE AU-DELÀ DES JEUX (chantier 24, #427) — ses
// cœurs, ses BIOS, ses paramètres et ses correctifs.
//
// Le porteur, 2026-10-22 : « on liste les jeux, cores, bios, etc. par source,
// c'est le même principe qu'avec les serveurs fédérés et ressources
// distantes. La seule règle qui change, c'est qu'un jeu qui se trouve sur
// /mnt/<…> peut être joué avec un émulateur qui est en local ».
//
// Donc les mêmes routes que pour un ami (`/servers/{id}/storage/{section}`),
// sous le disque : la liste, à la forme de celle d'ici, et l'import dans le
// dossier de ce serveur — une copie, jamais par-dessus ce qui existe. Une
// partie prend toujours sa console dans le dossier : ce qui n'est que sur un
// disque s'importe d'abord.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"gameflex/emuparams"
	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

// diskRoots — les sections d'un disque interne, rangées comme le dossier.
func diskRoots(disk docker.InternalDisk) libraryRoots {
	return libraryRoots{
		Roms: disk.RomsDir(), Cores: disk.Section("cores"), Bios: disk.Section("bios"),
		Patches: disk.Section("patches"), Params: disk.Dir,
	}
}

// handleDiskStorage — GET /api/v1/retrogaming/disks/{name}/storage/{section}
// (cores, bios, params, patches) : ce que ce disque en a, à la forme de la
// même liste pour ce serveur ou pour un ami — l'écran n'a qu'un dialecte.
func (server *Server) handleDiskStorage(w http.ResponseWriter, r *http.Request) {
	disk, ok := server.diskNamed(w, r, chi.URLParam(r, "name"))
	if !ok {
		return
	}
	section := chi.URLParam(r, "section")
	items := librarySection(diskRoots(disk), section)

	var shaped any
	switch section {
	case listing.Cores:
		shaped = coresSectionsFrom(items)
	case listing.Bios:
		shaped = biosSectionsFrom(items)
	case listing.Patches:
		shaped = remote.PatchSections(items, patchHere)
	case listing.Params:
		store := paramsStore()
		shaped = remote.ParamsSections(items, func(consoleID, emulator, presetID, presetName string) any {
			return store.OfferOf(disk.Name, consoleID, emulator, presetID, presetName)
		})
	default:
		writeError(w, http.StatusBadRequest, "section must be cores, bios, params or patches")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"consoles": shaped})
}

// handleDiskStorageImport — POST /api/v1/retrogaming/disks/{name}/storage/{section}
// avec `{consoleId, name}` (cœurs, BIOS, correctifs) ou `{consoleId, emulator,
// preset}` (paramètres) : l'élément du disque, copié dans le dossier de ce
// serveur. Un élément déjà là est REFUSÉ (409), jamais remplacé — la leçon de
// #142/#143.
func (server *Server) handleDiskStorageImport(w http.ResponseWriter, r *http.Request) {
	disk, ok := server.diskNamed(w, r, chi.URLParam(r, "name"))
	if !ok {
		return
	}
	section := chi.URLParam(r, "section")
	if section == listing.Params {
		server.importDiskParams(w, r, disk)

		return
	}

	var body struct {
		ConsoleID string `json:"consoleId"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corps illisible")

		return
	}
	consoleID := strings.ToLower(strings.TrimSpace(body.ConsoleID))
	name := strings.TrimSpace(body.Name)

	// Le nom devient un chemin : il passe par le registre, ou par le garde des
	// correctifs, jamais tel quel.
	var source, target string
	var companions [][2]string
	switch section {
	case listing.Cores:
		// Un fichier d'émulateur de bureau (chantier 29) : copié sous son nom.
		if retro_files.IsEmulatorFile(consoleID, name) {
			source = filepath.Join(disk.Section("cores"), consoleID, name)
			target = filepath.Join(server.retroCoresRoot(), consoleID, name)

			break
		}
		if !coreBelongsTo(consoleID, name) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("le core « %s » n'est pas déclaré pour %s", name, consoleID))

			return
		}
		file := retro_files.CoreFileName(name)
		source = filepath.Join(disk.Section("cores"), consoleID, file)
		target = filepath.Join(server.retroCoresRoot(), consoleID, file)
		// Sa variante Android, si le disque l'a : un téléphone la charge depuis
		// ce serveur (chantier 13, phase 5).
		android := retro_files.AndroidCoreFileName(name)
		companions = append(companions, [2]string{
			filepath.Join(disk.Section("cores"), consoleID, android),
			filepath.Join(server.retroCoresRoot(), consoleID, android),
		})
	case listing.Bios:
		if !biosBelongsTo(consoleID, name) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("le BIOS « %s » n'est pas attendu par %s", name, consoleID))

			return
		}
		path, _, err := retro_files.ResolveBiosFile(disk.Section("bios"), consoleID, name)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusBadRequest, "invalid bios reference: "+err.Error())

			return
		}
		source = path
		target = filepath.Join(server.retroBiosRoot(), consoleID, name)
	case listing.Patches:
		local, valid := patchPath(consoleID, name)
		if !valid {
			writeError(w, http.StatusBadRequest, "invalid patch")

			return
		}
		source = filepath.Join(disk.Section("patches"), consoleID, name)
		target = local
	default:
		writeError(w, http.StatusBadRequest, "section must be cores, bios, params or patches")

		return
	}

	if _, err := os.Stat(source); source == "" || err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("le disque %s n'a pas « %s »", disk.Name, name))

		return
	}
	if _, err := os.Stat(target); err == nil {
		writeError(w, http.StatusConflict,
			"ce fichier existe déjà : le supprimer d'abord pour en reprendre une version")

		return
	}

	written, err := copyIntoLibrary(source, target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())

		return
	}
	answer := map[string]any{
		"section": section, "consoleId": consoleID, "name": name, "sizeBytes": written, "source": disk.Name,
	}
	for _, companion := range companions {
		if _, err := os.Stat(companion[0]); err != nil {
			continue
		}
		if _, err := os.Stat(companion[1]); err == nil {
			continue
		}
		// La variante manquée ne défait pas le cœur, qui est copié : elle se
		// dit au journal, et l'écran la montre absente.
		size, err := copyIntoLibrary(companion[0], companion[1])
		if err != nil {
			slog.Warn("variante non copiée", "disk", disk.Name, "file", companion[0], "error", err)

			continue
		}
		answer["androidSizeBytes"] = size
	}
	slog.Info("élément importé d'un disque interne", "disk", disk.Name, "section", section,
		"console", consoleID, "name", name, "bytes", written)

	writeJSON(w, http.StatusOK, answer)
}

// importDiskParams — `{consoleId, emulator, preset}` : les valeurs choisies sur
// ce disque, ou un de ses préréglages perso, rangés ici en préréglage perso
// sous `<disque>-<préréglage>` (ou `<disque>`) — le même nommage que pour un
// ami, pour la même raison : deux sources qui ont chacune un « hd » ne
// s'écrasent pas.
func (server *Server) importDiskParams(w http.ResponseWriter, r *http.Request, disk docker.InternalDisk) {
	var body struct {
		ConsoleID string `json:"consoleId"`
		Emulator  string `json:"emulator"`
		Preset    string `json:"preset"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corps invalide")

		return
	}
	ref := emuparams.Ref{ConsoleID: body.ConsoleID, Emulator: body.Emulator}
	if !emuparams.IsShipped(ref) {
		writeError(w, http.StatusNotFound, "pas de paramètres pour "+ref.ConsoleID+"/"+ref.Emulator)

		return
	}
	lent, err := emuparams.Store{Root: disk.Dir}.Lend(ref, body.Preset)
	if err != nil {
		writeError(w, http.StatusNotFound, "ce disque n'a pas ce réglage")

		return
	}

	presetName := ""
	if body.Preset != "" {
		presetName = lent.Name
	}
	imported, ignored, err := paramsStore().Import(ref, consoleNames()[ref.ConsoleID],
		emuparams.ImportName(disk.Name, presetName), lent.Values)
	answerParamsImport(w, ref, disk.Name, imported, ignored, err)
}

// copyIntoLibrary — un fichier ou un dossier d'un disque interne, copié dans
// le dossier de ce serveur. La discipline de importIntoLibrary : un brouillon
// `.part`, renommé seulement quand tout est copié — un fichier tronqué qui
// porte le nom du vrai échoue plus tard, ailleurs, sans rien dire —, et ce qui
// est créé appartient au propriétaire de la bibliothèque (#146).
func copyIntoLibrary(source, target string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, fmt.Errorf("dossier impossible à créer : %w", err)
	}
	adoptParentOwner(filepath.Dir(target))

	partial := target + ".part"
	_ = os.RemoveAll(partial)
	written, err := copyTree(source, partial)
	if err == nil {
		err = os.Rename(partial, target)
	}
	if err != nil {
		_ = os.RemoveAll(partial)

		return 0, fmt.Errorf("copie interrompue : %w", err)
	}

	return written, nil
}

// copyTree — copie `source` (un fichier, ou un dossier et tout ce qu'il
// contient) vers `target`, qui n'existe pas encore. Les liens symboliques ne
// suivent pas : ils pointeraient hors du disque.
func copyTree(source, target string) (int64, error) {
	info, err := os.Stat(source)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return copyFile(source, target, info.Mode())
	}

	var total int64
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return err
			}
			adoptParentOwner(destination)

			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		written, err := copyFile(path, destination, fileInfo.Mode())
		total += written

		return err
	})

	return total, err
}

func copyFile(source, target string, mode fs.FileMode) (int64, error) {
	in, err := os.Open(source)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm()|0o200)
	if err != nil {
		return 0, err
	}
	written, err := io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	adoptParentOwner(target)

	return written, err
}
