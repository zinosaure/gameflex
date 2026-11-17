package metadata

// LES GESTES DE L'ADMINISTRATEUR SUR LA FICHE D'UN JEU (chantier 49, §7) —
// Réglages › Jeux du launcher :
//
//   - la source `manual` : un texte corrigé à la main (titre, date, éditeurs,
//     développeurs, genres, série) dans `metadata/<console>/<gameid>/manual/`
//     `source.json`, et des images (`manual/<sorte>.<ext>`). Elle passe
//     devant toutes les autres, champ par champ et image par image (§3) ;
//   - les candidats d'IGDB ou de SteamGridDB pour un titre, et le jeu
//     désigné parmi eux (`manual-pin`), que les rafraîchissements gardent :
//     le remède au « mauvais jeu » ;
//   - redemander une source, ou l'oublier.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// candidateLimit — les candidats montrés à l'administrateur.
const candidateLimit = 10

var (
	// ErrUnknownSource — une source qui n'existe pas, ou qui ne se prête pas
	// à ce geste.
	ErrUnknownSource = errors.New("unknown source")
	// ErrServiceNotConfigured — le service n'a pas de compte saisi.
	ErrServiceNotConfigured = errors.New("service not configured")
	// ErrInvalidExternalID — un identifiant qui n'est pas celui d'un jeu du
	// service (ni IGDB ni SteamGridDB n'en ont d'autres que des nombres).
	ErrInvalidExternalID = errors.New("invalid external id")
	// ErrInvalidManual — un texte manuel mal formé.
	ErrInvalidManual = errors.New("invalid manual metadata")
)

// Candidate — un jeu que le service propose pour un titre. Chez un serveur fédéré (chantier 57, peers.go) : un de ses
// jeux, sa référence pour identifiant et sa jaquette par le relais du kernel (`coverPath`, l'adresse chez le kernel qui
// affiche) ; ou, s'il partage ses fournisseurs, un jeu de l'un d'eux (`provider`, l'identifiant « igdb:<id> »).
type Candidate struct {
	ExternalID string   `json:"externalId"`
	Name       string   `json:"name"`
	Year       int      `json:"year,omitempty"`
	Platforms  []string `json:"platforms,omitempty"`
	CoverURL   string   `json:"coverUrl,omitempty"`
	CoverPath  string   `json:"coverPath,omitempty"`
	Provider   string   `json:"provider,omitempty"`
}

// SourceState — ce que garde une source à compte pour un jeu, tel que
// l'administrateur le voit après un geste.
type SourceState struct {
	Source     string    `json:"source"`
	Result     string    `json:"result"` // found, notFound, error
	Match      string    `json:"match,omitempty"`
	ExternalID string    `json:"externalId,omitempty"`
	Name       string    `json:"name,omitempty"`
	Error      string    `json:"error,omitempty"`
	FetchedAt  time.Time `json:"fetchedAt"`
}

func remoteServiceNamed(source string) (remoteService, bool) {
	switch source {
	case sourceIGDB:
		return igdbService{}, true
	case sourceSteamGridDB:
		return steamGridDBService{}, true
	}

	return nil, false
}

// gameDirectory — `metadata/<console>/<gameid>/` sous `gameRoot`, la racine
// de la source du jeu (Roots.Game) : LE SEUL POINT où il se compose, pour la
// fiche, les images et les gestes de l'administrateur. Le dossier que le jeu
// avait sous sa référence l'y suit, à ce premier accès (followReference).
func gameDirectory(gameRoot, consoleID, gamePath, reference string) string {
	gameID := GameKey(gamePath, reference)
	followReference(gameRoot, consoleID, reference, gameID)

	return filepath.Join(gameRoot, "metadata", consoleID, gameID)
}

// Candidates — les jeux que `source` (igdb, steamgriddb) trouve pour `query`
// sur la console ; `query` vide : le titre du jeu.
func Candidates(ctx context.Context, accounts Accounts, consoleID, gamePath, source, query string) ([]Candidate, error) {
	service, known := remoteServiceNamed(source)
	if !known {
		return nil, ErrUnknownSource
	}
	if !service.configured(accounts) {
		return nil, ErrServiceNotConfigured
	}
	query = strings.TrimSpace(query)
	if query == "" {
		query = searchTitle(GameName(gamePath))
	}

	return service.candidates(ctx, accounts, consoleID, query)
}

// FetchSource — refait tout de suite la recherche de `source` pour le jeu,
// ou, `externalID` donné, lui désigne ce jeu-là (`manual-pin`), que les
// rafraîchissements suivants relisent par son identifiant. Une recherche
// sans identifiant oublie la désignation. Les images d'avant partent ; les
// nouvelles se téléchargent à la demande. os.ErrNotExist : le service n'a
// pas de jeu de cet identifiant, et rien n'a changé.
func FetchSource(ctx context.Context, gameRoot string, accounts Accounts, consoleID, gamePath, reference, source, externalID string) (SourceState, error) {
	service, known := remoteServiceNamed(source)
	if !known {
		return SourceState{}, ErrUnknownSource
	}
	if !service.configured(accounts) {
		return SourceState{}, ErrServiceNotConfigured
	}
	externalID = strings.TrimSpace(externalID)
	if externalID != "" && !externalIDPattern.MatchString(externalID) {
		return SourceState{}, fmt.Errorf("%w: %q", ErrInvalidExternalID, externalID)
	}
	sourceDir := filepath.Join(gameDirectory(gameRoot, consoleID, gamePath, reference), source)
	title := searchTitle(GameName(gamePath))

	lock := lockFor(sourceDir)
	lock.Lock()
	defer lock.Unlock()
	var state remoteState
	var err error
	if externalID == "" {
		state, err = remoteRefresh(ctx, service, accounts, sourceDir, consoleID, title, "")
	} else {
		found, lookupErr := service.lookupByID(ctx, accounts, consoleID, externalID)
		switch {
		case lookupErr != nil:
			return SourceState{}, fmt.Errorf("%s: %w", source, lookupErr)
		case found == nil:
			return SourceState{}, os.ErrNotExist
		}
		state, err = remoteStore(service, sourceDir, title, externalID, found, nil)
	}
	if source == sourceIGDB {
		forgetSeries(consoleID, reference)
	}

	return SourceState{Source: source, Result: state.Result, Match: state.Match, ExternalID: state.ExternalID, Name: state.Name, Error: state.Error, FetchedAt: state.FetchedAt}, err
}

var externalIDPattern = regexp.MustCompile(`^[0-9]{1,18}$`)

// droppableSources — les sources dont le cache d'un jeu se jette : il se
// refait à la lecture suivante. `manual` est LA RÉINITIALISATION DE LA FICHE :
// les textes corrigés et les images remplacées partent d'un coup, la fiche
// revient à ce que donnent les services (le porteur, 2026-11-14 : « Le bouton
// réinitialiser permet de tout remettre comme avant. C'est suffisant »). Ce
// n'est plus une source qu'on choisit d'oublier, et la fiche ne dit plus
// qu'un texte ou une image vient de là (info.go).
// `server` est la fiche copiée d'un serveur fédéré (chantier 57) : elle ne se refait pas seule, elle part.
var droppableSources = map[string]bool{sourceIGDB: true, sourceSteamGridDB: true, sourceLibretro: true, sourceManual: true, sourceServer: true}

// DropSource — oublie ce que `source` a gardé pour le jeu, images comprises.
func DropSource(gameRoot, consoleID, gamePath, reference, source string) error {
	if !droppableSources[source] {
		return ErrUnknownSource
	}
	sourceDir := filepath.Join(gameDirectory(gameRoot, consoleID, gamePath, reference), source)

	lock := lockFor(sourceDir)
	lock.Lock()
	defer lock.Unlock()
	if err := os.RemoveAll(sourceDir); err != nil {
		return err
	}
	if source == sourceIGDB || source == sourceManual || source == sourceServer {
		forgetSeries(consoleID, reference)
	}

	return nil
}

// ── La source `manual` ──────────────────────────────────────────────────

// manualState — `manual/source.json`.
type manualState struct {
	Source      string    `json:"source"`
	Title       string    `json:"title,omitempty"`
	ReleaseDate string    `json:"releaseDate,omitempty"`
	Publishers  []string  `json:"publishers,omitempty"`
	Developers  []string  `json:"developers,omitempty"`
	Genres      []string  `json:"genres,omitempty"`
	Series      string    `json:"series,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (state manualState) text() gameText {
	return gameText{releaseDate: state.ReleaseDate, publishers: state.Publishers, developers: state.Developers, genres: state.Genres, series: state.Series}
}

func readManualState(gameDir string) manualState {
	state := manualState{Source: sourceManual}
	if content, err := os.ReadFile(filepath.Join(gameDir, sourceManual, "source.json")); err == nil {
		_ = json.Unmarshal(content, &state)
	}

	return state
}

// manualReleaseDate — une année, ou une date entière.
var manualReleaseDate = regexp.MustCompile(`^[0-9]{4}(-[0-9]{2}-[0-9]{2})?$`)

const (
	manualTextLimit = 200
	manualListLimit = 30
)

// SetManualText — applique au texte corrigé du jeu les champs de `changes`
// (title, releaseDate, publishers, developers, genres, series) : un champ
// absent ne change pas, `null` ou vide l'efface (les autres sources
// reprennent la main). Tout ou rien : un champ mal formé, et rien ne
// s'écrit.
func SetManualText(gameRoot, consoleID, gamePath, reference string, changes map[string]json.RawMessage) error {
	gameDir := gameDirectory(gameRoot, consoleID, gamePath, reference)
	lock := lockFor(filepath.Join(gameDir, sourceManual))
	lock.Lock()
	defer lock.Unlock()
	state := readManualState(gameDir)

	for field, raw := range changes {
		isNull := strings.TrimSpace(string(raw)) == "null"
		switch field {
		case "title", "releaseDate", "series":
			var value string
			if !isNull {
				if err := json.Unmarshal(raw, &value); err != nil {
					return fmt.Errorf("%w: %s must be a string or null", ErrInvalidManual, field)
				}
			}
			value = strings.TrimSpace(value)
			if len([]rune(value)) > manualTextLimit {
				return fmt.Errorf("%w: %s is too long", ErrInvalidManual, field)
			}
			switch field {
			case "title":
				state.Title = value
			case "releaseDate":
				if value != "" && !manualReleaseDate.MatchString(value) {
					return fmt.Errorf("%w: releaseDate is YYYY or YYYY-MM-DD", ErrInvalidManual)
				}
				if value != "" && len(value) == 10 {
					if _, err := time.Parse("2006-01-02", value); err != nil {
						return fmt.Errorf("%w: releaseDate is not a date", ErrInvalidManual)
					}
				}
				state.ReleaseDate = value
			case "series":
				state.Series = value
			}
		case "publishers", "developers", "genres":
			var values []string
			if !isNull {
				if err := json.Unmarshal(raw, &values); err != nil {
					return fmt.Errorf("%w: %s must be a list of strings or null", ErrInvalidManual, field)
				}
			}
			cleaned := []string{}
			for _, value := range values {
				value = strings.TrimSpace(value)
				if len([]rune(value)) > manualTextLimit {
					return fmt.Errorf("%w: a value of %s is too long", ErrInvalidManual, field)
				}
				cleaned = appendNew(cleaned, value)
			}
			if len(cleaned) > manualListLimit {
				return fmt.Errorf("%w: %s has too many values", ErrInvalidManual, field)
			}
			if len(cleaned) == 0 {
				cleaned = nil
			}
			switch field {
			case "publishers":
				state.Publishers = cleaned
			case "developers":
				state.Developers = cleaned
			case "genres":
				state.Genres = cleaned
			}
		default:
			return fmt.Errorf("%w: unknown field %q", ErrInvalidManual, field)
		}
	}

	state.Source, state.UpdatedAt = sourceManual, timeNow().UTC()
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomically(filepath.Join(gameDir, sourceManual, "source.json"), content); err != nil {
		return err
	}
	forgetSeries(consoleID, reference)

	return nil
}

// manualImageExtensions — les extensions d'une image manuelle.
var manualImageExtensions = []string{".png", ".jpg", ".webp"}

// manualImage — l'image `kind` déposée par l'administrateur, vide s'il n'y
// en a pas. Ses réductions (`cover-512.jpg`) sont à côté, sous un autre nom.
func manualImage(gameDir, kind string) string {
	for _, extension := range manualImageExtensions {
		path := filepath.Join(gameDir, sourceManual, kind+extension)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}

	return ""
}

func validImageKind(kind string) bool {
	_, known := retroOrder[kind]

	return known
}

// removeManualImage — l'image `kind` sous toutes ses extensions, et ses
// réductions.
func removeManualImage(manualDir, kind string) {
	for _, extension := range manualImageExtensions {
		_ = os.Remove(filepath.Join(manualDir, kind+extension))
	}
	reductions, _ := filepath.Glob(filepath.Join(manualDir, kind+"-*"))
	for _, reduction := range reductions {
		_ = os.Remove(reduction)
	}
}

// SetManualImage — l'image `kind` du jeu, déposée par l'administrateur
// (PNG, JPEG ou WebP, reconnus à leur contenu) : elle passe devant toutes les
// sources.
func SetManualImage(gameRoot, consoleID, gamePath, reference, kind string, content []byte) error {
	if !validImageKind(kind) {
		return ErrUnknownKind
	}
	extension := ImageExtension(content)
	if extension == "" {
		return fmt.Errorf("%w: not a PNG, JPEG or WebP", ErrUnreadableImage)
	}
	if extension != ".webp" {
		if _, err := decodeImage(content); err != nil {
			return fmt.Errorf("%w: %v", ErrUnreadableImage, err)
		}
	}
	manualDir := filepath.Join(gameDirectory(gameRoot, consoleID, gamePath, reference), sourceManual)

	lock := lockFor(filepath.Join(manualDir, kind))
	lock.Lock()
	defer lock.Unlock()
	removeManualImage(manualDir, kind)

	return writeAtomically(filepath.Join(manualDir, kind+extension), content)
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
