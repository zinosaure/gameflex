package metadata

// CHOISIR UNE IMAGE (chantier 53, P9 ; le porteur, 2026-11-14, remarque d : « au
// lieu de mettre upload file pour l'image, le mieux ce serait de mettre une
// search bar qui va taper sur les API metadata pour récupérer plus d'images et
// ensuite les proposer en format listing. Comme ça on choisit celle qu'on veut.
// Pareil quand on souhaite remplacer une image. ») :
//
//   - SearchGameImages, SearchBandImages — pour un titre, ce que chaque
//     fournisseur propose d'une sorte d'image : IGDB (la jaquette, les
//     artworks ou les captures des jeux trouvés sur la plateforme de la
//     console), SteamGridDB (les jaquettes, bandeaux, logos ou icônes des
//     premiers jeux trouvés), libretro-thumbnails (sans compte : les noms de
//     son index qui contiennent le titre). Ce sont les comptes et les
//     adresses de la chantier 49 (igdb.go, steamgriddb.go, libretro.go) ; un
//     fournisseur sans compte est sauté, et la réponse le dit ;
//   - FetchChosenImage — l'image choisie, récupérée SEULEMENT chez l'hôte
//     d'images de sa source (imageOrigins) : ni une autre adresse, ni une
//     redirection vers un autre hôte, 16 Mo au plus. Le serveur ne va jamais
//     chercher une adresse que la page aurait inventée.
//
// L'image choisie s'écrit ensuite comme une image déposée autrefois : la
// source `manual` d'un jeu (SetManualImage), un emplacement du fond d'une
// console (SetConsoleBackground). L'envoi d'un fichier est parti : seule la
// recherche pose une image.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"gameflex/consoles"
)

// ImageResult — une image proposée : sa source (igdb, steamgriddb, libretro),
// son adresse, sa vignette, le jeu d'où elle vient, sa taille quand le
// fournisseur la donne.
//
// UNE IMAGE D'UN SERVEUR FÉDÉRÉ (chantier 57) : la source `server`, son
// identifiant et son nom (`server`, `serverName`), la référence de son jeu, et
// sa vignette par le relais du kernel qui affiche (`thumbPath`, une adresse
// chez lui, que la page charge avec son jeton) ; pas d'adresse : elle se choisit
// par `{source: "server", server, reference}`. Une image d'un de SES
// fournisseurs (s'il les partage, `#719`) garde la source de ce fournisseur et
// son adresse, avec `server` et `serverName` en plus.
type ImageResult struct {
	Source     string `json:"source"`
	URL        string `json:"url,omitempty"`
	ThumbURL   string `json:"thumbUrl,omitempty"`
	Game       string `json:"game"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Server     int64  `json:"server,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	Reference  string `json:"reference,omitempty"`
	ThumbPath  string `json:"thumbPath,omitempty"`
}

// ImageSourceState — ce qu'un fournisseur a répondu : `ok`, `noAccount` (pas
// de compte saisi), `none` (rien de cette sorte, ou pas cette console chez
// lui), `error` (avec sa raison). Un serveur fédéré (`server`, avec son
// identifiant et son nom) : `ok`, `none`, ou `error` avec `refused` (il refuse
// notre jeton), `notLending` (une version qui ne prête pas encore ses fiches)
// ou `unreachable` ; `reading` : il lit encore les identifiants de ses jeux, et
// la recherche suivante trouvera le même jeu chez lui.
type ImageSourceState struct {
	Source     string `json:"source"`
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
	Server     int64  `json:"server,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	Reading    bool   `json:"reading,omitempty"`
}

// ImageSearch — la réponse d'une recherche : le titre cherché, la sorte, les
// images (IGDB, puis SteamGridDB, puis libretro, puis les serveurs fédérés —
// les fournisseurs d'abord, le porteur, 2026-11-15), et l'état de chaque
// fournisseur et de chaque serveur.
type ImageSearch struct {
	Query   string             `json:"query"`
	Kind    string             `json:"kind"`
	Items   []ImageResult      `json:"items"`
	Sources []ImageSourceState `json:"sources"`
}

// imagePlan — ce que chaque fournisseur donne d'une sorte : le champ d'un jeu
// IGDB, la liste de SteamGridDB, le dossier de libretro-thumbnails ; vide
// quand il n'a rien de cette sorte.
type imagePlan struct {
	igdb, steamGridDB, libretro string
}

// gameImagePlans — les sortes d'image d'un jeu (retroOrder).
var gameImagePlans = map[string]imagePlan{
	KindCover:      {igdb: "cover", steamGridDB: "grids", libretro: "Named_Boxarts"},
	KindBackground: {igdb: "artworks", steamGridDB: "heroes"},
	KindScreenshot: {igdb: "screenshots", libretro: "Named_Snaps"},
	KindTitle:      {libretro: "Named_Titles"},
	KindLogo:       {steamGridDB: "logos", libretro: "Named_Logos"},
	KindIcon:       {steamGridDB: "icons"},
}

// bandPlan — les images de la bande d'une console (son fond, ConsoleVisual du
// launcher) : les artworks d'IGDB, les bandeaux de SteamGridDB, et les
// jaquettes de libretro, celles du remplissage par défaut — la seule source
// sans compte.
var bandPlan = imagePlan{igdb: "artworks", steamGridDB: "heroes", libretro: "Named_Boxarts"}

// La sorte que rend une recherche de la bande.
const kindBand = "band"

const (
	// imageSearchPerSource — ce qu'un fournisseur propose au plus : la page en
	// feuillette dix à la fois.
	imageSearchPerSource = 40
	// imageSearchGames — les jeux de SteamGridDB dont on lit les images : une
	// requête chacun, au débit permis.
	imageSearchGames = 3
	// imageSearchTimeout — une recherche ne fait jamais attendre la page plus
	// longtemps : un fournisseur lent rend ce qu'il a, en erreur.
	imageSearchTimeout = 20 * time.Second
)

// ErrEmptyQuery — une recherche sans titre.
var ErrEmptyQuery = errors.New("a title to search is expected")

// SearchGameImages — les images de la sorte `kind` que les fournisseurs
// proposent pour `query` sur la console, puis celles des jeux de mes serveurs
// fédérés (`peers`, chantier 57) : par `gameID` (le même jeu), puis par titre.
// `cacheRoot` : où vit l'index de libretro (`cache/metadata/sources`).
func SearchGameImages(ctx context.Context, cacheRoot string, accounts Accounts, consoleID, kind, query, gameID string, peers []Peer) (ImageSearch, error) {
	plan, known := gameImagePlans[kind]
	if !known {
		return ImageSearch{}, ErrUnknownKind
	}

	return searchImages(ctx, cacheRoot, accounts, consoleID, kind, query, plan, gameID, peers)
}

// SearchBandImages — les images que les fournisseurs proposent pour `query`
// sur la console, pour un emplacement de son fond (console_backgrounds.go).
// La console d'un jeu à distance cherche dans celle qu'elle joue.
func SearchBandImages(ctx context.Context, cacheRoot string, accounts Accounts, consoleID, query string) (ImageSearch, error) {
	if !consoles.Exists(consoleID) {
		return ImageSearch{}, ErrUnknownConsole
	}

	return searchImages(ctx, cacheRoot, accounts, backgroundSourceConsole(consoleID), kindBand, query, bandPlan, "", nil)
}

// searchImages — chaque fournisseur et chaque pair en même temps (un pair
// répond en dix secondes au plus, dans les vingt de la recherche) ; une image
// dont l'adresse ou la vignette n'est pas chez l'hôte d'images de sa source
// n'est jamais proposée (elle serait refusée au choix), et celle d'un pair n'a
// que sa référence chez lui.
func searchImages(ctx context.Context, cacheRoot string, accounts Accounts, consoleID, kind, query string, plan imagePlan, gameID string, peers []Peer) (ImageSearch, error) {
	query = strings.Join(strings.Fields(query), " ")
	if query == "" {
		return ImageSearch{}, ErrEmptyQuery
	}
	ctx, cancel := context.WithTimeout(ctx, imageSearchTimeout)
	defer cancel()

	type answer struct {
		items []ImageResult
		state ImageSourceState
	}
	providers := []struct {
		source string
		run    func() ([]ImageResult, string, error)
	}{
		{sourceIGDB, func() ([]ImageResult, string, error) {
			return searchIGDBImages(ctx, accounts, consoleID, query, plan.igdb)
		}},
		{sourceSteamGridDB, func() ([]ImageResult, string, error) {
			return searchSteamGridDBImages(ctx, accounts, query, plan.steamGridDB)
		}},
		{sourceLibretro, func() ([]ImageResult, string, error) {
			return searchLibretroImages(ctx, cacheRoot, consoleID, query, plan.libretro)
		}},
	}
	answers := make([]answer, len(providers)+len(peers))
	var wait sync.WaitGroup
	for index, provider := range providers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			items, state, err := provider.run()
			answers[index] = answer{items: items, state: ImageSourceState{Source: provider.source, State: state}}
			if err != nil {
				answers[index].state.State, answers[index].state.Error = "error", err.Error()
			}
		}()
	}
	for index, peer := range peers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			items, state := peerImages(ctx, peer, consoleID, gameID, query, kind)
			answers[len(providers)+index] = answer{items: items, state: state}
		}()
	}
	wait.Wait()

	found := ImageSearch{Query: query, Kind: kind, Items: []ImageResult{}, Sources: []ImageSourceState{}}
	for _, answer := range answers {
		for _, item := range answer.items {
			lent := item.Source == sourceServer && item.Reference != "" && item.ThumbPath != ""
			if lent || (allowedImage(item.Source, item.URL) && allowedImage(item.Source, item.ThumbURL)) {
				found.Items = append(found.Items, item)
			}
		}
		found.Sources = append(found.Sources, answer.state)
	}

	return found, nil
}

// ── IGDB ────────────────────────────────────────────────────────────────

// igdbImageSizes — la vignette et l'image d'un champ : la jaquette en
// `cover_big` (264 × 374) et sa double (celle que la fiche garde, igdbFound),
// un artwork ou une capture en `screenshot_med` (569 × 320) et en 1080p.
func igdbImageSizes(field string) (thumb, full string) {
	if field == "cover" {
		return "t_cover_big", "t_cover_big_2x"
	}

	return "t_screenshot_med", "t_1080p"
}

func searchIGDBImages(ctx context.Context, accounts Accounts, consoleID, query, field string) ([]ImageResult, string, error) {
	service := igdbService{}
	if field == "" {
		return nil, "none", nil
	}
	if !service.configured(accounts) {
		return nil, "noAccount", nil
	}
	games, err := igdbGames(ctx, accounts, igdbSearch(consoleID, query, fmt.Sprintf("name,%[1]s.image_id,%[1]s.width,%[1]s.height", field), 10))
	if err != nil {
		return nil, "", err
	}
	thumb, full := igdbImageSizes(field)
	found := []ImageResult{}
	for _, game := range games {
		images := game.Screenshots
		switch field {
		case "cover":
			images = nil
			if game.Cover != nil {
				images = []igdbImage{*game.Cover}
			}
		case "artworks":
			images = game.Artworks
		}
		for _, picture := range images {
			if picture.ImageID == "" || len(found) == imageSearchPerSource {
				continue
			}
			found = append(found, ImageResult{Source: sourceIGDB, Game: game.Name, Width: picture.Width, Height: picture.Height,
				URL: igdbImages + "/" + full + "/" + picture.ImageID + ".jpg", ThumbURL: igdbImages + "/" + thumb + "/" + picture.ImageID + ".jpg"})
		}
	}

	return found, "ok", nil
}

// ── SteamGridDB ─────────────────────────────────────────────────────────

func searchSteamGridDBImages(ctx context.Context, accounts Accounts, query, list string) ([]ImageResult, string, error) {
	service := steamGridDBService{}
	if list == "" {
		return nil, "none", nil
	}
	if !service.configured(accounts) {
		return nil, "noAccount", nil
	}
	key := accounts["steamgriddb"]["api_key"]
	games, err := service.search(ctx, key, query)
	if err != nil {
		return nil, "", err
	}
	if len(games) > imageSearchGames {
		games = games[:imageSearchGames]
	}
	filter := "?types=static"
	if list == "grids" {
		filter += "&dimensions=600x900"
	}
	found := []ImageResult{}
	for _, game := range games {
		var images struct {
			Data []struct {
				URL    string `json:"url"`
				Thumb  string `json:"thumb"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
			} `json:"data"`
		}
		if err := service.get(ctx, key, fmt.Sprintf("%s/%s/game/%d%s", steamGridDBAPI, list, game.ID, filter), &images); err != nil {
			return found, "", err
		}
		for _, picture := range images.Data {
			if picture.URL == "" || len(found) == imageSearchPerSource {
				continue
			}
			found = append(found, ImageResult{Source: sourceSteamGridDB, Game: game.Name, URL: picture.URL, ThumbURL: firstOf(picture.Thumb, picture.URL),
				Width: picture.Width, Height: picture.Height})
		}
	}

	return found, "ok", nil
}

// ── libretro-thumbnails ─────────────────────────────────────────────────

// searchLibretroImages — les noms de l'index du dossier, pour chaque dossier
// de la console sur le site, dont le titre sans mentions contient celui
// cherché : le même titre d'abord, puis par ordre alphabétique. Le site n'a
// pas de vignettes : l'image est sa propre vignette (une jaquette y pèse
// quelques centaines de Ko).
func searchLibretroImages(ctx context.Context, cacheRoot, consoleID, query, folder string) ([]ImageResult, string, error) {
	systems := libretroSystems[consoleID]
	wanted := looseTitle(query)
	if folder == "" || len(systems) == 0 {
		return nil, "none", nil
	}
	if wanted == "" {
		return []ImageResult{}, "ok", nil
	}
	found := []ImageResult{}
	var lastErr error
	for _, system := range systems {
		index, err := libretroIndex(ctx, cacheRoot, system, folder)
		if err != nil {
			lastErr = err
			continue
		}
		names := []string{}
		for loose, candidates := range index.loose {
			if strings.Contains(loose, wanted) {
				names = append(names, candidates...)
			}
		}
		sort.Slice(names, func(left, right int) bool {
			leftSame, rightSame := looseTitle(names[left]) == wanted, looseTitle(names[right]) == wanted
			if leftSame != rightSame {
				return leftSame
			}

			return names[left] < names[right]
		})
		for _, name := range names {
			if len(found) == imageSearchPerSource {
				break
			}
			address := libretroURL(system, folder, name+".png")
			found = append(found, ImageResult{Source: sourceLibretro, Game: name, URL: address, ThumbURL: address})
		}
	}
	if len(found) == 0 && lastErr != nil {
		return nil, "", fmt.Errorf("libretro: %w", lastErr)
	}

	return found, "ok", nil
}

// ── L'image choisie ─────────────────────────────────────────────────────

// ChosenImageLimit — la taille d'une image choisie, celle d'une image déposée
// autrefois : une image de jeu tient largement en 16 Mo.
const ChosenImageLimit = 16 << 20

var (
	// ErrImageNotAllowed — une adresse qui n'est pas chez l'hôte d'images de
	// sa source, ou une redirection qui en sort.
	ErrImageNotAllowed = errors.New("this address is not an image of its source")
	// ErrImageTooLarge — plus de 16 Mo.
	ErrImageTooLarge = errors.New("image larger than 16 MiB")
)

// steamGridDBImageOrigins — les hôtes des images de SteamGridDB (le `url` et
// le `thumb` de ses listes) ; remplaçables dans un test.
var steamGridDBImageOrigins = []string{"https://cdn2.steamgriddb.com", "https://cdn.steamgriddb.com"}

// imageOrigins — LA LISTE DES HÔTES PERMIS, par source : le schéma, l'hôte et
// le port des images d'IGDB (igdbImages), de SteamGridDB, de libretro
// (libretroBase). Rien d'autre ne se récupère.
func imageOrigins(source string) []string {
	switch source {
	case sourceIGDB:
		return []string{originOf(igdbImages)}
	case sourceSteamGridDB:
		return steamGridDBImageOrigins
	case sourceLibretro:
		return []string{originOf(libretroBase)}
	}

	return nil
}

// originOf — « https://images.igdb.com », vide pour une adresse illisible.
func originOf(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return ""
	}

	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

// allowedImage — `address` est une image de `source` : son origine est l'une
// des siennes, sans identifiants dans l'adresse.
func allowedImage(source, address string) bool {
	parsed, err := url.Parse(address)
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return false
	}
	origin := originOf(address)
	for _, allowed := range imageOrigins(source) {
		if allowed != "" && origin == allowed {
			return true
		}
	}

	return false
}

// chosenImageClient — une redirection ne quitte jamais l'origine de la
// première adresse (trois au plus).
var chosenImageClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 3 || request.URL.User != nil || originOf(request.URL.String()) != originOf(via[0].URL.String()) {
			return ErrImageNotAllowed
		}

		return nil
	},
}

// FetchChosenImage — le contenu de l'image `address` choisie chez `source`,
// 16 Mo au plus. Ce qu'il est (PNG, JPEG, WebP) se vérifie en l'écrivant
// (SetManualImage, SetConsoleBackground).
func FetchChosenImage(ctx context.Context, source, address string) ([]byte, error) {
	if !allowedImage(source, address) {
		return nil, ErrImageNotAllowed
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, ErrImageNotAllowed
	}
	response, err := chosenImageClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", source, response.Status)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, ChosenImageLimit+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if len(content) > ChosenImageLimit {
		return nil, ErrImageTooLarge
	}

	return content, nil
}
