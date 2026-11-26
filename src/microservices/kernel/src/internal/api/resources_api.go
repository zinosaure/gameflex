package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gameflex/consoles"
	"gameflex/facade"
	"gameflex/kernel/internal/retro_files"
	"gameflex/resources"
	"gameflex/storage"
	"gameflex/storage/listing"

	"github.com/go-chi/chi/v5"
)

// LES RESSOURCES EXTERNES (Samba, WebDAV, kDrive, FTP, SFTP ; `#808`, 2026-11-20) — gameflex-federation intégré à
// GameFlex : « Un serveur Gameflex peut faire office de serveur fédéré seul » (le porteur). Réglages › Ressources les
// ajoute (edge : settings/people/Resources.jsx) ; leurs sections s'ajoutent au listing que ce serveur sert à ses amis,
// DERRIÈRE la bibliothèque et les disques internes — un élément qu'ils ont déjà n'est pas repris —, et un ami télécharge
// ce qu'elles seules portent par les routes habituelles (`/roms`, `/bios`, `/cores`, `/bundles`, `/patches`,
// `/params`), servi depuis la meilleure de ses copies. Leurs jeux ne se lancent pas ici : comme ceux d'un partage, ils
// s'importent avant de se jouer. Le code est celui du module partagé gameflex/resources, que le kernel du téléphone
// branche de même.
//
// Le magasin et le catalogue s'ouvrent au premier besoin : `databases/` (les ressources, leurs mots de passe scellés)
// et `cache/resources/` (ce que leur dernier parcours a trouvé).

// externalResources — le magasin et le catalogue des ressources, ouverts une fois.
func (server *Server) externalResources() (*resources.Store, *resources.Catalog, error) {
	server.resourcesOnce.Do(func() {
		store, err := resources.Open(filepath.Join(server.storageRoot, "databases"))
		if err != nil {
			server.resourcesErr = err
			slog.Error("ressources externes illisibles", "error", err)

			return
		}
		dataDir := filepath.Join(server.storageRoot, "cache", "resources")
		if err := os.MkdirAll(dataDir, 0o700); err != nil {
			server.resourcesErr = err

			return
		}
		server.resourceStore = store
		server.resourceCatalog = resources.New(store, dataDir, server.federationHostname)
	})

	return server.resourceStore, server.resourceCatalog, server.resourcesErr
}

// handleListResources — GET /api/v1/retrogaming/resources (administrateur).
func (server *Server) handleListResources(w http.ResponseWriter, _ *http.Request) {
	_, catalog, err := server.externalResources()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	views, err := catalog.Views()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, views)
}

type resourceBody struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Address  string `json:"address"`
	Folder   string `json:"folder"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (body resourceBody) resource() resources.Resource {
	return resources.Resource{Name: body.Name, Kind: body.Kind, Address: body.Address, Folder: body.Folder,
		Username: body.Username, Password: body.Password}
}

func resourceID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	return id, err == nil && id > 0
}

// handleAddResource — POST /api/v1/retrogaming/resources (administrateur) : l'enregistre, puis la parcourt en fond.
func (server *Server) handleAddResource(w http.ResponseWriter, r *http.Request) {
	store, catalog, err := server.externalResources()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	var body resourceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid resource")

		return
	}
	id, err := store.AddResource(body.resource())
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "resource_invalid", err.Error())

		return
	}
	added, _ := catalog.Find(id)
	catalog.Rescan(added)
	writeJSON(w, http.StatusCreated, catalog.ViewOf(added))
}

// handleUpdateResource — PATCH /api/v1/retrogaming/resources/{id} (administrateur) : un mot de passe vide garde l'ancien ;
// la connexion gardée ne vaut plus, et la ressource se relit.
func (server *Server) handleUpdateResource(w http.ResponseWriter, r *http.Request) {
	store, catalog, err := server.externalResources()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	id, ok := resourceID(r)
	var body resourceBody
	if !ok || json.NewDecoder(r.Body).Decode(&body) != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid resource")

		return
	}
	updated := body.resource()
	updated.ID = id
	if err := store.UpdateResource(updated); errors.Is(err, os.ErrNotExist) {
		writeErrorCode(w, http.StatusNotFound, "not_found", "no such resource")

		return
	} else if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "resource_invalid", err.Error())

		return
	}
	catalog.Forget(id, false)
	current, _ := catalog.Find(id)
	catalog.Rescan(current)
	writeJSON(w, http.StatusOK, catalog.ViewOf(current))
}

// handleRemoveResource — DELETE /api/v1/retrogaming/resources/{id} (administrateur) : la ressource, et ce que son
// parcours avait trouvé ; le listing servi ne la porte plus.
func (server *Server) handleRemoveResource(w http.ResponseWriter, r *http.Request) {
	store, catalog, err := server.externalResources()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	id, ok := resourceID(r)
	if !ok {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid id")

		return
	}
	if err := store.RemoveResource(id); errors.Is(err, os.ErrNotExist) {
		writeErrorCode(w, http.StatusNotFound, "not_found", "no such resource")

		return
	} else if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "resources_unavailable", err.Error())

		return
	}
	catalog.Forget(id, true)
	writeJSON(w, http.StatusOK, map[string]any{"removed": id})
}

// handleRescanResource — POST /api/v1/retrogaming/resources/{id}/rescan (administrateur) : la relire, en fond.
func (server *Server) handleRescanResource(w http.ResponseWriter, r *http.Request) {
	_, catalog, err := server.externalResources()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	id, _ := resourceID(r)
	resource, found := catalog.Find(id)
	if !found {
		writeErrorCode(w, http.StatusNotFound, "not_found", "no such resource")

		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"started": catalog.Rescan(resource)})
}

// followResources — le sujet `resources` du centre de messages (administrateur) : la liste, et chaque changement d'un
// parcours (commencé, fini, en échec) — la page n'interroge jamais.
func (server *Server) followResources(ctx context.Context, conn *facade.Conn) {
	_, catalog, err := server.externalResources()
	if err != nil {
		return
	}
	wake, stop := catalog.Watch()
	defer stop()

	facade.PushOnChange(ctx, conn, facade.Push{Type: "resources", Topic: "resources"}, wake, func() any {
		views, err := catalog.Views()
		if err != nil {
			return []resources.View{}
		}

		return views
	})
}

// withResources — les sections des ressources dans le listing servi, derrière la bibliothèque et les disques.
func (server *Server) withResources(served *listing.Listing) {
	if _, catalog, err := server.externalResources(); err == nil {
		catalog.MergeInto(served)
	}
}

// resourcesUpdatedAt — le dernier changement du catalogue des ressources ; zéro s'il n'a jamais rien trouvé.
func (server *Server) resourcesUpdatedAt() time.Time {
	_, catalog, err := server.externalResources()
	if err != nil {
		return time.Time{}
	}

	return catalog.UpdatedAt()
}

// serveFromResources — un élément qu'aucune bibliothèque d'ici n'a, servi depuis ses ressources ; faux s'il n'y est
// pas non plus (l'appelant répond « introuvable »).
func (server *Server) serveFromResources(w http.ResponseWriter, r *http.Request, section, consoleID, key string, file func(resources.Copy) string) bool {
	_, catalog, err := server.externalResources()
	if err != nil {
		return false
	}
	copies := catalog.Candidates(section, consoleID, key, resources.PreferredSource(r))

	return catalog.Serve(w, r, copies, file, func(status int, code, message string) {
		writeErrorCode(w, status, code, message)
	})
}

// ── Réglages › Consoles › une console › Jeux : la ressource comme source ─────

// serveResourceRoms — GET /api/v1/retrogaming/{consoleId}/roms?resource=<id> (tout compte : le Catalogue les montre à
// tous, comme les jeux d'ici) : les jeux de la console qu'une ressource porte, d'après son dernier parcours, sous la
// référence `res<id>:` (resources.Reference) ; « déjà ici » quand la bibliothèque ou un disque a le même.
func (server *Server) serveResourceRoms(w http.ResponseWriter, r *http.Request, resource, consoleID, query string) {
	_, catalog, err := server.externalResources()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	id, err := strconv.ParseInt(resource, 10, 64)
	if _, found := catalog.Find(id); err != nil || !found {
		writeErrorCode(w, http.StatusNotFound, "not_found", "no such resource")

		return
	}
	items, where := catalog.Games(id, consoleID)
	needle := strings.ToLower(strings.TrimSpace(query))
	roms := []retro_files.RomEntry{}
	for index, item := range items {
		if needle != "" && !strings.Contains(strings.ToLower(item.Name), needle) {
			continue
		}
		files := item.Files
		if files == nil {
			files = []string{}
		}
		rom := retro_files.RomEntry{Reference: resources.Reference(id, item.ID), Title: item.Name, Files: files, SizeBytes: item.Size, Folder: item.Dir}
		if index < len(where) {
			rom.SizeBytes, rom.Folder = where[index].Size, where[index].Dir
		}
		if _, _, _, err := server.findRom(r.Context(), consoleID, item.ID); err == nil {
			rom.Here = true
		}
		roms = append(roms, rom)
	}
	writeJSON(w, http.StatusOK, roms)
}

// handleResourceStorageImport — POST /api/v1/retrogaming/resources/{id}/storage/roms (administrateur, par la file des
// jeux, commande `download` route `resource`) : un jeu d'une ressource copié dans la bibliothèque — le fichier tel
// quel, ou le dossier zippé par la ressource et déplié ici, comme le jeu d'un ami (importIntoLibrary). Les jeux
// seulement : les BIOS, cœurs et correctifs d'une ressource se prêtent aux amis, ils ne s'importent pas encore d'ici.
func (server *Server) handleResourceStorageImport(w http.ResponseWriter, r *http.Request) {
	_, catalog, err := server.externalResources()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	id, ok := resourceID(r)
	resource, found := catalog.Find(id)
	if !ok || !found {
		writeErrorCode(w, http.StatusNotFound, "not_found", "no such resource")

		return
	}
	if section := chi.URLParam(r, "section"); section != listing.Roms {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "only games are imported from a resource")

		return
	}
	var body struct {
		ConsoleID string   `json:"consoleId"`
		Reference string   `json:"reference"`
		Title     string   `json:"title"`
		Files     []string `json:"files"`
		Folder    bool     `json:"folder"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "corps illisible")

		return
	}
	body.ConsoleID = strings.ToLower(strings.TrimSpace(body.ConsoleID))
	body.Reference = strings.TrimSpace(body.Reference)
	// La référence que la liste rend (`res<id>:<référence>`) : celle de la ressource derrière le préfixe.
	if _, inner, ok := resources.ParseReference(body.Reference); ok {
		body.Reference = inner
	}
	// La console devient un dossier sous roms/ : celle du registre, jamais une autre chaîne.
	if !consoles.Exists(body.ConsoleID) || body.Reference == "" {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "consoleId and reference are required")

		return
	}
	// Le nom du fichier écrit ici est DÉRIVÉ du titre, puis vérifié — jamais recopié tel quel.
	files := body.Files
	if body.Folder {
		files = nil
	}
	name, err := storage.RomFileName(body.Title, files)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	target := filepath.Join(server.retroRomsRoot(), body.ConsoleID, name)
	if _, err := os.Stat(target); err == nil {
		writeErrorCode(w, http.StatusConflict, "already_installed", "ce fichier existe déjà : le supprimer d'abord pour en reprendre une version")

		return
	}
	open := func() (io.ReadCloser, error) {
		in, _, err := catalog.Open(listing.Roms, body.ConsoleID, body.Reference, id)

		return in, err
	}
	written, err := importIntoLibrary(r.Context(), open, target, zipPolicyFor(listing.Roms, name, body.Files, body.Folder))
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "resource_unavailable", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"section": listing.Roms, "consoleId": body.ConsoleID, "name": name, "sizeBytes": written, "source": resource.Name,
	})
}

// handleLentResources — GET /api/v1/retrogaming/resources/catalog (tout compte) : les ressources telles que le Catalogue
// les montre — leur nom, leur genre, leurs jeux par console (resources.Lent).
func (server *Server) handleLentResources(w http.ResponseWriter, _ *http.Request) {
	_, catalog, err := server.externalResources()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	list, err := catalog.LentList()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "resources_unavailable", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, list)
}

// resourceGame — un jeu `res<id>:<référence>` que sa ressource porte encore ; faux pour toute autre référence.
func (server *Server) resourceGame(consoleID, reference string) (*resources.Catalog, int64, string, bool) {
	id, inner, ok := resources.ParseReference(reference)
	if !ok {
		return nil, 0, "", false
	}
	_, catalog, err := server.externalResources()
	if err != nil || !catalog.Has(id, consoleID, inner) {
		return nil, 0, "", false
	}

	return catalog, id, inner, true
}
