package retro_files

// The ROM catalog — an in-memory cache on top of scanner.go's raw disk
// scanning, giving every game a STABLE reference instead of the raw
// filename ScanConsole/ResolveRom used to hand out directly. A raw filename
// as the identifier used in URLs (ServeRom) and history was
// fragile (percent-encoding gymnastics for special characters — see
// urlParam's doc) and coupled those to the exact on-disk name forever.
//
// deriveRomID is DETERMINISTIC (UUID v5, hashed from consoleID + the entry's
// NAME ON DISK) rather than random: the same file always derives the same
// id across rescans, so a rescan (e.g. after adding new ROMs) doesn't
// invalidate existing history/session references — only an
// actual rename or delete does, exactly like the old raw-filename
// references would have.
//
// LE NOM SUR LE DISQUE, ET NON LE TITRE AFFICHÉ (2026-09-18). Le titre d'un
// fichier libre est son nom SANS extension : `Mario.nes` et `Mario.zip`
// tombaient donc sur le même identifiant et partageaient sauvegardes, favoris
// et historique. Un jeu-DOSSIER, lui, ne change pas — son titre EST le nom de
// son dossier, donc déjà son chemin.
//
// Unlike scanner.go's functions (still called fresh on every request, for
// ScanConsoles' folder counts, which don't need stable ids at all), this
// cache is rebuilt only when the disk says it must — the first time lazily
// on first use, then whenever one of the directories it was built from has
// changed (ensureCatalogScanned/isStale).
//
// IL N'Y A PAS DE ROUTE DE RESCAN dans le kernel, contrairement à ce que ce
// commentaire a longtemps dit : `handlers.go`'s `RescanRoms` appartenait à
// l'ancien service retrogaming. Sans rien pour l'invalider, le catalogue
// était donc scanné UNE fois par vie du processus — un jeu déposé après le
// démarrage n'apparaissait jamais dans la liste, alors que le compteur de la
// console (ScanConsoles, live) le voyait tout de suite. Deux réponses
// contradictoires à la même question, et une bibliothèque qui semblait
// perdre des jeux (constaté le 2026-09-10 avec le premier jeu PS5).
import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gameflex/consoles"
	"gameflex/storage"
)

// deriveRomID — `storage.RomID`, la dérivation partagée avec le téléphone
// (chantier 14) : une règle, un endroit. `entryName` est le nom SUR LE
// DISQUE — extension comprise pour un fichier libre —, pas le titre affiché.
func deriveRomID(consoleID, entryName string) string {
	return storage.RomID(consoleID, entryName)
}

// catalogEntry is the cached, per-game record — RomEntry (scanner.go) is the
// JSON shape ListRoms/ServeRom expose, derived from this one request at a
// time rather than stored directly, so the wire format can keep Reference
// as the field name (now holding ID, not Title) without this package's
// internals also being named that confusingly.
type catalogEntry struct {
	ID        string
	ConsoleID string
	Title     string // human display name — what Reference used to BE
	// Path — le nom de l'entrée SUR LE DISQUE : le nom du dossier pour un
	// jeu-dossier, le nom du fichier AVEC son extension pour un fichier
	// libre. C'est lui qui dérive l'identifiant, et non `Title` : deux
	// fichiers de même nom et d'extensions différentes sont deux jeux.
	Path      string
	Files     []string
	SizeBytes int64
	// IsDir — whether this entry is a FOLDER (Files lives inside
	// {consoleDir}/{Title}/) or a loose file directly inside {consoleDir}
	// (Files[0] IS the file, joined straight onto consoleDir). Set by
	// scanConsoleEntries, read by ResolveCatalogRom — len(Files) alone
	// can't distinguish these two cases: a folder containing exactly ONE
	// file (a common ROM layout — one .iso per named folder) produces the
	// exact same len(Files)==1 a genuinely loose file does, which used to
	// make ResolveCatalogRom join Files[0] onto consoleDir directly for
	// BOTH cases — wrong (and a real "file not found" bug) for the folder
	// one, since that file actually lives one directory deeper.
	IsDir bool
	// Engine — le moteur du jeu (storage.GameEngine, chantier 36).
	Engine string
}

type catalogState struct {
	mu        sync.RWMutex
	byID      map[string]catalogEntry
	byConsole map[string][]string // consoleID -> ids, in Title-sorted order
	// root — la racine que ce catalogue parcourt : la clé sous laquelle il est
	// rangé (catalogFor). Le dossier et chaque disque interne ont ainsi le
	// leur.
	root     string
	scanned  bool
	lastScan time.Time
	// stamps — la date de modification, au moment du scan, de CHAQUE dossier
	// qui décide du contenu du catalogue : la racine des roms (une console
	// apparaît), chaque dossier de console (un jeu apparaît), et chaque jeu
	// qui est lui-même un dossier (un fichier s'y ajoute). Comparer ces
	// dates coûte un `stat` par entrée — sans commune mesure avec le
	// parcours complet, et assez pour savoir s'il faut le refaire.
	//
	// CE QUE ÇA NE VOIT PAS : un fichier réécrit SUR PLACE, même nom et même
	// dossier. La date du dossier ne bouge pas, donc la taille en cache
	// reste celle d'avant jusqu'au prochain vrai changement. Assumé : le cas
	// qui compte est « j'ajoute un jeu », pas « je réécris une ROM sans la
	// renommer ».
	//
	// La carte n'est JAMAIS modifiée après publication (RescanCatalog en
	// construit une neuve), ce qui permet de la lire sous verrou puis de
	// faire les `stat` en dehors.
	stamps map[string]time.Time
	// rescan sérialise les reconstructions DE CETTE RACINE. Sans lui, la
	// première rafale de requêtes qui suit l'ajout d'un jeu lance autant de
	// parcours complets qu'il y a de requêtes — sur du stockage froid, ça se
	// sent.
	rescan sync.Mutex
}

// UN CATALOGUE PAR RACINE (chantier 23). La bibliothèque principale n'est
// plus seule : chaque disque interne (`/mnt/<nom>`) a ses jeux, rangés de la
// même façon et lus par le MÊME scanner — c'est lui que le moteur rétro
// refait au lancement, et un jeu listé par un autre scanner pourrait ne pas
// s'y retrouver. Chaque racine garde son propre état et ses propres dates :
// un jeu déposé sur un disque ne fait pas relire les autres.
var (
	catalogsMu sync.Mutex
	catalogs   = map[string]*catalogState{}
)

// catalogFor — l'état de la racine `root`, créé à la première demande.
func catalogFor(root string) *catalogState {
	catalogsMu.Lock()
	defer catalogsMu.Unlock()

	state, ok := catalogs[root]
	if !ok {
		state = &catalogState{
			byID:      make(map[string]catalogEntry),
			byConsole: make(map[string][]string),
			stamps:    make(map[string]time.Time),
			root:      root,
		}
		catalogs[root] = state
	}

	return state
}

// RescanCatalog rebuilds the entire in-memory catalog from disk — the only
// place a full directory walk happens (aside from ScanConsoles' own folder
// counts, which are unrelated and stay request-time). Called lazily on
// first use (ensureCatalogScanned) and explicitly by the admin rescan route.
func RescanCatalog(root string) error {
	return catalogFor(root).rebuild()
}

// rebuild — le parcours complet de la racine de ce catalogue.
func (catalog *catalogState) rebuild() error {
	root := catalog.root
	consoleSummaries, err := ScanConsoles(root)
	if err != nil {
		return err
	}

	byID := make(map[string]catalogEntry, len(catalog.byID))
	byConsole := make(map[string][]string, len(consoleSummaries))
	stamps := make(map[string]time.Time, len(consoleSummaries)+1)
	stampDir(stamps, root)

	for _, cs := range consoleSummaries {
		entries, err := scanConsoleEntries(root, cs.ID)
		if err != nil {
			continue
		}
		consoleDir, joinErr := safeJoin(root, cs.ID)
		if joinErr == nil {
			stampDir(stamps, consoleDir)
		}
		ids := make([]string, 0, len(entries))
		for _, e := range entries {
			e.ID = deriveRomID(cs.ID, e.Path)
			e.ConsoleID = cs.ID
			byID[e.ID] = e
			ids = append(ids, e.ID)
			// Un jeu-dossier a sa propre date : c'est elle qui bouge quand
			// un fichier s'ajoute DANS le jeu (la console, elle, ne voit
			// rien de ce qui se passe un cran plus bas).
			if e.IsDir && joinErr == nil {
				stampDir(stamps, filepath.Join(consoleDir, e.Title))
			}
		}
		byConsole[cs.ID] = ids
	}

	catalog.mu.Lock()
	catalog.byID = byID
	catalog.byConsole = byConsole
	catalog.stamps = stamps
	catalog.scanned = true
	catalog.lastScan = time.Now()
	catalog.mu.Unlock()
	return nil
}

// stampDir relève la date d'un dossier. Un dossier illisible ou absent est
// noté par la date zéro — sa disparition est elle aussi un changement, et
// doit déclencher un rescan plutôt que d'être silencieusement ignorée.
func stampDir(stamps map[string]time.Time, dir string) {
	info, err := os.Stat(dir)
	if err != nil {
		stamps[dir] = time.Time{}
		return
	}
	stamps[dir] = info.ModTime()
}

// isStale — vrai dès qu'un des dossiers relevés au dernier scan a
// changé de date, disparu, ou est réapparu.
func (catalog *catalogState) isStale() bool {
	catalog.mu.RLock()
	stamps := catalog.stamps
	catalog.mu.RUnlock()

	for dir, seen := range stamps {
		info, err := os.Stat(dir)
		if err != nil {
			if !seen.IsZero() {
				return true
			}
			continue
		}
		if !info.ModTime().Equal(seen) {
			return true
		}
	}
	return false
}

// ensureCatalogScanned scans lazily on whichever request happens to need the
// catalog first, puis À NOUVEAU dès que le disque a bougé (isStale) —
// c'est ce qui fait qu'un jeu déposé maintenant apparaît maintenant, sans
// bouton et sans redémarrage. Best-effort: a scan error just leaves the
// catalog empty (same observable behavior scanner.go's functions always had
// for a missing/unreadable root) rather than failing the triggering request.
func ensureCatalogScanned(root string) *catalogState {
	catalog := catalogFor(root)
	catalog.mu.RLock()
	scanned := catalog.scanned
	catalog.mu.RUnlock()
	if scanned && !catalog.isStale() {
		return catalog
	}

	catalog.rescan.Lock()
	defer catalog.rescan.Unlock()
	// Re-vérifier : pendant l'attente du verrou, un autre appel a pu faire
	// tout le travail — auquel cas il n'y a rien à refaire.
	catalog.mu.RLock()
	scanned = catalog.scanned
	catalog.mu.RUnlock()
	if scanned && !catalog.isStale() {
		return catalog
	}
	catalog.rebuild()

	return catalog
}

// ListCatalog returns the cached games for one console (rebuilding the
// cache first if this is the very first request since startup), optionally
// filtered by a case-insensitive title substring — same query semantics
// ScanConsole always had.
func ListCatalog(root, consoleID, query string) ([]RomEntry, error) {
	catalog := ensureCatalogScanned(root)

	query = strings.ToLower(strings.TrimSpace(query))
	catalog.mu.RLock()
	ids := catalog.byConsole[consoleID]
	matches := make([]catalogEntry, 0, len(ids))
	for _, id := range ids {
		e := catalog.byID[id]
		if query == "" || strings.Contains(strings.ToLower(e.Title), query) {
			matches = append(matches, e)
		}
	}
	catalog.mu.RUnlock()

	out := make([]RomEntry, len(matches))
	for i, e := range matches {
		out[i] = RomEntry{Reference: e.ID, Title: e.Title, Files: e.Files, SizeBytes: e.SizeBytes, Folder: e.IsDir, Engine: e.Engine}
	}
	return out, nil
}

// CatalogScannedAt — quand le catalogue de cette racine a été construit pour
// la dernière fois ; la date zéro s'il ne l'a jamais été. Il se reconstruit
// de lui-même dès qu'un de ses dossiers change : cette date avance donc avec
// le contenu, et dit à un ami que la liste a bougé (chantier 23).
func CatalogScannedAt(root string) time.Time {
	catalog := ensureCatalogScanned(root)
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()

	return catalog.lastScan
}

// CountCatalog — le nombre de jeux par console, lu dans le MÊME catalogue que
// la liste (#107).
//
// Le compteur des consoles relisait le disque à chaque affichage
// (`ScanConsoles`) et comptait tout fichier non caché ; la liste, elle, passe
// par ce catalogue, qui écarte ce qu'une console n'accepte pas (`Accepts`).
// Deux chemins, deux nombres : mesuré le 2026-09-28 sur la pile de dev, gbc
// annonçait 521 jeux et en listait 494. Lire le même catalogue supprime à la
// fois le parcours du disque et l'écart. Trié par identifiant, comme le
// rendait `ScanConsoles` (os.ReadDir trie par nom).
func CountCatalog(root string) []ConsoleSummary {
	catalog := ensureCatalogScanned(root)

	catalog.mu.RLock()
	summaries := make([]ConsoleSummary, 0, len(catalog.byConsole))
	for consoleID, ids := range catalog.byConsole {
		summaries = append(summaries, ConsoleSummary{ID: consoleID, Count: len(ids)})
	}
	catalog.mu.RUnlock()

	sort.Slice(summaries, func(left, right int) bool {
		return summaries[left].ID < summaries[right].ID
	})

	return summaries
}

// CatalogTitle — le titre d'un jeu du catalogue de cette racine, tel que la
// liste le montre. Le titre d'une partie rétro vient d'ici et plus de la
// requête de lancement : il s'affiche aux autres dans « Live now », et un
// compte y écrivait ce qu'il voulait (revue de sécurité du kernel, #601).
func CatalogTitle(root, consoleID, id string) (string, bool) {
	catalog := ensureCatalogScanned(root)

	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	e, ok := catalog.byID[id]
	if !ok || e.ConsoleID != consoleID {
		return "", false
	}

	return e.Title, true
}

// ResolveCatalogRom translates a catalog id back to its on-disk path — the
// cache-backed replacement for ResolveRom, used by ServeRom and
// resolveGameFile (stream_session.go). Directory case (multi-file game)
// mirrors ResolveRom's: Files holds every file inside consoleID/Title/.
func ResolveCatalogRom(root, consoleID, id string) (path string, isDir bool, err error) {
	catalog := ensureCatalogScanned(root)

	catalog.mu.RLock()
	e, ok := catalog.byID[id]
	catalog.mu.RUnlock()
	if !ok || e.ConsoleID != consoleID {
		return "", false, os.ErrNotExist
	}

	consoleDir, err := safeJoin(root, consoleID)
	if err != nil {
		return "", false, err
	}

	if e.IsDir {
		// Folder-based game (one file or several) — Title IS the folder
		// name (see scanConsoleEntries), Files live inside it.
		folderPath, err := safeJoin(root, consoleID, e.Title)
		if err != nil {
			return "", false, err
		}
		return folderPath, true, nil
	}
	if len(e.Files) == 1 {
		return filepath.Join(consoleDir, e.Files[0]), false, nil
	}
	return "", false, os.ErrNotExist
}

// scanConsoleEntries is scanner.go's ScanConsole logic, minus the query
// filter (applied later, against the cache, by ListCatalog) and returning
// catalogEntry instead of RomEntry (ID/ConsoleID filled in by the caller).
// Kept here rather than reusing ScanConsole directly since the id needs to
// be derived from the FULL unfiltered set, not whatever a caller's query
// happened to narrow it to.
func scanConsoleEntries(root, consoleID string) ([]catalogEntry, error) {
	consoleDir, err := safeJoin(root, consoleID)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(consoleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []catalogEntry{}, nil
		}
		return nil, err
	}

	out := make([]catalogEntry, 0, len(entries))
	for _, entry := range entries {
		if IsHidden(entry.Name()) {
			continue
		}
		if entry.IsDir() {
			rom, err := scanGameFolder(consoleID, consoleDir, entry.Name())
			if err != nil {
				continue
			}
			out = append(out, catalogEntry{Title: rom.Reference, Path: entry.Name(), Files: rom.Files, SizeBytes: rom.SizeBytes, IsDir: true, Engine: rom.Engine})
			continue
		}
		// UN FICHIER QUE LA CONSOLE N'ACCEPTE PAS N'EST PAS UN JEU.
		//
		// Ce filtre manquait, et le défaut était vivant (mesuré le 2026-09-18
		// sur la bibliothèque réelle) : 39 fichiers étaient listés comme des
		// jeux — 9 `.mp3`, 9 `.ogg`, 6 `.xml`, des `.sav`, `.sa1`, `.sqlite`,
		// `.torrent`, `.txt`. Pire, comme le titre est le nom SANS extension,
		// ils partageaient l'identifiant de la ROM voisine : sur `gbc`, 14
		// titres s'effondraient sur une seule référence, et celui qui gagnait
		// n'était pas la ROM. « Pokemon - Yellow Version » résolvait vers une
		// sauvegarde de 8 Ko, « Blade (U) [C][!] » vers un `.ogg` de 7 Ko —
		// donc injouables, sans qu'aucune erreur ne le dise.
		//
		// `consoles.Accepts` existait déjà et servait DÉJÀ aux deux autres
		// scanners (`shared/storage/scanner.go` pour les partages et le
		// téléphone, `kernel/internal/catalog` pour streamex) : seul celui-ci
		// ne l'appelait pas. Il laisse passer les archives, et accepte tout
		// d'une console qui ne déclare aucune extension — les consoles
		// streamex ne perdent donc rien.
		//
		// SEULS LES FICHIERS LIBRES SONT FILTRÉS. Un jeu-dossier n'a pas
		// d'extension (la branche ci-dessus), et un `.sav` RANGÉ DANS le
		// dossier d'un jeu en fait légitimement partie : `scanGameFolder`
		// continue de tout lister.
		//
		// LA COPIE DU RUNTIME DOIT RESTER IDENTIQUE
		// (`runtimes/retrogaming/.../catalog.go`) : c'est l'enfant qui
		// retrouve un jeu au lancement en rescannant, et deux scans qui ne
		// listent pas la même chose, c'est un jeu listé qui ne se lance pas.
		if !consoles.Accepts(consoleID, entry.Name()) {
			continue
		}
		title := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, catalogEntry{Title: title, Path: entry.Name(), Files: []string{entry.Name()}, SizeBytes: info.Size(), IsDir: false})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}

// AddFileToZip — streams one file into an open zip (the on-the-fly folder
// download, see api.handleRetroServeRom).
func AddFileToZip(zipWriter interface {
	Create(string) (io.Writer, error)
}, srcPath, nameInZip string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	destination, err := zipWriter.Create(nameInZip)
	if err != nil {
		return err
	}
	_, err = io.Copy(destination, file)
	return err
}
