package webdav

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gameflex/storage"
)

// Une vraie réponse de serveur WebDAV, avec ses préfixes d'espace de noms et
// le dossier interrogé qui se rend lui-même en premier — les deux pièges du
// protocole. Les préfixes sont volontairement DIFFÉRENTS de ceux qu'on
// envoie : un serveur choisit les siens, et matcher sur « D: » aurait marché
// contre un seul.
const multistatusBody = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:lp1="DAV:">
  <d:response>
    <d:href>/roms/ps1/</d:href>
    <d:propstat><d:prop><lp1:resourcetype><d:collection/></lp1:resourcetype></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
  <d:response>
    <d:href>/roms/ps1/Ace%20Combat%202.cue</d:href>
    <d:propstat><d:prop><lp1:resourcetype/><lp1:getcontentlength>75</lp1:getcontentlength></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
  <d:response>
    <d:href>/roms/ps1/disques/</d:href>
    <d:propstat><d:prop><lp1:resourcetype><d:collection/></lp1:resourcetype></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status></d:propstat>
  </d:response>
</d:multistatus>`

func TestListReadsAMultistatus(t *testing.T) {
	var seenDepth, seenMethod, seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod, seenDepth = r.Method, r.Header.Get("Depth")
		user, pass, _ := r.BasicAuth()
		seenAuth = user + ":" + pass
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, multistatusBody)
	}))
	defer server.Close()

	source, err := New(Config{Name: "kdrive", BaseURL: server.URL, Username: "marc", Password: "app-pass", Client: server.Client()})
	if err != nil {
		t.Fatalf("montage: %v", err)
	}

	entries, err := source.List("roms/ps1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if seenMethod != "PROPFIND" || seenDepth != "1" {
		t.Fatalf("requête = %s profondeur %q, attendu PROPFIND profondeur 1", seenMethod, seenDepth)
	}
	if seenAuth != "marc:app-pass" {
		t.Fatalf("authentification = %q", seenAuth)
	}

	// Le dossier interrogé ne se compte pas lui-même, sinon le scanner
	// descendrait indéfiniment dans le même dossier.
	if len(entries) != 2 {
		t.Fatalf("2 entrées attendues (le dossier lui-même exclu), %d : %+v", len(entries), entries)
	}

	byName := map[string]storage.Entry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	// Le nom est DÉCODÉ : un jeu s'appelle « Ace Combat 2.cue », pas
	// « Ace%20Combat%202.cue » — et le scanner juge sur l'extension.
	game, ok := byName["Ace Combat 2.cue"]
	if !ok || game.Dir || game.Size != 75 {
		t.Fatalf("le fichier attendu manque ou est mal lu : %+v", byName)
	}
	if folder, ok := byName["disques"]; !ok || !folder.Dir {
		t.Fatalf("le sous-dossier attendu manque ou n'est pas un dossier : %+v", byName)
	}
}

func TestListRejectsBadCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	source, _ := New(Config{Name: "kdrive", BaseURL: server.URL, Client: server.Client()})
	if _, err := source.List(""); err != ErrUnauthorized {
		t.Fatalf("erreur = %v, attendu ErrUnauthorized", err)
	}
}

// Un dossier absent n'est pas une panne : la bibliothèque doit survivre à un
// dossier renommé chez quelqu'un d'autre, comme côté local.
func TestMissingFolderIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	source, _ := New(Config{Name: "kdrive", BaseURL: server.URL, Client: server.Client()})
	entries, err := source.List("nulle/part")
	if err != nil || len(entries) != 0 {
		t.Fatalf("attendu vide sans erreur, got %+v / %v", entries, err)
	}
}

// Un chemin qui remonte au-dessus du montage donnerait accès à TOUT le
// partage. Le confinement est vérifié, pas supposé — même règle que la
// source locale, où un test avait déjà démenti l'inverse.
func TestPathCannotEscapeTheMount(t *testing.T) {
	source, _ := New(Config{Name: "kdrive", BaseURL: "https://example.test/roms"})

	if _, err := source.resolve("../../etc/passwd"); err != storage.ErrOutsideSource {
		t.Fatalf("erreur = %v, attendu ErrOutsideSource", err)
	}
}

func TestOpenReadsTheFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}
		_, _ = io.WriteString(w, "FILE-BEGIN")
	}))
	defer server.Close()

	source, _ := New(Config{Name: "kdrive", BaseURL: server.URL, Client: server.Client()})
	body, err := source.Open("roms/nes/Mario.nes")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer body.Close()

	content, _ := io.ReadAll(body)
	if string(content) != "FILE-BEGIN" {
		t.Fatalf("contenu = %q", content)
	}
}

// Écrire crée les dossiers manquants : un partage neuf est vide, et faire
// échouer la première sauvegarde là-dessus n'aiderait personne.
func TestPutCreatesMissingFolders(t *testing.T) {
	var methods []string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut {
			content, _ := io.ReadAll(r.Body)
			body = string(content)
		}
		if r.Method == "MKCOL" {
			w.WriteHeader(http.StatusMethodNotAllowed) // « existe déjà »
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	source, _ := New(Config{Name: "kdrive", BaseURL: server.URL, Client: server.Client()})
	if err := source.Put("saves/nes/Mario.state1", strings.NewReader("STATE")); err != nil {
		t.Fatalf("put: %v", err)
	}

	if body != "STATE" {
		t.Fatalf("corps écrit = %q", body)
	}
	// Un MKCOL par segment, puis le PUT.
	want := []string{"MKCOL /saves", "MKCOL /saves/nes", "PUT /saves/nes/Mario.state1"}
	if len(methods) != len(want) {
		t.Fatalf("requêtes = %v, attendu %v", methods, want)
	}
	for i, method := range want {
		if methods[i] != method {
			t.Fatalf("requêtes = %v, attendu %v", methods, want)
		}
	}
}

// La sonde dit ce qu'il y a DANS le dossier demandé — pas seulement qu'on
// atteint le serveur. « Joignable » ne dit pas si on a mis le doigt au bon
// endroit, et c'est précisément la question qu'on se pose après avoir ajouté
// un kDrive.
func TestProbeReportsWhatItSees(t *testing.T) {
	var asked string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, multistatusBody)
	}))
	defer server.Close()

	seen, err := Probe(Config{
		Name: "kdrive", BaseURL: server.URL, Username: "marc", Password: "app-pass",
		Client: server.Client(),
	}, "roms/ps1")
	if err != nil {
		t.Fatalf("sonde : %v", err)
	}

	// Le dossier demandé, pas la racine : sans ça on annoncerait « ça marche »
	// en ayant regardé ailleurs.
	// AVEC SA BARRE FINALE : l'intention de ce test — regarder le dossier
	// demandé et non la racine — est intacte, et la barre est désormais
	// exigée. Sans elle, un serveur qui range ses dossiers proprement redirige
	// (301) et le client Go suit la redirection en GET, ce qui rend une page
	// HTML au lieu du XML (mesuré le 2026-09-18, Apache mod_dav).
	if asked != "/roms/ps1/" {
		t.Fatalf("dossier interrogé = %q", asked)
	}
	if seen.Files != 1 {
		t.Fatalf("1 fichier attendu, %d", seen.Files)
	}
	if len(seen.Folders) != 1 || seen.Folders[0] != "disques" {
		t.Fatalf("sous-dossiers = %v", seen.Folders)
	}
}

// Des identifiants refusés ne se corrigent pas comme un dossier absent :
// l'appelant doit pouvoir les distinguer pour le dire.
func TestProbeSurfacesRefusedCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	if _, err := Probe(Config{
		Name: "kdrive", BaseURL: server.URL, Client: server.Client(),
	}, ""); err != ErrUnauthorized {
		t.Fatalf("erreur = %v, attendu ErrUnauthorized", err)
	}
}

// 401 et 403 ne disent PAS la même chose, et les confondre envoie chercher au
// mauvais endroit : l'un rejette des identifiants, l'autre refuse le service
// entier — un kDrive dont le WebDAV n'est pas activé répond 403 à tout, avec
// ou sans mot de passe (constaté en vrai le 2026-09-02).
func TestForbiddenIsNotTheSameAsRefusedCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	source, _ := New(Config{Name: "kdrive", BaseURL: server.URL, Client: server.Client()})
	if _, err := source.List(""); err != ErrForbidden {
		t.Fatalf("erreur = %v, attendu ErrForbidden", err)
	}
	if _, err := Probe(Config{
		Name: "kdrive", BaseURL: server.URL, Client: server.Client(),
	}, ""); err != ErrForbidden {
		t.Fatalf("sonde = %v, attendu ErrForbidden", err)
	}
}
