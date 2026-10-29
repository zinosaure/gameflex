// Package listing garde SUR DISQUE ce qu'une source contient : le LISTING.
//
// Chantier 15 (décidé le 2026-10-13) : « chaque serveur doit avoir sa propre
// listing déjà dans le cache […] C'est ce fichier qui est partagé avec les
// autres serveurs quand on fait la fédération » (le porteur). Un listing par
// source, au même format pour toutes :
//
//   - CE SERVEUR (`self`) : ce qu'il possède — jeux, BIOS, cœurs, correctifs,
//     paramètres. Ses propres écrans le lisent, et la fédération le sert.
//   - UN AMI : la copie de SON listing, téléchargée à l'ajout et quand il a
//     changé.
//   - UNE RESSOURCE DISTANTE (Samba, WebDAV, kDrive) : elle n'a pas de listing
//     à elle ; on le fait en la parcourant (Build), jeux seulement.
//
// Le cache vit dans `cache/` du dossier du serveur, en JSON lisible :
//
//	cache/listing/<source>/listing.json            l'en-tête (Header)
//	cache/listing/<source>/<console>/<section>.json une section d'une console
//
// Un fichier par console plutôt qu'un gros : c'est ainsi qu'on le LIT —
// personne n'affiche vingt consoles à la fois, et charger la NES ne doit pas
// coûter le prix de la PlayStation. Sur le réseau, en revanche, le listing
// voyage en UN document (Listing, encodé en JSON).
package listing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gameflex/storage"
	"gameflex/storage/gamestatus"
)

// Les sections d'un listing, chacune rangée par console.
const (
	Roms    = "roms"
	Bios    = "bios"
	Cores   = "cores"
	Patches = "patches"
	Params  = "params"
)

// Sections — toutes, dans l'ordre où « Rescan all » les refait.
var Sections = []string{Roms, Bios, Cores, Patches, Params}

// IsSection — ce nom est-il une section d'un listing ?
func IsSection(name string) bool {
	for _, section := range Sections {
		if section == name {
			return true
		}
	}

	return false
}

// Item — un élément listé : le nommer, dire son poids, et savoir le reprendre.
// Une seule forme pour les cinq sections ; chacune n'en remplit que sa part.
type Item struct {
	// ID — l'UUID d'un JEU (storage.GameID), ÉCRIT dans le listing : c'est
	// par lui qu'on sait qu'un jeu d'ailleurs est déjà ici (chantier 14).
	ID string `json:"id,omitempty"`
	// Name — le titre d'un jeu ; le nom d'un BIOS, d'un cœur, d'un dossier de
	// correctifs ; l'émulateur, pour les paramètres.
	Name string `json:"name"`
	// Files — les fichiers à la racine d'un jeu, ou d'un dossier de
	// correctifs : l'extension d'un jeu qu'un autre rangera chez lui.
	Files []string `json:"files,omitempty"`
	// Dir — l'élément est un DOSSIER : il voyage zippé, et se déplie à
	// l'arrivée.
	Dir  bool  `json:"dir,omitempty"`
	Size int64 `json:"sizeBytes"`
	// Path, Folder — le chemin virtuel d'où reprendre l'élément, et le
	// dossier qui EST ce jeu : pour une RESSOURCE seulement, qu'on lit
	// fichier par fichier. Un ami et ce serveur servent par identifiant.
	Path   string `json:"path,omitempty"`
	Folder string `json:"folder,omitempty"`
	// Variants — pour un cœur : ses variantes présentes et leur poids
	// (« linux », « android » — celle que les téléphones chargent).
	Variants map[string]int64 `json:"variants,omitempty"`
	// Values, Presets — pour les paramètres d'un émulateur : des valeurs
	// choisies, et les préréglages PERSO (les livrés sont les mêmes partout).
	Values  bool     `json:"values,omitempty"`
	Presets []Preset `json:"presets,omitempty"`
	// Reports — pour un jeu : ses signalements (chantier 30), sans les
	// versions des émulateurs. Absent : rien n'a été signalé, le jeu suit sa
	// console (Header.Statuses).
	Reports []gamestatus.Report `json:"reports,omitempty"`
	// Engine — pour un jeu : son moteur (storage.GameEngine, chantier 36),
	// absent s'il n'est pas reconnu. Il voyage avec le jeu : un téléphone sait
	// ainsi, avant de lancer, qu'un jeu d'un ami ne peut pas tourner chez lui.
	Engine string `json:"engine,omitempty"`
}

// Preset — un préréglage perso, tel qu'un autre serveur le voit.
type Preset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Header — ce que le listing dit de lui-même.
type Header struct {
	Version int `json:"version"`
	// Source — la nature de ce qui est listé : SelfSource pour ce serveur,
	// PeerSource pour la copie d'un ami, le genre d'une ressource sinon
	// (« webdav », « kdrive », « smb »).
	Source string `json:"source"`
	// Name — le nom de la source : le serveur, ou la ressource.
	Name string `json:"name,omitempty"`
	// UpdatedAt — la dernière fois que quelque chose a changé, toutes
	// sections confondues : ce qu'un ami compare pour savoir s'il doit
	// re-télécharger.
	UpdatedAt time.Time              `json:"updatedAt"`
	Sections  map[string]SectionInfo `json:"sections"`
	// Server, Machine — qui sert ce listing, et sur quoi (chantier 15 : « pour
	// que les autres serveurs fédérés sachent »). Vides pour une ressource.
	Server  *Server  `json:"server,omitempty"`
	Machine *Machine `json:"machine,omitempty"`
	// Scopes — ce que NOTRE jeton chez cet ami nous accorde (« catalog »,
	// « devices ») : posé sur la COPIE, au téléchargement ; jamais publié par
	// celui qui sert son listing.
	Scopes []string `json:"scopes,omitempty"`
	// Statuses — le statut de chaque console qui en a un (chantier 30) : celui
	// d'un jeu sans signalement. Absent : non testé.
	Statuses map[string]string `json:"statuses,omitempty"`
}

// SectionInfo — une section : quand elle a été refaite, et ce qu'elle compte.
type SectionInfo struct {
	UpdatedAt time.Time      `json:"updatedAt"`
	Consoles  map[string]int `json:"consoles"`
}

// Server — le GameFlex qui sert un listing.
type Server struct {
	Hostname string `json:"hostname"`
	Version  string `json:"version,omitempty"`
	Commit   string `json:"commit,omitempty"`
}

// Les natures de source qui ne sont pas un genre de ressource.
const (
	SelfSource = "self"
	PeerSource = "gameflex"
)

// SelfKey — la clé du listing de CE serveur.
const SelfKey = "self"

// Listing — un listing entier : ce que Load rend, et ce qui voyage par la
// fédération.
type Listing struct {
	Header  Header            `json:"header"`
	Roms    map[string][]Item `json:"roms"`
	Bios    map[string][]Item `json:"bios"`
	Cores   map[string][]Item `json:"cores"`
	Patches map[string][]Item `json:"patches"`
	Params  map[string][]Item `json:"params"`
}

// New — un listing vide, de cette nature et de ce nom.
func New(source, name string) Listing {
	return Listing{
		Header:  Header{Version: version, Source: source, Name: name, Sections: map[string]SectionInfo{}},
		Roms:    map[string][]Item{},
		Bios:    map[string][]Item{},
		Cores:   map[string][]Item{},
		Patches: map[string][]Item{},
		Params:  map[string][]Item{},
	}
}

// Section — les éléments d'une section, par console ; nil pour un nom inconnu.
func (l *Listing) Section(name string) map[string][]Item {
	switch name {
	case Roms:
		return l.Roms
	case Bios:
		return l.Bios
	case Cores:
		return l.Cores
	case Patches:
		return l.Patches
	case Params:
		return l.Params
	}

	return nil
}

// SetSection — remplace une section, et la date dans l'en-tête.
func (l *Listing) SetSection(name string, items map[string][]Item, at time.Time) {
	if items == nil {
		items = map[string][]Item{}
	}
	switch name {
	case Roms:
		l.Roms = items
	case Bios:
		l.Bios = items
	case Cores:
		l.Cores = items
	case Patches:
		l.Patches = items
	case Params:
		l.Params = items
	default:
		return
	}
	if l.Header.Sections == nil {
		l.Header.Sections = map[string]SectionInfo{}
	}
	l.Header.Sections[name] = SectionInfo{UpdatedAt: at.UTC(), Consoles: countsOf(items)}
	if at.After(l.Header.UpdatedAt) {
		l.Header.UpdatedAt = at.UTC()
	}
}

// GameItem — un jeu tel que le listing le porte, avec son UUID. `withPath`
// garde son chemin virtuel : une ressource se relit fichier par fichier.
func GameItem(game storage.Game, withPath bool) Item {
	files := game.Files
	if files == nil {
		files = []string{}
	}
	item := Item{
		ID:     storage.GameID(game),
		Name:   game.Title,
		Files:  files,
		Dir:    game.Folder != "",
		Size:   game.Size,
		Engine: game.Engine,
	}
	if withPath {
		item.Path, item.Folder = game.Path, game.Folder
	}

	return item
}

// Build parcourt une RESSOURCE distante et rend son listing : ses jeux, et
// rien d'autre — une ressource ne fournit ni cœurs ni BIOS, seul un serveur
// fédéré le peut (#153). Selon la convention (`roms/<consoleid>/…`, exigée
// par le porteur le 2026-10-13) : un partage rangé autrement ne montre rien.
func Build(source storage.Source, at time.Time) (Listing, error) {
	games, err := storage.ScanConvention(source, Roms)
	if err != nil {
		return Listing{}, err
	}

	built := New(storage.KindOf(source), source.Name())
	roms := map[string][]Item{}
	for _, game := range games {
		roms[game.Console] = append(roms[game.Console], GameItem(game, true))
	}
	built.SetSection(Roms, roms, at)
	for _, section := range Sections {
		if section != Roms {
			built.SetSection(section, nil, at)
		}
	}

	return built, nil
}

func countsOf(items map[string][]Item) map[string]int {
	counts := map[string]int{}
	for console, list := range items {
		if len(list) > 0 {
			counts[console] = len(list)
		}
	}

	return counts
}

// sortItems — par nom, sans tenir compte de la casse : l'ordre d'affichage,
// et un fichier qui ne change que si son contenu change.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
}

// ── L'identité d'une source ─────────────────────────────────────────────

// Key — la clé de cache d'une ressource ou d'un ami, dérivée de CE QU'ILS
// SONT.
//
// Le nom ne peut pas servir : il est choisi par son propriétaire, qui peut le
// réutiliser. Supprimer « mydrive » (le drive 133300) puis rajouter un autre
// « mydrive » (le drive 244733) faisait hériter le second du cache du
// premier — des jeux qui n'existent pas, annoncés comme présents (signalé le
// 2026-09-02). Dérivée plutôt que tirée au sort : la MÊME source retrouve sa
// clé, une AUTRE ne peut pas la prendre ; et le listing d'une source qu'on
// retire part avec elle (Remove).
//
// Renommer une ressource lui laisse son listing, ce qui est le bon
// comportement : on a changé son étiquette, pas son contenu.
func Key(kind, address, folder string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(kind)) + "|" +
		strings.TrimRight(strings.TrimSpace(address), "/") + "|" +
		strings.Trim(strings.TrimSpace(folder), "/")))

	// Seize octets suffisent largement à ne jamais se croiser, et gardent un
	// nom de dossier lisible dans un cache qu'on doit pouvoir inspecter.
	return hex.EncodeToString(sum[:16])
}

// ── Le cache sur disque ─────────────────────────────────────────────────

// ErrNoCache — rien d'UTILISABLE n'a été écrit pour cette source. Ce n'est pas
// une panne : c'est la première fois, ou le cache est d'une version
// antérieure.
var ErrNoCache = errors.New("listing: aucun listing pour cette source")

// version — la FORME de ce qu'on écrit ici.
//
// À incrémenter dès qu'un champ apparaît. Sans elle, un cache écrit par une
// version antérieure se relit sans erreur, avec les nouveaux champs à leur
// valeur nulle — et le code d'aujourd'hui prend ce vide pour une réponse.
// C'est exactement ce qui est arrivé à `folder` : tout jeu déjà en cache se
// déclarait « fichier seul », donc un PS1 partait sans ses .bin (2026-09-02).
// Un cache périmé n'est pas un problème : il se reconstruit.
//
// 3 — le dossier fait foi : un dossier sous une console est UN jeu (2026-09-02).
// 4 — le nom d'un dossier garde ses points (2026-09-02).
// 5 — le listing d'un serveur (chantier 15, 2026-10-13) : un en-tête (dates
//
//	par section, serveur, machine), cinq sections, l'UUID d'un jeu écrit.
//
// PAS DE 6 POUR LES STATUTS DES JEUX (chantier 30, 2026-10-26) : `reports` et
// `statuses` sont absents d'un listing qui n'en a pas, et leur absence dit
// vrai — rien n'a été signalé, tout est non testé. Un cache d'avant se relit
// donc juste, et un ami d'une version antérieure aussi. Incrémenter aurait
// jeté pour rien chaque listing de ressource (un kDrive se reparcourt en
// plusieurs minutes).
const version = 5

const headerFile = "listing.json"

// locks — une écriture à la fois par source : deux sections refaites en même
// temps (un cœur téléchargé pendant qu'un jeu s'importe) réécriraient chacune
// l'en-tête de l'autre. Et une lecture attend la fin d'une écriture : Save
// repart de zéro, SaveSection retire les fichiers d'une section avant de les
// réécrire — lue entre les deux, une console qui a des jeux en montrait zéro.
var locks sync.Map

func lockOf(base string) *sync.RWMutex {
	lock, _ := locks.LoadOrStore(base, &sync.RWMutex{})

	return lock.(*sync.RWMutex)
}

// reading — le verrou d'une lecture : plusieurs à la fois, aucune pendant une
// écriture. Rend de quoi le relâcher.
func reading(base string) func() {
	lock := lockOf(base)
	lock.RLock()

	return lock.RUnlock
}

// Save écrit un listing entier, en repartant de zéro : une console dont les
// éléments ont disparu doit voir son fichier disparaître aussi.
func Save(cacheDir, key string, l Listing) error {
	base := filepath.Join(cacheDir, safeName(key))
	lock := lockOf(base)
	lock.Lock()
	defer lock.Unlock()

	if err := os.RemoveAll(base); err != nil {
		return err
	}
	l.Header.Version = version
	if l.Header.Sections == nil {
		l.Header.Sections = map[string]SectionInfo{}
	}
	for _, section := range Sections {
		items := l.Section(section)
		info := l.Header.Sections[section]
		info.Consoles = countsOf(items)
		l.Header.Sections[section] = info
		if err := writeSection(base, section, items); err != nil {
			return err
		}
	}

	return writeJSON(filepath.Join(base, headerFile), l.Header)
}

// SaveSection remplace UNE section d'un listing déjà écrit, et sa date —
// ce que fait l'icône à côté d'un sélecteur de source, et ce que GameFlex fait
// après ses propres écritures. ErrNoCache sans listing : il faut d'abord
// l'écrire en entier.
func SaveSection(cacheDir, key, section string, items map[string][]Item, at time.Time) error {
	if !IsSection(section) {
		return fmt.Errorf("listing: section inconnue %q", section)
	}
	base := filepath.Join(cacheDir, safeName(key))
	lock := lockOf(base)
	lock.Lock()
	defer lock.Unlock()

	header, err := readHeader(base)
	if err != nil {
		return err
	}
	for console := range header.Sections[section].Consoles {
		_ = os.Remove(filepath.Join(base, safeName(console), section+".json"))
	}
	if err := writeSection(base, section, items); err != nil {
		return err
	}
	if header.Sections == nil {
		header.Sections = map[string]SectionInfo{}
	}
	header.Sections[section] = SectionInfo{UpdatedAt: at.UTC(), Consoles: countsOf(items)}
	if at.After(header.UpdatedAt) {
		header.UpdatedAt = at.UTC()
	}

	return writeJSON(filepath.Join(base, headerFile), header)
}

// SaveConsoleSection remplace la section d'UNE console : après un signalement,
// seule sa console a changé, et réécrire les vingt autres coûterait des
// mégaoctets pour rien. La date avance, celle de la section comme celle du
// listing : c'est elle qu'un ami compare.
func SaveConsoleSection(cacheDir, key, section, consoleID string, items []Item, at time.Time) error {
	if !IsSection(section) {
		return fmt.Errorf("listing: section inconnue %q", section)
	}
	base := filepath.Join(cacheDir, safeName(key))
	lock := lockOf(base)
	lock.Lock()
	defer lock.Unlock()

	header, err := readHeader(base)
	if err != nil {
		return err
	}
	target := filepath.Join(base, safeName(consoleID), section+".json")
	if len(items) == 0 {
		_ = os.Remove(target)
	} else {
		sortItems(items)
		if err := writeJSON(target, items); err != nil {
			return err
		}
	}
	if header.Sections == nil {
		header.Sections = map[string]SectionInfo{}
	}
	info := header.Sections[section]
	if info.Consoles == nil {
		info.Consoles = map[string]int{}
	}
	if len(items) == 0 {
		delete(info.Consoles, consoleID)
	} else {
		info.Consoles[consoleID] = len(items)
	}
	info.UpdatedAt = at.UTC()
	header.Sections[section] = info
	if at.After(header.UpdatedAt) {
		header.UpdatedAt = at.UTC()
	}

	return writeJSON(filepath.Join(base, headerFile), header)
}

// SaveHeader réécrit l'en-tête d'un listing déjà écrit (le serveur, la
// machine, les portées d'un jeton), sans toucher aux sections.
func SaveHeader(cacheDir, key string, change func(*Header)) error {
	base := filepath.Join(cacheDir, safeName(key))
	lock := lockOf(base)
	lock.Lock()
	defer lock.Unlock()

	header, err := readHeader(base)
	if err != nil {
		return err
	}
	change(&header)
	header.Version = version

	return writeJSON(filepath.Join(base, headerFile), header)
}

// SaveServer — qui sert ce listing, mis à jour au démarrage : son nom, sa
// version de GameFlex, sa machine. La date n'avance que si l'un d'eux a
// changé — c'est elle qu'un ami compare, et une nouvelle version doit se voir
// chez lui sans qu'aucun jeu n'ait bougé.
func SaveServer(cacheDir, key, name string, server *Server, machine *Machine, at time.Time) error {
	return SaveHeader(cacheDir, key, func(header *Header) {
		before, _ := json.Marshal([]any{header.Name, header.Server, header.Machine})
		header.Name, header.Server, header.Machine = name, server, machine
		after, _ := json.Marshal([]any{header.Name, header.Server, header.Machine})
		if string(before) != string(after) && at.After(header.UpdatedAt) {
			header.UpdatedAt = at.UTC()
		}
	})
}

func writeSection(base, section string, items map[string][]Item) error {
	for console, list := range items {
		if len(list) == 0 {
			continue
		}
		sortItems(list)
		if err := writeJSON(filepath.Join(base, safeName(console), section+".json"), list); err != nil {
			return err
		}
	}

	return nil
}

// Remove — le listing d'une source qu'on retire part avec elle : une donnée
// résiduelle mentirait au prochain qui la lirait.
func Remove(cacheDir, key string) error {
	base := filepath.Join(cacheDir, safeName(key))
	lock := lockOf(base)
	lock.Lock()
	defer lock.Unlock()

	return os.RemoveAll(base)
}

// LoadHeader relit l'en-tête seul : la date, le serveur, la machine.
func LoadHeader(cacheDir, key string) (Header, error) {
	base := filepath.Join(cacheDir, safeName(key))
	defer reading(base)()

	return readHeader(base)
}

func readHeader(base string) (Header, error) {
	var header Header
	if err := readJSON(filepath.Join(base, headerFile), &header); err != nil || header.Version != version {
		// Écrit par une autre version, ou jamais : on ne le lit pas. Il vaut
		// mieux reparcourir que répondre avec des champs vides.
		return Header{}, ErrNoCache
	}

	return header, nil
}

// PruneGrace — un listing écrit depuis moins longtemps n'est jamais retiré : la
// liste de sources qu'on compare a pu être lue juste avant l'ajout de la
// sienne, dont le listing vient d'arriver.
const PruneGrace = time.Minute

// Prune retire les listings dont la source n'est plus : parmi ceux que `owned`
// reconnaît (les copies d'amis, les ressources), ceux que `keep` ne garde pas —
// une source retirée emporte le sien, une donnée résiduelle mentirait au
// prochain qui la lirait. Un listing ILLISIBLE par cette version (un format
// d'avant) part aussi, quel qu'il soit : il ne servirait plus, et sa source,
// si elle existe encore, le refera. Rend les clés retirées.
func Prune(cacheDir string, keep map[string]bool, owned func(Header) bool) []string {
	removed := []string{}
	for _, key := range Keys(cacheDir) {
		if keep[key] {
			continue
		}
		header, err := LoadHeader(cacheDir, key)
		switch {
		case err == nil:
			if !owned(header) {
				continue
			}
			if written, err := WrittenAt(cacheDir, key); err != nil || time.Since(written) < PruneGrace {
				continue
			}
		default:
			// Illisible : un format d'avant, ou un premier listing en cours
			// d'écriture — que son dossier, tout neuf, protège.
			info, err := os.Stat(filepath.Join(cacheDir, safeName(key)))
			if err != nil || time.Since(info.ModTime()) < PruneGrace {
				continue
			}
		}
		if Remove(cacheDir, key) == nil {
			removed = append(removed, key)
		}
	}

	return removed
}

// WrittenAt — quand ce listing a été écrit ICI pour la dernière fois, et non
// la date qu'il porte : ce qui distingue une copie qu'on vient de prendre.
func WrittenAt(cacheDir, key string) (time.Time, error) {
	info, err := os.Stat(filepath.Join(cacheDir, safeName(key), headerFile))
	if err != nil {
		return time.Time{}, ErrNoCache
	}

	return info.ModTime(), nil
}

// Scanned dit si cette source a déjà un listing lisible par cette version.
// Faux veut dire : le premier parcours reste à faire.
func Scanned(cacheDir, key string) bool {
	_, err := LoadHeader(cacheDir, key)

	return err == nil
}

// Load relit un listing entier — ce qu'on sert à un ami.
func Load(cacheDir, key string) (Listing, error) {
	base := filepath.Join(cacheDir, safeName(key))
	defer reading(base)()
	header, err := readHeader(base)
	if err != nil {
		return Listing{}, err
	}

	loaded := New(header.Source, header.Name)
	loaded.Header = header
	for _, section := range Sections {
		items := loaded.Section(section)
		for console := range header.Sections[section].Consoles {
			var list []Item
			if readJSON(filepath.Join(base, safeName(console), section+".json"), &list) == nil && len(list) > 0 {
				items[console] = list
			}
		}
	}

	return loaded, nil
}

// LoadSection relit UNE section d'UNE console, sans toucher au reste — ce que
// l'interface demande quand elle affiche une console. Une console sans
// élément rend une liste vide, pas une erreur.
func LoadSection(cacheDir, key, consoleID, section string) ([]Item, error) {
	base := filepath.Join(cacheDir, safeName(key))
	defer reading(base)()
	header, err := readHeader(base)
	if err != nil {
		return nil, err
	}
	if header.Sections[section].Consoles[consoleID] == 0 {
		return []Item{}, nil
	}

	var items []Item
	if err := readJSON(filepath.Join(base, safeName(consoleID), section+".json"), &items); err != nil {
		return nil, ErrNoCache
	}

	return items, nil
}

// LoadSectionAll relit une section, toutes consoles : l'écran des cœurs ou
// des BIOS les montre ensemble.
func LoadSectionAll(cacheDir, key, section string) (map[string][]Item, error) {
	base := filepath.Join(cacheDir, safeName(key))
	defer reading(base)()
	header, err := readHeader(base)
	if err != nil {
		return nil, err
	}

	items := map[string][]Item{}
	for console := range header.Sections[section].Consoles {
		var list []Item
		if readJSON(filepath.Join(base, safeName(console), section+".json"), &list) == nil && len(list) > 0 {
			items[console] = list
		}
	}

	return items, nil
}

// Keys — les sources qui ont un listing dans ce cache : de quoi retirer
// celui d'une ressource qui n'existe plus.
func Keys(cacheDir string) []string {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil
	}

	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			keys = append(keys, entry.Name())
		}
	}

	return keys
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}

	partial := path + ".part"
	if err := os.WriteFile(partial, encoded, 0o644); err != nil {
		return err
	}
	if err := os.Rename(partial, path); err != nil {
		_ = os.Remove(partial)

		return fmt.Errorf("listing: %s: %w", filepath.Base(path), err)
	}

	return nil
}

func readJSON(path string, into any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, into)
}

// safeName — un identifiant utilisable comme nom de dossier. Une ressource
// s'appelle comme son propriétaire l'a nommée, et « mon NAS/perso » ne doit
// pas creuser une arborescence. La console vide (un BIOS à la racine de bios/)
// devient « _ ».
func safeName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 32 {
			return '_'
		}

		return r
	}, strings.TrimSpace(name))

	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "_"
	}

	return cleaned
}

// ── Ce que la source a appris de son arborescence ───────────────────────
//
// Rangé À CÔTÉ du listing, et pour la même raison : le parcours l'a déjà payé.
// Voir storage.Memo — sans ce fichier, le premier téléchargement de chaque
// console après un redémarrage repayait la marche chemin→numéro entière (une
// demi-minute sur 10 785 jeux NES, mesuré le 2026-09-02).
//
// Ce n'est PAS le listing : il n'a pas de version, et sa perte ne coûte qu'un
// aller-retour. Un identifiant qui aurait changé chez l'hébergeur mène
// simplement à une ouverture qui échoue, et la source relistera.

const memoFile = "refs.json"

// SaveMemo écrit ce qu'une source a appris, si elle sait le dire.
func SaveMemo(cacheDir, key string, source any) error {
	memo, ok := source.(storage.Memo)
	if !ok {
		return nil // une source qui adresse par chemin n'a rien à retenir
	}
	learned := memo.ExportMemo()
	if len(learned) == 0 {
		return nil
	}

	return writeJSON(filepath.Join(cacheDir, safeName(key), memoFile), learned)
}

// LoadMemo rend à une source ce qu'elle avait appris. Silencieux : ne rien
// retrouver est le cas normal d'une ressource jamais parcourue.
func LoadMemo(cacheDir, key string, source any) {
	memo, ok := source.(storage.Memo)
	if !ok {
		return
	}

	var learned map[string]string
	if readJSON(filepath.Join(cacheDir, safeName(key), memoFile), &learned) != nil {
		return
	}
	memo.ImportMemo(learned)
}
