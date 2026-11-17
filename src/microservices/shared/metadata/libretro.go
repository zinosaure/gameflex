package metadata

// LA SOURCE `libretro` (chantier 49, P3) — libretro-thumbnails, sans compte ni
// clé : des jaquettes (`Named_Boxarts`), des captures (`Named_Snaps`), des
// écrans-titres (`Named_Titles`) et des logos (`Named_Logos`), rangés par
// console sous le nom canonique du jeu (celui de No-Intro et de Redump).
//
// Le nom se trouve dans la liste que le site publie pour chaque console et
// chaque sorte d'image, gardée dans `cache/metadata/sources/libretro/` et
// rafraîchie chaque semaine : exact quand le fichier porte déjà le nom
// canonique (`match: "gameid"`), sinon par le titre sans ses mentions
// (`match: "title+platform"`, « Banjo Kazooie » → « Banjo-Kazooie (USA) »).
//
// À la demande, une image à la fois, et le résultat est noté dans
// `metadata/<console>/<gameid>/libretro/source.json` : une image absente ne se
// redemande qu'après 30 jours, une erreur du réseau après une heure.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

// libretroBase — le site des vignettes ; remplaçable dans un test.
var libretroBase = "https://thumbnails.libretro.com"

// libretroSystems — les dossiers du site pour chaque console du registre,
// dans l'ordre où on les essaie.
var libretroSystems = map[string][]string{
	"nes":        {"Nintendo - Nintendo Entertainment System", "Nintendo - Family Computer Disk System"},
	"snes":       {"Nintendo - Super Nintendo Entertainment System"},
	"gbc":        {"Nintendo - Game Boy Color", "Nintendo - Game Boy"},
	"gba":        {"Nintendo - Game Boy Advance"},
	"vb":         {"Nintendo - Virtual Boy"},
	"n64":        {"Nintendo - Nintendo 64"},
	"nds":        {"Nintendo - Nintendo DS"},
	"gc":         {"Nintendo - GameCube"},
	"wii":        {"Nintendo - Wii"},
	"wiiu":       {"Nintendo - Wii U"},
	"sms":        {"Sega - Master System - Mark III", "Sega - Game Gear"},
	"megadrive":  {"Sega - Mega Drive - Genesis", "Sega - 32X", "Sega - Mega-CD - Sega CD"},
	"saturn":     {"Sega - Saturn"},
	"dreamcast":  {"Sega - Dreamcast"},
	"atari2600":  {"Atari - 2600"},
	"atari5200":  {"Atari - 5200"},
	"atari7800":  {"Atari - 7800"},
	"lynx":       {"Atari - Lynx"},
	"jaguar":     {"Atari - Jaguar"},
	"pce":        {"NEC - PC Engine - TurboGrafx 16", "NEC - PC Engine CD - TurboGrafx-CD"},
	"pcfx":       {"NEC - PC-FX"},
	"neogeo":     {"SNK - Neo Geo", "FBNeo - Arcade Games"},
	"ngp":        {"SNK - Neo Geo Pocket Color", "SNK - Neo Geo Pocket"},
	"cps1":       {"FBNeo - Arcade Games", "MAME"},
	"cps2":       {"FBNeo - Arcade Games", "MAME"},
	"wswan":      {"Bandai - WonderSwan Color", "Bandai - WonderSwan"},
	"coleco":     {"Coleco - ColecoVision"},
	"vectrex":    {"GCE - Vectrex"},
	"c64":        {"Commodore - 64"},
	"amstrad":    {"Amstrad - CPC"},
	"zxspectrum": {"Sinclair - ZX Spectrum"},
	"doom":       {"DOOM"},
	"scummvm":    {"ScummVM"},
	"ps1":        {"Sony - PlayStation"},
	"ps2":        {"Sony - PlayStation 2"},
	"psp":        {"Sony - PlayStation Portable"},
	"ps3":        {"Sony - PlayStation 3"},
	"ps4":        {"Sony - PlayStation 4"},
	"xbox":       {"Microsoft - Xbox"},
	"xbox360":    {"Microsoft - Xbox 360"},
}

// libretroFolders — la sorte d'image, et son dossier sur le site.
var libretroFolders = map[string]string{
	KindCover:      "Named_Boxarts",
	KindScreenshot: "Named_Snaps",
	KindTitle:      "Named_Titles",
	KindLogo:       "Named_Logos",
}

const (
	libretroIndexAge  = 7 * 24 * time.Hour
	libretroAbsentAge = 30 * 24 * time.Hour
	libretroErrorAge  = time.Hour
)

var libretroClient = &http.Client{Timeout: 30 * time.Second}

// libretroSlots — quatre téléchargements à la fois au plus : une grille de
// jaquettes ne doit pas assaillir un service gratuit.
var libretroSlots = make(chan struct{}, 4)

// libretroSource — ce que `source.json` garde de la source.
type libretroSource struct {
	Source string                   `json:"source"`
	Images map[string]libretroImage `json:"images"`
}

type libretroImage struct {
	Result    string    `json:"result"` // found, notFound, error
	FetchedAt time.Time `json:"fetchedAt"`
	Match     string    `json:"match,omitempty"`
	System    string    `json:"system,omitempty"`
	Name      string    `json:"name,omitempty"`
	File      string    `json:"file,omitempty"`
}

// Libretro — le fichier d'origine de l'image `kind` du jeu, téléchargé au
// premier appel dans `<gameDir>/libretro/`. os.ErrNotExist si la console n'a
// pas de dossier sur le site ou le jeu pas cette image.
func Libretro(ctx context.Context, cacheRoot, gameDir, consoleID, gameName, kind string) (string, error) {
	folder, known := libretroFolders[kind]
	systems := libretroSystems[consoleID]
	if !known || len(systems) == 0 || gameName == "" {
		return "", os.ErrNotExist
	}

	sourceDir := filepath.Join(gameDir, "libretro")
	lock := lockFor(sourceDir)
	lock.Lock()
	defer lock.Unlock()

	state := readLibretroSource(sourceDir)
	if previous, found := state.Images[kind]; found {
		switch {
		case previous.Result == "found" && previous.File != "":
			path := filepath.Join(sourceDir, previous.File)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		case previous.Result == "notFound" && timeNow().Sub(previous.FetchedAt) < libretroAbsentAge:
			return "", os.ErrNotExist
		case previous.Result == "error" && timeNow().Sub(previous.FetchedAt) < libretroErrorAge:
			return "", os.ErrNotExist
		}
	}

	record := libretroImage{Result: "notFound", FetchedAt: timeNow().UTC()}
	var fetchErr error
	for _, system := range systems {
		index, err := libretroIndex(ctx, cacheRoot, system, folder)
		if err != nil {
			fetchErr = err
			continue
		}
		name, exact := index.match(gameName)
		if name == "" {
			continue
		}
		file := kind + ".png"
		if err := libretroDownload(ctx, system, folder, name, filepath.Join(sourceDir, file)); err != nil {
			fetchErr = err
			break
		}
		record = libretroImage{Result: "found", FetchedAt: record.FetchedAt, Match: "title+platform", System: system, Name: name, File: file}
		if exact {
			record.Match = "gameid"
		}
		break
	}
	if record.Result != "found" && fetchErr != nil {
		record.Result = "error"
	}
	state.Images[kind] = record
	if err := writeLibretroSource(sourceDir, state); err != nil {
		return "", err
	}
	if record.Result != "found" {
		if fetchErr != nil {
			return "", fmt.Errorf("libretro: %w", fetchErr)
		}
		return "", os.ErrNotExist
	}

	return filepath.Join(sourceDir, record.File), nil
}

func readLibretroSource(sourceDir string) libretroSource {
	state := libretroSource{Source: "libretro", Images: map[string]libretroImage{}}
	if content, err := os.ReadFile(filepath.Join(sourceDir, "source.json")); err == nil {
		_ = json.Unmarshal(content, &state)
	}
	if state.Images == nil {
		state.Images = map[string]libretroImage{}
	}
	state.Source = "libretro"

	return state
}

func writeLibretroSource(sourceDir string, state libretroSource) error {
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return writeAtomically(filepath.Join(sourceDir, "source.json"), content)
}

// writeAtomically — écrit à côté puis renomme.
func writeAtomically(target string, content []byte) error {
	return writeStreamAtomically(target, bytes.NewReader(content))
}

// writeStreamAtomically — la même chose depuis un flux, sans le garder en
// mémoire : un `.dat` de la libretro-database pèse jusqu'à 7 Mo.
func writeStreamAtomically(target string, content io.Reader) error {
	if err := makeOwnedDir(filepath.Dir(target)); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := io.Copy(temporary, content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}

	return renameOwned(temporary.Name(), target)
}

// libretroURL — l'adresse d'un dossier ou d'une image du site.
func libretroURL(parts ...string) string {
	escaped := make([]string, len(parts))
	for index, part := range parts {
		escaped[index] = url.PathEscape(part)
	}

	return libretroBase + "/" + strings.Join(escaped, "/")
}

func libretroDownload(ctx context.Context, system, folder, name, target string) error {
	libretroSlots <- struct{}{}
	defer func() { <-libretroSlots }()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, libretroURL(system, folder, name+".png"), nil)
	if err != nil {
		return err
	}
	response, err := libretroClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", name, response.Status)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return err
	}

	return writeAtomically(target, content)
}

// ── La liste des noms d'un dossier du site ─────────────────────────────

type libretroNames struct {
	exact map[string]string   // nom tel quel → nom
	loose map[string][]string // titre sans mentions → noms
}

var (
	indexesMutex sync.Mutex
	indexes      = map[string]*libretroNames{}
	indexLoaded  = map[string]time.Time{}
)

// libretroHref — une image de la page d'index du site.
var libretroHref = regexp.MustCompile(`<a href="([^"?/][^"]*)\.png">`)

// libretroIndex — les noms d'un dossier du site : en mémoire, sinon le
// fichier gardé s'il a moins d'une semaine, sinon la page du site.
func libretroIndex(ctx context.Context, cacheRoot, system, folder string) (*libretroNames, error) {
	key := system + "/" + folder
	indexesMutex.Lock()
	defer indexesMutex.Unlock()
	if names, found := indexes[key]; found && timeNow().Sub(indexLoaded[key]) < libretroIndexAge {
		return names, nil
	}

	cached := filepath.Join(cacheRoot, "libretro", system, folder+".txt")
	var list []string
	if info, err := os.Stat(cached); err == nil && timeNow().Sub(info.ModTime()) < libretroIndexAge {
		if content, err := os.ReadFile(cached); err == nil {
			list = strings.Split(strings.TrimSpace(string(content)), "\n")
		}
	}
	if list == nil {
		fetched, err := fetchLibretroIndex(ctx, system, folder)
		if err != nil {
			return nil, err
		}
		list = fetched
		if err := writeAtomically(cached, []byte(strings.Join(list, "\n")+"\n")); err != nil {
			return nil, err
		}
	}

	names := newLibretroNames(list)
	indexes[key] = names
	indexLoaded[key] = timeNow()

	return names, nil
}

func fetchLibretroIndex(ctx context.Context, system, folder string) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, libretroURL(system, folder)+"/", nil)
	if err != nil {
		return nil, err
	}
	response, err := libretroClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return []string{}, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index %s/%s: %s", system, folder, response.Status)
	}
	page, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	list := []string{}
	for _, found := range libretroHref.FindAllSubmatch(page, -1) {
		// L'adresse est échappée deux fois : en HTML (« &amp; ») puis en URL.
		if name, err := url.PathUnescape(html.UnescapeString(string(found[1]))); err == nil {
			list = append(list, name)
		}
	}
	if len(list) == 0 && len(page) > 0 && !strings.Contains(string(page), "Index of") {
		return nil, errors.New("index " + system + "/" + folder + ": unexpected page")
	}

	return list, nil
}

func newLibretroNames(list []string) *libretroNames {
	names := &libretroNames{exact: map[string]string{}, loose: map[string][]string{}}
	for _, name := range list {
		if name == "" {
			continue
		}
		names.exact[name] = name
		loose := looseTitle(name)
		names.loose[loose] = append(names.loose[loose], name)
	}

	return names
}

// libretroForbidden — les caractères que le site remplace par « _ » dans ses
// noms de fichiers.
var libretroForbidden = strings.NewReplacer("&", "_", "*", "_", "/", "_", ":", "_", "`", "_", "<", "_", ">", "_", "?", "_", `\`, "_", "|", "_")

// match — le nom du site pour ce jeu, et s'il est exact.
func (names *libretroNames) match(gameName string) (string, bool) {
	for _, candidate := range []string{gameName, libretroForbidden.Replace(gameName)} {
		if name, found := names.exact[candidate]; found {
			return name, true
		}
	}
	candidates := names.loose[looseTitle(gameName)]
	if len(candidates) == 0 {
		return "", false
	}

	return bestCandidate(candidates, gameName), false
}

// bestCandidate — parmi les éditions d'un même titre : celle qui partage la
// région du fichier, sinon la plus courante (USA, World, Europe), et jamais
// une préversion quand il y a mieux.
func bestCandidate(candidates []string, gameName string) string {
	wanted := strings.ToLower(gameName)
	score := func(name string) int {
		lower := strings.ToLower(name)
		value := 0
		for _, region := range []string{"(usa", "(world", "(europe", "(japan", "(france"} {
			if strings.Contains(lower, region) && strings.Contains(wanted, region) {
				value += 100
			}
		}
		switch {
		case strings.Contains(lower, "(usa"):
			value += 30
		case strings.Contains(lower, "(world"):
			value += 25
		case strings.Contains(lower, "(europe"):
			value += 20
		}
		for _, unwanted := range []string{"(beta", "(proto", "(sample", "(demo", "(kiosk", "(pirate", "(unl"} {
			if strings.Contains(lower, unwanted) {
				value -= 200
			}
		}

		return value - len(name)/10
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if score(candidate) > score(best) {
			best = candidate
		}
	}

	return best
}

// looseTitle — le titre sans ses mentions entre parenthèses ou crochets, sans
// ponctuation ni casse : « Banjo-Kazooie (USA) » et « Banjo Kazooie » donnent
// le même. Un article rejeté (« Saga, The ») est remis devant.
func looseTitle(name string) string {
	name = libretroMentions.ReplaceAllString(name, "")
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	for _, article := range []string{", the", ", a", ", an", ", le", ", la", ", les", ", l'"} {
		if strings.HasSuffix(lower, article) {
			lower = strings.TrimSpace(article[2:]) + " " + strings.TrimSuffix(lower, article)
			break
		}
	}
	var builder strings.Builder
	for _, character := range lower {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
		}
	}

	return builder.String()
}

var libretroMentions = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
