// Package sources — les genres de ressource que le service ajoute à ceux de GameFlex (Samba, WebDAV, kDrive, dans
// gameflex/storage) : FTP et SFTP (chantier 61, P4). Chacun est un storage.Source (lister) et un storage.Opener (lire),
// que le scan de GameFlex parcourt sans savoir d'où viennent les noms.
package sources

import (
	"errors"
	"io"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"gameflex/storage"

	"github.com/jlaffaye/ftp"
)

// KindFTP, KindSFTP — les mots affichés à côté d'un jeu.
const (
	KindFTP  = "ftp"
	KindSFTP = "sftp"
)

// FTPConfig — `ftp://hôte[:port]`, le dossier parcouru, et le compte (vide = anonyme).
type FTPConfig struct {
	Name     string
	Address  string
	Folder   string
	Username string
	Password string
}

// FTP — un serveur FTP. La connexion des listes est gardée (et refaite si elle meurt) ; chaque fichier ouvert a la
// sienne, puisqu'une connexion FTP ne sert qu'un transfert à la fois.
type FTP struct {
	config FTPConfig
	host   string

	mu   sync.Mutex
	conn *ftp.ServerConn
}

// NewFTP — prépare la source ; la première liste se connecte.
func NewFTP(config FTPConfig) (*FTP, error) {
	host, err := hostPort(config.Address, "ftp", "21")
	if err != nil {
		return nil, err
	}
	if config.Username == "" {
		config.Username, config.Password = "anonymous", "anonymous"
	}

	return &FTP{config: config, host: host}, nil
}

// hostPort — `hôte:port` d'une adresse `<schéma>://hôte[:port]`, ou d'un `hôte[:port]` nu.
func hostPort(address, scheme, port string) (string, error) {
	address = strings.TrimSpace(address)
	if !strings.Contains(address, "://") {
		address = scheme + "://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("adresse invalide : " + address)
	}
	if parsed.Scheme != scheme {
		return "", errors.New("l'adresse doit commencer par " + scheme + "://")
	}
	if parsed.Port() != "" {
		port = parsed.Port()
	}

	return parsed.Hostname() + ":" + port, nil
}

func (f *FTP) dial() (*ftp.ServerConn, error) {
	conn, err := ftp.Dial(f.host, ftp.DialWithTimeout(15*time.Second))
	if err != nil {
		return nil, err
	}
	if err := conn.Login(f.config.Username, f.config.Password); err != nil {
		_ = conn.Quit()

		return nil, err
	}

	return conn, nil
}

func (f *FTP) full(p string) string {
	return "/" + strings.Trim(path.Join(strings.Trim(f.config.Folder, "/"), p), "/")
}

// Name — le nom de la ressource.
func (f *FTP) Name() string { return f.config.Name }

// Kind — « ftp ».
func (f *FTP) Kind() string { return KindFTP }

// List — le contenu d'un dossier, relatif au dossier de la ressource.
func (f *FTP) List(dir string) ([]storage.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if f.conn == nil {
			conn, err := f.dial()
			if err != nil {
				return nil, err
			}
			f.conn = conn
		}
		found, err := f.conn.List(f.full(dir))
		if err != nil {
			// Une connexion gardée meurt (délai du serveur) : une seule nouvelle tentative, sur une neuve.
			_ = f.conn.Quit()
			f.conn = nil
			if attempt == 0 {
				continue
			}

			return nil, err
		}
		entries := make([]storage.Entry, 0, len(found))
		for _, e := range found {
			if e.Name == "." || e.Name == ".." {
				continue
			}
			switch e.Type {
			case ftp.EntryTypeFolder:
				entries = append(entries, storage.Entry{Name: e.Name, Dir: true})
			case ftp.EntryTypeFile:
				entries = append(entries, storage.Entry{Name: e.Name, Size: int64(e.Size)})
			}
		}

		return entries, nil
	}

	return nil, errors.New("liste impossible")
}

// ftpFile — un fichier en cours de lecture, et la connexion qui ne sert qu'à lui.
type ftpFile struct {
	*ftp.Response
	conn *ftp.ServerConn
}

func (file *ftpFile) Close() error {
	err := file.Response.Close()
	_ = file.conn.Quit()

	return err
}

// Open — le contenu d'un fichier, sur une connexion à lui.
func (f *FTP) Open(p string) (io.ReadCloser, error) {
	conn, err := f.dial()
	if err != nil {
		return nil, err
	}
	response, err := conn.Retr(f.full(p))
	if err != nil {
		_ = conn.Quit()

		return nil, err
	}

	return &ftpFile{Response: response, conn: conn}, nil
}

// Close — ferme la connexion des listes.
func (f *FTP) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn == nil {
		return nil
	}
	err := f.conn.Quit()
	f.conn = nil

	return err
}
