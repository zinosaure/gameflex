// Package config loads the service configuration from GAMEFLEX_* environment
// variables, mirroring the sessions service's conventions. Every secret has
// a process-local fallback that logs a warning (same spirit as the Python
// service: never silently usable defaults in production).
package config

import (
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"os"
)

// HandlerPort est fixé par la disposition du conteneur — la décision de base
// partagée (2026-08-19) l'a sorti de l'environnement.
const HandlerPort = 8080

// DBPath — une VARIABLE et non plus une constante depuis le 2026-09-01.
//
// Ce service tourne désormais à deux endroits : dans son conteneur, où le
// chemin est celui du volume, et DANS L'APPLICATION ANDROID, où la base vit
// avec la bibliothèque de jeux. Le téléphone a le même service d'identité que
// le serveur — un seul compte, root, qui se connecte tout seul — et le
// réimplémenter aurait donné deux comportements à faire coïncider à la main.
//
// Reste une constante pour tout le monde sauf celui qui la change avant
// d'ouvrir : cmd/auth ne la touche pas et garde le chemin du conteneur.
var DBPath = "/data/gameflex.db"

type Config struct {
	JWTSecret         string
	InternalSecret    string
	RootUsername      string
	RootPassword      string
	VAPIDPrivateKey   string
	VAPIDContactEmail string
	CommitSHA         string
}

func envOr(name, fallback string) string {
	if raw := os.Getenv(name); raw != "" {
		return raw
	}
	return fallback
}

func Load() Config {
	config := Config{
		RootUsername:      envOr("GAMEFLEX_ROOT_USERNAME", "root"),
		RootPassword:      os.Getenv("GAMEFLEX_ROOT_PASSWORD"),
		VAPIDContactEmail: envOr("GAMEFLEX_VAPID_CONTACT_EMAIL", "admin@example.com"),
		CommitSHA:         envOr("GITHUB_COMMIT_SHA", "dev"),
	}

	// JWT secret: must stay stable across restarts and MUST match the one
	// that issued existing client tokens (the Python service used the same
	// JWT_SECRET). A per-process random fallback only exists so a fork can
	// boot without env; it invalidates every token on each restart.
	config.JWTSecret = os.Getenv("GAMEFLEX_JWT_SECRET")
	if config.JWTSecret == "" {
		config.JWTSecret = RandomToken(32)
		slog.Warn("GAMEFLEX_JWT_SECRET not set — generated a random one for this process only, all existing sessions will be invalidated on restart")
	}

	// Secret de service à service — le kernel s'en sert pour faire vérifier
	// le jeton d'un compte venu d'un autre serveur (chantier 20) : lui possède
	// les fichiers, ce service possède l'identité. Même en-tête
	// X-Internal-Secret que tout le plan interne (internalCaller). Absent,
	// le plan interne refuse tout plutôt que de s'ouvrir : un secret vide qui
	// accepte serait une porte ouverte.
	config.InternalSecret = os.Getenv("GAMEFLEX_INTERNAL_SECRET")
	if config.InternalSecret == "" {
		slog.Warn("GAMEFLEX_INTERNAL_SECRET not set — every /internal route (account tokens, mounts, federated servers) will refuse every call")
	}

	// The VAPID key: this one when provided, otherwise the one kept in the
	// database (cmd/auth, database.StoredVAPIDKey) — never a per-process key,
	// which broke every push subscription on the next restart.
	config.VAPIDPrivateKey = os.Getenv("GAMEFLEX_VAPID_PRIVATE_KEY")

	return config
}

// RandomToken returns n random bytes as a base64url string (secrets that
// must be stable across restarts, like JWT_SECRET and VAPID keys).
func RandomToken(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		panic(err) // entropy failure is unrecoverable at boot
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}
