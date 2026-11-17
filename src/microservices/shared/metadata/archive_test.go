package metadata

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// L'ARCHIVE D'UN DOSSIER DE MÉTADONNÉES (le Catalogue, 2026-11-16) : ses fichiers sous `metadata/<console>/<gameid>/`,
// leur contenu intact ; ni un fichier caché, ni un lien qui mènerait ailleurs ; un jeu sans dossier n'en a pas.
func TestGameArchiveKeepsTheFolderAndNothingElse(t *testing.T) {
	root := t.TempDir()
	gamePath := filepath.Join(root, "roms", "nes", "Jeu.nes")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(gamePath, "le jeu")
	outside := filepath.Join(root, "secret.txt")
	write(outside, "hors du dossier")

	if _, _, err := GameArchiveFolder(root, "nes", gamePath, "ref-1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("un jeu sans métadonnées a un dossier : %v", err)
	}

	gameID := GameKey(gamePath, "ref-1")
	folder := filepath.Join(root, "metadata", "nes", gameID)
	write(filepath.Join(folder, "libretro", "source.json"), `{"name":"Jeu"}`)
	write(filepath.Join(folder, "manual", "cover.webp"), "une image")
	write(filepath.Join(folder, ".marqueur"), "caché")
	write(filepath.Join(folder, ".cache", "x"), "caché aussi")
	if err := os.Symlink(outside, filepath.Join(folder, "lien.txt")); err != nil {
		t.Fatal(err)
	}

	found, foundID, err := GameArchiveFolder(root, "nes", gamePath, "ref-1")
	if err != nil || found != folder || foundID != gameID {
		t.Fatalf("le dossier du jeu = %q %q %v, attendu %q %q", found, foundID, err, folder, gameID)
	}
	var archive bytes.Buffer
	if err := WriteGameArchive(&archive, found, "nes", foundID); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	contents := map[string]string{}
	for _, file := range reader.File {
		names = append(names, file.Name)
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(opened)
		opened.Close()
		contents[file.Name] = string(content)
	}
	sort.Strings(names)
	prefix := "metadata/nes/" + gameID + "/"
	if len(names) != 2 || names[0] != prefix+"libretro/source.json" || names[1] != prefix+"manual/cover.webp" {
		t.Fatalf("les entrées de l'archive : %v", names)
	}
	if contents[prefix+"libretro/source.json"] != `{"name":"Jeu"}` || contents[prefix+"manual/cover.webp"] != "une image" {
		t.Fatalf("le contenu de l'archive : %v", contents)
	}
}
