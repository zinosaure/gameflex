package database

import (
	"crypto/ecdh"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
)

// StoredVAPIDKey — la clé VAPID de ce service, tirée au sort à la première
// ouverture et gardée ensuite dans la base (2026-10-15).
//
// Un navigateur s'abonne contre la clé PUBLIQUE qu'on lui donne : la changer
// rend tous les abonnements déjà pris inutilisables. Le serveur la tirait
// pourtant au hasard à chaque démarrage quand GAMEFLEX_VAPID_PRIVATE_KEY
// n'était pas fourni — ce qui était le cas partout — et chaque redémarrage du
// service d'identité cassait silencieusement les notifications push de tout
// le monde. Le téléphone, lui, gardait déjà la sienne ainsi ; les deux
// partagent maintenant ce code.
func StoredVAPIDKey(db *sql.DB) (string, error) {
	// La table est créée ici : sur le téléphone elle ne fait pas partie du
	// schéma, et compter dessus ferait échouer la toute première ouverture
	// d'un appareil neuf.
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`,
	); err != nil {
		return "", fmt.Errorf("vapid: settings table: %w", err)
	}

	var stored string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = 'vapid_private_key'`).Scan(&stored)
	if err == nil && stored != "" {
		return stored, nil
	}

	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("vapid: generate key: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(key.Bytes())

	if _, err := db.Exec(
		`INSERT INTO settings (key, value) VALUES ('vapid_private_key', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, encoded,
	); err != nil {
		return "", fmt.Errorf("vapid: store key: %w", err)
	}

	return encoded, nil
}
