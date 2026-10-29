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
	"fmt"
	"log/slog"
	"net/http"
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
	}

	if options.TokenLifetime > 0 {
		auth.TokenLifetime = options.TokenLifetime
	}

	if err := seedRoot(db, cfg); err != nil {
		db.Close()

		return nil, err
	}

	// Les identifiants de partage encore en clair, scellés avec le secret de
	// l'appareil (#459), comme sur le serveur.
	api.SealRemoteMounts(db, cfg.JWTSecret)

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

	return auth.CreateToken(service.secret, id, name, role)
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

// Mount — un partage enregistré, mot de passe COMPRIS.
//
// Sur le serveur, le kernel va chercher la même liste par une route interne
// (`/internal/storage/mounts`) parce qu'il vit dans un autre processus. Ici il
// n'y a qu'un processus : passer par HTTP pour se parler à soi-même
// ajouterait un port, un secret et une panne possible, sans rien garantir de
// plus.
type Mount struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	// Folder — le sous-dossier à parcourir. Vide = la racine du partage.
	Folder   string `json:"folder"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Mounts rend les partages enregistrés. Une base sans la table (une version
// antérieure du schéma) rend une liste vide, pas une erreur : un montage
// absent n'est pas une raison de refuser de lister la bibliothèque.
func (service *Service) Mounts() ([]Mount, error) {
	rows, err := service.DB.Query(
		`SELECT id, name, kind, base_url, folder, username, password FROM remote_mounts ORDER BY name`,
	)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	mounts := []Mount{}
	for rows.Next() {
		var mount Mount
		if err := rows.Scan(
			&mount.ID, &mount.Name, &mount.Kind, &mount.BaseURL, &mount.Folder,
			&mount.Username, &mount.Password,
		); err != nil {
			return nil, fmt.Errorf("embed: read mounts: %w", err)
		}
		mounts = append(mounts, mount)
	}

	return mounts, rows.Err()
}

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
}

// FederationServers rend les serveurs enregistrés. Comme Mounts, une base sans
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
	if _, err := service.DB.Exec(
		`UPDATE users SET hashed_password = ? WHERE role_level = 0`, hashed,
	); err != nil {
		return "", fmt.Errorf("embed: reset root password: %w", err)
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

// RegistrationOpen — l'appareil accepte-t-il de nouvelles inscriptions ?
//
// FERMÉ par défaut, et c'est un choix : l'administrateur doit ouvrir la porte
// pour qu'elle s'ouvre. Un réglage qui part permissif ne se découvre jamais —
// on ne cherche pas l'interrupteur d'une chose qui marche déjà, et on
// l'apprend le jour où quelqu'un s'est inscrit sans qu'on le sache. Ouvrir
// pour la soirée, refermer après, est le geste qu'on veut rendre conscient.
func (service *Service) RegistrationOpen() bool {
	if _, err := service.DB.Exec(
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`,
	); err != nil {
		return false
	}
	var stored string
	if err := service.DB.QueryRow(
		`SELECT value FROM settings WHERE key = 'registration_open'`,
	).Scan(&stored); err != nil {
		// Rien d'enregistré : fermé. Voir ci-dessus.
		return false
	}

	return stored == "1"
}

// SetRegistrationOpen ouvre ou ferme les inscriptions.
func (service *Service) SetRegistrationOpen(open bool) error {
	if _, err := service.DB.Exec(
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`,
	); err != nil {
		return fmt.Errorf("embed: settings table: %w", err)
	}
	value := "0"
	if open {
		value = "1"
	}
	if _, err := service.DB.Exec(
		`INSERT INTO settings (key, value) VALUES ('registration_open', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, value,
	); err != nil {
		return fmt.Errorf("embed: set registration: %w", err)
	}

	return nil
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
func (service *Service) OpenFederatedSession(token string) (string, TokenAccount, error) {
	session, account, err := service.api.FederatedSession(token)
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

// RedeemInvitation — un autre serveur échange une invitation de ce téléphone
// contre son jeton, une seule fois (invitations.go).
func (service *Service) RedeemInvitation(code, kind, address string) (Redeemed, error) {
	return service.api.RedeemInvitation(code, kind, address)
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

// Register inscrit un compte ORDINAIRE, choisi par la personne elle-même.
//
// POURQUOI L'INSCRIPTION LIBRE EXISTE ICI ET PAS SUR LE SERVEUR : sur un
// serveur, les comptes sont créés par un administrateur — il n'y a
// délibérément aucune route d'inscription. Un téléphone n'a pas
// d'administrateur sous la main quand trois personnes veulent jouer dans le
// salon, et le périmètre n'est pas le même : un réseau local, et une machine
// qui n'est pas allumée en permanence.
//
// POURQUOI UN VRAI COMPTE ET PAS UNE IDENTITÉ JETABLE : un nom d'usage sans
// mot de passe ne permet pas de REVENIR. Le jour où son jeton expire, la
// personne redevient quelqu'un d'autre — elle perd ses sauvegardes, son
// mapping de manette et sa place dans la file. C'est ce qui a fait
// abandonner le modèle de l'invité anonyme (2026-09-04).
func (service *Service) Register(username, password, displayName string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	password = strings.TrimSpace(password)
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = username
	}
	if len(username) < 2 || len(username) > 32 {
		return "", fmt.Errorf("embed: le nom d'utilisateur doit faire entre 2 et 32 caractères")
	}
	for _, r := range username {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return "", fmt.Errorf("embed: lettres, chiffres, - et _ seulement")
		}
	}
	if len(password) < 4 {
		return "", fmt.Errorf("embed: le mot de passe doit faire au moins 4 caractères")
	}
	if !service.RegistrationOpen() {
		return "", fmt.Errorf("embed: les inscriptions sont fermées sur cet appareil")
	}

	var taken int
	if err := service.DB.QueryRow(
		`SELECT COUNT(*) FROM users WHERE username = ?`, username,
	).Scan(&taken); err != nil {
		return "", fmt.Errorf("embed: name check: %w", err)
	}
	if taken > 0 {
		return "", fmt.Errorf("embed: ce nom est déjà pris")
	}

	hashed, err := auth.HashPassword(password)
	if err != nil {
		return "", fmt.Errorf("embed: hash password: %w", err)
	}
	result, err := service.DB.Exec(
		`INSERT INTO users (username, hashed_password, display_name, role_level, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		username, hashed, displayName, auth.RoleUser,
		time.Now().Format("2006-01-02 15:04:05.999999"),
	)
	if err != nil {
		return "", fmt.Errorf("embed: register: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", fmt.Errorf("embed: register: %w", err)
	}
	token, err := auth.CreateToken(service.secret, id, username, auth.RoleUser)
	if err != nil {
		return "", fmt.Errorf("embed: token: %w", err)
	}
	slog.Info("embedded identity: account registered", "username", username)

	return token, nil
}

// RegisterGuest inscrit un invité, et rend son jeton.
//
// POURQUOI UN VRAI COMPTE, et pas seulement un jeton signé : le téléphone
// savait déjà frapper un jeton pour un invité du réseau, mais ce jeton ne
// porte qu'un nom — il n'a pas de ligne en base, donc `/api/v1/auth/me` le
// rejette, et la page retombe sur l'écran de connexion qu'on essayait
// justement de lui éviter. Le piège est déjà documenté pour le jeton
// d'appareil ; c'est le même.
//
// POURQUOI PAS LE COMPTE DU PROPRIÉTAIRE : prêter root à un invité lui donne
// L'IDENTITÉ de l'hôte. La file de joueurs est indexée par nom, donc les deux
// deviennent la même personne, et l'écran de jeu propose « reprendre la
// manette » au lieu de « rejoindre » (signalé le 2026-09-04).
//
// L'invité ne choisit donc qu'un NOM D'USAGE. L'identifiant et le mot de passe
// sont tirés au sort et ne servent jamais à ouvrir une session : c'est le
// jeton qui la porte, et il vit dans le navigateur de l'invité.
func (service *Service) RegisterGuest(displayName string) (string, string, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return "", "", fmt.Errorf("embed: guest name is required")
	}
	if len(displayName) > 32 {
		displayName = strings.TrimSpace(displayName[:32])
	}

	username, err := freeGuestUsername(service.DB, displayName)
	if err != nil {
		return "", "", err
	}

	// Un mot de passe que personne ne connaît, pas même nous : il n'existe
	// que pour que la colonne ne soit pas vide.
	hashed, err := auth.HashPassword(config.RandomToken(24))
	if err != nil {
		return "", "", fmt.Errorf("embed: hash guest password: %w", err)
	}

	result, err := service.DB.Exec(
		`INSERT INTO users (username, hashed_password, display_name, role_level, created_at)
		 VALUES (?, ?, ?, 2, ?)`,
		username, hashed, displayName,
		time.Now().Format("2006-01-02 15:04:05.999999"),
	)
	if err != nil {
		return "", "", fmt.Errorf("embed: register guest: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", "", fmt.Errorf("embed: register guest: %w", err)
	}

	token, err := auth.CreateToken(service.secret, id, username, 2)
	if err != nil {
		return "", "", fmt.Errorf("embed: guest token: %w", err)
	}
	slog.Info("embedded identity: guest registered", "username", username, "name", displayName)

	return token, username, nil
}

// freeGuestUsername dérive un identifiant du nom d'usage, et le rend unique.
//
// Deux invités qui s'appellent pareil ne doivent pas devenir la même personne
// dans la file de joueurs — c'est l'invariant que tout le reste suppose. Un
// nom déjà pris reçoit un suffixe plutôt qu'un refus : un invité est un
// invité, pas un compte avec qui négocier.
func freeGuestUsername(db *sql.DB, displayName string) (string, error) {
	base := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		default:
			return '-'
		}
	}, displayName)
	base = strings.Trim(base, "-")
	if base == "" {
		base = "invite"
	}

	for suffix := 0; suffix < 200; suffix++ {
		candidate := base
		if suffix > 0 {
			candidate = fmt.Sprintf("%s-%d", base, suffix+1)
		}
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM users WHERE username = ?`, candidate,
		).Scan(&count); err != nil {
			return "", fmt.Errorf("embed: guest name check: %w", err)
		}
		if count == 0 {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("embed: no free guest name for %q", displayName)
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

	if _, err := db.Exec(
		`INSERT INTO users (username, hashed_password, display_name, role_level, created_at)
		 VALUES (?, ?, ?, 0, ?)`,
		cfg.RootUsername, hashed, "Root", time.Now().Format("2006-01-02 15:04:05.999999"),
	); err != nil {
		return fmt.Errorf("embed: seed root: %w", err)
	}
	slog.Info("embedded identity: root account created", "username", cfg.RootUsername)

	return nil
}
