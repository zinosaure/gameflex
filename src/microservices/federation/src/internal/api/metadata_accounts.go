package api

// LES COMPTES DES FOURNISSEURS DE FICHES (chantier 61, P3, question 2 A) : l'administration du service les règle, comme
// Réglages › Fournisseur de métadonnées le fait pour GameFlex (mêmes services, mêmes champs, même réponse : le service
// d'identité, metadata_services.go) — sauf qu'ils vivent dans la base du service, qui ne dépend de personne.
//
//   - GET    /api/v1/admin/metadata            chaque service : configuré ou non, les champs remplis, ceux requis ;
//   - PUT    /api/v1/admin/metadata/{service}  {champ: valeur} : seuls les champs remplis changent ; la liste relue ;
//   - DELETE /api/v1/admin/metadata/{service}  le compte oublié.

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"gameflex/metadata"
)

var metadataServiceFields = map[string]struct {
	fields   []string
	required []string
}{
	"igdb":        {fields: []string{"client_id", "client_secret"}, required: []string{"client_id", "client_secret"}},
	"steamgriddb": {fields: []string{"api_key"}, required: []string{"api_key"}},
}

type metadataServiceView struct {
	Service    string   `json:"service"`
	Configured bool     `json:"configured"`
	Fields     []string `json:"fields"`
	Required   []string `json:"required"`
}

// metadataAccounts — les comptes, pour les fiches ; aucun si la base ne se lit pas (les fiches gardent ce que porte le
// jeu et libretro).
func (s *Server) metadataAccounts() metadata.Accounts {
	accounts, err := s.Store.MetadataAccounts()
	if err != nil {
		return nil
	}

	return accounts
}

func (s *Server) metadataServices() ([]metadataServiceView, error) {
	stored, err := s.Store.MetadataAccounts()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(metadataServiceFields))
	for name := range metadataServiceFields {
		names = append(names, name)
	}
	sort.Strings(names)
	views := []metadataServiceView{}
	for _, name := range names {
		spec := metadataServiceFields[name]
		view := metadataServiceView{Service: name, Fields: []string{}, Required: spec.required, Configured: true}
		for _, field := range spec.fields {
			if stored[name][field] != "" {
				view.Fields = append(view.Fields, field)
			}
		}
		for _, field := range spec.required {
			if stored[name][field] == "" {
				view.Configured = false
			}
		}
		views = append(views, view)
	}

	return views, nil
}

func (s *Server) listMetadataServices(w http.ResponseWriter, _ *http.Request) {
	views, err := s.metadataServices()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_failed", err.Error())

		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) saveMetadataService(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	spec, known := metadataServiceFields[service]
	if !known {
		writeError(w, http.StatusNotFound, "unknown_service", "unknown metadata service")

		return
	}
	var body map[string]string
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")

		return
	}
	fields := map[string]string{}
	for _, field := range spec.fields {
		if value := strings.TrimSpace(body[field]); value != "" {
			fields[field] = value
		}
	}
	if len(fields) == 0 {
		writeError(w, http.StatusBadRequest, "nothing_to_save", "no field given")

		return
	}
	if err := s.Store.SetMetadataFields(service, fields); err != nil {
		writeError(w, http.StatusInternalServerError, "store_failed", err.Error())

		return
	}
	s.listMetadataServices(w, r)
}

func (s *Server) removeMetadataService(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	if _, known := metadataServiceFields[service]; !known {
		writeError(w, http.StatusNotFound, "unknown_service", "unknown metadata service")

		return
	}
	if err := s.Store.RemoveMetadataService(service); err != nil {
		writeError(w, http.StatusInternalServerError, "store_failed", err.Error())

		return
	}
	s.listMetadataServices(w, r)
}
