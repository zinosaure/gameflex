package api

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/retro_files"
	"gameflex/metadata"
	"gameflex/storage"
)

// LES MÉTADONNÉES SANS JEU (le porteur, 2026-11-14) : listées au dossier du serveur et sur chaque disque présent,
// jamais un dossier par console (le fond, `_console/`) ni celui d'un jeu accessible ; retirées une à une, la
// suppression revérifiée (409 quand un jeu les porte), un gameid piégé refusé, une racine hors de la liste fermée
// refusée ; un joueur n'y a pas accès.
func TestMetadataWithoutAGame(t *testing.T) {
	server, root := listingServer(t, "roms/ps3/Bibliotheque.iso")
	server.config.JWTSecret = facadeTestSecret
	disk := testDisk(t, "Gamebox", true, "ps3/Disque.iso")
	withDisks(server, disk)
	router := server.retroRouter()
	admin := statusToken(t, "gino", auth.RoleAdmin)
	player := statusToken(t, "marc", 2)
	known := metadata.GameKey(filepath.Join(root, "roms", "ps3", "Bibliotheque.iso"), storage.RomID("ps3", "Bibliotheque.iso"))
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "metadata", "ps3", known, "manual", "source.json"), `{"title":"Un vrai jeu"}`)
	write(filepath.Join(root, "metadata", "ps3", "essai-orphelin", "manual", "source.json"), `{"title":"Orphelin d'essai"}`)
	write(filepath.Join(root, "metadata", "ps3", "essai-orphelin", "manual", "cover.png"), "\x89PNG\r\n\x1a\n")
	write(filepath.Join(root, "metadata", "ps3", "background", "01.png"), "png")
	write(filepath.Join(root, "metadata", "ps3", "_console", "cover.png"), "png")
	write(filepath.Join(root, "metadata", "ps3", ".write-123", "x"), "x")
	write(filepath.Join(disk.Dir, "metadata", "ps3", "disque-orphelin", "igdb", "source.json"), `{"source":"igdb","result":"found","name":"Sur le disque"}`)

	call := func(token, method, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}
	list := func() string {
		t.Helper()
		parts := []string{}
		for _, orphan := range listedOrphans(t, untilGamesRead(t, func() *httptest.ResponseRecorder { return call(admin, http.MethodGet, "/ps3/metadata/orphans") })) {
			parts = append(parts, orphan.GameID+"@"+orphan.Disk+"="+orphan.Title)
			if orphan.GameID == "essai-orphelin" && !orphan.Cover {
				t.Errorf("la jaquette de l'orphelin n'est pas dite")
			}
		}

		return strings.Join(parts, ",")
	}

	// La première lecture : ni la bibliothèque ni le disque n'ont leurs jeux lus — 503 aussitôt, la lecture part en fond.
	if answer := call(admin, http.MethodGet, "/ps3/metadata/orphans"); answer.Code != http.StatusServiceUnavailable || !strings.Contains(answer.Body.String(), "games_not_ready") {
		t.Fatalf("première lecture : %d %s", answer.Code, answer.Body.String())
	}
	if got := list(); got != "essai-orphelin@=Orphelin d'essai,disque-orphelin@Gamebox=Sur le disque" {
		t.Fatalf("orphelins : %s", got)
	}
	if answer := call(player, http.MethodGet, "/ps3/metadata/orphans"); answer.Code != http.StatusForbidden {
		t.Fatalf("un joueur : %d", answer.Code)
	}
	if answer := call(admin, http.MethodGet, "/ps3/metadata/orphans/essai-orphelin/cover"); answer.Code != http.StatusOK {
		t.Fatalf("jaquette : %d", answer.Code)
	}
	for _, step := range []struct {
		path string
		want int
	}{
		{"/ps3/metadata/orphans/" + known, http.StatusConflict},                     // un jeu le porte : gardé
		{"/ps3/metadata/orphans/background", http.StatusBadRequest},                 // le fond de la console
		{"/ps3/metadata/orphans/_console", http.StatusBadRequest},                   // ses images à elle
		{"/ps3/metadata/orphans/..%2F..%2Froms", http.StatusBadRequest},             // un chemin
		{"/ps3/metadata/orphans/.write-123", http.StatusBadRequest},                 // un nom caché
		{"/ps3/metadata/orphans/inconnu", http.StatusNotFound},                      // rien à ce nom
		{"/ps3/metadata/orphans/disque-orphelin?disk=Inconnu", http.StatusNotFound}, // hors de la liste fermée
		{"/nintendo99/metadata/orphans/essai-orphelin", http.StatusBadRequest},
	} {
		if answer := call(admin, http.MethodDelete, step.path); answer.Code != step.want {
			t.Fatalf("DELETE %s = %d %s, attendu %d", step.path, answer.Code, answer.Body.String(), step.want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "metadata", "ps3", known)); err != nil {
		t.Fatal("le dossier d'un vrai jeu a disparu")
	}
	for _, kept := range []string{"background", "_console"} {
		if _, err := os.Stat(filepath.Join(root, "metadata", "ps3", kept)); err != nil {
			t.Fatalf("%s a disparu", kept)
		}
	}

	if answer := call(admin, http.MethodDelete, "/ps3/metadata/orphans/essai-orphelin"); answer.Code != http.StatusOK {
		t.Fatalf("retirer l'orphelin : %d %s", answer.Code, answer.Body.String())
	}
	if answer := call(admin, http.MethodDelete, "/ps3/metadata/orphans/disque-orphelin?disk=Gamebox"); answer.Code != http.StatusOK {
		t.Fatalf("retirer l'orphelin du disque : %d %s", answer.Code, answer.Body.String())
	}
	if got := list(); got != "" {
		t.Fatalf("après : %s", got)
	}

	// UN JEU REVENU : le dossier d'un jeu du disque, le disque débranché, paraît orphelin ; rebranché, il ne l'est plus,
	// et sa suppression demandée entre-temps ne retire rien : 503 tant que les jeux du disque ne sont pas lus (sans
	// attendre leur lecture), 409 ensuite.
	other := testDisk(t, "Autre", true, "ps3/Revenu.iso")
	write(filepath.Join(other.Dir, "roms", "ps3", "Revenu.iso"), "un autre contenu")
	returned := metadata.GameKey(filepath.Join(other.Dir, "roms", "ps3", "Revenu.iso"), storage.RomID("ps3", "Revenu.iso"))
	write(filepath.Join(root, "metadata", "ps3", returned, "manual", "source.json"), `{"title":"Revenu"}`)
	if got := list(); got != returned+"@=Revenu" {
		t.Fatalf("disque débranché : %s", got)
	}
	withDisks(server, disk, other)
	if answer := call(admin, http.MethodDelete, "/ps3/metadata/orphans/"+returned); answer.Code != http.StatusServiceUnavailable || !strings.Contains(answer.Body.String(), "games_not_ready") {
		t.Fatalf("le jeu revenu, son disque pas encore lu : %d %s", answer.Code, answer.Body.String())
	}
	if got := list(); got != "" {
		t.Fatalf("disque rebranché : %s", got)
	}
	if answer := call(admin, http.MethodDelete, "/ps3/metadata/orphans/"+returned); answer.Code != http.StatusConflict {
		t.Fatalf("le jeu revenu : %d %s", answer.Code, answer.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "metadata", "ps3", returned)); err != nil {
		t.Fatal("le dossier du jeu revenu a disparu")
	}
}

// LA LISTE DES JEUX PAS ENCORE LUE (chantier 53) : la suppression et la liste répondent 503 `games_not_ready` AUSSITÔT,
// sans rien retirer — les empreintes se lisent en fond (101 s pour les ROM NES de Gamebox, une suppression restait
// deux minutes sans réponse) ; lues, la suppression se décide sur ce que la mémoire garde. Un jeu réécrit sur place
// (une autre empreinte, le catalogue n'en voit rien) se relit avant tout jugement : son nouveau contenu porte un
// dossier, qui est gardé.
func TestOrphansAnswerAtOnceWhileTheGamesAreRead(t *testing.T) {
	server, root := listingServer(t, "roms/nes/Jeu.nes")
	server.config.JWTSecret = facadeTestSecret
	router := server.retroRouter()
	admin := statusToken(t, "gino", auth.RoleAdmin)
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("Authorization", "Bearer "+admin)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}
	list := func() string {
		t.Helper()
		ids := []string{}
		for _, orphan := range listedOrphans(t, untilGamesRead(t, func() *httptest.ResponseRecorder { return call(http.MethodGet, "/nes/metadata/orphans") })) {
			ids = append(ids, orphan.GameID)
		}

		return strings.Join(ids, ",")
	}
	// L'empreinte qu'aura le jeu réécrit : la même, lue sur un fichier à côté de même contenu.
	tomorrow := filepath.Join(t.TempDir(), "Demain.nes")
	write(tomorrow, "le contenu de demain")
	future := metadata.GameKey(tomorrow, "repli")
	write(filepath.Join(root, "metadata", "nes", "orphelin", "manual", "source.json"), `{"title":"Orphelin"}`)
	write(filepath.Join(root, "metadata", "nes", future, "manual", "source.json"), `{"title":"Demain"}`)

	notReady := func(step string, answer *httptest.ResponseRecorder) {
		t.Helper()
		if answer.Code != http.StatusServiceUnavailable || !strings.Contains(answer.Body.String(), "games_not_ready") {
			t.Fatalf("%s : %d %s, attendu 503 games_not_ready", step, answer.Code, answer.Body.String())
		}
	}
	notReady("supprimer, les jeux pas lus", call(http.MethodDelete, "/nes/metadata/orphans/orphelin"))
	if _, err := os.Stat(filepath.Join(root, "metadata", "nes", "orphelin")); err != nil {
		t.Fatal("retiré alors que les jeux n'étaient pas lus")
	}
	if got := list(); got != future+",orphelin" {
		t.Fatalf("orphelins, les jeux lus : %s", got)
	}
	if answer := call(http.MethodDelete, "/nes/metadata/orphans/orphelin"); answer.Code != http.StatusOK {
		t.Fatalf("supprimer, les jeux en mémoire : %d %s", answer.Code, answer.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "metadata", "nes", "orphelin")); !os.IsNotExist(err) {
		t.Fatal("l'orphelin est resté")
	}

	// Le jeu réécrit sur place porte maintenant le dossier de demain : son empreinte d'hier ne vaut plus.
	write(filepath.Join(root, "roms", "nes", "Jeu.nes"), "le contenu de demain")
	notReady("supprimer, le jeu réécrit pas relu", call(http.MethodDelete, "/nes/metadata/orphans/"+future))
	if got := list(); got != "" {
		t.Fatalf("orphelins, le jeu réécrit relu : %s", got)
	}
	if answer := call(http.MethodDelete, "/nes/metadata/orphans/"+future); answer.Code != http.StatusConflict {
		t.Fatalf("supprimer le dossier du jeu réécrit : %d %s", answer.Code, answer.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "metadata", "nes", future)); err != nil {
		t.Fatal("le dossier du jeu réécrit a disparu")
	}
}

// LES JEUX-DOSSIERS (chantier 53, 2026-11-14) — un vidage Switch, PS3, PS4 ou PS5 est un DOSSIER : son empreinte est
// celle du dossier (l'identifiant de `sce_sys/`, sinon sa référence, tirée de son nom). Après un redémarrage du kernel,
// le catalogue se refait dès que la bibliothèque s'affiche, mais aucune de ces empreintes n'est en mémoire : la liste
// et la suppression répondent 503 `games_not_ready`, jamais une liste vide, puis la vraie liste. Le cas de la dev :
// « Final Fantasy VII - OG » renommé « Final Fantasy VII » sur Gamebox laisse le dossier de son ancienne référence
// orphelin ; un jeu-dossier renommé, ou qui reçoit un fichier, après la lecture de son empreinte se relit avant tout
// jugement.
func TestOrphansOfFolderGamesWaitForTheirKeys(t *testing.T) {
	server, root := listingServer(t, "roms/ps5/Astro Bot/eboot.bin", "roms/ps5/Astro Bot/sce_sys/param.json")
	server.config.JWTSecret = facadeTestSecret
	disk := testDisk(t, "Gamebox", true, "switch/Final Fantasy VII/jeu.nsp", "switch/Mario Kart 8 Deluxe/jeu.nsp")
	withDisks(server, disk)
	router := server.retroRouter()
	admin := statusToken(t, "gino", auth.RoleAdmin)
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("Authorization", "Bearer "+admin)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}
	notReady := func(step string, answer *httptest.ResponseRecorder) {
		t.Helper()
		if answer.Code != http.StatusServiceUnavailable || !strings.Contains(answer.Body.String(), "games_not_ready") {
			t.Fatalf("%s : %d %s, attendu 503 games_not_ready", step, answer.Code, answer.Body.String())
		}
	}
	list := func(consoleID string) string {
		t.Helper()
		parts := []string{}
		for _, orphan := range listedOrphans(t, untilGamesRead(t, func() *httptest.ResponseRecorder { return call(http.MethodGet, "/"+consoleID+"/metadata/orphans") })) {
			parts = append(parts, orphan.GameID+"@"+orphan.Disk)
		}

		return strings.Join(parts, ",")
	}
	write(filepath.Join(root, "roms", "ps5", "Astro Bot", "sce_sys", "param.json"), `{"titleId":"PPSA01325"}`)
	switchDir := filepath.Join(disk.Dir, "metadata", "switch")
	renamed, kept := storage.RomID("switch", "Final Fantasy VII - OG"), storage.RomID("switch", "Final Fantasy VII")
	deluxe := storage.RomID("switch", "Mario Kart 8 Deluxe")
	for _, gameID := range []string{renamed, kept, deluxe} {
		write(filepath.Join(switchDir, gameID, "igdb", "source.json"), `{"source":"igdb","result":"found"}`)
	}
	write(filepath.Join(switchDir, "background", "01.png"), "png")

	// La bibliothèque affichée après le redémarrage : le catalogue est construit et à jour, aucune empreinte lue.
	if _, err := retro_files.ListCatalog(disk.RomsDir(), "switch", ""); err != nil {
		t.Fatal(err)
	}
	if _, current := retro_files.CatalogPaths(disk.RomsDir(), "switch"); !current {
		t.Fatal("le catalogue du disque devrait être à jour")
	}
	notReady("la liste, les empreintes des jeux-dossiers pas lues", call(http.MethodGet, "/switch/metadata/orphans"))
	if got := list("switch"); got != renamed+"@Gamebox" {
		t.Fatalf("orphelins Switch : %q, attendu l'ancienne référence de Final Fantasy VII", got)
	}
	if answer := call(http.MethodDelete, "/switch/metadata/orphans/"+kept+"?disk=Gamebox"); answer.Code != http.StatusConflict {
		t.Fatalf("le dossier du jeu renommé, sous son nom d'aujourd'hui : %d %s", answer.Code, answer.Body.String())
	}

	// Renommé après la lecture de son empreinte : le dossier de son ancien nom n'est porté par personne.
	if err := os.Rename(filepath.Join(disk.Dir, "roms", "switch", "Mario Kart 8 Deluxe"), filepath.Join(disk.Dir, "roms", "switch", "Mario Kart 8")); err != nil {
		t.Fatal(err)
	}
	notReady("supprimer, le jeu-dossier renommé pas relu", call(http.MethodDelete, "/switch/metadata/orphans/"+deluxe+"?disk=Gamebox"))
	if got := list("switch"); got != renamed+"@Gamebox,"+deluxe+"@Gamebox" {
		t.Fatalf("orphelins Switch, un jeu renommé : %q", got)
	}

	// Un fichier ajouté DANS un jeu-dossier (une mise à jour) change la date du dossier : l'empreinte d'hier ne vaut
	// plus pour lui, il se relit — et reste porté.
	write(filepath.Join(disk.Dir, "roms", "switch", "Final Fantasy VII", "maj.nsp"), "x")
	notReady("supprimer, le jeu-dossier changé pas relu", call(http.MethodDelete, "/switch/metadata/orphans/"+kept+"?disk=Gamebox"))
	if got := list("switch"); got != renamed+"@Gamebox,"+deluxe+"@Gamebox" {
		t.Fatalf("orphelins Switch, un jeu changé : %q", got)
	}
	if answer := call(http.MethodDelete, "/switch/metadata/orphans/"+renamed+"?disk=Gamebox"); answer.Code != http.StatusOK {
		t.Fatalf("retirer l'orphelin : %d %s", answer.Code, answer.Body.String())
	}
	if answer := call(http.MethodDelete, "/switch/metadata/orphans/"+kept+"?disk=Gamebox"); answer.Code != http.StatusConflict {
		t.Fatalf("le dossier du jeu changé : %d %s", answer.Code, answer.Body.String())
	}
	for _, gameID := range []string{kept, deluxe, "background"} {
		if _, err := os.Stat(filepath.Join(switchDir, gameID)); err != nil {
			t.Fatalf("%s a disparu", gameID)
		}
	}
	if _, err := os.Stat(filepath.Join(switchDir, renamed)); !os.IsNotExist(err) {
		t.Fatal("l'orphelin est resté")
	}

	// Un jeu PS5 en dossier : son empreinte est l'identifiant de `sce_sys/param.json`, pas sa référence — le dossier
	// sous sa référence est orphelin, celui sous son identifiant est le sien.
	ps5Dir := filepath.Join(root, "metadata", "ps5")
	byReference := storage.RomID("ps5", "Astro Bot")
	write(filepath.Join(ps5Dir, "PPSA01325", "manual", "source.json"), `{"title":"Astro Bot"}`)
	write(filepath.Join(ps5Dir, byReference, "manual", "source.json"), `{"title":"Astro Bot"}`)
	if _, err := retro_files.ListCatalog(server.retroRomsRoot(), "ps5", ""); err != nil {
		t.Fatal(err)
	}
	notReady("la liste PS5, l'identifiant pas lu", call(http.MethodGet, "/ps5/metadata/orphans"))
	if got := list("ps5"); got != byReference+"@" {
		t.Fatalf("orphelins PS5 : %q", got)
	}
}

// switchPackageWithTicket — un `.nsp` (PFS0) qui ne range que le ticket de ce jeu : son identifiant se lit sans clé
// (storage.GameTitleID), comme celui des vrais paquets de Gamebox.
func switchPackageWithTicket(titleID string) string {
	names := append([]byte(strings.ToLower(titleID)+"0000000000000005.tik"), 0)
	header := make([]byte, 0x10+0x18)
	copy(header, "PFS0")
	binary.LittleEndian.PutUint32(header[4:], 1)
	binary.LittleEndian.PutUint32(header[8:], uint32(len(names)))
	binary.LittleEndian.PutUint64(header[0x10+8:], 0x2C0)

	return string(append(append(header, names...), make([]byte, 0x2C0)...))
}

// UN JEU SWITCH DONT L'IDENTIFIANT SE LIT DÉSORMAIS (2026-11-15) : le dossier de sa référence n'est pas orphelin. Une
// suppression qui arrive avant toute fiche le déplace sous l'identifiant au lieu de le retirer (409) ; la liste le
// déplace avant de juger, sur le disque comme au dossier du serveur. Seul celui dont l'identifiant a DÉJÀ son dossier
// reste, orphelin : plus aucun jeu ne le porte, et il se retire sans toucher à l'autre.
func TestOrphansMoveAReferenceFolderToItsTitleID(t *testing.T) {
	server, root := listingServer(t)
	server.config.JWTSecret = facadeTestSecret
	disk := testDisk(t, "Gamebox", true)
	withDisks(server, disk)
	router := server.retroRouter()
	admin := statusToken(t, "gino", auth.RoleAdmin)
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("Authorization", "Bearer "+admin)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}
	exists := func(path string) bool {
		_, err := os.Stat(path)

		return err == nil
	}
	write(filepath.Join(disk.Dir, "roms", "switch", "Crash Bandicoot", "Crash Bandicoot.nsp"), switchPackageWithTicket("0100D1B006744000"))
	write(filepath.Join(disk.Dir, "roms", "switch", "Super Mario Odyssey", "Super Mario Odyssey.nsp"), switchPackageWithTicket("0100000000010000"))
	crash, odyssey := storage.RomID("switch", "Crash Bandicoot"), storage.RomID("switch", "Super Mario Odyssey")
	onDisk, onServer := filepath.Join(disk.Dir, "metadata", "switch"), filepath.Join(root, "metadata", "switch")
	write(filepath.Join(onDisk, crash, "igdb", "source.json"), `{"source":"igdb","result":"found","name":"Crash"}`)
	write(filepath.Join(onServer, crash, "manual", "source.json"), `{"title":"Crash, au dossier du serveur"}`)
	write(filepath.Join(onDisk, odyssey, "manual", "source.json"), `{"title":"Le doublon"}`)
	write(filepath.Join(onDisk, "0100000000010000", "manual", "source.json"), `{"title":"Super Mario Odyssey"}`)

	// Les jeux du disque lus, comme après la bibliothèque affichée : la suppression se décide aussitôt.
	if _, err := retro_files.ListCatalog(disk.RomsDir(), "switch", ""); err != nil {
		t.Fatal(err)
	}
	paths, _ := retro_files.CatalogPaths(disk.RomsDir(), "switch")
	for reference, path := range paths {
		metadata.GameKey(path, reference)
	}
	if answer := call(http.MethodDelete, "/switch/metadata/orphans/"+crash+"?disk=Gamebox"); answer.Code != http.StatusConflict {
		t.Fatalf("supprimer le dossier de la référence de Crash : %d %s", answer.Code, answer.Body.String())
	}
	if exists(filepath.Join(onDisk, crash)) || !exists(filepath.Join(onDisk, "0100D1B006744000", "igdb", "source.json")) {
		t.Fatal("la suppression n'a pas déplacé le dossier de Crash sous son identifiant")
	}

	parts := []string{}
	for _, orphan := range listedOrphans(t, untilGamesRead(t, func() *httptest.ResponseRecorder { return call(http.MethodGet, "/switch/metadata/orphans") })) {
		parts = append(parts, orphan.GameID+"@"+orphan.Disk+"="+orphan.Title)
	}
	if got := strings.Join(parts, ","); got != odyssey+"@Gamebox=Le doublon" {
		t.Fatalf("orphelins : %q, attendu le seul doublon d'Odyssey", got)
	}
	if exists(filepath.Join(onServer, crash)) || !exists(filepath.Join(onServer, "0100D1B006744000", "manual", "source.json")) {
		t.Fatal("la liste n'a pas déplacé le dossier de Crash du dossier du serveur")
	}

	if answer := call(http.MethodDelete, "/switch/metadata/orphans/"+odyssey+"?disk=Gamebox"); answer.Code != http.StatusOK {
		t.Fatalf("retirer le doublon : %d %s", answer.Code, answer.Body.String())
	}
	if exists(filepath.Join(onDisk, odyssey)) || !exists(filepath.Join(onDisk, "0100000000010000", "manual", "source.json")) {
		t.Fatal("retirer le doublon a touché au dossier de l'identifiant")
	}
}

// untilGamesRead — une réponse de la liste, relue comme la page la relit tant que le kernel répond 503
// `games_not_ready` : les empreintes se lisent en fond.
func untilGamesRead(t *testing.T, get func() *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	answer := get()
	for deadline := time.Now().Add(10 * time.Second); answer.Code == http.StatusServiceUnavailable; answer = get() {
		if time.Now().After(deadline) {
			t.Fatalf("les jeux ne sont jamais lus : %s", answer.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	return answer
}

// listedOrphans — les dossiers d'une réponse de la liste.
func listedOrphans(t *testing.T, answer *httptest.ResponseRecorder) []metadataOrphan {
	t.Helper()
	if answer.Code != http.StatusOK {
		t.Fatalf("liste : %d %s", answer.Code, answer.Body.String())
	}
	var body struct {
		Data struct {
			Orphans []metadataOrphan `json:"orphans"`
		} `json:"data"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	return body.Data.Orphans
}
