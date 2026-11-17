package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gameflex/consoles"
)

// virtualBoyCovers — un site de vignettes fabriqué qui a les jaquettes des
// jeux du fond de la Virtual Boy, sauf `missing`.
func virtualBoyCovers(t *testing.T, missing string) {
	t.Helper()
	names := []string{}
	for _, game := range consoleBackgroundGames["vb"] {
		if game != missing {
			names = append(names, game)
		}
	}
	fakeLibretro(t, map[string][]string{"Nintendo - Virtual Boy/Named_Boxarts": names})
}

func backgroundFiles(t *testing.T, directory string) []string {
	t.Helper()
	entries, _ := os.ReadDir(directory)
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

// LE REMPLISSAGE PAR DÉFAUT : une jaquette par jeu de la liste, 01.jpg,
// 02.jpg… dans son ordre ; un jeu absent du site laisse son numéro vide ;
// `.seeded` dit ce qui est venu d'où.
func TestBackgroundsAreSeededFromTheList(t *testing.T) {
	missing := consoleBackgroundGames["vb"][2]
	virtualBoyCovers(t, missing)
	root := t.TempDir()

	slots, err := ConsoleBackgrounds(root, nil, "vb", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"01.jpg", "02.jpg", "04.jpg", "05.jpg", "06.jpg", "07.jpg", "08.jpg"}
	if names := filledSlots(slots); strings.Join(names, ",") != strings.Join(want, ",") || slots.Slots[2].Name != "" || len(slots.Extra) != 0 {
		t.Fatalf("fond : %+v, attendu %v et l'emplacement 3 libre", slots, want)
	}
	content, err := os.ReadFile(filepath.Join(ConsoleBackgroundDir(root, "vb"), backgroundMarker))
	if err != nil {
		t.Fatalf("pas de .seeded : %v", err)
	}
	var seed backgroundSeed
	if err := json.Unmarshal(content, &seed); err != nil || len(seed.Games) != 8 ||
		seed.Games[0].Source != "libretro" || seed.Games[0].Result != "found" || seed.Games[2].Result != "notFound" {
		t.Fatalf(".seeded : %s", content)
	}
	if picture, err := jpeg.Decode(bytes.NewReader(mustRead(t, filepath.Join(ConsoleBackgroundDir(root, "vb"), "01.jpg")))); err != nil || picture.Bounds().Dx() != 600 {
		t.Fatalf("01.jpg : %v", err)
	}
}

// UN DOSSIER VIDÉ EXPRÈS RESTE VIDE : le marqueur empêche de le remplir de
// nouveau, sans rien demander au site.
func TestTheMarkerPreventsRefill(t *testing.T) {
	virtualBoyCovers(t, "")
	root := t.TempDir()
	if _, err := ConsoleBackgrounds(root, nil, "vb", 10*time.Second); err != nil {
		t.Fatal(err)
	}
	directory := ConsoleBackgroundDir(root, "vb")
	for _, name := range listBackgrounds(directory) {
		if err := os.Remove(filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
	}

	requests := fakeLibretro(t, map[string][]string{"Nintendo - Virtual Boy/Named_Boxarts": consoleBackgroundGames["vb"]})
	slots, err := ConsoleBackgrounds(root, nil, "vb", 10*time.Second)
	if err != nil || len(filledSlots(slots)) != 0 || requests.Load() != 0 {
		t.Fatalf("après le vidage : %+v, %v, %d requêtes", slots, err, requests.Load())
	}
}

// LE FICHIER DU JOUEUR GAGNE : déposé avant le remplissage, son numéro
// n'est pas téléchargé et lui n'est pas touché ; remplacé après, c'est lui
// qui se sert.
func TestAUserFileWins(t *testing.T) {
	virtualBoyCovers(t, "")
	root := t.TempDir()
	directory := ConsoleBackgroundDir(root, "vb")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	userCover := solidJPEG(t, 300, 400, color.RGBA{200, 0, 0, 255})
	if err := os.WriteFile(filepath.Join(directory, "02.jpg"), userCover, 0o644); err != nil {
		t.Fatal(err)
	}

	slots, err := ConsoleBackgrounds(root, nil, "vb", 10*time.Second)
	if err != nil || len(filledSlots(slots)) != 8 {
		t.Fatalf("fond : %+v, %v", slots, err)
	}
	if !bytes.Equal(mustRead(t, filepath.Join(directory, "02.jpg")), userCover) {
		t.Fatal("02.jpg du joueur réécrit")
	}

	// Remplacé après coup, plus récent que sa réduction : la réduction se refait.
	path, err := ConsoleBackground(root, "vb", "01.jpg", 256)
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.WriteFile(filepath.Join(directory, "01.jpg"), userCover, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(filepath.Join(directory, "01.jpg"), future, future)
	path, err = ConsoleBackground(root, "vb", "01.jpg", 256)
	if err != nil {
		t.Fatal(err)
	}
	picture, err := jpeg.Decode(bytes.NewReader(mustRead(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	if red, green, _, _ := picture.At(10, 10).RGBA(); red < 0xb000 || green > 0x2000 {
		t.Fatalf("la réduction ne montre pas l'image du joueur : %v", picture.At(10, 10))
	}
}

// LES NOMS : rien ne sort du dossier, rien d'autre qu'une image ne se sert,
// une console inconnue est une erreur.
func TestBackgroundNameValidation(t *testing.T) {
	root := t.TempDir()
	directory := ConsoleBackgroundDir(root, "vb")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(directory, "01.jpg"), solidJPEG(t, 40, 40, color.RGBA{0, 0, 200, 255}), 0o644)
	_ = os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "metadata", "vb", "secret.jpg"), solidJPEG(t, 40, 40, color.RGBA{}), 0o644)

	for _, name := range []string{"../secret.jpg", "..", ".seeded", "notes.txt", "a/01.jpg", "/etc/passwd", "01.jpg/", "..jpg", "missing.jpg", ""} {
		if _, err := ConsoleBackground(root, "vb", name, 256); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%q : %v, attendu os.ErrNotExist", name, err)
		}
	}
	if _, err := ConsoleBackground(root, "vb", "01.jpg", 256); err != nil {
		t.Fatalf("01.jpg : %v", err)
	}
	if _, err := ConsoleBackground(root, "playstation9", "01.jpg", 256); !errors.Is(err, ErrUnknownConsole) {
		t.Fatalf("console inconnue : %v", err)
	}
	if _, err := ConsoleBackgrounds(root, nil, "playstation9", time.Second); !errors.Is(err, ErrUnknownConsole) {
		t.Fatalf("liste d'une console inconnue : %v", err)
	}
}

// LES RÉDUCTIONS VONT DANS LE CACHE : le dossier du joueur ne garde que ses
// images et le marqueur.
func TestReductionsLiveOutsideTheUserFolder(t *testing.T) {
	virtualBoyCovers(t, "")
	root := t.TempDir()
	if _, err := ConsoleBackgrounds(root, nil, "vb", 10*time.Second); err != nil {
		t.Fatal(err)
	}
	directory := ConsoleBackgroundDir(root, "vb")
	before := backgroundFiles(t, directory)

	path, err := ConsoleBackground(root, "vb", "03.jpg", 256)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(root, "cache", "metadata", "console-backgrounds", "vb")+string(filepath.Separator)) {
		t.Fatalf("réduction rangée en %s", path)
	}
	if picture, err := jpeg.Decode(bytes.NewReader(mustRead(t, path))); err != nil || picture.Bounds().Dx() != 256 {
		t.Fatalf("réduction : %v", err)
	}
	if after := backgroundFiles(t, directory); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Fatalf("dossier du joueur changé : %v → %v", before, after)
	}
}

// SANS SOURCE, PAS DE MARQUEUR : la PS5 n'a pas de dossier chez libretro ;
// sans compte IGDB ni SteamGridDB, rien n'est tenté, et le remplissage se
// fera quand un compte sera saisi.
func TestAConsoleWithoutSourceIsNotMarked(t *testing.T) {
	root := t.TempDir()
	slots, err := ConsoleBackgrounds(root, nil, "ps5", 5*time.Second)
	if err != nil || len(filledSlots(slots)) != 0 || len(slots.Slots) != BackgroundSlotCount {
		t.Fatalf("ps5 : %+v, %v", slots, err)
	}
	if _, err := os.Stat(filepath.Join(ConsoleBackgroundDir(root, "ps5"), backgroundMarker)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marqueur posé sans source : %v", err)
	}
}

// UNE CONSOLE SANS AUCUNE IMAGE ICI MONTRE CELLES DU PREMIER SERVEUR FÉDÉRÉ QUI EN PRÊTE (`#740`) : l'annuaire dans
// son ordre — une version plus ancienne (sa route absente), un serveur muet, un serveur sans image laissent la place ;
// le premier qui en prête est seul interrogé, ses noms hors de leur emplacement et ses images en trop ne viennent pas,
// et rien ne s'écrit ici. Une console qui a une image à elle ne lit même pas l'annuaire.
func TestAConsoleWithoutImagesShowsTheFirstLendingServer(t *testing.T) {
	root := t.TempDir()
	lending := &fakePeer{backgrounds: BackgroundSlots{
		Slots: []BackgroundSlot{{Slot: 1, Name: "01.jpg"}, {Slot: 2, Name: "../02.jpg"}, {Slot: 3, Name: "07.jpg"}, {Slot: 4, Name: "04.png"}, {Slot: 9, Name: "09.jpg"}},
		Extra: []string{"ff9.png"},
	}}
	after := &fakePeer{backgrounds: BackgroundSlots{Slots: []BackgroundSlot{{Slot: 1, Name: "01.jpg"}}}}
	peers := []Peer{
		{ID: 4, Name: "ancien", Client: &fakePeer{err: errors.Join(errors.New("405"), fs.ErrNotExist)}},
		{ID: 5, Name: "muet", Client: &fakePeer{err: errors.New("connection refused")}},
		{ID: 6, Name: "vide", Client: &fakePeer{backgrounds: backgroundSlotsOf(nil)}},
		{ID: 3, Name: "ami", Client: lending},
		{ID: 7, Name: "après", Client: after},
	}
	reads := 0
	directory := func() []Peer {
		reads++

		return peers
	}

	slots, err := ShownBackgrounds(context.Background(), root, nil, "ps5", time.Second, directory)
	if err != nil || slots.Server != 3 || slots.ServerName != "ami" || strings.Join(filledSlots(slots), ",") != "01.jpg,04.png" ||
		len(slots.Slots) != BackgroundSlotCount || len(slots.Extra) != 0 {
		t.Fatalf("le fond prêté = %+v, %v ; attendu celui de « ami » (01.jpg, 04.png)", slots, err)
	}
	if len(after.asked) != 0 {
		t.Fatalf("le serveur suivant a été interrogé : %v", after.asked)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Fatalf("montrer le fond d'un serveur fédéré a écrit ici : %v", entries)
	}

	none, err := ShownBackgrounds(context.Background(), root, nil, "ps5", time.Second, func() []Peer { return peers[:3] })
	if err != nil || none.Server != 0 || len(filledSlots(none)) != 0 || len(none.Slots) != BackgroundSlotCount {
		t.Fatalf("aucun serveur ne prête : %+v, %v ; attendu le fond d'ici, vide", none, err)
	}

	if _, err := SetConsoleBackground(root, "ps5", 2, solidJPEG(t, 640, 360, color.RGBA{R: 200, A: 255})); err != nil {
		t.Fatal(err)
	}
	reads = 0
	own, err := ShownBackgrounds(context.Background(), root, nil, "ps5", time.Second, directory)
	if err != nil || own.Server != 0 || strings.Join(filledSlots(own), ",") != "02.jpg" || reads != 0 {
		t.Fatalf("une console qui a une image à elle = %+v, %v, l'annuaire lu %d fois ; attendu la sienne, sans lui", own, err, reads)
	}
	if _, err := ShownBackgrounds(context.Background(), root, nil, "playstation9", time.Second, directory); !errors.Is(err, ErrUnknownConsole) {
		t.Fatalf("une console inconnue : %v", err)
	}
}

// LA LISTE NE NOMME QUE DES CONSOLES DU REGISTRE.
func TestBackgroundListsNameRegistryConsoles(t *testing.T) {
	for consoleID, games := range consoleBackgroundGames {
		if !consoles.Exists(consoleID) {
			t.Errorf("%s n'est pas au registre", consoleID)
		}
		if len(games) == 0 {
			t.Errorf("%s : liste vide", consoleID)
		}
		// Un jeu par emplacement (remarque e du porteur) : au-delà, il serait en trop.
		if len(games) > BackgroundSlotCount {
			t.Errorf("%s : %d jeux pour %d emplacements", consoleID, len(games), BackgroundSlotCount)
		}
	}
	for alias, target := range consoleBackgroundAliases {
		if len(consoleBackgroundGames[target]) == 0 {
			t.Errorf("%s → %s : pas de liste", alias, target)
		}
	}
}

func solidJPEG(t *testing.T, width, height int, fill color.RGBA) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for index := 0; index < len(picture.Pix); index += 4 {
		picture.Pix[index], picture.Pix[index+1], picture.Pix[index+2], picture.Pix[index+3] = fill.R, fill.G, fill.B, 255
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, picture, nil); err != nil {
		t.Fatal(err)
	}

	return encoded.Bytes()
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return content
}

// filledSlots — les images des emplacements pris, dans leur ordre.
func filledSlots(slots BackgroundSlots) []string {
	names := []string{}
	for _, slot := range slots.Slots {
		if slot.Name != "" {
			names = append(names, slot.Name)
		}
	}

	return names
}

// LES GESTES DE L'ADMINISTRATEUR, PAR EMPLACEMENT (remarque e du porteur) :
// remplir un emplacement libre, remplacer une image (l'extension suit le
// contenu) ; les réductions de l'image changée partent. Un emplacement ne se
// vide pas : une image se remplace (le porteur, 2026-11-14).
func TestAdministratorBackgroundGestures(t *testing.T) {
	root := t.TempDir()
	directory := ConsoleBackgroundDir(root, "vb")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	blue := solidJPEG(t, 600, 800, color.RGBA{0, 0, 200, 255})
	for _, name := range []string{"01.jpg", "02.jpg"} {
		_ = os.WriteFile(filepath.Join(directory, name), blue, 0o644)
	}
	_ = os.WriteFile(filepath.Join(directory, backgroundMarker), []byte("{}"), 0o644)
	var red bytes.Buffer
	_ = png.Encode(&red, transparentIcon(300, 400))

	added, err := SetConsoleBackground(root, "vb", 3, red.Bytes())
	if err != nil || added != "03.png" {
		t.Fatalf("emplacement 3 rempli : %q, %v", added, err)
	}

	// Une réduction de 01.jpg existe ; remplacé par un PNG, il devient 01.png
	// et sa réduction part.
	reduction, err := ConsoleBackground(root, "vb", "01.jpg", 256)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := SetConsoleBackground(root, "vb", 1, red.Bytes())
	if err != nil || replaced != "01.png" {
		t.Fatalf("remplacement : %q, %v", replaced, err)
	}
	if _, err := os.Stat(reduction); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("réduction de l'ancienne image restée : %v", err)
	}
	if names := listBackgrounds(directory); strings.Join(names, ",") != "01.png,02.jpg,03.png" {
		t.Fatalf("fond : %v", names)
	}
	// Remplacé sous la même extension : la réduction se refait.
	first, _ := ConsoleBackground(root, "vb", "02.jpg", 256)
	if _, err := SetConsoleBackground(root, "vb", 2, solidJPEG(t, 600, 800, color.RGBA{200, 0, 0, 255})); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("réduction de 02.jpg restée après son remplacement")
	}
	again, _ := ConsoleBackground(root, "vb", "02.jpg", 256)
	picture, err := jpeg.Decode(bytes.NewReader(mustRead(t, again)))
	if err != nil {
		t.Fatal(err)
	}
	if redLevel, _, blueLevel, _ := picture.At(10, 10).RGBA(); redLevel < 0xb000 || blueLevel > 0x2000 {
		t.Fatalf("la réduction montre l'ancienne image : %v", picture.At(10, 10))
	}

	if _, err := os.Stat(filepath.Join(directory, backgroundMarker)); err != nil {
		t.Fatalf(".seeded retiré : %v", err)
	}
	slots, _ := ConsoleBackgrounds(root, nil, "vb", time.Second)
	if names := filledSlots(slots); strings.Join(names, ",") != "01.png,02.jpg,03.png" {
		t.Fatalf("après les remplacements : %+v", slots)
	}

	// UN emplacement reprend son image par défaut : la sienne part, réductions comprises, les autres restent, et
	// .seeded part (le remplissage refera cet emplacement libre).
	if _, err := ConsoleBackground(root, "vb", "02.jpg", 256); err != nil {
		t.Fatal(err)
	}
	if err := ResetConsoleBackgroundSlot(root, "vb", 2); err != nil {
		t.Fatal(err)
	}
	if names := listBackgrounds(directory); strings.Join(names, ",") != "01.png,03.png" {
		t.Fatalf("après la remise de l'emplacement 2 : %v", names)
	}
	if reductions, _ := filepath.Glob(filepath.Join(backgroundCacheDir(root, "vb"), "02.jpg-*")); len(reductions) != 0 {
		t.Fatalf("réductions restées : %v", reductions)
	}
	if _, err := os.Stat(filepath.Join(directory, backgroundMarker)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(".seeded toujours là après la remise d'un emplacement")
	}

	// LA VRAIE REMISE À ZÉRO (chantier 53, P9) : les huit emplacements se vident, réductions comprises, et .seeded part
	// — le remplissage les refait tous ; une image hors des emplacements (les fichiers du joueur) reste.
	_ = os.WriteFile(filepath.Join(directory, backgroundMarker), []byte("{}"), 0o644)
	_ = os.WriteFile(filepath.Join(directory, "ff9.png"), red.Bytes(), 0o644)
	if _, err := ConsoleBackground(root, "vb", "01.png", 256); err != nil {
		t.Fatal(err)
	}
	if err := ResetConsoleBackgrounds(root, "vb"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, backgroundMarker)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(".seeded toujours là")
	}
	if names := listBackgrounds(directory); strings.Join(names, ",") != "ff9.png" {
		t.Fatalf("après la remise à zéro : %v (attendu la seule image hors des emplacements)", names)
	}
	if reductions, _ := filepath.Glob(filepath.Join(backgroundCacheDir(root, "vb"), "0*")); len(reductions) != 0 {
		t.Fatalf("réductions des emplacements restées : %v", reductions)
	}
	if err := ResetConsoleBackgroundSlot(root, "vb", 9); !errors.Is(err, ErrBackgroundSlot) {
		t.Fatalf("emplacement 9 : %v", err)
	}
	if err := ResetConsoleBackgroundSlot(root, "playstation9", 1); !errors.Is(err, ErrUnknownConsole) {
		t.Fatalf("console inconnue : %v", err)
	}
}

// HUIT EMPLACEMENTS AU PLUS, RIEN D'EFFACÉ : une console qui a déjà plus
// d'images sur le disque les voit signalées en trop (une deuxième image du
// même emplacement comprise) ; aucun geste ne passe au-delà du huitième, et
// les images en trop restent, servies comme les autres.
func TestMoreThanEightImagesAreReportedNeverErased(t *testing.T) {
	root := t.TempDir()
	directory := ConsoleBackgroundDir(root, "vb")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	blue := solidJPEG(t, 60, 80, color.RGBA{0, 0, 200, 255})
	extras := []string{"03.png", "09.jpg", "tekken.webp"}
	for _, name := range append([]string{"01.jpg", "02.jpg", "03.jpg", "04.jpg", "05.jpg", "06.jpg", "07.jpg", "08.jpg"}, extras...) {
		_ = os.WriteFile(filepath.Join(directory, name), blue, 0o644)
	}
	_ = os.WriteFile(filepath.Join(directory, backgroundMarker), []byte("{}"), 0o644)

	slots, err := ConsoleBackgrounds(root, nil, "vb", time.Second)
	if err != nil || len(slots.Slots) != BackgroundSlotCount || len(filledSlots(slots)) != BackgroundSlotCount || slots.Slots[2].Name != "03.jpg" ||
		strings.Join(slots.Extra, ",") != strings.Join(extras, ",") {
		t.Fatalf("fond : %+v, %v ; attendu huit emplacements et %v en trop", slots, err, extras)
	}
	for _, slot := range []int{0, 9, -1, 100} {
		if _, err := SetConsoleBackground(root, "vb", slot, blue); !errors.Is(err, ErrBackgroundSlot) {
			t.Errorf("emplacement %d rempli : %v", slot, err)
		}
	}
	if _, err := SetConsoleBackground(root, "vb", 4, solidJPEG(t, 60, 80, color.RGBA{200, 0, 0, 255})); err != nil {
		t.Fatal(err)
	}
	for _, name := range extras {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Errorf("%s effacée : %v", name, err)
		}
	}
	if _, err := ConsoleBackground(root, "vb", "09.jpg", 256); err != nil {
		t.Fatalf("une image en trop ne se sert plus : %v", err)
	}
}

// LES EMPLACEMENTS ET LES CONTENUS REFUSÉS : rien d'autre qu'une image, rien
// pour une console inconnue, rien d'écrit.
func TestAdministratorBackgroundValidation(t *testing.T) {
	root := t.TempDir()
	var image bytes.Buffer
	_ = png.Encode(&image, transparentIcon(20, 20))
	for _, content := range [][]byte{[]byte("<svg/>"), []byte("\x89PNG\r\n\x1a\nfaux"), nil} {
		if _, err := SetConsoleBackground(root, "vb", 1, content); !errors.Is(err, ErrUnreadableImage) {
			t.Errorf("contenu %q : %v", content, err)
		}
	}
	if _, err := SetConsoleBackground(root, "playstation9", 1, image.Bytes()); !errors.Is(err, ErrUnknownConsole) {
		t.Errorf("console inconnue : %v", err)
	}
	if err := ResetConsoleBackgrounds(root, "playstation9"); !errors.Is(err, ErrUnknownConsole) {
		t.Errorf("reset d'une console inconnue : %v", err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("écrit : %v", entries)
	}
}
