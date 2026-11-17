package metadata

// LES SOURCES À COMPTE (chantier 49, P4) — IGDB et SteamGridDB : une
// recherche par titre et console, qui rend l'adresse de chaque sorte d'image
// et, pour IGDB, le texte de la fiche (date de sortie, éditeurs,
// développeurs, genres) ; puis l'image elle-même au premier appel.
//
// Le résultat de la recherche se garde dans
// `metadata/<console>/<gameid>/<source>/source.json` : un jeu introuvable
// n'est redemandé qu'après 30 jours, une erreur après une heure. Une seule
// recherche par jeu sert les images ET le texte. Un service sans compte est
// simplement sauté.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Accounts — les comptes des services, tels que l'administrateur les a
// saisis (service d'identité, table metadata_services). Vide : aucun service
// à compte.
type Accounts map[string]map[string]string

// remoteLookup — ce qu'une recherche a trouvé.
type remoteLookup struct {
	ExternalID string
	Name       string
	URLs       map[string]string // sorte d'image → adresse
	Text       gameText
	// Les collections et franchises d'IGDB, dans son ordre : la série s'en
	// tire à la lecture (seriesName).
	Collections, Franchises []string
}

// remoteService — une source à compte.
type remoteService interface {
	name() string
	// configured — les champs du compte sont là.
	configured(accounts Accounts) bool
	// lookup — le jeu `title` sur la console `consoleID`. Un jeu introuvable
	// rend (nil, nil).
	lookup(ctx context.Context, accounts Accounts, consoleID, title string) (*remoteLookup, error)
	// lookupByID — le jeu d'identifiant `externalID` chez le service, celui
	// que l'administrateur a désigné ; (nil, nil) s'il n'existe pas.
	lookupByID(ctx context.Context, accounts Accounts, consoleID, externalID string) (*remoteLookup, error)
	// candidates — les jeux que le service trouve pour `query`, dans son
	// ordre, `candidateLimit` au plus.
	candidates(ctx context.Context, accounts Accounts, consoleID, query string) ([]Candidate, error)
}

// remoteFormat — la forme de `source.json`. Un fichier d'une forme trop
// ancienne pour son service est refait, jamais lu à moitié : le texte (2)
// puis la série (3) se sont ajoutés aux images (1) ; la 5 a refait chaque
// recherche avec le choix du candidat de titles.go (la 4 n'a vécu qu'en
// essai sur la dev) ; la 6 garde les collections et franchises d'IGDB, et la
// série s'en calcule à la lecture : une règle de série changée vaut aussitôt,
// sans rien redemander.
const remoteFormat = 6

// remoteFormatNeeded — la forme la plus ancienne qu'un service accepte
// encore : rien n'a changé pour SteamGridDB depuis la 5, ses images ne se
// retéléchargent pas pour une affaire de série.
func remoteFormatNeeded(service string) int {
	if service == sourceSteamGridDB {
		return 5
	}

	return remoteFormat
}

type remoteState struct {
	Source      string            `json:"source"`
	Format      int               `json:"format"`
	Result      string            `json:"result"` // found, notFound, error
	FetchedAt   time.Time         `json:"fetchedAt"`
	Match       string            `json:"match,omitempty"`
	ExternalID  string            `json:"externalId,omitempty"`
	Name        string            `json:"name,omitempty"`
	Query       string            `json:"query,omitempty"`
	ReleaseDate string            `json:"releaseDate,omitempty"` // AAAA-MM-JJ, en UTC
	Developers  []string          `json:"developers,omitempty"`
	Publishers  []string          `json:"publishers,omitempty"`
	Genres      []string          `json:"genres,omitempty"`
	Collections []string          `json:"collections,omitempty"` // IGDB, dans son ordre
	Franchises  []string          `json:"franchises,omitempty"`
	URLs        map[string]string `json:"urls,omitempty"`
	Images      map[string]string `json:"images,omitempty"` // sorte → fichier téléchargé
	Error       string            `json:"error,omitempty"`
}

var remoteClient = &http.Client{Timeout: 30 * time.Second}

// Remote — le fichier de l'image `kind` du jeu chez `service`, téléchargé au
// premier appel. os.ErrNotExist si le service n'a pas de compte, ne connaît
// pas le jeu ou pas cette image.
func Remote(ctx context.Context, service remoteService, accounts Accounts, gameDir, consoleID, title, kind string) (string, error) {
	if !service.configured(accounts) || title == "" {
		return "", os.ErrNotExist
	}
	sourceDir := filepath.Join(gameDir, service.name())
	lock := lockFor(sourceDir)
	lock.Lock()
	defer lock.Unlock()

	state, err := remoteFound(ctx, service, accounts, sourceDir, consoleID, title)
	if err != nil {
		return "", err
	}
	if state.Result != "found" || state.URLs[kind] == "" {
		return "", os.ErrNotExist
	}
	if file := state.Images[kind]; file != "" {
		if _, err := os.Stat(filepath.Join(sourceDir, file)); err == nil {
			return filepath.Join(sourceDir, file), nil
		}
	}

	extension := strings.ToLower(path.Ext(strings.SplitN(state.URLs[kind], "?", 2)[0]))
	if extension != ".png" && extension != ".jpg" && extension != ".jpeg" && extension != ".webp" {
		extension = ".img"
	}
	file := kind + extension
	if err := downloadTo(ctx, state.URLs[kind], filepath.Join(sourceDir, file)); err != nil {
		return "", fmt.Errorf("%s: %w", service.name(), err)
	}
	state.Images[kind] = file
	if err := writeRemoteState(sourceDir, state); err != nil {
		return "", err
	}

	return filepath.Join(sourceDir, file), nil
}

// remoteText — le texte de la fiche du jeu chez `service`, par la même
// recherche que ses images. os.ErrNotExist si le service n'a pas de compte
// ou ne connaît pas le jeu.
func remoteText(ctx context.Context, service remoteService, accounts Accounts, gameDir, consoleID, title string) (gameText, error) {
	if !service.configured(accounts) || title == "" {
		return gameText{}, os.ErrNotExist
	}
	sourceDir := filepath.Join(gameDir, service.name())
	lock := lockFor(sourceDir)
	lock.Lock()
	defer lock.Unlock()

	state, err := remoteFound(ctx, service, accounts, sourceDir, consoleID, title)
	if err != nil {
		return gameText{}, err
	}
	if state.Result != "found" {
		return gameText{}, os.ErrNotExist
	}

	return state.text(), nil
}

// cachedRemoteText — le texte déjà gardé du jeu chez `service`, sans rien
// demander : vide s'il n'y a pas de recherche réussie, de la forme du jour.
func cachedRemoteText(service remoteService, gameDir string) gameText {
	state := readRemoteState(filepath.Join(gameDir, service.name()), service.name())
	if state.Format < remoteFormatNeeded(service.name()) || state.Result != "found" {
		return gameText{}
	}

	return state.text()
}

func (state remoteState) text() gameText {
	return gameText{releaseDate: state.ReleaseDate, publishers: state.Publishers, developers: state.Developers, genres: state.Genres,
		series: seriesName(state.Name, state.Collections, state.Franchises)}
}

// matchManualPin — le jeu a été désigné par l'administrateur (son
// identifiant chez le service) : il se relit par cet identifiant, jamais par
// une recherche.
const matchManualPin = "manual-pin"

// remoteFound — l'état de la recherche du jeu chez `service`, refaite quand
// elle manque, a changé de titre, date (introuvable depuis 30 jours, erreur
// depuis une heure) ou d'une autre forme. Un jeu désigné par
// l'administrateur se relit par son identifiant, et le titre n'y change
// rien. Le verrou de `sourceDir` est tenu par l'appelant.
func remoteFound(ctx context.Context, service remoteService, accounts Accounts, sourceDir, consoleID, title string) (remoteState, error) {
	state := readRemoteState(sourceDir, service.name())
	pinned := state.Match == matchManualPin && state.ExternalID != ""
	stale := state.Result == "" ||
		state.Format < remoteFormatNeeded(service.name()) ||
		(state.Result == "notFound" && timeNow().Sub(state.FetchedAt) >= libretroAbsentAge) ||
		(state.Result == "error" && timeNow().Sub(state.FetchedAt) >= libretroErrorAge) ||
		(!pinned && state.Query != title)
	if !stale {
		return state, nil
	}
	pin := ""
	if pinned {
		pin = state.ExternalID
	}

	return remoteRefresh(ctx, service, accounts, sourceDir, consoleID, title, pin)
}

// remoteRefresh — refait la recherche par titre (`pin` vide) ou relit le jeu
// désigné, et le garde. Les images de l'état d'avant partent avec lui : un
// logo d'un autre jeu ne doit pas rester à côté du nouveau résultat.
func remoteRefresh(ctx context.Context, service remoteService, accounts Accounts, sourceDir, consoleID, title, pin string) (remoteState, error) {
	var found *remoteLookup
	var err error
	if pin != "" {
		found, err = service.lookupByID(ctx, accounts, consoleID, pin)
	} else {
		found, err = service.lookup(ctx, accounts, consoleID, title)
	}

	return remoteStore(service, sourceDir, title, pin, found, err)
}

// remoteStore — l'état d'une recherche ou d'une relecture, écrit dans
// `source.json` à la place du précédent et de ses images. Un jeu désigné
// garde sa désignation même quand sa relecture échoue.
func remoteStore(service remoteService, sourceDir, title, pin string, found *remoteLookup, err error) (remoteState, error) {
	removeRemoteImages(sourceDir)
	state := remoteState{Source: service.name(), Format: remoteFormat, FetchedAt: timeNow().UTC(), Query: title, Images: map[string]string{}}
	if pin != "" {
		state.Match, state.ExternalID = matchManualPin, pin
	}
	switch {
	case err != nil:
		state.Result, state.Error = "error", err.Error()
	case found == nil:
		state.Result = "notFound"
	default:
		state.Result, state.ExternalID, state.Name, state.URLs = "found", found.ExternalID, found.Name, found.URLs
		if pin == "" {
			state.Match = "title+platform"
		}
		state.ReleaseDate, state.Publishers, state.Developers, state.Genres = found.Text.releaseDate, found.Text.publishers, found.Text.developers, found.Text.genres
		state.Collections, state.Franchises = found.Collections, found.Franchises
	}
	if writeErr := writeRemoteState(sourceDir, state); writeErr != nil {
		return state, writeErr
	}
	if err != nil {
		return state, fmt.Errorf("%s: %w", service.name(), err)
	}

	return state, nil
}

// removeRemoteImages — tout le dossier d'une source sauf son `source.json` :
// les images téléchargées et leurs réductions.
func removeRemoteImages(sourceDir string) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() != "source.json" {
			_ = os.RemoveAll(filepath.Join(sourceDir, entry.Name()))
		}
	}
}

func readRemoteState(sourceDir, name string) remoteState {
	state := remoteState{Source: name}
	if content, err := os.ReadFile(filepath.Join(sourceDir, "source.json")); err == nil {
		_ = json.Unmarshal(content, &state)
	}
	if state.Images == nil {
		state.Images = map[string]string{}
	}

	return state
}

func writeRemoteState(sourceDir string, state remoteState) error {
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return writeAtomically(filepath.Join(sourceDir, "source.json"), content)
}

func downloadTo(ctx context.Context, address, target string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	response, err := remoteClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New(response.Status)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return err
	}

	return writeAtomically(target, content)
}

// searchTitle — le titre à chercher : le nom du jeu sans ses mentions entre
// parenthèses ou crochets (région, langues, version).
func searchTitle(gameName string) string {
	return strings.Join(strings.Fields(libretroMentions.ReplaceAllString(gameName, " ")), " ")
}

// SearchTitle — le titre à chercher pour le jeu rangé en `gamePath` : celui
// que la recherche d'images prend quand la page n'en donne pas.
func SearchTitle(gamePath string) string {
	return searchTitle(GameName(gamePath))
}
