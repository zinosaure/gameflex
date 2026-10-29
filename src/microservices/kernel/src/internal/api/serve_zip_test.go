package api

import (
	"archive/zip"
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func zipNames(t *testing.T, body []byte) []string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("archive illisible: %v", err)
	}
	names := make([]string, 0, len(reader.File))
	for _, f := range reader.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

// Un jeu arborescent se téléchargeait amputé : les sous-dossiers étaient
// ignorés sans un mot, donc 31 Mo au lieu de 3,2 Go (constaté le 2026-09-10
// sur le premier jeu PS5).
func TestZippingAGameFolderKeepsItsSubfolders(t *testing.T) {
	game := t.TempDir()
	if err := os.MkdirAll(filepath.Join(game, "Media", "Modules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for path, content := range map[string]string{
		"eboot.bin":                 "boot",
		"Media/level0":              "niveau",
		"Media/Modules/PS5Util.prx": "module",
		".hidden":                   "ignoré",
	} {
		if err := os.WriteFile(filepath.Join(game, filepath.FromSlash(path)), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	recorder := httptest.NewRecorder()
	serveDirectoryAsZip(recorder, httptest.NewRequest("GET", "/x", nil), game, "Richman 11")

	got := zipNames(t, recorder.Body.Bytes())
	want := []string{"Media/Modules/PS5Util.prx", "Media/level0", "eboot.bin"}
	if len(got) != len(want) {
		t.Fatalf("archive incomplète : %v (attendu %v)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("archive %v, attendu %v", got, want)
		}
	}
}

// Un dossier PLAT (le cas historique : un .cue et ses .bin) doit produire
// exactement la même archive qu'avant — le chemin relatif y est le nom du
// fichier.
func TestZippingAFlatFolderIsUnchanged(t *testing.T) {
	game := t.TempDir()
	for _, name := range []string{"disc.cue", "track01.bin", "track02.bin"} {
		if err := os.WriteFile(filepath.Join(game, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	recorder := httptest.NewRecorder()
	serveDirectoryAsZip(recorder, httptest.NewRequest("GET", "/x", nil), game, "Un jeu")

	got := zipNames(t, recorder.Body.Bytes())
	want := []string{"disc.cue", "track01.bin", "track02.bin"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("archive %v, attendu %v", got, want)
		}
	}
}

// HEAD ne doit rien écrire : la page interroge la taille/le nom avant de
// lancer un téléchargement de plusieurs gigaoctets.
func TestHeadDoesNotStreamTheArchive(t *testing.T) {
	game := t.TempDir()
	if err := os.WriteFile(filepath.Join(game, "eboot.bin"), []byte("boot"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	recorder := httptest.NewRecorder()
	serveDirectoryAsZip(recorder, httptest.NewRequest("HEAD", "/x", nil), game, "Richman 11")

	if recorder.Body.Len() != 0 {
		t.Fatalf("HEAD a renvoyé %d octets de corps", recorder.Body.Len())
	}
	if got := recorder.Header().Get("Content-Disposition"); got != `attachment; filename="Richman 11.zip"` {
		t.Fatalf("Content-Disposition inattendu: %q", got)
	}
}
