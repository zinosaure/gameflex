// Surface fédérée — chantier 6, revue aux chantiers 15 et 20.
//
// Ce que voit quelqu'un qui n'a PAS de compte ici, et qui présente un jeton
// de catalogue : le LISTING de ce serveur (ses jeux, ses BIOS, ses cœurs, ses
// correctifs, ses paramètres), sa date, et les fichiers qu'il annonce. Pas de
// session, pas de sauvegarde, pas de stockage, pas d'administration. C'est le
// LIEN entre deux serveurs, « comme avant » (le porteur, 2026-10-21).
//
// À PART, une seule porte de plus (chantier 20) : un membre de l'autre serveur
// qui a un COMPTE ici y joue sous ce compte. Son serveur échange le jeton de ce
// compte — que le membre a recopié de son profil d'ici et collé là-bas, à la
// place de l'identifiant et du mot de passe d'autrefois — contre une session à
// son nom (handleFederationSession). Le jeton de catalogue n'y ouvre rien.
//
// Pourquoi une famille de routes séparée plutôt qu'un middleware de plus sur
// /api/v1/retrogaming : parce que la liste de ce qui est ouvert doit se lire
// d'un coup d'oeil. Un jeton greffé sur le routeur existant s'ouvrirait à
// tout ce qu'on y ajouterait ensuite, sans que personne ne l'ait décidé —
// c'est exactement comme ça que /api/v1/remoteplay s'est retrouvé sans
// authentification du tout pendant des mois.
//
// Le fichier lui-même est servi par les MÊMES fonctions que le chemin
// authentifié (retro_files) : une seule façon de résoudre un jeu, donc pas
// de second résolveur qui divergerait — y compris sur le cas du jeu-dossier
// zippé à la volée.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/listing"
)

// federationIdentity — ce qu'un client apprend du serveur avant même
// d'afficher quoi que ce soit : comment il s'appelle, sous quel nom il nous
// connaît, et ce que son jeton lui ouvre ici. Ce dernier point dit à
// l'ami s'il peut lancer nos jeux chez nous (« devices », chantier 15).
type federationIdentity struct {
	Hostname string   `json:"hostname"`
	Identity string   `json:"identity"`
	Scopes   []string `json:"scopes"`
}

// tokenLabelKey — l'identité du porteur, posée sur la requête par le
// middleware pour que les handlers n'aient pas à re-vérifier.
type tokenLabelKey struct{}

// tokenScopesKey — ce que le porteur a le droit de faire.
type tokenScopesKey struct{}

// scopeCatalog — lire le catalogue et télécharger un jeu ou un BIOS. La
// seule portée qui existe, et le service d'identité n'en accorde pas
// d'autre.
//
// Elle est VÉRIFIÉE ici plutôt que déduite du fait qu'aucune autre route ne
// soit montée sous ce préfixe. La différence compte le jour où quelqu'un
// ajoutera une route à ce routeur sans se demander à qui il l'ouvre : sans
// portée exigée, elle serait ouverte à tous les porteurs par le seul fait
// d'avoir été écrite là.
const scopeCatalog = "catalog"

// scopeDevices — voir la LISTE des consoles à distance de ce serveur
// (chantier 9, C9-P2). Séparée de scopeCatalog exprès : la greffer sur elle
// aurait élargi, sans que personne ne le décide, tous les jetons déjà
// distribués. C'est le cas que le commentaire de requireScope annonce.
//
// Elle ouvre une liste, pas une console : lancer une session dessus demande
// un COMPTE chez celui qui la tient.
const scopeDevices = "devices"

// verifiedToken — ce que le service d'identité rend d'un jeton valide.
type verifiedToken struct {
	Label  string
	Scopes string
}

// verifyFederationToken demande au service d'identité si ce jeton existe.
//
// Aucun cache, volontairement : une révocation doit mordre tout de suite.
// Le coût est un aller-retour local par requête, contre un téléchargement
// de plusieurs mégaoctets — il ne se mesure pas.
//
// SEUL UN 401 EST UN REFUS (chantier 20). Toute autre réponse du service
// d'identité est une panne, et se dit comme telle : l'autre serveur lisait
// sinon un refus de son jeton à la moindre panne d'ici, et effaçait sa copie
// de notre catalogue (remote.Copies.failed).
func (server *Server) verifyFederationToken(token string) (verifiedToken, error) {
	if server.config.CredentialURL == "" || server.config.InternalSecret == "" {
		return verifiedToken{}, errors.New("federation is not configured")
	}

	response, err := server.identityRelay("/internal/federation/verify", token)
	if err != nil {
		return verifiedToken{}, err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return verifiedToken{}, errFederationDenied
	default:
		return verifiedToken{}, errors.New("identity service answered " + response.Status)
	}

	var decoded struct {
		Item struct {
			Label  string `json:"label"`
			Scopes string `json:"scopes"`
		} `json:"item"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return verifiedToken{}, err
	}
	return verifiedToken{Label: decoded.Item.Label, Scopes: decoded.Item.Scopes}, nil
}

// identityRelay — porter un jeton à une route interne du service d'identité.
func (server *Server) identityRelay(path, token string) (*http.Response, error) {
	return server.identityPost(path, map[string]string{"token": token})
}

// identityPost — un corps JSON à une route interne du service d'identité.
func (server *Server) identityPost(path string, body any) (*http.Response, error) {
	payload, _ := json.Marshal(body)
	request, err := http.NewRequest(http.MethodPost, server.config.CredentialURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)

	return (&http.Client{Timeout: 5 * time.Second}).Do(request)
}

var errFederationDenied = errors.New("invalid catalogue token")

// federationCORS — cette surface est faite pour être appelée D'AILLEURS.
//
// C'est toute sa raison d'être : le client est le GameFlex de quelqu'un
// d'autre, donc une autre origine. Sans en-têtes CORS, un navigateur (ou la
// WebView de l'application) refuse la réponse avant même de la lire, et
// l'erreur qu'il rapporte ne parle jamais du jeton — on cherche du mauvais
// côté pendant une heure.
//
// Origine libre et AUCUN cookie : l'autorisation tient entièrement dans le
// jeton porté par l'en-tête Authorization. Rien ici ne dépend de l'identité
// ambiante du navigateur, donc rien à protéger contre une requête forgée par
// un site tiers — il n'obtiendrait que ce qu'il a déjà, à savoir un jeton
// qu'il faudrait lui avoir donné.
func federationCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		// Le pré-vol n'a pas de jeton à présenter : il demande la permission
		// de poser la question, pas la réponse. Le laisser traverser
		// l'authentification le ferait échouer en 401, et le navigateur
		// n'enverrait jamais la vraie requête.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// federationAuth — le jeton arrive comme un Bearer, comme partout ailleurs.
// Il n'est PAS un JWT et ne doit jamais être confondu avec un : c'est
// pourquoi il ne passe pas par auth.Middleware.
func (server *Server) federationAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := auth.BearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing catalogue token")
			return
		}

		verified, err := server.verifyFederationToken(token)
		if errors.Is(err, errFederationDenied) {
			writeError(w, http.StatusUnauthorized, "invalid catalogue token")
			return
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "cannot verify catalogue token: "+err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), tokenLabelKey{}, verified.Label)
		ctx = context.WithValue(ctx, tokenScopesKey{}, verified.Scopes)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleFederationIdentity — GET /api/v1/federation/identity.
// Appelée par le formulaire d'ajout de serveur : elle confirme que le jeton
// est bon ET rapporte le nom du serveur, pour qu'une faute de frappe se voie
// tout de suite au lieu d'attendre le premier scan.
func (server *Server) handleFederationIdentity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, federationIdentity{
		Hostname: server.federationHostname(),
		Identity: tokenLabelOf(r),
		Scopes:   scopesOf(r),
	})
}

// handleFederationListing — GET /api/v1/federation/listing (chantier 15).
//
// LE LISTING DE CE SERVEUR, EN UN DOCUMENT : ce que l'ami copie chez lui à
// notre ajout, et re-télécharge quand sa date avance. Il remplace les trois
// listes construites à la volée (`/catalog`, `/storage/cores`,
// `/storage/bios`), supprimées. Dans l'en-tête, ce que SON jeton lui ouvre
// ici : c'est par là qu'il sait s'il peut lancer nos jeux chez nous.
func (server *Server) handleFederationListing(w http.ResponseWriter, r *http.Request) {
	server.ensureSelf()
	served, err := listing.Load(listingCacheDir, listing.SelfKey)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "the listing of this server is not built yet")

		return
	}
	served.Header.Scopes = scopesOf(r)
	// Les disques internes avec la bibliothèque, comme une seule (chantier 23).
	server.withInternalDisks(r.Context(), &served)

	writeJSON(w, http.StatusOK, served)
}

// handleFederationListingUpdated — GET /api/v1/federation/listing/updated : la
// date du listing (décision 12) — de quoi savoir s'il a changé sans le
// re-télécharger —, et ce que le jeton du porteur lui ouvre ici : une portée
// accordée ou retirée ne change pas la date.
func (server *Server) handleFederationListingUpdated(w http.ResponseWriter, r *http.Request) {
	server.ensureSelf()
	header, err := listing.LoadHeader(listingCacheDir, listing.SelfKey)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "the listing of this server is not built yet")

		return
	}

	updatedAt := server.disksUpdatedAt(r.Context(), header.UpdatedAt)

	writeJSON(w, http.StatusOK, map[string]any{"updatedAt": updatedAt, "scopes": scopesOf(r)})
}

// federationSession — une session ouverte ici par le jeton d'un compte.
type federationSession struct {
	Token       string    `json:"token"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Consoles    []string  `json:"consoles"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// handleFederationSession — POST /api/v1/federation/session (chantier 20).
//
// UN MEMBRE D'UN AUTRE SERVEUR JOUE ICI, SOUS SON COMPTE D'ICI : son serveur
// présente le jeton de ce compte — celui que le membre a recopié de son profil
// d'ici et collé là-bas — et reçoit en échange une session à son nom. Ses pages
// lancent ensuite avec elle comme avec n'importe quelle connexion : les mêmes
// routes, les mêmes refus par console. C'est l'ancien formulaire de connexion,
// sans mot de passe : « un token et c'est bon » (le porteur).
//
// La session n'est jamais administrateur, ne dure pas plus que le jeton, et
// meurt avec lui : le service d'identité la signe avec la génération du jeton,
// que la porte des comptes vérifie (internal/accounts). Un compte sans console
// en reçoit une aussi : il ne lance rien, et la page le dit.
//
// Le jeton de CATALOGUE n'ouvre rien ici : il n'a pas de compte derrière lui.
func (server *Server) handleFederationSession(w http.ResponseWriter, r *http.Request) {
	token := auth.BearerToken(r)
	if token == "" || server.config.CredentialURL == "" || server.config.InternalSecret == "" {
		writeError(w, http.StatusUnauthorized, "missing account token")

		return
	}
	response, err := server.identityRelay("/internal/federation/session", token)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot open a session: "+err.Error())

		return
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		writeError(w, http.StatusUnauthorized, "invalid account token")

		return
	default:
		writeError(w, http.StatusServiceUnavailable, "cannot open a session: identity service answered "+response.Status)

		return
	}

	var decoded struct {
		Item struct {
			AccessToken string `json:"access_token"`
			User        struct {
				Username    string   `json:"username"`
				DisplayName string   `json:"display_name"`
				Consoles    []string `json:"consoles"`
			} `json:"user"`
		} `json:"item"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot open a session: "+err.Error())

		return
	}
	// La session se relit avec NOTRE secret avant de partir : une session que
	// ce kernel refuserait ensuite ne doit pas sortir d'ici.
	claims, err := auth.ParseToken(server.config.JWTSecret, decoded.Item.AccessToken)
	if err != nil || claims.ExpiresAt == nil || claims.Fed == "" {
		writeError(w, http.StatusServiceUnavailable, "the identity service returned an unusable session")

		return
	}
	if decoded.Item.User.Consoles == nil {
		decoded.Item.User.Consoles = []string{}
	}

	writeJSON(w, http.StatusOK, federationSession{
		Token:       decoded.Item.AccessToken,
		Username:    decoded.Item.User.Username,
		DisplayName: decoded.Item.User.DisplayName,
		Consoles:    decoded.Item.User.Consoles,
		ExpiresAt:   claims.ExpiresAt.Time.UTC(),
	})
}

// handleFederationCore — GET /api/v1/federation/cores/{consoleId}/{name}.
//
// LE PENDANT MANQUANT DE /storage/cores, qui ne faisait que LISTER. Sans cette
// route, un pair pouvait dire « j'ai fceumm » sans qu'on puisse jamais le
// rapatrier — la liste sans le fichier (#154).
//
// Le nom est vérifié CONTRE LE REGISTRE, comme partout ailleurs : c'est lui qui
// devient un chemin sur le disque, et un nom arbitraire ne doit jamais pouvoir
// en sortir.
func (server *Server) handleFederationCore(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	name := chi.URLParam(r, "name")
	if !coreBelongsTo(consoleID, name) {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("le core « %s » n'est pas déclaré pour %s", name, consoleID))

		return
	}

	// LA VARIANTE DEMANDÉE (chantier 13, phase 5) : `?platform=android` rend
	// le fichier que les téléphones chargent, rangé à côté de la version Linux.
	file := retro_files.CoreFileName(name)
	if r.URL.Query().Get("platform") == "android" {
		file = retro_files.AndroidCoreFileName(name)
	}
	// La bibliothèque, puis les disques internes (chantier 24) : pour un ami,
	// ils ne font qu'un.
	for _, roots := range server.libraryRootsInOrder(r.Context()) {
		path := filepath.Join(roots.Cores, consoleID, file)
		if _, err := os.Stat(path); err == nil {
			http.ServeFile(w, r, path)

			return
		}
	}
	writeError(w, http.StatusNotFound, "core not found")
}

// handleFederationBundle — GET /api/v1/federation/bundles/{consoleId}/{file}.
//
// UN FICHIER D'ÉMULATEUR DE BUREAU, que le listing annonce : un paquet du
// téléphone pour un téléphone ami (chantier 28, #447), l'archive de l'éditeur ou
// notre construction pour un serveur ami (chantier 29) —
// `cores/<console>/<fichier>`. Le nom passe par sa règle
// (`retro_files.IsEmulatorFile`, qui exige une console à émulateur de bureau) :
// rien d'autre ne se lit par cette route.
func (server *Server) handleFederationBundle(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	file := chi.URLParam(r, "file")
	if !retro_files.IsEmulatorFile(consoleID, file) {
		writeError(w, http.StatusBadRequest, "paquet inconnu")

		return
	}
	for _, roots := range server.libraryRootsInOrder(r.Context()) {
		path := filepath.Join(roots.Cores, consoleID, file)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)

			return
		}
	}
	writeError(w, http.StatusNotFound, "bundle not found")
}

// handleFederationRom — GET/HEAD /api/v1/federation/roms/{consoleId}/{ref}.
// Délègue mot pour mot au chemin authentifié : même résolution, même zip à
// la volée pour un jeu-dossier.
func (server *Server) handleFederationRom(w http.ResponseWriter, r *http.Request) {
	server.handleRetroServeRom(w, r)
}

// handleFederationBios — GET /api/v1/federation/bios/{consoleId}/{name}.
//
// La bibliothèque, puis les disques internes (chantier 24).
func (server *Server) handleFederationBios(w http.ResponseWriter, r *http.Request) {
	var path string
	var isDir bool
	err := os.ErrNotExist
	for _, roots := range server.libraryRootsInOrder(r.Context()) {
		path, isDir, err = retro_files.ResolveBiosFile(roots.Bios, chi.URLParam(r, "consoleId"), chi.URLParam(r, "name"))
		if !errors.Is(err, os.ErrNotExist) {
			break
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "bios not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid bios reference: "+err.Error())
		return
	}

	// Un BIOS peut être un DOSSIER (un dump plus ses fichiers compagnons —
	// .bin + .NVM + .MEC). Le client a besoin des trois, donc il part zippé,
	// exactement comme un jeu multi-fichiers.
	if isDir {
		serveDirectoryAsZip(w, r, path, chi.URLParam(r, "name"))
		return
	}
	http.ServeFile(w, r, path)
}

// federationHostname — le nom SOUS LEQUEL ce serveur s'annonce aux autres.
//
// Surtout pas os.Hostname() : dans un conteneur il rend l'identifiant de
// l'image ("bd056dc1fa11"), qui ne veut rien dire pour la personne qui lira
// « Super Mario Bros — bd056dc1fa11 » sous un jeu. On préfère un nom choisi
// (GAMEFLEX_SERVER_NAME), et à défaut l'adresse par laquelle on est déjà
// joignable, qui est au moins reconnaissable.
func (server *Server) federationHostname() string {
	if name := os.Getenv("GAMEFLEX_SERVER_NAME"); name != "" {
		return name
	}
	if server.config.PublicHost != "" {
		return server.config.PublicHost
	}
	return "gameflex"
}

// tokenLabelOf — sous quel nom ce porteur est connu. Vide est impossible en
// pratique (le middleware refuse avant), mais un handler ne doit pas en
// dépendre pour être correct.
func tokenLabelOf(r *http.Request) string {
	label, _ := r.Context().Value(tokenLabelKey{}).(string)
	return label
}

// scopesOf — ce que le jeton du porteur lui ouvre ici, une portée par entrée.
func scopesOf(r *http.Request) []string {
	granted, _ := r.Context().Value(tokenScopesKey{}).(string)
	scopes := []string{}
	for _, one := range strings.Split(granted, ",") {
		if one = strings.TrimSpace(one); one != "" {
			scopes = append(scopes, one)
		}
	}

	return scopes
}

// requireScope — une route ne s'ouvre qu'à un porteur qui a la portée
// correspondante. Les portées voyagent en clair séparées par des virgules ;
// on compare des segments entiers, pas des sous-chaînes, sinon "catalogue"
// contiendrait "catalog".
func requireScope(scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		granted, _ := r.Context().Value(tokenScopesKey{}).(string)
		for _, one := range strings.Split(granted, ",") {
			if strings.TrimSpace(one) == scope {
				next(w, r)
				return
			}
		}
		writeError(w, http.StatusForbidden, "this token does not grant "+scope)
	}
}

// federationRouter — la liste complète de ce qu'un jeton ouvre : elle tient
// en quelques lignes, et c'est le but.
func (server *Server) federationRouter() http.Handler {
	router := chi.NewRouter()
	router.Use(federationCORS)

	// LE JETON D'UN COMPTE, À PART (chantier 20) : il ne passe pas par le
	// jeton de catalogue, et le jeton de catalogue ne passe pas par lui. Un
	// appel de serveur à serveur, pas d'un navigateur.
	router.Post("/session", server.handleFederationSession)
	// UNE INVITATION, À PART AUSSI (chantier 33) : son code tient lieu de
	// jeton, une seule fois (federation_invitations.go).
	router.Post("/redeem", server.handleFederationRedeem)

	router.Group(func(router chi.Router) {
		router.Use(server.federationAuth)
		server.catalogRoutes(router)
	})

	return router
}

// catalogRoutes — ce qu'un jeton de catalogue ouvre, portée par portée.
func (server *Server) catalogRoutes(router chi.Router) {

	// /identity ne demande aucune portée : elle sert à VÉRIFIER un jeton au
	// moment de l'ajouter, avant même de savoir ce qu'il ouvre. Elle ne
	// révèle que le nom du serveur, le nom du porteur et ce que son jeton
	// lui ouvre — ce que ce dernier sait déjà, puisqu'il le tient en main.
	router.Options("/*", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	router.Get("/identity", server.handleFederationIdentity)

	// LE LISTING DE CE SERVEUR, et sa date (chantier 15) : de la
	// bibliothèque, pas de la machine — la portée du catalogue.
	router.Get("/listing", requireScope(scopeCatalog, server.handleFederationListing))
	router.Get("/listing/updated", requireScope(scopeCatalog, server.handleFederationListingUpdated))
	router.Get("/roms/{consoleId}/{ref}", requireScope(scopeCatalog, server.handleFederationRom))
	router.Head("/roms/{consoleId}/{ref}", requireScope(scopeCatalog, server.handleFederationRom))
	router.Get("/bios/{consoleId}/{name}", requireScope(scopeCatalog, server.handleFederationBios))
	router.Head("/bios/{consoleId}/{name}", requireScope(scopeCatalog, server.handleFederationBios))

	// Et le FICHIER d'un cœur, pas seulement sa ligne dans le listing : les
	// BIOS et les ROMs avaient déjà le leur ; les cœurs n'en avaient pas
	// (#154).
	router.Get("/cores/{consoleId}/{name}", requireScope(scopeCatalog, server.handleFederationCore))
	router.Head("/cores/{consoleId}/{name}", requireScope(scopeCatalog, server.handleFederationCore))
	router.Get("/bundles/{consoleId}/{file}", requireScope(scopeCatalog, server.handleFederationBundle))
	router.Head("/bundles/{consoleId}/{file}", requireScope(scopeCatalog, server.handleFederationBundle))

	// Ce que ses paramètres prêtent, et ses correctifs (chantier 15) : de la
	// bibliothèque, comme le reste.
	router.Get("/params/{consoleId}/{emulator}", requireScope(scopeCatalog, server.handleFederationParams))
	router.Get("/params/{consoleId}/{emulator}/presets/{preset}", requireScope(scopeCatalog, server.handleFederationParams))
	router.Get("/patches/{consoleId}/{name}", requireScope(scopeCatalog, server.handleFederationPatch))
	router.Head("/patches/{consoleId}/{name}", requireScope(scopeCatalog, server.handleFederationPatch))

	// La LISTE des consoles à distance, et rien d'autre (chantier 9, C9-P2).
	// Sa portée est distincte : un jeton de catalogue ne la voit pas.
	router.Get("/remoteplay/devices", requireScope(scopeDevices, server.handleFederationDevices))

	// TOUJOURS PAS DE LANCEMENT, ni streamex ni remoteplay, et ce n'est pas
	// un oubli. Lancer une session démarre un conteneur sur CETTE machine ;
	// remoteplay va jusqu'à une console physique dans CE salon. Prêter sa
	// bibliothèque n'est pas prêter sa machine — voir le chantier 6, C6-P9.
	//
	// Ce que le chantier 9 ajoute est la LISTE, pas le droit d'y toucher :
	// pour jouer sur la console de quelqu'un, on se connecte chez lui avec
	// un compte — depuis le chantier 20, le jeton de ce compte, échangé par
	// /session —, et c'est son service qui décide.
}
