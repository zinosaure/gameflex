package api

import (
	"database/sql"
	"log/slog"

	"gameflex/credential/internal/secretbox"
)

// SealRemoteMounts scelle les identifiants de partage encore en clair (#459) :
// tous l'étaient avant le 2026-10-24. Une fois, au démarrage ; ensuite rien ne
// s'écrit plus en clair. Sans secret, rien ne se scelle, et le service le dit.
// Le serveur (cmd/auth) et le téléphone (embed) l'appellent.
func SealRemoteMounts(db *sql.DB, secret string) {
	rows, err := db.Query(`SELECT id, password FROM remote_mounts`)
	if err != nil {
		slog.Error("sealing share credentials: read failed", "error", err)
		return
	}
	type pending struct {
		id    int64
		plain string
	}
	todo := []pending{}
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.id, &item.plain); err == nil && !secretbox.Sealed(item.plain) {
			todo = append(todo, item)
		}
	}
	rows.Close()

	for _, item := range todo {
		sealed, err := secretbox.Seal(secret, item.plain)
		if err != nil {
			slog.Error("sealing share credentials failed", "error", err)
			return
		}
		if _, err := db.Exec(`UPDATE remote_mounts SET password = ? WHERE id = ?`, sealed, item.id); err != nil {
			slog.Error("sealing share credentials: update failed", "mount_id", item.id, "error", err)
			return
		}
	}
	if len(todo) > 0 {
		slog.Info("share credentials sealed with the GameFlex secret", "count", len(todo))
	}
}
