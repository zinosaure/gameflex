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
-- télécharger, voir les consoles à distance d'ici), « metadata-providers » (la recherche des fiches interroge aussi nos comptes
-- IGDB et SteamGridDB pour son porteur, chantier 57). consoles (chantier 66,
-- colonne ajoutée plus bas) dit ce que les comptes venus par lui lancent.
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
-- LES CONNEXIONS FAITES AVEC CE JETON (#781, le porteur, 2026-11-18 : « on peut voir l'état d'un token mais on ne sait
-- pas sur quel lien/appareils c'est utilisé ») : une ligne par serveur ou appareil qui l'a échangé contre une session —
-- le nom qu'il se donne (from, déclaré par lui) et l'adresse d'où il est venu (celle que le proxy a vue) —, la première
-- et la dernière fois, et combien. Régénérer le jeton les efface : elles parlaient de l'ancien.
CREATE TABLE IF NOT EXISTS account_token_connections (
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    address       TEXT NOT NULL,
    first_used_at DATETIME NOT NULL,
    last_used_at  DATETIME NOT NULL,
    use_count     INTEGER NOT NULL DEFAULT 1,
    member        TEXT NOT NULL DEFAULT '',
    kind          TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, name, address)
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
-- Elle n'est pas chiffrée au repos, et c'est un choix : ce que ce jeton
-- ouvre est la lecture du catalogue de quelqu'un d'autre, et lui peut le
-- révoquer d'un clic.

-- LES COMPTES DES SERVICES DE MÉTADONNÉES (chantier 49, décision du porteur
-- du 2026-11-11 : « dans la base de données on peut mettre les comptes ») —
-- IGDB et SteamGridDB. Une ligne par service ; fields est le
-- JSON de ses champs, scellé comme un mot de passe de partage : il entre par
-- la page d'administration, sert au kernel, et ne ressort jamais par l'API.
CREATE TABLE IF NOT EXISTS metadata_services (
    service       VARCHAR(40) PRIMARY KEY,
    fields        TEXT NOT NULL,
    updated_at    DATETIME,
    updated_by_id INTEGER REFERENCES users(id)
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
--
-- DEUX SORTES DE PLUS, QU'UNE PERSONNE OUVRE ELLE-MÊME (chantier 55) : join (un
-- nouveau membre choisit son mot de passe) et reset (un administrateur l'a
-- réinitialisé) — le lien <launcher>/join/<code>, 24 heures, une fois, un seul
-- vivant par membre (user_id), label étant son nom affiché. La fédération ne les
-- voit pas (member_links.go).
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

-- LES FAVORIS D'UN COMPTE (le porteur, 2026-11-16 : « remplace le bouton
-- download this rom par add to my favorite ») : un jeu par sa console et sa
-- référence, celle que la liste des jeux donne — l'UUID d'un jeu d'ici, le
-- même sur la bibliothèque et sur un disque (storage.RomID), et
-- srv<id>:<référence> pour un jeu d'un serveur fédéré, qui garde AUSSI son
-- serveur : retiré du registre, il emporte ses favoris, comme les comptes
-- qu'on y avait juste au-dessus. Le titre, pour qu'une liste des favoris se
-- lise sans relire chaque source. Ils étaient partis le 2026-10-29 (journal
-- 344, leur table effacée à chaque démarrage) ; le porteur les redemande.
CREATE TABLE IF NOT EXISTS favorites (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    console_id VARCHAR(100) NOT NULL,
    reference  VARCHAR(255) NOT NULL,
    server_id  INTEGER REFERENCES federation_servers(id) ON DELETE CASCADE,
    title      VARCHAR(255),
    created_at DATETIME,
    UNIQUE (user_id, console_id, reference)
);

-- LES SERVEURS FÉDÉRÉS QU'UN COMPTE A ÉTEINTS (le porteur, 2026-11-16 :
-- « chaque utilisateur choisit quel serveur fédéré il a envie de voir les
-- jeux » ; Réglages › Application, api/hidden_servers.go) : seulement les
-- éteints — un serveur se montre par défaut, et un serveur ajouté plus tard
-- aussi. Retiré du registre, ou le compte supprimé, la ligne part avec lui.
CREATE TABLE IF NOT EXISTS hidden_servers (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id  INTEGER NOT NULL REFERENCES federation_servers(id) ON DELETE CASCADE,
    created_at DATETIME,
    PRIMARY KEY (user_id, server_id)
);

-- LE STATUT DES CONSOLES ET DES JEUX (chantier 30 ; le porteur, 2026-11-17 : « je vois que les statuts des jeux et
-- consoles sont pas dans la base de données mais dans un fichier json. Il faut que ça soit dans la base de données »).
-- Ils vivaient dans status/<console>.json (un fichier par console), abandonnés : on part de zéro. Le statut d'une console réglée d'un geste (vide : son défaut,
-- gamestatus.Default) ; les SIGNALEMENTS d'un jeu, du plus ancien au plus récent — le statut d'un jeu se calcule
-- (gamestatus.Resolve), il n'est écrit nulle part. Lus et écrits par le kernel seul (/internal/game-status, et
-- credential/embed au téléphone) : les gardes de l'administrateur sont les siennes.
CREATE TABLE IF NOT EXISTS game_status_consoles (
    console_id VARCHAR(100) PRIMARY KEY,
    status     VARCHAR(20) NOT NULL,
    set_by     VARCHAR(255),
    set_at     DATETIME
);
CREATE TABLE IF NOT EXISTS game_status_reports (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    console_id VARCHAR(100) NOT NULL,
    game_id    VARCHAR(255) NOT NULL,
    status     VARCHAR(20) NOT NULL,
    emulators  TEXT,
    versions   TEXT,
    note       TEXT,
    reported_by VARCHAR(255),
    reported_at DATETIME
);
CREATE INDEX IF NOT EXISTS game_status_reports_game ON game_status_reports (console_id, game_id, id);
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
	// LES CONSOLES D'UN ACCÈS DÉLIVRÉ (chantier 66, `#795`) : le même format que celles d'un compte, « * » par défaut —
	// l'administrateur restreint un accès comme un membre. Elles remplacent la portée `devices` (chantier 9), qui ne
	// disait que « voir les consoles Remote Play » : retirée des jetons et des invitations.
	if err := ensureColumn(db, "federation_tokens", "consoles", "VARCHAR(500) NOT NULL DEFAULT '*'"); err != nil {
		db.Close()
		return nil, err
	}
	for _, table := range []string{"federation_tokens", "invitations"} {
		if _, err := db.Exec(`UPDATE ` + table + ` SET scopes = trim(replace(replace(replace(',' || scopes || ',', ',devices,', ','), ',,', ','), ',,', ','), ',')
			WHERE ',' || scopes || ',' LIKE '%,devices,%'`); err != nil {
			db.Close()
			return nil, fmt.Errorf("drop the devices scope from %s: %w", table, err)
		}
	}
	// LES CONSOLES COUPÉES POUR TOUT LE SERVEUR (chantier 66) : une ligne par console coupée — personne ne la lance,
	// ni un membre d'ici, administrateur compris, ni un joueur venu par la fédération.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS disabled_consoles (
		console_id     VARCHAR(40) PRIMARY KEY,
		disabled_at    DATETIME,
		disabled_by_id INTEGER REFERENCES users(id) ON DELETE SET NULL
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create disabled_consoles: %w", err)
	}
	// LES PARTAGES MONTÉS PAR APPAREIL SONT PARTIS (chantier 61, P6) : gameflex-federation les a remplacés — un appareil
	// le rejoint comme un serveur fédéré. La table et ses identifiants partent avec eux, sans rien garder.
	if _, err := db.Exec(`DROP TABLE IF EXISTS remote_mounts`); err != nil {
		db.Close()
		return nil, fmt.Errorf("drop remote_mounts: %w", err)
	}
	// Un jeton pour chaque compte d'avant le déclencheur.
	if _, err := db.Exec(`INSERT INTO account_tokens (user_id, token, created_at)
		SELECT id, lower(hex(randomblob(32))), strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime')
		FROM users WHERE id NOT IN (SELECT user_id FROM account_tokens)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed account tokens: %w", err)
	}
	// ADMINISTRER CE SERVEUR DEPUIS SES SERVEURS FÉDÉRÉS (chantier 73) : accordé ACCÈS PAR ACCÈS, coupé par défaut — le
	// porteur, 2026-11-20 : « je ne veux pas que ma smart TV puisse avoir la possibilité de faire du remote admin ».
	// Il remplace l'interrupteur de chaque jeton de compte, puis celui de tout le serveur, essayés le même jour.
	if err := ensureColumn(db, "federation_tokens", "remote_admin", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, err
	}
	// …et voyage avec l'invitation ou le code d'appairage jusqu'à l'accès qu'ils font naître.
	if err := ensureColumn(db, "invitations", "remote_admin", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, err
	}
	// LE RELAIS DE LA FÉDÉRATION (chantier 83 ; le porteur, 2026-11-26) : « Peut me relayer à ses appareils », coupé par
	// défaut — le serveur qui a cet accès peut repasser nos jeux aux appareils à qui il en donne un ; et, sur l'accès
	// qu'on délivre, les serveurs fédérés qu'on relaie à son porteur, un par un (leurs identifiants, séparés par des
	// virgules).
	if err := ensureColumn(db, "federation_tokens", "relay_allowed", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureColumn(db, "federation_tokens", "relayed_servers", "VARCHAR(500) NOT NULL DEFAULT ''"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS remote_administration`); err != nil {
		db.Close()
		return nil, fmt.Errorf("drop remote_administration: %w", err)
	}
	if err := dropColumn(db, "account_tokens", "remote_admin"); err != nil {
		db.Close()
		return nil, err
	}
	// Invite list sorting — existing databases predate the column.
	if err := ensureColumn(db, "users", "last_login", "DATETIME"); err != nil {
		db.Close()
		return nil, err
	}
	// LE MOT DE PASSE QU'UN MEMBRE CHOISIT, ET SES SESSIONS (chantier 55).
	//
	// password_set_at — quand le membre a choisi son mot de passe, par le lien
	// d'un compte ou dans « Mon mot de passe » ; vide, il attend son lien (son
	// `hashed_password` est vide aussi, et la connexion le refuse). Les comptes
	// qui ont un mot de passe sans cette date — ceux d'avant la colonne, ceux
	// qu'un chemin n'a pas datés — reçoivent leur date de création : ils ont
	// rejoint. Rejoué à chaque démarrage, sans effet une fois fait.
	//
	// session_generation — la génération des sessions du compte, que chaque
	// jeton porte (claim `gen`, auth.Claims) : la faire avancer coupe toutes les
	// sessions d'avant, sur tous les appareils.
	if err := ensureColumn(db, "users", "password_set_at", "DATETIME"); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureColumn(db, "users", "session_generation", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, err
	}
	// games_generation — la génération de ses PARTIES : quand elle avance, ses
	// parties en cours finissent, au kernel qui la lit dans la liste des comptes.
	// Elle avance avec celle des sessions quand toutes ses sessions tombent — une
	// réinitialisation, « Déconnecter tous les appareils » —, et non quand
	// celui-ci reste connecté (« Mon mot de passe », « Déconnecter les autres
	// appareils ») : rien ne dit de quel appareil une partie vient.
	if err := ensureColumn(db, "users", "games_generation", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, err
	}
	// LES CODES D'APPAIRAGE (chantier 60, api/pairing.go) : six chiffres, deux
	// minutes, dans la table des invitations. Leurs modes ne vivent qu'en
	// mémoire : un démarrage les a tous fermés, et les codes restants partent.
	// QUI A COLLÉ LE JETON, ET SUR QUOI (#787) : le compte de l'appelant et son genre (serveur ou application).
	for _, column := range []string{"member", "kind"} {
		if err := ensureColumn(db, "account_token_connections", column, "TEXT NOT NULL DEFAULT ''"); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := ensureColumn(db, "invitations", "pairing", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`DELETE FROM invitations WHERE pairing = 1`); err != nil {
		db.Close()
		return nil, fmt.Errorf("purge the pairing codes: %w", err)
	}
	// LA DOUBLE AUTHENTIFICATION (chantier 80, api/mfa.go) : `mfa_required`, l'administrateur l'impose à ce compte ;
	// `user_mfa`, le secret scellé avec la clé de `keys/` (internal/mfa), son activation, le dernier pas de code
	// employé (un code ne sert qu'une fois) et les empreintes des codes de secours (JSON). Une ligne sans `enabled_at`
	// est une activation commencée, pas encore prouvée par un premier code. La règle « imposée à tout le serveur » est
	// dans `settings` (`mfa_required_all`).
	if err := ensureColumn(db, "users", "mfa_required", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS user_mfa (
		user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		secret     TEXT NOT NULL,
		enabled_at DATETIME,
		last_step  INTEGER NOT NULL DEFAULT 0,
		recovery   TEXT NOT NULL DEFAULT '[]'
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create user_mfa: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create settings: %w", err)
	}
	if _, err := db.Exec(`UPDATE users SET password_set_at = COALESCE(created_at, strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime'))
		WHERE password_set_at IS NULL AND hashed_password != ''`); err != nil {
		db.Close()
		return nil, fmt.Errorf("date the chosen passwords: %w", err)
	}

	return db, nil
}

// dropColumn — retire une colonne qui n'a plus d'usage, si elle est encore là.
func dropColumn(db *sql.DB, table, column string) error {
	var present int
	if err := db.QueryRow(fmt.Sprintf("SELECT count(*) FROM pragma_table_info('%s') WHERE name = ?", table), column).Scan(&present); err != nil {
		return fmt.Errorf("inspect %s: %w", table, err)
	}
	if present == 0 {
		return nil
	}
	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column)); err != nil {
		return fmt.Errorf("drop %s.%s: %w", table, column, err)
	}

	return nil
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
