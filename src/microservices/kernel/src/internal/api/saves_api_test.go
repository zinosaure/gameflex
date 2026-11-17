package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/docker"
	"gameflex/storage/saves"
)

func savesTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()

	return &Server{config: config.Config{JWTSecret: facadeTestSecret}, storageRoot: root}, root
}

func savesRequest(t *testing.T, handler http.Handler, method, target, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

func kh3Save(t *testing.T, tree, content string) string {
	t.Helper()
	path := filepath.Join(tree, "streamex", "ps4", "data", "home", "1000", "savedata", "CUSA07010", "SAVEDATA00", "data.bin")
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s : %v", path, err)
	}

	return string(raw)
}

// UN ENVOI N'ÉCRIT QUE DANS UNE COPIE, au nom de l'expéditeur, même depuis
// une session ouverte par un autre serveur ; restaurer, faire un backup et
// effacer restent des gestes d'ici.
func TestStreamexSavesTravelIntoCopiesOnly(t *testing.T) {
	server, root := savesTestServer(t)
	router := server.savesRouter()
	here := testToken(t, auth.Claims{Username: "gino"})
	fromPeer := testToken(t, auth.Claims{Username: "gino", Fed: "1"})

	principal := filepath.Join(root, "saves", "gino")
	kept := kh3Save(t, principal, "kh3 d'ici")
	elsewhere := t.TempDir()
	kh3Save(t, elsewhere, "kh3 de la prod")
	var archive bytes.Buffer
	if err := saves.WriteArchive(&archive, elsewhere, "ps4", "CUSA07010"); err != nil {
		t.Fatal(err)
	}

	game := "/streamex/game?console=ps4&id=CUSA07010"
	if answer := savesRequest(t, router, http.MethodPost, game, fromPeer, archive.Bytes()); answer.Code != http.StatusBadRequest {
		t.Fatalf("envoi sans source : %d", answer.Code)
	}
	if answer := savesRequest(t, router, http.MethodPost, game+"&source=gameflex.example.org", fromPeer, archive.Bytes()); answer.Code != http.StatusOK {
		t.Fatalf("envoi : %d %s", answer.Code, answer.Body)
	}
	if fileText(t, kept) != "kh3 d'ici" {
		t.Fatal("l'envoi a écrit dans le principal")
	}
	received := filepath.Join(principal, "sources", "gameflex.example.org", "streamex", "ps4", "data", "home", "1000", "savedata", "CUSA07010", "SAVEDATA00", "data.bin")
	if fileText(t, received) != "kh3 de la prod" {
		t.Fatal("la copie n'a pas reçu l'envoi")
	}
	if answer := savesRequest(t, router, http.MethodPost, game+"&source=local", fromPeer, archive.Bytes()); answer.Code != http.StatusOK {
		t.Fatalf("envoi d'un serveur nommé local : %d", answer.Code)
	}
	if _, err := os.Stat(filepath.Join(principal, "sources", "local")); !os.IsNotExist(err) {
		t.Fatal("un serveur nommé « local » a écrit dans les backups")
	}

	var listed struct {
		Data struct {
			Self    string   `json:"self"`
			Sources []string `json:"sources"`
		} `json:"data"`
	}
	answer := savesRequest(t, router, http.MethodGet, "/sources", here, nil)
	if json.Unmarshal(answer.Body.Bytes(), &listed); listed.Data.Self == "" || strings.Join(listed.Data.Sources, ",") != "gameflex.example.org,local_" {
		t.Fatalf("sources : %s", answer.Body)
	}

	restore := []byte(`{"source":"gameflex.example.org","console":"ps4","ids":["CUSA07010"]}`)
	if answer := savesRequest(t, router, http.MethodPost, "/streamex/restore", fromPeer, restore); answer.Code != http.StatusForbidden {
		t.Fatalf("restaurer depuis un autre serveur : %d", answer.Code)
	}
	if answer := savesRequest(t, router, http.MethodPost, "/streamex/restore", here, restore); answer.Code != http.StatusOK || strings.Contains(answer.Body.String(), "error") {
		t.Fatalf("restaurer : %d %s", answer.Code, answer.Body)
	}
	if fileText(t, kept) != "kh3 de la prod" {
		t.Fatal("non restauré")
	}
	backup := filepath.Join(principal, "sources", "local", "streamex", "ps4", "data", "home", "1000", "savedata", "CUSA07010", "SAVEDATA00", "data.bin")
	if fileText(t, backup) != "kh3 d'ici" {
		t.Fatal("l'ancienne version n'est pas dans local")
	}

	answer = savesRequest(t, router, http.MethodGet, "/streamex?source=local", here, nil)
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"CUSA07010"`) || strings.Contains(answer.Body.String(), "xbox360") {
		t.Fatalf("liste de local : %d %s", answer.Code, answer.Body)
	}
	if answer := savesRequest(t, router, http.MethodGet, game+"&source=gameflex.example.org", fromPeer, nil); answer.Code != http.StatusOK || answer.Body.Len() == 0 {
		t.Fatalf("archive d'une copie : %d", answer.Code)
	}
	if answer := savesRequest(t, router, http.MethodGet, "/streamex/game?console=ps4&id=CUSA00001", here, nil); answer.Code != http.StatusNotFound {
		t.Fatalf("jeu absent : %d", answer.Code)
	}
	if answer := savesRequest(t, router, http.MethodGet, "/streamex?source=../bob", here, nil); answer.Code != http.StatusBadRequest {
		t.Fatalf("source hors du compte : %d", answer.Code)
	}

	if answer := savesRequest(t, router, http.MethodDelete, game, here, nil); answer.Code != http.StatusBadRequest {
		t.Fatalf("effacer le principal d'ici : %d", answer.Code)
	}
	if answer := savesRequest(t, router, http.MethodDelete, game+"&source=local", here, nil); answer.Code != http.StatusOK {
		t.Fatalf("effacer une copie : %d", answer.Code)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("la copie n'est pas effacée")
	}
}

// streamexSaveFile — un fichier de sauvegarde d'une console récente, `relative`
// sous `streamex/<console>/` d'un rangement.
func streamexSaveFile(t *testing.T, tree, console, relative, content string) {
	t.Helper()
	path := filepath.Join(tree, "streamex", console, filepath.FromSlash(relative))
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ticketNSP — un paquet Switch qui ne porte que le ticket de son jeu
// (`<rightsid>.tik`) : son identifiant se lit sans les clés de la console.
func ticketNSP(titleID string) []byte {
	name := titleID + "0000000000000000.tik"
	header := make([]byte, 0x10+0x18)
	copy(header, "PFS0")
	binary.LittleEndian.PutUint32(header[4:], 1)
	binary.LittleEndian.PutUint32(header[8:], uint32(len(name)+1))

	return append(append(header, name...), 0)
}

// UNE SAUVEGARDE PORTE LE NOM DE SON JEU (le porteur, 2026-11-16 : « Oui il
// faut les nom des jeux et la console dans mes sauvegardes ») : celui du jeu de
// la bibliothèque — ou d'un disque interne — dont l'identifiant de titre est le
// sien ; sinon le titre qu'elle se donne ; sinon rien, et la page montre son
// identifiant.
func TestStreamexSavesCarryTheirGameTitle(t *testing.T) {
	server, root := savesTestServer(t)
	disk := t.TempDir()
	server.internalDisks = func(context.Context) []docker.InternalDisk {
		return []docker.InternalDisk{{Name: "Gamebox", Dir: disk, Present: true}}
	}
	router := server.savesRouter()
	token := testToken(t, auth.Claims{Username: "claude"})

	astro := filepath.Join(root, "roms", "ps5", "Astro Bot", "sce_sys", "param.json")
	os.MkdirAll(filepath.Dir(astro), 0o755)
	os.WriteFile(astro, []byte(`{"titleId":"PPSA12345"}`), 0o644)
	finalFantasy := filepath.Join(disk, "roms", "switch", "Final Fantasy", "FINAL FANTASY I[01000EA014150000].nsp")
	os.MkdirAll(filepath.Dir(finalFantasy), 0o755)
	os.WriteFile(finalFantasy, ticketNSP("01000EA014150000"), 0o644)

	tree := filepath.Join(root, "saves", "claude")
	streamexSaveFile(t, tree, "ps5", "savedata/PPSA12345/sdimg_SAVEDATA00", "astro")
	streamexSaveFile(t, tree, "ps5", "savedata/PPSA99999/sdimg_SAVEDATA00", "un jeu absent")
	streamexSaveFile(t, tree, "switch", "nand/user/save/0000000000000000/CB4433B032A5AC306FD0DCCA511C514B/01000EA014150000/save.dat", "ff1")
	streamexSaveFile(t, tree, "wiiu", "mlc01/usr/save/00050000/10101e00/meta/meta.xml", `<shortname_en type="string">New SUPER MARIO BROS. U</shortname_en>`)

	answer := savesRequest(t, router, http.MethodGet, "/streamex", token, nil)
	var listed struct {
		Data struct {
			Games []struct {
				Console   string `json:"console"`
				ID        string `json:"id"`
				Title     string `json:"title"`
				Reference string `json:"reference"`
				Disk      string `json:"disk"`
			} `json:"games"`
		} `json:"data"`
	}
	if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &listed) != nil {
		t.Fatalf("liste : %d %s", answer.Code, answer.Body)
	}
	found := map[string]string{}
	for _, game := range listed.Data.Games {
		found[game.Console+"/"+game.ID] = game.Title + "|" + game.Disk + "|" + map[bool]string{true: "jeu", false: "-"}[game.Reference != ""]
	}
	want := map[string]string{
		"ps5/PPSA12345":           "Astro Bot||jeu",
		"ps5/PPSA99999":           "||-",
		"switch/01000EA014150000": "Final Fantasy|Gamebox|jeu",
		"wiiu/10101e00":           "New SUPER MARIO BROS. U||-",
	}
	for key, expected := range want {
		if found[key] != expected {
			t.Errorf("%s : %q, attendu %q", key, found[key], expected)
		}
	}
}

// LE RÉTRO, PAR SES ROUTES DE TOUJOURS : l'envoi d'un emplacement va dans une
// copie avec le titre du jeu, et la restauration le remet dans le principal.
func TestStateUploadsLandInACopyAndRestore(t *testing.T) {
	server, root := savesTestServer(t)
	retro := server.retroRouter()
	router := server.savesRouter()
	here := testToken(t, auth.Claims{Username: "gino"})
	fromPeer := testToken(t, auth.Claims{Username: "gino", Fed: "1"})

	slot := "/savestate/file?consoleId=nes&reference=metroid-ref&slot=1"
	if answer := savesRequest(t, retro, http.MethodPost, slot, fromPeer, []byte("state")); answer.Code != http.StatusBadRequest {
		t.Fatalf("envoi sans source : %d", answer.Code)
	}
	if answer := savesRequest(t, retro, http.MethodPost, slot+"&source=prod&title=Metroid", fromPeer, []byte("state")); answer.Code != http.StatusOK {
		t.Fatalf("envoi : %d %s", answer.Code, answer.Body)
	}
	principal := filepath.Join(root, "saves", "gino", "states", "nes", "metroid-ref_slot1.state")
	if _, err := os.Stat(principal); !os.IsNotExist(err) {
		t.Fatal("l'envoi a écrit dans le principal")
	}
	answer := savesRequest(t, retro, http.MethodGet, "/savestate/user?source=prod", here, nil)
	if !strings.Contains(answer.Body.String(), `"title":"Metroid"`) {
		t.Fatalf("le titre de la copie : %s", answer.Body)
	}
	if answer := savesRequest(t, retro, http.MethodGet, slot+"&source=prod", here, nil); answer.Body.String() != "state" {
		t.Fatalf("lire la copie : %d %s", answer.Code, answer.Body)
	}

	restore := []byte(`{"source":"prod","items":[{"consoleId":"nes","reference":"metroid-ref","slot":1},{"consoleId":"nes","reference":"metroid-ref","slot":2}]}`)
	if answer := savesRequest(t, router, http.MethodPost, "/states/restore", fromPeer, restore); answer.Code != http.StatusForbidden {
		t.Fatalf("restaurer depuis un autre serveur : %d", answer.Code)
	}
	answer = savesRequest(t, router, http.MethodPost, "/states/restore", here, restore)
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"slot":2,"error":"not found"`) {
		t.Fatalf("restaurer : %d %s", answer.Code, answer.Body)
	}
	if fileText(t, principal) != "state" {
		t.Fatal("non restauré")
	}
	if answer := savesRequest(t, router, http.MethodPost, "/states/backup", here, []byte(`{"items":[{"consoleId":"nes","reference":"metroid-ref","slot":1}]}`)); answer.Code != http.StatusOK {
		t.Fatalf("backup : %d", answer.Code)
	}
	if fileText(t, filepath.Join(root, "saves", "gino", "sources", "local", "states", "nes", "metroid-ref_slot1.state")) != "state" {
		t.Fatal("le backup n'est pas dans local")
	}
}

// UN BACKUP EMPORTE SA NOTE (saves/notes.go) : écrite avec lui, relue,
// corrigée, vidée ; jugée avant le geste ; jamais depuis une session venue
// d'un autre serveur.
func TestABackupCarriesItsNote(t *testing.T) {
	server, root := savesTestServer(t)
	router := server.savesRouter()
	here := testToken(t, auth.Claims{Username: "gino"})
	fromPeer := testToken(t, auth.Claims{Username: "gino", Fed: "1"})
	kh3Save(t, filepath.Join(root, "saves", "gino"), "kh3")
	body := func(fields map[string]any) []byte {
		raw, _ := json.Marshal(fields)

		return raw
	}

	tooLong := body(map[string]any{"console": "ps4", "ids": []string{"CUSA07010"}, "note": strings.Repeat("x", saves.MaxNoteRunes+1)})
	if answer := savesRequest(t, router, http.MethodPost, "/streamex/backup", here, tooLong); answer.Code != http.StatusUnprocessableEntity {
		t.Fatalf("note trop longue : %d", answer.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "saves", "gino", "sources", "local")); !os.IsNotExist(err) {
		t.Fatal("la note refusée, le backup s'est fait quand même")
	}
	backup := body(map[string]any{"console": "ps4", "ids": []string{"CUSA07010"}, "note": "avant la mise à jour"})
	if answer := savesRequest(t, router, http.MethodPost, "/streamex/backup", here, backup); answer.Code != http.StatusOK || strings.Contains(answer.Body.String(), "error") {
		t.Fatalf("backup : %d %s", answer.Code, answer.Body)
	}
	if listed := savesRequest(t, router, http.MethodGet, "/notes", here, nil).Body.String(); !strings.Contains(listed, `"note":"avant la mise à jour"`) ||
		!strings.Contains(listed, `"account":"gino"`) || !strings.Contains(listed, `"id":"CUSA07010"`) {
		t.Fatalf("liste : %s", listed)
	}

	edit := body(map[string]any{"kind": "streamex", "console": "ps4", "id": "CUSA07010", "note": "avant le DLC"})
	if answer := savesRequest(t, router, http.MethodPut, "/notes", fromPeer, edit); answer.Code != http.StatusForbidden {
		t.Fatalf("depuis un autre serveur : %d", answer.Code)
	}
	if answer := savesRequest(t, router, http.MethodPut, "/notes", here, edit); answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"note":"avant le DLC"`) {
		t.Fatalf("corrigée : %d %s", answer.Code, answer.Body)
	}
	absent := body(map[string]any{"kind": "states", "console": "nes", "reference": "ici", "slot": 1, "note": "x"})
	if answer := savesRequest(t, router, http.MethodPut, "/notes", here, absent); answer.Code != http.StatusNotFound {
		t.Fatalf("sans backup : %d", answer.Code)
	}
	unknown := body(map[string]any{"kind": "states", "console": "atari2600x", "reference": "ici", "slot": 1, "note": "x"})
	if answer := savesRequest(t, router, http.MethodPut, "/notes", here, unknown); answer.Code != http.StatusBadRequest {
		t.Fatalf("console inconnue : %d", answer.Code)
	}
	cleared := body(map[string]any{"kind": "streamex", "console": "ps4", "id": "CUSA07010", "note": " "})
	if answer := savesRequest(t, router, http.MethodPut, "/notes", here, cleared); answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `"item":null`) {
		t.Fatalf("vidée : %d %s", answer.Code, answer.Body)
	}
}
