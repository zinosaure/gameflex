package main

// Le mot de passe root de la variable ne sert qu'à la CRÉATION du compte ; le
// remettre se demande (`auth reset-root-password`). Écrit le 2026-09-12 pour
// une réconciliation à chaque démarrage, revu le 2026-10-24 : c'est à
// l'utilisateur de changer ce mot de passe, et un redémarrage ne doit plus
// l'écraser.

import (
	"database/sql"
	"path/filepath"
	"testing"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/database"
)

// withRoot ouvre une base neuve où root est né avec le mot de passe donné —
// database.Open monte le schéma, seedRoot crée le compte.
func withRoot(t *testing.T, rootPassword string) *sql.DB {
	t.Helper()
	config.DBPath = filepath.Join(t.TempDir(), "auth.db")
	db, err := database.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedRoot(db, config.Config{RootUsername: "root", RootPassword: rootPassword})
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

// UN REDÉMARRAGE NE TOUCHE PAS AU MOT DE PASSE : la variable a créé le compte,
// l'utilisateur l'a changé depuis ; un nouveau démarrage, même avec une autre
// valeur dans la variable, le laisse tel quel.
func TestARestartKeepsTheRootPasswordTheUserSet(t *testing.T) {
	db := withRoot(t, "celui-de-la-premiere-fois")
	hashed, err := auth.HashPassword("change-depuis-la-page")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET hashed_password = ? WHERE role_level = 0`, hashed); err != nil {
		t.Fatal(err)
	}

	seedRoot(db, config.Config{RootUsername: "root", RootPassword: "celui-de-la-premiere-fois"})

	if !auth.VerifyPassword("change-depuis-la-page", rootHash(t, db)) {
		t.Fatal("un redémarrage a écrasé le mot de passe changé depuis la page")
	}
}

// LA REMISE, SUR DEMANDE, IMPOSE LA VARIABLE ET RÉVOQUE L'ANCIEN : le remède
// pour qui a perdu son mot de passe.
func TestResetImposesTheEnvPassword(t *testing.T) {
	db := withRoot(t, "ancien-perdu")
	if err := resetRootPassword(db, config.Config{RootUsername: "root", RootPassword: "le-nouveau"}); err != nil {
		t.Fatal(err)
	}
	if auth.VerifyPassword("ancien-perdu", rootHash(t, db)) {
		t.Fatal("l'ancien mot de passe ouvre encore le compte après la remise")
	}
	if !auth.VerifyPassword("le-nouveau", rootHash(t, db)) {
		t.Fatal("le mot de passe de la variable n'ouvre pas le compte")
	}
	// Qui avait l'ancien perd aussi ses sessions (chantier 55).
	var generation int
	if err := db.QueryRow(`SELECT session_generation FROM users WHERE role_level = 0`).Scan(&generation); err != nil || generation != 1 {
		t.Fatalf("génération des sessions de root après la remise : %d (%v), attendu 1", generation, err)
	}
	if err := resetRootPassword(db, config.Config{RootUsername: "root", RootPassword: ""}); err == nil {
		t.Fatal("une remise sans valeur doit être refusée")
	}
}
