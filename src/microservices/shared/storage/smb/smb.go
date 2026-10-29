// Package smb monte un partage SMB — un NAS, un serveur Samba, un dossier
// partagé depuis Windows — comme une source de la bibliothèque.
//
// Comme WebDAV, et contrairement à un serveur GameFlex fédéré, un partage
// SMB est un ARBRE DE DOSSIERS : exactement ce que storage.Source décrit. Il
// n'y a donc rien de neuf côté scan — un dossier `nes` sur un NAS se lit
// comme un dossier `nes` sur le disque.
//
// Le protocole, lui, ne s'écrit pas à la main
// -------------------------------------------
// WebDAV et kDrive sont du HTTP écrit à la main dans ce module, sans aucune
// dépendance. SMB ne s'y prête pas : négociation SMB2/3, NTLM enveloppé dans
// SPNEGO, signature des messages. D'où la PREMIÈRE bibliothèque tierce de
// `gameflex/storage` : github.com/hirochachacha/go-smb2 — Go pur (le chemin
// gomobile cross-compile pour Android sans cgo), licence BSD à deux clauses.
//
// Le fork que maintient rclone (cloudsoda/go-smb2) aurait été le choix
// naturel, et il a été écarté pour une raison mesurée, pas par goût : il
// exige Go 1.25, et le kernel compile en 1.23 (vérifié le 2026-09-15). Le
// prendre, c'était monter la toolchain du kernel entier pour un protocole.
//
// Une session, gardée
// -------------------
// Une requête WebDAV se suffit à elle-même ; une opération SMB non. Il faut
// d'abord une connexion TCP, une authentification, puis un « tree connect »
// au partage — trois allers-retours avant le premier nom de fichier. Les
// refaire pour chaque dossier multiplierait le parcours d'une bibliothèque
// d'autant. La source ouvre donc sa session au premier besoin et la GARDE ;
// les deux kernels, eux, gardent déjà une source par partage.
//
// Ce qu'on garde finit par mourir : un NAS se met en veille, Samba ferme les
// sessions restées inactives. Une opération qui échoue PARCE QUE la
// connexion est tombée rouvre la session et réessaie une seule fois — pas
// davantage, pour qu'un serveur éteint échoue vite au lieu de boucler.
package smb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hirochachacha/go-smb2"

	"gameflex/storage"
)

// DefaultPort — le port SMB direct. NetBIOS (139) n'est pas pris en charge
// par la bibliothèque, et aucun NAS récent n'en a besoin.
const DefaultPort = 445

// guestAccount — le compte présenté pour une connexion « anonyme ».
//
// La bibliothèque refuse un nom vide (« Anonymous account is not supported
// yet. Use guest account instead »). Un nom que le serveur ne connaît pas
// est justement ce qu'un partage ouvert attend : Samba le range dans son
// compte invité (`map to guest = Bad User`), et les NAS font de même.
const guestAccount = "guest"

var (
	// ErrUnauthorized — le serveur refuse ce compte ou ce mot de passe.
	ErrUnauthorized = errors.New("smb: identifiants refusés")
	// ErrForbidden — le compte est accepté, mais ce partage ou ce dossier
	// lui est fermé. Ce n'est PAS le mot de passe : ce sont les droits du
	// partage, et ils se règlent sur le serveur.
	ErrForbidden = errors.New("smb: accès refusé à ce partage")
	// ErrShareNotFound — le serveur répond, mais n'a aucun partage à ce nom.
	// Distinct d'une adresse injoignable : l'adresse est bonne, c'est le nom
	// qui ne l'est pas, et c'est ce qu'il faut aller corriger.
	ErrShareNotFound = errors.New("smb: aucun partage à ce nom sur ce serveur")
	// ErrGuestRefused — le serveur exige que chaque message soit SIGNÉ ET
	// marque la session comme invitée, qui n'a aucune clé pour signer : la
	// bibliothèque refuse d'aller plus loin, et aucun réglage côté client n'y
	// change rien — il faut un compte. Samba 4.17 n'y tombe pas : il signe la
	// session invitée (mesuré le 2026-09-15). C'est le cas des serveurs qui
	// ne le font pas.
	ErrGuestRefused = errors.New("smb: ce serveur exige une connexion signée, impossible sans compte")
	// ErrAddress — ce qui a été saisi ne désigne pas un partage.
	ErrAddress = errors.New("smb: adresse de partage invalide")

	errClosed = errors.New("smb: source fermée")
)

// Les codes NTSTATUS qu'on sait nommer. La bibliothèque les définit dans un
// paquet `internal`, qu'on ne peut pas importer.
const (
	statusLogonFailure          = 0xC000006D
	statusAccountRestriction    = 0xC000006E
	statusPasswordExpired       = 0xC0000071
	statusAccountDisabled       = 0xC0000072
	statusNetworkNameDeleted    = 0xC00000C9
	statusBadNetworkName        = 0xC00000CC
	statusUserSessionDeleted    = 0xC0000203
	statusAccountLockedOut      = 0xC0000234
	statusNetworkSessionExpired = 0xC000035C
)

var (
	// connectTimeout borne la connexion, l'authentification et l'accès au
	// partage : un NAS éteint doit se dire en secondes, pas au bout du délai
	// TCP du système.
	connectTimeout = 10 * time.Second
	// listTimeout borne la lecture d'un dossier. Large : celui de la NES
	// d'une vraie collection dépasse les dix mille entrées.
	listTimeout = 2 * time.Minute
	// closeTimeout — l'au revoir poli au serveur ne doit pas retenir la
	// fermeture si le serveur ne répond plus.
	closeTimeout = 3 * time.Second
)

// Config — ce qu'il faut pour monter un partage.
type Config struct {
	// Name est le nom de montage, premier segment du chemin virtuel.
	Name string
	// Address — `smb://hôte[:port]/partage`, telle qu'Address la compose.
	// Un chemin après le partage devient la racine, comme un dossier.
	Address string
	// Folder — le sous-dossier parcouru, relatif au partage. Vide = sa racine.
	Folder string
	// Username vide = connexion invitée (voir guestAccount).
	Username string
	Password string
}

// Source est un partage SMB monté. Sûre pour un usage concurrent.
type Source struct {
	name     string
	hostport string
	share    string
	root     string
	username string
	password string

	mu      sync.Mutex
	conn    net.Conn
	session *smb2.Session
	fs      *smb2.Share
	// active compte les opérations en cours ET les fichiers ouverts : Close
	// attend qu'il retombe à zéro avant de couper, sans quoi un « Relire »
	// pendant un téléchargement tuerait le téléchargement.
	active int
	closed bool
}

// Address compose l'adresse d'un partage à partir de ce que le formulaire
// demande : un serveur, un port, un nom de partage.
//
// UNE FORME, TOUJOURS LA MÊME — `smb://hôte:port/partage`, port compris même
// quand c'est 445. L'adresse entre dans la clé du cache des listings : deux
// saisies du même partage, l'une avec « :445 » et l'autre sans, donneraient
// sinon deux ressources dont une ne trouve jamais le cache de l'autre.
//
// Et on accepte ce que les gens tapent réellement dans le champ du serveur :
// `\\nas\jeux` tel que Windows l'affiche, `smb://nas/jeux` tel que macOS ou
// Linux l'écrivent, ou un nom seul. Refuser `\\nas\jeux` parce que le
// partage « devait aller dans l'autre champ » serait correct et inutile.
func Address(host string, port int, share string) (string, error) {
	rest := strings.TrimSpace(strings.ReplaceAll(host, `\`, "/"))
	if len(rest) >= len("smb://") && strings.EqualFold(rest[:len("smb://")], "smb://") {
		rest = rest[len("smb://"):]
	}
	hostPart, inAddress, _ := strings.Cut(strings.TrimLeft(rest, "/"), "/")
	inAddress = strings.Trim(inAddress, "/")

	name, explicitPort, err := splitHostPort(hostPart)
	if err != nil {
		return "", err
	}
	if explicitPort != 0 {
		port = explicitPort
	}
	if port == 0 {
		port = DefaultPort
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("%w : le port %d n'existe pas", ErrAddress, port)
	}

	share = strings.Trim(strings.TrimSpace(strings.ReplaceAll(share, `\`, "/")), "/")
	switch {
	case share != "" && inAddress != "":
		return "", fmt.Errorf("%w : l'adresse du serveur nomme déjà un partage (%q)", ErrAddress, inAddress)
	case share == "":
		share = inAddress
	case strings.Contains(share, "/"):
		return "", fmt.Errorf("%w : %q n'est pas un nom de partage — un sous-dossier se renseigne à part", ErrAddress, share)
	}
	if share == "" {
		return "", fmt.Errorf("%w : il manque le nom du partage", ErrAddress)
	}

	return (&url.URL{
		Scheme: "smb",
		Host:   net.JoinHostPort(name, strconv.Itoa(port)),
		Path:   "/" + share,
	}).String(), nil
}

// splitHostPort — un nom, une IPv4, une IPv6 avec ou sans crochets, et un
// port éventuel.
func splitHostPort(value string) (string, int, error) {
	if value == "" {
		return "", 0, fmt.Errorf("%w : il manque l'adresse du serveur", ErrAddress)
	}
	// Plusieurs « : » sans crochets : une IPv6 nue, qui n'a pas de port.
	if !strings.HasPrefix(value, "[") && strings.Count(value, ":") != 1 {
		return value, 0, nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return strings.Trim(value, "[]"), 0, nil
	}

	name, portText, err := net.SplitHostPort(value)
	if err != nil {
		return "", 0, fmt.Errorf("%w : %v", ErrAddress, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || name == "" {
		return "", 0, fmt.Errorf("%w : %q", ErrAddress, value)
	}

	return name, port, nil
}

// New prépare la source. Rien ne part sur le réseau ici : la session
// s'ouvre à la première lecture, comme une requête WebDAV partirait.
func New(config Config) (*Source, error) {
	parsed, err := url.Parse(strings.TrimSpace(config.Address))
	if err != nil || !strings.EqualFold(parsed.Scheme, "smb") || parsed.Hostname() == "" {
		return nil, fmt.Errorf("%w : %q", ErrAddress, config.Address)
	}
	share, inAddress, _ := strings.Cut(strings.Trim(parsed.Path, "/"), "/")
	if share == "" {
		return nil, fmt.Errorf("%w : %q ne nomme aucun partage", ErrAddress, config.Address)
	}
	port := parsed.Port()
	if port == "" {
		port = strconv.Itoa(DefaultPort)
	}

	prefix, err := clean(inAddress)
	if err != nil {
		return nil, err
	}
	folder, err := clean(config.Folder)
	if err != nil {
		return nil, err
	}

	return &Source{
		name:     config.Name,
		hostport: net.JoinHostPort(parsed.Hostname(), port),
		share:    share,
		root:     strings.Trim(path.Join(prefix, folder), "/"),
		username: config.Username,
		password: config.Password,
	}, nil
}

// Name — le nom de montage.
func (s *Source) Name() string { return s.name }

// Kind — un partage SMB. C'est le mot qu'affiche la liste des jeux.
func (s *Source) Kind() string { return storage.KindSMB }

// List rend le contenu d'un dossier, chemin relatif au montage.
func (s *Source) List(dir string) ([]storage.Entry, error) {
	target, err := s.resolve(dir)
	if err != nil {
		return nil, err
	}

	var infos []os.FileInfo
	release, err := s.attempt(func(share *smb2.Share) error {
		ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
		defer cancel()
		got, err := share.WithContext(ctx).ReadDir(target)
		infos = got

		return err
	})
	if err != nil {
		return nil, classify(err)
	}
	release()

	entries := make([]storage.Entry, 0, len(infos))
	for _, info := range infos {
		if info.Name() == "." || info.Name() == ".." {
			continue
		}
		entry := storage.Entry{Name: info.Name(), Dir: info.IsDir()}
		if !entry.Dir {
			entry.Size = info.Size()
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// Open rend le contenu d'un fichier, chemin relatif au montage. L'appelant
// ferme — et tant qu'il ne l'a pas fait, la session reste ouverte même si
// la source est fermée entre-temps.
func (s *Source) Open(name string) (io.ReadCloser, error) {
	target, err := s.resolve(name)
	if err != nil {
		return nil, err
	}

	var file *smb2.File
	release, err := s.attempt(func(share *smb2.Share) error {
		got, err := share.Open(target)
		file = got

		return err
	})
	if err != nil {
		return nil, classify(err)
	}

	return &openFile{File: file, release: release}, nil
}

// openFile rend sa place dans le compte des usages quand on le ferme.
type openFile struct {
	*smb2.File
	release func()
}

func (f *openFile) Close() error {
	err := f.File.Close()
	f.release()

	return err
}

// Close met fin à la session — dès que plus rien ne s'en sert.
//
// Appelée quand un kernel oublie la source (un « Relire », un partage
// modifié). Sans elle, chaque oubli laissait une connexion TCP et sa
// goroutine de réception ouvertes jusqu'à ce que le serveur s'en lasse.
func (s *Source) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	if s.active == 0 {
		s.disconnect()
	}

	return nil
}

// attempt fait une opération sur la session vivante, en la rouvrant une fois
// si la connexion est morte sous elle. En cas de succès, l'usage reste
// compté et c'est à l'appelant d'appeler release ; en cas d'échec, c'est déjà
// fait.
func (s *Source) attempt(operation func(share *smb2.Share) error) (release func(), err error) {
	for try := 0; ; try++ {
		share, err := s.acquire()
		if err != nil {
			return nil, err
		}

		err = operation(share)
		if err == nil {
			var once sync.Once

			return func() { once.Do(s.release) }, nil
		}

		broken := brokenConnection(err)
		if broken {
			s.drop(share)
		}
		s.release()
		if !broken || try > 0 {
			return nil, err
		}
	}
}

// acquire rend la session, en l'ouvrant au besoin, et compte un usage.
func (s *Source) acquire() (*smb2.Share, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, errClosed
	}
	if s.fs == nil {
		if err := s.connect(); err != nil {
			return nil, err
		}
	}
	s.active++

	return s.fs, nil
}

func (s *Source) release() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.active--
	if s.closed && s.active == 0 {
		s.disconnect()
	}
}

// connect ouvre la connexion, la session et le partage. Verrou tenu.
func (s *Source) connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	// KeepAlive : une opération sur un fichier ouvert n'a pas de délai (un
	// téléchargement peut durer), donc c'est TCP qui doit s'apercevoir
	// qu'un serveur a disparu, plutôt qu'une lecture qui attendrait toujours.
	dialer := net.Dialer{KeepAlive: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.hostport)
	if err != nil {
		return err
	}

	username := s.username
	if username == "" {
		username = guestAccount
	}
	session, err := (&smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{User: username, Password: s.password},
	}).DialContext(ctx, conn)
	if err != nil {
		_ = conn.Close()

		return err
	}

	share, err := session.WithContext(ctx).Mount(s.share)
	if err != nil {
		_ = session.WithContext(ctx).Logoff()
		_ = conn.Close()

		return err
	}

	s.conn, s.session, s.fs = conn, session, share

	return nil
}

// drop abandonne une session dont la connexion est tombée — sans lui dire au
// revoir : un au revoir à un pair mort attendrait son délai pour rien.
func (s *Source) drop(broken *smb2.Share) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Une autre opération a déjà rouvert : ne pas couper la neuve.
	if s.fs != broken {
		return
	}
	_ = s.conn.Close()
	s.conn, s.session, s.fs = nil, nil, nil
}

// disconnect ferme poliment ce qui est ouvert. Verrou tenu.
func (s *Source) disconnect() {
	if s.conn == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	_ = s.fs.WithContext(ctx).Umount()
	_ = s.session.WithContext(ctx).Logoff()
	_ = s.conn.Close()
	s.conn, s.session, s.fs = nil, nil, nil
}

// resolve — le chemin, dans le partage, d'un chemin RELATIF au montage.
func (s *Source) resolve(name string) (string, error) {
	cleaned, err := clean(name)
	if err != nil {
		return "", err
	}

	return strings.ReplaceAll(strings.Trim(path.Join(s.root, cleaned), "/"), "/", `\`), nil
}

// clean — un chemin relatif propre, sans barres autour, "" pour la racine.
//
// La remontée est détectée AVANT le nettoyage : path.Clean aplatirait
// « ../../x » en « x » et on lirait un autre fichier que celui demandé, sans
// rien dire. Même règle que la source locale et la source WebDAV.
func clean(name string) (string, error) {
	trimmed := strings.Trim(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"), "/")
	depth := 0
	for _, segment := range strings.Split(trimmed, "/") {
		switch segment {
		case "", ".":
		case "..":
			depth--
			if depth < 0 {
				return "", storage.ErrOutsideSource
			}
		default:
			depth++
		}
	}

	cleaned := strings.Trim(path.Clean("/"+trimmed), "/")

	return cleaned, nil
}

// classify — l'erreur de la bibliothèque, traduite en ce qu'on peut corriger.
//
// L'erreur d'origine reste dans le message : « identifiants refusés » dit
// quoi faire, le code NTSTATUS dit ce que le serveur a répondu, et celui qui
// lit un journal a besoin des deux.
func classify(err error) error {
	if err == nil {
		return nil
	}

	var response *smb2.ResponseError
	if errors.As(err, &response) {
		switch response.Code {
		case statusLogonFailure, statusAccountRestriction, statusPasswordExpired,
			statusAccountDisabled, statusAccountLockedOut:
			return fmt.Errorf("%w (%v)", ErrUnauthorized, err)
		case statusBadNetworkName:
			return fmt.Errorf("%w (%v)", ErrShareNotFound, err)
		}
	}
	// La bibliothèque traduit elle-même STATUS_ACCESS_DENIED.
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w (%v)", ErrForbidden, err)
	}
	var invalid *smb2.InvalidResponseError
	if errors.As(err, &invalid) && strings.Contains(invalid.Message, "doesn't support signing") {
		return fmt.Errorf("%w (%v)", ErrGuestRefused, err)
	}

	return err
}

// brokenConnection — l'échec vient-il du LIEN, et non de la demande ?
//
// Seul ce cas-là justifie de rouvrir et de réessayer : un mot de passe
// refusé ou un dossier absent le resteraient, et réessayer ne ferait que
// doubler l'attente.
func brokenConnection(err error) bool {
	var transport *smb2.TransportError
	if errors.As(err, &transport) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}

	var response *smb2.ResponseError
	if errors.As(err, &response) {
		switch response.Code {
		case statusNetworkNameDeleted, statusUserSessionDeleted, statusNetworkSessionExpired:
			return true
		}
	}

	return false
}

// Probe regarde la racine du montage (le dossier configuré) et rend ce
// qu'elle contient. Un seul niveau : c'est une vérification, pas un scan.
func Probe(config Config) (storage.ProbeResult, error) {
	source, err := New(config)
	if err != nil {
		return storage.ProbeResult{}, err
	}
	defer source.Close()

	entries, err := source.List("")
	if err != nil {
		return storage.ProbeResult{}, err
	}

	result := storage.ProbeResult{Folders: []string{}}
	for _, entry := range entries {
		if entry.Dir {
			result.Folders = append(result.Folders, entry.Name)

			continue
		}
		result.Files++
	}

	return result, nil
}
