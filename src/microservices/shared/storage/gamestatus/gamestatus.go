// Package gamestatus — LE STATUT D'UN JEU (chantier 30) : jouable, plante en
// jeu, injouable, non testé.
//
// Le porteur, 2026-10-25 : « un attribut statut pour chaque jeu […] comme ça
// dans les listings des jeux on peut voir les jeux qui sont jouables ou non ».
// Puis, le 26 : « on va update les statuts pour faire passer à jouable pour
// tous » — une console entière d'un geste —, et la version de l'émulateur pour
// l'administrateur seul.
//
// LE DÉFAUT, sans réglage de l'administrateur (Default) : Jouable pour le
// rétro, Non testé pour les consoles récentes. Le porteur, 2026-10-26 : « c'est
// plus facile de tout mettre en playable et dès qu'un jeu ne joue pas on le
// fait passer à not playable. Les retrogaming ont moins de problèmes que les
// nouveaux émulateurs (éprouvés depuis des années par la communauté). »
//
// CE QUI SE RANGE. Un fichier par console, `status/<console>.json` dans le
// dossier du serveur : le statut de la console entière, et les SIGNALEMENTS
// de ses jeux, du plus ancien au plus récent. Le statut d'un jeu se CALCULE
// (Resolve) : il n'est écrit nulle part, pour qu'une console réglée d'un geste
// ne coûte qu'une ligne et qu'un jeu ajouté plus tard la suive.
//
// CE QUI VOYAGE. Le listing d'un serveur (chantier 15) porte les signalements
// de chaque jeu, SANS les versions (Public), et le statut de chaque console
// dans son en-tête : un ami calcule le même statut que nous.
package gamestatus

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gameflex/consoles"
)

// Les quatre statuts. Untested est aussi celui d'un jeu dont rien ne parle.
const (
	Untested   = "untested"
	Playable   = "playable"
	Crashes    = "crashes"
	Unplayable = "unplayable"
)

// rank — l'ordre du MEILLEUR, quand les émulateurs d'une console ne disent
// pas la même chose : « peut-on y jouer ? » — jouable d'abord, puis jouable
// jusqu'à un plantage, puis inconnu, puis non.
var rank = map[string]int{Playable: 4, Crashes: 3, Untested: 2, Unplayable: 1}

// Valid — un des quatre statuts.
func Valid(status string) bool {
	_, ok := rank[status]

	return ok
}

// MaxNote — une note est une précision (« plante au chapitre 2 »), pas un
// rapport : elle voyage avec le listing chez chaque ami.
const MaxNote = 500

// Report — UN SIGNALEMENT : ce qu'un administrateur a constaté.
type Report struct {
	Status string `json:"status"`
	// Emulators — les émulateurs qu'il concerne, par leur nom au registre
	// (« pcsx_rearmed », « Eden (streamex) »). Vide : tous ceux de la console.
	Emulators []string `json:"emulators,omitempty"`
	// Versions — la version de chacun au moment du signalement, prise par le
	// serveur. Pour l'historique de l'administrateur : elle ne voyage pas
	// (Public), et une nouvelle version ne change aucun statut.
	Versions map[string]string `json:"versions,omitempty"`
	Note     string            `json:"note,omitempty"`
	By       string            `json:"by"`
	At       time.Time         `json:"at"`
}

// Console — le fichier d'une console.
type Console struct {
	// Status — celui de la console entière ; vide : son Default.
	Status string     `json:"status,omitempty"`
	By     string     `json:"by,omitempty"`
	At     *time.Time `json:"at,omitempty"`
	// Games — l'identifiant d'un jeu (storage.GameID) → ses signalements.
	Games map[string][]Report `json:"games,omitempty"`
}

// Default — le statut d'une console qu'aucun administrateur n'a réglée :
// Jouable pour une console rétro (des cœurs libretro), Non testé pour les
// autres.
func Default(consoleID string) string {
	if consoles.MethodOf(consoleID) == consoles.MethodRetroGaming {
		return Playable
	}

	return Untested
}

// Setting — le réglage d'une console tel que l'administrateur le lit : le
// sien s'il en a fait un, et Default, ce qui vaut sans lui.
type Setting struct {
	Status  string     `json:"status,omitempty"`
	By      string     `json:"by,omitempty"`
	At      *time.Time `json:"at,omitempty"`
	Default string     `json:"default"`
}

// Setting — ce fichier, sans les signalements.
func (console Console) Setting(consoleID string) Setting {
	return Setting{Status: console.Status, By: console.By, At: console.At, Default: Default(consoleID)}
}

// Public — les signalements tels qu'ils voyagent : sans les versions.
func Public(reports []Report) []Report {
	if len(reports) == 0 {
		return nil
	}
	public := make([]Report, len(reports))
	for index, report := range reports {
		report.Versions = nil
		public[index] = report
	}

	return public
}

// ── Le calcul ──────────────────────────────────────────────────────────

// Note — une note d'un signalement, telle que la fiche la montre.
type Note struct {
	Status    string    `json:"status"`
	Emulators []string  `json:"emulators,omitempty"`
	Note      string    `json:"note"`
	By        string    `json:"by"`
	At        time.Time `json:"at"`
}

// View — ce que la page montre d'un jeu.
type View struct {
	// Status — le meilleur statut, celui du badge.
	Status string `json:"status"`
	// Emulator — l'émulateur qui le donne, quand ils ne disent pas tous la
	// même chose (« Jouable · Eden ») ; vide sinon.
	Emulator string `json:"emulator,omitempty"`
	// Emulators — le statut de chaque émulateur de la console, pour la fiche.
	Emulators map[string]string `json:"emulators,omitempty"`
	// Notes — les notes des signalements, de la plus récente à la plus
	// ancienne.
	Notes []Note `json:"notes,omitempty"`
}

// Resolve — le statut d'un jeu de cette console : ses signalements, le plus
// récent l'emportant pour chaque émulateur qu'il nomme (pour tous s'il n'en
// nomme aucun) ; ce qu'aucun signalement ne couvre suit la console.
func Resolve(consoleID, consoleStatus string, reports []Report) View {
	base := consoleStatus
	if !Valid(base) {
		base = Default(consoleID)
	}
	others := base
	named := map[string]string{}
	for _, report := range reports {
		if !Valid(report.Status) {
			continue
		}
		if len(report.Emulators) == 0 {
			others = report.Status
			for emulator := range named {
				named[emulator] = report.Status
			}

			continue
		}
		for _, emulator := range report.Emulators {
			named[emulator] = report.Status
		}
	}

	view := View{Status: others}
	cores := consoles.CoresFor(consoleID)
	if len(cores) > 0 || len(named) > 0 {
		view.Emulators = map[string]string{}
		for _, core := range cores {
			view.Emulators[core] = others
		}
		for emulator, status := range named {
			view.Emulators[emulator] = status
		}
	}

	// LE MEILLEUR, parmi ce qui s'applique vraiment : « les autres » ne
	// comptent que si un émulateur de la console reste sans signalement à
	// lui — sinon un jeu injouable partout se dirait « non testé ».
	var left []string
	for _, core := range cores {
		if _, ok := named[core]; !ok {
			left = append(left, core)
		}
	}
	uncovered := len(cores) == 0 || len(left) > 0
	best, from, differ := "", "", false
	consider := func(status, emulator string) {
		if best != "" && status != best {
			differ = true
		}
		if best == "" || rank[status] > rank[best] {
			best, from = status, emulator
		}
	}
	if uncovered {
		// Un seul émulateur sans signalement : c'est lui qu'on nomme.
		from := ""
		if len(left) == 1 {
			from = left[0]
		}
		consider(others, from)
	}
	for _, emulator := range sortedKeys(named) {
		consider(named[emulator], emulator)
	}
	if best != "" {
		view.Status = best
		if differ {
			view.Emulator = from
		}
	}

	for index := len(reports) - 1; index >= 0; index-- {
		report := reports[index]
		if strings.TrimSpace(report.Note) == "" {
			continue
		}
		view.Notes = append(view.Notes, Note{
			Status: report.Status, Emulators: report.Emulators, Note: report.Note, By: report.By, At: report.At,
		})
	}

	return view
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

// ── Le fichier ─────────────────────────────────────────────────────────

// mu — un signalement lit le fichier, ajoute, réécrit : deux en même temps
// ne doivent pas s'écraser.
var mu sync.Mutex

// Store — le dossier `status/` d'une bibliothèque.
type Store struct {
	Dir string
	// AfterWrite — appelé sur chaque fichier et dossier créé : le kernel, qui
	// tourne en root, les rend au propriétaire de la bibliothèque.
	AfterWrite func(path string)
}

// ErrInvalid — un statut inconnu, une console inconnue, un jeu sans nom.
var ErrInvalid = errors.New("gamestatus: signalement invalide")

func (s Store) path(consoleID string) (string, error) {
	if consoleID == "" || strings.ContainsAny(consoleID, `/\`) || strings.HasPrefix(consoleID, ".") {
		return "", fmt.Errorf("%w : console %q", ErrInvalid, consoleID)
	}

	return filepath.Join(s.Dir, consoleID+".json"), nil
}

// Load — le fichier d'une console ; vide quand il n'existe pas.
func (s Store) Load(consoleID string) (Console, error) {
	mu.Lock()
	defer mu.Unlock()

	return s.load(consoleID)
}

func (s Store) load(consoleID string) (Console, error) {
	target, err := s.path(consoleID)
	if err != nil {
		return Console{}, err
	}
	raw, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return Console{}, nil
	}
	if err != nil {
		return Console{}, err
	}
	var console Console
	if err := json.Unmarshal(raw, &console); err != nil {
		return Console{}, fmt.Errorf("gamestatus: %s illisible : %w", target, err)
	}

	return console, nil
}

// Add — un signalement de plus pour ce jeu.
func (s Store) Add(consoleID, gameID string, report Report) error {
	if gameID == "" || !Valid(report.Status) {
		return fmt.Errorf("%w : jeu %q, statut %q", ErrInvalid, gameID, report.Status)
	}
	report.Note = clip(strings.TrimSpace(report.Note), MaxNote)
	mu.Lock()
	defer mu.Unlock()
	console, err := s.load(consoleID)
	if err != nil {
		return err
	}
	if console.Games == nil {
		console.Games = map[string][]Report{}
	}
	console.Games[gameID] = append(console.Games[gameID], report)

	return s.save(consoleID, console)
}

// SetConsole — le statut de toute la console.
func (s Store) SetConsole(consoleID, status, by string, at time.Time) error {
	if !Valid(status) {
		return fmt.Errorf("%w : statut %q", ErrInvalid, status)
	}
	mu.Lock()
	defer mu.Unlock()
	console, err := s.load(consoleID)
	if err != nil {
		return err
	}
	stamp := at.UTC()
	console.Status, console.By, console.At = status, by, &stamp

	return s.save(consoleID, console)
}

// Statuses — le statut de chaque console qui en a un : ce que l'en-tête du
// listing publie.
func (s Store) Statuses() map[string]string {
	mu.Lock()
	defer mu.Unlock()
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil
	}
	statuses := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		consoleID := strings.TrimSuffix(name, ".json")
		console, err := s.load(consoleID)
		if err == nil && Valid(console.Status) {
			statuses[consoleID] = console.Status
		}
	}

	return statuses
}

// save — à côté puis renommé : un fichier à moitié écrit perdrait tous les
// signalements de la console.
func (s Store) save(consoleID string, console Console) error {
	target, err := s.path(consoleID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	if s.AfterWrite != nil {
		s.AfterWrite(s.Dir)
	}
	content, err := json.MarshalIndent(console, "", "  ")
	if err != nil {
		return err
	}
	partial := target + ".part"
	if err := os.WriteFile(partial, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(partial, target); err != nil {
		_ = os.Remove(partial)

		return err
	}
	if s.AfterWrite != nil {
		s.AfterWrite(target)
	}

	return nil
}

func clip(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}

	return string([]rune(text)[:limit])
}
