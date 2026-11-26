// Package resources — LES RESSOURCES EXTERNES DE CE SERVEUR (Samba, WebDAV, kDrive, FTP, SFTP ; `#808`, 2026-11-20).
//
// Le service gameflex-federation s'intègre à GameFlex (le porteur, 2026-11-20 : « Un serveur Gameflex peut faire office
// de serveur fédéré seul ») : ses sources (sources/), son parcours (library/, catalog.go) et son classement des copies
// (rank.go) sont repris tels quels ; ce qui existait déjà ici — jetons, invitations, code d'appairage, disques internes,
// comptes des fournisseurs de métadonnées, journaux — reste celui de GameFlex. Une ressource apporte ses sections au
// listing que ce serveur sert à ses amis, derrière la bibliothèque et les disques internes (api/resources_api.go).
//
// LE STOCKAGE : un fichier JSON à côté de la base de GameFlex (`databases/resources.json`, mode 600), les mots de passe
// scellés (AES-GCM) par une clé tirée au premier démarrage (`databases/resources.key`, mode 600) — ce que faisait le
// service dans sa propre base. Jamais un mot de passe dans une réponse de l'API.
package resources

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Kinds — les genres de ressource.
var Kinds = []string{"smb", "webdav", "kdrive", "ftp", "sftp"}

// Resource — une ressource enregistrée. Password est descellé en mémoire pour ouvrir la source ; l'API ne le rend jamais.
type Resource struct {
	ID int64 `json:"id"`
	// Name — le nom montré, et le premier segment des chemins que la source rend.
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Address — l'adresse recomposée par le formulaire : `smb://nas/jeux`, `https://hôte:8443/chemin`, `ftp://hôte`,
	// `sftp://hôte:22` ; pour kDrive, le numéro du drive.
	Address string `json:"address"`
	// Folder — le dossier, dans la ressource, qui porte la bibliothèque (`roms/`, `bios/`…) ; vide : sa racine.
	Folder    string    `json:"folder"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

// stored — une ressource telle qu'elle est écrite : le mot de passe scellé.
type stored struct {
	Resource
	Sealed string `json:"password,omitempty"`
}

type document struct {
	NextID    int64             `json:"nextId"`
	Resources []stored          `json:"resources"`
	Settings  map[string]string `json:"settings,omitempty"`
}

// Store — les ressources et quelques réglages (la clé retenue d'un serveur SFTP).
type Store struct {
	path string
	key  []byte

	mu   sync.Mutex
	data document
}

// Open — le magasin du dossier `dir` (créé au besoin), sa clé tirée au premier appel.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := loadKey(filepath.Join(dir, "resources.key"))
	if err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "resources.json"), key: key, data: document{NextID: 1, Settings: map[string]string{}}}
	if text, err := os.ReadFile(s.path); err == nil {
		if err := json.Unmarshal(text, &s.data); err != nil {
			return nil, fmt.Errorf("%s : %w", s.path, err)
		}
		if s.data.Settings == nil {
			s.data.Settings = map[string]string{}
		}
		if s.data.NextID < 1 {
			s.data.NextID = 1
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	return s, nil
}

// saveLocked — écrit le document (fichier temporaire puis renommage : une coupure ne laisse pas un fichier tronqué).
func (s *Store) saveLocked() error {
	text, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, text, 0o600); err != nil {
		return err
	}

	return os.Rename(tmp, s.path)
}

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

// anonymous — une connexion sans compte (le porteur, 2026-11-20 : « mon serveur SFTP peut être en local et j'ai pas
// besoin de mot de passe ») : un nom d'utilisateur vide, sauf kDrive, qui n'a qu'un jeton. Chaque source y présente ce
// que son protocole attend d'un anonyme (invité SMB, `anonymous` en FTP et SFTP, aucun en-tête en WebDAV).
func anonymous(r Resource) bool {
	return r.Kind != "kdrive" && strings.TrimSpace(r.Username) == ""
}

// normalized — la ressource revalidée (Validate) et nettoyée, telle qu'elle s'enregistre.
func normalized(r Resource) (Resource, error) {
	r.Name, r.Kind, r.Address = strings.TrimSpace(r.Name), strings.TrimSpace(r.Kind), strings.TrimSpace(r.Address)
	r.Folder, r.Username = strings.Trim(r.Folder, "/ "), strings.TrimSpace(r.Username)
	if err := Validate(r); err != nil {
		return Resource{}, err
	}
	if anonymous(r) {
		r.Password = ""
	}

	return r, nil
}

// AddResource — enregistre une ressource et rend son identifiant.
func (s *Store) AddResource(r Resource) (int64, error) {
	r, err := normalized(r)
	if err != nil {
		return 0, err
	}
	sealed, err := s.seal(r.Password)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID, r.CreatedAt, r.Password = s.data.NextID, time.Now().UTC(), ""
	s.data.NextID++
	s.data.Resources = append(s.data.Resources, stored{Resource: r, Sealed: sealed})

	return r.ID, s.saveLocked()
}

// UpdateResource — change une ressource ; un mot de passe vide garde l'ancien, une connexion passée en anonyme le perd.
func (s *Store) UpdateResource(r Resource) error {
	r, err := normalized(r)
	if err != nil {
		return err
	}
	sealed := ""
	if r.Password != "" {
		if sealed, err = s.seal(r.Password); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, kept := range s.data.Resources {
		if kept.ID != r.ID {
			continue
		}
		switch {
		case anonymous(r):
			kept.Sealed = ""
		case sealed != "":
			kept.Sealed = sealed
		}
		r.CreatedAt, r.Password = kept.CreatedAt, ""
		s.data.Resources[index] = stored{Resource: r, Sealed: kept.Sealed}

		return s.saveLocked()
	}

	return os.ErrNotExist
}

// RemoveResource — retire une ressource, et ce qui se gardait pour elle.
func (s *Store) RemoveResource(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, kept := range s.data.Resources {
		if kept.ID == id {
			s.data.Resources = append(s.data.Resources[:index], s.data.Resources[index+1:]...)
			delete(s.data.Settings, sftpHostKeySetting(id))

			return s.saveLocked()
		}
	}

	return os.ErrNotExist
}

// Resources — toutes les ressources, mots de passe descellés (pour les ouvrir ; l'API ne les rend jamais).
func (s *Store) Resources() ([]Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resources := make([]Resource, 0, len(s.data.Resources))
	for _, kept := range s.data.Resources {
		r := kept.Resource
		password, err := s.unseal(kept.Sealed)
		if err != nil {
			return nil, fmt.Errorf("ressource %d : mot de passe illisible (%w)", r.ID, err)
		}
		r.Password = password
		resources = append(resources, r)
	}

	return resources, nil
}

// Setting — un réglage gardé, ou `fallback`.
func (s *Store) Setting(key, fallback string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value, ok := s.data.Settings[key]; ok {
		return value
	}

	return fallback
}

// SetSetting — garde un réglage.
func (s *Store) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Settings[key] = value

	return s.saveLocked()
}

// sftpHostKeySetting — la clé d'un serveur SFTP, retenue à la première connexion et vérifiée ensuite.
func sftpHostKeySetting(id int64) string { return fmt.Sprintf("sftp_host_key_%d", id) }
