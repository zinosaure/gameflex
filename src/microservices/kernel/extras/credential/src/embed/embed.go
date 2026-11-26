// Package embed fait tourner le service d'identité DANS un autre programme.
//
// Il existe pour le téléphone. L'application doit offrir la même chose que le
// web — favoris, historique, mappages de manette, préférences — et ces
// surfaces vivent toutes ici, derrière un compte. Le téléphone n'en a qu'un,
// root, qui se connecte tout seul ; c'est la SEULE différence, et elle tient
// dans une table à une ligne.
//
// Pourquoi un paquet exporté plutôt qu'un import direct : le kernel mobile est
// un AUTRE module Go, et Go interdit d'importer `internal/` depuis l'extérieur
// du module qui le contient. Cette porte-ci est dans le bon module, elle a
// donc le droit — et elle n'expose qu'une fonction.
//
// Pourquoi pas une réimplémentation : il y a une trentaine de routes, et deux
// copies d'une même règle finissent toujours par ne plus dire la même chose.
// C'est le défaut que ce dépôt passe son temps à retirer. Le téléphone fait
// tourner LE MÊME code, donc le même comportement par construction.
package embed

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"gameflex/credential/internal/api"
	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/database"
	"gameflex/credential/internal/presence"
	"gameflex/credential/internal/web_push"
)

// Options — ce qu'un hôte doit fournir. Tout le reste a une valeur par défaut
// sensée : l'appelant n'est pas un conteneur, il n'a pas d'environnement à
// lire.
type Options struct {
	// DBPath est le fichier SQLite. Sur le téléphone il vit à côté de la
	// bibliothèque de jeux, pas dans le privé de l'application : c'est la
	// même décision que pour les ROMs et les sauvegardes — ce qui appartient
	// à quelqu'un doit rester visible et sauvegardable par lui.
	DBPath string
	// JWTSecret DOIT être celui de l'hôte : les jetons qu'il frappe déjà
	// (le jeton d'appareil) et ceux que ce service émet doivent se vérifier
	// mutuellement, sinon la page se connecte et la façade la rejette.
	JWTSecret string
	// RootPassword — celui du compte unique. Vide : un mot de passe
	// aléatoire est tiré, ce qui convient à un appareil dont le porteur ne
	// se connecte jamais à la main.
	RootPassword string
	// TokenLifetime — la durée de vie des jetons. Zéro garde celle du
	// serveur (24 h). Un téléphone la rallonge : ses invités rouvrent une
	// page des jours plus tard, et retaper un mot de passe pour rejoindre la
	// partie du salon n'a aucun sens.
	TokenLifetime time.Duration
	// KeysDir — le dossier des clés (chantier 80, la double authentification), comme `keys/` d'un serveur : à la racine
	// du stockage du téléphone, à côté de `databases/`. Vide : `keys/` à côté du dossier de la base.
	KeysDir string
	// Server — le service tourne dans un SERVEUR tout en un (le PC, l'AppImage du chantier 70), pas dans un téléphone :
	// les règles d'un serveur s'y appliquent (le porteur, 2026-11-24 : « tous la plupart des paramètres du serveur
	// [doivent être] dans l'AppImage aussi par exemple pour le MFA ») — la double authentification imposée à tous.
	Server bool
}

// Service — le service d'identité embarqué.
type Service struct {
	Handler http.Handler
	DB      *sql.DB
	api     *api.Server
	secret  string
	// Username du compte unique, pour que l'hôte sache au nom de qui frapper
	// le jeton d'ouverture automatique.
	Username string
}

// New ouvre la base, s'assure qu'un compte root existe, et rend le service
// prêt à être monté.
func New(options Options) (*Service, error) {
	if options.DBPath == "" {
		return nil, fmt.Errorf("embed: DBPath is required")
	}
	if options.JWTSecret == "" {
		return nil, fmt.Errorf("embed: JWTSecret is required")
	}

	// Le chemin est GLOBAL au paquet database : on le pose avant d'ouvrir.
	// C'est la raison pour laquelle DBPath est une variable (voir config).
	config.DBPath = options.DBPath
	config.KeysDir = options.KeysDir
	if config.KeysDir == "" {
		config.KeysDir = filepath.Join(filepath.Dir(filepath.Dir(options.DBPath)), "keys")
	}

	db, err := database.Open()
	if err != nil {
		return nil, fmt.Errorf("embed: open database: %w", err)
	}

	cfg := config.Config{
		JWTSecret:         options.JWTSecret,
		RootUsername:      "root",
		RootPassword:      options.RootPassword,
		VAPIDContactEmail: "admin@example.com",
		CommitSHA:         "device",
		Device:            !options.Server,
	}

	if options.TokenLifetime > 0 {
		auth.TokenLifetime = options.TokenLifetime
	}

	if err := seedRoot(db, cfg); err != nil {
		db.Close()

		return nil, err
	}

	// L'INSCRIPTION LIBRE EST PARTIE (chantier 55) : un administrateur crée le
	// compte et la personne choisit son mot de passe par son lien, comme sur un
	// serveur. Son interrupteur ne reste pas en base. La table peut ne pas
	// exister : rien à purger alors.
	_, _ = db.Exec(`DELETE FROM settings WHERE key = 'registration_open'`)

	// Révoquer le mot de passe PUBLIÉ des versions précédentes s'il traîne
	// encore ici : la faille se ferme au démarrage suivant, sans geste du
	// porteur (voir revokePublishedDefault). Sur une base fournie avec un
	// RootPassword explicite, il n'y a rien à révoquer.
	if options.RootPassword == "" {
		if err := revokePublishedDefault(db); err != nil {
			db.Close()

			return nil, err
		}
	}

	// Une clé VAPID PROPRE À CET APPAREIL, tirée au sort une fois et rangée
	// avec le reste.
	//
	// Le client de poussée web refuse une clé vide, et il a raison : une clé
	// invalide qui ne se signale qu'à l'envoi est pire que rien. Mais un
	// téléphone n'a pas d'administrateur pour lui en fournir une, et ces
	// notifications-là ne sortent pas de l'appareil — il s'en frappe donc une.
	privateKey, err := database.StoredVAPIDKey(db)
	if err != nil {
		db.Close()

		return nil, err
	}
	pushClient, err := web_push.NewClient(privateKey, cfg.VAPIDContactEmail)
	if err != nil {
		db.Close()

		return nil, fmt.Errorf("embed: web_push: %w", err)
	}

	// LES DEUX MODES DU CODE D'APPAIRAGE (chantier 60), et les jetons de catalogue : le téléphone partage comme un
	// serveur (`#756` ; le porteur, 2026-11-17 : « le téléphone fait tout ce que fait un serveur »).
	server := api.NewServer(cfg, db, presence.NewRegistry(), pushClient)

	return &Service{
		Handler:  server,
		DB:       db,
		api:      server,
		secret:   cfg.JWTSecret,
		Username: cfg.RootUsername,
	}, nil
}

// TokenForOwner — un jeton d'identité pour le compte du propriétaire (voir
// ownerAccount).
//
// Frappé PAR LE SERVICE et non par l'hôte, pour une raison qu'on a apprise à
// nos dépens : ce service lit la claim `sub` (l'identifiant de la ligne) et va
// chercher l'utilisateur en base. Un jeton signé à côté, qui ne porte que le
// nom, se fait rejeter par /api/v1/auth/me — et la page retombe sur un écran de
// connexion, sur un appareil qui n'a personne à qui demander un mot de passe
// (2026-09-01).
//
// Les deux claims sont donc remplies par le même code que côté serveur : ce
// jeton est un vrai jeton de compte, et le nom qu'il porte est celui de la
// base.
func (service *Service) TokenForOwner() (string, error) {
	var (
		id   int64
		name string
		role int
	)
	if err := service.DB.QueryRow(ownerAccount).Scan(&id, &name, &role); err != nil {
		return "", fmt.Errorf("embed: no owner account: %w", err)
	}

	// À la génération de ses sessions (chantier 55), comme tout jeton du service.
	return service.api.SessionToken(id)
}

// ownerAccount — le compte du propriétaire de l'appareil : SON compte
// administrateur, le premier créé.
//
// Comme sur un serveur : à la première utilisation, `root` et son mot de passe
// aléatoire ; avec lui on crée le compte administrateur, et c'est celui-là
// qu'on utilise ensuite. Sur un téléphone, cet administrateur EST le
// propriétaire. Tant qu'il n'existe pas, c'est `root`. En créer d'autres reste
// possible, comme sur un serveur — c'est un usage, pas une règle (précisé par
// le porteur le 2026-10-09, après qu'une règle « root seul administrateur »
// avait rétrogradé son propre compte).
const ownerAccount = `SELECT id, username, role_level FROM users
	WHERE role_level <= 1
	ORDER BY CASE role_level WHEN 1 THEN 0 ELSE 1 END, id
	LIMIT 1`

// FederationServer — un serveur enregistré, JETON COMPRIS.
//
// Le jeton ne sort jamais par l'API d'administration : c'est délibéré, il
// ouvre le catalogue de quelqu'un d'autre. Mais il faut bien que quelque
// chose l'utilise, et sur le téléphone ce quelque chose est le kernel, dans
// le même processus.
type FederationServer struct {
	ID       int64  `json:"id"`
	Link     string `json:"link"`
	Token    string `json:"token"`
	Hostname string `json:"hostname"`
	Identity string `json:"identity"`
	// RelayedBy — un serveur qu'un ami nous relaie (chantier 83) : le nom de cet ami ; vide pour un pair direct. Posé
	// par l'hôte, jamais rangé dans la base.
	RelayedBy string `json:"relayedBy,omitempty"`
}

// FederationServers rend les serveurs enregistrés. Une base sans
// la table rend une liste vide plutôt qu'une erreur.
func (service *Service) FederationServers() ([]FederationServer, error) {
	rows, err := service.DB.Query(
		`SELECT id, link, token, hostname, identity FROM federation_servers ORDER BY id`,
	)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	servers := []FederationServer{}
	for rows.Next() {
		var server FederationServer
		if err := rows.Scan(
			&server.ID, &server.Link, &server.Token, &server.Hostname, &server.Identity,
		); err != nil {
			return nil, fmt.Errorf("embed: read federation servers: %w", err)
		}
		servers = append(servers, server)
	}

	return servers, rows.Err()
}

// MetadataAccounts — les comptes des services de métadonnées (IGDB, SteamGridDB), EN CLAIR, pour le kernel du
// téléphone (chantier 57, P4) : son fournisseur à lui, pour ses jeux. Saisis par la page d'administration
// (`/api/v1/metadata-services`, que ce service monte), scellés en base par le secret de l'appareil, ils ne ressortent
// jamais par l'API — seulement ici, dans le même processus, comme les jetons des serveurs fédérés.
func (service *Service) MetadataAccounts() (map[string]map[string]string, error) {
	return service.api.MetadataAccounts()
}

// MetadataWriteOnGameDisk — l'interrupteur « Écrire les métadonnées sur le disque du jeu » (allumé par défaut) : coupé,
// celles d'un jeu d'un disque interne vivent dans la bibliothèque du téléphone (un disque NTFS y est en lecture seule).
func (service *Service) MetadataWriteOnGameDisk() (bool, error) {
	return service.api.MetadataWriteOnGameDisk()
}

// SetDisplayName donne au compte unique le nom SOUS LEQUEL ON LE VOIT.
//
// « Root » est le nom d'un compte système, pas de quelqu'un. Dans une partie
// à plusieurs, tout le monde s'appelait donc Root et on ne pouvait plus dire
// qui demandait la manette (2026-09-01). Le nom de l'appareil, lui, distingue
// — « Samsung de Marc » se reconnaît sans explication.
//
// Le nom de CONNEXION ne bouge pas : c'est `root` qui ouvre la session, et le
// changer invaliderait les jetons déjà signés.
//
// Idempotent, et silencieux si le nom est déjà le bon : cette fonction est
// appelée à chaque démarrage, pas seulement à la création du compte — un
// téléphone renommé doit se renommer ici aussi.
func (service *Service) SetDisplayName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	_, err := service.DB.Exec(
		`UPDATE users SET display_name = ? WHERE role_level = 0 AND display_name IS NOT ?`,
		name, name,
	)
	if err != nil {
		return fmt.Errorf("embed: set display name: %w", err)
	}

	return nil
}

// revokePublishedDefault ferme une faille des versions d'avant le 2026-09-12.
//
// Jusque-là, une installation neuve posait sur root un mot de passe PUBLIÉ et
// identique sur tous les appareils ("GameFlex"), marqué en base par
// `root_password_seeded`. Le kernel écoute sur toutes les interfaces pour ses
// invités du réseau local (voir serve) : quiconque sur ce réseau pouvait donc
// se connecter en root et obtenir le rôle d'administrateur. Le raisonnement
// d'origine — « un réseau domestique, un téléphone qu'on tient en main » — ne
// tenait pas : le lien invité se partage, et un mot de passe connu de tous
// n'est pas un mot de passe.
//
// La valeur a disparu du code ; reste à la RÉVOQUER là où elle a déjà été
// posée. Si le marqueur est présent, on remplace le mot de passe par un secret
// aléatoire que PERSONNE ne connaît, et on efface le marqueur — une seule
// fois. Le porteur obtient alors un mot de passe connu de lui seul par la
// console (« Mot de passe de root »), qui en tire un neuf et l'affiche sur
// l'écran qu'il tient déjà en main.
//
// Une installation neuve n'a pas ce marqueur : root y est déjà né avec un
// secret aléatoire (voir seedRoot), et cette fonction n'a rien à faire — les
// deux chemins se rejoignent, root a un mot de passe que seul le porteur peut
// se donner.
func revokePublishedDefault(db *sql.DB) error {
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`,
	); err != nil {
		return fmt.Errorf("embed: settings table: %w", err)
	}
	var marker string
	if err := db.QueryRow(
		`SELECT value FROM settings WHERE key = 'root_password_seeded'`,
	).Scan(&marker); err != nil || marker != "1" {
		// Jamais posé le défaut publié : rien à révoquer.
		return nil
	}

	scrambled, err := auth.HashPassword(config.RandomToken(24))
	if err != nil {
		return fmt.Errorf("embed: scramble published default: %w", err)
	}
	if _, err := db.Exec(
		`UPDATE users SET hashed_password = ? WHERE role_level = 0`, scrambled,
	); err != nil {
		return fmt.Errorf("embed: revoke published default: %w", err)
	}
	if _, err := db.Exec(
		`DELETE FROM settings WHERE key = 'root_password_seeded'`,
	); err != nil {
		return fmt.Errorf("embed: clear default marker: %w", err)
	}
	slog.Warn("embedded identity: revoked the published default root password — " +
		"the holder sets a new one from the device console")

	return nil
}

// ResetRootPassword TIRE un mot de passe neuf pour le compte root, et le rend.
//
// ALÉATOIRE, et non plus une valeur publiée. Jusqu'au 2026-09-12 il remettait
// la constante "GameFlex" — connue de tous, donc une porte ouverte sur le
// réseau local. Le raisonnement d'origine (« un secret tiré au sort serait
// introuvable ») ne tient pas ici : la console AFFICHE celui-ci, en grand et
// copiable, sur l'écran de celui qui tient déjà le téléphone. Il ne repasse
// jamais par le réseau.
//
// POURQUOI ÇA EXISTE : c'est ainsi que le porteur obtient un mot de passe
// utilisable. Une installation neuve crée root avec un secret aléatoire que
// personne ne connaît (seedRoot) ; un appui ici en tire un nouveau, connu de
// lui, avec lequel il ouvre l'interface et crée les administrateurs — comme
// sur le serveur, où root existe et où l'on crée les comptes depuis là.
func (service *Service) ResetRootPassword() (string, error) {
	password := config.RandomToken(18)
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return "", fmt.Errorf("embed: hash root password: %w", err)
	}
	var rootID int64
	if err := service.DB.QueryRow(`SELECT id FROM users WHERE role_level = 0 ORDER BY id LIMIT 1`).Scan(&rootID); err != nil {
		return "", fmt.Errorf("embed: no root account: %w", err)
	}
	if _, err := service.DB.Exec(
		`UPDATE users SET hashed_password = ?, password_set_at = ? WHERE id = ?`,
		hashed, time.Now().Format("2006-01-02 15:04:05.999999"), rootID,
	); err != nil {
		return "", fmt.Errorf("embed: reset root password: %w", err)
	}
	// Qui avait l'ancien mot de passe perd aussi ses sessions (chantier 55) : la
	// génération avance, et le kernel du téléphone ferme ses canaux.
	if err := service.api.AdvanceSessions(rootID); err != nil {
		return "", fmt.Errorf("embed: end root sessions: %w", err)
	}
	slog.Info("embedded identity: root password reset from the device console")

	return password, nil
}

// Owner — le compte du propriétaire (voir ownerAccount) : son nom, et son
// niveau de rôle.
//
// Existe pour que la console de l'appareil puisse RÉPONDRE à « pourquoi je
// n'ai pas les écrans d'administration ». Une interface qui masque ne dit
// jamais pourquoi elle masque ; la console lit à la source.
func (service *Service) Owner() (string, int, error) {
	var (
		id   int64
		name string
		role int
	)
	if err := service.DB.QueryRow(ownerAccount).Scan(&id, &name, &role); err != nil {
		return "", 0, fmt.Errorf("embed: no account: %w", err)
	}

	return name, role, nil
}

// Accounts — le nom du compte root, et combien la base compte
// d'administrateurs et de joueurs (root à part).
//
// Pour la page « Compte root » de la console de l'appareil (le porteur,
// 2026-11-17) : elle ne montre plus UN compte administrateur — lequel, quand
// il y en a plusieurs ? —, mais root et ces deux nombres ; à zéro
// administrateur, elle dit de commencer par root pour en créer un.
func (service *Service) Accounts() (root string, admins, players int, err error) {
	if err := service.DB.QueryRow(
		`SELECT username FROM users WHERE role_level = 0 ORDER BY id LIMIT 1`,
	).Scan(&root); err != nil {
		return "", 0, 0, fmt.Errorf("embed: no root account: %w", err)
	}
	if err := service.DB.QueryRow(
		`SELECT COUNT(CASE WHEN role_level = 1 THEN 1 END), COUNT(CASE WHEN role_level >= 2 THEN 1 END) FROM users`,
	).Scan(&admins, &players); err != nil {
		return "", 0, 0, fmt.Errorf("embed: count accounts: %w", err)
	}

	return root, admins, players, nil
}

// RoleOf rend le niveau de rôle que porte ce jeton, tel que LA BASE le dit.
//
// Le jeton porte bien un `role_level`, mais c'est une copie figée à l'émission
// — et l'autorité est la base (voir auth.go). Un compte rétrogradé garderait
// sinon ses droits jusqu'à l'expiration de son jeton, qui dure un an ici.
//
// Les règles sont celles du serveur (auth.LoadToken, chantier 20) : un compte
// supprimé n'a plus de rôle, et une session ouverte par le jeton d'un compte
// depuis un autre serveur suit ce jeton — jamais administrateur.
func (service *Service) RoleOf(token string) (int, error) {
	principal, err := auth.LoadToken(service.DB, service.secret, token)
	if err != nil {
		return 0, fmt.Errorf("embed: %w", err)
	}

	return principal.RoleLevel, nil
}

// ErrTokenRefused — ce jeton de compte ne vaut pas (ou plus) ici.
var ErrTokenRefused = api.ErrTokenRefused

// ErrAccessRefused — le serveur qui ouvre la session ne présente aucun accès délivré valable (chantier 66).
var ErrAccessRefused = api.ErrAccessRefused

// TokenAccount — le compte d'un jeton : son nom, comment il s'affiche, ce qu'il
// peut lancer (`*`, `retro`, des identifiants).
type TokenAccount struct {
	Username    string
	DisplayName string
	Consoles    []string
	ExpiresAt   time.Time
}

func tokenAccount(account api.TokenAccount) TokenAccount {
	return TokenAccount{
		Username: account.Username, DisplayName: account.DisplayName,
		Consoles: account.Grants, ExpiresAt: account.ExpiresAt,
	}
}

// OpenFederatedSession — une session au nom du compte de ce jeton, pour son
// propriétaire qui joue ici depuis son serveur : jamais administrateur, pas
// plus longue que le jeton. ErrTokenRefused s'il ne vaut pas.
// Caller — qui présente le jeton (#781, #787) : le nom, le compte et le genre qu'il déclare, l'adresse vue.
type Caller = api.TokenCaller

func (service *Service) OpenFederatedSession(token string, caller Caller) (string, TokenAccount, error) {
	session, account, err := service.api.FederatedSession(token, caller)
	if err != nil {
		return "", TokenAccount{}, err
	}

	return session, tokenAccount(account), nil
}

// Les refus d'une invitation (chantier 33) : inconnue, servie ou échue ; d'une
// autre sorte ; le jeton de son compte éteint. Aucun des deux derniers ne la
// consomme. ThrottledError : trop d'échecs depuis cette adresse.
var (
	ErrInvitationRefused  = api.ErrInvitationRefused
	ErrInvitationKind     = api.ErrInvitationKind
	ErrInvitationTokenOff = api.ErrInvitationTokenOff
)

type (
	ThrottledError = api.ThrottledError
	Redeemed       = api.Redeemed
)

// ErrPairingClosed — un code d'appairage présenté hors du mode de sa sorte
// (chantier 60) : refusé sans rien compter.
var ErrPairingClosed = api.ErrPairingClosed

// RedeemInvitation — un autre serveur échange une invitation de ce téléphone
// contre son jeton, une seule fois (invitations.go) ; `from` est son nom, que
// la fenêtre d'un code d'appairage montre.
func (service *Service) RedeemInvitation(code, kind, from, address string) (Redeemed, error) {
	return service.api.RedeemInvitation(code, kind, from, address)
}

// PairingFrame — les modes découverte et les codes vivants (chantier 60), comme
// la trame `pairing` du serveur les porte : jamais la valeur d'un code.
func (service *Service) PairingFrame() []byte {
	return service.api.PairingFrame()
}

// OnPairingChanged — `callback(frame)` après chaque changement d'un mode ou
// d'un code : le kernel du téléphone le pousse au sujet `pairing`.
func (service *Service) OnPairingChanged(callback func(frame []byte)) {
	service.api.OnPairingChanged(callback)
}

// ErrCatalogTokenRefused — un jeton de catalogue inconnu ou révoqué.
var ErrCatalogTokenRefused = api.ErrCatalogTokenRefused

// VerifyCatalogToken — le nom et les portées derrière un jeton de catalogue émis ici (« Accès délivrés »), ou
// ErrCatalogTokenRefused : la règle du serveur (`/internal/federation/verify`), sans le plan interne. Le kernel du
// téléphone garde sa surface fédérée avec (`#756`).
func (service *Service) VerifyCatalogToken(token string) (label, scopes string, err error) {
	return service.api.VerifyCatalogToken(token)
}

// VerifyCatalogAccess — de même, avec les consoles que cet accès laisse lancer (chantier 66) : le téléphone les dit à
// l'ami avec son listing.
func (service *Service) VerifyCatalogAccess(token string) (label, scopes string, consoles []string, err error) {
	return service.api.VerifyCatalogAccess(token)
}

// CatalogRelay — le relais de la fédération par cet accès (chantier 83) : son porteur peut-il nous relayer, et les
// serveurs fédérés qu'on lui relaie.
func (service *Service) CatalogRelay(token string) (allowed bool, relayed []int64, err error) {
	return service.api.CatalogRelay(token)
}

// DisabledConsoles — les consoles coupées pour tout l'appareil (chantier 66).
func (service *Service) DisabledConsoles() ([]string, error) {
	return service.api.DisabledConsoles()
}

// ErrNoFederatedAccount — ce membre n'a pas collé de jeton pour ce serveur.
var ErrNoFederatedAccount = api.ErrNoFederatedAccount

// FederatedAccountToken — le jeton qu'un membre de ce téléphone a collé pour un
// serveur fédéré (chantier 20) : celui de son compte là-bas.
func (service *Service) FederatedAccountToken(username string, serverID int64) (string, error) {
	return service.api.FederatedAccountToken(username, serverID)
}

// SetFederatedAccountToken — le garder, vérifié chez l'ami par le kernel.
func (service *Service) SetFederatedAccountToken(username string, serverID int64, token string) error {
	return service.api.SetFederatedAccountToken(username, serverID, token)
}

// ForgetFederatedAccountToken — l'oublier.
func (service *Service) ForgetFederatedAccountToken(username string, serverID int64) error {
	return service.api.ForgetFederatedAccountToken(username, serverID)
}

// OnAccountsChanged — `callback` après chaque changement d'un compte ou de
// son jeton : le kernel du téléphone coupe ce qui ne vaut plus.
func (service *Service) OnAccountsChanged(callback func()) {
	service.api.OnAccountsChanged(callback)
}

// seedRoot — le compte unique, créé une fois. Copie fidèle de ce que fait
// cmd/auth au démarrage : même table, même rôle, même idempotence.
func seedRoot(db *sql.DB, cfg config.Config) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE role_level = 0`).Scan(&count); err != nil {
		return fmt.Errorf("embed: root seed check: %w", err)
	}
	if count > 0 {
		return nil
	}

	password := cfg.RootPassword
	if password == "" {
		password = config.RandomToken(16)
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("embed: hash root password: %w", err)
	}

	created := time.Now().Format("2006-01-02 15:04:05.999999")
	if _, err := db.Exec(
		`INSERT INTO users (username, hashed_password, display_name, role_level, created_at, password_set_at)
		 VALUES (?, ?, ?, 0, ?, ?)`,
		cfg.RootUsername, hashed, "Root", created, created,
	); err != nil {
		return fmt.Errorf("embed: seed root: %w", err)
	}
	slog.Info("embedded identity: root account created", "username", cfg.RootUsername)

	return nil
}

// ── Le statut des consoles et des jeux (chantier 30, en base depuis le 2026-11-17) ──
//
// Les mêmes fonctions que les routes /internal/game-status du serveur (api/game_status.go), dans le même processus :
// le kernel du téléphone en fait son gamestatus.Store. La forme échangée est le JSON de gamestatus.Console et de
// gamestatus.Report, pour que ce module n'ait pas à connaître gameflex/storage.

// GameStatusConsole — une console et les signalements de ses jeux, en JSON.
func (service *Service) GameStatusConsole(consoleID string) ([]byte, error) {
	console, err := api.LoadGameStatus(service.DB, consoleID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(console)
}

// AddGameStatusReport — un signalement (JSON de gamestatus.Report).
func (service *Service) AddGameStatusReport(consoleID, gameID string, report []byte) error {
	var decoded api.GameStatusReport
	if err := json.Unmarshal(report, &decoded); err != nil {
		return err
	}

	return api.AddGameStatusReport(service.DB, consoleID, gameID, decoded)
}

// RemoveGameStatusReport — retire le signalement de cet instant.
func (service *Service) RemoveGameStatusReport(consoleID, gameID string, at time.Time) error {
	return api.RemoveGameStatusReport(service.DB, consoleID, gameID, at)
}

// SetGameStatusConsole — le statut de toute la console.
func (service *Service) SetGameStatusConsole(consoleID, status, by string, at time.Time) error {
	return api.SetGameStatusConsole(service.DB, consoleID, status, by, at)
}

// GameStatuses — le statut de chaque console réglée.
func (service *Service) GameStatuses() (map[string]string, error) {
	return api.GameStatuses(service.DB)
}
