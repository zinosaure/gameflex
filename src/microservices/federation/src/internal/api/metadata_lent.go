package api

// LES FOURNISSEURS PRÊTÉS (chantier 61, B ; le porteur, 2026-11-18 : « Il manque le partage […] C'est ça le plus
// important ») — les deux routes de GameFlex (federation_metadata.go du kernel), sur les jeux et les comptes du service :
//
//   - GET /api/v1/federation/metadata/{consoleId}/search?gameId=&q=&kind=  ses fiches d'abord (ses jeux du même
//     `gameId`, puis du titre), et, si le jeton a la portée `metadata-providers`, ses comptes IGDB et SteamGridDB pour un
//     titre qu'il n'a pas (metadata.SearchLent) ;
//   - GET /api/v1/federation/metadata/{consoleId}/providers/{service}/{externalId}  le jeu d'un de ses fournisseurs,
//     relu par son compte — seulement avec cette portée. Ses clés ne sortent jamais.

import (
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/federation/internal/store"
	"gameflex/metadata"
)

func lendsProviders(r *http.Request) bool {
	return slices.Contains(tokenOf(r).Scopes, store.ScopeMetadataProviders)
}

// lentGames — les jeux d'une console du service, chacun avec le fichier et les racines de sa fiche (sheetPlace).
func (s *Server) lentGames(consoleID string) []metadata.LentGame {
	games := []metadata.LentGame{}
	for _, item := range s.Catalog.Listing([]string{store.ScopeCatalog}).Roms[consoleID] {
		gamePath, roots, ok := s.sheetPlace(consoleID, item.ID)
		if ok {
			games = append(games, metadata.LentGame{Reference: item.ID, Title: item.Name, Path: gamePath, Roots: roots})
		}
	}

	return games
}

func limited(value string, size int) string {
	if runes := []rune(value); len(runes) > size {
		return string(runes[:size])
	}

	return value
}

func (s *Server) metadataSearch(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeError(w, http.StatusBadRequest, "console_unknown", "unknown console")

		return
	}
	query := r.URL.Query()
	kind := query.Get("kind")
	if kind != "" && !metadata.IsImageKind(kind) {
		writeError(w, http.StatusBadRequest, "bad_request", "unknown image kind")

		return
	}
	writeJSON(w, http.StatusOK, metadata.SearchLent(r.Context(), metadata.LentSearch{
		ConsoleID: consoleID, GameID: limited(query.Get("gameId"), 64), Query: limited(query.Get("q"), 200), Kind: kind,
		Games: s.lentGames(consoleID), Accounts: s.metadataAccounts(), LendProviders: lendsProviders(r),
	}))
}

func (s *Server) providerGame(w http.ResponseWriter, r *http.Request) {
	if !lendsProviders(r) {
		writeError(w, http.StatusForbidden, "scope_missing", "this token does not share the metadata providers")

		return
	}
	consoleID := chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeError(w, http.StatusBadRequest, "console_unknown", "unknown console")

		return
	}
	sheet, err := metadata.LendProviderGame(r.Context(), s.metadataAccounts(), consoleID, chi.URLParam(r, "service"), chi.URLParam(r, "externalId"))
	if err != nil {
		refused := metadata.Refusal(err)
		writeError(w, refused.Status, refused.Code, refused.Message)

		return
	}
	writeJSON(w, http.StatusOK, sheet)
}
