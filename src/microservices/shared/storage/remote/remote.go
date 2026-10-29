// Package remote parle à un autre serveur GameFlex — chantier 6, revu au
// chantier 15.
//
// Pourquoi ce n'est PAS une storage.Source
// ----------------------------------------
// Une Source est un arbre de dossiers qu'on liste, et que le scanner
// interprète : un nom de dossier donne la console, une extension confirme.
// Un serveur GameFlex, lui, a DÉJÀ fait ce travail — son LISTING rend ses jeux
// classés par console, avec leur UUID, leurs fichiers et leur taille, et avec
// eux ses BIOS, ses cœurs, ses correctifs et ses paramètres.
//
// Le faire passer pour un arbre reviendrait à défaire son classement pour
// que le scanner le refasse, avec deux occasions de tomber sur un désaccord :
// une console qu'il connaît et que nous non, une extension qu'il accepte et
// que nous refusons. Ce serait la deuxième copie d'une même vérité, le
// défaut que ce dépôt passe son temps à retirer.
//
// Donc : son listing se copie tel quel chez nous (copies.go), au format de
// tous les listings. Ce paquet n'est pas un montage, c'est un client.
//
// SMB et WebDAV, eux, sont bien des arbres — ils passent par Source.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gameflex/storage/listing"
)

var (
	// ErrDenied — le jeton n'est pas (ou plus) accepté. À distinguer d'une
	// panne : un serveur injoignable se retente, un jeton révoqué non.
	ErrDenied = errors.New("jeton refusé par le serveur")
	// ErrNoLink — un serveur sans adresse n'est pas un serveur.
	ErrNoLink = errors.New("adresse de serveur manquante")
)

// Server est un serveur GameFlex distant, tel qu'on l'a ajouté : une
// adresse et un jeton.
type Server struct {
	link  string
	token string
	// client — les TRANSFERTS (Listing, OpenRom, OpenBios). Généreux : un jeu
	// peut peser des gigaoctets.
	client *http.Client
	// lister — les LISTES (identité, consoles, date du listing).
	//
	// Court exprès. Elles ne rendent que quelques kilo-octets, et surtout
	// personne ne les abandonnait : `handleFederationDevices` du kernel
	// mobile interroge ses pairs EN SÉRIE sans échéance, donc hors ligne un
	// téléphone attendait jusqu'à deux minutes PAR PAIR avant d'afficher
	// quoi que ce soit (#149, mesuré le 2026-09-17). Le chemin du catalogue
	// s'était déjà fabriqué son propre client à 20 s pour cette raison, en
	// double de celui-ci — c'est cette asymétrie qui disparaît ici.
	lister *http.Client
	// relay — un jeu RELAYÉ tel quel à un navigateur qui le télécharge
	// (`StreamRom`). Sans échéance : la durée d'un transfert de plusieurs
	// gigaoctets ne se borne pas d'avance, et c'est le contexte de l'appelant
	// — la requête du navigateur — qui l'arrête s'il abandonne.
	relay *http.Client
}

// Identity est ce qu'un serveur dit de lui-même.
type Identity struct {
	// Hostname est le nom qu'il se donne — celui qui s'affichera sous ses
	// jeux.
	Hostname string `json:"hostname"`
	// Identity est le nom du jeton, donc SOUS QUEL NOM il nous connaît.
	Identity string `json:"identity"`
	// Scopes — ce que ce jeton nous ouvre chez lui : « catalog » (son
	// listing, ses fichiers), « devices » (la liste de ses consoles à
	// distance). Jouer chez lui ne passe jamais par ce jeton, mais par celui
	// d'un compte de chez lui (Session).
	Scopes []string `json:"scopes,omitempty"`
}

// New prépare un client. Ne joint rien : c'est Check() qui parle au réseau,
// pour qu'un constructeur ne puisse pas bloquer une interface.
func New(link, token string) (*Server, error) {
	link = strings.TrimRight(strings.TrimSpace(link), "/")
	if link == "" {
		return nil, ErrNoLink
	}
	if _, err := url.Parse(link); err != nil {
		return nil, fmt.Errorf("adresse invalide : %w", err)
	}

	return &Server{
		link:  link,
		token: strings.TrimSpace(token),
		// Un listing de 23 000 jeux pèse quelques mégaoctets ; un jeu pèse
		// bien plus. La limite est donc généreuse pour un TRANSFERT.
		client: &http.Client{Timeout: 2 * time.Minute},
		// Une liste, elle, arrive vite ou n'arrivera pas : cinq secondes
		// suffisent, et un pair éteint ne retient plus l'affichage.
		lister: &http.Client{Timeout: listTimeout},
		relay:  &http.Client{},
	}, nil
}

// Link est l'adresse telle qu'elle a été saisie, normalisée.
func (server *Server) Link() string { return server.link }

// listTimeout — au-delà, un pair qui ne répond pas ne doit plus retenir une
// liste. Voir le champ `lister` pour le pourquoi.
const listTimeout = 5 * time.Second

// get — un TRANSFERT. Pas d'échéance courte, et surtout pas de contexte :
// l'appelant lit le corps APRÈS le retour (OpenRom rend `response.Body`), et
// un contexte expiré avorterait le téléchargement en pleine lecture.
func (server *Server) get(path string) (*http.Response, error) {
	return server.do(path, server.client)
}

// list — une LISTE, bornée court. Tout le reste du paquet passe par ici.
func (server *Server) list(path string) (*http.Response, error) {
	return server.do(path, server.lister)
}

func (server *Server) do(path string, client *http.Client) (*http.Response, error) {
	return server.doContext(context.Background(), path, client)
}

func (server *Server) doContext(ctx context.Context, path string, client *http.Client) (*http.Response, error) {
	return server.send(ctx, http.MethodGet, path, client)
}

func (server *Server) send(ctx context.Context, method, path string, client *http.Client) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, server.link+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+server.token)

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	switch response.StatusCode {
	case http.StatusOK:
		return response, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		response.Body.Close()
		return nil, ErrDenied
	default:
		response.Body.Close()
		return nil, &answeredError{code: response.StatusCode, status: response.Status}
	}
}

// answeredError — le serveur a répondu, mais pas 200 : son statut, pour qui
// doit distinguer un 404 (il ne connaît pas) d'une panne.
type answeredError struct {
	code   int
	status string
}

func (e *answeredError) Error() string { return "le serveur a répondu " + e.status }

// notFound — le serveur a répondu 404.
func notFound(err error) bool {
	var answered *answeredError

	return errors.As(err, &answered) && answered.code == http.StatusNotFound
}

// Check confirme que l'adresse ET le jeton sont bons, et rapporte le nom du
// serveur.
//
// Appelée au moment d'AJOUTER un serveur, avant d'enregistrer quoi que ce
// soit : sans elle, une faute de frappe ne se voit qu'au premier scan, sous
// la forme d'une liste vide qui ne dit pas pourquoi.
func (server *Server) Check() (Identity, error) {
	response, err := server.list("/api/v1/federation/identity")
	if err != nil {
		return Identity{}, err
	}
	defer response.Body.Close()

	var envelope struct {
		Data Identity `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return Identity{}, fmt.Errorf("réponse illisible : %w", err)
	}
	if envelope.Data.Hostname == "" {
		// Une réponse 200 sans nom de serveur n'est pas un serveur GameFlex
		// — c'est un portail captif, ou la page d'accueil de quelqu'un.
		return Identity{}, errors.New("ce n'est pas un serveur GameFlex")
	}

	return envelope.Data, nil
}

// Session — une session chez ce serveur, au nom d'un compte de chez lui
// (chantier 20) : celui dont un membre d'ici a collé le jeton, à la place de
// l'identifiant et du mot de passe d'autrefois. La page parle ensuite à l'hôte
// avec elle — son canal, le flux d'une partie —, comme avec n'importe quelle
// connexion de chez lui.
type Session struct {
	Token       string    `json:"token"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Consoles    []string  `json:"consoles"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// OpenSession — échanger le jeton de ce client contre une session chez lui :
// le jeton d'un COMPTE de chez lui, jamais celui du lien, qui n'ouvre que le
// catalogue. ErrDenied s'il le refuse (désactivé, échu, régénéré, compte
// supprimé). Chaque échange compte « une connexion » chez lui.
func (server *Server) OpenSession() (Session, error) {
	response, err := server.send(context.Background(), http.MethodPost, "/api/v1/federation/session", server.lister)
	if err != nil {
		return Session{}, err
	}
	defer response.Body.Close()

	var envelope struct {
		Data Session `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return Session{}, fmt.Errorf("réponse illisible : %w", err)
	}
	if envelope.Data.Token == "" {
		return Session{}, errors.New("ce serveur n'a pas rendu de session")
	}

	return envelope.Data, nil
}

// SessionNow — cette session vaut-elle encore chez lui, et que lance son compte
// À L'INSTANT ? Il en juge lui-même (`/api/v1/auth/me`) : un jeton désactivé ou
// un secret changé la tuent avant son échéance, et une console retirée au
// compte doit se voir sur la fiche dès sa prochaine ouverture — pas au prochain
// échange (relevé par la recette du chantier 20). Rend la session, ses
// consoles relues ; faux s'il la refuse ; une erreur s'il ne répond pas.
func (server *Server) SessionNow(session Session) (Session, bool, error) {
	holder := &Server{link: server.link, token: session.Token}
	response, err := holder.send(context.Background(), http.MethodGet, "/api/v1/auth/me", server.lister)
	if errors.Is(err, ErrDenied) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	defer response.Body.Close()

	var envelope struct {
		Item struct {
			Consoles *[]string `json:"consoles"`
		} `json:"item"`
	}
	// Une réponse sans consoles garde celles de l'échange : il a répondu, la
	// session vaut.
	if json.NewDecoder(response.Body).Decode(&envelope) == nil && envelope.Item.Consoles != nil {
		session.Consoles = *envelope.Item.Consoles
	}

	return session, true, nil
}

// Device — une console à distance vue chez un serveur fédéré.
//
// Ce type vit dans un paquet nommé `storage`, et ce n'est pas sa place au
// sens strict : une PS5 n'est pas un fichier. Il y vit quand même parce que
// ce paquet EST le client d'un GameFlex distant — l'adresse, le jeton, la
// façon de lire son enveloppe et de nommer ses échecs. En ouvrir un second
// pour une seule route donnerait deux clients à tenir d'accord, et c'est
// exactement ce qu'on évite ailleurs dans ce dépôt.
type Device struct {
	ID               string `json:"id"`
	HostID           string `json:"hostId"`
	HostName         string `json:"hostName"`
	HostType         string `json:"hostType"`
	IPAddress        string `json:"ipAddress"`
	Status           string `json:"status"`
	HasActiveSession bool   `json:"hasActiveSession"`
}

// Devices — les consoles à distance de ce serveur (chantier 9, C9-P2).
//
// Demande une portée que le jeton de catalogue N'A PAS : un serveur qui
// n'accorde que le catalogue répond 403, donc ErrDenied. L'appelant traite ce
// cas comme « ce serveur ne prête pas ses consoles », pas comme une panne —
// c'est une réponse, et elle est définitive tant que le jeton ne change pas.
//
// Un serveur qui ne connaît pas la route répond 404. Ce n'est pas ErrDenied
// et ce n'est pas grave — l'appelant le saute, comme il saute un serveur muet.
func (server *Server) Devices() ([]Device, error) {
	response, err := server.list("/api/v1/federation/remoteplay/devices")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var envelope struct {
		Data []Device `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("réponse illisible : %w", err)
	}

	return envelope.Data, nil
}

// Listing — le listing de ce serveur, en UN document (chantier 15) : ce qu'il
// possède, section par section, et dans son en-tête ce que notre jeton nous
// y ouvre (`Scopes`). Un TRANSFERT et non une liste : celui d'une grande
// bibliothèque pèse plusieurs mégaoctets.
func (server *Server) Listing() (listing.Listing, error) {
	response, err := server.get("/api/v1/federation/listing")
	if err != nil {
		return listing.Listing{}, err
	}
	defer response.Body.Close()

	var envelope struct {
		Data listing.Listing `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return listing.Listing{}, fmt.Errorf("listing illisible : %w", err)
	}
	if envelope.Data.Header.Version == 0 {
		// Une réponse 200 sans en-tête n'est pas un listing : un portail
		// captif, ou la page d'accueil de quelqu'un.
		return listing.Listing{}, errors.New("ce serveur n'a pas rendu de listing")
	}

	return envelope.Data, nil
}

// Freshness — de quoi savoir si la copie d'un ami a vieilli, sans la
// re-télécharger (décision 12) : la date de son listing, et ce que notre jeton
// nous ouvre chez lui — une portée accordée ou retirée ne change pas la date.
type Freshness struct {
	UpdatedAt time.Time `json:"updatedAt"`
	Scopes    []string  `json:"scopes"`
}

// ListingUpdated — sa Freshness. Une LISTE, bornée court : un ami éteint ne
// retient pas l'affichage.
func (server *Server) ListingUpdated() (Freshness, error) {
	response, err := server.list("/api/v1/federation/listing/updated")
	if err != nil {
		return Freshness{}, err
	}
	defer response.Body.Close()

	var envelope struct {
		Data Freshness `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return Freshness{}, fmt.Errorf("réponse illisible : %w", err)
	}

	return envelope.Data, nil
}

// LentParams — ce qu'un ami prête d'un émulateur (chantier 15) : ses valeurs
// choisies, ou un de ses préréglages perso. Déclaré ici plutôt qu'importé :
// ce paquet parle le protocole, pas le format des paramètres.
type LentParams struct {
	Name   string         `json:"name"`
	System string         `json:"system"`
	Values map[string]any `json:"values"`
}

// ErrNotLent — ce serveur ne prête pas (ou plus) ce qu'on lui demande : il
// répond 404. Ni un refus de notre jeton, ni une panne.
var ErrNotLent = errors.New("ce serveur ne prête pas cela")

// Params — ce que ce serveur prête d'un émulateur : ses valeurs (`preset`
// vide), ou ce préréglage perso.
func (server *Server) Params(consoleID, emulator, preset string) (LentParams, error) {
	path := "/api/v1/federation/params/" + url.PathEscape(consoleID) + "/" + url.PathEscape(emulator)
	if preset != "" {
		path += "/presets/" + url.PathEscape(preset)
	}
	response, err := server.list(path)
	if err != nil {
		if notFound(err) {
			return LentParams{}, ErrNotLent
		}

		return LentParams{}, err
	}
	defer response.Body.Close()

	var envelope struct {
		Data LentParams `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return LentParams{}, fmt.Errorf("réponse illisible : %w", err)
	}

	return envelope.Data, nil
}

// OpenPatch rend un correctif (chantier 15) : un dossier arrive ZIPPÉ, comme
// un jeu-dossier. L'appelant ferme.
func (server *Server) OpenPatch(consoleID, name string) (io.ReadCloser, error) {
	response, err := server.get(
		"/api/v1/federation/patches/" + url.PathEscape(consoleID) + "/" + url.PathEscape(name),
	)
	if err != nil {
		if notFound(err) {
			return nil, ErrNotLent
		}

		return nil, err
	}

	return response.Body, nil
}

// OpenRom rend le contenu d'un jeu. L'appelant ferme.
//
// Attention à ce que ça peut être : un jeu en plusieurs fichiers arrive
// ZIPPÉ (le serveur l'assemble à la volée). C'est à celui qui range de
// décider quoi en faire — ici on ne fait que transporter.
func (server *Server) OpenRom(consoleID, reference string) (io.ReadCloser, error) {
	response, err := server.get(
		"/api/v1/federation/roms/" + url.PathEscape(consoleID) + "/" + url.PathEscape(reference),
	)
	if err != nil {
		return nil, err
	}

	return response.Body, nil
}

// StreamRom ouvre un jeu pour le RELAYER : la réponse entière, en-têtes
// compris — sa taille et le nom de son fichier voyagent avec lui jusqu'au
// navigateur qui le télécharge (chantier 14). L'appelant ferme le corps.
//
// `ctx` borne le transfert, et lui seul : un navigateur qui abandonne coupe
// aussi la lecture chez le pair. Un jeu en plusieurs fichiers arrive zippé,
// comme pour OpenRom.
func (server *Server) StreamRom(ctx context.Context, consoleID, reference string) (*http.Response, error) {
	return server.doContext(ctx,
		"/api/v1/federation/roms/"+url.PathEscape(consoleID)+"/"+url.PathEscape(reference),
		server.relay,
	)
}

// OpenCore rend le fichier d'un cœur. L'appelant ferme.
//
// `name` est l'identifiant du registre (« fceumm »), pas un nom de fichier :
// c'est le serveur d'en face qui ajoute `_libretro.so`, comme partout ailleurs
// dans ce dépôt. Passer « fceumm_libretro.so » ferait chercher un suffixe
// doublé — l'erreur a coûté une journée le 2026-09-17, voir
// docs/TROUBLESHOOTING.md.
func (server *Server) OpenCore(consoleID, name string) (io.ReadCloser, error) {
	response, err := server.get(
		"/api/v1/federation/cores/" + url.PathEscape(consoleID) + "/" + url.PathEscape(name),
	)
	if err != nil {
		return nil, err
	}

	return response.Body, nil
}

// OpenEmulatorFile rend un FICHIER D'ÉMULATEUR DE BUREAU du pair (chantier 29) :
// l'archive de l'éditeur, sa construction, ou un paquet du téléphone, désigné
// par son nom tel que son listing l'annonce. L'appelant ferme.
func (server *Server) OpenEmulatorFile(consoleID, file string) (io.ReadCloser, error) {
	response, err := server.get(
		"/api/v1/federation/bundles/" + url.PathEscape(consoleID) + "/" + url.PathEscape(file),
	)
	if err != nil {
		return nil, err
	}

	return response.Body, nil
}

// OpenAndroidCore rend la VARIANTE ANDROID d'un cœur (`<nom>_libretro_android.so`,
// arm64), celle que les téléphones chargent (chantier 13, phase 5). Même route
// que OpenCore, `?platform=android` : c'est encore le serveur d'en face qui
// choisit le fichier. L'appelant ferme.
func (server *Server) OpenAndroidCore(consoleID, name string) (io.ReadCloser, error) {
	response, err := server.get(
		"/api/v1/federation/cores/" + url.PathEscape(consoleID) + "/" + url.PathEscape(name) + "?platform=android",
	)
	if err != nil {
		return nil, err
	}

	return response.Body, nil
}

// HasAndroidCore — ce serveur a-t-il la variante Android de ce cœur ? Une
// question COURTE (le client des listes, en HEAD) : un serveur injoignable ne
// doit pas retenir un lancement le temps d'un transfert.
func (server *Server) HasAndroidCore(consoleID, name string) bool {
	request, err := http.NewRequest(http.MethodHead,
		server.link+"/api/v1/federation/cores/"+url.PathEscape(consoleID)+"/"+url.PathEscape(name)+"?platform=android", nil)
	if err != nil {
		return false
	}
	request.Header.Set("Authorization", "Bearer "+server.token)
	response, err := server.lister.Do(request)
	if err != nil {
		return false
	}
	response.Body.Close()

	return response.StatusCode == http.StatusOK
}

// OpenBios rend le contenu d'un BIOS. Même remarque sur le zip : un dump en
// dossier (un .bin et ses fichiers compagnons) arrive zippé.
func (server *Server) OpenBios(consoleID, name string) (io.ReadCloser, error) {
	response, err := server.get(
		"/api/v1/federation/bios/" + url.PathEscape(consoleID) + "/" + url.PathEscape(name),
	)
	if err != nil {
		return nil, err
	}

	return response.Body, nil
}
