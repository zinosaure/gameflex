package webdav

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"gameflex/storage"
)

// LE POINT QUI COMPTE : un partage WebDAV se scanne comme un disque.
//
// Rien n'est ajouté au scanner — c'est tout l'intérêt de l'interface Source,
// et ce test le prouve plutôt que de l'affirmer : on monte un faux serveur,
// on scanne, et on obtient un jeu classé par console.
func TestScannerReadsAWebdavShare(t *testing.T) {
	tree := map[string]string{
		"/": `<d:multistatus xmlns:d="DAV:"><d:response><d:href>/</d:href>
			<d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
			<d:response><d:href>/nes/</d:href>
			<d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
			</d:multistatus>`,
		// LES CLÉS PORTENT LEUR BARRE FINALE, comme les demande le client :
		// un PROPFIND de collection s'adresse à « /nes/ » et non à « /nes »,
		// sans quoi un vrai serveur redirige (301) et le client Go suit la
		// redirection en dégradant la méthode en GET — on reçoit alors une page
		// HTML au lieu du XML (mesuré le 2026-09-18 sur un Apache mod_dav).
		//
		// Ce faux serveur étant une table indexée par chemin EXACT, une clé
		// sans barre rendait 404, que List traite comme « dossier disparu » :
		// le scan trouvait zéro jeu sans qu'aucune erreur ne remonte.
		"/nes/": `<d:multistatus xmlns:d="DAV:"><d:response><d:href>/nes/</d:href>
			<d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
			<d:response><d:href>/nes/Super%20Mario%20Bros.nes</d:href>
			<d:propstat><d:prop><d:resourcetype/><d:getcontentlength>40976</d:getcontentlength></d:prop></d:propstat></d:response>
			</d:multistatus>`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := tree[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	source, err := New(Config{Name: "kdrive", BaseURL: server.URL, Client: server.Client()})
	if err != nil {
		t.Fatalf("montage: %v", err)
	}

	games, err := storage.ScanConvention(source, "")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("1 jeu attendu, %d : %+v", len(games), games)
	}
	game := games[0]
	if game.Console != "nes" || game.Title != "Super Mario Bros" || game.Kind != storage.KindWebDAV {
		t.Fatalf("jeu mal classé : %+v", game)
	}
	if game.Path != "kdrive/nes/Super Mario Bros.nes" {
		t.Fatalf("chemin virtuel = %q", game.Path)
	}
}
