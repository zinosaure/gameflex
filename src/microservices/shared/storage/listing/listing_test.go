package listing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"gameflex/storage"
	"gameflex/storage/gamestatus"
)

// Une source en mémoire à l'arborescence réelle : bios/, cores/, roms/.
type fakeSource struct {
	name string
	tree map[string][]storage.Entry
}

func (s fakeSource) Name() string { return s.name }
func (s fakeSource) Kind() string { return "webdav" }

func (s fakeSource) List(dir string) ([]storage.Entry, error) {
	return s.tree[dir], nil
}

func sample() fakeSource {
	return fakeSource{
		name: "kDrive",
		tree: map[string][]storage.Entry{
			"": {
				{Name: "bios", Dir: true},
				{Name: "cores", Dir: true},
				{Name: "roms", Dir: true},
			},
			"bios":     {{Name: "ps1", Dir: true}, {Name: "commun.bin", Size: 10}},
			"bios/ps1": {{Name: "scph5501.bin", Size: 524288}},
			"cores":    {{Name: "fceumm_libretro.so", Size: 2868136}},
			"roms":     {{Name: "nes", Dir: true}, {Name: "gba", Dir: true}},
			"roms/nes": {{Name: "Metroid.nes", Size: 131072}, {Name: "Zelda.nes", Size: 131072}},
			"roms/gba": {{Name: "Fire Emblem.gba", Size: 8388608}},
		},
	}
}

var t0 = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// UNE RESSOURCE NE FOURNIT QUE DES JEUX (#153, chantier 15) : ses BIOS et ses
// cœurs ne sont pas indexés. Chaque jeu porte son UUID, ÉCRIT dans le listing.
func TestBuildListsOnlyTheGamesOfAResource(t *testing.T) {
	built, err := Build(sample(), t0)
	if err != nil {
		t.Fatalf("build : %v", err)
	}

	if len(built.Roms["nes"]) != 2 || built.Roms["nes"][0].Name != "Metroid" || len(built.Roms["gba"]) != 1 {
		t.Fatalf("jeux = %+v", built.Roms)
	}
	metroid := built.Roms["nes"][0]
	if metroid.ID != storage.RomID("nes", "Metroid.nes") || metroid.Path != "kDrive/roms/nes/Metroid.nes" ||
		len(metroid.Files) != 1 || metroid.Files[0] != "Metroid.nes" || metroid.Dir {
		t.Fatalf("Metroid = %+v", metroid)
	}
	if len(built.Bios) != 0 || len(built.Cores) != 0 {
		t.Fatalf("une ressource n'a ni BIOS ni cœurs : %+v %+v", built.Bios, built.Cores)
	}
	if built.Header.Source != "webdav" || built.Header.Name != "kDrive" || !built.Header.UpdatedAt.Equal(t0) ||
		built.Header.Sections[Roms].Consoles["nes"] != 2 {
		t.Fatalf("en-tête = %+v", built.Header)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	built, _ := Build(sample(), t0)
	if err := Save(dir, "kDrive", built); err != nil {
		t.Fatalf("save : %v", err)
	}

	// L'ARBORESCENCE DEMANDÉE, à la lettre : un cache qu'on doit pouvoir
	// lire et vider à la main.
	for _, wanted := range []string{"kDrive/listing.json", "kDrive/nes/roms.json", "kDrive/gba/roms.json"} {
		if _, err := os.Stat(filepath.Join(dir, wanted)); err != nil {
			t.Fatalf("%s manque : %v", wanted, err)
		}
	}

	back, err := Load(dir, "kDrive")
	if err != nil {
		t.Fatalf("load : %v", err)
	}
	if len(back.Roms["nes"]) != 2 || back.Roms["nes"][1].ID != storage.RomID("nes", "Zelda.nes") ||
		!back.Header.UpdatedAt.Equal(t0) {
		t.Fatalf("relu = %+v", back)
	}
}

// Charger UNE console ne doit pas coûter le prix de toutes ; une console sans
// élément est une liste vide, pas une panne ; une source jamais listée l'est.
func TestLoadSectionReadsOneFile(t *testing.T) {
	dir := t.TempDir()
	built, _ := Build(sample(), t0)
	_ = Save(dir, "kDrive", built)

	roms, err := LoadSection(dir, "kDrive", "nes", Roms)
	if err != nil || len(roms) != 2 || roms[0].ID == "" {
		t.Fatalf("roms nes = %+v / %v", roms, err)
	}
	if empty, err := LoadSection(dir, "kDrive", "snes", Roms); err != nil || len(empty) != 0 {
		t.Fatalf("une console sans jeux = %+v / %v", empty, err)
	}
	if _, err := LoadSection(dir, "inconnue", "nes", Roms); err != ErrNoCache {
		t.Fatalf("une source jamais listée doit rendre ErrNoCache, pas %v", err)
	}
	if !Scanned(dir, "kDrive") || Scanned(dir, "inconnue") {
		t.Fatal("Scanned ne distingue pas une source listée d'une autre")
	}
}

// UNE SECTION SE REFAIT SEULE — l'icône à côté d'un sélecteur de source, et
// GameFlex après ses propres écritures : les autres ne bougent pas, et la date
// avance.
func TestSaveSectionReplacesOneSection(t *testing.T) {
	dir := t.TempDir()
	self := New(SelfSource, "kingdom")
	self.SetSection(Roms, map[string][]Item{"nes": {{ID: "a", Name: "Metroid"}}}, t0)
	self.SetSection(Cores, map[string][]Item{"nes": {{Name: "fceumm"}}, "snes": {{Name: "snes9x"}}}, t0)
	if err := Save(dir, SelfKey, self); err != nil {
		t.Fatal(err)
	}

	later := t0.Add(time.Hour)
	if err := SaveSection(dir, SelfKey, Cores, map[string][]Item{"gba": {{Name: "mgba"}}}, later); err != nil {
		t.Fatalf("section : %v", err)
	}

	cores, err := LoadSectionAll(dir, SelfKey, Cores)
	if err != nil || len(cores) != 1 || cores["gba"][0].Name != "mgba" {
		t.Fatalf("cœurs = %+v / %v", cores, err)
	}
	if roms, _ := LoadSection(dir, SelfKey, "nes", Roms); len(roms) != 1 {
		t.Fatalf("les jeux ont bougé : %+v", roms)
	}
	header, _ := LoadHeader(dir, SelfKey)
	if !header.UpdatedAt.Equal(later) || !header.Sections[Cores].UpdatedAt.Equal(later) || !header.Sections[Roms].UpdatedAt.Equal(t0) {
		t.Fatalf("dates = %+v", header)
	}

	if err := SaveSection(t.TempDir(), SelfKey, Cores, nil, later); err != ErrNoCache {
		t.Fatalf("une section sans listing = %v, attendu ErrNoCache", err)
	}
	if err := SaveSection(dir, SelfKey, "jeux", nil, later); err == nil {
		t.Fatal("une section inconnue doit être refusée")
	}
}

// UNE CONSOLE SEULE (chantier 30) : un signalement réécrit sa console, garde
// les autres, et fait avancer la date qu'un ami compare. Les signalements et
// le statut des consoles font l'aller-retour.
func TestSaveConsoleSectionReplacesOneConsole(t *testing.T) {
	dir := t.TempDir()
	self := New(SelfSource, "kingdom")
	self.SetSection(Roms, map[string][]Item{
		"nes":  {{ID: "a", Name: "Metroid"}},
		"snes": {{ID: "b", Name: "F-Zero"}},
	}, t0)
	self.Header.Statuses = map[string]string{"snes": gamestatus.Playable}
	if err := Save(dir, SelfKey, self); err != nil {
		t.Fatal(err)
	}

	later := t0.Add(time.Hour)
	reported := []Item{{ID: "a", Name: "Metroid", Reports: []gamestatus.Report{{Status: gamestatus.Crashes, By: "root", At: later}}}}
	if err := SaveConsoleSection(dir, SelfKey, Roms, "nes", reported, later); err != nil {
		t.Fatal(err)
	}

	nes, _ := LoadSection(dir, SelfKey, "nes", Roms)
	if len(nes) != 1 || len(nes[0].Reports) != 1 || nes[0].Reports[0].Status != gamestatus.Crashes {
		t.Fatalf("la NES n'a pas son signalement : %+v", nes)
	}
	if snes, _ := LoadSection(dir, SelfKey, "snes", Roms); len(snes) != 1 {
		t.Fatalf("la SNES a bougé : %+v", snes)
	}
	header, _ := LoadHeader(dir, SelfKey)
	if !header.UpdatedAt.Equal(later) || !header.Sections[Roms].UpdatedAt.Equal(later) {
		t.Fatalf("dates = %+v", header)
	}
	if header.Sections[Roms].Consoles["nes"] != 1 || header.Sections[Roms].Consoles["snes"] != 1 {
		t.Fatalf("comptes = %+v", header.Sections[Roms].Consoles)
	}
	if header.Statuses["snes"] != gamestatus.Playable {
		t.Fatalf("le statut de la SNES est perdu : %+v", header.Statuses)
	}

	// Une console vidée disparaît de la section.
	if err := SaveConsoleSection(dir, SelfKey, Roms, "nes", nil, later); err != nil {
		t.Fatal(err)
	}
	header, _ = LoadHeader(dir, SelfKey)
	if _, still := header.Sections[Roms].Consoles["nes"]; still {
		t.Fatalf("la NES vide est restée : %+v", header.Sections[Roms].Consoles)
	}
	if err := SaveConsoleSection(t.TempDir(), SelfKey, Roms, "nes", nil, later); err != ErrNoCache {
		t.Fatalf("sans listing = %v, attendu ErrNoCache", err)
	}
}

// UNE LECTURE N'ATTRAPE JAMAIS UNE ÉCRITURE EN COURS : Save repart de zéro et
// SaveSection retire les fichiers d'une section avant de les réécrire. Lue
// entre les deux, une console qui a des jeux en montrait zéro — la liste d'un
// joueur vidée le temps qu'un autre importe un jeu.
func TestAReaderNeverSeesAHalfWrittenListing(t *testing.T) {
	dir := t.TempDir()
	games := map[string][]Item{"nes": {{ID: "a", Name: "Metroid"}, {ID: "b", Name: "Zelda"}}}
	self := New(SelfSource, "kingdom")
	self.SetSection(Roms, games, t0)
	if err := Save(dir, SelfKey, self); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 300; i++ {
			if i%10 == 0 {
				_ = Save(dir, SelfKey, self)
			} else {
				_ = SaveSection(dir, SelfKey, Roms, games, t0)
			}
		}
	}()
	var failure string
	for writing := true; writing && failure == ""; {
		select {
		case <-done:
			writing = false
		default:
		}
		if roms, err := LoadSection(dir, SelfKey, "nes", Roms); err != nil || len(roms) != 2 {
			failure = fmt.Sprintf("%d jeux (%v)", len(roms), err)
		} else if all, err := LoadSectionAll(dir, SelfKey, Roms); err != nil || len(all["nes"]) != 2 {
			failure = fmt.Sprintf("%+v (%v)", all, err)
		}
	}
	<-done // l'écrivain fini avant que le dossier ne parte
	if failure != "" {
		t.Fatalf("lu pendant une écriture : %s", failure)
	}
}

// UNE SECTION RANGÉE PAR CONSOLE SUR LE DISQUE (les correctifs, les BIOS du
// téléphone) : un élément par entrée sous `<console>/`, un dossier pèse son
// arborescence ; ce qui est caché, ou posé à la racine sans console, n'y entre
// pas.
func TestLocalTreeListsWhatIsUnderEachConsole(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{
		"wiiu/NewSuperMarioBrosU/CrashFix/rules.txt": "12345",
		"wiiu/NewSuperMarioBrosU/.DS_Store":          "xx",
		"ps1/scph5501.bin":                           "123",
		"ps1/.cache":                                 "x",
		"sans-console.bin":                           "x",
		".corbeille/ps1/ancien.bin":                  "x",
	} {
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tree := LocalTree(root)
	if len(tree) != 2 {
		t.Fatalf("consoles = %+v", tree)
	}
	pack := tree["wiiu"]
	if len(pack) != 1 || pack[0].Name != "NewSuperMarioBrosU" || !pack[0].Dir || pack[0].Size != 5 ||
		strings.Join(pack[0].Files, ",") != "CrashFix" {
		t.Fatalf("correctifs = %+v", pack)
	}
	bios := tree["ps1"]
	if len(bios) != 1 || bios[0].Name != "scph5501.bin" || bios[0].Dir || bios[0].Size != 3 {
		t.Fatalf("BIOS = %+v", bios)
	}
	if missing := LocalTree(filepath.Join(root, "absent")); len(missing) != 0 {
		t.Fatalf("un dossier absent = %+v", missing)
	}
}

// QUI SERT LE LISTING, MIS À JOUR AU DÉMARRAGE : une nouvelle version de
// GameFlex avance la date — un ami doit la voir — ; un redémarrage à
// l'identique, non.
func TestSaveServerMovesTheDateOnlyWhenSomethingChanged(t *testing.T) {
	dir := t.TempDir()
	self := New(SelfSource, "kingdom")
	self.Header.Server = &Server{Hostname: "kingdom", Version: "0.2.0"}
	self.SetSection(Roms, map[string][]Item{"nes": {{ID: "a", Name: "Metroid"}}}, t0)
	if err := Save(dir, SelfKey, self); err != nil {
		t.Fatal(err)
	}

	later := t0.Add(time.Hour)
	if err := SaveServer(dir, SelfKey, "kingdom", &Server{Hostname: "kingdom", Version: "0.2.0"}, nil, later); err != nil {
		t.Fatal(err)
	}
	if header, _ := LoadHeader(dir, SelfKey); !header.UpdatedAt.Equal(t0) {
		t.Fatalf("un redémarrage à l'identique a avancé la date : %v", header.UpdatedAt)
	}
	if err := SaveServer(dir, SelfKey, "kingdom", &Server{Hostname: "kingdom", Version: "0.3.0"}, nil, later); err != nil {
		t.Fatal(err)
	}
	header, _ := LoadHeader(dir, SelfKey)
	if !header.UpdatedAt.Equal(later) || header.Server.Version != "0.3.0" || header.Sections[Roms].Consoles["nes"] != 1 {
		t.Fatalf("après une nouvelle version : %+v", header)
	}
}

// UNE SOURCE RETIRÉE EMPORTE SON LISTING : parmi ceux qu'on reconnaît, ceux
// qu'on ne garde plus ; jamais un listing écrit dans la minute ; et un listing
// d'un format d'avant, illisible, part quel qu'il soit.
func TestPruneRemovesWhatNoSourceKeeps(t *testing.T) {
	dir := t.TempDir()
	for key, source := range map[string]string{SelfKey: SelfSource, "gardee": "webdav", "partie": "webdav", "ami": PeerSource} {
		if err := Save(dir, key, New(source, key)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "format4", "nes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "format4", "listing.json"), []byte(`{"version":4}`), 0o644); err != nil {
		t.Fatal(err)
	}
	resource := func(header Header) bool { return header.Source != SelfSource && header.Source != PeerSource }
	keep := map[string]bool{SelfKey: true, "gardee": true}

	if removed := Prune(dir, keep, resource); len(removed) != 0 {
		t.Fatalf("des listings tout neufs ont été retirés : %v", removed)
	}
	old := time.Now().Add(-2 * PruneGrace)
	for _, key := range []string{"partie", "ami", "format4"} {
		for _, path := range []string{filepath.Join(dir, key), filepath.Join(dir, key, "listing.json")} {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}

	removed := Prune(dir, keep, resource)
	sort.Strings(removed)
	if strings.Join(removed, ",") != "format4,partie" {
		t.Fatalf("retirés = %v, attendu format4 et partie", removed)
	}
	for _, key := range []string{SelfKey, "gardee", "ami"} {
		if !Scanned(dir, key) {
			t.Errorf("%s a été retiré", key)
		}
	}
}

// LE LISTING VOYAGE EN UN DOCUMENT : celui qui le reçoit le range sous la même
// forme, et le lit comme le sien.
func TestTheListingTravelsAsOneDocument(t *testing.T) {
	served, received := t.TempDir(), t.TempDir()
	self := New(SelfSource, "kingdom")
	self.Header.Server = &Server{Hostname: "kingdom", Version: "0.2.0"}
	self.Header.Machine = &Machine{OS: "linux", Cores: 16, RAMBytes: 40 << 30}
	self.SetSection(Roms, map[string][]Item{"nes": {{ID: storage.RomID("nes", "Metroid.nes"), Name: "Metroid", Files: []string{"Metroid.nes"}}}}, t0)
	self.SetSection(Params, map[string][]Item{"ps3": {{Name: "rpcs3", Values: true, Presets: []Preset{{ID: "rapide", Name: "Rapide"}}}}}, t0)
	if err := Save(served, SelfKey, self); err != nil {
		t.Fatal(err)
	}

	whole, _ := Load(served, SelfKey)
	document, err := json.Marshal(whole)
	if err != nil {
		t.Fatal(err)
	}
	var copied Listing
	if err := json.Unmarshal(document, &copied); err != nil {
		t.Fatal(err)
	}
	copied.Header.Source, copied.Header.Scopes = PeerSource, []string{"catalog", "devices"}
	if err := Save(received, "ami", copied); err != nil {
		t.Fatal(err)
	}

	header, _ := LoadHeader(received, "ami")
	if header.Server.Hostname != "kingdom" || header.Machine.Cores != 16 || len(header.Scopes) != 2 || !header.UpdatedAt.Equal(t0) {
		t.Fatalf("en-tête reçu = %+v", header)
	}
	if params, _ := LoadSection(received, "ami", "ps3", Params); len(params) != 1 || params[0].Presets[0].ID != "rapide" {
		t.Fatalf("paramètres reçus = %+v", params)
	}
}

// L'EN-TÊTE SE RÉÉCRIT SEUL (les portées d'un jeton, la machine) sans toucher
// aux sections.
func TestSaveHeaderKeepsTheSections(t *testing.T) {
	dir := t.TempDir()
	built, _ := Build(sample(), t0)
	_ = Save(dir, "ami", built)

	if err := SaveHeader(dir, "ami", func(header *Header) { header.Scopes = []string{"catalog"} }); err != nil {
		t.Fatal(err)
	}
	header, _ := LoadHeader(dir, "ami")
	if len(header.Scopes) != 1 || header.Sections[Roms].Consoles["nes"] != 2 {
		t.Fatalf("en-tête = %+v", header)
	}
	if roms, _ := LoadSection(dir, "ami", "nes", Roms); len(roms) != 2 {
		t.Fatalf("jeux = %+v", roms)
	}
}

// Un nouveau parcours doit FAIRE DISPARAÎTRE ce que la source n'a plus.
func TestSaveForgetsWhatIsGone(t *testing.T) {
	dir := t.TempDir()
	built, _ := Build(sample(), t0)
	_ = Save(dir, "kDrive", built)

	shrunk := sample()
	shrunk.tree["roms"] = []storage.Entry{{Name: "nes", Dir: true}}
	rebuilt, _ := Build(shrunk, t0.Add(time.Minute))
	if err := Save(dir, "kDrive", rebuilt); err != nil {
		t.Fatalf("save : %v", err)
	}

	if gba, err := LoadSection(dir, "kDrive", "gba", Roms); err != nil || len(gba) != 0 {
		t.Fatalf("la console disparue est encore listée : %+v / %v", gba, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kDrive", "gba")); !os.IsNotExist(err) {
		t.Fatalf("son fichier est resté : %v", err)
	}
}

// LE LISTING D'UNE SOURCE QU'ON RETIRE PART AVEC ELLE.
func TestRemoveForgetsTheSource(t *testing.T) {
	dir := t.TempDir()
	built, _ := Build(sample(), t0)
	_ = Save(dir, "kDrive", built)
	_ = Save(dir, "autre", built)

	if keys := Keys(dir); len(keys) != 2 {
		t.Fatalf("clés = %v", keys)
	}
	if err := Remove(dir, "kDrive"); err != nil {
		t.Fatal(err)
	}
	if Scanned(dir, "kDrive") || !Scanned(dir, "autre") || len(Keys(dir)) != 1 {
		t.Fatalf("après retrait : %v", Keys(dir))
	}
}

// Un nom de source est choisi par son propriétaire : il ne doit pas pouvoir
// creuser l'arborescence.
func TestResourceNameCannotEscapeTheCache(t *testing.T) {
	dir := t.TempDir()
	built, _ := Build(sample(), t0)
	if err := Save(dir, "../../echappe", built); err != nil {
		t.Fatalf("save : %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || strings.ContainsAny(entries[0].Name(), `\/`) {
		t.Fatalf("le nom porte encore un séparateur : %+v", entries)
	}
	if _, err := LoadSection(dir, "../../echappe", "nes", Roms); err != nil {
		t.Fatalf("le cache doit rester relisible sous son nom assaini : %v", err)
	}
}

// LA CLÉ DÉCRIT LA SOURCE, PAS SON ÉTIQUETTE : deux drives portant le même nom
// se seraient partagé un cache (signalé le 2026-09-02).
func TestKeyDistinguishesResourcesNotNames(t *testing.T) {
	first := Key("kdrive", "133300", "games")
	if Key("kdrive", "244733", "games") == first {
		t.Fatal("deux drives différents partagent une clé")
	}
	if Key("kdrive", "133300", "games") != first {
		t.Fatal("la même ressource doit retrouver sa clé")
	}
	if Key("kdrive", "133300", "autre") == first {
		t.Fatal("deux dossiers d'un même drive partagent une clé")
	}
	if Key("webdav", "133300", "games") == first {
		t.Fatal("deux protocoles partagent une clé")
	}
	if Key("kdrive", "133300/", "/games/") != first {
		t.Fatal("une barre en trop ne doit pas changer l'identité")
	}
	if strings.ContainsAny(first, `\/:*?"<>| `) || first == "" {
		t.Fatalf("clé inutilisable comme nom de dossier : %q", first)
	}
}

// UN LISTING D'UNE AUTRE VERSION NE SE LIT PAS : un champ ajouté depuis se
// relirait à sa valeur nulle, et le code d'aujourd'hui prendrait ce vide pour
// une réponse — un PS1 s'est ainsi déclaré « fichier seul » (2026-09-02).
func TestACacheFromAnotherVersionIsIgnored(t *testing.T) {
	dir := t.TempDir()
	built, _ := Build(sample(), t0)
	if err := Save(dir, "abc", built); err != nil {
		t.Fatalf("save: %v", err)
	}

	header := filepath.Join(dir, "abc", "listing.json")
	raw, _ := os.ReadFile(header)
	aged := strings.Replace(string(raw), `"version":`+strconv.Itoa(version), `"version":`+strconv.Itoa(version-1), 1)
	if aged == string(raw) {
		t.Fatalf("la version n'est pas écrite dans listing.json : %s", raw)
	}
	if err := os.WriteFile(header, []byte(aged), 0o644); err != nil {
		t.Fatalf("écriture : %v", err)
	}

	if _, err := Load(dir, "abc"); err != ErrNoCache {
		t.Fatalf("Load = %v, attendu ErrNoCache", err)
	}
	if _, err := LoadSection(dir, "abc", "nes", Roms); err != ErrNoCache {
		t.Fatalf("LoadSection = %v, attendu ErrNoCache", err)
	}
	if Scanned(dir, "abc") {
		t.Fatal("un listing d'une autre version est dit lisible")
	}
}

// LA MACHINE, lue comme Linux la montre à un conteneur.
func TestReadMachineFromProcAndSys(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		target := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("proc/sys/kernel/osrelease", "7.0.0-31-generic\n")
	write("proc/cpuinfo", "processor\t: 0\nmodel name\t: AMD Ryzen 7 255 w/ Radeon 780M Graphics\n\nprocessor\t: 1\nmodel name\t: AMD Ryzen 7 255 w/ Radeon 780M Graphics\n")
	write("proc/meminfo", "MemTotal:       39793516 kB\nMemFree:        1000 kB\n")
	write("sys/class/drm/card1/device/vendor", "0x1002\n")
	write("sys/class/drm/card1/device/mem_info_vram_total", "8589934592\n")
	write("sys/class/drm/card1-DP-1/device/vendor", "0x1002\n") // une sortie, pas une carte
	if err := os.MkdirAll(filepath.Join(root, "drivers/amdgpu"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "drivers/amdgpu"), filepath.Join(root, "sys/class/drm/card1/device/driver")); err != nil {
		t.Fatal(err)
	}

	machine := readMachine(root)
	if machine.OS != "linux 7.0.0-31-generic" || machine.CPU != "AMD Ryzen 7 255 w/ Radeon 780M Graphics" ||
		machine.Cores != 2 || machine.RAMBytes != 39793516*1024 {
		t.Fatalf("machine = %+v", machine)
	}
	if len(machine.GPUs) != 1 || machine.GPUs[0] != (GPU{Vendor: "AMD", Name: "Radeon 780M Graphics", Driver: "amdgpu", VRAMBytes: 8589934592}) {
		t.Fatalf("cartes = %+v", machine.GPUs)
	}

	// UNE CARTE AMD AJOUTÉE (en OCuLink) : rien ne dit laquelle est la puce
	// intégrée, aucune ne prend son nom.
	write("sys/class/drm/card2/device/vendor", "0x1002\n")
	machine = readMachine(root)
	if len(machine.GPUs) != 2 || machine.GPUs[0].Name != "" || machine.GPUs[1].Name != "" {
		t.Fatalf("deux cartes AMD = %+v", machine.GPUs)
	}
}

// UN TÉLÉPHONE SNAPDRAGON : son processeur graphique se lit dans le pilote
// KGSL, et son nom se dit comme on le connaît.
func TestReadMachineNamesAnAdreno(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "sys/class/kgsl/kgsl-3d0/gpu_model")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("Adreno830v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	machine := readMachine(root)
	if len(machine.GPUs) != 1 || machine.GPUs[0] != (GPU{Vendor: "Qualcomm", Name: "Adreno 830", Driver: "kgsl"}) {
		t.Fatalf("cartes = %+v", machine.GPUs)
	}
}
