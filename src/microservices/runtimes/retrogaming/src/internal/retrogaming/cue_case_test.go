package retrogaming

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGame(t *testing.T, cue string, files ...string) string {
	t.Helper()

	folder := t.TempDir()
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(folder, name), []byte("piste"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cuePath := filepath.Join(folder, "game.cue")
	if err := os.WriteFile(cuePath, []byte(cue), 0o644); err != nil {
		t.Fatal(err)
	}

	return cuePath
}

// Le cas de FF V (PAL) : la fiche dit « .ISO » en capitales, le disque a
// « .iso ». Le cœur doit trouver la piste sous le nom QUE LA FICHE DONNE.
func TestACueNamingItsTrackInAnotherCaseIsRestaged(t *testing.T) {
	cuePath := writeGame(t, "FILE \"FINAL_FANTASY_5_(PAL)_[SCES-13840].ISO\" BINARY\r\n  TRACK 01 MODE2/2352\r\n    INDEX 01 00:00:00\r\n",
		"Final_Fantasy_5_(PAL)_[SCES-13840].iso")

	staged, err := cueWithExactNames(cuePath, t.TempDir())
	if err != nil {
		t.Fatalf("fiche : %v", err)
	}
	if staged == cuePath {
		t.Fatal("la fiche n'a pas été refaite, le cœur ne trouvera pas sa piste")
	}

	track, err := os.ReadFile(filepath.Join(filepath.Dir(staged), "FINAL_FANTASY_5_(PAL)_[SCES-13840].ISO"))
	if err != nil || string(track) != "piste" {
		t.Fatalf("la piste n'est pas lisible sous le nom de la fiche : %v", err)
	}
}

// Une fiche juste ne coûte rien : elle est rendue telle quelle.
func TestAnExactCueIsHandedAsIs(t *testing.T) {
	cuePath := writeGame(t, "FILE \"Game.bin\" BINARY\n  TRACK 01 MODE2/2352\n", "Game.bin")

	staged, err := cueWithExactNames(cuePath, t.TempDir())
	if err != nil || staged != cuePath {
		t.Fatalf("(%q, %v), attendu la fiche d'origine", staged, err)
	}
}

// Un nom introuvable même à la casse près (le CD2 de Gran Turismo 2) : on ne
// devine pas, le cœur dira ce qui manque.
func TestATrulyMissingTrackIsLeftToTheCore(t *testing.T) {
	cuePath := writeGame(t, "FILE \"[PSX] Grand Turismo 2 [CD2] [EUR].img\" BINARY\n", "autre chose.img")

	staged, err := cueWithExactNames(cuePath, t.TempDir())
	if err != nil || staged != cuePath {
		t.Fatalf("(%q, %v), attendu la fiche d'origine", staged, err)
	}
}

// Les noms sans guillemets se lisent aussi.
func TestAnUnquotedFileNameIsRead(t *testing.T) {
	cuePath := writeGame(t, "FILE SYPHON2CD1.BIN BINARY\n", "syphon2cd1.bin")

	staged, err := cueWithExactNames(cuePath, t.TempDir())
	if err != nil || staged == cuePath {
		t.Fatalf("(%q, %v), attendu une fiche refaite", staged, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(staged), "SYPHON2CD1.BIN")); err != nil {
		t.Fatalf("la piste n'est pas lisible sous le nom de la fiche : %v", err)
	}
}

// LE .SBI SUIT LA FICHE REFAITE (LibCrypt, Final Fantasy VIII PAL) : sans
// lui, la protection se déclenche plus loin dans le jeu. Et un .sbi rangé
// sous une autre casse que la fiche suffit à la refaire.
func TestTheSBIFollowsTheRestagedCue(t *testing.T) {
	cuePath := writeGame(t, "FILE \"FF8.BIN\" BINARY\n", "ff8.bin", "game.sbi")

	staged, err := cueWithExactNames(cuePath, t.TempDir())
	if err != nil || staged == cuePath {
		t.Fatalf("(%q, %v), attendu une fiche refaite", staged, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(staged), "game.sbi")); err != nil {
		t.Fatalf("le .sbi n'a pas suivi la fiche : %v", err)
	}

	cuePath = writeGame(t, "FILE \"disc.bin\" BINARY\n", "disc.bin", "GAME.SBI")
	staged, err = cueWithExactNames(cuePath, t.TempDir())
	if err != nil || staged == cuePath {
		t.Fatalf("(%q, %v), un .sbi d'une autre casse doit faire refaire la fiche", staged, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(staged), "game.sbi")); err != nil {
		t.Fatalf("le .sbi n'est pas lisible sous le nom de la fiche : %v", err)
	}

	cuePath = writeGame(t, "FILE \"disc.bin\" BINARY\n", "disc.bin", "game.sbi")
	if staged, err := cueWithExactNames(cuePath, t.TempDir()); err != nil || staged != cuePath {
		t.Fatalf("(%q, %v), une fiche juste et son .sbi restent en place", staged, err)
	}
}
