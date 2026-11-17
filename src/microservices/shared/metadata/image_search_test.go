package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeImageProviders — IGDB (jeton, recherche), SteamGridDB (recherche, listes)
// et leurs hôtes d'images, fabriqués sur un même serveur ; une liste de
// SteamGridDB glisse une image chez un autre hôte, qui ne doit jamais être
// proposée. Rend l'adresse du serveur et les requêtes reçues (chemin, requête
// et corps d'une recherche IGDB).
func fakeImageProviders(t *testing.T) (string, func() []string) {
	t.Helper()
	var mutex sync.Mutex
	seen := []string{}
	png1 := func(w http.ResponseWriter) {
		_ = png.Encode(w, image.NewGray(image.Rect(0, 0, 64, 48)))
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mutex.Lock()
		seen = append(seen, strings.TrimSpace(r.URL.Path+"?"+r.URL.RawQuery+" "+string(body)))
		mutex.Unlock()
		switch {
		case r.URL.Path == "/token":
			fmt.Fprint(w, `{"access_token":"jeton","expires_in":5000000}`)
		case r.URL.Path == "/igdb/games":
			fmt.Fprint(w, `[{"id":1,"name":"Chrono Trigger","cover":{"image_id":"co1","width":600,"height":800},`+
				`"artworks":[{"image_id":"ar1","width":1920,"height":1080},{"image_id":"ar2"}],"screenshots":[{"image_id":"sc1"}]},`+
				`{"id":2,"name":"Chrono Cross","artworks":[{"image_id":"ar3"}]}]`)
		case strings.HasPrefix(r.URL.Path, "/sgdb/search/autocomplete/"):
			fmt.Fprint(w, `{"data":[{"id":11,"name":"Chrono Trigger"},{"id":12,"name":"Chrono Cross"},{"id":13,"name":"Chrono Trigger DS"},{"id":14,"name":"Quatrième"}]}`)
		case strings.HasPrefix(r.URL.Path, "/sgdb/"):
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/sgdb/"), "/") // grids, game, 11
			fmt.Fprintf(w, `{"data":[{"url":"%[1]s/cdn/%[2]s/%[3]s-a.png","thumb":"%[1]s/cdn/thumb/%[3]s-a.jpg","width":600,"height":900},`+
				`{"url":"%[1]s/cdn/%[2]s/%[3]s-b.png","thumb":"","width":0,"height":0},`+
				`{"url":"https://ailleurs.invalid/%[3]s.png","thumb":"https://ailleurs.invalid/t.png","width":1,"height":1}]}`, server.URL, parts[0], parts[2])
		case r.URL.Path == "/img/huge.jpg":
			_, _ = w.Write(bytes.Repeat([]byte{0xff}, ChosenImageLimit+10))
		case r.URL.Path == "/img/moved.jpg":
			http.Redirect(w, r, "/img/t_1080p/ar1.jpg", http.StatusFound)
		case r.URL.Path == "/img/elsewhere.jpg":
			http.Redirect(w, r, "http://ailleurs.invalid/x.jpg", http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/img/"), strings.HasPrefix(r.URL.Path, "/cdn/"):
			png1(w)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	previous := []string{igdbTokenURL, igdbAPI, igdbImages, steamGridDBAPI}
	previousOrigins := steamGridDBImageOrigins
	igdbTokenURL, igdbAPI, igdbImages, steamGridDBAPI = server.URL+"/token", server.URL+"/igdb", server.URL+"/img", server.URL+"/sgdb"
	steamGridDBImageOrigins = []string{originOf(server.URL)}
	t.Cleanup(func() {
		igdbTokenURL, igdbAPI, igdbImages, steamGridDBAPI = previous[0], previous[1], previous[2], previous[3]
		steamGridDBImageOrigins = previousOrigins
	})
	igdbTokenMutex.Lock()
	igdbTokens = map[string]igdbToken{}
	igdbTokenMutex.Unlock()

	return server.URL, func() []string {
		mutex.Lock()
		defer mutex.Unlock()

		return slices.Clone(seen)
	}
}

// sourcesOf — « igdb=ok steamgriddb=noAccount … », l'état de chaque fournisseur.
func sourcesOf(found ImageSearch) string {
	states := []string{}
	for _, source := range found.Sources {
		states = append(states, source.Source+"="+source.State)
	}

	return strings.Join(states, " ")
}

// UNE RECHERCHE DE JAQUETTES : IGDB (la jaquette de chaque jeu trouvé, sur la
// plateforme de la console), SteamGridDB (les jaquettes 600 × 900 de ses trois
// premiers jeux), libretro (les noms qui contiennent le titre, le même titre
// d'abord), dans cet ordre ; une image chez un autre hôte n'est jamais
// proposée ; chaque image a sa vignette.
func TestAGameImageSearchAsksEachProvider(t *testing.T) {
	base, seen := fakeImageProviders(t)
	fakeLibretro(t, map[string][]string{"Nintendo - Super Nintendo Entertainment System/Named_Boxarts": {
		"Super Mario World (USA)", "Chrono Trigger (Japan)", "Chrono Trigger (USA)", "Radical Dreamers - Chrono Trigger (Japan)"}})
	cacheRoot := t.TempDir()

	found, err := SearchGameImages(context.Background(), cacheRoot, testAccounts, "snes", KindCover, "  Chrono   Trigger ", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if found.Query != "Chrono Trigger" || found.Kind != KindCover || sourcesOf(found) != "igdb=ok steamgriddb=ok libretro=ok" {
		t.Fatalf("recherche : %q %q %s", found.Query, found.Kind, sourcesOf(found))
	}
	want := []string{
		"igdb " + base + "/img/t_cover_big_2x/co1.jpg " + base + "/img/t_cover_big/co1.jpg Chrono Trigger 600x800",
		"steamgriddb " + base + "/cdn/grids/11-a.png " + base + "/cdn/thumb/11-a.jpg Chrono Trigger 600x900",
		"steamgriddb " + base + "/cdn/grids/11-b.png " + base + "/cdn/grids/11-b.png Chrono Trigger 0x0",
		"steamgriddb " + base + "/cdn/grids/12-a.png " + base + "/cdn/thumb/12-a.jpg Chrono Cross 600x900",
		"steamgriddb " + base + "/cdn/grids/12-b.png " + base + "/cdn/grids/12-b.png Chrono Cross 0x0",
		"steamgriddb " + base + "/cdn/grids/13-a.png " + base + "/cdn/thumb/13-a.jpg Chrono Trigger DS 600x900",
		"steamgriddb " + base + "/cdn/grids/13-b.png " + base + "/cdn/grids/13-b.png Chrono Trigger DS 0x0",
		"libretro " + libretroURL("Nintendo - Super Nintendo Entertainment System", "Named_Boxarts", "Chrono Trigger (Japan).png") + " = Chrono Trigger (Japan) 0x0",
		"libretro " + libretroURL("Nintendo - Super Nintendo Entertainment System", "Named_Boxarts", "Chrono Trigger (USA).png") + " = Chrono Trigger (USA) 0x0",
		"libretro " + libretroURL("Nintendo - Super Nintendo Entertainment System", "Named_Boxarts", "Radical Dreamers - Chrono Trigger (Japan).png") + " = Radical Dreamers - Chrono Trigger (Japan) 0x0",
	}
	got := []string{}
	for _, item := range found.Items {
		thumb := item.ThumbURL
		if item.Source == sourceLibretro && thumb == item.URL {
			thumb = "="
		}
		got = append(got, fmt.Sprintf("%s %s %s %s %dx%d", item.Source, item.URL, thumb, item.Game, item.Width, item.Height))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("images :\n%s\nattendu :\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	requests := strings.Join(seen(), "\n")
	for _, expected := range []string{`search "Chrono Trigger"; fields name,cover.image_id,cover.width,cover.height; where platforms = (19);`,
		"/sgdb/grids/game/11?types=static&dimensions=600x900"} {
		if !strings.Contains(requests, expected) {
			t.Errorf("requête %q absente :\n%s", expected, requests)
		}
	}
	if strings.Contains(requests, "/sgdb/grids/game/14") {
		t.Error("un quatrième jeu de SteamGridDB a été lu")
	}
}

// SANS COMPTE, LIBRETRO SEUL : IGDB et SteamGridDB ne sont pas interrogés, et
// la réponse le dit ; un fournisseur qui n'a rien de cette sorte non plus.
func TestWithoutAccountsOnlyLibretroAnswers(t *testing.T) {
	_, seen := fakeImageProviders(t)
	fakeLibretro(t, map[string][]string{"Nintendo - Super Nintendo Entertainment System/Named_Logos": {"Chrono Trigger (USA)"}})

	found, err := SearchGameImages(context.Background(), t.TempDir(), nil, "snes", KindLogo, "Chrono Trigger", "", nil)
	if err != nil || sourcesOf(found) != "igdb=none steamgriddb=noAccount libretro=ok" || len(found.Items) != 1 || len(seen()) != 0 {
		t.Fatalf("sans compte : %+v, %v, %d requêtes", found, err, len(seen()))
	}
	// Une console que libretro n'a pas : rien chez lui non plus.
	found, err = SearchGameImages(context.Background(), t.TempDir(), nil, "ps5", KindTitle, "Astro Bot", "", nil)
	if err != nil || sourcesOf(found) != "igdb=none steamgriddb=none libretro=none" || len(found.Items) != 0 {
		t.Fatalf("ps5 sans compte : %+v, %v", found, err)
	}
}

// LA BANDE D'UNE CONSOLE : les artworks d'IGDB, les bandeaux de SteamGridDB,
// les jaquettes de libretro ; une console d'un jeu à distance cherche dans
// celle qu'elle joue. Un titre vide, une sorte ou une console inconnue sont
// refusés.
func TestABandSearchAndItsRefusals(t *testing.T) {
	base, seen := fakeImageProviders(t)
	fakeLibretro(t, map[string][]string{"Sony - PlayStation 4/Named_Boxarts": {"Persona 5 (USA)"}})

	found, err := SearchBandImages(context.Background(), t.TempDir(), testAccounts, "rp-ps4", "Persona 5")
	if err != nil || found.Kind != "band" || sourcesOf(found) != "igdb=ok steamgriddb=ok libretro=ok" {
		t.Fatalf("bande : %+v, %v", found, err)
	}
	if first := found.Items[0]; first.URL != base+"/img/t_1080p/ar1.jpg" || first.ThumbURL != base+"/img/t_screenshot_med/ar1.jpg" || first.Width != 1920 {
		t.Fatalf("premier artwork : %+v", first)
	}
	if last := found.Items[len(found.Items)-1]; last.Source != sourceLibretro || last.Game != "Persona 5 (USA)" {
		t.Fatalf("dernière image : %+v", last)
	}
	requests := strings.Join(seen(), "\n")
	if !strings.Contains(requests, "where platforms = (48);") || !strings.Contains(requests, "/sgdb/heroes/game/11?types=static") || strings.Contains(requests, "/sgdb/heroes/game/11?types=static&") {
		t.Fatalf("requêtes de la bande :\n%s", requests)
	}

	if _, err := SearchBandImages(context.Background(), t.TempDir(), testAccounts, "playstation9", "x"); !errors.Is(err, ErrUnknownConsole) {
		t.Errorf("console inconnue : %v", err)
	}
	if _, err := SearchGameImages(context.Background(), t.TempDir(), testAccounts, "snes", "poster", "x", "", nil); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("sorte inconnue : %v", err)
	}
	if _, err := SearchGameImages(context.Background(), t.TempDir(), testAccounts, "snes", KindCover, "   ", "", nil); !errors.Is(err, ErrEmptyQuery) {
		t.Errorf("titre vide : %v", err)
	}
}

// LA LISTE DES HÔTES PERMIS, telle qu'en service : les images d'IGDB, de
// SteamGridDB et de libretro, en https, rien d'autre — ni un hôte qui leur
// ressemble, ni un autre schéma, ni des identifiants, ni une source inconnue.
func TestTheImageHostAllowlist(t *testing.T) {
	for address, source := range map[string]string{
		"https://images.igdb.com/igdb/image/upload/t_1080p/ar1.jpg":                                          sourceIGDB,
		"https://cdn2.steamgriddb.com/grid/abc.png":                                                          sourceSteamGridDB,
		"https://cdn.steamgriddb.com/thumb/abc.jpg":                                                          sourceSteamGridDB,
		"https://thumbnails.libretro.com/Sony%20-%20PlayStation/Named_Boxarts/Crash%20Bandicoot%20(USA).png": sourceLibretro,
	} {
		if !allowedImage(source, address) {
			t.Errorf("%s refusée chez %s", address, source)
		}
	}
	for _, refused := range []struct{ source, address string }{
		{sourceIGDB, "https://images.igdb.com.evil.example/x.jpg"},
		{sourceIGDB, "http://images.igdb.com/igdb/image/upload/x.jpg"},
		{sourceIGDB, "https://user:secret@images.igdb.com/x.jpg"},
		{sourceIGDB, "https://cdn2.steamgriddb.com/grid/abc.png"},
		{sourceSteamGridDB, "https://www.steamgriddb.com/api/v2/grids/game/1"},
		{sourceLibretro, "file:///etc/passwd"},
		{sourceLibretro, "https://thumbnails.libretro.com:8443/x.png"},
		{"screenscraper", "https://images.igdb.com/x.jpg"},
		{sourceManual, "https://images.igdb.com/x.jpg"},
		{sourceIGDB, "http://127.0.0.1:8080/internal/x"},
		{sourceIGDB, "images.igdb.com/x.jpg"},
		{sourceIGDB, ""},
	} {
		if allowedImage(refused.source, refused.address) {
			t.Errorf("%s permise chez %s", refused.address, refused.source)
		}
		if _, err := FetchChosenImage(context.Background(), refused.source, refused.address); !errors.Is(err, ErrImageNotAllowed) {
			t.Errorf("%s récupérée chez %s : %v", refused.address, refused.source, err)
		}
	}
}

// L'IMAGE CHOISIE : récupérée chez son hôte (une redirection sur le même hôte
// est suivie), refusée quand une redirection en sort ou qu'elle passe 16 Mo ;
// une source qui ne la rend pas est une erreur à part.
func TestAChosenImageIsFetchedOnlyFromItsHost(t *testing.T) {
	base, _ := fakeImageProviders(t)

	content, err := FetchChosenImage(context.Background(), sourceIGDB, base+"/img/t_1080p/ar1.jpg")
	if err != nil || ImageExtension(content) != ".png" {
		t.Fatalf("image : %v, %d o", err, len(content))
	}
	if _, err := FetchChosenImage(context.Background(), sourceIGDB, base+"/img/moved.jpg"); err != nil {
		t.Fatalf("redirection sur le même hôte : %v", err)
	}
	if _, err := FetchChosenImage(context.Background(), sourceIGDB, base+"/img/elsewhere.jpg"); !errors.Is(err, ErrImageNotAllowed) {
		t.Fatalf("redirection vers un autre hôte : %v", err)
	}
	if _, err := FetchChosenImage(context.Background(), sourceIGDB, base+"/img/huge.jpg"); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("plus de 16 Mo : %v", err)
	}
	if _, err := FetchChosenImage(context.Background(), sourceSteamGridDB, base+"/absente.png"); err == nil || errors.Is(err, ErrImageNotAllowed) {
		t.Fatalf("une image que la source ne rend pas : %v", err)
	}
}

// UNE IMAGE DE JEU POSÉE DEPUIS UN RÉSULTAT : la recherche la propose, son
// adresse se récupère et devient l'image manuelle de sa sorte, qui passe
// devant les services ; la fiche la dit mise à la main.
func TestAGameImageIsSetFromASearchResult(t *testing.T) {
	fakeImageProviders(t)
	root := t.TempDir()
	game := ps5Game(t, "Chrono Trigger")

	found, err := SearchGameImages(context.Background(), filepath.Join(root, "cache", "metadata", "sources"), testAccounts, "ps5", KindBackground, SearchTitle(game), "", nil)
	if err != nil || len(found.Items) == 0 {
		t.Fatalf("recherche : %+v, %v", found, err)
	}
	chosen := found.Items[0]
	content, err := FetchChosenImage(context.Background(), chosen.Source, chosen.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetManualImage(root, "ps5", game, "ref", KindBackground, content); err != nil {
		t.Fatal(err)
	}
	gameDir := filepath.Join(root, "metadata", "ps5", "PPSA01234")
	if path := manualImage(gameDir, KindBackground); path == "" {
		t.Fatal("pas d'image manuelle")
	} else if stored, _ := os.ReadFile(path); !bytes.Equal(stored, content) {
		t.Fatal("l'image écrite n'est pas celle choisie")
	}
	// La fiche ne dit pas qu'une image a été remplacée (le porteur, 2026-11-14) : l'image le montre, pas la fiche.
	info := Info(context.Background(), onRoot(root), nil, "ps5", game, "ref")
	if encoded, _ := json.Marshal(info); strings.Contains(string(encoded), "manual") {
		t.Fatalf("la fiche dit « manual » : %s", encoded)
	}
}

// UN EMPLACEMENT DU FOND REMPLI DEPUIS UN RÉSULTAT DE LA BANDE.
func TestABackgroundSlotIsSetFromASearchResult(t *testing.T) {
	fakeImageProviders(t)
	root := t.TempDir()
	directory := ConsoleBackgroundDir(root, "ps5")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(directory, backgroundMarker), []byte("{}"), 0o644)

	found, err := SearchBandImages(context.Background(), t.TempDir(), testAccounts, "ps5", "Astro Bot")
	if err != nil || len(found.Items) == 0 {
		t.Fatalf("recherche : %+v, %v", found, err)
	}
	content, err := FetchChosenImage(context.Background(), found.Items[0].Source, found.Items[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	written, err := SetConsoleBackground(root, "ps5", 7, content)
	if err != nil || written != "07.png" {
		t.Fatalf("emplacement 7 : %q, %v", written, err)
	}
	slots, _ := ConsoleBackgrounds(root, nil, "ps5", 0)
	if slots.Slots[6].Name != "07.png" || len(filledSlots(slots)) != 1 {
		t.Fatalf("fond : %+v", slots)
	}
}
