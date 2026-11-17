package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rawChanges(t *testing.T, document string) map[string]json.RawMessage {
	t.Helper()
	var changes map[string]json.RawMessage
	if err := json.Unmarshal([]byte(document), &changes); err != nil {
		t.Fatal(err)
	}

	return changes
}

// LE TEXTE MANUEL PASSE DEVANT, CHAMP PAR CHAMP : ce qu'il donne vient de
// lui, le reste d'IGDB ; un champ absent ne change pas ; null ou vide
// l'efface et IGDB reprend la main.
func TestManualTextWinsFieldByField(t *testing.T) {
	fakeServices(t)
	root := t.TempDir()
	game := ps5Game(t, "Final Fantasy VII Rebirth")
	accounts := Accounts{"igdb": testAccounts["igdb"]}

	err := SetManualText(root, "ps5", game, "ref", rawChanges(t, `{"title":"FF VII Rebirth","releaseDate":"2024","genres":["RPG"," RPG ","Aventure"],"series":"Compilation of FF VII"}`))
	if err != nil {
		t.Fatal(err)
	}
	info := Info(context.Background(), onRoot(root), accounts, "ps5", game, "ref")
	if info.Title != "FF VII Rebirth" || info.ReleaseDate != "2024" || strings.Join(info.Genres, "|") != "RPG|Aventure" || info.Series != "Compilation of FF VII" {
		t.Fatalf("corrigé : %+v", info)
	}
	// Un champ corrigé ne cite aucune source : la fiche ne dit jamais « à la main » (le porteur, 2026-11-14).
	for _, field := range []string{"title", "releaseDate", "genres", "series"} {
		if source, cited := info.Sources[field]; cited {
			t.Fatalf("%s cite %q", field, source)
		}
	}
	if info.Publishers[0] != "Square Enix" || info.Sources["publishers"] != "igdb" {
		t.Fatalf("le reste d'IGDB : %+v", info)
	}
	if series, source := CachedSeries("ps5")("ref", "Final Fantasy VII Rebirth"); series != "Compilation of FF VII" || source != "" {
		t.Fatalf("série de la liste : %q %q", series, source)
	}

	if err := SetManualText(root, "ps5", game, "ref", rawChanges(t, `{"genres":null,"releaseDate":""}`)); err != nil {
		t.Fatal(err)
	}
	info = Info(context.Background(), onRoot(root), accounts, "ps5", game, "ref")
	if info.ReleaseDate != "2024-02-29" || info.Sources["releaseDate"] != "igdb" || info.Sources["genres"] != "igdb" ||
		info.Series != "Compilation of FF VII" || info.Title != "FF VII Rebirth" {
		t.Fatalf("après effacement : %+v", info)
	}

	for _, document := range []string{`{"releaseDate":"2024-13-45"}`, `{"releaseDate":"20 avril"}`, `{"genres":"RPG"}`, `{"title":12}`, `{"rating":5}`} {
		if err := SetManualText(root, "ps5", game, "ref", rawChanges(t, document)); !errors.Is(err, ErrInvalidManual) {
			t.Errorf("%s : %v", document, err)
		}
	}
	// Tout ou rien : le refus n'a rien écrit.
	if info := Info(context.Background(), onRoot(root), accounts, "ps5", game, "ref"); info.Title != "FF VII Rebirth" {
		t.Fatalf("après les refus : %+v", info)
	}
	t.Cleanup(func() { forgetSeries("ps5", "ref") })
}

// L'IMAGE MANUELLE PASSE DEVANT, et les remettre par défaut rend celle de la
// source (une image seule ne se retire plus : elle se remplace, le porteur,
// 2026-11-14).
func TestManualImageWins(t *testing.T) {
	fakeServices(t)
	root := t.TempDir()
	game := ps5Game(t, "Final Fantasy VII Rebirth")
	accounts := Accounts{"igdb": testAccounts["igdb"]}
	var cover bytes.Buffer
	_ = png.Encode(&cover, transparentIcon(300, 400))

	if err := SetManualImage(root, "ps5", game, "ref", KindCover, cover.Bytes()); err != nil {
		t.Fatal(err)
	}
	path, err := Image(context.Background(), onRoot(root), accounts, "ps5", game, "ref", KindCover, 256)
	if err != nil || !strings.Contains(path, string(filepath.Separator)+"manual"+string(filepath.Separator)) {
		t.Fatalf("image : %s, %v", path, err)
	}
	// Réinitialiser la fiche (DropSource `manual`) la retire : l'image des services revient.
	if err := DropSource(root, "ps5", game, "ref", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restes de l'image remplacée : %v", err)
	}
	path, err = Image(context.Background(), onRoot(root), accounts, "ps5", game, "ref", KindCover, 256)
	if err != nil || !strings.Contains(path, "igdb") {
		t.Fatalf("après la réinitialisation : %s, %v", path, err)
	}
	if err := DropSource(root, "ps5", game, "ref", "manual"); err != nil {
		t.Fatalf("une seconde réinitialisation : %v", err)
	}
	if err := SetManualImage(root, "ps5", game, "ref", "../poster", cover.Bytes()); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("sorte inconnue : %v", err)
	}
	if err := SetManualImage(root, "ps5", game, "ref", KindLogo, []byte("<svg/>")); !errors.Is(err, ErrUnreadableImage) {
		t.Fatalf("SVG : %v", err)
	}
}

// LES CANDIDATS d'IGDB et de SteamGridDB, dans leur ordre ; sans compte, un
// refus clair.
func TestCandidates(t *testing.T) {
	fakeSearch(t, "Final Fantasy VII Remake", "Final Fantasy VII Rebirth")
	game := ps5Game(t, "Final Fantasy VII Rebirth")

	igdb, err := Candidates(context.Background(), testAccounts, "ps5", game, "igdb", "")
	if err != nil || len(igdb) != 2 || igdb[1].ExternalID != "2" || igdb[1].Name != "Final Fantasy VII Rebirth" ||
		igdb[1].Year != 2023 || strings.Join(igdb[1].Platforms, ",") != "PS5" || !strings.HasSuffix(igdb[1].CoverURL, "/t_cover_big/co2.jpg") {
		t.Fatalf("igdb : %+v, %v", igdb, err)
	}
	grid, err := Candidates(context.Background(), testAccounts, "ps5", game, "steamgriddb", "Final Fantasy")
	if err != nil || len(grid) != 2 || grid[0].ExternalID != "1" {
		t.Fatalf("steamgriddb : %+v, %v", grid, err)
	}
	if _, err := Candidates(context.Background(), nil, "ps5", game, "igdb", ""); !errors.Is(err, ErrServiceNotConfigured) {
		t.Fatalf("sans compte : %v", err)
	}
	if _, err := Candidates(context.Background(), testAccounts, "ps5", game, "libretro", ""); !errors.Is(err, ErrUnknownSource) {
		t.Fatalf("source sans candidats : %v", err)
	}
}

// LE JEU DÉSIGNÉ RESTE : la recherche par titre prendrait « Rebirth » ;
// l'administrateur désigne « Remake » (id 1), et un rafraîchissement (une
// forme ancienne de source.json) le relit par son identifiant. Une
// recherche sans identifiant l'oublie.
func TestAPinSurvivesARefresh(t *testing.T) {
	fakeSearch(t, "Final Fantasy VII Remake", "Final Fantasy VII Rebirth")
	root := t.TempDir()
	game := ps5Game(t, "Final Fantasy VII Rebirth")

	for _, source := range []string{"igdb", "steamgriddb"} {
		state, err := FetchSource(context.Background(), root, testAccounts, "ps5", game, "ref", source, "1")
		if err != nil || state.Match != "manual-pin" || state.ExternalID != "1" || state.Name != "Final Fantasy VII Remake" {
			t.Fatalf("%s désigné : %+v, %v", source, state, err)
		}
		sourceDir := filepath.Join(gameDirectory(root, "ps5", game, "ref"), source)
		old := readRemoteState(sourceDir, source)
		old.Format = 1
		if err := writeRemoteState(sourceDir, old); err != nil {
			t.Fatal(err)
		}
		kind := KindLogo
		if source == "igdb" {
			kind = KindCover
		}
		_, _ = Remote(context.Background(), remoteServices[source], testAccounts, filepath.Dir(sourceDir), "ps5", "Final Fantasy VII Rebirth", kind)
		if again := readRemoteState(sourceDir, source); again.Format != remoteFormat || again.Match != "manual-pin" || again.Name != "Final Fantasy VII Remake" {
			t.Fatalf("%s rafraîchi : %+v", source, again)
		}

		state, err = FetchSource(context.Background(), root, testAccounts, "ps5", game, "ref", source, "")
		if err != nil || state.Match != "title+platform" || state.Name != "Final Fantasy VII Rebirth" {
			t.Fatalf("%s recherché : %+v, %v", source, state, err)
		}
	}

	if _, err := FetchSource(context.Background(), root, testAccounts, "ps5", game, "ref", "igdb", "99"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("identifiant inconnu : %v", err)
	}
	if state := readRemoteState(filepath.Join(gameDirectory(root, "ps5", game, "ref"), "igdb"), "igdb"); state.Name != "Final Fantasy VII Rebirth" {
		t.Fatalf("un identifiant inconnu a changé l'état : %+v", state)
	}
	for _, id := range []string{"abc", "-1", "1; fields *", "../1"} {
		if _, err := FetchSource(context.Background(), root, testAccounts, "ps5", game, "ref", "igdb", id); !errors.Is(err, ErrInvalidExternalID) {
			t.Errorf("identifiant %q : %v", id, err)
		}
	}
	if _, err := FetchSource(context.Background(), root, nil, "ps5", game, "ref", "igdb", ""); !errors.Is(err, ErrServiceNotConfigured) {
		t.Fatalf("sans compte : %v", err)
	}
	if _, err := FetchSource(context.Background(), root, testAccounts, "ps5", game, "ref", "manual", ""); !errors.Is(err, ErrUnknownSource) {
		t.Fatalf("manual n'a pas de recherche : %v", err)
	}
}

var remoteServices = map[string]remoteService{"igdb": igdbService{}, "steamgriddb": steamGridDBService{}}

// OUBLIER UNE SOURCE : son dossier part ; une source inconnue ou un chemin
// sont refusés.
func TestDropSource(t *testing.T) {
	root := t.TempDir()
	game := ps5Game(t, "Final Fantasy VII Rebirth")
	gameDir := gameDirectory(root, "ps5", game, "ref")
	for _, source := range []string{"igdb", "steamgriddb", "libretro", "manual", "embedded"} {
		writeTestFile(t, filepath.Join(gameDir, source, "source.json"), "{}")
	}

	for _, source := range []string{"igdb", "steamgriddb", "libretro", "manual"} {
		if err := DropSource(root, "ps5", game, "ref", source); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(gameDir, source)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s resté", source)
		}
	}
	for _, source := range []string{"embedded", "../igdb", "", "libretro-database"} {
		if err := DropSource(root, "ps5", game, "ref", source); !errors.Is(err, ErrUnknownSource) {
			t.Errorf("%q : %v", source, err)
		}
	}
	if _, err := os.Stat(filepath.Join(gameDir, "embedded")); err != nil {
		t.Fatal("embedded supprimé")
	}
}

// LES DEUX RACINES : les métadonnées d'un jeu (image, réductions,
// source.json, texte manuel) sous la racine de sa source ; l'index de
// libretro-thumbnails, un cache, sous le dossier du serveur ; rien du jeu
// dans le dossier du serveur, rien du cache sur le disque.
func TestGameDataUnderItsSourceRootCachesUnderTheServer(t *testing.T) {
	fakeLibretro(t, map[string][]string{
		"Nintendo - Super Nintendo Entertainment System/Named_Boxarts": {"Super Metroid (Japan, USA) (En,Ja)"},
	})
	server, disk := t.TempDir(), t.TempDir()
	roots := Roots{Server: server, Game: disk}
	game := rom(t, "Super Metroid (Japan, USA) (En,Ja).sfc")

	path, err := Image(context.Background(), roots, nil, "snes", game, "ref", KindCover, 256)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(disk, "metadata", "snes")+string(filepath.Separator)) {
		t.Fatalf("image rangée en %s", path)
	}
	if err := SetManualText(roots.Game, "snes", game, "ref", rawChanges(t, `{"title":"Metroid"}`)); err != nil {
		t.Fatal(err)
	}
	if sheets, _ := filepath.Glob(filepath.Join(disk, "metadata", "snes", "*", "manual", "source.json")); len(sheets) != 1 {
		t.Fatalf("texte manuel : %v", sheets)
	}
	if _, err := os.Stat(filepath.Join(server, "metadata")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("des métadonnées du jeu dans le dossier du serveur : %v", err)
	}
	if indexes, _ := filepath.Glob(filepath.Join(server, "cache", "metadata", "sources", "libretro", "*", "Named_Boxarts.txt")); len(indexes) != 1 {
		t.Fatalf("index de libretro : %v", indexes)
	}
	if _, err := os.Stat(filepath.Join(disk, "cache")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("un cache sur le disque : %v", err)
	}
}

// RÉINITIALISER LA FICHE (le porteur, 2026-11-14 : « Le bouton réinitialiser permet de tout remettre comme avant ») :
// le titre corrigé et les images remplacées partent d'un coup, la fiche revient à ce que donnent les services, et rien
// n'y dit qu'elle avait été corrigée.
func TestResettingTheSheetBringsBackTheServices(t *testing.T) {
	fakeServices(t)
	root := t.TempDir()
	game := ps5Game(t, "Final Fantasy VII Rebirth")
	accounts := Accounts{"igdb": testAccounts["igdb"]}
	var picture bytes.Buffer
	_ = png.Encode(&picture, transparentIcon(300, 400))
	if err := SetManualImage(root, "ps5", game, "ref", KindCover, picture.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := SetManualText(root, "ps5", game, "ref", rawChanges(t, `{"title":"FF VII Rebirth","releaseDate":"2024"}`)); err != nil {
		t.Fatal(err)
	}
	if info := Info(context.Background(), onRoot(root), accounts, "ps5", game, "ref"); info.Title != "FF VII Rebirth" || info.ReleaseDate != "2024" {
		t.Fatalf("corrigée : %+v", info)
	}

	if err := DropSource(root, "ps5", game, "ref", "manual"); err != nil {
		t.Fatal(err)
	}
	info := Info(context.Background(), onRoot(root), accounts, "ps5", game, "ref")
	if info.Title != "" || info.ReleaseDate != "2024-02-29" || info.Sources["releaseDate"] != "igdb" {
		t.Fatalf("réinitialisée : %+v", info)
	}
	if path, err := Image(context.Background(), onRoot(root), accounts, "ps5", game, "ref", KindCover, 256); err != nil || !strings.Contains(path, "igdb") {
		t.Fatalf("la jaquette après : %s, %v", path, err)
	}
	t.Cleanup(func() { forgetSeries("ps5", "ref") })
}
