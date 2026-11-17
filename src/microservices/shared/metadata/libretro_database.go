package metadata

// LA SOURCE `libretro-database` (chantier 49, P3) — le texte de la fiche d'un
// jeu rétro, sans compte ni clé : développeurs, éditeurs, genres et date de
// sortie, lus dans les `.dat` que le dépôt libretro-database publie sous
// `metadat/` (CC BY-SA 4.0 : la fiche le cite). Relevé le 2026-11-12 :
//
//   - un `.dat` par champ et par système (`developer/`, `publisher/`,
//     `genre/`, `releaseyear/`, `franchise/` pour la série, `maxusers/` pour
//     le nombre de joueurs), une cinquantaine
//     de systèmes à cartouches de
//     l'Atari 2600 à la GBA, la N64 et la Mega Drive, rangés par CRC de la
//     ROM ; quelques-uns par numéro de série (PSP, Dreamcast) ;
//   - un `.dat` complet pour la PS1, la PS2 et la PSP (`metadat/<système>.dat`,
//     tiré de psxdatacenter), par numéro de série, avec le jour de sortie ;
//   - rien pour la GameCube, la Wii, la Saturn, la Neo Geo, ni les consoles
//     récentes : leurs jeux n'ont ici que leur nom.
//
// LE CRC EST CELUI DE NO-INTRO, en-tête compris pour la NES. Un dump GoodNES
// porte un autre en-tête iNES : sur 200 NES de la dev, aucune empreinte ne se
// retrouve ; c'est leur nom qui les reconnaît. Les `.zip` de SNES, eux, se
// retrouvent à 214 sur 216.
//
// Téléchargés au premier besoin d'un système, gardés dans
// `cache/metadata/sources/libretro-database/` sous les mêmes chemins que dans
// le dépôt, et refaits après 30 jours ; un fichier que le dépôt n'a pas est
// gardé vide, et n'est pas redemandé. Lus en flux, sans garder que ce qui
// sert : le `.dat` de la PS1 pèse 6,7 Mo, presque tout en résumés.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"gameflex/storage"
)

// libretroDatabaseBase — le dossier `metadat/` du dépôt ; remplaçable dans un
// test.
var libretroDatabaseBase = "https://raw.githubusercontent.com/libretro/libretro-database/master/metadat"

// libretroDatabaseFiles — les `.dat` lus pour un système : le complet (vide :
// `metadat/<système>.dat`), puis un par champ. `releasemonth/` n'est pas lu :
// sans le jour, la date reste une année. `maxusers/` donne le nombre de
// joueurs d'un jeu à cartouche (sa clé `users`) ; le `.dat` complet d'une
// PlayStation le porte déjà (chantier 53, P12 : « si on peut avoir l'info,
// rajouter le nombre de joueurs possible »).
var libretroDatabaseFiles = []string{"", "developer", "publisher", "genre", "releaseyear", "franchise", "maxusers"}

const (
	libretroDatabaseAge   = 30 * 24 * time.Hour
	libretroDatabaseLimit = 64 << 20
)

// libretroDatabaseSlots — deux téléchargements à la fois au plus, pour tout le
// serveur : le premier passage sur une console en demande six.
var libretroDatabaseSlots = make(chan struct{}, 2)

// databaseEntry — ce que la base sait d'un jeu.
type databaseEntry struct {
	developers, publishers, genres []string
	year, month, day               string
	franchise                      string
	users                          string // le nombre de joueurs, « 2 »
}

// databaseIndex — les jeux d'un système, par empreinte, numéro de série et
// nom ; `names` sert la recherche par nom approché (`match`).
type databaseIndex struct {
	byCRC    map[uint32]*databaseEntry
	bySerial map[string]*databaseEntry
	byName   map[string]*databaseEntry
	names    *libretroNames

	// franchises — la série déjà trouvée pour un nom de la liste des jeux
	// (cachedFranchise) : l'index ne change plus une fois lu, et une liste de
	// 10 783 NES ne refait pas 10 783 recherches approchées à chaque affichage.
	franchisesMutex sync.Mutex
	franchises      map[string]string
}

type loadedDatabase struct {
	index    *databaseIndex
	loadedAt time.Time
	complete bool
}

var (
	databasesMutex sync.Mutex
	databases      = map[string]loadedDatabase{}
)

// libretroDatabaseText — le texte de la fiche du jeu, pris dans les `.dat` des
// systèmes de la console : par l'empreinte de `gameKey` (« crc-1A2B3C4D »),
// sinon par un numéro de série lu dans un nom (« [SCES-02066] »), sinon par le
// nom exact, puis approché. os.ErrNotExist si la base ne le connaît pas.
func libretroDatabaseText(ctx context.Context, cacheRoot, consoleID, gameKey string, gameNames []string) (gameText, error) {
	systems := libretroSystems[consoleID]
	if len(systems) == 0 {
		return gameText{}, os.ErrNotExist
	}

	crc, hasCRC := uint32(0), false
	if hexadecimal, found := strings.CutPrefix(gameKey, "crc-"); found {
		if value, err := strconv.ParseUint(hexadecimal, 16, 32); err == nil {
			crc, hasCRC = uint32(value), true
		}
	}
	serials := []string{}
	for _, name := range gameNames {
		serials = append(serials, storage.SerialsIn(name)...)
	}

	// Les identifiants d'abord, sur tous les systèmes de la console : un nom
	// approché de la NES ne doit pas passer devant l'empreinte exacte d'un
	// jeu du Famicom Disk System.
	indexes := []*databaseIndex{}
	var loadErr error
	for _, system := range systems {
		index, err := libretroDatabase(ctx, cacheRoot, system)
		if err != nil {
			loadErr = err
			continue
		}
		indexes = append(indexes, index)
		if entry := index.byIdentifier(crc, hasCRC, serials); entry != nil {
			return entry.text(), nil
		}
	}
	for _, index := range indexes {
		for _, name := range gameNames {
			if entry := index.byGameName(name); entry != nil {
				return entry.text(), nil
			}
		}
	}
	if len(indexes) == 0 && loadErr != nil {
		return gameText{}, fmt.Errorf("libretro-database: %w", loadErr)
	}

	return gameText{}, os.ErrNotExist
}

// serialKey — un numéro de série sans sa ponctuation : « SLES-12966 » et
// « SLES12966 » se retrouvent.
func serialKey(serial string) string {
	var builder strings.Builder
	for _, character := range strings.ToUpper(serial) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
		}
	}

	return builder.String()
}

func (index *databaseIndex) byIdentifier(crc uint32, hasCRC bool, serials []string) *databaseEntry {
	if hasCRC {
		if entry, found := index.byCRC[crc]; found {
			return entry
		}
	}
	for _, serial := range serials {
		if entry, found := index.bySerial[serial]; found {
			return entry
		}
	}

	return nil
}

func (index *databaseIndex) byGameName(gameName string) *databaseEntry {
	name, _ := index.names.match(gameName)
	if name == "" {
		return nil
	}

	return index.byName[name]
}

// text — l'entrée telle que la fiche la montre. La date : le jour quand la
// base le donne (les PlayStation), sinon l'année seule.
func (entry *databaseEntry) text() gameText {
	text := gameText{developers: entry.developers, publishers: entry.publishers, genres: entry.genres, series: entry.franchise}
	if players, err := strconv.Atoi(entry.users); err == nil && players > 0 {
		text.players = players
	}
	year, yearErr := strconv.Atoi(entry.year)
	month, monthErr := strconv.Atoi(entry.month)
	day, dayErr := strconv.Atoi(entry.day)
	switch {
	case yearErr != nil || len(entry.year) != 4:
	case monthErr == nil && dayErr == nil && month >= 1 && month <= 12 && day >= 1 && day <= 31:
		text.releaseDate = fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	default:
		text.releaseDate = entry.year
	}

	return text
}

// cachedFranchise — la série d'un jeu de la liste, par le numéro de série de
// son nom, puis son nom exact, puis approché ; "" si l'index ne la connaît
// pas. Gardée par nom.
func (index *databaseIndex) cachedFranchise(gameName string) string {
	index.franchisesMutex.Lock()
	defer index.franchisesMutex.Unlock()
	if franchise, found := index.franchises[gameName]; found {
		return franchise
	}

	franchise := ""
	if entry := index.byIdentifier(0, false, storage.SerialsIn(gameName)); entry != nil {
		franchise = entry.franchise
	} else if entry := index.byGameName(gameName); entry != nil {
		franchise = entry.franchise
	}
	if index.franchises == nil {
		index.franchises = map[string]string{}
	}
	index.franchises[gameName] = franchise

	return franchise
}

// loadedDatabases — les index déjà en mémoire des systèmes d'une console,
// sans rien lire ni télécharger.
func loadedDatabases(consoleID string) []*databaseIndex {
	databasesMutex.Lock()
	defer databasesMutex.Unlock()
	indexes := []*databaseIndex{}
	for _, system := range libretroSystems[consoleID] {
		if loaded, found := databases[system]; found {
			indexes = append(indexes, loaded.index)
		}
	}

	return indexes
}

// libretroDatabase — l'index d'un système : en mémoire, sinon lu dans les
// fichiers gardés, téléchargés s'ils manquent ou ont 30 jours. Un système dont
// un fichier n'a pas pu venir est relu au bout d'une heure.
func libretroDatabase(ctx context.Context, cacheRoot, system string) (*databaseIndex, error) {
	lock := lockFor(filepath.Join(cacheRoot, "libretro-database", system))
	lock.Lock()
	defer lock.Unlock()

	databasesMutex.Lock()
	loaded, found := databases[system]
	databasesMutex.Unlock()
	if found {
		age := timeNow().Sub(loaded.loadedAt)
		if age < libretroDatabaseAge && (loaded.complete || age < libretroErrorAge) {
			return loaded.index, nil
		}
	}

	index := &databaseIndex{byCRC: map[uint32]*databaseEntry{}, bySerial: map[string]*databaseEntry{}, byName: map[string]*databaseEntry{}}
	complete, read := true, 0
	var lastErr error
	for _, field := range libretroDatabaseFiles {
		label := field
		if label == "" {
			label = "full"
		}
		path, err := libretroDatabaseFile(ctx, cacheRoot, field, system)
		if err != nil {
			complete, lastErr = false, err
			slog.Warn("libretro-database", "system", system, "file", label, "error", err)
		}
		if path == "" {
			continue
		}
		if err := index.read(path); err != nil {
			complete, lastErr = false, err
			slog.Warn("libretro-database", "system", system, "file", label, "error", err)
			continue
		}
		read++
	}
	if read == 0 && lastErr != nil {
		return nil, lastErr
	}

	names := make([]string, 0, len(index.byName))
	for name := range index.byName {
		names = append(names, name)
	}
	index.names = newLibretroNames(names)
	// Une requête abandonnée par la page ne doit pas priver tout le serveur
	// de la base pendant une heure : rien n'est retenu.
	if ctx.Err() == nil {
		databasesMutex.Lock()
		databases[system] = loadedDatabase{index: index, loadedAt: timeNow(), complete: complete}
		databasesMutex.Unlock()
	}

	return index, nil
}

// libretroDatabaseFile — le fichier gardé d'un `.dat`, téléchargé s'il manque
// ou a 30 jours. Un téléchargement raté laisse l'ancien fichier, s'il y en a
// un : le chemin vient alors avec l'erreur.
func libretroDatabaseFile(ctx context.Context, cacheRoot, field, system string) (string, error) {
	parts := []string{system + ".dat"}
	if field != "" {
		parts = []string{field, system + ".dat"}
	}
	cached := filepath.Join(append([]string{cacheRoot, "libretro-database"}, parts...)...)
	info, statErr := os.Stat(cached)
	if statErr == nil && timeNow().Sub(info.ModTime()) < libretroDatabaseAge {
		return cached, nil
	}

	if err := fetchLibretroDatabase(ctx, parts, cached); err != nil {
		if statErr == nil {
			return cached, err
		}
		return "", err
	}

	return cached, nil
}

func fetchLibretroDatabase(ctx context.Context, parts []string, target string) error {
	select {
	case libretroDatabaseSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-libretroDatabaseSlots }()

	escaped := make([]string, len(parts))
	for index, part := range parts {
		escaped[index] = url.PathEscape(part)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, libretroDatabaseBase+"/"+strings.Join(escaped, "/"), nil)
	if err != nil {
		return err
	}
	response, err := libretroClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotFound:
		// Le dépôt n'a pas ce champ pour ce système : gardé vide.
		return writeAtomically(target, nil)
	case response.StatusCode != http.StatusOK:
		return fmt.Errorf("%s: %s", strings.Join(parts, "/"), response.Status)
	case response.ContentLength > libretroDatabaseLimit:
		return fmt.Errorf("%s: %d bytes", strings.Join(parts, "/"), response.ContentLength)
	}

	return writeStreamAtomically(target, io.LimitReader(response.Body, libretroDatabaseLimit))
}

// ── Lire un `.dat` clrmamepro ─────────────────────────────────────────

// read — ajoute à l'index les jeux d'un `.dat` : des blocs
// `game ( clé valeur … rom ( clé valeur … ) )`, sur une ou plusieurs lignes,
// avec des valeurs entre guillemets ou nues (`crc 74D7BAE1`). Chaque bloc
// commence à un `game (` : un bloc mal formé ne fait perdre que lui.
func (index *databaseIndex) read(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	tokens := datTokens{reader: bufio.NewReaderSize(file, 64<<10)}

	previous := datToken{}
	for {
		token, err := tokens.next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if !token.is("(") {
			previous = token
			continue
		}
		block, err := tokens.block()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if previous.is("game") {
			index.add(block)
		}
		previous = datToken{}
	}
}

// add — un jeu, sous chacune de ses clés. Un même jeu revient dans le `.dat`
// de chaque champ, sous la même empreinte ou le même numéro de série, mais pas
// toujours sous le même nom (« Kid: Maboroshi » et « Kid - Maboroshi ») : ses
// noms désignent donc l'entrée de son identifiant, qui réunit les champs. Une
// valeur déjà connue n'est pas remplacée : le `.dat` complet, lu d'abord,
// prime sur ceux d'un champ.
func (index *databaseIndex) add(block datBlock) {
	crcs, serials, names := []uint32{}, []string{}, []string{}
	for _, values := range append([]map[string]string{block.values}, block.roms...) {
		if value, err := strconv.ParseUint(values["crc"], 16, 32); err == nil {
			crcs = append(crcs, uint32(value))
		}
		if serial := serialKey(values["serial"]); serial != "" {
			serials = append(serials, serial)
		}
	}
	for _, name := range []string{block.values["name"], block.values["comment"]} {
		if name != "" {
			names = append(names, name)
		}
	}
	for _, rom := range block.roms {
		if rom["name"] != "" {
			names = append(names, strings.TrimSuffix(rom["name"], filepath.Ext(rom["name"])))
		}
	}

	var entry *databaseEntry
	for _, crc := range crcs {
		if entry == nil {
			entry = index.byCRC[crc]
		}
	}
	for _, serial := range serials {
		if entry == nil {
			entry = index.bySerial[serial]
		}
	}
	// Par le nom seulement pour un jeu sans identifiant : deux dumps
	// différents peuvent porter le même nom.
	if len(crcs) == 0 && len(serials) == 0 {
		for _, name := range names {
			if entry == nil {
				entry = index.byName[name]
			}
		}
	}
	if entry == nil {
		entry = &databaseEntry{}
	}

	if entry.developers == nil {
		entry.developers = splitList(block.values["developer"])
	}
	if entry.publishers == nil {
		entry.publishers = splitList(block.values["publisher"])
	}
	if entry.genres == nil {
		entry.genres = splitList(block.values["genre"])
	}
	if entry.year == "" {
		entry.year, entry.month, entry.day = block.values["releaseyear"], block.values["releasemonth"], block.values["releaseday"]
	}
	if entry.franchise == "" {
		entry.franchise = strings.TrimSpace(block.values["franchise"])
	}
	if entry.users == "" {
		entry.users = strings.TrimSpace(block.values["users"])
	}
	for _, crc := range crcs {
		if index.byCRC[crc] == nil {
			index.byCRC[crc] = entry
		}
	}
	for _, serial := range serials {
		if index.bySerial[serial] == nil {
			index.bySerial[serial] = entry
		}
	}
	for _, name := range names {
		if index.byName[name] == nil {
			index.byName[name] = entry
		}
	}
}

// legalSuffixes — ce qui suit une virgule sans être une autre société :
// « SOFEL Co., Ltd. » est un seul éditeur.
var legalSuffixes = regexp.MustCompile(`^(?i:inc|ltd|llc|co|corp|s\.a|gmbh|limited)\.?$`)

// splitList — une valeur de la base, qui sépare ses sociétés et ses genres
// par des virgules (« Nintendo R&D4, Intelligent Systems »).
func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	items := []string{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case legalSuffixes.MatchString(part) && len(items) > 0:
			items[len(items)-1] += ", " + part
		default:
			items = appendNew(items, part)
		}
	}

	return items
}

// datWanted — les clés gardées d'un bloc ; le reste (résumés, empreintes MD5
// et SHA1, classifications) est lu et jeté.
var datWanted = map[string]bool{
	"name": true, "comment": true, "serial": true, "crc": true,
	"developer": true, "publisher": true, "genre": true,
	"releaseyear": true, "releasemonth": true, "releaseday": true, "franchise": true,
	"users": true,
}

type datBlock struct {
	values map[string]string
	roms   []map[string]string
}

type datToken struct {
	text   string
	quoted bool
}

func (token datToken) is(paren string) bool {
	return !token.quoted && token.text == paren
}

type datTokens struct {
	reader *bufio.Reader
}

// block — un bloc ouvert, jusqu'à sa parenthèse fermante. Les blocs
// imbriqués autres que `rom` sont sautés.
func (tokens *datTokens) block() (datBlock, error) {
	block := datBlock{values: map[string]string{}}
	for {
		key, err := tokens.next()
		if err != nil {
			return block, err
		}
		if key.is(")") {
			return block, nil
		}
		value, err := tokens.next()
		if err != nil {
			return block, err
		}
		if value.is(")") {
			return block, nil
		}
		if value.is("(") {
			child, err := tokens.block()
			if err != nil {
				return block, err
			}
			if key.text == "rom" {
				block.roms = append(block.roms, child.values)
			}
			continue
		}
		if datWanted[key.text] {
			if _, seen := block.values[key.text]; !seen {
				block.values[key.text] = value.text
			}
		}
	}
}

// next — le prochain mot, chaîne entre guillemets ou parenthèse.
func (tokens *datTokens) next() (datToken, error) {
	for {
		character, err := tokens.reader.ReadByte()
		if err != nil {
			return datToken{}, err
		}
		switch {
		case character == ' ' || character == '\t' || character == '\r' || character == '\n':
			continue
		case character == '(' || character == ')':
			return datToken{text: string(character)}, nil
		case character == '"':
			text, err := tokens.quoted()
			return datToken{text: text, quoted: true}, err
		}

		var builder strings.Builder
		builder.WriteByte(character)
		for {
			next, err := tokens.reader.ReadByte()
			if errors.Is(err, io.EOF) {
				return datToken{text: builder.String()}, nil
			}
			if err != nil {
				return datToken{}, err
			}
			if next == ' ' || next == '\t' || next == '\r' || next == '\n' || next == '(' || next == ')' {
				_ = tokens.reader.UnreadByte()
				return datToken{text: builder.String()}, nil
			}
			builder.WriteByte(next)
		}
	}
}

// quoted — la fin d'une chaîne ouverte. clrmamepro n'échappe rien : le
// guillemet qui la ferme est celui que suit un blanc, une parenthèse ou la
// fin du fichier. La GBA a `developer ""Natsume", Vicarious Visions"`, qui
// décalait toute la suite du fichier.
func (tokens *datTokens) quoted() (string, error) {
	var builder strings.Builder
	for {
		part, err := tokens.reader.ReadString('"')
		if err != nil {
			return "", err
		}
		builder.WriteString(part)
		following, err := tokens.reader.Peek(1)
		if errors.Is(err, io.EOF) || (err == nil && strings.ContainsRune(" \t\r\n()", rune(following[0]))) {
			return strings.TrimSuffix(builder.String(), `"`), nil
		}
		if err != nil {
			return "", err
		}
	}
}
