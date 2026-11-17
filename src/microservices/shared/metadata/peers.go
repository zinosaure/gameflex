package metadata

// UN SERVEUR FÉDÉRÉ, SOURCE DE MES PROPRES JEUX (chantier 57, P3 ; le porteur, 2026-11-15 : « Si j'ai un serveur fédéré,
// alors je peux récupérer les images de celui pour populer mes metadata de mes propres jeux. […] la source n'est pas
// toujours des metadata provider »). Les deux bouts, partagés par les deux kernels :
//
// LE PAIR CHERCHE DANS SES FICHES (SearchLent, sa route `…/federation/metadata/{c}/search`) : ses jeux de la console
// qui portent le même `gameId` (lu dans leur contenu : le même jeu porte le même identifiant sur toutes les machines),
// puis ceux dont le nom contient le titre cherché, dix au plus ; avec une sorte d'image, seulement ceux qui l'ont. Les
// identifiants qu'il ne connaît pas encore se lisent en fond — la réponse le dit (`ready: false`), et ce qui est déjà
// connu répond tout de suite. S'IL PARTAGE SES FOURNISSEURS (l'option du jeton, `#719`, décision 10 : « si je partage à
// moi-même il n'y a pas de risques d'abus de quota »), un titre qu'il n'a pas se cherche aussi chez SES comptes
// (IGDB, SteamGridDB) : il rend les candidats et les images, ses clés ne sortent jamais (LendProviderGame).
//
// LE KERNEL QUI CHERCHE les interroge à côté de ses fournisseurs — les fournisseurs d'abord, les serveurs fédérés
// ensuite (décision 10) : « Choisir une image » (SearchGameImages, image_search.go), « Ce n'est pas le bon jeu ? »
// (PeerCandidates). CE QU'ON Y PREND DEVIENT À MOI : une image choisie s'écrit comme une image d'IGDB (la source
// `manual`, FetchPeerImage puis SetManualImage) ; un jeu désigné se copie dans la source `server`
// (`metadata/<console>/<gameid>/server/`, FetchPeerSource) — son texte, la source d'origine de chaque champ, ses
// images —, et c'est une copie, pas un lien : mon jeu garde sa fiche quand le pair s'éteint. Un jeu TÉLÉCHARGÉ d'un pair
// emporte sa fiche de la même façon (le kernel l'appelle à la fin du téléchargement). Jamais de recherche automatique
// chez les pairs à la lecture.
//
// Ce paquet ne parle pas le protocole : le client d'un pair (PeerClient) est `remote.Server`, que chaque kernel
// construit avec le lien et le jeton de son registre.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// sourceServer — la fiche copiée d'un serveur fédéré, et la source d'une image d'un pair dans une recherche.
const sourceServer = "server"

// peerTimeout — un pair répond à une recherche en dix secondes, ou se dit muet : la recherche d'images n'en attend pas
// plus de vingt en tout (imageSearchTimeout).
const peerTimeout = 10 * time.Second

// peerCopyWidth — la largeur à laquelle une image d'un pair se copie : la plus grande qu'il rende (Widths).
const peerCopyWidth = 1920

var (
	// ErrPeerDenied — le serveur fédéré refuse notre jeton (ou ce que le jeton ouvre ne suffit pas).
	ErrPeerDenied = errors.New("this federated server refuses our token")
	// ErrPeerNotLending — le serveur fédéré ne prête pas cela : une version qui n'a pas encore les fiches, ou un jeu
	// qu'il n'a plus.
	ErrPeerNotLending = errors.New("this federated server does not lend this game's metadata")
)

// PeerClient — ce qu'un serveur fédéré répond à ses routes des fiches (remote.Server), et celles du fond d'une console. Un refus de notre jeton se lit
// fs.ErrPermission ; une route ou un jeu absent, fs.ErrNotExist ; le reste, un pair qui ne répond pas. Les réponses se
// décodent dans la forme donnée (`into`).
type PeerClient interface {
	SearchMetadata(ctx context.Context, consoleID string, query url.Values, into any) error
	GameMetadata(ctx context.Context, consoleID, reference string, into any) error
	GameImage(ctx context.Context, consoleID, reference, kind string, width int, limit int64) ([]byte, error)
	ProviderGame(ctx context.Context, consoleID, provider, externalID string, into any) error
	// ConsoleBackgrounds — les emplacements du fond d'une console chez lui (ShownBackgrounds, `#740`).
	ConsoleBackgrounds(ctx context.Context, consoleID string, into any) error
}

// Peer — un serveur fédéré de mon registre : son identifiant chez moi, son nom, son lien, son client.
type Peer struct {
	ID     int64
	Name   string
	Link   string
	Client PeerClient
}

// PeerImagePath — l'adresse, chez le kernel qui affiche, de l'image `kind` du jeu `reference` du serveur fédéré
// `serverID` : le relais de la P2, la référence préfixée de son serveur (`srv<id>:<ref>`) et `?server=<id>` — la même
// route sur le serveur et sur le téléphone.
func PeerImagePath(consoleID string, serverID int64, reference, kind string, width int) string {
	server := strconv.FormatInt(serverID, 10)

	return "/api/v1/retrogaming/" + url.PathEscape(consoleID) + "/roms/" + url.PathEscape("srv"+server+":"+reference) +
		"/images/" + url.PathEscape(kind) + "?server=" + server + "&w=" + strconv.Itoa(width)
}

// ── Le pair : sa recherche dans ses fiches ─────────────────────────────

// LentGame — un de mes jeux, tel que ma recherche le voit : la référence de mon listing, son titre, son fichier, les
// racines de ses métadonnées.
type LentGame struct {
	Reference string
	Title     string
	Path      string
	Roots     Roots
}

// PeerGame — un jeu que propose un pair : sa référence chez lui, son titre, son `gameId` s'il le connaît, comment il a
// été trouvé (`gameId`, le même jeu ; `title`), sa sortie, et avec une sorte d'image la taille de celle qu'il garde.
type PeerGame struct {
	Reference   string `json:"reference"`
	Title       string `json:"title"`
	GameID      string `json:"gameId,omitempty"`
	Match       string `json:"match"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// PeerAnswer — la réponse de la recherche d'un pair : `ready` faux tant qu'il lit encore les identifiants de ses jeux,
// ses jeux trouvés ; s'il partage ses fournisseurs et qu'aucun de ses jeux ne porte le titre cherché, leurs images
// (avec une sorte) ou leurs candidats (sans).
type PeerAnswer struct {
	Ready      bool          `json:"ready"`
	Items      []PeerGame    `json:"items"`
	Images     []ImageResult `json:"images,omitempty"`
	Candidates []Candidate   `json:"candidates,omitempty"`
}

// LentSearch — la question qu'un ami pose à mes fiches : la console, le `gameId` de son jeu, le titre qu'il cherche,
// une sorte d'image ou aucune ; mes jeux de la console et mes comptes ; et si son jeton partage mes fournisseurs.
type LentSearch struct {
	ConsoleID     string
	GameID        string
	Query         string
	Kind          string
	Games         []LentGame
	Accounts      Accounts
	LendProviders bool
}

// lentInfoTimeout — la sortie d'un jeu trouvé (pour l'année d'un candidat) ne retient pas la réponse : ce qui n'est pas
// su à temps n'y est pas.
const lentInfoTimeout = 6 * time.Second

// SearchLent — ce que mes fiches proposent pour la question d'un ami (voir l'en-tête).
func SearchLent(ctx context.Context, search LentSearch) PeerAnswer {
	type found struct {
		game         LentGame
		gameID, from string
	}
	answer := PeerAnswer{Ready: true, Items: []PeerGame{}}
	matches := []found{}
	seen := map[string]bool{}
	if search.GameID != "" {
		for _, game := range search.Games {
			key, known := KnownGameKey(game.Path, game.Reference)
			if !known {
				answer.Ready = false
				continue
			}
			if key == search.GameID && !seen[game.Reference] {
				seen[game.Reference] = true
				matches = append(matches, found{game: game, gameID: key, from: "gameId"})
			}
		}
	}
	// Le titre cherché : ai-je un jeu qui le porte (trouvé par son gameid ou par ce titre) ?
	wanted, hasTitle := looseTitle(search.Query), false
	if wanted != "" {
		for _, game := range search.Games {
			if !strings.Contains(looseTitle(game.Title), wanted) {
				continue
			}
			hasTitle = true
			if seen[game.Reference] {
				continue
			}
			seen[game.Reference] = true
			key, _ := KnownGameKey(game.Path, game.Reference)
			matches = append(matches, found{game: game, gameID: key, from: "title"})
		}
	}

	// Avec une sorte d'image, seulement les jeux qui l'ont : deux fois la limite regardés au plus.
	kept := []found{}
	for index, match := range matches {
		if len(kept) == candidateLimit || (search.Kind != "" && index == 2*candidateLimit) {
			break
		}
		item := PeerGame{Reference: match.game.Reference, Title: match.game.Title, GameID: match.gameID, Match: match.from}
		if search.Kind != "" {
			width, height, err := ImageSize(ctx, match.game.Roots, search.Accounts, search.ConsoleID, match.game.Path, match.game.Reference, search.Kind)
			if err != nil {
				continue
			}
			item.Width, item.Height = width, height
		}
		kept = append(kept, match)
		answer.Items = append(answer.Items, item)
	}

	// Sans sorte d'image (« Ce n'est pas le bon jeu ? »), la sortie de chacun, pour son année.
	if search.Kind == "" && len(kept) > 0 {
		dated, cancel := context.WithTimeout(ctx, lentInfoTimeout)
		var wait sync.WaitGroup
		for index, match := range kept {
			wait.Add(1)
			go func() {
				defer wait.Done()
				answer.Items[index].ReleaseDate = Info(dated, match.game.Roots, search.Accounts, search.ConsoleID, match.game.Path, match.game.Reference).ReleaseDate
			}()
		}
		wait.Wait()
		cancel()
	}

	// UN TITRE QUE JE N'AI PAS (décision 10) : aucun de mes jeux ne le porte — le même jeu trouvé par son gameid n'y
	// change rien, l'ami cherche un autre titre.
	if search.LendProviders && wanted != "" && !hasTitle {
		lendProviders(ctx, search, &answer)
	}

	return answer
}

// lendProviders — un titre que je n'ai pas, cherché chez mes comptes (IGDB, SteamGridDB) pour l'ami dont le jeton les
// partage : les images d'une sorte, ou les candidats. Libretro n'en est pas : l'ami l'a sans compte.
func lendProviders(ctx context.Context, search LentSearch, answer *PeerAnswer) {
	if search.Kind != "" {
		plan := gameImagePlans[search.Kind]
		plan.libretro = ""
		if found, err := searchImages(ctx, "", search.Accounts, search.ConsoleID, search.Kind, search.Query, plan, "", nil); err == nil {
			answer.Images = found.Items
		}

		return
	}
	for _, service := range []remoteService{igdbService{}, steamGridDBService{}} {
		if !service.configured(search.Accounts) {
			continue
		}
		candidates, err := service.candidates(ctx, search.Accounts, search.ConsoleID, strings.TrimSpace(search.Query))
		if err != nil {
			slog.Warn("lent providers", "source", service.name(), "console", search.ConsoleID, "error", err)
			continue
		}
		for _, candidate := range candidates {
			candidate.Provider = service.name()
			answer.Candidates = append(answer.Candidates, candidate)
		}
	}
}

// ProviderSheet — un jeu d'un de mes fournisseurs, tel que je le prête à un ami dont le jeton partage mes fournisseurs :
// son texte, et l'adresse de chacune de ses images chez l'hôte d'images du fournisseur (aucune clé n'y est).
type ProviderSheet struct {
	Provider    string            `json:"provider"`
	ExternalID  string            `json:"externalId"`
	Name        string            `json:"name"`
	ReleaseDate string            `json:"releaseDate,omitempty"`
	Publishers  []string          `json:"publishers,omitempty"`
	Developers  []string          `json:"developers,omitempty"`
	Genres      []string          `json:"genres,omitempty"`
	Series      string            `json:"series,omitempty"`
	Images      map[string]string `json:"images,omitempty"`
}

// LendProviderGame — le jeu `externalID` de mon fournisseur `provider`, relu par mon compte (`#719`). os.ErrNotExist :
// le fournisseur n'a pas ce jeu.
func LendProviderGame(ctx context.Context, accounts Accounts, consoleID, provider, externalID string) (ProviderSheet, error) {
	service, known := remoteServiceNamed(provider)
	if !known {
		return ProviderSheet{}, ErrUnknownSource
	}
	if !service.configured(accounts) {
		return ProviderSheet{}, ErrServiceNotConfigured
	}
	if !externalIDPattern.MatchString(externalID) {
		return ProviderSheet{}, fmt.Errorf("%w: %q", ErrInvalidExternalID, externalID)
	}
	found, err := service.lookupByID(ctx, accounts, consoleID, externalID)
	switch {
	case err != nil:
		return ProviderSheet{}, fmt.Errorf("%s: %w", provider, err)
	case found == nil:
		return ProviderSheet{}, os.ErrNotExist
	}
	sheet := ProviderSheet{Provider: provider, ExternalID: firstOf(found.ExternalID, externalID), Name: found.Name,
		ReleaseDate: found.Text.releaseDate, Publishers: found.Text.publishers, Developers: found.Text.developers, Genres: found.Text.genres,
		Series: seriesName(found.Name, found.Collections, found.Franchises), Images: map[string]string{}}
	for kind, address := range found.URLs {
		if validImageKind(kind) && allowedImage(provider, address) {
			sheet.Images[kind] = address
		}
	}

	return sheet, nil
}

// ── Le kernel qui cherche : mes pairs comme source ────────────────────

// askPeer — la recherche d'un pair, en dix secondes au plus.
func askPeer(ctx context.Context, peer Peer, consoleID, gameID, query, kind string) (PeerAnswer, error) {
	ctx, cancel := context.WithTimeout(ctx, peerTimeout)
	defer cancel()
	values := url.Values{}
	for name, value := range map[string]string{"gameId": gameID, "q": query, "kind": kind} {
		if value != "" {
			values.Set(name, value)
		}
	}
	var answer PeerAnswer
	if err := peer.Client.SearchMetadata(ctx, consoleID, values, &answer); err != nil {
		return PeerAnswer{}, peerError(peer, err)
	}

	return answer, nil
}

// peerError — l'erreur d'un pair, nommée : il refuse notre jeton (ErrPeerDenied), il ne prête pas cela
// (ErrPeerNotLending), ou il ne répond pas.
func peerError(peer Peer, err error) error {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%s: %w", peer.Name, ErrPeerDenied)
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%s: %w", peer.Name, ErrPeerNotLending)
	}

	return fmt.Errorf("%s: %w", peer.Name, err)
}

// peerReason — la raison d'un pair muet, pour l'état d'une recherche.
func peerReason(err error) string {
	switch {
	case errors.Is(err, ErrPeerDenied):
		return "refused"
	case errors.Is(err, ErrPeerNotLending):
		return "notLending"
	}

	return "unreachable"
}

// peerImages — ce qu'un pair propose de la sorte `kind` pour mon jeu : les images de ses jeux (par son relais), puis
// celles de ses fournisseurs s'il les partage ; et son état.
func peerImages(ctx context.Context, peer Peer, consoleID, gameID, query, kind string) ([]ImageResult, ImageSourceState) {
	state := ImageSourceState{Source: sourceServer, State: "ok", Server: peer.ID, ServerName: peer.Name}
	answer, err := askPeer(ctx, peer, consoleID, gameID, query, kind)
	if err != nil {
		state.State, state.Error = "error", peerReason(err)

		return nil, state
	}
	items := []ImageResult{}
	for _, game := range answer.Items {
		items = append(items, ImageResult{Source: sourceServer, Game: game.Title, Width: game.Width, Height: game.Height, Server: peer.ID,
			ServerName: peer.Name, Reference: game.Reference, ThumbPath: PeerImagePath(consoleID, peer.ID, game.Reference, kind, Widths[0])})
	}
	for _, image := range answer.Images {
		if image.Source != sourceIGDB && image.Source != sourceSteamGridDB {
			continue
		}
		image.Server, image.ServerName, image.Reference, image.ThumbPath = peer.ID, peer.Name, "", ""
		items = append(items, image)
	}
	if len(items) == 0 {
		state.State = "none"
	}
	if !answer.Ready {
		state.Reading = true
	}

	return items, state
}

// PeerCandidates — « Ce n'est pas le bon jeu ? » chez un pair : ses jeux du même `gameId` puis du titre `query` (vide :
// celui du jeu), leur jaquette par son relais ; puis, s'il partage ses fournisseurs, leurs candidats (« igdb:<id> »).
func PeerCandidates(ctx context.Context, peer Peer, consoleID, gamePath, reference, query string) ([]Candidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		query = searchTitle(GameName(gamePath))
	}
	answer, err := askPeer(ctx, peer, consoleID, GameKey(gamePath, reference), query, "")
	if err != nil {
		return nil, err
	}
	candidates := []Candidate{}
	for _, game := range answer.Items {
		candidates = append(candidates, Candidate{ExternalID: game.Reference, Name: game.Title, Year: yearOf(game.ReleaseDate),
			CoverPath: PeerImagePath(consoleID, peer.ID, game.Reference, KindCover, Widths[0])})
	}
	for _, provided := range answer.Candidates {
		if _, known := remoteServiceNamed(provided.Provider); !known || !externalIDPattern.MatchString(provided.ExternalID) {
			continue
		}
		provided.ExternalID = provided.Provider + ":" + provided.ExternalID
		if provided.CoverURL != "" && !allowedImage(provided.Provider, provided.CoverURL) {
			provided.CoverURL = ""
		}
		provided.CoverPath = ""
		candidates = append(candidates, provided)
	}

	return candidates, nil
}

// yearOf — l'année d'une sortie (« 2024-09-06 », « 2024 ») ; 0 sans elle.
func yearOf(releaseDate string) int {
	if len(releaseDate) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(releaseDate[:4])

	return year
}

// FetchPeerImage — l'image `kind` du jeu `reference` d'un pair, à la plus grande largeur, 16 Mo au plus : une image
// choisie dans une recherche, qui s'écrit ensuite comme les autres (SetManualImage). Le lien et le jeton viennent du
// registre, jamais de la page.
func FetchPeerImage(ctx context.Context, peer Peer, consoleID, reference, kind string) ([]byte, error) {
	if !validImageKind(kind) {
		return nil, ErrUnknownKind
	}
	content, err := peer.Client.GameImage(ctx, consoleID, reference, kind, peerCopyWidth, ChosenImageLimit)
	if err != nil {
		return nil, peerError(peer, err)
	}

	return content, nil
}

// fetchProviderImage — une image d'un fournisseur prêtée par un pair, récupérée chez l'hôte d'images de ce fournisseur
// seulement (FetchChosenImage) ; remplaçable dans un test.
var fetchProviderImage = FetchChosenImage

// providerReference — « igdb:123 » : un jeu d'un fournisseur prêté par un pair (`#719`), et non une référence de son
// listing.
func providerReference(externalID string) (provider, id string, ok bool) {
	provider, id, ok = strings.Cut(externalID, ":")
	if _, known := remoteServiceNamed(provider); !ok || !known || !externalIDPattern.MatchString(id) {
		return "", "", false
	}

	return provider, id, true
}

// FetchPeerSource — DÉSIGNER UN JEU CHEZ UN PAIR, et en copier la fiche dans la source `server` de mon jeu : le jeu
// `externalID` de son listing (son texte, la source d'origine de chaque champ, chacune de ses images), ou celui d'un de
// ses fournisseurs (« igdb:<id> », s'il les partage). Sans `externalID`, la copie se refait chez lui, par `gameId` puis
// par le titre. `name` : le titre du jeu chez lui, quand on le connaît. La copie remplace la précédente ; une image qui
// ne vient pas est notée au journal, et le reste se copie quand même. os.ErrNotExist : il n'a aucun jeu à proposer.
func FetchPeerSource(ctx context.Context, gameRoot string, peer Peer, consoleID, gamePath, reference, externalID, name string) (SourceState, error) {
	externalID = strings.TrimSpace(externalID)
	if len(externalID) > 1024 {
		return SourceState{}, fmt.Errorf("%w: too long", ErrInvalidExternalID)
	}
	if externalID == "" {
		answer, err := askPeer(ctx, peer, consoleID, GameKey(gamePath, reference), searchTitle(GameName(gamePath)), "")
		if err != nil {
			return SourceState{}, err
		}
		if len(answer.Items) == 0 {
			return SourceState{}, os.ErrNotExist
		}
		externalID, name = answer.Items[0].Reference, answer.Items[0].Title
	}

	copied := serverCopy{Source: sourceServer, Match: matchManualPin, Server: peer.ID, ServerName: peer.Name, Link: peer.Link,
		ExternalID: externalID, Name: name, FetchedAt: timeNow().UTC(), Sources: map[string]string{}}
	var images map[string][]byte
	if provider, id, isProvider := providerReference(externalID); isProvider {
		var sheet ProviderSheet
		if err := peer.Client.ProviderGame(ctx, consoleID, provider, id, &sheet); err != nil {
			return SourceState{}, peerError(peer, err)
		}
		copied.Provider, copied.Name = provider, firstOf(sheet.Name, name)
		copied.ReleaseDate, copied.Publishers, copied.Developers, copied.Genres, copied.Series = sheet.ReleaseDate, sheet.Publishers, sheet.Developers, sheet.Genres, sheet.Series
		for field, present := range copied.present() {
			if present {
				copied.Sources[field] = provider
			}
		}
		images = providerImages(ctx, provider, sheet.Images)
	} else {
		var info GameInfo
		if err := peer.Client.GameMetadata(ctx, consoleID, externalID, &info); err != nil {
			return SourceState{}, peerError(peer, err)
		}
		copied.Reference, copied.Name, copied.Title = externalID, firstOf(name, info.Title), info.Title
		copied.ReleaseDate, copied.Publishers, copied.Developers, copied.Genres, copied.Series, copied.Players =
			info.ReleaseDate, info.Publishers, info.Developers, info.Genres, info.Series, info.Players
		for field, present := range copied.present() {
			if present && info.Sources[field] != "" {
				copied.Sources[field] = info.Sources[field]
			}
		}
		images = lentImages(ctx, peer, consoleID, externalID)
	}
	if err := writeServerCopy(gameRoot, consoleID, gamePath, reference, copied, images); err != nil {
		return SourceState{}, err
	}
	forgetSeries(consoleID, reference)

	return SourceState{Source: sourceServer, Result: "found", Match: matchManualPin, ExternalID: externalID, Name: copied.Name, FetchedAt: copied.FetchedAt}, nil
}

// copiedKinds — les sortes d'image d'un jeu, dans l'ordre où elles se copient.
var copiedKinds = []string{KindCover, KindBackground, KindLogo, KindTitle, KindScreenshot, KindIcon}

// lentImages — chaque sorte d'image que le pair a de son jeu, en même temps, à la plus grande largeur. Une sorte qu'il
// n'a pas est sautée ; une qui échoue, notée.
func lentImages(ctx context.Context, peer Peer, consoleID, reference string) map[string][]byte {
	images := map[string][]byte{}
	var mutex sync.Mutex
	var wait sync.WaitGroup
	for _, kind := range copiedKinds {
		wait.Add(1)
		go func() {
			defer wait.Done()
			content, err := peer.Client.GameImage(ctx, consoleID, reference, kind, peerCopyWidth, ChosenImageLimit)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return
			case err != nil:
				slog.Warn("federated sheet copy: image not copied", "server", peer.Name, "console", consoleID, "kind", kind, "error", err)
				return
			}
			mutex.Lock()
			images[kind] = content
			mutex.Unlock()
		}()
	}
	wait.Wait()

	return images
}

// providerImages — les images d'un jeu d'un fournisseur prêté par un pair, chez l'hôte d'images de ce fournisseur.
func providerImages(ctx context.Context, provider string, addresses map[string]string) map[string][]byte {
	images := map[string][]byte{}
	for _, kind := range copiedKinds {
		address := addresses[kind]
		if address == "" {
			continue
		}
		content, err := fetchProviderImage(ctx, provider, address)
		if err != nil {
			slog.Warn("federated sheet copy: image not copied", "source", provider, "kind", kind, "error", err)
			continue
		}
		images[kind] = content
	}

	return images
}

// ── La source `server` : la fiche copiée ──────────────────────────────

// serverCopy — `server/source.json` : d'où vient la copie (le serveur, son lien, la référence de son jeu, ou le jeu de
// son fournisseur), quand, le texte du pair et la source d'origine de chaque champ, et le fichier de chaque image.
type serverCopy struct {
	Source      string            `json:"source"`
	Match       string            `json:"match"`
	Server      int64             `json:"server"`
	ServerName  string            `json:"serverName,omitempty"`
	Link        string            `json:"link,omitempty"`
	Reference   string            `json:"reference,omitempty"`
	Provider    string            `json:"provider,omitempty"`
	ExternalID  string            `json:"externalId"`
	Name        string            `json:"name,omitempty"`
	FetchedAt   time.Time         `json:"fetchedAt"`
	Title       string            `json:"title,omitempty"`
	ReleaseDate string            `json:"releaseDate,omitempty"`
	Publishers  []string          `json:"publishers,omitempty"`
	Developers  []string          `json:"developers,omitempty"`
	Genres      []string          `json:"genres,omitempty"`
	Series      string            `json:"series,omitempty"`
	Players     int               `json:"players,omitempty"`
	Sources     map[string]string `json:"sources,omitempty"`
	Images      map[string]string `json:"images,omitempty"`
}

// present — les champs du texte que la copie porte.
func (copied serverCopy) present() map[string]bool {
	return map[string]bool{
		"releaseDate": copied.ReleaseDate != "", "publishers": len(copied.Publishers) > 0, "developers": len(copied.Developers) > 0,
		"genres": len(copied.Genres) > 0, "series": copied.Series != "", "players": copied.Players > 0,
	}
}

func (copied serverCopy) text() gameText {
	return gameText{releaseDate: copied.ReleaseDate, publishers: copied.Publishers, developers: copied.Developers, genres: copied.Genres,
		series: copied.Series, players: copied.Players}
}

func readServerCopy(gameDir string) serverCopy {
	copied := serverCopy{}
	if content, err := os.ReadFile(filepath.Join(gameDir, sourceServer, "source.json")); err == nil {
		_ = json.Unmarshal(content, &copied)
	}

	return copied
}

// serverImage — l'image `kind` de la fiche copiée, vide si elle n'en a pas.
func serverImage(gameDir, kind string) string {
	file := readServerCopy(gameDir).Images[kind]
	if file == "" || filepath.Base(file) != file {
		return ""
	}
	path := filepath.Join(gameDir, sourceServer, file)
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return ""
	}

	return path
}

// writeServerCopy — la copie, à la place de la précédente : ses images (PNG, JPEG ou WebP, reconnues à leur contenu ;
// une autre est sautée) puis son `source.json`.
func writeServerCopy(gameRoot, consoleID, gamePath, reference string, copied serverCopy, images map[string][]byte) error {
	serverDir := filepath.Join(gameDirectory(gameRoot, consoleID, gamePath, reference), sourceServer)
	lock := lockFor(serverDir)
	lock.Lock()
	defer lock.Unlock()
	if err := os.RemoveAll(serverDir); err != nil {
		return err
	}
	copied.Images = map[string]string{}
	for kind, content := range images {
		extension := ImageExtension(content)
		if extension == "" {
			slog.Warn("federated sheet copy: not an image", "server", copied.ServerName, "console", consoleID, "kind", kind)
			continue
		}
		if extension != ".webp" {
			if _, err := decodeImage(content); err != nil {
				slog.Warn("federated sheet copy: unreadable image", "server", copied.ServerName, "console", consoleID, "kind", kind, "error", err)
				continue
			}
		}
		if err := writeAtomically(filepath.Join(serverDir, kind+extension), content); err != nil {
			return err
		}
		copied.Images[kind] = kind + extension
	}
	content, err := json.MarshalIndent(copied, "", "  ")
	if err != nil {
		return err
	}

	return writeAtomically(filepath.Join(serverDir, "source.json"), content)
}
