// Package webdav monte un partage WebDAV comme une source de la
// bibliothèque — chantier « ressources externes ».
//
// Contrairement à `remote` (un autre serveur GameFlex, qui a déjà classé ses
// jeux et rend un catalogue), un partage WebDAV est un ARBRE DE DOSSIERS :
// exactement ce que storage.Source décrit, et exactement ce que le scanner
// sait interpréter. C'est ce que le commentaire de `remote` annonçait —
// « SMB et WebDAV, eux, sont bien des arbres — ils passeront par Source » —
// et il n'y a donc rien de neuf à inventer côté scan. Un dossier `ps1` sur
// un NAS se lit comme un dossier `ps1` sur le disque.
//
// kDrive n'est pas un protocole de plus
// -------------------------------------
// kDrive (Infomaniak) EST un serveur WebDAV, et il n'a pas d'autre porte
// d'entrée pour ce qu'on veut faire. Il se réduit donc à une adresse d'une
// forme particulière, construite à partir de l'identifiant du drive — voir
// KDriveURL. Le seul point qui le distingue vraiment est humain : WebDAV y
// refuse le mot de passe principal du compte, il faut un mot de passe
// d'application créé dans le Manager. C'est au formulaire de le dire ; le
// code, lui, n'a qu'un couple identifiant/mot de passe.
package webdav

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"gameflex/storage"
)

// ErrUnauthorized — l'identifiant ou le mot de passe est refusé. Distingué
// d'une panne réseau parce que les deux ne se corrigent pas au même endroit :
// l'un dans le formulaire, l'autre chez son fournisseur d'accès.
var ErrUnauthorized = errors.New("webdav: identifiants refusés")

// ErrForbidden — le serveur refuse SANS proposer de s'authentifier.
//
// Ce n'est pas la même panne, et les confondre envoie chercher au mauvais
// endroit. Un 401 dit « qui es-tu ? » et rejette une réponse ; un 403 dit
// « non », point — les identifiants n'y changent rien.
//
// Le cas vu en vrai (2026-09-02) : un kDrive dont le WebDAV n'est pas activé
// répond 403 à tout, sans en-tête WWW-Authenticate, avec ou sans mot de
// passe. On accusait alors le mot de passe d'application, et son propriétaire
// pouvait en fabriquer dix sans que rien ne change.
var ErrForbidden = errors.New("webdav: accès refusé par le serveur")

// Config — ce qu'il faut pour monter un partage.
type Config struct {
	// Name est le nom de montage, premier segment du chemin virtuel.
	Name string
	// BaseURL est l'adresse du partage, avec son schéma. Le chemin qu'elle
	// porte devient la racine : un utilisateur qui range ses jeux dans
	// /Public/roms monte cette adresse-là, et non la racine du drive.
	BaseURL  string
	Username string
	Password string
	// Client est injectable pour les tests. nil = un client par défaut, avec
	// un délai d'attente : un partage muet ne doit pas suspendre un scan.
	Client *http.Client
}

// Source — un partage WebDAV vu comme une source de bibliothèque.
type Source struct {
	name   string
	base   *url.URL
	user   string
	pass   string
	client *http.Client
}

// New monte un partage. L'adresse est vérifiée ici : une adresse illisible
// se corrige dans le formulaire, pas au milieu d'un scan.
func New(config Config) (*Source, error) {
	if config.Name == "" {
		return nil, storage.ErrEmptyName
	}

	base, err := url.Parse(strings.TrimRight(config.BaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("webdav: adresse invalide: %q", config.BaseURL)
	}

	client := config.Client
	if client == nil {
		// Sans limite de DURÉE : un gros jeu a le droit d'être long.
		// Voir storage.NewTransferClient — c'est l'attente qu'on borne.
		client = storage.NewTransferClient()
	}

	return &Source{
		name:   config.Name,
		base:   base,
		user:   config.Username,
		pass:   config.Password,
		client: client,
	}, nil
}

func (s *Source) Name() string { return s.name }

// Kind — ce qu'on affiche à côté d'un jeu. kDrive et un NAS WebDAV rendent
// le même mot : c'est le même protocole, et l'origine précise se lit au nom
// du montage, que son propriétaire a choisi.
func (s *Source) Kind() string { return storage.KindWebDAV }

// List — le contenu d'un dossier, par un PROPFIND de profondeur 1.
//
// Profondeur 1 et non « infinity » : le scanner descend lui-même, et
// beaucoup de serveurs refusent purement et simplement l'infinie (elle
// permet de leur faire énumérer tout un disque d'une seule requête).
func (s *Source) List(dir string) ([]storage.Entry, error) {
	target, err := s.resolve(dir)
	if err != nil {
		return nil, err
	}

	// LA BARRE FINALE N'EST PAS COSMÉTIQUE : sans elle, un serveur qui range
	// ses dossiers proprement REDIRIGE (301 vers l'adresse avec barre), et le
	// client Go suit la redirection EN DÉGRADANT la méthode en GET. On reçoit
	// alors la page d'index HTML du serveur à la place du XML, et le décodeur
	// rend « réponse illisible: expected element type <multistatus> but have
	// <html> » — un message qui n'aide personne à trouver la cause.
	//
	// Mesuré le 2026-09-18 sur un partage Apache jetable (mod_dav) : le
	// journal du serveur montre les deux requêtes à la suite,
	// « PROPFIND /roms -> 301 » puis « GET /roms/ -> 200 ». Le scan d'un
	// partage échouait donc entièrement, et c'est le cas ORDINAIRE — la racine
	// d'une source est `resolve("")`, qui rend la base nue (voir resolve :
	// `cleaned` devient vide, et `s.base.Path` est déjà débarrassé de sa
	// barre par New). Apache redirige par défaut, Nextcloud et beaucoup
	// d'autres aussi.
	//
	// Ici et non dans `resolve` : celui-ci sert AUSSI aux fichiers (Open, Put),
	// où une barre finale désignerait une collection et casserait la requête.
	// Seul le listage parle de collections.
	if !strings.HasSuffix(target.Path, "/") {
		target.Path += "/"
	}

	request, err := http.NewRequestWithContext(context.Background(), "PROPFIND", target.String(), strings.NewReader(propfindBody))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Depth", "1")
	request.Header.Set("Content-Type", "application/xml; charset=utf-8")
	request.SetBasicAuth(s.user, s.pass)

	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case http.StatusForbidden:
		return nil, ErrForbidden
	case http.StatusNotFound:
		// Un dossier disparu n'est pas une panne de scan — même règle que
		// la source locale, pour que la bibliothèque survive à un dossier
		// renommé chez quelqu'un d'autre.
		return nil, nil
	}
	if response.StatusCode != http.StatusMultiStatus && response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("webdav: PROPFIND %s: %s", target.Path, response.Status)
	}

	var multi multistatus
	if err := xml.NewDecoder(response.Body).Decode(&multi); err != nil {
		return nil, fmt.Errorf("webdav: réponse illisible: %w", err)
	}

	// Le dossier interrogé fait partie de la réponse : c'est la règle du
	// protocole, et le premier élément est lui-même. On le reconnaît à son
	// chemin, pas à sa position — l'ordre n'est garanti nulle part.
	self := strings.Trim(target.EscapedPath(), "/")

	entries := make([]storage.Entry, 0, len(multi.Responses))
	for _, item := range multi.Responses {
		href, err := url.PathUnescape(strings.Trim(item.Href, "/"))
		if err != nil {
			continue
		}
		if trimmed := strings.Trim(item.Href, "/"); trimmed == self {
			continue
		}

		name := path.Base(href)
		if name == "" || name == "." || name == "/" {
			continue
		}

		entry := storage.Entry{Name: name, Dir: item.isDir()}
		if !entry.Dir {
			entry.Size = item.size()
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// Open — le contenu d'un fichier. C'est ce qui permet de LANCER un jeu qui
// vit sur le partage, et non seulement de le voir dans la liste.
func (s *Source) Open(name string) (io.ReadCloser, error) {
	target, err := s.resolve(name)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.SetBasicAuth(s.user, s.pass)

	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusForbidden {
		response.Body.Close()

		return nil, ErrForbidden
	}
	if response.StatusCode == http.StatusUnauthorized {
		response.Body.Close()

		return nil, ErrUnauthorized
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()

		return nil, fmt.Errorf("webdav: GET %s: %s", target.Path, response.Status)
	}

	// La taille vient de la réponse : la demander séparément voulait dire
	// lister le dossier parent avant chaque téléchargement.
	return storage.WithSize(response.Body, response.ContentLength), nil
}

// Put — écrire un fichier sur le partage.
//
// Le sens qui manque à une source de lecture seule, et la raison d'être de
// ce montage pour beaucoup de gens : une sauvegarde de partie doit pouvoir
// vivre AILLEURS que sur le téléphone, sans jamais partir chez quelqu'un
// d'autre. Les dossiers parents sont créés au besoin — un partage neuf est
// vide, et faire échouer la première sauvegarde là-dessus serait absurde.
func (s *Source) Put(name string, content io.Reader) error {
	target, err := s.resolve(name)
	if err != nil {
		return err
	}

	if err := s.mkdirAll(path.Dir(strings.Trim(name, "/"))); err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPut, target.String(), content)
	if err != nil {
		return err
	}
	request.SetBasicAuth(s.user, s.pass)

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusForbidden {
		return ErrForbidden
	}
	if response.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if response.StatusCode >= 300 {
		return fmt.Errorf("webdav: PUT %s: %s", target.Path, response.Status)
	}

	return nil
}

// mkdirAll — MKCOL sur chaque segment. Un 405 signifie « existe déjà », et
// c'est un succès : le protocole n'a pas d'équivalent de -p.
func (s *Source) mkdirAll(dir string) error {
	dir = strings.Trim(dir, "/")
	if dir == "" || dir == "." {
		return nil
	}

	built := ""
	for _, segment := range strings.Split(dir, "/") {
		if segment == "" {
			continue
		}
		built = path.Join(built, segment)

		target, err := s.resolve(built)
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(context.Background(), "MKCOL", target.String(), nil)
		if err != nil {
			return err
		}
		request.SetBasicAuth(s.user, s.pass)

		response, err := s.client.Do(request)
		if err != nil {
			return err
		}
		response.Body.Close()

		if response.StatusCode == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		if response.StatusCode >= 300 && response.StatusCode != http.StatusMethodNotAllowed {
			return fmt.Errorf("webdav: MKCOL %s: %s", target.Path, response.Status)
		}
	}

	return nil
}

// resolve — l'adresse d'un chemin RELATIF au montage.
//
// Le confinement est vérifié et non supposé, pour la même raison que côté
// local : un chemin venu du réseau qui remonte au-dessus de la racine
// donnerait accès à tout le partage, et pas seulement au dossier monté.
func (s *Source) resolve(name string) (*url.URL, error) {
	// La remontée est détectée AVANT le nettoyage, et non après.
	//
	// path.Clean("/../../etc/passwd") vaut "/etc/passwd" : il APLATIT la
	// remontée au lieu de la signaler. Le chemin ne sort donc pas du
	// montage — mais on lirait un autre fichier que celui demandé, sans
	// rien dire. Un chemin qui essaie de remonter est une erreur d'appelant,
	// et elle doit se voir (même règle que LocalSource, où un test avait
	// déjà démenti la supposition inverse).
	trimmed := strings.Trim(strings.ReplaceAll(name, "\\", "/"), "/")
	depth := 0
	for _, segment := range strings.Split(trimmed, "/") {
		switch segment {
		case "", ".":
		case "..":
			depth--
			if depth < 0 {
				return nil, storage.ErrOutsideSource
			}
		default:
			depth++
		}
	}

	cleaned := path.Clean("/" + trimmed)
	if cleaned == "/" {
		cleaned = ""
	}

	target := *s.base
	target.Path = strings.TrimRight(s.base.Path, "/") + cleaned
	// RawPath vide : url laisse alors Path s'encoder tout seul à l'envoi,
	// ce qui est ce qu'on veut pour un nom de jeu plein d'espaces et de
	// crochets.
	target.RawPath = ""

	return &target, nil
}

// ── La réponse du serveur ───────────────────────────────────────────────
//
// Les espaces de noms varient d'un serveur à l'autre (D:, d:, lp1:), donc on
// ne matche QUE le nom local des balises. Le préfixe n'est pas une
// information, c'est un choix du serveur.

const propfindBody = `<?xml version="1.0" encoding="utf-8"?>` +
	`<propfind xmlns="DAV:"><prop>` +
	`<resourcetype/><getcontentlength/>` +
	`</prop></propfind>`

type multistatus struct {
	XMLName   xml.Name      `xml:"multistatus"`
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string        `xml:"href"`
	Propstat []davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Prop davProp `xml:"prop"`
}

type davProp struct {
	ResourceType struct {
		Collection *struct{} `xml:"collection"`
	} `xml:"resourcetype"`
	ContentLength string `xml:"getcontentlength"`
}

func (r davResponse) isDir() bool {
	for _, stat := range r.Propstat {
		if stat.Prop.ResourceType.Collection != nil {
			return true
		}
	}

	return false
}

func (r davResponse) size() int64 {
	for _, stat := range r.Propstat {
		if stat.Prop.ContentLength == "" {
			continue
		}
		if size, err := strconv.ParseInt(stat.Prop.ContentLength, 10, 64); err == nil {
			return size
		}
	}

	return 0
}

// ── La sonde ────────────────────────────────────────────────────────────

// Probe regarde UN dossier d'un partage et rend ce qu'il contient.
//
// Volontairement à un seul niveau : c'est une vérification, pas un scan. Un
// drive entier met des minutes à se parcourir, et personne n'attend devant un
// formulaire pour apprendre que son mot de passe est bon.
//
// Les erreurs sont celles du paquet (ErrUnauthorized notamment) : l'appelant
// peut donc distinguer « je n'ai pas le droit » de « je n'ai pas trouvé »,
// qui ne se corrigent pas du tout de la même façon.
func Probe(config Config, folder string) (storage.ProbeResult, error) {
	source, err := New(config)
	if err != nil {
		return storage.ProbeResult{}, err
	}

	entries, err := source.List(folder)
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
