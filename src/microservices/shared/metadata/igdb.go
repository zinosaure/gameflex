package metadata

// IGDB (chantier 49, P4) — jaquettes, artworks (les fonds, jusqu'à
// 1920×1080) et captures, cherchés par titre sur la plateforme de la console ;
// dans la même requête, la date de sortie, les genres, les éditeurs et les
// développeurs de la fiche. Compte : une application Twitch « Confidential » (client_id,
// client_secret) ; un jeton d'environ 60 jours, gardé en mémoire. Débit : 4
// requêtes par seconde au plus (au-delà, 429).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	igdbTokenURL = "https://id.twitch.tv/oauth2/token"
	igdbAPI      = "https://api.igdb.com/v4"
	igdbImages   = "https://images.igdb.com/igdb/image/upload"
)

// igdbPlatforms — les plateformes IGDB de chaque console du registre.
var igdbPlatforms = map[string]int{
	"ps1": 7, "ps2": 8, "ps3": 9, "ps4": 48, "ps5": 167, "psp": 38,
	"switch": 130, "switch2": 508, "wiiu": 41, "wii": 5, "gc": 21, "n64": 4, "snes": 19, "nes": 18,
	"gbc": 22, "gba": 24, "nds": 20, "vb": 87,
	"sms": 64, "megadrive": 29, "saturn": 32, "dreamcast": 23,
	"xbox": 11, "xbox360": 12, "xboxone": 49,
	"atari2600": 59, "atari5200": 66, "atari7800": 60, "lynx": 61, "jaguar": 62,
	"pce": 86, "pcfx": 274, "neogeo": 80, "ngp": 119, "wswan": 57, "coleco": 68, "vectrex": 70,
	"c64": 15, "amstrad": 25, "zxspectrum": 26,
}

type igdbService struct{}

func (igdbService) name() string { return "igdb" }

func (igdbService) configured(accounts Accounts) bool {
	return accounts["igdb"]["client_id"] != "" && accounts["igdb"]["client_secret"] != ""
}

// igdbPace — une requête toutes les 260 ms au plus, pour tout le serveur.
var igdbPace = time.NewTicker(260 * time.Millisecond)

var (
	igdbTokenMutex sync.Mutex
	igdbTokens     = map[string]igdbToken{} // client_id → jeton
)

type igdbToken struct {
	value   string
	expires time.Time
}

func igdbAccessToken(ctx context.Context, clientID, secret string) (string, error) {
	igdbTokenMutex.Lock()
	defer igdbTokenMutex.Unlock()
	if token, found := igdbTokens[clientID]; found && timeNow().Before(token.expires) {
		return token.value, nil
	}
	query := url.Values{"client_id": {clientID}, "client_secret": {secret}, "grant_type": {"client_credentials"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, igdbTokenURL+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	response, err := remoteClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token: %s (check the IGDB client id and secret)", response.Status)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body.AccessToken == "" {
		return "", errors.New("token: unreadable answer")
	}
	// Renouvelé une heure avant l'échéance.
	igdbTokens[clientID] = igdbToken{value: body.AccessToken, expires: timeNow().Add(time.Duration(body.ExpiresIn)*time.Second - time.Hour)}

	return body.AccessToken, nil
}

// igdbImage — une image d'IGDB ; sa taille d'origine, que la recherche
// d'images montre (image_search.go).
type igdbImage struct {
	ImageID string `json:"image_id"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
}

type igdbNamed struct {
	Name string `json:"name"`
}

// igdbCompany — une société du jeu, et son rôle : un même studio peut
// développer ET éditer.
type igdbCompany struct {
	Company   *igdbNamed `json:"company"`
	Developer bool       `json:"developer"`
	Publisher bool       `json:"publisher"`
}

type igdbGame struct {
	ID                int64         `json:"id"`
	Name              string        `json:"name"`
	Cover             *igdbImage    `json:"cover"`
	Artworks          []igdbImage   `json:"artworks"`
	Screenshots       []igdbImage   `json:"screenshots"`
	FirstReleaseDate  int64         `json:"first_release_date"` // secondes Unix
	Genres            []igdbNamed   `json:"genres"`
	InvolvedCompanies []igdbCompany `json:"involved_companies"`
	Collections       []igdbNamed   `json:"collections"`
	Franchises        []igdbNamed   `json:"franchises"`
	Platforms         []struct {
		Abbreviation string `json:"abbreviation"`
		Name         string `json:"name"`
	} `json:"platforms"`
}

// igdbFields — ce qu'une recherche demande : les images et le texte de la
// fiche, en une seule requête par jeu.
const igdbFields = "name,cover.image_id,artworks.image_id,screenshots.image_id," +
	"first_release_date,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher," +
	"collections.name,franchises.name"

func (service igdbService) lookup(ctx context.Context, accounts Accounts, consoleID, title string) (*remoteLookup, error) {
	games, err := igdbGames(ctx, accounts, igdbSearch(consoleID, searchQuery(title), igdbFields, 5))
	if err != nil {
		return nil, err
	}
	game := bestIGDBGame(games, title)
	if game == nil {
		return nil, nil
	}

	return igdbFound(game), nil
}

// lookupByID — le jeu que l'administrateur a désigné (`manual-pin`), sans
// recherche : son identifiant IGDB.
func (service igdbService) lookupByID(ctx context.Context, accounts Accounts, consoleID, externalID string) (*remoteLookup, error) {
	id, err := strconv.ParseInt(externalID, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("%w: %q", ErrInvalidExternalID, externalID)
	}
	games, err := igdbGames(ctx, accounts, fmt.Sprintf(`fields %s; where id = %d; limit 1;`, igdbFields, id))
	if err != nil {
		return nil, err
	}
	if len(games) == 0 {
		return nil, nil
	}

	return igdbFound(&games[0]), nil
}

// candidates — ce qu'IGDB trouve pour `query` sur la console, pour que
// l'administrateur désigne le bon jeu.
func (service igdbService) candidates(ctx context.Context, accounts Accounts, consoleID, query string) ([]Candidate, error) {
	games, err := igdbGames(ctx, accounts, igdbSearch(consoleID, query, "name,cover.image_id,first_release_date,platforms.abbreviation,platforms.name", candidateLimit))
	if err != nil {
		return nil, err
	}
	found := []Candidate{}
	for _, game := range games {
		candidate := Candidate{ExternalID: fmt.Sprint(game.ID), Name: game.Name}
		if game.FirstReleaseDate != 0 {
			candidate.Year = time.Unix(game.FirstReleaseDate, 0).UTC().Year()
		}
		for _, platform := range game.Platforms {
			candidate.Platforms = appendNew(candidate.Platforms, firstOf(platform.Abbreviation, platform.Name))
		}
		if game.Cover != nil && game.Cover.ImageID != "" {
			candidate.CoverURL = igdbImages + "/t_cover_big/" + game.Cover.ImageID + ".jpg"
		}
		found = append(found, candidate)
	}

	return found, nil
}

// igdbSearch — une recherche par titre sur la plateforme de la console.
func igdbSearch(consoleID, title, fields string, limit int) string {
	where := ""
	if platform, found := igdbPlatforms[consoleID]; found {
		where = fmt.Sprintf("where platforms = (%d); ", platform)
	}

	return fmt.Sprintf(`search "%s"; fields %s; %slimit %d;`, strings.NewReplacer(`"`, ``, `\`, ``).Replace(title), fields, where, limit)
}

// igdbGames — une requête à `/games`, au débit permis.
func igdbGames(ctx context.Context, accounts Accounts, query string) ([]igdbGame, error) {
	account := accounts["igdb"]
	token, err := igdbAccessToken(ctx, account["client_id"], account["client_secret"])
	if err != nil {
		return nil, err
	}

	select {
	case <-igdbPace.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, igdbAPI+"/games", bytes.NewBufferString(query))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Client-ID", account["client_id"])
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := remoteClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		igdbTokenMutex.Lock()
		delete(igdbTokens, account["client_id"])
		igdbTokenMutex.Unlock()
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search: %s", response.Status)
	}
	var games []igdbGame
	if err := json.NewDecoder(response.Body).Decode(&games); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return games, nil
}

// igdbFound — ce qu'une fiche IGDB donne : l'adresse de chaque image et le
// texte.
func igdbFound(game *igdbGame) *remoteLookup {
	found := &remoteLookup{ExternalID: fmt.Sprint(game.ID), Name: game.Name, URLs: map[string]string{}}
	if game.Cover != nil && game.Cover.ImageID != "" {
		found.URLs[KindCover] = igdbImages + "/t_cover_big_2x/" + game.Cover.ImageID + ".jpg"
	}
	if len(game.Artworks) > 0 {
		found.URLs[KindBackground] = igdbImages + "/t_1080p/" + game.Artworks[0].ImageID + ".jpg"
	}
	if len(game.Screenshots) > 0 {
		found.URLs[KindScreenshot] = igdbImages + "/t_1080p/" + game.Screenshots[0].ImageID + ".jpg"
	}
	found.Text = igdbText(game)
	for _, collection := range game.Collections {
		found.Collections = appendNew(found.Collections, strings.TrimSpace(collection.Name))
	}
	for _, franchise := range game.Franchises {
		found.Franchises = appendNew(found.Franchises, strings.TrimSpace(franchise.Name))
	}

	return found
}

// igdbText — le texte de la fiche : la première date de sortie, en UTC (IGDB
// la donne à minuit UTC du jour annoncé), les genres et les sociétés selon
// leur rôle, sans doublon. La série se calcule à la lecture, des
// collections et franchises gardées (remoteState.text).
func igdbText(game *igdbGame) gameText {
	var text gameText
	if game.FirstReleaseDate != 0 {
		text.releaseDate = time.Unix(game.FirstReleaseDate, 0).UTC().Format("2006-01-02")
	}
	for _, genre := range game.Genres {
		text.genres = appendNew(text.genres, genre.Name)
	}
	for _, involved := range game.InvolvedCompanies {
		if involved.Company == nil {
			continue
		}
		if involved.Developer {
			text.developers = appendNew(text.developers, involved.Company.Name)
		}
		if involved.Publisher {
			text.publishers = appendNew(text.publishers, involved.Company.Name)
		}
	}

	return text
}

// bestIGDBGame — le résultat retenu par bestTitle (titles.go), nil si aucun
// n'est le jeu cherché : IGDB classe par pertinence, et son premier
// résultat peut être un autre épisode ou un DLC.
func bestIGDBGame(games []igdbGame, title string) *igdbGame {
	names := make([]string, len(games))
	for index, game := range games {
		names[index] = game.Name
	}
	if index := bestTitle(names, title); index >= 0 {
		return &games[index]
	}

	return nil
}
