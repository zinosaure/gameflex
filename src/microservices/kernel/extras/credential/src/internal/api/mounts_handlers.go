package api

// Les montages distants — un partage WebDAV, un kDrive ou un partage SMB,
// monté comme une source de la bibliothèque.
//
// Séparé des serveurs de fédération, et ce n'est pas de la duplication : un
// serveur GameFlex a déjà classé ses jeux et rend un catalogue, un partage
// est un arbre de dossiers qu'on scanne. Ni les mêmes champs, ni le même
// client, ni la même façon d'échouer.
//
// LE MOT DE PASSE NE RESSORT JAMAIS. Contrairement au jeton d'un serveur
// fédéré — qu'on peut vouloir relire pour le donner à un autre appareil —
// celui-ci est un identifiant de compte chez un tiers. Il entre, il sert au
// kernel qui monte le partage, et il ne repart pas par l'API d'administration.

import (
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/secretbox"
	"gameflex/storage"
	"gameflex/storage/kdrive"
	"gameflex/storage/smb"
	"gameflex/storage/webdav"
)

type remoteMountResponse struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	// Folder — le sous-dossier parcouru. Vide = la racine du partage.
	Folder    string `json:"folder"`
	Username  string `json:"username"`
	CreatedAt any    `json:"created_at"`
}

// probeShare — regarder une source distante, quel que soit son protocole.
//
// Tous rendent la MÊME chose (storage.ProbeResult) : l'écran qui l'affiche
// n'a donc pas à savoir lequel lui répond — c'est ce qui a permis d'ajouter
// SMB sans toucher à l'écran.
func probeShare(kind, name, base, folder, username, password string) (storage.ProbeResult, error) {
	switch kind {
	case "smb":
		return smb.Probe(smb.Config{
			Name: name, Address: base, Folder: folder, Username: username, Password: password,
		})
	case "webdav":
		return webdav.Probe(webdav.Config{
			Name: name, BaseURL: base, Username: username, Password: password,
		}, folder)
	case "kdrive":
		return kdrive.Probe(kdrive.Config{
			Name: name, DriveID: base, Token: password, Folder: folder,
		})
	default:
		return storage.ProbeResult{}, errors.New("unsupported mount kind: " + kind)
	}
}

// smbAddressProblem — ce qui a été saisi ne compose pas une adresse de
// partage. Le détail vient de smb.Address, qui sait dire lequel des trois
// champs pèche.
func smbAddressProblem(err error) string {
	return "This is not a share address: " + err.Error()
}

// normalizeFolder — un chemin de dossier propre, sans barres autour.
//
// « /roms/ », « roms » et « roms/ » désignent la même chose et doivent être
// rangés pareil : sinon le même partage ajouté deux fois donne deux adresses
// différentes, et l'une des deux ne trouve rien.
func normalizeFolder(folder string) string {
	return strings.Trim(strings.TrimSpace(strings.ReplaceAll(folder, "\\", "/")), "/")
}

// handleListRemoteMounts — GET /api/v1/mount-registry.
func (server *Server) handleListRemoteMounts(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	rows, err := server.db.Query(
		`SELECT id, name, kind, base_url, folder, username, created_at FROM remote_mounts ORDER BY name`,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	defer rows.Close()

	items := []remoteMountResponse{}
	for rows.Next() {
		var item remoteMountResponse
		var createdAt sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Kind, &item.BaseURL, &item.Folder, &item.Username, &createdAt,
		); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		item.CreatedAt = isoDatetime(createdAt)
		items = append(items, item)
	}

	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

// handleAddRemoteMount — POST /api/v1/mount-registry.
//
// kDrive, WebDAV et SMB entrent par la MÊME porte : seule la façon d'obtenir
// l'adresse diffère, et elle se règle ici plutôt que dans le formulaire —
// qui ne demande alors, pour un kDrive, que l'identifiant du drive, et pour
// un partage SMB, le serveur et le nom du partage.
func (server *Server) handleAddRemoteMount(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		BaseURL  string `json:"base_url"`
		DriveID  string `json:"drive_id"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Share    string `json:"share"`
		Folder   string `json:"folder"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	name := strings.TrimSpace(body.Name)
	kind := strings.ToLower(strings.TrimSpace(body.Kind))
	username := strings.TrimSpace(body.Username)

	base := strings.TrimRight(strings.TrimSpace(body.BaseURL), "/")
	if kind == "kdrive" {
		// L'IDENTIFIANT DU DRIVE TIENT LIEU D'ADRESSE.
		//
		// kDrive ne passe plus par WebDAV : il ne l'ouvre qu'aux offres Team
		// et Pro, et sur les autres le drive refuse tout sans même demander
		// qui vous êtes (2026-09-02). L'API REST, elle, s'authentifie par
		// jeton — il n'y a donc plus d'adresse à construire, seulement un
		// numéro à retenir.
		base = strings.TrimSpace(body.DriveID)
		// Le jeton voyage dans le champ du mot de passe : c'est le même
		// secret, au même endroit, et lui inventer une colonne à lui n'aurait
		// rien changé à la façon dont on le protège.
		username = ""
	}

	password := body.Password
	var addressErr error
	if kind == "smb" {
		// LE SERVEUR ET LE PARTAGE TIENNENT LIEU D'ADRESSE — même règle que
		// kDrive. L'adresse se compose ici, sous une forme UNIQUE : deux
		// saisies du même partage donnent la même adresse, donc la même clé
		// de cache (voir smb.Address).
		base, addressErr = smb.Address(body.Host, body.Port, body.Share)
		// Pas de compte, pas de mot de passe : c'est la connexion invitée, et
		// un mot de passe resté dans le champ n'y signifierait rien.
		if username == "" {
			password = ""
		}
	}

	switch {
	case name == "":
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "name"}, "msg": "A name is required.", "type": "value_error.missing",
		}})
		return
	case kind != "webdav" && kind != "kdrive" && kind != "smb":
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "kind"}, "msg": "Only webdav, kdrive and smb are supported.", "type": "value_error",
		}})
		return
	case kind == "smb" && addressErr != nil:
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "host"}, "msg": smbAddressProblem(addressErr), "type": "value_error",
		}})
		return
	case kind == "kdrive" && strings.TrimSpace(body.DriveID) == "":
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "drive_id"}, "msg": "A kDrive id is required.", "type": "value_error.missing",
		}})
		return
	case kind == "webdav" && base == "":
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "base_url"}, "msg": "An address is required.", "type": "value_error.missing",
		}})
		return
	case kind == "kdrive" && body.Password == "":
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "password"}, "msg": "An API token is required.",
			"type": "value_error.missing",
		}})
		return
	// SMB n'y est pas : un partage ouvert se lit sans compte, et c'est ce
	// que « Connexion anonyme » veut dire.
	case kind == "webdav" && (username == "" || body.Password == ""):
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "username"}, "msg": "A username and a password are required.", "type": "value_error.missing",
		}})
		return
	}

	// Scellé avec le secret de GameFlex (#459) : jamais en clair dans la base.
	sealed, err := secretbox.Seal(server.config.JWTSecret, password)
	if err != nil {
		slog.Error("mount: cannot seal its credential", "error", err)
		respondError(w, http.StatusInternalServerError, "This server cannot store passwords: its secret is not set.")
		return
	}
	result, err := server.db.Exec(
		`INSERT INTO remote_mounts (name, kind, base_url, folder, username, password, created_at, created_by_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		name, kind, base, normalizeFolder(body.Folder), username, sealed, now(), principal.ID,
	)
	if err != nil {
		// L'unicité porte sur le NOM : c'est le premier segment du chemin
		// virtuel de chaque jeu, et deux montages homonymes en cacheraient
		// un silencieusement.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			respondError(w, http.StatusConflict, "A mount already uses this name.")
			return
		}
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	id, _ := result.LastInsertId()

	respondItem(w, http.StatusCreated, remoteMountResponse{
		ID: id, Name: name, Kind: kind, BaseURL: base,
		Folder: normalizeFolder(body.Folder), Username: username,
	})
}

// handleCheckRemoteMount — POST /api/v1/mount-registry/check.
//
// « Est-ce que ça marche ? » — la question qu'on se pose juste après avoir
// ajouté un kDrive, et à laquelle rien ne répondait : le montage
// s'enregistrait, et il fallait attendre la bibliothèque pour découvrir qu'il
// ne rendait rien (signalé le 2026-09-02).
//
// Deux appelants, une seule route : le formulaire sonde ce qu'on vient de
// SAISIR (mot de passe compris, avant tout enregistrement), la liste sonde un
// montage DÉJÀ enregistré par son identifiant — dont le mot de passe ne sort
// jamais d'ici, ce qui est précisément pourquoi la sonde vit dans ce service.
func (server *Server) handleCheckRemoteMount(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		ID       int64  `json:"id"`
		Kind     string `json:"kind"`
		BaseURL  string `json:"base_url"`
		DriveID  string `json:"drive_id"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Share    string `json:"share"`
		Folder   string `json:"folder"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	name, base, folder, username, password := "probe",
		strings.TrimRight(strings.TrimSpace(body.BaseURL), "/"),
		normalizeFolder(body.Folder),
		strings.TrimSpace(body.Username),
		body.Password

	kind := strings.ToLower(strings.TrimSpace(body.Kind))
	if kind == "kdrive" {
		base = strings.TrimSpace(body.DriveID)
	}
	// Un partage SMB qu'on vient de SAISIR : son adresse se compose comme à
	// l'enregistrement, pour que la sonde lise exactement ce qu'on rangera.
	if kind == "smb" && body.ID == 0 {
		composed, err := smb.Address(body.Host, body.Port, body.Share)
		if err != nil {
			respondValidationError(w, []map[string]any{{
				"loc": []string{"body", "host"}, "msg": smbAddressProblem(err), "type": "value_error",
			}})

			return
		}
		base = composed
		if username == "" {
			password = ""
		}
	}

	// Un montage enregistré : on relit ses identifiants ici plutôt que de les
	// faire redescendre jusqu'au navigateur pour qu'il nous les rende.
	kindOf := ""
	if body.ID != 0 {
		row := server.db.QueryRow(
			`SELECT name, kind, base_url, folder, username, password FROM remote_mounts WHERE id = ?`,
			body.ID,
		)
		var sealed string
		if err := row.Scan(&name, &kindOf, &base, &folder, &username, &sealed); err != nil {
			respondError(w, http.StatusNotFound, "Mount not found.")

			return
		}
		opened, err := secretbox.Open(server.config.JWTSecret, sealed)
		if err != nil {
			respondError(w, http.StatusConflict, "This share's password cannot be read: "+err.Error()+". Delete the share and add it again.")

			return
		}
		password = opened
	}

	if body.ID != 0 && base != "" && !strings.Contains(base, "://") {
		// Un montage enregistré dont l'adresse n'est qu'un nombre est un
		// kDrive : c'est ce qui le distingue une fois relu de la base.
		kind = "kdrive"
	}
	if body.ID != 0 && kindOf != "" {
		kind = kindOf
	}

	if base == "" || (kind == "webdav" && username == "") {
		respondValidationError(w, []map[string]any{{
			"loc": []string{"body", "base_url"}, "msg": "An address and a username are required.",
			"type": "value_error.missing",
		}})

		return
	}

	seen, err := probeShare(kind, name, base, folder, username, password)
	if err != nil {
		respondItem(w, http.StatusOK, map[string]any{
			"reachable": false,
			"detail":    probeFailure(kind, folder, err),
		})

		return
	}

	respondItem(w, http.StatusOK, map[string]any{
		"reachable": true,
		"folder":    folder,
		"folders":   seen.Folders,
		"files":     seen.Files,
	})
}

// probeFailure — ce qu'on répond quand la sonde échoue.
//
// LE DÉTAIL, PAS « échec ». « Identifiants refusés » et « dossier
// introuvable » ne se corrigent pas de la même façon, et un message unique
// oblige à tout réessayer au hasard.
func probeFailure(kind, folder string, err error) string {
	var unreachable net.Error

	switch {
	// SMB : quatre refus qui se ressemblent vus de loin, et qui se corrigent
	// chacun ailleurs — le mot de passe, le réglage d'accès anonyme, le nom
	// du partage, les droits posés sur le serveur.
	case errors.Is(err, smb.ErrUnauthorized):
		return "The server refused this username or password."
	case errors.Is(err, smb.ErrGuestRefused):
		return "This server only accepts signed connections, which anonymous " +
			"access cannot provide. Turn anonymous access off and enter an account."
	case errors.Is(err, smb.ErrShareNotFound):
		return "The server answered, but has no share by that name. " +
			"Check the share name — the folder inside it is a separate field."
	case errors.Is(err, smb.ErrForbidden):
		return "The server accepted the connection but does not let it into " +
			"this share or folder. That is a permission set on the server, not a password problem."
	case kind == "smb" && errors.Is(err, fs.ErrNotExist):
		return "The share opens, but has no folder \"" + folder + "\"."
	case kind == "smb" && errors.As(err, &unreachable):
		return "Nothing answers at this address. Check the server address " +
			"and port, and that the server is switched on."
	case errors.Is(err, webdav.ErrUnauthorized):
		return "The share refused these credentials. " +
			"kDrive needs an APPLICATION password, not the account one."
	case errors.Is(err, kdrive.ErrUnauthorized):
		return "kDrive refused this API token. Create one in the " +
			"Infomaniak manager with access to your drive."
	case errors.Is(err, webdav.ErrForbidden):
		// LE SERVEUR REFUSE SANS RIEN DEMANDER — les identifiants n'y
		// changent rien, et les accuser envoie fabriquer des mots de passe
		// pour rien.
		//
		// Sur kDrive, la cause est l'offre : WebDAV n'est ouvert qu'aux
		// comptes Team et Pro. Le drive répond alors 403 à tout, sans en-tête
		// d'authentification, tout en existant bel et bien (une adresse
		// inventée rend 404, la sienne 403 — constaté le 2026-09-02).
		//
		// On ne renvoie donc PAS vers un réglage à activer : sur une offre
		// qui n'y a pas droit, il n'y a rien à activer, et promettre le
		// contraire fait chercher pour rien.
		return "The server refused access without asking who you are, " +
			"so this is not a password problem. On kDrive, WebDAV is " +
			"available on Team and Pro plans only — on any other plan " +
			"there is nothing to switch on."
	}

	return "Cannot read this share: " + err.Error()
}

// handleRemoveRemoteMount — DELETE /api/v1/mount-registry/{id}.
func (server *Server) handleRemoveRemoteMount(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid id.")
		return
	}

	result, err := server.db.Exec(`DELETE FROM remote_mounts WHERE id = ?`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondError(w, http.StatusNotFound, "Mount not found.")
		return
	}

	respondItem(w, http.StatusOK, map[string]any{"id": id})
}

// handleInternalRemoteMounts — GET /internal/storage/mounts.
//
// La MÊME liste, mots de passe COMPRIS, pour le kernel seul : c'est lui qui
// monte les partages et scanne, et il ne peut pas le faire avec un mot de
// passe qu'on lui cache. Route interne, jamais exposée à un navigateur —
// même séparation que le reste des routes /internal.
func (server *Server) handleInternalRemoteMounts(w http.ResponseWriter, r *http.Request) {
	// Les mots de passe des partages : au kernel seulement (internalCaller).
	if !server.internalCaller(w, r) {
		return
	}

	rows, err := server.db.Query(
		`SELECT id, name, kind, base_url, folder, username, password FROM remote_mounts ORDER BY name`,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	defer rows.Close()

	type internalMount struct {
		remoteMountResponse
		Password string `json:"password"`
	}

	items := []internalMount{}
	for rows.Next() {
		var item internalMount
		var sealed string
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Kind, &item.BaseURL, &item.Folder,
			&item.Username, &sealed,
		); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		// Un identifiant illisible (le secret a changé) : le partage part
		// sans, et son montage le dira ; les autres restent servis.
		opened, err := secretbox.Open(server.config.JWTSecret, sealed)
		if err != nil {
			slog.Warn("mount: credential cannot be opened", "mount", item.Name, "error", err)
		}
		item.Password = opened
		items = append(items, item)
	}

	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}
