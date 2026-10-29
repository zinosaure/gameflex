package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
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
