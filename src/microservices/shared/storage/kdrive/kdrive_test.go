package kdrive

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gameflex/storage"
)

// Un faux kDrive : un arbre en mémoire, servi comme l'API le fait — par
// NUMÉRO, jamais par chemin. C'est tout l'intérêt du test : vérifier qu'on
// traduit correctement, puisque le scanner ne parle que chemins.
func fakeDrive(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()

	// racine (0) → « gameflex » (10) → « nes » (20) → « Metroid.nes » (30)
	children := map[string]string{
		"": `[{"id":10,"name":"gameflex","type":"dir"},
		      {"id":11,"name":"Photos","type":"dir"}]`,
		"10": `[{"id":20,"name":"nes","type":"dir"}]`,
		"20": `[{"id":30,"name":"Metroid.nes","type":"file","size":131072},
		        {"id":31,"name":"notes.txt","type":"file","size":12}]`,
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer jeton-de-marc" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"result":"error","error":{"code":"not_authorized"}}`)

			return
		}
		*calls = append(*calls, r.URL.Path)

		if strings.HasSuffix(r.URL.Path, "/download") {
			_, _ = io.WriteString(w, "ROM-BYTES")

			return
		}

		// /2/drive/42/files            → la racine
		// /2/drive/42/files/<id>/files → un dossier
		id := ""
		if parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/"); len(parts) == 6 {
			id = parts[4]
		}
		body, known := children[id]
		if !known {
			body = "[]"
		}
		_, _ = io.WriteString(w, fmt.Sprintf(`{"result":"success","data":%s}`, body))
	}))
}

func TestListWalksPathsIntoFileIds(t *testing.T) {
	var calls []string
	server := fakeDrive(t, &calls)
	defer server.Close()
	baseURL = server.URL

	source, err := New(Config{
		Name: "kdrive", DriveID: "42", Token: "jeton-de-marc",
		Folder: "gameflex", Client: server.Client(),
	})
	if err != nil {
		t.Fatalf("montage : %v", err)
	}

	entries, err := source.List("nes")
	if err != nil {
		t.Fatalf("list : %v", err)
	}

	// Le dossier « nes » vit sous « gameflex », lui-même sous la racine :
	// deux traductions avant de pouvoir lister.
	byName := map[string]storage.Entry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	if game, ok := byName["Metroid.nes"]; !ok || game.Dir || game.Size != 131072 {
		t.Fatalf("le jeu attendu manque ou est mal lu : %+v", byName)
	}

	// LA MÉMOIRE : relister ne redescend pas l'arbre. Sans elle, chaque
	// dossier d'une bibliothèque se paierait la descente entière.
	before := len(calls)
	if _, err := source.List("nes"); err != nil {
		t.Fatalf("second list : %v", err)
	}
	if added := len(calls) - before; added != 1 {
		t.Fatalf("%d requêtes pour relister, 1 attendue : %v", added, calls[before:])
	}
}

func TestOpenReadsThroughTheTree(t *testing.T) {
	var calls []string
	server := fakeDrive(t, &calls)
	defer server.Close()
	baseURL = server.URL

	source, _ := New(Config{
		Name: "kdrive", DriveID: "42", Token: "jeton-de-marc",
		Folder: "gameflex", Client: server.Client(),
	})

	body, err := source.Open("nes/Metroid.nes")
	if err != nil {
		t.Fatalf("open : %v", err)
	}
	defer body.Close()

	content, _ := io.ReadAll(body)
	if string(content) != "ROM-BYTES" {
		t.Fatalf("contenu = %q", content)
	}
}

// Un jeton refusé se corrige en changeant de jeton : il doit se distinguer
// d'un dossier absent, qui se corrige ailleurs.
func TestRefusedTokenIsItsOwnError(t *testing.T) {
	var calls []string
	server := fakeDrive(t, &calls)
	defer server.Close()
	baseURL = server.URL

	source, _ := New(Config{
		Name: "kdrive", DriveID: "42", Token: "mauvais", Client: server.Client(),
	})
	if _, err := source.List(""); err != ErrUnauthorized {
		t.Fatalf("erreur = %v, attendu ErrUnauthorized", err)
	}
}

// Un dossier absent rend une liste vide, pas une panne : la bibliothèque doit
// survivre à un dossier renommé chez quelqu'un d'autre.
func TestMissingFolderIsNotAnError(t *testing.T) {
	var calls []string
	server := fakeDrive(t, &calls)
	defer server.Close()
	baseURL = server.URL

	source, _ := New(Config{
		Name: "kdrive", DriveID: "42", Token: "jeton-de-marc", Client: server.Client(),
	})
	entries, err := source.List("nulle/part")
	if err != nil || len(entries) != 0 {
		t.Fatalf("attendu vide sans erreur, got %+v / %v", entries, err)
	}
}

// La sonde dit ce qu'il y a à la racine du montage — la même réponse que
// celle du WebDAV, pour que l'écran n'ait pas à savoir qui lui parle.
func TestProbeReportsTheMountRoot(t *testing.T) {
	var calls []string
	server := fakeDrive(t, &calls)
	defer server.Close()
	baseURL = server.URL

	seen, err := Probe(Config{
		Name: "kdrive", DriveID: "42", Token: "jeton-de-marc",
		Folder: "gameflex/nes", Client: server.Client(),
	})
	if err != nil {
		t.Fatalf("sonde : %v", err)
	}
	if seen.Files != 2 || len(seen.Folders) != 0 {
		t.Fatalf("vu = %+v", seen)
	}
}

// Une panne PASSAGÈRE ne doit pas perdre un parcours de centaines de
// requêtes ; un refus, lui, ne s'arrangera pas en insistant.
func TestTransientFailureIsRetriedButRefusalIsNot(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)

			return
		}
		_, _ = io.WriteString(w, `{"result":"success","data":[{"id":1,"name":"roms","type":"dir"}]}`)
	}))
	defer server.Close()
	baseURL = server.URL

	source, _ := New(Config{Name: "kdrive", DriveID: "42", Token: "jeton", Client: server.Client()})
	entries, err := source.List("")
	if err != nil || len(entries) != 1 {
		t.Fatalf("un 500 passager doit être réessayé : %+v / %v (%d appels)", entries, err, calls)
	}

	// Un jeton refusé : une seule tentative, pas deux.
	calls = 0
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer refusing.Close()
	baseURL = refusing.URL

	refused, _ := New(Config{Name: "kdrive", DriveID: "42", Token: "faux", Client: refusing.Client()})
	if _, err := refused.List(""); err != ErrUnauthorized {
		t.Fatalf("erreur = %v, attendu ErrUnauthorized", err)
	}
	if calls != 1 {
		t.Fatalf("%d appels pour un refus — insister martèlerait le serveur d'en face", calls)
	}
}
