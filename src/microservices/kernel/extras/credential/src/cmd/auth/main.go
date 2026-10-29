// auth service — absorbs the Python service's user management surface
// (users, auth, history, mappings, push, presence) with an
// identical HTTP contract, so the frontend keeps working unchanged.
package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"gameflex/credential/internal/api"
	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/database"
	"gameflex/credential/internal/presence"
	"gameflex/credential/internal/web_push"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cfg := config.Load()

	// LE MOT DE PASSE ROOT EST FOURNI PAR CELUI QUI INSTALLE : la variable
	// GAMEFLEX_ROOT_PASSWORD du compose, en dev comme dans une installation
	// publique (décidé le 2026-09-15). Sans elle, ce service tirait un mot de
	// passe au hasard et ne l'affichait jamais — une installation neuve
	// s'enfermait dehors (S5). Il refuse désormais de démarrer, en le disant.
	if cfg.RootPassword == "" {
		slog.Error("GAMEFLEX_ROOT_PASSWORD is required: set the root password in this service's environment")
		os.Exit(1)
	}

	db, err := database.Open()
	if err != nil {
		slog.Error("database open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// LE MOT DE PASSE ROOT NE SERT QU'À LA CRÉATION DU COMPTE (le porteur,
	// 2026-10-24 : « utilisé une seule fois pour créer les comptes admin la
	// première fois […] c'est à lui seul de le changer après »). Le remettre
	// depuis la variable se demande : `auth reset-root-password`
	// (`./launch.sh reset-password`), pour qui l'a perdu.
	if len(os.Args) > 1 && os.Args[1] == "reset-root-password" {
		if err := resetRootPassword(db, cfg); err != nil {
			slog.Error("root password reset failed", "error", err)
			os.Exit(1)
		}
		return
	}
	seedRoot(db, cfg)
	api.SealRemoteMounts(db, cfg.JWTSecret)

	// La clé VAPID : celle fournie par l'environnement, sinon celle gardée
	// dans la base — jamais une clé du seul processus, qui casserait chaque
	// abonnement au redémarrage suivant (voir database.StoredVAPIDKey).
	vapidKey := cfg.VAPIDPrivateKey
	if vapidKey == "" {
		vapidKey, err = database.StoredVAPIDKey(db)
		if err != nil {
			slog.Error("vapid key", "error", err)
			os.Exit(1)
		}
	}
	pushClient, err := web_push.NewClient(vapidKey, cfg.VAPIDContactEmail)
	if err != nil {
		slog.Error("web_push init failed", "error", err)
		os.Exit(1)
	}

	registry := presence.NewRegistry()
	server := api.NewServer(cfg, db, registry, pushClient)

	address := fmt.Sprintf(":%d", config.HandlerPort)
	slog.Info("auth service listening", "address", address)
	if err := http.ListenAndServe(address, server); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// seedRoot creates the one root account when none exists, same idempotent
// behaviour as the Python service's on_startup hook.
func seedRoot(db *sql.DB, cfg config.Config) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE role_level = 0`).Scan(&count); err != nil {
		slog.Error("root seed check failed", "error", err)
		return
	}
	if count > 0 {
		return
	}

	// Toujours fourni : main refuse de démarrer sans lui.
	hashed, err := auth.HashPassword(cfg.RootPassword)
	if err != nil {
		slog.Error("root password hashing failed", "error", err)
		return
	}
	if _, err := db.Exec(
		`INSERT INTO users (username, hashed_password, display_name, role_level, created_at) VALUES (?, ?, ?, 0, ?)`,
		cfg.RootUsername, hashed, "Root", time.Now().Format("2006-01-02 15:04:05.999999"),
	); err != nil {
		slog.Error("root seed failed", "error", err)
		return
	}
	slog.Info("seeded root account", "username", cfg.RootUsername)
}

// resetRootPassword remet le compte root sur GAMEFLEX_ROOT_PASSWORD — SUR
// DEMANDE SEULEMENT (`auth reset-root-password`).
//
// Jusqu'au 2026-10-24, cela se faisait à CHAQUE démarrage (écrit le
// 2026-09-12 : l'utilisateur avait changé la variable sans effet, et n'avait
// plus l'ancien mot de passe) : un mot de passe changé depuis l'interface était
// écrasé au redémarrage suivant. Or la variable ne sert qu'à créer le compte,
// et c'est à l'utilisateur de le changer ensuite. Le remède à la perte reste,
// mais il se demande : une commande, pas un démarrage.
//
// La valeur n'est jamais journalisée.
func resetRootPassword(db *sql.DB, cfg config.Config) error {
	if cfg.RootPassword == "" {
		return fmt.Errorf("GAMEFLEX_ROOT_PASSWORD is empty")
	}
	var id int64
	if err := db.QueryRow(
		`SELECT id FROM users WHERE role_level = 0 ORDER BY id LIMIT 1`,
	).Scan(&id); err != nil {
		return fmt.Errorf("no root account: %w", err)
	}
	hashed, err := auth.HashPassword(cfg.RootPassword)
	if err != nil {
		return fmt.Errorf("hashing: %w", err)
	}
	if _, err := db.Exec(`UPDATE users SET hashed_password = ? WHERE id = ?`, hashed, id); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	slog.Warn("root password reset to GAMEFLEX_ROOT_PASSWORD", "username", cfg.RootUsername)

	return nil
}
