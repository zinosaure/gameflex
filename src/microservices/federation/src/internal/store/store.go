// Package store — LA BASE DU SERVICE, À LUI SEUL (chantier 61, le porteur, 2026-11-18 : « Il faut qu'il ait sa propre
// base pour stocker ses propres infos »).
//
// gameflex-federation se lance sans aucun autre service de GameFlex : ses ressources, ses jetons de catalogue et ses
// réglages vivent dans un fichier SQLite de son volume de données. Les mots de passe des ressources y sont SCELLÉS
// (AES-256-GCM) par une clé tirée au premier démarrage et rangée à côté de la base, jamais dans la base : une copie de
// la base seule ne livre aucun mot de passe.
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Kinds — les genres de ressource que le service sait ouvrir.
var Kinds = []string{"smb", "webdav", "kdrive", "ftp", "sftp"}

// Resource — un partage déclaré : où il est, et de quoi s'y connecter.
type Resource struct {
	ID int64 `json:"id"`
	// Name — son nom, choisi par l'administrateur ; il s'affiche à côté des jeux.
	Name string `json:"name"`
	// Kind — « smb », « webdav », « kdrive », « ftp », « sftp ».
	Kind string `json:"kind"`
	// Address — `smb://hôte[:port]/partage`, l'URL WebDAV, le numéro d'un kDrive, `ftp://hôte[:port]` ou
	// `sftp://hôte[:port]`.
	Address string `json:"address"`
	// Folder — le dossier parcouru dans le partage (celui qui porte `roms/`) ; vide = sa racine.
	Folder   string `json:"folder"`
	Username string `json:"username"`
	// Password — en clair en mémoire seulement : la base garde sa forme scellée. Jamais rendu par l'API.
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

// Token — un jeton de catalogue, tel que l'administration le voit : jamais sa valeur, qui n'est montrée qu'une fois.
type Token struct {
	ID        int64     `json:"id"`
	Label     string    `json:"label"`
	Scopes    []string  `json:"scopes"`
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsedAt,omitzero"`
}

// Store — la base ouverte, et la clé qui scelle.
type Store struct {
	db  *sql.DB
	key []byte
}

const schema = `
CREATE TABLE IF NOT EXISTS resources (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	kind       TEXT NOT NULL,
	address    TEXT NOT NULL,
	folder     TEXT NOT NULL DEFAULT '',
	username   TEXT NOT NULL DEFAULT '',
	password   TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tokens (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	label        TEXT NOT NULL,
	hash         TEXT NOT NULL UNIQUE,
	scopes       TEXT NOT NULL,
	created_at   TEXT NOT NULL,
	last_used_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS invitations (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	label      TEXT NOT NULL,
	hash       TEXT NOT NULL UNIQUE,
	expires_at TEXT NOT NULL,
	used_at    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS admin_sessions (
	hash       TEXT PRIMARY KEY,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// Open — la base de `dir` (créée au besoin), et sa clé de scellement (tirée au premier démarrage).
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := loadKey(filepath.Join(dir, "seal.key"))
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "federation.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()

		return nil, err
	}

	return &Store{db: db, key: key}, nil
}

// Close — ferme la base.
func (s *Store) Close() error { return s.db.Close() }

// loadKey — la clé de 32 octets du fichier, ou une neuve écrite en 0600 s'il n'existe pas.
func loadKey(path string) ([]byte, error) {
	if text, err := os.ReadFile(path); err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(text)))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("%s : clé de scellement illisible", path)
		}

		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	return key, os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600)
}

func (s *Store) seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (s *Store) unseal(sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < aead.NonceSize() {
		return "", errors.New("mot de passe scellé tronqué")
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], nil)

	return string(plain), err
}

// ── Les ressources ──────────────────────────────────────────────────────────

func validKind(kind string) bool {
	for _, known := range Kinds {
		if kind == known {
			return true
		}
	}

	return false
}

// AddResource — enregistre une ressource et rend son identifiant.
func (s *Store) AddResource(r Resource) (int64, error) {
	r.Name, r.Kind, r.Address = strings.TrimSpace(r.Name), strings.TrimSpace(r.Kind), strings.TrimSpace(r.Address)
	if r.Name == "" || r.Address == "" || !validKind(r.Kind) {
		return 0, errors.New("une ressource demande un nom, une adresse et un genre (smb, webdav, kdrive, ftp, sftp)")
	}
	sealed, err := s.seal(r.Password)
	if err != nil {
		return 0, err
	}
	result, err := s.db.Exec(`INSERT INTO resources (name, kind, address, folder, username, password, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, r.Name, r.Kind, r.Address, strings.Trim(r.Folder, "/ "), r.Username, sealed,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// UpdateResource — change une ressource ; un mot de passe vide garde l'ancien.
func (s *Store) UpdateResource(r Resource) error {
	if !validKind(r.Kind) || strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.Address) == "" {
		return errors.New("une ressource demande un nom, une adresse et un genre (smb, webdav, kdrive, ftp, sftp)")
	}
	query := `UPDATE resources SET name = ?, kind = ?, address = ?, folder = ?, username = ? WHERE id = ?`
	args := []any{strings.TrimSpace(r.Name), r.Kind, strings.TrimSpace(r.Address), strings.Trim(r.Folder, "/ "), r.Username, r.ID}
	if r.Password != "" {
		sealed, err := s.seal(r.Password)
		if err != nil {
			return err
		}
		query = `UPDATE resources SET name = ?, kind = ?, address = ?, folder = ?, username = ?, password = ? WHERE id = ?`
		args = []any{args[0], args[1], args[2], args[3], args[4], sealed, r.ID}
	}
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return os.ErrNotExist
	}

	return nil
}

// RemoveResource — retire une ressource.
func (s *Store) RemoveResource(id int64) error {
	result, err := s.db.Exec(`DELETE FROM resources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return os.ErrNotExist
	}

	return nil
}

// Resources — toutes les ressources, mots de passe descellés (pour les ouvrir ; l'API ne les rend jamais).
func (s *Store) Resources() ([]Resource, error) {
	rows, err := s.db.Query(`SELECT id, name, kind, address, folder, username, password, created_at FROM resources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resources := []Resource{}
	for rows.Next() {
		var r Resource
		var sealed, created string
		if err := rows.Scan(&r.ID, &r.Name, &r.Kind, &r.Address, &r.Folder, &r.Username, &sealed, &created); err != nil {
			return nil, err
		}
		if r.Password, err = s.unseal(sealed); err != nil {
			return nil, fmt.Errorf("ressource %d : mot de passe illisible (%w)", r.ID, err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		resources = append(resources, r)
	}

	return resources, rows.Err()
}

// ── Les jetons de catalogue ─────────────────────────────────────────────────

// ScopeCatalog — la seule portée du service : son catalogue et ses fiches. Il n'a ni consoles ni appareils à prêter.
const ScopeCatalog = "catalog"

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))

	return hex.EncodeToString(sum[:])
}

// IssueToken — un jeton neuf pour `label` ; sa valeur n'est rendue qu'ici, la base n'en garde que l'empreinte.
func (s *Store) IssueToken(label string) (Token, string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return Token{}, "", errors.New("un jeton demande un nom (l'appareil qui le recevra)")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, "", err
	}
	value := "gff_" + base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	result, err := s.db.Exec(`INSERT INTO tokens (label, hash, scopes, created_at) VALUES (?, ?, ?, ?)`,
		label, hashToken(value), ScopeCatalog, now.Format(time.RFC3339))
	if err != nil {
		return Token{}, "", err
	}
	id, _ := result.LastInsertId()

	return Token{ID: id, Label: label, Scopes: []string{ScopeCatalog}, CreatedAt: now}, value, nil
}

// VerifyToken — le jeton de cette valeur, s'il existe ; sa dernière utilisation est notée (à la minute près).
func (s *Store) VerifyToken(value string) (Token, bool) {
	var t Token
	var scopes, created, used string
	err := s.db.QueryRow(`SELECT id, label, scopes, created_at, last_used_at FROM tokens WHERE hash = ?`,
		hashToken(value)).Scan(&t.ID, &t.Label, &scopes, &created, &used)
	if err != nil {
		return Token{}, false
	}
	t.Scopes = strings.Split(scopes, ",")
	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	t.LastUsed, _ = time.Parse(time.RFC3339, used)
	if now := time.Now().UTC(); now.Sub(t.LastUsed) > time.Minute {
		_, _ = s.db.Exec(`UPDATE tokens SET last_used_at = ? WHERE id = ?`, now.Format(time.RFC3339), t.ID)
		t.LastUsed = now
	}

	return t, true
}

// Tokens — les jetons émis.
func (s *Store) Tokens() ([]Token, error) {
	rows, err := s.db.Query(`SELECT id, label, scopes, created_at, last_used_at FROM tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := []Token{}
	for rows.Next() {
		var t Token
		var scopes, created, used string
		if err := rows.Scan(&t.ID, &t.Label, &scopes, &created, &used); err != nil {
			return nil, err
		}
		t.Scopes = strings.Split(scopes, ",")
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		t.LastUsed, _ = time.Parse(time.RFC3339, used)
		tokens = append(tokens, t)
	}

	return tokens, rows.Err()
}

// RevokeToken — retire un jeton : l'appareil qui le tient ne lit plus rien.
func (s *Store) RevokeToken(id int64) error {
	result, err := s.db.Exec(`DELETE FROM tokens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return os.ErrNotExist
	}

	return nil
}

// ── Les invitations (chantier 33 de GameFlex) ──────────────────────────────

// InvitationLifetime — la durée d'une invitation, celle de GameFlex.
const InvitationLifetime = 12 * time.Hour

// Invitation — une invitation, telle que l'administration la voit.
type Invitation struct {
	ID        int64     `json:"id"`
	Label     string    `json:"label"`
	ExpiresAt time.Time `json:"expiresAt"`
	UsedAt    time.Time `json:"usedAt,omitzero"`
}

// ErrInvitationRefused — inconnue, déjà servie ou échue.
var ErrInvitationRefused = errors.New("invitation refusée")

// CreateInvitation — une invitation pour `label` : un code de 22 caractères (128 bits), à usage unique, rendu
// seulement ici. C'est lui qui voyage par une messagerie, dans un lien `<service>/invite/<code>` ou son QR, et non le
// jeton durable, que l'appareil reçoit en l'échangeant.
func (s *Store) CreateInvitation(label string) (Invitation, string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return Invitation{}, "", errors.New("une invitation demande un nom (l'appareil qui la recevra)")
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return Invitation{}, "", err
	}
	code := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().UTC().Add(InvitationLifetime)
	result, err := s.db.Exec(`INSERT INTO invitations (label, hash, expires_at) VALUES (?, ?, ?)`,
		label, hashToken(code), expires.Format(time.RFC3339))
	if err != nil {
		return Invitation{}, "", err
	}
	id, _ := result.LastInsertId()

	return Invitation{ID: id, Label: label, ExpiresAt: expires}, code, nil
}

// RedeemInvitation — consomme l'invitation de ce code et émet le jeton de l'appareil qui la présente.
func (s *Store) RedeemInvitation(code string) (Token, string, error) {
	var id int64
	var label, expires, used string
	err := s.db.QueryRow(`SELECT id, label, expires_at, used_at FROM invitations WHERE hash = ?`, hashToken(code)).
		Scan(&id, &label, &expires, &used)
	if err != nil {
		return Token{}, "", ErrInvitationRefused
	}
	at, err := time.Parse(time.RFC3339, expires)
	if err != nil || used != "" || time.Now().UTC().After(at) {
		return Token{}, "", ErrInvitationRefused
	}
	// Servie une seule fois, même si deux appareils la présentent en même temps.
	result, err := s.db.Exec(`UPDATE invitations SET used_at = ? WHERE id = ? AND used_at = ''`,
		time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return Token{}, "", err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return Token{}, "", ErrInvitationRefused
	}

	return s.IssueToken(label)
}

// Invitations — les invitations encore utilisables ou servies, les plus récentes d'abord.
func (s *Store) Invitations() ([]Invitation, error) {
	rows, err := s.db.Query(`SELECT id, label, expires_at, used_at FROM invitations ORDER BY id DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Invitation{}
	for rows.Next() {
		var i Invitation
		var expires, used string
		if err := rows.Scan(&i.ID, &i.Label, &expires, &used); err != nil {
			return nil, err
		}
		i.ExpiresAt, _ = time.Parse(time.RFC3339, expires)
		i.UsedAt, _ = time.Parse(time.RFC3339, used)
		list = append(list, i)
	}

	return list, rows.Err()
}

// RevokeInvitation — retire une invitation pas encore servie.
func (s *Store) RevokeInvitation(id int64) error {
	result, err := s.db.Exec(`DELETE FROM invitations WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return os.ErrNotExist
	}

	return nil
}

// ── Les sessions de l'administrateur ──────────────────────────────────────

// SessionLifetime — la durée d'une connexion à l'administration.
const SessionLifetime = 30 * 24 * time.Hour

// OpenSession — une session neuve ; sa valeur n'est rendue qu'ici, la base n'en garde que l'empreinte.
func (s *Store) OpenSession() (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	value := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().UTC().Add(SessionLifetime)
	_, _ = s.db.Exec(`DELETE FROM admin_sessions WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	_, err := s.db.Exec(`INSERT INTO admin_sessions (hash, expires_at) VALUES (?, ?)`, hashToken(value), expires.Format(time.RFC3339))

	return value, expires, err
}

// ValidSession — cette session existe-t-elle et vit-elle encore ?
func (s *Store) ValidSession(value string) bool {
	var expires string
	if s.db.QueryRow(`SELECT expires_at FROM admin_sessions WHERE hash = ?`, hashToken(value)).Scan(&expires) != nil {
		return false
	}
	at, err := time.Parse(time.RFC3339, expires)

	return err == nil && time.Now().UTC().Before(at)
}

// CloseSession — la déconnexion.
func (s *Store) CloseSession(value string) {
	_, _ = s.db.Exec(`DELETE FROM admin_sessions WHERE hash = ?`, hashToken(value))
}

// ── Les réglages ────────────────────────────────────────────────────────────

// Setting — la valeur d'un réglage, `fallback` s'il n'est pas posé.
func (s *Store) Setting(key, fallback string) string {
	var value string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value); err != nil {
		return fallback
	}

	return value
}

// SetSetting — pose un réglage.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)

	return err
}
