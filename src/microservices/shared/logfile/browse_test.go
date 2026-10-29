package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seed(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(LogsRoot(root), filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("20260909/kernel.log", "un\ndeux\ntrois\n")
	write("20260909/141932/retro-nes-001.log", strings.Repeat("ligne\n", 500))
	write("20260909/141932/notes.txt", "pas un journal")

	return root
}

// Le rangement porte l'information : le jour, et l'heure quand c'est une
// session. On ne relit pas le contenu pour le savoir.
func TestListReadsTheLayout(t *testing.T) {
	entries, err := List(seed(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("%d entrées, attendu 2 (le .txt n'est pas un journal) : %+v", len(entries), entries)
	}
	byName := map[string]Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if got := byName["kernel.log"]; got.Day != "20260909" || got.Session != "" {
		t.Errorf("service : jour=%q session=%q, attendu 20260909 et vide", got.Day, got.Session)
	}
	if got := byName["retro-nes-001.log"]; got.Session != "141932" {
		t.Errorf("session : %q, attendu 141932", got.Session)
	}
}

// La fin, pas le début — et sans charger tout le fichier.
func TestTailReturnsTheEnd(t *testing.T) {
	out, err := Tail(seed(t), "20260909/141932/retro-nes-001.log", 10)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Split(out, "\n")); n != 10 {
		t.Fatalf("%d lignes, attendu 10", n)
	}
}

// Le chemin vient d'une requête HTTP. Sans cette garde, le bouton de
// suppression devient une arme.
func TestTraversalIsRefused(t *testing.T) {
	root := seed(t)
	for _, evil := range []string{"../gameflex.db", "20260909/../../secret", "/etc/passwd"} {
		if _, err := Tail(root, evil, 5); err == nil {
			t.Errorf("%q accepté — il devait être refusé", evil)
		}
		if err := Remove(root, evil); err == nil {
			t.Errorf("%q supprimable — il devait être refusé", evil)
		}
	}
}

// Un dossier de session vidé ne doit pas rester : l'écran montrerait une
// session dont on vient d'effacer la trace.
func TestRemoveTakesTheEmptySessionFolder(t *testing.T) {
	root := seed(t)
	if err := Remove(root, "20260909/141932/notes.txt"); err == nil {
		t.Error("un non-journal ne se supprime pas ici")
	}
	if err := os.Remove(filepath.Join(LogsRoot(root), "20260909/141932/notes.txt")); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, "20260909/141932/retro-nes-001.log"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(LogsRoot(root), "20260909/141932")); !os.IsNotExist(err) {
		t.Error("le dossier de session vide devait partir avec son dernier journal")
	}
}

// Un journal est du JSON structuré : le montrer brut redonne à l'écran ce
// qu'un terminal donne déjà.
func TestParseTailSplitsAJsonLine(t *testing.T) {
	lines := ParseTail(`{"time":"2026-09-09T00:48:18.038323015Z","level":"INFO",` +
		`"msg":"controller: button via input","sessionId":"id_60","button":"SQUARE","action":"release"}`)
	if len(lines) != 1 {
		t.Fatalf("%d lignes, attendu 1", len(lines))
	}
	got := lines[0]
	if got.Message != "controller: button via input" {
		t.Errorf("message = %q", got.Message)
	}
	if got.Level != "INFO" {
		t.Errorf("niveau = %q", got.Level)
	}
	if got.Clock != "00:48:18" {
		t.Errorf("heure = %q, attendu 00:48:18", got.Clock)
	}
	// L'ordre d'écriture porte du sens — une map le perdrait.
	if got.Details != "sessionId=id_60 button=SQUARE action=release" {
		t.Errorf("détails = %q", got.Details)
	}
}

// Une ligne qui n'est pas du JSON n'est pas une erreur : c'est un autre
// format qui cohabite dans le même fichier. Ne pas la montrer serait cacher
// précisément celle qui sort de l'ordinaire.
func TestParseTailKeepsWhatItCannotRead(t *testing.T) {
	access := `2026/09/09 00:48:18 "GET http://x/status HTTP/1.1" from 172.18.0.4 - 200`
	lines := ParseTail(access + "\n\n" + `{"msg":"ok","time":"2026-09-09T01:00:00Z"}`)
	if len(lines) != 2 {
		t.Fatalf("%d lignes, attendu 2 (la ligne vide est écartée)", len(lines))
	}
	if lines[0].Raw != access {
		t.Errorf("ligne brute = %q", lines[0].Raw)
	}
	if lines[1].Message != "ok" {
		t.Errorf("seconde ligne = %+v", lines[1])
	}
}

// Les journaux comptent des images et des octets. %v rendrait 7680000000 en
// notation scientifique, et personne ne cherche « 7.68e+09 » dans un filtre.
func TestParseTailKeepsNumbersReadable(t *testing.T) {
	lines := ParseTail(`{"time":"2026-09-09T00:00:00Z","msg":"stats","created":7680000000,"ratio":0.5}`)
	if lines[0].Details != "created=7680000000 ratio=0.5" {
		t.Errorf("détails = %q", lines[0].Details)
	}
}
