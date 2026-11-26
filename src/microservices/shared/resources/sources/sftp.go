package sources

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"path"
	"strings"
	"sync"
	"time"

	"gameflex/storage"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPConfig — `sftp://hôte[:port]`, le dossier parcouru, le compte, et la clé du serveur retenue.
type SFTPConfig struct {
	Name     string
	Address  string
	Folder   string
	Username string
	Password string
	// HostKey — la clé publique du serveur (format authorized_keys), retenue à la première connexion ; vide la
	// première fois. Une clé qui CHANGE ensuite est refusée : c'est le seul rempart contre un serveur qui se fait
	// passer pour le vrai.
	HostKey string
	// Remember — appelée à la première connexion avec la clé à retenir.
	Remember func(hostKey string)
}

// SFTP — un serveur SSH qui sert ses fichiers. Une connexion gardée (le client SFTP accepte plusieurs requêtes à la
// fois), refaite si elle meurt.
type SFTP struct {
	config SFTPConfig
	host   string

	mu     sync.Mutex
	ssh    *ssh.Client
	client *sftp.Client
}

// NewSFTP — prépare la source ; la première liste se connecte.
func NewSFTP(config SFTPConfig) (*SFTP, error) {
	host, err := hostPort(config.Address, "sftp", "22")
	if err != nil {
		return nil, err
	}
	// ANONYME (le porteur, 2026-11-20) : SSH nomme toujours un compte ; sans nom, `anonymous`, sans mot de passe — le
	// client essaie d'abord la méthode « none », qu'un serveur local sans mot de passe accepte.
	if config.Username == "" {
		config.Username, config.Password = "anonymous", ""
	}

	return &SFTP{config: config, host: host}, nil
}

func (s *SFTP) hostKeyCallback(_ string, _ net.Addr, key ssh.PublicKey) error {
	seen := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	if s.config.HostKey == "" {
		s.config.HostKey = seen
		if s.config.Remember != nil {
			s.config.Remember(seen)
		}

		return nil
	}
	known, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s.config.HostKey))
	if err != nil {
		return fmt.Errorf("clé retenue illisible : %w", err)
	}
	if !bytes.Equal(known.Marshal(), key.Marshal()) {
		return fmt.Errorf("la clé du serveur %s a changé (%s) : connexion refusée", s.host, ssh.FingerprintSHA256(key))
	}

	return nil
}

func (s *SFTP) connectLocked() error {
	if s.client != nil {
		return nil
	}
	conn, err := ssh.Dial("tcp", s.host, &ssh.ClientConfig{
		User:            s.config.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(s.config.Password)},
		HostKeyCallback: s.hostKeyCallback,
		Timeout:         15 * time.Second,
	})
	if err != nil {
		return err
	}
	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()

		return err
	}
	s.ssh, s.client = conn, client

	return nil
}

func (s *SFTP) dropLocked() {
	if s.client != nil {
		s.client.Close()
	}
	if s.ssh != nil {
		s.ssh.Close()
	}
	s.client, s.ssh = nil, nil
}

func (s *SFTP) full(p string) string {
	folder := strings.TrimSpace(s.config.Folder)
	if strings.HasPrefix(folder, "/") {
		return path.Join(folder, p)
	}

	return path.Join(".", folder, p)
}

// Name — le nom de la ressource.
func (s *SFTP) Name() string { return s.config.Name }

// Kind — « sftp ».
func (s *SFTP) Kind() string { return KindSFTP }

// client — le client SFTP, connecté.
func (s *SFTP) current() (*sftp.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.connectLocked(); err != nil {
		return nil, err
	}

	return s.client, nil
}

func (s *SFTP) retry(do func(*sftp.Client) error) error {
	client, err := s.current()
	if err != nil {
		return err
	}
	if err = do(client); err == nil {
		return nil
	}
	// Une connexion gardée meurt (le serveur a redémarré) : une seule nouvelle tentative, sur une neuve.
	s.mu.Lock()
	if s.client == client {
		s.dropLocked()
	}
	s.mu.Unlock()
	if client, err = s.current(); err != nil {
		return err
	}

	return do(client)
}

// List — le contenu d'un dossier, relatif au dossier de la ressource.
func (s *SFTP) List(dir string) ([]storage.Entry, error) {
	var entries []storage.Entry
	err := s.retry(func(client *sftp.Client) error {
		found, err := client.ReadDir(s.full(dir))
		if err != nil {
			return err
		}
		entries = make([]storage.Entry, 0, len(found))
		for _, info := range found {
			if info.IsDir() {
				entries = append(entries, storage.Entry{Name: info.Name(), Dir: true})
			} else if info.Mode().IsRegular() {
				entries = append(entries, storage.Entry{Name: info.Name(), Size: info.Size()})
			}
		}

		return nil
	})

	return entries, err
}

// Open — le contenu d'un fichier.
func (s *SFTP) Open(p string) (io.ReadCloser, error) {
	var file io.ReadCloser
	err := s.retry(func(client *sftp.Client) error {
		opened, err := client.Open(s.full(p))
		if err != nil {
			return err
		}
		file = opened

		return nil
	})

	return file, err
}

// Close — ferme la connexion.
func (s *SFTP) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropLocked()

	return nil
}
