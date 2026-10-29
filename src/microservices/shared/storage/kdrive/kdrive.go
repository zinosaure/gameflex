// Package kdrive lit un kDrive d'Infomaniak par son API REST.
//
// POURQUOI PAS WEBDAV, qui est déjà écrit : parce que kDrive ne l'ouvre
// qu'aux offres Team et Pro. Sur toute autre offre le drive répond « accès
// refusé » à tout, sans même demander qui vous êtes — vérifié en vrai le
// 2026-09-02 : une adresse de drive inventée rend 404, une vraie rend 403.
// L'API REST, elle, s'authentifie par jeton et ne passe pas par là.
//
// LA DIFFÉRENCE DE FOND avec toutes les autres sources : ici un fichier
// s'adresse par un NUMÉRO, pas par un chemin. Le scanner, lui, ne connaît que
// des chemins. Tout ce paquet tient dans ce décalage — on descend l'arbre
// niveau par niveau pour traduire un chemin en numéro, et on retient la
// correspondance, sinon chaque dossier d'une bibliothèque de vingt mille jeux
// se paierait un aller-retour de plus.
package kdrive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gameflex/storage"
)

// ErrUnauthorized — le jeton est refusé. Distingué du reste : il se corrige
// en changeant de jeton, pas en cherchant un dossier.
var ErrUnauthorized = errors.New("kdrive: jeton refusé")

// baseURL — l'API publique d'Infomaniak. Variable pour les tests, jamais
// changée en production.
var baseURL = "https://api.infomaniak.com"

// Config décrit un drive à monter.
type Config struct {
	// Name — le nom de montage, premier segment du chemin virtuel.
	Name string
	// DriveID — le nombre qui identifie le drive (celui qui suit /drive/
	// dans son adresse web).
	DriveID string
	// Token — un jeton d'API Infomaniak. C'est la seule chose qui
	// authentifie : il n'y a ni login ni mot de passe ici.
	Token string
	// Folder — le sous-dossier à parcourir. Vide = la racine du drive.
	Folder string
	// Client — pour les tests. Nil = un client avec un délai d'attente.
	Client *http.Client
}

// Source est un kDrive monté.
type Source struct {
	name    string
	driveID string
	token   string
	folder  string
	client  *http.Client

	// ids — la table « chemin → numéro », remplie en descendant. Le zéro
	// désigne la racine du drive, qui se liste par une autre route.
	mu  sync.Mutex
	ids map[string]int64
}

// New monte un drive. L'appel ne touche PAS au réseau : un montage qui
// échouerait au démarrage empêcherait la bibliothèque de se charger, alors
// qu'un drive injoignable ne doit faire disparaître que SES jeux.
func New(config Config) (*Source, error) {
	if strings.TrimSpace(config.Name) == "" {
		return nil, storage.ErrEmptyName
	}
	if strings.TrimSpace(config.DriveID) == "" {
		return nil, errors.New("kdrive: un identifiant de drive est requis")
	}
	if strings.TrimSpace(config.Token) == "" {
		return nil, errors.New("kdrive: un jeton est requis")
	}

	client := config.Client
	if client == nil {
		// Sans limite de DURÉE : un gros jeu a le droit d'être long.
		// Voir storage.NewTransferClient — c'est l'attente qu'on borne.
		client = storage.NewTransferClient()
	}

	// LE BANC D'ESSAI, et rien d'autre : GAMEFLEX_KDRIVE_API remplace
	// l'adresse de l'API par celle du faux drive. Sans cette couture, le
	// kernel réel ne pourrait jamais parler au kDrive factice du banc —
	// et un banc où la source qu'on teste ne répond pas est un banc qui
	// ne prouve rien. Laisse à vide en production : le comportement est
	// strictement celui d'avant.
	api := strings.TrimSuffix(strings.TrimSpace(os.Getenv("GAMEFLEX_KDRIVE_API")), "/")
	if api != "" && api != baseURL {
		baseURL = api
	}

	return &Source{
		name:    config.Name,
		driveID: strings.TrimSpace(config.DriveID),
		token:   strings.TrimSpace(config.Token),
		folder:  strings.Trim(strings.TrimSpace(config.Folder), "/"),
		client:  client,
		ids:     map[string]int64{"": 0},
	}, nil
}

func (s *Source) Name() string { return s.name }

// Kind — le kDrive se signale comme lui-même et non comme du WebDAV : ce
// n'est plus le même protocole, et l'interface le dit à qui regarde d'où
// vient un jeu.
func (s *Source) Kind() string { return "kdrive" }

// List rend le contenu d'un dossier, chemin relatif à la source.
func (s *Source) List(dir string) ([]storage.Entry, error) {
	id, err := s.resolve(dir)
	if err != nil {
		// Un dossier absent n'est pas une panne : la bibliothèque doit
		// survivre à un dossier renommé chez quelqu'un d'autre, comme partout
		// ailleurs dans ce paquet.
		if errors.Is(err, errNotFound) {
			return nil, nil
		}

		return nil, err
	}

	files, err := s.children(id)
	if err != nil {
		return nil, err
	}

	// ON RETIENT LES NUMÉROS EN PASSANT : le parcours voit déjà tous ces
	// fichiers, et ne pas les noter obligeait à relister le dossier entier
	// pour en ouvrir un seul (voir Open).
	s.rememberAll(dir, files)

	entries := make([]storage.Entry, 0, len(files))
	for _, file := range files {
		entries = append(entries, storage.Entry{
			Name: file.Name,
			Dir:  file.isDir(),
			Size: file.Size,
		})
	}

	return entries, nil
}

// ExportMemo / ImportMemo — les numéros appris, pour que le prochain
// démarrage n'ait pas à refaire la marche. Voir storage.Memo.
func (s *Source) ExportMemo() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	learned := make(map[string]string, len(s.ids))
	for path, id := range s.ids {
		if path != "" && id != 0 {
			learned[path] = strconv.FormatInt(id, 10)
		}
	}

	return learned
}

func (s *Source) ImportMemo(learned map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for path, raw := range learned {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		s.ids[path] = id
	}
}

// rememberAll note le numéro de tout ce qu'un dossier contient.
//
// Appelé par les DEUX chemins qui listent — le parcours et l'ouverture.
// N'être appelé que par le premier laissait l'ouverture relister le même
// dossier à chaque fichier, y compris juste après l'avoir fait elle-même :
// trente secondes par cartouche, deux fois de suite (mesuré le 2026-09-02).
func (s *Source) rememberAll(dir string, files []file) {
	base := strings.Trim(strings.ReplaceAll(dir, "\\", "/"), "/")

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range files {
		path := entry.Name
		if base != "" {
			path = base + "/" + entry.Name
		}
		s.ids[path] = entry.ID
	}
}

// lookupID — le numéro déjà connu de ce chemin, s'il l'est.
func (s *Source) lookupID(path string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, known := s.ids[path]

	return id, known && id != 0
}

// Open lit un fichier.
func (s *Source) Open(name string) (io.ReadCloser, error) {
	trimmed := strings.Trim(strings.ReplaceAll(name, "\\", "/"), "/")
	cut := strings.LastIndex(trimmed, "/")
	dir, file := "", trimmed
	if cut >= 0 {
		dir, file = trimmed[:cut], trimmed[cut+1:]
	}

	// LE NUMÉRO DÉJÀ CONNU, s'il l'est — AVANT toute question au réseau.
	//
	// L'API n'adresse que par numéro, donc ouvrir un fichier suppose de
	// savoir lequel. On listait le dossier parent à CHAQUE téléchargement —
	// 10 783 entrées paginées pour une cartouche NES de 41 Ko, et un délai
	// dépassé côté application avant même le premier octet (2026-09-02).
	//
	// Le parcours de la bibliothèque a déjà vu ces fichiers : il retient
	// maintenant leur numéro en passant, et l'ouverture n'a plus rien à
	// redemander.
	if id, known := s.lookupID(trimmed); known {
		return s.download(id)
	}

	parent, err := s.resolve(dir)
	if err != nil {
		return nil, err
	}

	files, err := s.children(parent)
	if err != nil {
		return nil, err
	}
	// Ce dossier vient d'être payé : on ne le repaiera pas au fichier
	// suivant.
	s.rememberAll(dir, files)

	for _, candidate := range files {
		if candidate.Name != file || candidate.isDir() {
			continue
		}

		return s.download(candidate.ID)
	}

	return nil, fmt.Errorf("kdrive: %s: %w", name, errNotFound)
}

var errNotFound = errors.New("kdrive: introuvable")

// resolve traduit un chemin en numéro de dossier, en descendant niveau par
// niveau depuis la racine — et en retenant chaque étape.
//
// La descente est la seule voie : l'API n'expose pas « donne-moi le dossier
// à ce chemin ». Sans la mémoire, scanner vingt consoles referait vingt fois
// le même premier niveau.
func (s *Source) resolve(dir string) (int64, error) {
	full := s.join(dir)

	s.mu.Lock()
	if id, known := s.ids[full]; known {
		s.mu.Unlock()

		return id, nil
	}
	s.mu.Unlock()

	current := int64(0)
	walked := ""
	for _, segment := range strings.Split(full, "/") {
		if segment == "" || segment == "." {
			continue
		}
		if segment == ".." {
			// Une source doit rester une source, quoi qu'on lui passe.
			return 0, storage.ErrOutsideSource
		}

		walked = strings.TrimPrefix(walked+"/"+segment, "/")

		s.mu.Lock()
		known, cached := s.ids[walked]
		s.mu.Unlock()
		if cached {
			current = known

			continue
		}

		files, err := s.children(current)
		if err != nil {
			return 0, err
		}

		found := false
		for _, file := range files {
			if !file.isDir() {
				continue
			}
			s.mu.Lock()
			s.ids[strings.TrimPrefix(walked[:len(walked)-len(segment)]+file.Name, "/")] = file.ID
			s.mu.Unlock()
			if file.Name == segment {
				current, found = file.ID, true
			}
		}
		if !found {
			return 0, fmt.Errorf("kdrive: %s: %w", walked, errNotFound)
		}
	}

	s.mu.Lock()
	s.ids[full] = current
	s.mu.Unlock()

	return current, nil
}

// join place un chemin sous le dossier du montage.
func (s *Source) join(dir string) string {
	dir = strings.Trim(strings.ReplaceAll(dir, "\\", "/"), "/")
	switch {
	case s.folder == "":
		return dir
	case dir == "":
		return s.folder
	}

	return s.folder + "/" + dir
}

// file — une entrée telle que l'API la rend.
//
// LES NOMS DE CHAMPS SONT TOLÉRANTS À DESSEIN. Le type d'une entrée est lu
// de deux façons parce que l'API a porté les deux formes selon les versions,
// et se tromper ici transforme un dossier en fichier — le scanner cesse alors
// de descendre, et la bibliothèque paraît vide sans qu'une seule erreur ne
// soit remontée. Mieux vaut accepter les deux que de parier.
type file struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Kind string `json:"file_type"`
	Dir  bool   `json:"is_directory"`
	Size int64  `json:"size"`
}

func (f file) isDir() bool {
	return f.Dir || f.Type == "dir" || f.Type == "directory" ||
		f.Kind == "dir" || f.Kind == "directory"
}

// children liste le contenu d'un dossier. Le numéro zéro désigne la racine,
// qui a sa propre route.
func (s *Source) children(id int64) ([]file, error) {
	path := "/2/drive/" + url.PathEscape(s.driveID) + "/files"
	if id != 0 {
		path += "/" + strconv.FormatInt(id, 10) + "/files"
	}

	// Une page par défaut ne suffit pas pour un dossier de jeux : on demande
	// le maximum et on tourne tant qu'il en revient autant.
	const perPage = 1000
	all := []file{}
	for page := 1; ; page++ {
		var envelope struct {
			Result string `json:"result"`
			Data   []file `json:"data"`
			Error  struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		}
		query := fmt.Sprintf("?per_page=%d&page=%d", perPage, page)
		if err := s.get(path+query, &envelope); err != nil {
			return nil, err
		}
		if envelope.Result == "error" {
			return nil, fmt.Errorf("kdrive: %s: %s",
				envelope.Error.Code, envelope.Error.Description)
		}

		all = append(all, envelope.Data...)
		if len(envelope.Data) < perPage {
			return all, nil
		}
	}
}

// download ouvre le contenu d'un fichier.
func (s *Source) download(id int64) (io.ReadCloser, error) {
	target := baseURL + "/2/drive/" + url.PathEscape(s.driveID) +
		"/files/" + strconv.FormatInt(id, 10) + "/download"

	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+s.token)

	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusUnauthorized ||
		response.StatusCode == http.StatusForbidden {
		response.Body.Close()

		return nil, ErrUnauthorized
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()

		return nil, fmt.Errorf("kdrive: téléchargement %d : %s", id, response.Status)
	}

	// La taille vient de la réponse : la demander séparément voulait dire
	// lister le dossier parent avant chaque téléchargement.
	return storage.WithSize(response.Body, response.ContentLength), nil
}

// get interroge l'API, avec une SECONDE CHANCE sur une panne passagère.
//
// Parcourir un drive demande des centaines de requêtes : un seul hoquet du
// serveur perdait le parcours entier (constaté en vrai le 2026-09-02, un 500
// sur la toute première page). On réessaie donc une fois sur un 5xx — et
// JAMAIS sur un 4xx : un jeton refusé ou un dossier absent ne s'arrangeront
// pas en redemandant, et insister reviendrait à marteler le serveur de
// quelqu'un d'autre.
func (s *Source) get(path string, into any) error {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}

		retryable, err := s.getOnce(path, into)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable {
			return err
		}
	}

	return lastErr
}

// getOnce rend l'erreur ET si elle vaut la peine d'être réessayée.
func (s *Source) getOnce(path string, into any) (retryable bool, err error) {
	request, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+s.token)
	request.Header.Set("Accept", "application/json")

	response, err := s.client.Do(request)
	if err != nil {
		// Réseau coupé, délai dépassé : ça peut passer.
		return true, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized ||
		response.StatusCode == http.StatusForbidden {
		return false, ErrUnauthorized
	}
	if response.StatusCode >= 500 {
		return true, fmt.Errorf("kdrive: %s: %s", path, response.Status)
	}
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("kdrive: %s: %s", path, response.Status)
	}

	if err := json.NewDecoder(response.Body).Decode(into); err != nil {
		// Une réponse tronquée est un accident de transport, pas un refus.
		return true, err
	}

	return false, nil
}

// Probe regarde UN dossier et rend ce qu'il contient — le pendant de celle
// du paquet webdav, et pour la même raison : « joignable » ne dit pas si on a
// mis le doigt au bon endroit.
func Probe(config Config) (storage.ProbeResult, error) {
	source, err := New(config)
	if err != nil {
		return storage.ProbeResult{}, err
	}

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
