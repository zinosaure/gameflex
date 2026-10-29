package embed

// Le mot de passe de root sur un appareil : révoquer le défaut publié, et n'en
// rendre que de l'aléatoire. Écrit le 2026-09-12 avec le correctif — la faille
// (« GameFlex » connu de tous, sur un kernel qui écoute tout le réseau local)
// méritait une sentinelle, pas seulement une relecture.

import (
	"database/sql"
	"path/filepath"
	"testing"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/database"
)

// freshDB ouvre une base neuve avec un root déjà créé — l'état d'une
// installation qui vient de démarrer. database.Open monte le schéma lui-même.
func freshDB(t *testing.T) *sql.DB {
	t.Helper()
	config.DBPath = filepath.Join(t.TempDir(), "identity.db")
	db, err := database.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := seedRoot(db, config.Config{RootUsername: "root"}); err != nil {
		t.Fatalf("seedRoot: %v", err)
	}
	return db
}

func rootHash(t *testing.T, db *sql.DB) string {
	t.Helper()
	var hash string
	if err := db.QueryRow(
		`SELECT hashed_password FROM users WHERE role_level = 0`,
	).Scan(&hash); err != nil {
		t.Fatalf("lire le hash de root: %v", err)
	}
	return hash
}

// LE MOT DE PASSE PUBLIÉ EST RÉVOQUÉ AU DÉMARRAGE.
//
// Un appareil installé avant le 2026-09-12 porte root = "GameFlex" et son
// marqueur. Le démarrage suivant doit rendre ce mot de passe inutilisable,
// sans quoi la faille survivrait à la mise à jour.
func TestRevokePublishedDefaultClosesTheHole(t *testing.T) {
	db := freshDB(t)

	// L'état d'un appareil d'AVANT le correctif.
	published, err := auth.HashPassword("GameFlex")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE users SET hashed_password = ? WHERE role_level = 0`, published,
	); err != nil {
		t.Fatalf("poser le défaut publié: %v", err)
	}
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`,
	); err != nil {
		t.Fatalf("table settings: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO settings (key, value) VALUES ('root_password_seeded', '1')`,
	); err != nil {
		t.Fatalf("marqueur: %v", err)
	}

	// La prémisse : avant révocation, « GameFlex » ouvre bien. Sans ça, le
	// test ne prouverait rien.
	if !auth.VerifyPassword("GameFlex", rootHash(t, db)) {
		t.Fatal("prémisse fausse : « GameFlex » n'ouvrait déjà pas le compte")
	}

	if err := revokePublishedDefault(db); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// Après : « GameFlex » ne marche plus, et le marqueur a disparu (donc on
	// ne rescramble pas au démarrage d'après).
	if auth.VerifyPassword("GameFlex", rootHash(t, db)) {
		t.Fatal("« GameFlex » ouvre encore le compte root après révocation")
	}
	var marker string
	if err := db.QueryRow(
		`SELECT value FROM settings WHERE key = 'root_password_seeded'`,
	).Scan(&marker); err == nil {
		t.Fatalf("le marqueur root_password_seeded est resté (=%q)", marker)
	}
}

// UNE INSTALLATION NEUVE N'EST PAS TOUCHÉE.
//
// Pas de marqueur : root est né avec un secret aléatoire (seedRoot), et la
// révocation ne doit rien changer — sinon elle rescramblerait à chaque
// démarrage le mot de passe que le porteur vient de se donner.
func TestRevokeLeavesAFreshInstallAlone(t *testing.T) {
	db := freshDB(t)
	before := rootHash(t, db)

	if err := revokePublishedDefault(db); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if rootHash(t, db) != before {
		t.Fatal("la révocation a changé le mot de passe d'une installation neuve")
	}
}

// LE MOT DE PASSE RENDU EST ALÉATOIRE, PAS UNE CONSTANTE.
//
// Le cœur du correctif : ResetRootPassword ne remet plus « GameFlex ». Deux
// appels donnent deux valeurs différentes, non vides, dont aucune n'est la
// constante publiée — et la dernière ouvre réellement le compte.
func TestResetRootPasswordIsRandom(t *testing.T) {
	db := freshDB(t)
	service := &Service{DB: db}

	first, err := service.ResetRootPassword()
	if err != nil {
		t.Fatalf("premier reset: %v", err)
	}
	second, err := service.ResetRootPassword()
	if err != nil {
		t.Fatalf("second reset: %v", err)
	}

	for _, password := range []string{first, second} {
		if password == "" {
			t.Fatal("mot de passe vide")
		}
		if password == "GameFlex" {
			t.Fatal("ResetRootPassword rend encore la constante publiée")
		}
	}
	if first == second {
		t.Fatal("deux resets ont rendu le même mot de passe — ce n'est pas aléatoire")
	}
	if !auth.VerifyPassword(second, rootHash(t, db)) {
		t.Fatal("le mot de passe rendu n'ouvre pas le compte root")
	}
}
