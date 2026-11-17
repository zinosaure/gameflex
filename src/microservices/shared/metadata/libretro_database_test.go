package metadata

import (
	"context"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDatabase — un dossier `metadat/` fabriqué : `files` par chemin
// (« developer/Nintendo - Nintendo Entertainment System.dat »), 404 pour le
// reste. Compte les requêtes, et vide l'index gardé en mémoire.
func fakeDatabase(t *testing.T, files map[string]string) *atomic.Int32 {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		content, found := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !found {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, content)
	}))
	t.Cleanup(server.Close)
	previous := libretroDatabaseBase
	libretroDatabaseBase = server.URL
	t.Cleanup(func() { libretroDatabaseBase = previous })
	forgetDatabases()

	return requests
}

func forgetDatabases() {
	databasesMutex.Lock()
	databases = map[string]loadedDatabase{}
	databasesMutex.Unlock()
}

func romCRC32(content string) string {
	return fmt.Sprintf("%08X", crc32.ChecksumIEEE([]byte(content)))
}

// fieldDat — un `.dat` d'un champ, comme ceux de `metadat/developer/`.
func fieldDat(system, field string, games ...[3]string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "clrmamepro (\n\tname \"%s\"\n\tdescription \"%s\"\n)\n\n", system, system)
	for _, game := range games {
		fmt.Fprintf(&builder, "game (\n\tcomment \"%s\"\n\t%s \"%s\"\n\trom ( crc %s )\n)\n\n", game[0], field, game[1], game[2])
	}

	return builder.String()
}

const nesSystem = "Nintendo - Nintendo Entertainment System"

// L'EMPREINTE D'ABORD : un fichier mal nommé est reconnu par son CRC. Les
// sept `.dat` du système ne sont demandés qu'une fois, gardés dans le cache,
// et redemandés au bout d'un mois. La série vient de `franchise/`, le nombre
// de joueurs de `maxusers/` (sa valeur nue, comme dans le dépôt).
func TestAGameIsFoundByItsCRC(t *testing.T) {
	game := rom(t, "Jeu sans nom.nes")
	content, _ := os.ReadFile(game)
	crc := romCRC32(string(content))
	requests := fakeDatabase(t, map[string]string{
		"developer/" + nesSystem + ".dat": fieldDat(nesSystem, "developer", [3]string{"Other (USA)", "Other", "0BADBEEF"}, [3]string{"Kid Icarus (USA)", "Nintendo R&D1, Tose", crc}),
		"publisher/" + nesSystem + ".dat": fieldDat(nesSystem, "publisher", [3]string{"Kid Icarus (USA)", "SOFEL Co., Ltd.", crc}),
		// Une ROM sur plusieurs lignes, en minuscules : la forme du `.dat` de la DS.
		"genre/" + nesSystem + ".dat":       "game (\n   comment \"Kid Icarus (USA)\"\n   genre \"Action, Platform\"\n   rom (\n\t\tcrc " + strings.ToLower(crc) + "\n\t)\n)\n",
		"releaseyear/" + nesSystem + ".dat": fieldDat(nesSystem, "releaseyear", [3]string{"Kid Icarus (USA)", "1987", crc}),
		"franchise/" + nesSystem + ".dat":   fieldDat(nesSystem, "franchise", [3]string{"Kid Icarus (USA)", "Kid Icarus", crc}),
		"maxusers/" + nesSystem + ".dat":    "game (\n\tcomment \"Kid Icarus (USA)\"\n\tusers 2\n\trom ( crc " + crc + " )\n)\n",
	})
	root := t.TempDir()

	info := Info(context.Background(), onRoot(root), nil, "nes", game, "ref")
	if info.Series != "Kid Icarus" || info.Sources["series"] != "libretro-database" {
		t.Fatalf("series: %+v", info)
	}
	if info.Players != 2 || info.Sources["players"] != "libretro-database" {
		t.Fatalf("players: %+v", info)
	}
	if info.GameID != "crc-"+crc || info.ReleaseDate != "1987" ||
		strings.Join(info.Developers, "|") != "Nintendo R&D1|Tose" ||
		strings.Join(info.Publishers, "|") != "SOFEL Co., Ltd." ||
		strings.Join(info.Genres, "|") != "Action|Platform" {
		t.Fatalf("info = %+v", info)
	}
	for _, field := range []string{"releaseDate", "publishers", "developers", "genres"} {
		if info.Sources[field] != "libretro-database" {
			t.Fatalf("sources = %v", info.Sources)
		}
	}

	// Le complet et les six champs de la NES ; le Famicom Disk System, second
	// système de la console, n'est lu qu'à défaut.
	if got := requests.Load(); got != 7 {
		t.Fatalf("%d requests, want 7", got)
	}
	if _, err := os.Stat(filepath.Join(root, "cache", "metadata", "sources", "libretro-database", "developer", nesSystem+".dat")); err != nil {
		t.Fatal(err)
	}
	if empty, err := os.ReadFile(filepath.Join(root, "cache", "metadata", "sources", "libretro-database", nesSystem+".dat")); err != nil || len(empty) != 0 {
		t.Fatalf("the missing full dat is not kept empty: %q %v", empty, err)
	}

	// Une seconde fiche, et un redémarrage : rien n'est redemandé.
	Info(context.Background(), onRoot(root), nil, "nes", game, "ref")
	forgetDatabases()
	if again := Info(context.Background(), onRoot(root), nil, "nes", game, "ref"); again.ReleaseDate != "1987" {
		t.Fatalf("from the cache: %+v", again)
	}
	if got := requests.Load(); got != 7 {
		t.Fatalf("%d requests after reading the cache, want 7", got)
	}

	timeNow = func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }
	t.Cleanup(func() { timeNow = time.Now })
	Info(context.Background(), onRoot(root), nil, "nes", game, "ref")
	if got := requests.Load(); got != 14 {
		t.Fatalf("%d requests after a month, want 14", got)
	}
}

// LE NUMÉRO DE SÉRIE d'un nom de dossier PS1, cherché dans le `.dat` complet
// (psxdatacenter), qui donne le jour de sortie.
func TestAPlayStationGameIsFoundByTheSerialInItsName(t *testing.T) {
	fakeDatabase(t, map[string]string{
		"Sony - PlayStation.dat": `clrmamepro (
	name "Sony - PlayStation"
	version "2020.3.19"
)

game (
	name "Ace Combat 2 (Europe)"
	serial "SCES-00901"
	description "Ace Combat 2 is the sequel to the 1995 Arcade/Flight-sim Air Combat (with a long summary)."
	developer "Namco"
	publisher "Sony Computer Entertainment"
	releaseyear "1997"
	releasemonth "9"
	releaseday "10"
	users "1"
	genre "Shooter"
	rom (
		serial "SCES-00901"
		name "Ace Combat 2 (Europe).cue"
	)
)
`,
	})
	game := filepath.Join(t.TempDir(), "[PS1] Ace Combat 2 [F] [SCES-00901]")
	writeTestFile(t, filepath.Join(game, "Ace Combat 2.cue"), "FILE \"Ace Combat 2.bin\" BINARY")

	info := Info(context.Background(), onRoot(t.TempDir()), nil, "ps1", game, "ref")
	if info.ReleaseDate != "1997-09-10" || info.Publishers[0] != "Sony Computer Entertainment" || info.Developers[0] != "Namco" || info.Genres[0] != "Shooter" {
		t.Fatalf("info = %+v", info)
	}
	if info.GameID != "ref" || info.Sources["releaseDate"] != "libretro-database" {
		t.Fatalf("info = %+v", info)
	}
	// Le `.dat` complet d'une PlayStation porte le nombre de joueurs, entre guillemets.
	if info.Players != 1 || info.Sources["players"] != "libretro-database" {
		t.Fatalf("players: %+v", info)
	}
}

// LE NOM, quand ni l'empreinte ni un numéro de série ne mènent au jeu : exact,
// puis approché. Deux `.dat` qui écrivent le même jeu autrement (« : » et
// « - ») se retrouvent par son empreinte.
func TestAGameIsFoundByItsName(t *testing.T) {
	system := "Nintendo - Super Nintendo Entertainment System"
	fakeDatabase(t, map[string]string{
		"developer/" + system + ".dat": fieldDat(system, "developer",
			[3]string{"Super Metroid (Japan, USA) (En,Ja)", "Nintendo R&D1", "D63ED4F2"},
			[3]string{"Dollar Kid: Maboroshi (Japan)", "SOFEL", "FCCB5CEA"}),
		"genre/" + system + ".dat": fieldDat(system, "genre",
			[3]string{"Super Metroid (Japan, USA) (En,Ja)", "Action", "D63ED4F2"},
			[3]string{"Dollar Kid - Maboroshi (Japan)", "Gambling", "FCCB5CEA"}),
	})
	root := t.TempDir()

	loose := Info(context.Background(), onRoot(root), nil, "snes", rom(t, "Super Metroid.sfc"), "ref")
	if strings.Join(loose.Developers, "|") != "Nintendo R&D1" || strings.Join(loose.Genres, "|") != "Action" {
		t.Fatalf("loose name: %+v", loose)
	}

	exact := Info(context.Background(), onRoot(root), nil, "snes", rom(t, "Dollar Kid - Maboroshi (Japan).sfc"), "ref")
	if strings.Join(exact.Developers, "|") != "SOFEL" || strings.Join(exact.Genres, "|") != "Gambling" {
		t.Fatalf("exact name: %+v", exact)
	}
	if exact.ReleaseDate != "" || exact.Publishers != nil || exact.Sources["releaseDate"] != "" {
		t.Fatalf("an unknown field is not absent: %+v", exact)
	}
}

// L'ORDRE DU § 3 pour le rétro : la libretro-database d'abord, IGDB pour ce
// qu'elle n'a pas — et pas du tout quand elle a tout.
func TestTheMergeFollowsTheOrder(t *testing.T) {
	system := "Nintendo - Super Nintendo Entertainment System"
	game := rom(t, "7th Saga, The (USA).sfc")
	content, _ := os.ReadFile(game)
	crc := romCRC32(string(content))
	fakeDatabase(t, map[string]string{
		"developer/" + system + ".dat": fieldDat(system, "developer", [3]string{"7th Saga, The (USA)", "Produce", crc}),
		"genre/" + system + ".dat":     fieldDat(system, "genre", [3]string{"7th Saga, The (USA)", "Role-playing (RPG)", crc}),
		"franchise/" + system + ".dat": fieldDat(system, "franchise", [3]string{"7th Saga, The (USA)", "Seventh Saga", crc}),
	})
	requests, lastQuery := fakeServices(t)
	accounts := Accounts{"igdb": testAccounts["igdb"]}

	info := Info(context.Background(), onRoot(t.TempDir()), accounts, "snes", game, "ref")
	if strings.Join(info.Developers, "|") != "Produce" || strings.Join(info.Genres, "|") != "Role-playing (RPG)" ||
		info.Sources["developers"] != "libretro-database" || info.Sources["genres"] != "libretro-database" {
		t.Fatalf("libretro-database first: %+v", info)
	}
	if info.Publishers[0] != "Square Enix" || info.ReleaseDate != "2024-02-29" ||
		info.Sources["publishers"] != "igdb" || info.Sources["releaseDate"] != "igdb" {
		t.Fatalf("igdb for the rest: %+v", info)
	}
	if !strings.Contains(*lastQuery, `search "7th Saga, The"`) || !strings.Contains(*lastQuery, "where platforms = (19)") {
		t.Fatalf("query = %q", *lastQuery)
	}
	// La série fait exception : IGDB d'abord.
	if info.Series != "The 7th Saga" || info.Sources["series"] != "igdb" {
		t.Fatalf("series: %+v", info)
	}

	// Un jeu que la base connaît en entier : IGDB n'est pas interrogé.
	full := rom(t, "Kid Icarus (USA).nes")
	fullContent, _ := os.ReadFile(full)
	fullCRC := romCRC32(string(fullContent))
	fakeDatabase(t, map[string]string{
		"developer/" + nesSystem + ".dat":   fieldDat(nesSystem, "developer", [3]string{"Kid Icarus (USA)", "Nintendo R&D1", fullCRC}),
		"publisher/" + nesSystem + ".dat":   fieldDat(nesSystem, "publisher", [3]string{"Kid Icarus (USA)", "Nintendo", fullCRC}),
		"genre/" + nesSystem + ".dat":       fieldDat(nesSystem, "genre", [3]string{"Kid Icarus (USA)", "Action", fullCRC}),
		"releaseyear/" + nesSystem + ".dat": fieldDat(nesSystem, "releaseyear", [3]string{"Kid Icarus (USA)", "1987", fullCRC}),
	})
	before := requests.Load()
	if complete := Info(context.Background(), onRoot(t.TempDir()), accounts, "nes", full, "ref"); complete.Sources["publishers"] != "libretro-database" {
		t.Fatalf("info = %+v", complete)
	}
	if requests.Load() != before {
		t.Fatal("IGDB was asked for a game the database knows entirely")
	}
}

// Un service tombé ne prive pas des autres : la base injoignable, IGDB donne
// ce qu'il a.
func TestAnUnreachableDatabaseFallsBackToIGDB(t *testing.T) {
	fakeServices(t)
	previous := libretroDatabaseBase
	libretroDatabaseBase = "http://127.0.0.1:1"
	t.Cleanup(func() { libretroDatabaseBase = previous })
	forgetDatabases()

	info := Info(context.Background(), onRoot(t.TempDir()), Accounts{"igdb": testAccounts["igdb"]}, "snes", rom(t, "Final Fantasy VII Rebirth.sfc"), "ref")
	if info.Sources["genres"] != "igdb" || info.ReleaseDate != "2024-02-29" {
		t.Fatalf("info = %+v", info)
	}
}

func TestSplitList(t *testing.T) {
	for value, want := range map[string]string{
		"Nintendo R&D4, Nintendo R&D1, Intelligent Systems": "Nintendo R&D4|Nintendo R&D1|Intelligent Systems",
		"SOFEL Co., Ltd.":      "SOFEL Co., Ltd.",
		"Konami, Inc., Hudson": "Konami, Inc.|Hudson",
		"Board, Shogi, Board":  "Board|Shogi",
		"":                     "",
	} {
		if got := strings.Join(splitList(value), "|"); got != want {
			t.Errorf("splitList(%q) = %q, want %q", value, got, want)
		}
	}
}

// clrmamepro n'échappe rien : la GBA a des guillemets dans une valeur, et un
// bloc mal formé ne doit pas faire perdre la suite du fichier.
func TestADatWithQuotesInsideAValueKeepsTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gba.dat")
	writeTestFile(t, path, `game (
	comment "2 Games in 1: Monstruos, S.A. + Buscando a Nemo (Spain)"
	developer ""Natsume", Vicarious Visions"
	rom ( crc 9FF018F9 )
)

game (
	comment "Broken (World)"
	developer
)

game (
	comment "Advance Wars (USA)"
	developer "Intelligent Systems"
	rom ( name "Advance Wars (USA).gba" size 4194304 crc 5AD0E571 serial "AWRE" )
)
`)
	index := &databaseIndex{byCRC: map[uint32]*databaseEntry{}, bySerial: map[string]*databaseEntry{}, byName: map[string]*databaseEntry{}}
	if err := index.read(path); err != nil {
		t.Fatal(err)
	}
	if got := index.byCRC[0x9FF018F9]; got == nil || strings.Join(got.developers, "|") != `"Natsume"|Vicarious Visions` {
		t.Fatalf("quoted value: %+v", got)
	}
	if got := index.byCRC[0x5AD0E571]; got == nil || got.developers[0] != "Intelligent Systems" || index.byName["Advance Wars (USA)"] != got {
		t.Fatalf("the game after them is lost: %+v", got)
	}
}

// LA SÉRIE POUR LA LISTE, sans rien lire ni télécharger : celle qu'IGDB a
// donnée à la fiche du jeu, sinon la franchise d'un index déjà en mémoire, par
// le numéro de série du nom ou par le nom ; rien tant que l'index n'est pas
// chargé.
func TestCachedSeriesReadsOnlyMemory(t *testing.T) {
	requests := fakeDatabase(t, map[string]string{
		"Sony - PlayStation.dat": "game (\n\tname \"Tekken 3 (Europe)\"\n\tserial \"SCES-01237\"\n\tfranchise \"Tekken\"\n)\n",
	})
	if series, source := CachedSeries("ps1")("ref", "[PS1] Tekken 3 [E] [SCES-01237]"); series != "" || source != "" || requests.Load() != 0 {
		t.Fatalf("before any sheet: %q %q after %d requests", series, source, requests.Load())
	}

	game := filepath.Join(t.TempDir(), "[PS1] Tekken 3 [E] [SCES-01237]")
	writeTestFile(t, filepath.Join(game, "Tekken 3.cue"), "x")
	Info(context.Background(), onRoot(t.TempDir()), nil, "ps1", game, "ref")
	before := requests.Load()
	lookup := CachedSeries("ps1")
	for round := 0; round < 2; round++ {
		if series, source := lookup("autre", "[PS1] Tekken 3 [E] [SCES-01237]"); series != "Tekken" || source != "libretro-database" {
			t.Fatalf("by serial: %q %q", series, source)
		}
		if series, _ := lookup("autre", "Tekken 3 (Europe)"); series != "Tekken" {
			t.Fatalf("by name: %q", series)
		}
	}
	if requests.Load() != before {
		t.Fatal("the listing lookup asked the network")
	}

	rememberSeries("ps1", "ref", "Tekken (IGDB)", sourceIGDB)
	t.Cleanup(func() { forgetSeries("ps1", "ref") })
	if series, source := CachedSeries("ps1")("ref", "[PS1] Tekken 3 [E] [SCES-01237]"); series != "Tekken (IGDB)" || source != "igdb" {
		t.Fatalf("igdb first: %q %q", series, source)
	}
}
