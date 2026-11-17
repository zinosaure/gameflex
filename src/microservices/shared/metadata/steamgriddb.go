package metadata

// SteamGridDB (chantier 49, P4) — des images seulement, envoyées par ses
// utilisateurs : jaquettes 600×900 (`grids`), bandeaux larges (`heroes`, les
// fonds), logos transparents. Cherché par titre (rien par identifiant
// PlayStation ni Xbox). Compte : une clé personnelle. Usage personnel et non
// commercial ; pas de redistribution sans permission écrite (les images d'un
// ami lui arrivent en lien vers SteamGridDB, décision 3 du porteur).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var steamGridDBAPI = "https://www.steamgriddb.com/api/v2"

type steamGridDBService struct{}

func (steamGridDBService) name() string { return "steamgriddb" }

func (steamGridDBService) configured(accounts Accounts) bool {
	return accounts["steamgriddb"]["api_key"] != ""
}

// steamGridDBPace — quatre requêtes par seconde au plus : le service ne
// publie pas de limite, on reste raisonnable.
var steamGridDBPace = time.NewTicker(250 * time.Millisecond)

func (service steamGridDBService) get(ctx context.Context, key, address string, into any) error {
	select {
	case <-steamGridDBPace.C:
	case <-ctx.Done():
		return ctx.Err()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := remoteClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s (check the SteamGridDB key)", response.Status)
	}

	return json.NewDecoder(response.Body).Decode(into)
}

// steamGridDBGame — un jeu de la recherche de SteamGridDB.
type steamGridDBGame struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ReleaseDate int64  `json:"release_date"` // secondes Unix, parfois absente
}

func (service steamGridDBService) search(ctx context.Context, key, query string) ([]steamGridDBGame, error) {
	var search struct {
		Data []steamGridDBGame `json:"data"`
	}
	if err := service.get(ctx, key, steamGridDBAPI+"/search/autocomplete/"+url.PathEscape(query), &search); err != nil {
		return nil, err
	}

	return search.Data, nil
}

func (service steamGridDBService) lookup(ctx context.Context, accounts Accounts, consoleID, title string) (*remoteLookup, error) {
	key := accounts["steamgriddb"]["api_key"]
	games, err := service.search(ctx, key, searchQuery(title))
	if err != nil {
		return nil, err
	}
	// Le premier résultat n'est pas toujours le jeu (« EA Sports FC 24 »
	// pour « EA Sports FC 27 ») : bestTitle, et rien plutôt qu'un autre.
	names := make([]string, len(games))
	for index, candidate := range games {
		names[index] = candidate.Name
	}
	index := bestTitle(names, title)
	if index < 0 {
		return nil, nil
	}

	return service.found(ctx, key, games[index])
}

// lookupByID — le jeu que l'administrateur a désigné (`manual-pin`).
func (service steamGridDBService) lookupByID(ctx context.Context, accounts Accounts, consoleID, externalID string) (*remoteLookup, error) {
	id, err := strconv.ParseInt(externalID, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("%w: %q", ErrInvalidExternalID, externalID)
	}
	key := accounts["steamgriddb"]["api_key"]
	var answer struct {
		Data *steamGridDBGame `json:"data"`
	}
	if err := service.get(ctx, key, fmt.Sprintf("%s/games/id/%d", steamGridDBAPI, id), &answer); err != nil {
		return nil, err
	}
	if answer.Data == nil || answer.Data.ID == 0 {
		return nil, nil
	}

	return service.found(ctx, key, *answer.Data)
}

// candidates — ce que SteamGridDB trouve pour `query`. Sans jaquette : il
// en faudrait une requête de plus par candidat.
func (service steamGridDBService) candidates(ctx context.Context, accounts Accounts, consoleID, query string) ([]Candidate, error) {
	games, err := service.search(ctx, accounts["steamgriddb"]["api_key"], query)
	if err != nil {
		return nil, err
	}
	found := []Candidate{}
	for _, game := range games {
		candidate := Candidate{ExternalID: fmt.Sprint(game.ID), Name: game.Name}
		if game.ReleaseDate > 0 {
			candidate.Year = time.Unix(game.ReleaseDate, 0).UTC().Year()
		}
		found = append(found, candidate)
		if len(found) == candidateLimit {
			break
		}
	}

	return found, nil
}

// found — l'adresse de la première jaquette, du premier bandeau et du
// premier logo du jeu.
func (service steamGridDBService) found(ctx context.Context, key string, game steamGridDBGame) (*remoteLookup, error) {
	found := &remoteLookup{ExternalID: fmt.Sprint(game.ID), Name: game.Name, URLs: map[string]string{}}
	for kind, address := range map[string]string{
		KindCover:      fmt.Sprintf("%s/grids/game/%d?dimensions=600x900&types=static", steamGridDBAPI, game.ID),
		KindBackground: fmt.Sprintf("%s/heroes/game/%d?types=static", steamGridDBAPI, game.ID),
		KindLogo:       fmt.Sprintf("%s/logos/game/%d?types=static", steamGridDBAPI, game.ID),
	} {
		var images struct {
			Data []struct {
				URL string `json:"url"`
			} `json:"data"`
		}
		if err := service.get(ctx, key, address, &images); err != nil {
			return nil, err
		}
		if len(images.Data) > 0 {
			found.URLs[kind] = images.Data[0].URL
		}
	}

	return found, nil
}
