package api

import (
	"archive/zip"
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
	"gameflex/metadata"
	"gameflex/storage"
)

// archiveEntries — les noms des entrées d'une archive .zip reçue.
func archiveEntries(t *testing.T, body []byte) []string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("une archive illisible : %v", err)
	}
	names := []string{}
	for _, file := range reader.File {
		names = append(names, file.Name)
	}

	return names
}

// ticketIn — le ticket que rend l'émission (`{data: {ticket}}`), ou un échec du test.
func ticketIn(t *testing.T, answer *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Data struct {
			Ticket string `json:"ticket"`
		} `json:"data"`
	}
	if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &body) != nil || body.Data.Ticket == "" {
		t.Fatalf("l'émission du ticket = %d %s", answer.Code, answer.Body.String())
	}

	return body.Data.Ticket
}

// L'ARCHIVE DES MÉTADONNÉES D'UN JEU D'ICI PART PAR LE NAVIGATEUR (le Catalogue, 2026-11-16), par le vrai routeur : un
// ticket émis sous un compte (sans jeton, 401), consommé UNE fois sans en-tête ; l'archive nomme le jeu et garde
// `metadata/<console>/<gameid>/…` ; un jeu sans métadonnées n'a pas de ticket (404 `no_metadata`) ; le ticket d'un jeu
// n'ouvre pas ses métadonnées.
func TestGameMetadataArchiveOfAGameHere(t *testing.T) {
	local, root := listingServer(t, "roms/ps3/Jeu.iso", "roms/ps3/Nu.iso")
	local.config = config.Config{JWTSecret: facadeTestSecret}
	// Un autre contenu : l'identifiant d'un jeu se lit dans son contenu (metadata.GameKey), et deux jeux identiques
	// partageraient leur dossier.
	writeUnder(t, root, "roms/ps3/Nu.iso", "un autre jeu")
	game := storage.RomID("ps3", "Jeu.iso")
	gamePath := filepath.Join(root, "roms", "ps3", "Jeu.iso")
	if err := metadata.SetManualText(root, "ps3", gamePath, game, map[string]json.RawMessage{"genres": json.RawMessage(`["Action"]`)}); err != nil {
		t.Fatal(err)
	}
	router := local.retroRouter()
	member := statusToken(t, "marc", auth.RoleAdmin+1)
	call := func(method, path, bearer string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		return recorder
	}

	if anonymous := call(http.MethodPost, "/ps3/roms/"+game+"/metadata/ticket", ""); anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("un ticket sans compte = %d, attendu 401", anonymous.Code)
	}
	nonce := ticketIn(t, call(http.MethodPost, "/ps3/roms/"+game+"/metadata/ticket", member))
	archive := call(http.MethodGet, "/downloads/metadata/"+nonce, "")
	if archive.Code != http.StatusOK || archive.Header().Get("Content-Type") != "application/zip" ||
		!strings.Contains(archive.Header().Get("Content-Disposition"), `filename="Jeu - metadata.zip"`) {
		t.Fatalf("l'archive = %d %v %s", archive.Code, archive.Header(), archive.Body.String())
	}
	prefix := "metadata/ps3/" + metadata.GameKey(gamePath, game) + "/"
	entries := archiveEntries(t, archive.Body.Bytes())
	if len(entries) == 0 {
		t.Fatal("une archive vide, pour un jeu dont la fiche est corrigée")
	}
	for _, name := range entries {
		if !strings.HasPrefix(name, prefix) {
			t.Fatalf("une entrée hors de %s : %v", prefix, entries)
		}
	}
	if again := call(http.MethodGet, "/downloads/metadata/"+nonce, ""); again.Code != http.StatusForbidden {
		t.Fatalf("le même ticket, deux fois = %d, attendu 403", again.Code)
	}

	bare := call(http.MethodPost, "/ps3/roms/"+storage.RomID("ps3", "Nu.iso")+"/metadata/ticket", member)
	if bare.Code != http.StatusNotFound || !strings.Contains(bare.Body.String(), "no_metadata") {
		t.Fatalf("un jeu sans métadonnées = %d %s, attendu 404 no_metadata", bare.Code, bare.Body.String())
	}
	if absent := call(http.MethodPost, "/ps3/roms/"+storage.RomID("ps3", "Absent.iso")+"/metadata/ticket", member); absent.Code != http.StatusNotFound {
		t.Fatalf("un jeu absent = %d, attendu 404", absent.Code)
	}
	romTicket := ticketIn(t, call(http.MethodPost, "/ps3/roms/"+game+"/ticket", member))
	if crossed := call(http.MethodGet, "/downloads/metadata/"+romTicket, ""); crossed.Code != http.StatusForbidden {
		t.Fatalf("le ticket d'un jeu ouvre ses métadonnées = %d, attendu 403", crossed.Code)
	}
}

// L'ARCHIVE D'UN JEU D'UN PAIR SE LIT CHEZ LUI ET SE RELAIE (le Catalogue, 2026-11-16), par deux vrais routeurs : sa fiche
// corrigée et sa jaquette dans l'archive ; un de ses jeux sans métadonnées, un jeton qui n'ouvre pas son catalogue :
// pas de ticket ; rien ne s'écrit chez le kernel qui relaie.
func TestGameMetadataArchiveOfAPeerGame(t *testing.T) {
	surface, game, diskGame := sheetPeer(t)
	client, call := sheetClient(t, surface.URL, "catalogue")

	nonce := ticketIn(t, call(http.MethodPost, "/ps3/roms/srv3%3A"+game+"/metadata/ticket", ""))
	archive := call(http.MethodGet, "/downloads/metadata/"+nonce, "")
	if archive.Code != http.StatusOK || archive.Header().Get("Content-Type") != "application/zip" ||
		!strings.Contains(archive.Header().Get("Content-Disposition"), "Jeu - metadata.zip") {
		t.Fatalf("l'archive relayée = %d %v %s", archive.Code, archive.Header(), archive.Body.String())
	}
	entries := strings.Join(archiveEntries(t, archive.Body.Bytes()), " ")
	if !strings.Contains(entries, "metadata/ps3/") || !strings.Contains(entries, "cover") {
		t.Fatalf("l'archive relayée n'a pas la fiche du pair : %s", entries)
	}

	if bare := call(http.MethodPost, "/ps3/roms/srv3%3A"+diskGame+"/metadata/ticket", ""); bare.Code != http.StatusNotFound {
		t.Fatalf("un jeu du pair sans métadonnées = %d %s, attendu 404", bare.Code, bare.Body.String())
	}
	_, devicesOnly := sheetClient(t, surface.URL, "consoles")
	if denied := devicesOnly(http.MethodPost, "/ps3/roms/srv3%3A"+game+"/metadata/ticket", ""); denied.Code != http.StatusForbidden {
		t.Fatalf("un jeton qui n'ouvre que ses consoles = %d %s, attendu 403", denied.Code, denied.Body.String())
	}
	if _, err := os.Stat(filepath.Join(client.storageRoot, "metadata")); !os.IsNotExist(err) {
		t.Fatalf("relayer l'archive d'un jeu fédéré a écrit chez le kernel qui relaie : %v", err)
	}
}
