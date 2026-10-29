package api

import (
	"os"
	"testing"
)

// LE LISTING D'UN TEST N'EST JAMAIS CELUI DU SERVEUR. Les tests tournent dans
// le conteneur du kernel de dev, où `listingCacheDir` désigne le vrai stockage
// chaud : sans ce détour, un test qui écrit une section (un cœur téléchargé,
// des paramètres enregistrés) la réécrirait dans le listing que la pile de dev
// sert à ses écrans et à ses amis.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gameflex-listing-")
	if err != nil {
		panic(err)
	}
	listingCacheDir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// freshListing — un cache de listings à soi, pour un test qui lit ou écrit
// celui de ce serveur : un autre test ne lui laisse pas le sien.
func freshListing(t *testing.T) {
	t.Helper()
	previous := listingCacheDir
	listingCacheDir = t.TempDir()
	t.Cleanup(func() { listingCacheDir = previous })
}
