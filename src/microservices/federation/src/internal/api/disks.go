package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gameflex/emuparams"
	"gameflex/federation/internal/catalog"
	"gameflex/federation/internal/library"
	"gameflex/federation/internal/store"
	"gameflex/metadata"
	"gameflex/storage"
	"gameflex/storage/listing"
	"gameflex/storage/remote"

	"github.com/go-chi/chi/v5"
)

// SERVIR UN ÉLÉMENT DEPUIS SES SOURCES (chantier 61). Toutes les routes de fichiers de la surface fédérée — un jeu, un
// BIOS, un cœur, un paquet, un correctif — passent par les copies de l'élément, de la meilleure à la moins bonne
// (catalog/rank.go) : `?source=<id>` en tête si l'appareil en a choisi une. La première qui répond sert ; une source qui
// ne s'ouvre pas est marquée indisponible et la suivante prend le relais ; chaque transfert réussi donne son débit à la
// mesure de sa source. Un transfert déjà commencé ne peut plus changer de source : ses en-têtes sont partis.
//
// Les sauvegardes ne se partagent pas encore : la fédération de GameFlex ne sait pas les transporter.

func preferredSource(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.URL.Query().Get("source"), 10, 64)

	return id
}

// countingWriter — compte ce qui part, pour mesurer le débit d'une source.
type countingWriter struct {
	io.Writer
	written int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	n, err := w.Writer.Write(data)
	w.written += int64(n)

	return n, err
}

// serveCopies — sert l'élément depuis la première de ses copies qui répond. `file` : le chemin du fichier à servir
// dans la source (le locator par défaut).
func (s *Server) serveCopies(w http.ResponseWriter, r *http.Request, copies []catalog.Copy, file func(catalog.Copy) string) {
	if len(copies) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "not found")

		return
	}
	var lastErr error
	for _, copy := range copies {
		target := file(copy)
		if target == "" {
			continue
		}
		if r.Method == http.MethodHead {
			s.writeHeaders(w, copy, target)

			return
		}
		source, err := copy.Open()
		if err != nil {
			s.Catalog.Failed(copy.Source, err)
			lastErr = err

			continue
		}
		opener, ok := source.(storage.Opener)
		if !ok {
			lastErr = errors.New("this resource cannot be read")

			continue
		}
		started := time.Now()
		if copy.Where.Dir && target == copy.Where.Path {
			if _, err := source.List(target); err != nil {
				s.Catalog.Failed(copy.Source, err)
				lastErr = err

				continue
			}
			counter := &countingWriter{Writer: w}
			s.writeHeaders(w, copy, target)
			s.zipFolder(counter, r, source, opener, target)
			s.Catalog.Served(copy.Source, counter.written, time.Since(started))

			return
		}
		in, err := opener.Open(target)
		if err != nil {
			s.Catalog.Failed(copy.Source, err)
			lastErr = err

			continue
		}
		s.writeHeaders(w, copy, target)
		written, err := io.Copy(w, in)
		in.Close()
		if err == nil {
			s.Catalog.Served(copy.Source, written, time.Since(started))
		}

		return
	}
	message := "no source could serve it"
	if lastErr != nil {
		message += ": " + lastErr.Error()
	}
	writeError(w, http.StatusBadGateway, "resource_unavailable", message)
}

func (s *Server) writeHeaders(w http.ResponseWriter, copy catalog.Copy, target string) {
	name := strings.ReplaceAll(path.Base(target), `"`, "")
	w.Header().Set("X-GameFlex-Source", strconv.FormatInt(copy.Source, 10))
	if copy.Where.Dir && target == copy.Where.Path {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.zip"`)

		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if copy.Where.Size > 0 && copy.Where.Android == "" {
		w.Header().Set("Content-Length", strconv.FormatInt(copy.Where.Size, 10))
	}
}

func locatorPath(copy catalog.Copy) string { return copy.Where.Path }

// rom — le fichier d'un jeu ; un jeu en plusieurs fichiers part zippé, comme chez un kernel.
func (s *Server) rom(w http.ResponseWriter, r *http.Request) {
	s.serveCopies(w, r, s.Catalog.Candidates(listing.Roms, chi.URLParam(r, "consoleId"), chi.URLParam(r, "ref"), preferredSource(r)), locatorPath)
}

func (s *Server) bios(w http.ResponseWriter, r *http.Request) {
	s.serveCopies(w, r, s.Catalog.Candidates(listing.Bios, chi.URLParam(r, "consoleId"), chi.URLParam(r, "name"), preferredSource(r)), locatorPath)
}

// core — le fichier d'un cœur ; `?platform=android` sa variante Android.
func (s *Server) core(w http.ResponseWriter, r *http.Request) {
	consoleID, name := chi.URLParam(r, "consoleId"), chi.URLParam(r, "name")
	if !library.CoreBelongsTo(consoleID, name) {
		writeError(w, http.StatusBadRequest, "emulator_unknown", fmt.Sprintf("the core %q is not declared for %s", name, consoleID))

		return
	}
	android := r.URL.Query().Get("platform") == "android"
	s.serveCopies(w, r, s.Catalog.Candidates(listing.Cores, consoleID, name, preferredSource(r)), func(copy catalog.Copy) string {
		if android {
			return copy.Where.Android
		}
		if copy.Item.Variants["linux"] == 0 && copy.Where.Path == "" {
			return ""
		}

		return copy.Where.Path
	})
}

// bundle — un émulateur ou un paquet du téléphone d'une console récente.
func (s *Server) bundle(w http.ResponseWriter, r *http.Request) {
	consoleID, file := chi.URLParam(r, "consoleId"), chi.URLParam(r, "file")
	if !library.IsEmulatorFile(consoleID, file) {
		writeError(w, http.StatusBadRequest, "not_found", "unknown bundle")

		return
	}
	s.serveCopies(w, r, s.Catalog.Candidates(listing.Cores, consoleID, file, preferredSource(r)), locatorPath)
}

func (s *Server) patch(w http.ResponseWriter, r *http.Request) {
	consoleID, name := chi.URLParam(r, "consoleId"), chi.URLParam(r, "name")
	if !remote.PatchName(consoleID) || !remote.PatchName(name) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid patch")

		return
	}
	s.serveCopies(w, r, s.Catalog.Candidates(listing.Patches, consoleID, name, preferredSource(r)), locatorPath)
}

func (s *Server) params(w http.ResponseWriter, r *http.Request) {
	ref := emuparams.Ref{ConsoleID: chi.URLParam(r, "consoleId"), Emulator: chi.URLParam(r, "emulator")}
	if !emuparams.IsShipped(ref) {
		writeError(w, http.StatusNotFound, "not_found", "no parameters for "+ref.ConsoleID+"/"+ref.Emulator)

		return
	}
	lent, err := s.Catalog.LendParams(ref, chi.URLParam(r, "preset"), preferredSource(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_lent", "nothing lent here")

		return
	}
	writeJSON(w, http.StatusOK, lent)
}

// ── Les fiches des jeux ────────────────────────────────────────────────────

// sheetGame — un jeu dont la fiche se lit ici (chantier 61, P3, question 2 A) :
//
//   - une copie sur un disque qui porte déjà sa fiche (`metadata/<console>/<gameid>`, une bibliothèque GameFlex) : elle
//     se lit là ;
//   - sinon, la fiche que le service fait lui-même avec ses comptes (metadataAccounts), dans son volume : celle d'un jeu
//     de disque sans fiche (le disque est monté en lecture seule), et celle d'un jeu d'un partage — son fichier n'est pas
//     sur cette machine, il se désigne par sa référence et se cherche par son nom.
func (s *Server) sheetGame(w http.ResponseWriter, r *http.Request) (consoleID, reference, gamePath string, roots metadata.Roots, ok bool) {
	consoleID, reference = chi.URLParam(r, "consoleId"), chi.URLParam(r, "ref")
	gamePath, roots, ok = s.sheetPlace(consoleID, reference)
	if !ok {
		writeError(w, http.StatusNotFound, "game_not_found", "rom not found")
	}

	return consoleID, reference, gamePath, roots, ok
}

// sheetPlace — le fichier d'un jeu et les racines de sa fiche (voir sheetGame) ; faux si le service ne l'a pas.
func (s *Server) sheetPlace(consoleID, reference string) (gamePath string, roots metadata.Roots, ok bool) {
	copies := s.Catalog.Candidates(listing.Roms, consoleID, reference, 0)
	if len(copies) == 0 {
		return "", roots, false
	}
	own := metadata.Roots{Server: s.DataDir, Game: s.DataDir}
	for _, copy := range copies {
		if copy.Kind != "disk" {
			continue
		}
		// Un jeu en dossier se lit depuis son dossier, comme chez GameFlex.
		gamePath = filepath.Join(copy.Resource.Address, filepath.FromSlash(copy.Where.Path))
		sheet := filepath.Join(copy.Resource.Address, "metadata", consoleID, metadata.GameKey(gamePath, reference))
		if _, err := os.Stat(sheet); err == nil {
			return gamePath, metadata.Roots{Server: s.DataDir, Game: copy.Resource.Address}, true
		}

		return gamePath, own, true
	}
	// Un jeu d'un partage : un chemin qui n'existe pas ici, au nom de son fichier — sa clé est sa référence.
	gamePath = filepath.Join(s.DataDir, "shared-games", consoleID, path.Base(copies[0].Where.Path))

	return gamePath, own, true
}

func (s *Server) gameSheet(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, roots, ok := s.sheetGame(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, metadata.Info(r.Context(), roots, s.metadataAccounts(), consoleID, gamePath, reference))
}

func (s *Server) gameImage(w http.ResponseWriter, r *http.Request) {
	consoleID, reference, gamePath, roots, ok := s.sheetGame(w, r)
	if !ok {
		return
	}
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))
	file, err := metadata.Image(r.Context(), roots, s.metadataAccounts(), consoleID, gamePath, reference, chi.URLParam(r, "kind"), width)
	if err != nil || file == "" {
		writeError(w, http.StatusNotFound, "no_image", "no image")

		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, file)
}

// ── Les ressources internes ────────────────────────────────────────────────

func (s *Server) presentDisks() []library.Disk {
	if s.DisksDir == "" {
		return nil
	}

	return library.Present(library.Disks(s.DisksDir))
}

// DiskResources — les disques présents, vus comme des ressources du catalogue.
func (s *Server) DiskResources() []store.Resource {
	resources := []store.Resource{}
	for _, disk := range s.presentDisks() {
		resources = append(resources, disk.Resource())
	}

	return resources
}

type diskView struct {
	Name    string        `json:"name"`
	Present bool          `json:"present"`
	Pinned  bool          `json:"pinned"`
	State   catalog.State `json:"state"`
}

func (s *Server) listDisks(w http.ResponseWriter, _ *http.Request) {
	states := s.Catalog.States()
	views := []diskView{}
	if s.DisksDir != "" {
		for _, disk := range library.Disks(s.DisksDir) {
			views = append(views, diskView{Name: disk.Name, Present: disk.Present, Pinned: s.Catalog.Pinned(disk.ID()), State: states[disk.ID()]})
		}
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) diskNamed(w http.ResponseWriter, r *http.Request) (library.Disk, bool) {
	for _, disk := range s.presentDisks() {
		if disk.Name == chi.URLParam(r, "name") {
			return disk, true
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "no such disk")

	return library.Disk{}, false
}

func (s *Server) rescanDisk(w http.ResponseWriter, r *http.Request) {
	if disk, ok := s.diskNamed(w, r); ok {
		writeJSON(w, http.StatusAccepted, map[string]any{"started": s.Catalog.Rescan(disk.Resource())})
	}
}

// pinBody — épingler une source en tête de tout (le porteur, 2026-11-18 : « Oui bonne idée pour épingler une
// source »).
type pinBody struct {
	Pinned bool `json:"pinned"`
}

func (s *Server) pinDisk(w http.ResponseWriter, r *http.Request) {
	disk, ok := s.diskNamed(w, r)
	var body pinBody
	if !ok || json.NewDecoder(r.Body).Decode(&body) != nil {
		if ok {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		}

		return
	}
	if err := s.Catalog.Pin(disk.ID(), body.Pinned); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pinned": body.Pinned})
}

func (s *Server) pinResource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	var body pinBody
	if _, found := s.resourceByID(id); !ok || !found || json.NewDecoder(r.Body).Decode(&body) != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid resource or body")

		return
	}
	if err := s.Catalog.Pin(id, body.Pinned); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pinned": body.Pinned})
}
