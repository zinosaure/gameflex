package api

// Jetons de catalogue — chantier 6.
//
// Un jeton donne à quelqu'un qui n'a pas de compte ici le droit de LIRE le
// catalogue et de télécharger un jeu ou un BIOS. Il n'ouvre ni session, ni
// sauvegarde, ni administration.
//
// Pourquoi ce service et pas le kernel : le kernel possède les fichiers,
// celui-ci possède l'identité et la base. Un jeton EST une identité — la
// plus pauvre qui soit, un nom et rien d'autre — donc il vit ici, et le
// kernel vient le faire vérifier par le plan interne. Une seule copie.
//
// Le secret ne sort qu'UNE fois, à la création. Ensuite la base n'en a que
// le SHA-256 : rien ne peut le ré-afficher, pas même l'administrateur qui
// l'a créé. C'est la contrepartie de ne pas avoir de compte en face — si le
// jeton se perd, on en crée un autre et on révoque l'ancien.
//
// C'EST LE LIEN, ET RIEN QUE LE LIEN (le porteur, 2026-10-21 : « le lien entre
// 2 serveurs reste comme avant »). Jouer chez nous depuis l'autre serveur ne
// passe jamais par lui : un membre d'en face qui a un compte ici colle le
// jeton de ce compte (account_tokens.go), et c'est son compte qui décide.

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"gameflex/consoles"
	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
)

// tokenPrefixLength — de quoi reconnaître un jeton dans une liste sans
// jamais le redonner en entier.
const tokenPrefixLength = 8

// ScopeCatalog — lire le catalogue, télécharger un jeu ou un BIOS.
//
// Il n'y a toujours pas de portée "play". Lancer une session démarre un
// conteneur sur la machine de celui qui prête — mémoire, bloc de ports ICE,
// port alloué — et remoteplay touche une console physique chez lui. Prêter sa
// bibliothèque et prêter sa machine sont deux décisions différentes ; les
// confondre dans un même jeton, c'est faire prendre la seconde à quelqu'un
// qui croyait prendre la première.
//
// Ce commentaire posait aussi la condition à laquelle "play" devrait
// répondre : QUI joue. Le chantier 9 y répond sans ce jeton — on se CONNECTE
// au serveur d'en face avec un compte de chez lui, et il sait alors exactement
// qui est là ; depuis le chantier 20, avec le jeton de ce compte plutôt
// qu'un mot de passe (account_tokens.go). Un jeton de CATALOGUE n'aura donc
// jamais à porter cette question.
const ScopeCatalog = "catalog"

// LES CONSOLES D'UN ACCÈS (chantier 66, `#795`) remplacent la portée `devices`
// du chantier 9, qui ne disait que « voir les consoles Remote Play ». Elles
// vivent dans `federation_tokens.consoles`, au format des consoles d'un compte
// (gameflex/consoles) : ce que les membres venus par cet accès peuvent lancer,
// en plus de leurs propres droits. Les consoles Remote Play se voient avec le
// catalogue ; y jouer suit cette liste.

// ScopeMetadataProviders — PARTAGER MES FOURNISSEURS DE MÉTADONNÉES (chantier
// 57, `#719` ; le porteur, 2026-11-15 : « En plus de partager les jeux et
// consoles on propose de partager les fournisseurs metadata. Comme ça si je
// partage à moi-même il n'y a pas de risques d'abus de quota »). Avec elle, la
// recherche des fiches de ce serveur interroge aussi ses comptes (IGDB,
// SteamGridDB) pour le porteur du jeton, pour un titre qu'il n'a pas ; ses
// clés ne sortent jamais. Sans elle, il ne prête que les fiches de ses jeux.
// Coupée par défaut, et séparée : aucun jeton déjà distribué ne s'élargit sans
// qu'on le décide.
const ScopeMetadataProviders = "metadata-providers"

// grantableScopes — ce qu'un administrateur peut accorder. Une portée
// inconnue est REFUSÉE plutôt qu'ignorée : un jeton créé avec une faute de
// frappe ouvrirait moins que ce que son créateur croit, et rien ne le lui
// dirait avant que l'autre ne se plaigne.
var grantableScopes = map[string]bool{
	ScopeCatalog:           true,
	ScopeMetadataProviders: true,
}

// parseScopes — la liste demandée, nettoyée, dédoublonnée et remise dans un
// ordre stable. LE CATALOGUE EST TOUJOURS ACCORDÉ (le porteur, 2026-11-16 :
// « par défaut les jeux sont toujours autorisé ») : un accès délivré ouvre
// toujours les jeux ; les consoles et les fournisseurs de métadonnées s'y
// ajoutent ou non. Cela remplace « les consoles se demandent seules » (un
// jeton `devices` sans le catalogue n'existe plus). Vide veut donc dire
// ScopeCatalog, comme avant.
func parseScopes(asked []string) (string, error) {
	granted := []string{}
	seen := map[string]bool{}

	for _, one := range asked {
		scope := strings.TrimSpace(one)
		if scope == "" {
			continue
		}
		if !grantableScopes[scope] {
			return "", fmt.Errorf("unknown scope %q", scope)
		}
		if !seen[scope] {
			seen[scope] = true
			granted = append(granted, scope)
		}
	}

	if !seen[ScopeCatalog] {
		granted = append(granted, ScopeCatalog)
	}
	sort.Strings(granted)

	return strings.Join(granted, ","), nil
}

type federationTokenResponse struct {
	ID     int64  `json:"id"`
	Label  string `json:"label"`
	Prefix string `json:"prefix"`
	Scopes string `json:"scopes"`
	// Consoles — ce que cet accès laisse lancer (chantier 66), au format des
	// consoles d'un compte : « * », « retro », `ps5`, `rp-ps5`…
	Consoles []string `json:"consoles"`
	// RemoteAdmin — la session qu'un administrateur d'ici ouvre par cet accès garde son rang (chantier 73) : coupé par
	// défaut, accordé accès par accès — le porteur, 2026-11-20 : « je ne veux pas que ma smart TV puisse avoir la
	// possibilité de faire du remote admin ».
	RemoteAdmin bool `json:"remote_admin"`
	// RelayAllowed — le serveur qui a cet accès peut repasser nos jeux à ses appareils (chantier 83) ; RelayedServers —
	// les serveurs fédérés d'ici qu'on relaie au porteur de cet accès, un par un.
	RelayAllowed   bool    `json:"relay_allowed"`
	RelayedServers []int64 `json:"relayed_servers"`
	CreatedAt      any     `json:"created_at"`
	LastUsedAt  any  `json:"last_used_at"`
	// Token n'est rempli qu'à la création, jamais dans une liste.
	Token string `json:"token,omitempty"`
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// handleCreateFederationToken — POST /api/v1/federation/tokens {label}.
func (server *Server) handleCreateFederationToken(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		Label       string   `json:"label"`
		Scopes      []string `json:"scopes"`
		RemoteAdmin bool     `json:"remote_admin"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	// L'ADMINISTRATION À DISTANCE (chantier 73) ne s'accorde jamais depuis une session venue d'ailleurs.
	if body.RemoteAdmin && refuseFederated(w, principal) {
		return
	}

	// Le nom est obligatoire, et ce n'est pas de la coquetterie : c'est ce
	// qui permet de révoquer UN accès plutôt que tous, et ce qui s'affiche
	// à côté des jeux qu'il fait apparaître.
	label := strings.TrimSpace(body.Label)
	if label == "" {
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "label"}, "msg": "A name is required.", "type": "value_error.missing",
		}})
		return
	}

	// Les portées se choisissent depuis le 2026-09-07, parce qu'il y en a
	// enfin deux. Le commentaire d'avant refusait ce paramètre, et il avait
	// raison tant qu'une seule valeur existait — un choix à une option
	// n'invite qu'à en inventer une seconde sans la peser.
	scopes, err := parseScopes(body.Scopes)
	if err != nil {
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "scopes"}, "msg": err.Error(), "type": "value_error",
		}})
		return
	}

	secret := config.RandomToken(32)
	result, err := server.db.Exec(
		`INSERT INTO federation_tokens (label, scopes, remote_admin, token_hash, prefix, created_at, created_by_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		label, scopes, body.RemoteAdmin, hashToken(secret), secret[:tokenPrefixLength], now(), principal.ID,
	)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	id, _ := result.LastInsertId()
	// Un accès de plus dans la copie des kernels (chantier 66).
	server.accountsChanged()

	respondItem(w, http.StatusCreated, federationTokenResponse{
		ID:          id,
		Label:       label,
		Scopes:      scopes,
		RemoteAdmin: body.RemoteAdmin,
		Consoles:    []string{consoles.GrantAll},
		Prefix:      secret[:tokenPrefixLength],
		// La seule et unique fois où il sort d'ici.
		Token: secret,
	})
}

// handleListFederationTokens — GET /api/v1/federation/tokens.
func (server *Server) handleListFederationTokens(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	rows, err := server.db.Query(
		`SELECT id, label, scopes, consoles, prefix, created_at, last_used_at, remote_admin, relay_allowed, relayed_servers FROM federation_tokens ORDER BY id DESC`,
	)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	defer rows.Close()

	items := []federationTokenResponse{}
	for rows.Next() {
		var item federationTokenResponse
		var createdAt, lastUsedAt sql.NullString
		var granted, relayed string
		if err := rows.Scan(&item.ID, &item.Label, &item.Scopes, &granted, &item.Prefix, &createdAt, &lastUsedAt, &item.RemoteAdmin, &item.RelayAllowed, &relayed); err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		item.CreatedAt = isoDatetime(createdAt)
		item.LastUsedAt = isoDatetime(lastUsedAt)
		item.Consoles = consoles.SplitGrants(granted)
		item.RelayedServers = parseRelayed(relayed)
		items = append(items, item)
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

// handleUpdateFederationToken — PATCH /api/v1/catalog-tokens/{id} {scopes?, consoles?, remote_admin?}.
// L'administration à distance (chantier 73) ne se change jamais depuis une session venue d'ailleurs : un accès ne
// s'accorde pas lui-même ce rang, ni ne l'accorde à un autre.
// Les consoles (chantier 66) se changent seules, comme celles d'un membre : chaque interrupteur part à son changement.
//
// Change CE QU'UN JETON OUVRE, sans toucher au secret. Les deux sont
// délibérément dissociés : le secret ne peut pas être réémis (la base n'en a
// que l'empreinte), mais ce qu'il ouvre est une décision qu'on doit pouvoir
// revoir.
//
// Sans cette route, élargir un accès obligeait à révoquer et recréer — donc à
// retourner coller un nouveau jeton sur chaque appareil qui l'avait. Cette
// friction pousse à tout accorder d'avance « au cas où », ce qui est
// exactement le contraire de ce que les portées séparées cherchent à obtenir.
//
// Elle ne donne aucun pouvoir nouveau : celui qui peut modifier ici est celui
// qui pouvait déjà révoquer et recréer.
func (server *Server) handleUpdateFederationToken(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		Scopes         *[]string `json:"scopes"`
		Consoles       *[]string `json:"consoles"`
		RemoteAdmin    *bool     `json:"remote_admin"`
		RelayAllowed   *bool     `json:"relay_allowed"`
		RelayedServers *[]int64  `json:"relayed_servers"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if (body.RemoteAdmin != nil || body.RelayAllowed != nil || body.RelayedServers != nil) && refuseFederated(w, principal) {
		return
	}
	answer := federationTokenResponse{}
	if body.Scopes != nil {
		scopes, err := parseScopes(*body.Scopes)
		if err != nil {
			respondValidationError(w, []map[string]any{{
				"loc": []string{"body", "scopes"}, "msg": err.Error(), "type": "value_error",
			}})
			return
		}
		if !server.updateFederationToken(w, r, `UPDATE federation_tokens SET scopes = ? WHERE id = ?`, scopes) {
			return
		}
		answer.Scopes = scopes
	}
	if body.Consoles != nil {
		granted, err := consoles.ParseGrants(*body.Consoles)
		if err != nil {
			respondValidationError(w, []map[string]any{{
				"loc": []string{"body", "consoles"}, "msg": err.Error(), "type": "value_error",
			}})
			return
		}
		if !server.updateFederationToken(w, r, `UPDATE federation_tokens SET consoles = ? WHERE id = ?`, granted) {
			return
		}
		answer.Consoles = consoles.SplitGrants(granted)
		// Les kernels décident des lancements sur leur copie : elle change tout de suite.
		server.accountsChanged()
	}
	if body.RemoteAdmin != nil {
		allowed := "0"
		if *body.RemoteAdmin {
			allowed = "1"
		}
		if !server.updateFederationToken(w, r, `UPDATE federation_tokens SET remote_admin = ? WHERE id = ?`, allowed) {
			return
		}
		answer.RemoteAdmin = *body.RemoteAdmin
		slog.Info("remote administration", "username", principal.Username, "access", r.PathValue("id"), "allowed", *body.RemoteAdmin)
		// Le rang des sessions de cet accès change tout de suite, au kernel aussi.
		server.accountsChanged()
	}
	if body.RelayAllowed != nil {
		allowed := "0"
		if *body.RelayAllowed {
			allowed = "1"
		}
		if !server.updateFederationToken(w, r, `UPDATE federation_tokens SET relay_allowed = ? WHERE id = ?`, allowed) {
			return
		}
		answer.RelayAllowed = *body.RelayAllowed
		slog.Info("federation relay allowed", "username", principal.Username, "access", r.PathValue("id"), "allowed", *body.RelayAllowed)
	}
	if body.RelayedServers != nil {
		if !server.updateFederationToken(w, r, `UPDATE federation_tokens SET relayed_servers = ? WHERE id = ?`, joinRelayed(*body.RelayedServers)) {
			return
		}
		answer.RelayedServers = parseRelayed(joinRelayed(*body.RelayedServers))
		slog.Info("federation relayed servers", "username", principal.Username, "access", r.PathValue("id"), "servers", answer.RelayedServers)
	}

	respondItem(w, http.StatusOK, answer)
}

// updateFederationToken — une colonne d'un jeton ; false, la réponse d'erreur est partie.
func (server *Server) updateFederationToken(w http.ResponseWriter, r *http.Request, statement, value string) bool {
	result, err := server.db.Exec(statement, value, r.PathValue("id"))
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return false
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondErrorCode(w, http.StatusNotFound, "not_found", "Token not found.")
		return false
	}

	return true
}

// handleRevokeFederationToken — DELETE /api/v1/federation/tokens/{id}.
// La ligne part : un jeton révoqué n'a pas d'histoire à garder, et le
// garder en "revoked" laisserait croire qu'on peut le rallumer.
func (server *Server) handleRevokeFederationToken(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	result, err := server.db.Exec(`DELETE FROM federation_tokens WHERE id = ?`, r.PathValue("id"))
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondErrorCode(w, http.StatusNotFound, "not_found", "Token not found.")
		return
	}
	// L'accès quitte la copie des kernels (chantier 66) : ses sessions ne lancent plus rien.
	server.accountsChanged()
	w.WriteHeader(http.StatusNoContent)
}

// internalCaller — la requête vient-elle du kernel ? Une route `/internal/…`
// ne sert que lui, avec le secret partagé (`X-Internal-Secret`) ; sinon elle
// répond elle-même le refus, et l'appelant s'arrête.
//
// UNE SEULE VÉRIFICATION, pour toutes ces routes (2026-10-21). Elle était
// recopiée dans chacune, et la troisième, `/internal/storage/mounts`, l'avait
// oubliée : elle rendait les mots de passe des partages distants à quiconque
// joignait le port publié du service (10203, 11203), sans rien demander.
//
// Un secret interne vide ne doit JAMAIS ouvrir la porte — sans ça, un
// déploiement qui oublie la variable transforme la vérification en tampon
// automatique.
func (server *Server) internalCaller(w http.ResponseWriter, r *http.Request) bool {
	if server.config.InternalSecret == "" {
		respondErrorCode(w, http.StatusServiceUnavailable, "server_failed", "Internal secret not configured.")

		return false
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Secret")), []byte(server.config.InternalSecret)) != 1 {
		respondErrorCode(w, http.StatusUnauthorized, "server_failed", "Invalid internal secret.")

		return false
	}

	return true
}

// handleVerifyFederationToken — POST /internal/federation/verify {token}.
//
// Appelée par le kernel, jamais par un navigateur. Répond l'identité
// derrière le jeton, ou 401. C'est ici que last_used_at est touché : le
// kernel n'a pas à savoir qu'on tient ce compteur.
func (server *Server) handleVerifyFederationToken(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	item, err := server.verifyFederationToken(body.Token)
	if errors.Is(err, ErrCatalogTokenRefused) {
		respondErrorCode(w, http.StatusUnauthorized, "auth_required", "Invalid token.")
		return
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	respondItem(w, http.StatusOK, item)
}

// ErrCatalogTokenRefused — ce jeton de catalogue n'existe pas, ou plus (révoqué).
var ErrCatalogTokenRefused = errors.New("catalog token refused")

// verifyFederationToken — l'identité derrière un jeton de catalogue, ou ErrCatalogTokenRefused. UNE SEULE RÈGLE pour
// les deux hôtes : la route interne la sert au kernel du serveur, le téléphone l'appelle en process (credential/embed),
// lui qui n'a pas de secret interne. Elle touche last_used_at : le kernel n'a pas à savoir qu'on tient ce compteur.
func (server *Server) verifyFederationToken(token string) (federationTokenResponse, error) {
	var item federationTokenResponse
	var createdAt, lastUsedAt sql.NullString
	var granted, relayed string
	err := server.db.QueryRow(
		`SELECT id, label, scopes, consoles, prefix, created_at, last_used_at, remote_admin, relay_allowed, relayed_servers FROM federation_tokens WHERE token_hash = ?`,
		hashToken(token),
	).Scan(&item.ID, &item.Label, &item.Scopes, &granted, &item.Prefix, &createdAt, &lastUsedAt, &item.RemoteAdmin, &item.RelayAllowed, &relayed)
	if errors.Is(err, sql.ErrNoRows) || token == "" {
		return federationTokenResponse{}, ErrCatalogTokenRefused
	}
	if err != nil {
		return federationTokenResponse{}, err
	}

	// Best-effort : savoir qu'un jeton sert encore est utile, mais ça ne
	// vaut pas de refuser un téléchargement parce que l'écriture a échoué.
	server.db.Exec(`UPDATE federation_tokens SET last_used_at = ? WHERE id = ?`, now(), item.ID)

	item.CreatedAt = isoDatetime(createdAt)
	item.LastUsedAt = isoDatetime(lastUsedAt)
	item.Consoles = consoles.SplitGrants(granted)
	item.RelayedServers = parseRelayed(relayed)

	return item, nil
}

// VerifyCatalogToken — le nom et les portées derrière un jeton de catalogue (verifyFederationToken), pour un hôte qui
// embarque ce service (le téléphone).
func (server *Server) VerifyCatalogToken(token string) (label, scopes string, err error) {
	item, err := server.verifyFederationToken(token)

	return item.Label, item.Scopes, err
}

// VerifyCatalogAccess — de même, avec ce que cet accès laisse lancer (chantier 66).
func (server *Server) VerifyCatalogAccess(token string) (label, scopes string, granted []string, err error) {
	item, err := server.verifyFederationToken(token)

	return item.Label, item.Scopes, item.Consoles, err
}

// CatalogRelay — le relais de la fédération par cet accès (chantier 83), pour un hôte qui embarque ce service : son
// porteur peut-il nous relayer, et les serveurs fédérés qu'on lui relaie.
func (server *Server) CatalogRelay(token string) (allowed bool, relayed []int64, err error) {
	item, err := server.verifyFederationToken(token)

	return item.RelayAllowed, item.RelayedServers, err
}

// DisabledConsoles — les consoles coupées pour tout le serveur (chantier 66), pour un hôte qui embarque ce service.
func (server *Server) DisabledConsoles() ([]string, error) {
	return server.disabledConsoles()
}

// ── L'autre sens : les serveurs auxquels NOUS avons accès ────────────────
//
// Un jeton reçu de quelqu'un d'autre, et l'adresse à laquelle le présenter.
//
// Ce service les GARDE ; c'est le kernel qui les VÉRIFIE (il possède le
// client fédéré, et le module partagé qui va avec). D'où la forme du
// parcours côté interface : sonder d'abord — « serveur trouvé : nas-du-salon,
// vous y êtes connu comme Marc » — puis enregistrer. Deux appels, mais on ne
// range jamais une adresse dont on ne sait pas si elle répond.

type federationServerResponse struct {
	ID        int64  `json:"id"`
	Link      string `json:"link"`
	Hostname  string `json:"hostname"`
	Identity  string `json:"identity"`
	CreatedAt any    `json:"created_at"`
	// Token ne sort JAMAIS vers un navigateur — seulement vers le kernel,
	// par le plan interne, qui en a besoin pour aller chercher le catalogue.
	Token string `json:"token,omitempty"`
}

// handleAddFederationServer — POST /api/v1/server-registry.
func (server *Server) handleAddFederationServer(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		Link     string `json:"link"`
		Token    string `json:"token"`
		Hostname string `json:"hostname"`
		Identity string `json:"identity"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	link := strings.TrimRight(strings.TrimSpace(body.Link), "/")
	token := strings.TrimSpace(body.Token)
	if link == "" || token == "" {
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "link"}, "msg": "A link and a token are required.", "type": "value_error.missing",
		}})
		return
	}

	// hostname vient de la sonde du kernel. Vide, on retombe sur l'adresse :
	// une ligne sans nom dans la liste serait pire qu'une adresse brute.
	hostname := strings.TrimSpace(body.Hostname)
	if hostname == "" {
		hostname = link
	}

	result, err := server.db.Exec(
		`INSERT INTO federation_servers (link, token, hostname, identity, created_at, created_by_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		link, token, hostname, strings.TrimSpace(body.Identity), now(), principal.ID,
	)
	if err != nil {
		// L'unicité porte sur le lien : ajouter deux fois le même serveur
		// donnerait deux fois ses jeux dans la liste.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			respondErrorCode(w, http.StatusConflict, "alreadyRegistered", "This server is already in the list.")
			return
		}
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	id, _ := result.LastInsertId()

	respondItem(w, http.StatusCreated, federationServerResponse{
		ID: id, Link: link, Hostname: hostname, Identity: strings.TrimSpace(body.Identity),
	})
}

func (server *Server) scanServers(rows *sql.Rows, withToken bool) ([]federationServerResponse, error) {
	defer rows.Close()

	items := []federationServerResponse{}
	for rows.Next() {
		var item federationServerResponse
		var identity, createdAt sql.NullString
		var token string
		if err := rows.Scan(&item.ID, &item.Link, &token, &item.Hostname, &identity, &createdAt); err != nil {
			return nil, err
		}
		item.Identity = identity.String
		item.CreatedAt = isoDatetime(createdAt)
		if withToken {
			item.Token = token
		}
		items = append(items, item)
	}

	return items, nil
}

// handleListFederationServers — GET /api/v1/server-registry.
func (server *Server) handleListFederationServers(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	rows, err := server.db.Query(
		`SELECT id, link, token, hostname, identity, created_at FROM federation_servers ORDER BY id`)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	items, err := server.scanServers(rows, false)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

// handleRemoveFederationServer — DELETE /api/v1/server-registry/{id}.
func (server *Server) handleRemoveFederationServer(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	result, err := server.db.Exec(`DELETE FROM federation_servers WHERE id = ?`, r.PathValue("id"))
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondErrorCode(w, http.StatusNotFound, "not_found", "Server not found.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleInternalFederationServers — GET /internal/federation/servers.
//
// La seule réponse qui contient les jetons. Elle ne sort pas de la machine :
// c'est le kernel qui la lit, pour aller chercher les catalogues.
func (server *Server) handleInternalFederationServers(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}

	rows, err := server.db.Query(
		`SELECT id, link, token, hostname, identity, created_at FROM federation_servers ORDER BY id`)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	items, err := server.scanServers(rows, true)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

// parseRelayed — les serveurs relayés d'un accès, tels qu'ils sont rangés (« 3,7 ») ; jamais nul : une liste vide dit
// « aucun ».
func parseRelayed(stored string) []int64 {
	ids := []int64{}
	for _, part := range strings.Split(stored, ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}

	return ids
}

// joinRelayed — la forme rangée d'une liste de serveurs relayés : les identifiants valides, sans doublon.
func joinRelayed(ids []int64) string {
	seen := map[int64]bool{}
	parts := []string{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			parts = append(parts, strconv.FormatInt(id, 10))
		}
	}

	return strings.Join(parts, ",")
}

