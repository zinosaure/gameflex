package metadata

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
)

// UN DISQUE MONTÉ EN LECTURE SEULE (un dossier monté ainsi dans un conteneur d'essai, jamais un vrai disque :
// GAMEFLEX_READONLY_DISK le désigne) : l'interrupteur allumé, l'écriture y échoue sur EROFS, que le kernel dit tel
// quel (disk_read_only, api/game_metadata.go) ; coupé, la racine est le dossier du serveur, et la même correction
// s'écrit et se lit.
func TestAReadOnlyDiskMount(t *testing.T) {
	disk := os.Getenv("GAMEFLEX_READONLY_DISK")
	if disk == "" {
		t.Skip("GAMEFLEX_READONLY_DISK : un dossier monté en lecture seule")
	}
	game := ps5Game(t, "Final Fantasy VII Rebirth")
	if err := SetManualText(disk, "ps5", game, "ref", rawChanges(t, `{"title":"Essai"}`)); !errors.Is(err, syscall.EROFS) {
		t.Fatalf("allumé, sur le disque en lecture seule : %v (attendu EROFS)", err)
	}
	server := t.TempDir()
	if err := SetManualText(server, "ps5", game, "ref", rawChanges(t, `{"title":"Essai"}`)); err != nil {
		t.Fatalf("coupé : %v", err)
	}
	if info := Info(context.Background(), Roots{Server: server, Game: server}, nil, "ps5", game, "ref"); info.Title != "Essai" {
		t.Fatalf("coupé, la fiche : %+v", info)
	}
	t.Cleanup(func() { forgetSeries("ps5", "ref") })
}
