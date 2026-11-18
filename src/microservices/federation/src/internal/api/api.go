// Package api — CE QUE LE SERVICE SERT (chantier 61).
//
// Deux surfaces :
//
//   - LA FÉDÉRATION, `/api/v1/federation/…`, aux mêmes chemins et au même format qu'un kernel de GameFlex
//     (federation_api.go du kernel) : un appareil ajoute ce service comme n'importe quel serveur fédéré, sans rien
//     changer de son côté. Sous un jeton de catalogue émis ici. Lecture seule : l'identité, le listing et sa date, le
//     fichier d'un jeu. Ni consoles, ni sessions, ni Remote Play : `/sessions` et `/remoteplay/devices` répondent vide.
//   - L'ADMINISTRATION, `/api/v1/admin/…` : on s'y connecte avec le compte que l'environnement du conteneur donne
//     (FEDERATION_ADMIN_USERNAME, FEDERATION_ADMIN_PASSWORD), puis une session (en-tête Bearer) ouvre les ressources,
//     leur parcours, les jetons. Le porteur, 2026-11-18 : « On se login avec un compte passé en varenv dans le
//     docker ».
package api

import (
	"archive/zip"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"gameflex/federation/internal/catalog"
	"gameflex/federation/internal/store"
	"gameflex/federation/internal/web"
	"gameflex/logfile"
	"gameflex/storage"

	"github.com/go-chi/chi/v5"
)

// Server — le service.
type Server struct {
	Store   *store.Store
	Catalog *catalog.Catalog
	// AdminUsername, AdminPassword — le compte de l'administrateur, comparés en temps constant.
	AdminUsername string
	AdminPassword string
	// Name — le nom du serveur, tel que les appareils le voient.
	Name func() string
	// DataDir — le volume de données : les journaux y vivent (logs.go).
	DataDir string
	// DisksDir — où les ressources internes sont montées (/mnt) ; vide : aucune (disks.go).
	DisksDir string

	throttle redeemThrottle
	pairing  *pairing
	once     sync.Once
}

// Router — toutes les routes.
func (s *Server) Router() http.Handler {
	s.once.Do(func() { s.pairing = newPairing(s.Name) })
	router := chi.NewRouter()
	// Le journal d'accès de GameFlex (les secrets des adresses masqués) ; il s'écrit ou non selon les réglages.
	router.Use(logfile.HTTPLogger)
	router.Get("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Les briques d'edge que l'interface reprend demandent les capacités du serveur au chargement : le service n'en a
	// aucune (ni consoles, ni disques, ni application).
	router.Get("/api/v1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"capabilities":{}}`))
	})
	router.Route("/api/v1/federation", func(router chi.Router) {
		router.Use(cors)
		router.Options("/*", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
		// L'échange d'une invitation ou d'un code d'appairage : sans jeton, c'est lui qui en donne un.
		router.Post("/redeem", s.redeem)
		router.Group(func(router chi.Router) {
			router.Use(s.catalogToken)
			router.Get("/identity", s.identity)
			router.Get("/listing", s.listing)
			router.Get("/listing/updated", s.listingUpdated)
			router.Get("/roms/{consoleId}/{ref}", s.rom)
			router.Head("/roms/{consoleId}/{ref}", s.rom)
			// Ce que les sources apportent au-delà des jeux (disks.go).
			router.Get("/bios/{consoleId}/{name}", s.bios)
			router.Head("/bios/{consoleId}/{name}", s.bios)
			router.Get("/cores/{consoleId}/{name}", s.core)
			router.Head("/cores/{consoleId}/{name}", s.core)
			router.Get("/bundles/{consoleId}/{file}", s.bundle)
			router.Head("/bundles/{consoleId}/{file}", s.bundle)
			router.Get("/params/{consoleId}/{emulator}", s.params)
			router.Get("/params/{consoleId}/{emulator}/presets/{preset}", s.params)
			router.Get("/patches/{consoleId}/{name}", s.patch)
			router.Head("/patches/{consoleId}/{name}", s.patch)
			// LES FOURNISSEURS PRÊTÉS (chantier 61, B ; ceux de GameFlex, federation_metadata.go du kernel) : sa
			// recherche dans ses fiches, et avec la portée `metadata-providers` dans ses comptes ; le jeu d'un de ses
			// fournisseurs, seulement avec elle. Un segment statique l'emporte sur `{ref}` chez chi.
			router.Get("/metadata/{consoleId}/search", s.metadataSearch)
			router.Get("/metadata/{consoleId}/providers/{service}/{externalId}", s.providerGame)
			router.Get("/metadata/{consoleId}/{ref}", s.gameSheet)
			router.Get("/metadata/{consoleId}/{ref}/images/{kind}", s.gameImage)
			router.Get("/sessions", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, []any{}) })
			router.Get("/remoteplay/devices", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, []any{}) })
		})
	})
	router.Post("/api/v1/admin/login", s.login)
	router.Route("/api/v1/admin", func(router chi.Router) {
		router.Use(s.admin)
		router.Post("/logout", s.logout)
		router.Get("/me", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"username": s.AdminUsername})
		})
		router.Get("/resources", s.listResources)
		router.Post("/resources", s.addResource)
		router.Put("/resources/{id}", s.updateResource)
		router.Delete("/resources/{id}", s.removeResource)
		router.Post("/resources/{id}/rescan", s.rescanResource)
		router.Get("/tokens", s.listTokens)
		router.Post("/tokens", s.issueToken)
		router.Patch("/tokens/{id}", s.updateToken)
		router.Delete("/tokens/{id}", s.revokeToken)
		router.Get("/settings", s.getSettings)
		router.Get("/metadata", s.listMetadataServices)
		router.Put("/metadata/{service}", s.saveMetadataService)
		router.Delete("/metadata/{service}", s.removeMetadataService)
		router.Put("/settings", s.putSettings)
		router.Get("/invitations", s.listInvitations)
		router.Post("/invitations", s.createInvitation)
		router.Delete("/invitations/{id}", s.revokeInvitation)
		router.Get("/pairing", s.pairingState)
		router.Post("/pairing/discovery", s.pairingDiscovery)
		router.Post("/pairing/code", s.pairingCode)
		router.Delete("/pairing/code/{id}", s.pairingCancel)
		router.Get("/events", s.events)
		router.Get("/disks", s.listDisks)
		router.Post("/disks/{name}/rescan", s.rescanDisk)
		router.Put("/disks/{name}/pin", s.pinDisk)
		router.Put("/resources/{id}/pin", s.pinResource)
		router.Get("/logs", s.listLogs)
		router.Get("/logs/settings", s.getLogSettings)
		router.Put("/logs/settings", s.putLogSettings)
		router.Get("/logs/tail", s.tailLog)
		router.Get("/logs/download", s.downloadLog)
		router.Delete("/logs/file", s.removeLog)
	})
	// Tout le reste est l'interface (internal/web) ; une adresse /api/ inconnue reste une erreur d'API.
	router.HandleFunc("/api/*", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such route")
	})
	router.Handle("/*", web.Handler())

	return router
}

// ── Les réponses, au format du kernel ───────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errorCode": code, "errorMessage": message})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Range")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		next.ServeHTTP(w, r)
	})
}

// ── La fédération ───────────────────────────────────────────────────────────

type tokenKey struct{}

func bearer(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if value, ok := strings.CutPrefix(header, "Bearer "); ok {
		return strings.TrimSpace(value)
	}

	return ""
}

func (s *Server) catalogToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := bearer(r)
		if value == "" {
			writeError(w, http.StatusUnauthorized, "token_refused", "missing catalogue token")

			return
		}
		token, ok := s.Store.VerifyToken(value)
		if !ok {
			writeError(w, http.StatusUnauthorized, "token_refused", "invalid catalogue token")

			return
		}
		next.ServeHTTP(w, r.WithContext(contextWith(r, token)))
	})
}

func (s *Server) identity(w http.ResponseWriter, r *http.Request) {
	token := tokenOf(r)
	writeJSON(w, http.StatusOK, map[string]any{"hostname": s.Name(), "identity": token.Label, "scopes": token.Scopes})
}

func (s *Server) listing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Catalog.Listing(tokenOf(r).Scopes))
}

func (s *Server) listingUpdated(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"updatedAt": s.Catalog.UpdatedAt(), "scopes": tokenOf(r).Scopes, "catalogOnly": true})
}

// zipFolder — le dossier d'un élément, zippé à la volée sans compression (des jeux déjà compressés pour la plupart),
// dans `w` ; les en-têtes sont posés par l'appelant.
func (s *Server) zipFolder(w io.Writer, _ *http.Request, source storage.Source, opener storage.Opener, folder string) {
	archive := zip.NewWriter(w)
	defer archive.Close()
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := source.List(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			child := path.Join(dir, e.Name)
			if e.Dir {
				if err := walk(child); err != nil {
					return err
				}

				continue
			}
			name := strings.TrimPrefix(strings.TrimPrefix(child, folder), "/")
			out, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
			if err != nil {
				return err
			}
			in, err := opener.Open(child)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, in)
			in.Close()
			if err != nil {
				return err
			}
		}

		return nil
	}
	if err := walk(folder); err != nil {
		slog.Warn("relais d'un dossier de jeu interrompu", "folder", folder, "error", err)
	}
}

// ── L'administration ────────────────────────────────────────────────────────

func (s *Server) admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := bearer(r)
		if value == "" && (strings.HasSuffix(r.URL.Path, "/events") || strings.HasSuffix(r.URL.Path, "/logs/download")) {
			// Un EventSource et un lien de téléchargement n'envoient pas d'en-tête : la session voyage dans l'adresse,
			// sous le nom `token`, que le journal d'accès masque (logfile.Redact).
			value = r.URL.Query().Get("token")
		}
		if value == "" || !s.Store.ValidSession(value) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")

			return
		}
		next.ServeHTTP(w, r)
	})
}

// login — le compte de l'environnement contre une session.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	userOK := subtle.ConstantTimeCompare([]byte(body.Username), []byte(s.AdminUsername)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.AdminPassword)) == 1
	if !userOK || !passOK {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")

		return
	}
	value, expires, err := s.Store.OpenSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": value, "expiresAt": expires, "username": s.AdminUsername})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.Store.CloseSession(bearer(r))
	writeJSON(w, http.StatusOK, map[string]any{"signedOut": true})
}

type resourceView struct {
	store.Resource
	HasPassword bool          `json:"hasPassword"`
	Pinned      bool          `json:"pinned"`
	State       catalog.State `json:"state"`
}

func (s *Server) listResources(w http.ResponseWriter, _ *http.Request) {
	resources, err := s.Store.Resources()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	states := s.Catalog.States()
	views := make([]resourceView, 0, len(resources))
	for _, r := range resources {
		views = append(views, resourceView{Resource: r, HasPassword: r.Password != "", Pinned: s.Catalog.Pinned(r.ID), State: states[r.ID]})
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

func (b resourceBody) resource() store.Resource {
	return store.Resource{Name: b.Name, Kind: b.Kind, Address: b.Address, Folder: b.Folder,
		Username: b.Username, Password: b.Password}
}

func (s *Server) resourceByID(id int64) (store.Resource, bool) {
	resources, err := s.Store.Resources()
	if err != nil {
		return store.Resource{}, false
	}
	for _, r := range resources {
		if r.ID == id {
			return r, true
		}
	}

	return store.Resource{}, false
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	return id, err == nil
}

func (s *Server) addResource(w http.ResponseWriter, r *http.Request) {
	var body resourceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	id, err := s.Store.AddResource(body.resource())
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	added, _ := s.resourceByID(id)
	s.Catalog.Rescan(added)
	writeJSON(w, http.StatusCreated, resourceView{Resource: added, HasPassword: added.Password != ""})
}

func (s *Server) updateResource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	var body resourceBody
	if !ok || json.NewDecoder(r.Body).Decode(&body) != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid resource")

		return
	}
	updated := body.resource()
	updated.ID = id
	if err := s.Store.UpdateResource(updated); errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "not_found", "no such resource")

		return
	} else if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	// Une adresse ou des identifiants changés : la connexion gardée ne vaut plus, et le partage se relit.
	s.Catalog.Forget(id, false)
	current, _ := s.resourceByID(id)
	s.Catalog.Rescan(current)
	writeJSON(w, http.StatusOK, resourceView{Resource: current, HasPassword: current.Password != ""})
}

func (s *Server) removeResource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")

		return
	}
	if err := s.Store.RemoveResource(id); errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "not_found", "no such resource")

		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	s.Catalog.Forget(id, true)
	writeJSON(w, http.StatusOK, map[string]any{"removed": id})
}

func (s *Server) rescanResource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	resource, found := s.resourceByID(id)
	if !ok || !found {
		writeError(w, http.StatusNotFound, "not_found", "no such resource")

		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"started": s.Catalog.Rescan(resource)})
}

func (s *Server) listTokens(w http.ResponseWriter, _ *http.Request) {
	tokens, err := s.Store.Tokens()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (s *Server) issueToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label  string   `json:"label"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	token, value, err := s.Store.IssueToken(body.Label, body.Scopes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	// La valeur n'est montrée qu'ici : la base n'en garde que l'empreinte.
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "value": value})
}

// updateToken — PATCH /api/v1/admin/tokens/{id} {scopes} : ce qu'un accès partage, changé à l'instant.
func (s *Server) updateToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")

		return
	}
	var body struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())

		return
	}
	token, err := s.Store.UpdateTokenScopes(id, body.Scopes)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "not_found", "no such token")

		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, token)
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")

		return
	}
	if err := s.Store.RevokeToken(id); errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "not_found", "no such token")

		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": id})
}

// SettingServerName — le nom du serveur, tel que les appareils le voient.
const SettingServerName = "server_name"

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"serverName": s.Name()})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServerName string `json:"serverName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ServerName) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "a server name is required")

		return
	}
	if err := s.Store.SetSetting(SettingServerName, strings.TrimSpace(body.ServerName)); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"serverName": s.Name()})
}
