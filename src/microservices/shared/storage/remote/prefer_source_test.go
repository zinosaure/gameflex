package remote

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// La source choisie (chantier 61) part en `?source=` avec chaque fichier demandé ; sans choix, rien n'est ajouté.
func TestPreferSourceAsksForTheChosenSource(t *testing.T) {
	asked := ""
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RawQuery
		_, _ = w.Write([]byte("ROM"))
	}))
	defer peer.Close()
	client, _ := New(peer.URL, "jeton")

	for _, c := range []struct{ source, want string }{{"", ""}, {"3", "source=3"}} {
		rom, err := client.PreferSource(c.source).OpenRom("nes", "abc")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(rom)
		rom.Close()
		if asked != c.want {
			t.Fatalf("source %q : la requête porte %q, attendu %q", c.source, asked, c.want)
		}
	}
}
