// Package database owns the SQLite file: schema creation (idempotent,
// mirrors the Python service's create_all + title-column patches). The
// whole platform shares one database at a fixed container path
// (/data/gameflex.db, see config.DBPath) — the per-service copies and the
// one-shot migration from gameflex-access.db are gone (2026-08-19).
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"gameflex/credential/internal/config"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        VARCHAR(100) NOT NULL UNIQUE,
    hashed_password VARCHAR(255) NOT NULL,
    display_name    VARCHAR(100) NOT NULL,
    role_level      INTEGER NOT NULL DEFAULT 2,
    created_at      DATETIME,
    created_by_id   INTEGER REFERENCES users(id),
    last_login      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

CREATE TABLE IF NOT EXISTS play_history (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id),
    console_id    VARCHAR(100) NOT NULL,
    reference     VARCHAR(255) NOT NULL,
    title         VARCHAR(255),
    go_session_id VARCHAR(64),
    started_at    DATETIME,
    ended_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_play_history_user ON play_history(user_id);

CREATE TABLE IF NOT EXISTS controller_mappings (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    console_id VARCHAR(100) NOT NULL,
    keyboard   JSON,
    gamepad    JSON,
    updated_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_controller_mappings_user ON controller_mappings(user_id);

CREATE TABLE IF NOT EXISTS key_mappings (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id              INTEGER NOT NULL REFERENCES users(id),
    physical_device_id   VARCHAR(255) NOT NULL,
    physical_device_type VARCHAR(20) NOT NULL,
    controller_name      VARCHAR(50) NOT NULL,
    mapping              JSON NOT NULL,
    updated_at           DATETIME
);
CREATE INDEX IF NOT EXISTS idx_key_mappings_user ON key_mappings(user_id);
CREATE INDEX IF NOT EXISTS idx_key_mappings_device ON key_mappings(physical_device_id);

CREATE TABLE IF NOT EXISTS key_positionings (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    controller_name VARCHAR(50) NOT NULL,
    slot_index      INTEGER NOT NULL,
    is_default      BOOLEAN NOT NULL DEFAULT 0,
    layout          JSON,
    opacity         REAL NOT NULL DEFAULT 1.0,
    updated_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_key_positionings_user ON key_positionings(user_id);

CREATE TABLE IF NOT EXISTS notification_preferences (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER NOT NULL REFERENCES users(id),
    notification_type VARCHAR(30) NOT NULL,
    enabled           BOOLEAN NOT NULL DEFAULT 1,
    updated_at        DATETIME
);
CREATE INDEX IF NOT EXISTS idx_notification_preferences_user ON notification_preferences(user_id);

CREATE TABLE IF NOT EXISTS push_subscriptions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    endpoint   VARCHAR(1024) NOT NULL UNIQUE,
    p256dh     VARCHAR(255) NOT NULL,
    auth       VARCHAR(255) NOT NULL,
    created_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user ON push_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_push_subscriptions_endpoint ON push_subscriptions(endpoint);

-- Jetons de catalogue (chantier 6) — LE LIEN entre deux serveurs. Un jeton
-- donne à un autre serveur (ou à un téléphone) le droit de lire le catalogue
-- et de télécharger un jeu, un BIOS, un cœur. Rien d'autre : ni session, ni
-- sauvegarde, ni administration. Jouer ici depuis là-bas passe par le jeton
-- d'un COMPTE (account_tokens, plus bas).
--
-- Le chantier 20 l'avait retiré au profit du jeton d'un compte ; le porteur
-- l'a rétabli le soir même (« le lien entre 2 serveurs reste comme avant »).
--
-- Le secret n'est pas stocké — seulement son SHA-256. Un jeton est 32
-- octets d'aléa, pas un mot de passe : il n'y a rien à deviner par force
-- brute, donc pas de bcrypt à payer sur chaque requête de téléchargement.
-- Le prefix (les 8 premiers caractères) n'existe que pour que la liste
-- puisse montrer DE QUEL jeton on parle sans jamais le ré-afficher.
--
-- label est obligatoire, et c'est le coeur de la décision : un jeton
-- anonyme ne se révoque qu'en bloc et ne dit jamais qui consomme.
-- scopes dit CE QUE le jeton ouvre : « catalog » (lire le catalogue,
-- télécharger), « devices » (voir la liste des consoles à distance d'ici).
--
-- Lancer une session n'est PAS de la lecture : ça démarre un conteneur avec
-- sa limite mémoire, son bloc de ports ICE et son port alloué — du calcul
-- sur la machine de celui qui prête. Et remoteplay va plus loin encore : il
-- touche une console physique chez lui. D'où un COMPTE pour jouer, jamais ce
-- bout de papier.
CREATE TABLE IF NOT EXISTS federation_tokens (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    label         VARCHAR(100) NOT NULL,
    scopes        VARCHAR(255) NOT NULL DEFAULT 'catalog',
    token_hash    VARCHAR(64) NOT NULL UNIQUE,
    prefix        VARCHAR(12) NOT NULL,
    created_at    DATETIME,
    created_by_id INTEGER REFERENCES users(id),
    last_used_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_federation_tokens_hash ON federation_tokens(token_hash);

-- LE JETON D'UN COMPTE (chantier 20, le porteur, 2026-10-21) — pour JOUER ici
-- depuis un autre serveur. Son propriétaire le recopie dans Réglages › Profil
-- et le colle chez lui, sur la fiche d'un de nos jeux, à la place de
-- l'identifiant et du mot de passe d'autrefois : « un token et c'est bon ».
--
-- Un seul par compte, créé avec lui (le déclencheur ci-dessous : tous les
-- chemins qui créent un compte, d'ici ou du téléphone, en reçoivent un) et
-- DÉSACTIVÉ par défaut. L'administrateur l'active pour une durée
-- (expires_at), le désactive, ou le régénère. Ce qu'il ouvre n'est pas ici :
-- ce sont les consoles du compte (users.consoles) — « le compte gère les
-- accès et le token ne fait que suivre ».
--
-- Stocké tel quel, pas haché : le joueur le recopie depuis son profil, à tout
-- moment ; l'administrateur le coupe d'un geste.
--
-- use_count compte les sessions ouvertes avec lui (« le nombre de fois c'est
-- connecté ») ; last_used_at, la dernière d'entre elles.
CREATE TABLE IF NOT EXISTS account_tokens (
    user_id      INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    token        VARCHAR(64) NOT NULL UNIQUE,
    enabled      INTEGER NOT NULL DEFAULT 0,
    expires_at   DATETIME,
    created_at   DATETIME,
    use_count    INTEGER NOT NULL DEFAULT 0,
    last_used_at DATETIME
);
CREATE TRIGGER IF NOT EXISTS users_account_token AFTER INSERT ON users
BEGIN
    INSERT OR IGNORE INTO account_tokens (user_id, token, created_at)
    VALUES (NEW.id, lower(hex(randomblob(32))), strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime'));
END;

-- L'AUTRE sens de la fédération : les serveurs auxquels NOUS avons accès.
--
-- Ici le jeton est stocké tel quel, pas haché — c'est une clé qu'on doit
-- pouvoir REPRÉSENTER à l'autre serveur à chaque appel, pas un mot de passe
-- qu'on vérifie. Un hachage la rendrait inutilisable.
--
-- Elle n'est pas chiffrée au repos, contrairement aux identifiants TURN
-- (GAMEFLEX_PASSPHRASE_SALT), et c'est un choix : ce que ce jeton ouvre est
-- la lecture du catalogue de quelqu'un d'autre, et lui peut le révoquer
-- d'un clic. Les identifiants Cloudflare, eux, portent hors de GameFlex.
-- Si la parité est souhaitée un jour, c'est ici qu'elle se fait.
-- Les montages distants : un partage WebDAV (ou kDrive, qui EST du WebDAV)
-- monté comme une source de la bibliothèque.
--
-- Table à part des federation_servers, et ce n'est pas de la duplication :
-- un serveur GameFlex a déjà classé ses jeux et rend un catalogue, un
-- partage est un arbre de dossiers qu'on scanne. Les deux n'ont ni les
-- mêmes champs, ni le même client, ni la même façon d'échouer.
--
-- La colonne kind distingue « webdav » de « kdrive » POUR LE FORMULAIRE
-- seulement (pas d'accent grave ici : ce SQL vit dans un littéral Go
-- délimité par des accents graves, et un seul refermerait la chaîne) :
-- l'un demande une adresse, l'autre un identifiant de drive dont on
-- construit l'adresse. Le client est le même.
CREATE TABLE IF NOT EXISTS remote_mounts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          VARCHAR(100) NOT NULL UNIQUE,
    kind          VARCHAR(20)  NOT NULL,
    base_url      VARCHAR(500) NOT NULL,
    -- Le sous-dossier à parcourir. Vide = la racine du partage.
    folder        VARCHAR(255) NOT NULL DEFAULT '',
    username      VARCHAR(255) NOT NULL,
    password      VARCHAR(255) NOT NULL,
    created_at    DATETIME,
    created_by_id INTEGER REFERENCES users(id)
);

-- LES INVITATIONS (chantier 33, le porteur, 2026-10-26) — ce qui voyage par
-- une messagerie À LA PLACE d'un jeton : un code de 128 bits, à usage unique,
-- valable 12 heures. Le serveur d'en face l'échange auprès de nous, de serveur
-- à serveur, contre le vrai jeton : un jeton de catalogue (kind federation,
-- créé à l'échange avec label et scopes) ou le jeton du compte user_id (kind
-- account). La ligne part à l'échange ; un code inconnu, servi ou échu ne
-- vaut rien.
--
-- Seule l'empreinte du code est gardée, comme pour les jetons de catalogue.
-- expires_at en RFC 3339 UTC, comme l'échéance d'un jeton de compte.
CREATE TABLE IF NOT EXISTS invitations (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    kind          VARCHAR(20) NOT NULL,
    code_hash     VARCHAR(64) NOT NULL UNIQUE,
    label         VARCHAR(100) NOT NULL,
    scopes        VARCHAR(255) NOT NULL DEFAULT '',
    user_id       INTEGER REFERENCES users(id) ON DELETE CASCADE,
    expires_at    TEXT NOT NULL,
    created_at    DATETIME,
    created_by_id INTEGER REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS federation_servers (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    link          VARCHAR(255) NOT NULL UNIQUE,
    token         VARCHAR(255) NOT NULL,
    hostname      VARCHAR(100) NOT NULL,
    identity      VARCHAR(100),
    created_at    DATETIME,
    created_by_id INTEGER REFERENCES users(id)
);

-- MON COMPTE CHEZ UN SERVEUR FÉDÉRÉ (chantier 20, le porteur, 2026-10-21) : le
-- jeton qu'un membre d'ici a reçu chez cet ami (Réglages › Profil, là-bas),
-- collé UNE fois sur la fiche d'un de ses jeux — « avec mon compte » : il vaut
-- sur tous ses appareils. Le kernel l'échange contre une session à son nom
-- quand il joue là-bas ; il ne repasse jamais par la page.
--
-- Stocké tel quel, comme le jeton du lien juste au-dessus : il se représente à
-- l'ami à chaque échange. Il part avec le membre, et avec le serveur.
CREATE TABLE IF NOT EXISTS federated_accounts (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id  INTEGER NOT NULL REFERENCES federation_servers(id) ON DELETE CASCADE,
    token      VARCHAR(255) NOT NULL,
    created_at DATETIME,
    PRIMARY KEY (user_id, server_id)
);
`

// Open opens (or creates) the service database and ensures the schema is
// complete. When the database does not exist yet and the Python service's
// file does, the Python file is copied first so users, bcrypt hashes and
// mappings migrate untouched.
func Open() (*sql.DB, error) {
	path := config.DBPath

	// modernc sqlite needs the directory to exist and the file to be
	// created before opening; database/sql would fail with "out of memory"
	// on a missing directory otherwise.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// WAL keeps the three services (credential/storage/remoteplay) coexisting
	// on the shared file; busy_timeout covers transient write overlap — and it
	// comes FIRST: switching to WAL takes a lock, and without a timeout that
	// switch failed outright on a neighbour's lock whenever the stack was
	// recreated (release-storage, "database is locked", 2026-10-02).
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// LES FAVORIS NE SONT PLUS (le porteur, 2026-10-29 : « on n'aura plus
	// cette notion de favoris ») : leur table part, avec ses lignes.
	if _, err := db.Exec(`DROP TABLE IF EXISTS favorites`); err != nil {
		db.Close()
		return nil, fmt.Errorf("drop favorites: %w", err)
	}
	// Same idempotent column patch as the Python service's on_startup:
	// a copied file may predate the `title` column (uuid-catalog era).
	if err := ensureColumn(db, "play_history", "title", "VARCHAR(255)"); err != nil {
		db.Close()
		return nil, err
	}
	// Les consoles d'un compte (chantier 20) : « * », tout, par défaut —
	// c'est l'administrateur qui restreint. Voir gameflex/consoles, grants.go.
	if err := ensureColumn(db, "users", "consoles", "VARCHAR(500) NOT NULL DEFAULT '*'"); err != nil {
		db.Close()
		return nil, err
	}
	// Un jeton pour chaque compte d'avant le déclencheur.
	if _, err := db.Exec(`INSERT INTO account_tokens (user_id, token, created_at)
		SELECT id, lower(hex(randomblob(32))), strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime')
		FROM users WHERE id NOT IN (SELECT user_id FROM account_tokens)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed account tokens: %w", err)
	}
	// LE DOSSIER À PARCOURIR sur un partage.
	//
	// Sans lui, le scan partait de la racine du drive — chez quelqu'un qui y
	// range aussi ses photos et ses documents, c'est très long pour ne rien
	// trouver, et rien ne disait où chercher. Vide = la racine, le
	// comportement d'avant.
	if err := ensureColumn(db, "remote_mounts", "folder", "VARCHAR(255) NOT NULL DEFAULT ''"); err != nil {
		db.Close()
		return nil, err
	}
	// Invite list sorting — existing databases predate the column.
	if err := ensureColumn(db, "users", "last_login", "DATETIME"); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("inspect %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("read %s columns: %w", table, err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s columns: %w", table, err)
	}

	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}
