package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/docker"
	"gameflex/storage"
	"gameflex/storage/listing"
)

// testDisk — un disque interne de test : un dossier rangé comme la
// bibliothèque (roms/<console>/…), avec ces fichiers.
func testDisk(t *testing.T, name string, present bool, files ...string) docker.InternalDisk {
	t.Helper()
	dir := t.TempDir()
	for _, file := range files {
		target := filepath.Join(dir, "roms", filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return docker.InternalDisk{Name: name, Dir: dir, HostDir: "/hote/" + name, Present: present}
}

// withDisks — le serveur voit ces disques internes, dans cet ordre.
func withDisks(server *Server, disks ...docker.InternalDisk) {
	server.internalDisks = func(context.Context) []docker.InternalDisk { return disks }
}

// UN JEU SE JOUE DEPUIS LE PREMIER ENDROIT QUI L'A (chantier 23) : la
// bibliothèque principale, puis les disques internes dans l'ordre. Un disque
// absent est sauté, même s'il a le jeu.
func TestAGameIsFoundInTheLibraryFirstThenOnTheDisksInOrder(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Mario.nes")
	absent := testDisk(t, "a-absent", false, "nes/Zelda.nes")
	first := testDisk(t, "b-premier", true, "nes/Mario.nes", "nes/Zelda.nes")
	second := testDisk(t, "c-second", true, "nes/Zelda.nes", "nes/Metroid.nes")
	withDisks(server, absent, first, second)
	ctx := context.Background()

	cases := map[string]string{"Mario.nes": "", "Zelda.nes": "b-premier", "Metroid.nes": "c-second"}
	for file, wantDisk := range cases {
		_, _, disk, err := server.findRom(ctx, "nes", storage.RomID("nes", file))
		if err != nil {
			t.Fatalf("%s introuvable : %v", file, err)
		}
		got := ""
		if disk != nil {
			got = disk.Name
		}
		if got != wantDisk {
			t.Fatalf("%s trouvé sur %q, attendu %q (vide = bibliothèque)", file, got, wantDisk)
		}
	}
	if _, _, _, err := server.findRom(ctx, "nes", storage.RomID("nes", "Kirby.nes")); err == nil {
		t.Fatal("un jeu absent partout a été trouvé")
	}
}

// LA LISTE D'UN DISQUE : ses jeux, avec la même référence que dans la
// bibliothèque, et le nom du disque. Un disque absent répond 503, un nom
// inconnu 404.
func TestTheListOfADiskCarriesItsGamesAndItsName(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Mario.nes")
	disk := testDisk(t, "ssd", true, "nes/Mario.nes", "nes/Zelda.nes")
	withDisks(server, disk, testDisk(t, "usb", false))

	router := chi.NewRouter()
	router.Get("/{consoleId}/roms", server.handleRetroRoms)
	get := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/nes/roms?"+query, nil))

		return recorder
	}

	recorder := get("disk=ssd")
	var answer struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("liste du disque = %d %s", recorder.Code, recorder.Body.String())
	}
	if len(answer.Data) != 2 {
		t.Fatalf("2 jeux attendus sur le disque, %d", len(answer.Data))
	}
	for _, rom := range answer.Data {
		if rom["disk"] != "ssd" || rom["kind"] != nil {
			t.Fatalf("un jeu de disque interne est un jeu d'ici qui dit son disque : %v", rom)
		}
	}
	if answer.Data[0]["reference"] != storage.RomID("nes", "Mario.nes") {
		t.Fatalf("référence différente de celle de la bibliothèque : %v", answer.Data[0]["reference"])
	}

	if code := get("disk=usb").Code; code != http.StatusServiceUnavailable {
		t.Fatalf("disque absent : %d, attendu 503", code)
	}
	if code := get("disk=nulle-part").Code; code != http.StatusNotFound {
		t.Fatalf("disque inconnu : %d, attendu 404", code)
	}
}

// POUR UN AMI, LA BIBLIOTHÈQUE ET LES DISQUES INTERNES NE FONT QU'UN : un jeu
// présent à deux endroits n'y est qu'une fois, et les compteurs suivent.
func TestAFriendSeesTheLibraryAndTheDisksAsOne(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Mario.nes")
	withDisks(server,
		testDisk(t, "ssd", true, "nes/Mario.nes", "nes/Zelda.nes", "gba/Golden Sun.gba"),
		testDisk(t, "usb", true, "nes/Zelda.nes", "nes/Metroid.nes"),
	)

	served := listing.New(listing.SelfSource, "ici")
	served.SetSection(listing.Roms, map[string][]listing.Item{
		"nes": {{ID: storage.RomID("nes", "Mario.nes"), Name: "Mario"}},
	}, served.Header.UpdatedAt)
	server.withInternalDisks(context.Background(), &served)

	names := map[string]bool{}
	for _, item := range served.Roms["nes"] {
		if names[item.Name] {
			t.Fatalf("%s listé deux fois", item.Name)
		}
		names[item.Name] = true
	}
	if len(served.Roms["nes"]) != 3 || !names["Mario"] || !names["Zelda"] || !names["Metroid"] {
		t.Fatalf("nes : %v, attendu Mario, Zelda, Metroid", served.Roms["nes"])
	}
	if len(served.Roms["gba"]) != 1 {
		t.Fatalf("une console qui n'est que sur un disque manque : %v", served.Roms["gba"])
	}
	counts := served.Header.Sections[listing.Roms].Consoles
	if counts["nes"] != 3 || counts["gba"] != 1 {
		t.Fatalf("compteurs %v, attendu nes 3, gba 1", counts)
	}
}

// LE MENU GARDE UNE CONSOLE DONT LES JEUX NE SONT PLUS QUE SUR UN DISQUE, et
// un jeu présent deux fois ne compte qu'une fois.
func TestConsoleCountsIncludeTheDisksWithoutDoubles(t *testing.T) {
	server, _ := listingServer(t, "roms/nes/Mario.nes")
	withDisks(server,
		testDisk(t, "ssd", true, "nes/Mario.nes", "nes/Zelda.nes", "gba/Golden Sun.gba"),
		testDisk(t, "usb", true, "nes/Zelda.nes"),
	)

	counts := server.mergedConsoleCounts(context.Background(), map[string]int{"nes": 1})
	if counts["nes"] != 2 || counts["gba"] != 1 {
		t.Fatalf("compteurs %v, attendu nes 2 (Mario, Zelda), gba 1", counts)
	}
}

// LE LISTING N'EST PAS LE DISQUE : des jeux retirés de la bibliothèque à la
// main restent dans son listing jusqu'au parcours. Leur copie sur un disque
// interne ne doit pas les compter une seconde fois.
func TestConsoleCountsDoNotDoubleWhatTheListingStillShows(t *testing.T) {
	server, root := listingServer(t, "roms/nes/Mario.nes", "roms/nes/Zelda.nes")
	withDisks(server, testDisk(t, "ssd", true, "nes/Mario.nes", "nes/Zelda.nes"))
	server.selfItems(listing.Roms, "nes") // le listing est construit, avec les deux jeux

	if err := os.RemoveAll(filepath.Join(root, "roms")); err != nil {
		t.Fatal(err)
	}
	counts := server.mergedConsoleCounts(context.Background(), map[string]int{"nes": 2})
	if counts["nes"] != 2 {
		t.Fatalf("nes compte %d, attendu 2 (les deux jeux, une seule fois)", counts["nes"])
	}
}
