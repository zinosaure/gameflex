package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gameflex/storage"
)

// lentLibrary — une bibliothèque PS3 : « Jeu (Europe).iso » et « Jeu - copie.iso » portent le même contenu (un même jeu
// sous deux noms : le même gameid), « Autre.iso » un autre. Rend la racine, les jeux, et le gameid du premier, lu sur un
// fichier à part du même contenu : les trois jeux n'ont pas encore le leur en mémoire.
func lentLibrary(t *testing.T) (string, []LentGame, string) {
	t.Helper()
	root := t.TempDir()
	games := []LentGame{}
	for name, content := range map[string]string{"Jeu (Europe).iso": "le même jeu", "Jeu - copie.iso": "le même jeu", "Autre.iso": "un autre"} {
		path := filepath.Join(root, "roms", "ps3", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		games = append(games, LentGame{Reference: storage.RomID("ps3", name), Title: strings.TrimSuffix(name, ".iso"), Path: path, Roots: Roots{Server: root, Game: root}})
	}
	// Dans l'ordre du nom, comme un catalogue.
	for left := range games {
		for right := left + 1; right < len(games); right++ {
			if games[right].Title < games[left].Title {
				games[left], games[right] = games[right], games[left]
			}
		}
	}
	twin := filepath.Join(t.TempDir(), "jumeau.iso")
	if err := os.WriteFile(twin, []byte("le même jeu"), 0o644); err != nil {
		t.Fatal(err)
	}

	return root, games, GameKey(twin, "jumeau")
}

func titlesOf(items []PeerGame) string {
	described := []string{}
	for _, item := range items {
		described = append(described, item.Title+"/"+item.Match)
	}

	return strings.Join(described, " ")
}

// LA RECHERCHE DANS MES FICHES (le pair, chantier 57, P3) : par gameid d'abord — un même jeu sous deux noms —, puis par
// titre ; un gameid encore inconnu se dit (`ready: false`), et la recherche suivante le trouve ; avec une sorte
// d'image, seulement les jeux qui l'ont, avec sa taille.
func TestALentSearchFindsTheSameGameThenTheTitle(t *testing.T) {
	fakeLibretro(t, nil)
	root, games, gameID := lentLibrary(t)

	first := SearchLent(context.Background(), LentSearch{ConsoleID: "ps3", GameID: gameID, Query: "autre", Games: games})
	if first.Ready || titlesOf(first.Items) != "Autre/title" {
		t.Fatalf("avant que les gameid soient lus : ready=%v, %s", first.Ready, titlesOf(first.Items))
	}
	for _, game := range games {
		GameKey(game.Path, game.Reference)
	}
	second := SearchLent(context.Background(), LentSearch{ConsoleID: "ps3", GameID: gameID, Query: "autre", Games: games})
	if !second.Ready || titlesOf(second.Items) != "Jeu (Europe)/gameId Jeu - copie/gameId Autre/title" || second.Items[0].GameID != gameID {
		t.Fatalf("une fois lus : ready=%v, %s", second.Ready, titlesOf(second.Items))
	}
	if byTitle := SearchLent(context.Background(), LentSearch{ConsoleID: "ps3", Query: "JEU (USA)", Games: games}); titlesOf(byTitle.Items) != "Jeu (Europe)/title Jeu - copie/title" {
		t.Fatalf("par titre, sans casse ni mentions : %s", titlesOf(byTitle.Items))
	}

	var cover bytes.Buffer
	_ = png.Encode(&cover, image.NewGray(image.Rect(0, 0, 300, 400)))
	if err := SetManualImage(root, "ps3", games[1].Path, games[1].Reference, KindCover, cover.Bytes()); err != nil {
		t.Fatal(err)
	}
	// Les deux noms du même jeu partagent son dossier : la jaquette posée sur l'un est aussi celle de l'autre.
	withCover := SearchLent(context.Background(), LentSearch{ConsoleID: "ps3", GameID: gameID, Query: "autre", Kind: KindCover, Games: games})
	if titlesOf(withCover.Items) != "Jeu (Europe)/gameId Jeu - copie/gameId" || withCover.Items[0].Width != 300 || withCover.Items[1].Height != 400 {
		t.Fatalf("seulement les jeux qui ont la jaquette, avec sa taille : %+v", withCover.Items)
	}
	if none := SearchLent(context.Background(), LentSearch{ConsoleID: "ps3", Query: "inconnu", Games: games}); len(none.Items) != 0 || none.Images != nil || none.Candidates != nil {
		t.Fatalf("un titre que je n'ai pas, sans partager mes fournisseurs : %+v", none)
	}
}

// MES FOURNISSEURS NE SE PRÊTENT QU'AVEC L'OPTION DU JETON (`#719`), et pour un titre que je n'ai pas : leurs images
// (IGDB, SteamGridDB, jamais libretro), leurs candidats, et le jeu de l'un d'eux relu par mon compte — ses images chez
// l'hôte du fournisseur, aucune clé.
func TestALentSearchLendsProvidersOnlyWhenAllowed(t *testing.T) {
	fakeImageProviders(t)
	fakeLibretro(t, nil)
	_, games, _ := lentLibrary(t)
	asked := LentSearch{ConsoleID: "snes", Query: "Chrono Trigger", Kind: KindCover, Accounts: testAccounts}

	if closed := SearchLent(context.Background(), asked); len(closed.Images) != 0 {
		t.Fatalf("sans l'option, mes fournisseurs ont répondu : %+v", closed.Images)
	}
	asked.LendProviders = true
	lent := SearchLent(context.Background(), asked)
	sources := map[string]int{}
	for _, item := range lent.Images {
		sources[item.Source]++
		if !allowedImage(item.Source, item.URL) || strings.Contains(item.URL, "cle") {
			t.Errorf("une image prêtée hors de l'hôte de sa source, ou avec une clé : %+v", item)
		}
	}
	if sources[sourceIGDB] == 0 || sources[sourceSteamGridDB] == 0 || sources[sourceLibretro] != 0 {
		t.Fatalf("les images de mes fournisseurs, par source : %v", sources)
	}
	asked.Kind = ""
	candidates := SearchLent(context.Background(), asked).Candidates
	if len(candidates) == 0 || candidates[0].Provider != sourceIGDB || candidates[0].ExternalID != "1" {
		t.Fatalf("les candidats de mes fournisseurs : %+v", candidates)
	}
	if mine := SearchLent(context.Background(), LentSearch{ConsoleID: "ps3", Query: "jeu", Games: games, Accounts: testAccounts, LendProviders: true}); len(mine.Candidates) != 0 {
		t.Fatalf("un titre que j'ai : mes fournisseurs n'avaient pas à répondre : %+v", mine.Candidates)
	}
	// Le même jeu trouvé par son gameid, mais un autre titre cherché : celui-là, je ne l'ai pas.
	for _, game := range games {
		GameKey(game.Path, game.Reference)
	}
	sameGame, _ := KnownGameKey(games[1].Path, games[1].Reference)
	other := SearchLent(context.Background(), LentSearch{ConsoleID: "ps3", GameID: sameGame, Query: "Chrono Trigger", Games: games, Accounts: testAccounts, LendProviders: true})
	if len(other.Items) != 2 || len(other.Candidates) == 0 {
		t.Fatalf("le même jeu par son gameid, et les candidats d'un titre que je n'ai pas : %d jeux, %d candidats", len(other.Items), len(other.Candidates))
	}

	sheet, err := LendProviderGame(context.Background(), testAccounts, "snes", sourceIGDB, "1")
	if err != nil || sheet.Name != "Chrono Trigger" || sheet.Images[KindCover] == "" || !allowedImage(sourceIGDB, sheet.Images[KindCover]) {
		t.Fatalf("le jeu de mon fournisseur = %+v (%v)", sheet, err)
	}
	if _, err := LendProviderGame(context.Background(), nil, "snes", sourceIGDB, "1"); !errors.Is(err, ErrServiceNotConfigured) {
		t.Errorf("sans compte : %v", err)
	}
	if _, err := LendProviderGame(context.Background(), testAccounts, "snes", sourceIGDB, "../1"); !errors.Is(err, ErrInvalidExternalID) {
		t.Errorf("un identifiant inventé : %v", err)
	}
}

// fakePeer — un serveur fédéré, en mémoire : sa recherche, la fiche et les images de son jeu « lent-1 », le jeu de son
// fournisseur ; `err` le fait refuser (fs.ErrPermission), ne pas prêter (fs.ErrNotExist) ou se taire.
type fakePeer struct {
	answer      PeerAnswer
	sheet       GameInfo
	images      map[string][]byte
	provider    ProviderSheet
	backgrounds BackgroundSlots
	err         error
	asked       []string
}

func (peer *fakePeer) SearchMetadata(ctx context.Context, consoleID string, query url.Values, into any) error {
	peer.asked = append(peer.asked, "search "+consoleID+" "+query.Encode())
	if peer.err != nil {
		return peer.err
	}

	return roundTrip(peer.answer, into)
}

func (peer *fakePeer) GameMetadata(ctx context.Context, consoleID, reference string, into any) error {
	peer.asked = append(peer.asked, "sheet "+reference)
	if peer.err != nil {
		return peer.err
	}
	if reference != "lent-1" {
		return fs.ErrNotExist
	}

	return roundTrip(peer.sheet, into)
}

func (peer *fakePeer) GameImage(ctx context.Context, consoleID, reference, kind string, width int, limit int64) ([]byte, error) {
	peer.asked = append(peer.asked, fmt.Sprintf("image %s %s %d", reference, kind, width))
	if peer.err != nil {
		return nil, peer.err
	}
	if content, found := peer.images[kind]; found && reference == "lent-1" {
		return content, nil
	}

	return nil, fs.ErrNotExist
}

func (peer *fakePeer) ProviderGame(ctx context.Context, consoleID, provider, externalID string, into any) error {
	peer.asked = append(peer.asked, "provider "+provider+" "+externalID)
	if peer.err != nil {
		return peer.err
	}

	return roundTrip(peer.provider, into)
}

func (peer *fakePeer) ConsoleBackgrounds(ctx context.Context, consoleID string, into any) error {
	peer.asked = append(peer.asked, "backgrounds "+consoleID)
	if peer.err != nil {
		return peer.err
	}

	return roundTrip(peer.backgrounds, into)
}

// roundTrip — la réponse, passée par JSON comme sur le réseau.
func roundTrip(value, into any) error {
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, into)
}

func pngSized(width, height int) []byte {
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewGray(image.Rect(0, 0, width, height)))

	return picture.Bytes()
}

// UN PAIR, SOURCE DE « CHOISIR UNE IMAGE » : ses jeux après les fournisseurs, leur vignette par le relais du kernel
// (`srv<id>:<ref>`, `?server=`) ; ses fournisseurs prêtés à la suite, leur adresse chez leur hôte ; chaque pair dit son
// état — refusé, ne prête pas (une version plus ancienne), muet —, et un pair qui lit encore ses gameid le dit.
func TestAPeerIsASourceOfTheImageSearch(t *testing.T) {
	fakeLibretro(t, nil)
	lending := &fakePeer{answer: PeerAnswer{Ready: false, Items: []PeerGame{{Reference: "lent-1", Title: "Jeu chez l'ami", Match: "gameId", Width: 600, Height: 800}},
		Images: []ImageResult{
			{Source: sourceIGDB, URL: "https://images.igdb.com/igdb/image/upload/t_cover_big_2x/co9.jpg", ThumbURL: "https://images.igdb.com/igdb/image/upload/t_cover_big/co9.jpg", Game: "Jeu"},
			{Source: sourceIGDB, URL: "https://ailleurs.invalid/x.jpg", ThumbURL: "https://ailleurs.invalid/x.jpg", Game: "Piège"},
			{Source: sourceLibretro, URL: "https://thumbnails.libretro.com/x.png", ThumbURL: "https://thumbnails.libretro.com/x.png", Game: "Pas de libretro prêté"},
		}}}
	peers := []Peer{
		{ID: 3, Name: "ami", Client: lending},
		{ID: 4, Name: "refuse", Client: &fakePeer{err: errors.Join(errors.New("401"), fs.ErrPermission)}},
		{ID: 5, Name: "ancien", Client: &fakePeer{err: errors.Join(errors.New("405"), fs.ErrNotExist)}},
		{ID: 6, Name: "muet", Client: &fakePeer{err: errors.New("connection refused")}},
	}

	found, err := SearchGameImages(context.Background(), t.TempDir(), nil, "ps3", KindCover, "Jeu", "crc-12345678", peers)
	if err != nil {
		t.Fatal(err)
	}
	if lending.asked[0] != "search ps3 gameId=crc-12345678&kind=cover&q=Jeu" {
		t.Fatalf("la question posée au pair : %v", lending.asked)
	}
	if len(found.Items) != 2 {
		t.Fatalf("les images proposées : %+v", found.Items)
	}
	lent, provided := found.Items[0], found.Items[1]
	if lent.Source != sourceServer || lent.Server != 3 || lent.ServerName != "ami" || lent.Reference != "lent-1" || lent.URL != "" || lent.Width != 600 ||
		lent.ThumbPath != "/api/v1/retrogaming/ps3/roms/srv3:lent-1/images/cover?server=3&w=256" {
		t.Fatalf("l'image d'un jeu du pair : %+v", lent)
	}
	if provided.Source != sourceIGDB || provided.Server != 3 || provided.ThumbPath != "" || provided.URL == "" {
		t.Fatalf("l'image d'un fournisseur prêtée par le pair : %+v", provided)
	}
	states := []string{}
	for _, state := range found.Sources {
		states = append(states, fmt.Sprintf("%s%d=%s/%s/%v", state.Source, state.Server, state.State, state.Error, state.Reading))
	}
	want := "igdb0=noAccount//false steamgriddb0=noAccount//false libretro0=ok//false server3=ok//true server4=error/refused/false server5=error/notLending/false server6=error/unreachable/false"
	if strings.Join(states, " ") != want {
		t.Fatalf("les fournisseurs d'abord, puis l'état de chaque pair :\n%s\nattendu\n%s", strings.Join(states, " "), want)
	}
}

// « CE N'EST PAS LE BON JEU ? » CHEZ UN PAIR, PUIS LA COPIE : ses candidats (sa jaquette par le relais ; un jeu de son
// fournisseur en « igdb:<id> ») ; désigner l'un d'eux copie sa fiche dans la source `server` — le texte, la source
// d'origine de chaque champ, ses images — ; elle passe devant les services et derrière la correction, et reste quand
// le pair s'éteint ; « Oublier » la retire.
func TestAPeerSheetIsCopiedAndOutlivesThePeer(t *testing.T) {
	fakeLibretro(t, nil)
	root := t.TempDir()
	game := filepath.Join(root, "roms", "ps3", "Mon jeu.iso")
	if err := os.MkdirAll(filepath.Dir(game), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(game, []byte("mon jeu"), 0o644); err != nil {
		t.Fatal(err)
	}
	reference := storage.RomID("ps3", "Mon jeu.iso")
	roots := Roots{Server: root, Game: root}
	friend := &fakePeer{
		answer: PeerAnswer{Ready: true, Items: []PeerGame{{Reference: "lent-1", Title: "Jeu chez l'ami", Match: "title", ReleaseDate: "2006-11-11"}},
			Candidates: []Candidate{{ExternalID: "77", Name: "Chez IGDB", Provider: sourceIGDB, CoverURL: "https://images.igdb.com/igdb/image/upload/t_cover_big/co7.jpg"},
				{ExternalID: "x", Name: "Piège", Provider: sourceIGDB}}},
		sheet: GameInfo{GameID: "BLES00001", Title: "Le titre corrigé chez lui", ReleaseDate: "2006", Genres: []string{"Action"}, Publishers: []string{"Sony"},
			Series: "Saga", Sources: map[string]string{"genres": "libretro-database", "series": "igdb"}},
		images: map[string][]byte{KindCover: pngSized(300, 400), KindLogo: []byte("pas une image")},
	}
	peer := Peer{ID: 3, Name: "ami", Link: "https://ami.example", Client: friend}

	candidates, err := PeerCandidates(context.Background(), peer, "ps3", game, reference, "")
	if err != nil || len(candidates) != 2 {
		t.Fatalf("les candidats du pair = %+v (%v)", candidates, err)
	}
	if candidates[0].ExternalID != "lent-1" || candidates[0].Year != 2006 || candidates[0].CoverPath != "/api/v1/retrogaming/ps3/roms/srv3:lent-1/images/cover?server=3&w=256" ||
		candidates[1].ExternalID != "igdb:77" || candidates[1].CoverPath != "" {
		t.Fatalf("les candidats du pair = %+v", candidates)
	}

	state, err := FetchPeerSource(context.Background(), root, peer, "ps3", game, reference, "lent-1", "Jeu chez l'ami")
	if err != nil || state.Source != sourceServer || state.Match != matchManualPin || state.ExternalID != "lent-1" || state.Name != "Jeu chez l'ami" {
		t.Fatalf("la copie = %+v (%v)", state, err)
	}
	info := Info(context.Background(), roots, nil, "ps3", game, reference)
	if info.Title != "Le titre corrigé chez lui" || info.ReleaseDate != "2006" || info.Genres[0] != "Action" || info.Series != "Saga" ||
		info.Sources["genres"] != "libretro-database" || info.Sources["series"] != "igdb" || info.Sources["releaseDate"] != "" {
		t.Fatalf("la fiche après la copie : %+v", info)
	}
	cover, err := Image(context.Background(), roots, nil, "ps3", game, reference, KindCover, 256)
	if err != nil || !strings.Contains(cover, string(filepath.Separator)+sourceServer+string(filepath.Separator)) {
		t.Fatalf("la jaquette après la copie : %s (%v)", cover, err)
	}
	if _, err := Image(context.Background(), roots, nil, "ps3", game, reference, KindLogo, 256); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("un logo qui n'est pas une image s'est copié : %v", err)
	}

	friend.err = errors.New("éteint")
	if still := Info(context.Background(), roots, nil, "ps3", game, reference); still.Genres == nil || still.Genres[0] != "Action" {
		t.Fatalf("le pair éteint, la copie reste : %+v", still)
	}
	if err := SetManualText(root, "ps3", game, reference, map[string]json.RawMessage{"genres": json.RawMessage(`["Course"]`)}); err != nil {
		t.Fatal(err)
	}
	if corrected := Info(context.Background(), roots, nil, "ps3", game, reference); corrected.Genres[0] != "Course" || corrected.Series != "Saga" {
		t.Fatalf("la correction passe devant la copie : %+v", corrected)
	}
	if err := DropSource(root, "ps3", game, reference, sourceServer); err != nil {
		t.Fatal(err)
	}
	if forgotten := Info(context.Background(), roots, nil, "ps3", game, reference); forgotten.Series != "" || forgotten.ReleaseDate != "" {
		t.Fatalf("la copie oubliée : %+v", forgotten)
	}

	// Sans identifiant, la copie se refait chez lui par son premier jeu trouvé ; un jeu de son fournisseur se copie
	// avec ses images, prises chez l'hôte d'images du fournisseur.
	friend.err = nil
	if again, err := FetchPeerSource(context.Background(), root, peer, "ps3", game, reference, "", ""); err != nil || again.ExternalID != "lent-1" || again.Name != "Jeu chez l'ami" {
		t.Fatalf("la copie refaite = %+v (%v)", again, err)
	}
	friend.provider = ProviderSheet{Provider: sourceIGDB, ExternalID: "77", Name: "Chez IGDB", Genres: []string{"Course"},
		Images: map[string]string{KindCover: "https://images.igdb.com/igdb/image/upload/t_cover_big_2x/co7.jpg"}}
	previous := fetchProviderImage
	fetchProviderImage = func(ctx context.Context, source, address string) ([]byte, error) {
		if source != sourceIGDB || !allowedImage(source, address) {
			return nil, ErrImageNotAllowed
		}

		return pngSized(264, 374), nil
	}
	t.Cleanup(func() { fetchProviderImage = previous })
	if provided, err := FetchPeerSource(context.Background(), root, peer, "ps3", game, reference, "igdb:77", ""); err != nil || provided.Name != "Chez IGDB" {
		t.Fatalf("la copie d'un jeu de son fournisseur = %+v (%v)", provided, err)
	}
	copied := readServerCopy(gameDirectory(root, "ps3", game, reference))
	if copied.Provider != sourceIGDB || copied.Sources["genres"] != sourceIGDB || copied.Images[KindCover] == "" || copied.Reference != "" {
		t.Fatalf("la copie d'un jeu de son fournisseur : %+v", copied)
	}

	// Un pair qui refuse, qui ne prête pas, qui ne connaît pas le jeu désigné.
	friend.err = errors.Join(errors.New("403"), fs.ErrPermission)
	if _, err := FetchPeerSource(context.Background(), root, peer, "ps3", game, reference, "lent-1", ""); !errors.Is(err, ErrPeerDenied) || Refusal(err).Status != 403 {
		t.Errorf("un pair qui refuse : %v", err)
	}
	friend.err = nil
	if _, err := FetchPeerSource(context.Background(), root, peer, "ps3", game, reference, "inconnu", ""); !errors.Is(err, ErrPeerNotLending) || Refusal(err).Code != "peer_not_lending" {
		t.Errorf("un jeu qu'il n'a pas : %v", err)
	}
	if _, err := FetchPeerImage(context.Background(), peer, "ps3", "lent-1", KindCover); err != nil {
		t.Errorf("une image choisie chez lui : %v", err)
	}
}

// LE CORPS D'UNE IMAGE CHOISIE : une image d'un fournisseur (`{source, url}`), ou celle d'un jeu d'un pair (`{source:
// "server", server, reference}`, jamais d'adresse) ; le fond d'une console ne prend pas les images des pairs.
func TestAChosenImageComesFromAProviderOrAPeer(t *testing.T) {
	for body, valid := range map[string]bool{
		`{"source":"igdb","url":"https://images.igdb.com/x.jpg"}`: true,
		`{"source":"server","server":3,"reference":"lent-1"}`:     true,
		`{"source":"server","url":"https://ailleurs.invalid/x"}`:  false,
		`{"source":"server","server":3}`:                          false,
		`{"source":"igdb"}`:                                       false,
		`{"url":"https://images.igdb.com/x.jpg"}`:                 false,
	} {
		chosen, err := DecodeChosenImage(strings.NewReader(body))
		if (err == nil) != valid {
			t.Errorf("%s : %+v, %v", body, chosen, err)
		}
	}
	fetched := false
	_, err := ReadChosenImage(context.Background(), strings.NewReader(`{"source":"server","server":3,"reference":"lent-1"}`),
		func(context.Context, string, string) ([]byte, error) { fetched = true; return nil, nil })
	if !errors.Is(err, ErrInvalidChoice) || fetched {
		t.Errorf("le fond d'une console a pris l'image d'un pair : %v", err)
	}
}
