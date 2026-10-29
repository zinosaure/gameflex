package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"gameflex/credential/internal/web_push"
)

// LA CLÉ VAPID SURVIT AU REDÉMARRAGE : la même à chaque ouverture de la base,
// et une vraie clé — le client de poussée web l'accepte. Une clé tirée par
// processus cassait tous les abonnements au redémarrage suivant.
func TestTheVAPIDKeyIsKeptAcrossOpenings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.db")
	open := func() *sql.DB {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		return db
	}

	first := open()
	key, err := StoredVAPIDKey(first)
	if err != nil {
		t.Fatalf("première ouverture : %v", err)
	}
	first.Close()

	second := open()
	defer second.Close()
	again, err := StoredVAPIDKey(second)
	if err != nil {
		t.Fatalf("seconde ouverture : %v", err)
	}
	if again != key {
		t.Fatal("la clé a changé d'une ouverture à l'autre : les abonnements pris seraient perdus")
	}

	if _, err := web_push.NewClient(key, "admin@example.com"); err != nil {
		t.Fatalf("le client de poussée web refuse la clé gardée : %v", err)
	}
}
