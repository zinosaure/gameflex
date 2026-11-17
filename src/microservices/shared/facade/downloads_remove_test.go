package facade

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Une suppression suivie dit son genre, son étape et sa progression, et le dossier part en entier.
func TestRemoveTreeReportsItsProgress(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Un jeu")
	if err := os.MkdirAll(filepath.Join(game, "sce_sys"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, size := range map[string]int{"eboot.bin": 3000, "sce_sys/param.sfo": 1000} {
		if err := os.WriteFile(filepath.Join(game, name), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var downloads Downloads
	release := make(chan struct{})
	id, err := downloads.Start(DownloadJob{Owner: "gino", Label: "Un jeu", Key: "remove:x", Kind: "remove",
		Run: func(ctx context.Context) (int, []byte) {
			if err := RemoveTree(ctx, game); err != nil {
				return 500, ErrorBody("delete_failed", err.Error())
			}
			<-release

			return 200, OKBody(map[string]bool{"removed": true})
		}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		list := downloads.List("gino")
		if len(list) == 1 && list[0].Received == 4000 {
			if list[0].Kind != "remove" || list[0].Phase != "remove" || list[0].Total != 4000 || list[0].DownloadID != id {
				t.Fatalf("entry = %+v", list[0])
			}

			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("progress never reached 4000: %+v", list)
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	if _, err := os.Stat(game); !os.IsNotExist(err) {
		t.Fatalf("the game is still there: %v", err)
	}
}
