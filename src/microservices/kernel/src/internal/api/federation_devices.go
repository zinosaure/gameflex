// Les consoles à distance de ce serveur, vues d'ailleurs — chantier 9, C9-P2.
//
// La surface fédérée du chantier 6 ne montrait que des jeux, et son propre
// commentaire disait pourquoi : « prêter sa bibliothèque n'est pas prêter sa
// machine ». Rien ne change là-dessus. Ce fichier ouvre strictement une
// LISTE — voir qu'il y a une PS5 dans ce salon — et la portée qui la protège
// est distincte de celle du catalogue, pour que les jetons déjà distribués
// n'en héritent pas en silence.
//
// Jouer dessus est une autre décision, et elle passe par un COMPTE chez celui
// qui la tient. Ce jeton-ci n'y donne aucun droit.
//
// LA RÈGLE DE CE FICHIER : une projection, jamais un relais. Le service
// remote-play rend `DeviceOut`, qui embarque la ligne de base complète — donc
// `rp_key`, `regist_key`, `psnAccountId` : de quoi réveiller la console et la
// streamer. Les relayer telles quelles rouvrirait, en plus large, la faille
// corrigée le 2026-08-27. Les six champs ci-dessous sont donc recopiés à la
// main, et rien ne peut s'y glisser plus tard sans qu'on l'ait écrit.
package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gameflex/consoles"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage"
	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

// federationDevice — tout ce qu'un porteur de jeton apprend d'une console.
//
// L'adresse en fait partie : deux PS5 portent souvent le même nom, et c'est
// elle qui les distingue dans une liste qui en mêle plusieurs serveurs. Elle
// n'ouvre rien — la console refuse tout ce qui n'est pas appairé.
type federationDevice struct {
	ID               string `json:"id"`
	HostID           string `json:"hostId"`
	HostName         string `json:"hostName"`
	HostType         string `json:"hostType"`
	IPAddress        string `json:"ipAddress"`
	Status           string `json:"status"`
	HasActiveSession bool   `json:"hasActiveSession"`
}

// errNoRemoteplay — ce serveur n'a pas de plan de contrôle remote-play.
var errNoRemoteplay = errors.New("remote play is not configured")

// handleFederationDevices — GET /api/v1/federation/remoteplay/devices.
//
// Une liste vide n'est pas une erreur : un serveur sans console à distance
// est le cas le plus courant (un téléphone, par exemple). Répondre 404 ou 503
// obligerait chaque client à distinguer « pas de console » de « serveur en
// panne », alors que les deux se traitent pareil — on n'affiche rien.
func (server *Server) handleFederationDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := server.remoteplayDevices()
	if errors.Is(err, errNoRemoteplay) {
		writeJSON(w, http.StatusOK, []federationDevice{})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "cannot reach remote play: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, devices)
}

// remoteplayDevices demande la liste au service remote-play par le plan
// interne — le même chemin que les conteneurs de session, et le seul que son
// garde accepte sans compte (voir auth_guard.go).
func (server *Server) remoteplayDevices() ([]federationDevice, error) {
	if server.config.RemoteplayControlURL == "" || server.config.InternalSecret == "" {
		return nil, errNoRemoteplay
	}

	request, err := http.NewRequest(
		http.MethodGet,
		server.config.RemoteplayControlURL+"/api/v1/playstation/my-devices",
		nil,
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)

	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, errors.New(response.Status)
	}

	// On décode dans une structure QUI N'A QUE CE QU'ON REND. Le reste de la
	// réponse — les clés d'appairage — n'entre jamais en mémoire ici, donc
	// personne ne peut le rendre par distraction en ajoutant un champ à la
	// sérialisation.
	var envelope struct {
		Data []federationDevice `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if envelope.Data == nil {
		return []federationDevice{}, nil
	}

	return envelope.Data, nil
}

// ── L'autre sens : les consoles des serveurs qu'ON a enregistrés ──────────

// remoteDevice — une console fédérée telle qu'elle arrive dans NOTRE liste.
//
// Deux champs s'ajoutent à ce que le serveur d'en face a dit, et ils sont
// tous les deux à nous : `source`, le nom sous lequel on l'a enregistré (la
// carte l'affiche entre parenthèses), et `serverId`, par quoi on le
// retrouvera au moment de s'y connecter.
type remoteDevice struct {
	federationDevice
	Source   string `json:"source"`
	ServerID int64  `json:"serverId"`
	// L'ADRESSE, parce que la page devra s'y CONNECTER (C9-P3) et qu'elle
	// n'a aucun autre moyen de l'apprendre : la liste des serveurs est
	// réservée aux administrateurs, alors qu'emprunter une console est un
	// geste de joueur.
	//
	// Ce n'est pas un secret — c'est ce que quelqu'un a tapé pour ajouter ce
	// serveur. Le jeton, lui, ne sort toujours pas.
	Link string `json:"link"`
}

// handleRemoteDevices — GET /api/v1/servers/remoteplay/devices.
//
// L'aval de handleFederationDevices : celle-là sert nos consoles aux autres,
// celle-ci va chercher les leurs. C'est le kernel qui interroge, et non le
// navigateur, pour une raison qui n'est pas de goût : le jeton d'un serveur
// enregistré ne sort JAMAIS de la base (voir handleListFederationServers, qui
// le retire de sa réponse). La page n'a donc rien à présenter là-bas, et
// c'est très bien ainsi.
//
// Ne rend QUE le fédéré. Les consoles d'ici ont déjà leur route, avec leur
// forme complète ; les mêler ici obligerait à choisir laquelle des deux
// formes rendre, et l'une des deux y perdrait.
func (server *Server) handleRemoteDevices(w http.ResponseWriter, r *http.Request) {
	registered, err := server.registeredServers()
	if err != nil || len(registered) == 0 {
		writeJSON(w, http.StatusOK, []remoteDevice{})
		return
	}

	// LES AMIS SONT INTERROGÉS EN PARALLÈLE (#106). En série, chacun ajoutait
	// jusqu'au délai du client : trois amis éteints faisaient attendre une
	// liste qui ne dépend pas d'eux. Chaque réponse garde sa PLACE — l'ordre
	// du registre — pour que la liste ne change pas d'un affichage à l'autre
	// selon celui qui répond le premier.
	perFriend := make([][]remoteDevice, len(registered))
	var wait sync.WaitGroup
	for index, entry := range registered {
		wait.Add(1)
		go func() {
			defer wait.Done()
			perFriend[index] = devicesOf(entry)
		}()
	}
	wait.Wait()

	devices := []remoteDevice{}
	for _, found := range perFriend {
		devices = append(devices, found...)
	}

	writeJSON(w, http.StatusOK, devices)
}

// devicesOf — les consoles à distance d'UN ami, identifiants préfixés.
func devicesOf(entry registeredServer) []remoteDevice {
	client, err := remote.New(entry.Link, entry.Token)
	if err != nil {
		return nil
	}

	// UN SERVEUR QUI NE RÉPOND PAS EST SAUTÉ, JAMAIS FATAL.
	//
	// Même règle que pour les catalogues, et pour la même raison : une liste
	// ne doit pas dépendre du routeur de quelqu'un d'autre. Trois refus
	// légitimes se confondent ici, et c'est voulu — le serveur est éteint, il
	// est d'avant ce chantier (404), ou son jeton n'accorde pas les consoles
	// (403). Les trois s'affichent pareil : rien.
	found, err := client.Devices()
	if err != nil {
		return nil
	}

	devices := make([]remoteDevice, 0, len(found))
	for _, device := range found {
		// L'IDENTIFIANT EST PRÉFIXÉ, comme celui d'un jeu fédéré.
		//
		// Deux serveurs frappent leurs identifiants dans le même espace, et la
		// page s'en sert pour chercher une session dans NOTRE façade. Sans
		// préfixe, une console de là-bas pourrait se voir attribuer une partie
		// d'ici.
		device.ID = remoteReference(entry.ID, device.ID)
		devices = append(devices, remoteDevice{
			federationDevice: federationDevice(device),
			Source:           entry.Hostname,
			ServerID:         entry.ID,
			Link:             entry.Link,
		})
	}

	return devices
}

// ── Ce qu'un pair possède : la copie de son listing ─────────────────────

// handleRemoteStorageCores — GET /api/v1/servers/{id}/storage/cores.
// handleRemoteStorageBios  — GET /api/v1/servers/{id}/storage/bios.
// handleRemoteStorageRoms  — GET /api/v1/servers/{id}/storage/roms.
//
// LUS DANS LA COPIE DE SON LISTING (chantier 15), et non plus chez lui à
// chaque affichage : sa date se demande au plus une fois par minute, et la
// copie ne se re-télécharge que si elle a avancé (remote.Copies). Les mêmes
// formes que les écrans de cette machine — mêmes constructeurs, appliqués à
// sa copie.
//
// SEUL UN SERVEUR FÉDÉRÉ PEUT FOURNIR DES CŒURS ET DES BIOS : un montage
// Samba, WebDAV ou kDrive ne porte que des ROMs.
//
// UN SEUL pair, désigné par son identifiant, et non tous à la fois : l'écran
// demande « que contient CELUI-CI ». Fondre plusieurs bibliothèques en une
// liste ne dirait plus chez qui un fichier manque, ce qui est justement la
// question qu'on se pose en la regardant.
func (server *Server) handleRemoteStorageCores(w http.ResponseWriter, r *http.Request) {
	server.serveRemoteStorage(w, r, listing.Cores, func(_ registeredServer, items map[string][]listing.Item) any {
		return coresSectionsFrom(items)
	})
}

func (server *Server) handleRemoteStorageBios(w http.ResponseWriter, r *http.Request) {
	server.serveRemoteStorage(w, r, listing.Bios, func(_ registeredServer, items map[string][]listing.Item) any {
		return biosSectionsFrom(items)
	})
}

func (server *Server) handleRemoteStorageRoms(w http.ResponseWriter, r *http.Request) {
	server.serveRemoteStorage(w, r, listing.Roms, func(entry registeredServer, items map[string][]listing.Item) any {
		// Le statut de ses consoles (chantier 30) est dans l'en-tête de la copie.
		header, _ := server.copies().Header(peerOf(entry))

		return remote.RomSections(items, header.Statuses)
	})
}

func (server *Server) serveRemoteStorage(
	w http.ResponseWriter, r *http.Request, section string,
	shape func(registeredServer, map[string][]listing.Item) any,
) {
	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}

	items, err := server.copies().SectionAll(peerOf(entry), section)
	if err != nil {
		// UN PAIR MUET N'EST PAS UNE PANNE D'ICI. Les refus légitimes se
		// confondent, et c'est voulu : il est éteint, ou son jeton n'accorde
		// pas le catalogue (403). L'écran doit lire « rien à montrer », pas
		// « erreur ».
		//
		// `[]any{}` et non une forme précise : ce repli sert les trois
		// sections (cœurs, BIOS, ROMs), et rendre la forme de l'une d'elles
		// ici serait juste par accident.
		slog.Info("peer storage: skipped", "server", entry.Hostname, "section", section, "error", err)
		writeJSON(w, http.StatusOK, map[string]any{"consoles": []any{}})

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"consoles": shape(entry, items)})
}

// registeredByID résout UN pair par son identifiant, et répond lui-même en cas
// d'échec — même forme que mountByID pour les partages montés.
//
// Extrait parce que trois routes le faisaient déjà à la main : un registre
// illisible et un identifiant inconnu ne doivent pas avoir trois nuances de
// message selon la route qui les rencontre.
func (server *Server) registeredByID(w http.ResponseWriter, r *http.Request) (registeredServer, bool) {
	return server.registeredByIDValue(w, r.PathValue("id"))
}

// registeredByIDValue — la même résolution, pour un identifiant qui arrive
// ailleurs que dans le chemin (le `?server=` de la liste des jeux).
func (server *Server) registeredByIDValue(w http.ResponseWriter, raw string) (registeredServer, bool) {
	wanted, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server id")

		return registeredServer{}, false
	}

	// Un serveur tout juste ajouté ne doit pas être « inconnu » pendant les
	// 10 s du cache : `find` relit une fois (#230).
	entry, found, err := server.serverRegistry.find(server.fetchRegisteredServers, func(candidate registeredServer) bool {
		return candidate.ID == wanted
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "cannot read the server registry")

		return registeredServer{}, false
	}
	if !found {
		writeError(w, http.StatusNotFound, "unknown server")

		return registeredServer{}, false
	}

	return entry, true
}

// biosBelongsTo — ce BIOS est-il attendu par cette console ?
//
// Le jumeau de coreBelongsTo, et pour la même raison : le nom devient un chemin
// sur le disque. Le registre déclare les fichiers qu'une console réclame
// (consoles.BiosFor) ; tout le reste est refusé, jamais rangé « au cas où ».
func biosBelongsTo(consoleID, name string) bool {
	if consoleID == "" || name == "" {
		return false
	}
	for _, declared := range consoles.BiosFor(consoleID) {
		if declared == name {
			return true
		}
	}

	return false
}

// handleRemoteRescan — POST /api/v1/servers/{id}/rescan.
//
// LE MÊME NOM QUE POUR UN MONTAGE, et c'est voulu : `POST /api/v1/mounts/
// {id}/rescan` fait déjà ce geste sur l'autre famille de source. Un lecteur
// n'a donc qu'un vocabulaire à apprendre.
//
// Mais pas le même travail derrière : un montage se PARCOURT (des minutes sur
// un vrai kDrive, donc en tâche de fond), alors qu'un pair a déjà fait son
// listing — « le bouton rescan qui en vérité ne va que re-télécharger la
// dernière version de ce fichier » (le porteur, chantier 15). La page
// l'appelle aussi juste après avoir ajouté le serveur : c'est la copie prise à
// l'ajout.
func (server *Server) handleRemoteRescan(w http.ResponseWriter, r *http.Request) {
	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}

	header, err := server.copies().Refresh(peerOf(entry))
	if err != nil {
		refused := remote.Refusal(err)
		writeError(w, refused.Status, refused.Message)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rescanned": true,
		"server":    entry.Hostname,
		"updatedAt": header.UpdatedAt,
	})
}

// ── Rapatrier un élément depuis un pair ──────────────────────────────────

// handleRemoteStorageImport — POST /api/v1/servers/{id}/storage/{section}
// avec `{consoleId, name}`. Section : « cores » ou « bios ».
//
// L'ÉCHANGE DANS NOTRE SENS (#154). Les routes de listage disent ce qu'un pair
// possède ; celle-ci le dépose chez nous, dans `{cores,bios}/<console>/`. Un
// BIOS ne vient QUE de là (personne ne le distribue) et un cœur peut venir de
// là ou de sa source officielle — c'est la matrice des sources.
//
// Écriture atomique, et le même garde-fou que la mise en scène d'un jeu
// distant : `.part` d'abord, renommage seulement si tout est arrivé. Un fichier
// tronqué qui porte le nom du vrai est pire qu'un fichier absent — il échoue
// plus tard, ailleurs, sans rien dire.
//
// ON NE REMPLACE JAMAIS UN FICHIER QUI EXISTE. C'est la leçon de #142/#143 :
// un téléchargement automatique avait écrasé deux cœurs qui marchaient, et
// deux consoles avaient été perdues en une nuit. Pour reprendre une version,
// on supprime d'abord — un geste explicite.
func (server *Server) handleRemoteStorageImport(w http.ResponseWriter, r *http.Request) {
	// LES TROIS SECTIONS — et « roms » manquait ici (#157).
	//
	// Signalé par l'utilisateur le 2026-09-17 : « section must be cores or
	// bios » en essayant de rapatrier un jeu depuis un pair. Ce garde précède
	// le `switch` qui choisit la racine, et j'avais ajouté le cas `roms` à ce
	// switch SANS LIRE LE DÉBUT DE LA FONCTION : le chemin était donc mort
	// avant d'y arriver, et les tests de compilation ne pouvaient rien en dire.
	//
	// La leçon, puisqu'elle se répète : lire une fonction ENTIÈRE avant d'y
	// greffer une branche — un garde en tête invalide silencieusement tout ce
	// qu'on ajoute plus bas.
	section := r.PathValue("section")
	// LES PARAMÈTRES ET LES CORRECTIFS (chantier 15) ont leur propre corps :
	// un réglage d'émulateur, un dossier de packs — pas un fichier désigné
	// par un nom ou une référence.
	switch section {
	case listing.Params:
		server.importPeerParams(w, r)

		return
	case listing.Patches:
		server.importPeerPatch(w, r)

		return
	}
	if section != "cores" && section != "bios" && section != "roms" {
		writeError(w, http.StatusBadRequest, "section must be cores, bios, roms, params or patches")

		return
	}

	var body struct {
		ConsoleID string `json:"consoleId"`
		Name      string `json:"name"`
		// UN JEU NE SE DÉSIGNE PAS COMME UN CŒUR. Son identifiant chez l'autre
		// est une RÉFÉRENCE de catalogue, et le nom du fichier à écrire ici se
		// déduit de son titre. `Files` ne sert qu'à connaître l'extension d'un
		// jeu mono-fichier : sans « .nes », aucun cœur ne le chargera (même
		// leçon que l'en-tête de disposition corrigé le 2026-09-02).
		Reference string   `json:"reference"`
		Title     string   `json:"title"`
		Files     []string `json:"files"`
		// Folder — le pair l'annonce en dossier : il arrivera zippé et se
		// dépliera, même avec un seul fichier à sa racine (un `.iso` PSP).
		Folder bool `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corps illisible")

		return
	}
	body.ConsoleID = strings.ToLower(strings.TrimSpace(body.ConsoleID))
	body.Name = strings.TrimSpace(body.Name)
	body.Reference = strings.TrimSpace(body.Reference)
	if body.ConsoleID == "" {
		writeError(w, http.StatusBadRequest, "consoleId est requis")

		return
	}
	// UNE CONSOLE DU REGISTRE, comme l'import depuis un partage
	// (handleMountImport). Elle devient un dossier sous roms/ (ou bios/,
	// cores/) : `../../../../etc/cron.d` faisait écrire le jeu hors du stockage,
	// en root, dans un conteneur qui a le socket Docker (revue de sécurité,
	// 2026-10-28).
	if !consoles.Exists(body.ConsoleID) {
		writeError(w, http.StatusBadRequest, "console inconnue")

		return
	}

	if section == "roms" {
		// Le nom de fichier est DÉRIVÉ, puis vérifié — jamais recopié tel
		// quel : il vient d'une page, et il sert à écrire sur le disque.
		files := body.Files
		if body.Folder {
			files = nil // un dossier garde son titre, sans extension
		}
		derived, err := storage.RomFileName(body.Title, files)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())

			return
		}
		if body.Reference == "" {
			writeError(w, http.StatusBadRequest, "reference est requise pour un jeu")

			return
		}
		// Une fois dérivé, le jeu suit le même chemin que les autres : le
		// contrôle d'existence et la réponse n'ont pas à connaître trois cas.
		body.Name = derived
	} else if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name est requis")

		return
	}

	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}

	client, err := remote.New(entry.Link, entry.Token)
	if err != nil {
		writeError(w, http.StatusBadGateway, "this address did not answer")

		return
	}

	// Le nom devient un chemin : il passe par le registre, jamais tel quel.
	var target string
	var open func() (io.ReadCloser, error)
	policy := zipPolicyFor(section, body.Name, body.Files, body.Folder)
	switch section {
	case "cores":
		// UN FICHIER D'ÉMULATEUR DE BUREAU (chantier 29) : rangé sous son nom,
		// et gardé tel quel — l'archive de shadPS4 ou un paquet du téléphone
		// EST le fichier attendu.
		if retro_files.IsEmulatorFile(body.ConsoleID, body.Name) {
			target = filepath.Join(server.retroCoresRoot(), body.ConsoleID, body.Name)
			open = func() (io.ReadCloser, error) { return client.OpenEmulatorFile(body.ConsoleID, body.Name) }
			policy = zipKeep

			break
		}
		if !coreBelongsTo(body.ConsoleID, body.Name) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("le core « %s » n'est pas déclaré pour %s", body.Name, body.ConsoleID))

			return
		}
		target = filepath.Join(server.retroCoresRoot(), body.ConsoleID,
			retro_files.CoreFileName(body.Name))
		open = func() (io.ReadCloser, error) { return client.OpenCore(body.ConsoleID, body.Name) }
	case "bios":
		if !biosBelongsTo(body.ConsoleID, body.Name) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("le BIOS « %s » n'est pas attendu par %s", body.Name, body.ConsoleID))

			return
		}
		target = filepath.Join(server.retroBiosRoot(), body.ConsoleID, body.Name)
		open = func() (io.ReadCloser, error) { return client.OpenBios(body.ConsoleID, body.Name) }
	case "roms":
		// PAS DE CONTRÔLE CONTRE LE REGISTRE ICI, et c'est la différence de
		// fond avec les deux autres : le registre déclare quels cœurs et quels
		// BIOS une console attend, donc un nom inventé s'y heurte. Personne ne
		// déclare la liste des jeux qui existent — c'est le catalogue de
		// l'autre serveur qui la porte, et c'est lui qui répondra 404 si la
		// référence ne lui dit rien. Ce qui est vérifié ici, c'est le seul
		// élément dangereux : le nom du fichier qu'on va écrire (`romFileName`).
		target = filepath.Join(server.retroRomsRoot(), body.ConsoleID, body.Name)
		open = func() (io.ReadCloser, error) {
			return client.OpenRom(body.ConsoleID, body.Reference)
		}
	default:
		// FILET, et non validation : le garde en tête de fonction a déjà
		// tranché. Il reste parce qu'un `switch` sans défaut laisserait
		// `target` et `open` vides, et on écrirait alors n'importe où.
		writeError(w, http.StatusBadRequest,
			"section inconnue : cores, bios ou roms")

		return
	}

	if _, err := os.Stat(target); err == nil {
		writeError(w, http.StatusConflict,
			"ce fichier existe déjà : le supprimer d'abord pour en reprendre une version")

		return
	}

	// CE QU'ON FAIT D'UNE ARCHIVE DÉPEND DE LA SECTION, et pour un jeu du
	// nombre de fichiers que le pair annonce — pas de sa signature (voir
	// `zipPolicy`). Une liste vide ou d'un seul élément veut dire « un
	// fichier » : on garde l'archive telle quelle, ce qui est la forme normale
	// d'une ROM. C'est aussi le repli le plus sûr quand le pair n'annonce
	// rien : un cœur sait lire un jeu zippé, il ne sait pas lire un dossier
	// nommé « … .zip ».
	written, err := importIntoLibrary(open, target, policy)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"section": section, "consoleId": body.ConsoleID, "name": body.Name,
		"sizeBytes": written, "source": entry.Hostname,
	})
}

// importIntoLibrary écrit ce que le pair envoie, sans jamais laisser de
// demi-fichier derrière soi.
//
// CE QUI ARRIVE N'EST PAS TOUJOURS UN FICHIER. `OpenRom` et `OpenBios` disent
// dans leur propre commentaire qu'un jeu multi-fichiers et un dump en dossier
// arrivent ZIPPÉS — le serveur d'en face les assemble à la volée — et que
// « c'est à celui qui range de décider quoi en faire ». C'est ici, ce
// décideur.
//
// LA SIGNATURE NE SUFFIT PAS À DÉCIDER, et j'ai eu tort de le croire.
//
// J'avais écrit que les quatre premiers octets faisaient autorité, contre la
// liste de fichiers « qui fait l'aller-retour par le navigateur ». C'est faux
// pour un jeu : un `.zip` EST la forme normale d'une ROM — les cœurs libretro
// les lisent zippées, et une bibliothèque entière est en `.zip`. La signature
// ne distingue donc pas « ce jeu est une archive » de « ce dossier a été zippé
// à la volée » : les deux commencent par `PK\x03\x04`.
//
// Le seul discriminant est le NOMBRE DE FICHIERS que le catalogue du pair
// annonce, ce que #154 disait déjà avant que je ne le remplace par mon
// raisonnement sur les octets. D'où une politique explicite, choisie par
// l'appelant :
//
//   - `zipRefuse`  un cœur. Un `.so` ne peut pas être une archive : la recevoir
//     signifie qu'on reçoit autre chose, et mieux vaut refuser que poser un
//     fichier qui mentira au premier lancement.
//   - `zipKeep`    une ROM d'un seul fichier, ou un BIOS dont le nom DÉCLARÉ
//     finit par « .zip » (`neogeo.zip`, que le registre attend tel quel).
//     L'archive est le fichier attendu : on l'écrit telle quelle. La déplier
//     produirait un dossier portant « .zip » dans son nom, que le scanner
//     prendrait pour un jeu multi-fichiers — et, pour la Neo-Geo, un BIOS que
//     le cœur ne trouverait plus.
//   - `zipExpand`  plusieurs fichiers, ou un dump de BIOS en dossier. Là
//     l'archive est un emballage, et le dossier est le résultat attendu.
//
// La signature garde donc un rôle — dire s'il y a une archive — mais c'est
// l'appelant qui dit quoi en faire.
type zipPolicy int

const (
	zipRefuse zipPolicy = iota
	zipKeep
	zipExpand
)

// zipPolicyFor — la règle, nommée et testable.
//
// Elle vit dans une fonction plutôt qu'en ligne dans le handler parce que je
// l'ai écrite À L'ENVERS une première fois (décider sur la signature du flux au
// lieu du nombre de fichiers annoncé), et que rien ne l'avait rattrapée : une
// règle qu'aucun test ne tient se reperd.
//
// Une liste vide compte comme UN fichier : c'est le repli le plus sûr quand un
// pair n'annonce rien, puisqu'un cœur sait lire un jeu zippé mais pas un
// dossier nommé « … .zip ».
//
// LE NOMBRE NE SUFFIT PAS POUR AUTANT : `files` ne liste que la racine, et un
// jeu PSP rangé dans son dossier n'y a qu'un `.iso`. Le pair l'envoie pourtant
// zippé, puisque c'est un dossier — `folder` le dit, et il l'emporte
// (2026-10-10).
//
// LE NOM COMPTE AUSSI, pour les BIOS. Le registre en déclare un qui EST une
// archive — `neogeo.zip` — et le déplier donnerait un dossier `neogeo.zip/` que
// le cœur ne trouverait pas. Ce cas ne se devine pas du flux : il se lit du nom
// attendu, donc de chez nous.
func zipPolicyFor(section, name string, files []string, folder bool) zipPolicy {
	switch {
	case section == "cores":
		return zipRefuse
	case section == "roms" && !folder && len(files) <= 1:
		return zipKeep
	case section == "bios" && strings.HasSuffix(strings.ToLower(name), ".zip"):
		return zipKeep
	default:
		return zipExpand
	}
}

func importIntoLibrary(
	open func() (io.ReadCloser, error), target string, policy zipPolicy,
) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, fmt.Errorf("dossier impossible à créer : %w", err)
	}
	// Ce qu'on rapatrie est à la personne, pas au noyau (#146).
	adoptParentOwner(filepath.Dir(target))

	body, err := open()
	if err != nil {
		if errors.Is(err, remote.ErrDenied) {
			return 0, errors.New("ce serveur refuse notre jeton")
		}

		return 0, fmt.Errorf("ce serveur ne l'a pas donné : %w", err)
	}
	defer body.Close()

	partial := target + ".part"
	file, err := os.Create(partial)
	if err != nil {
		return 0, fmt.Errorf("écriture impossible : %w", err)
	}

	// Les quatre premiers octets disent si c'est une archive (« PK\x03\x04 »).
	// On les lit AVANT d'écrire quoi que ce soit : refuser après avoir posé
	// trente mégaoctets serait poli mais inutile.
	head := make([]byte, 4)
	read, _ := io.ReadFull(body, head)
	isZip := read == 4 && string(head) == "PK\x03\x04"
	if isZip && policy == zipRefuse {
		file.Close()
		_ = os.Remove(partial)

		return 0, errors.New(
			"ce serveur a répondu une archive là où un seul fichier est attendu")
	}

	written, err := io.Copy(file, io.MultiReader(bytes.NewReader(head[:read]), body))
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(partial)

		return 0, fmt.Errorf("téléchargement interrompu : %w", err)
	}
	if written == 0 {
		_ = os.Remove(partial)

		return 0, errors.New("ce serveur a répondu un fichier vide")
	}

	// ON NE DÉPLIE QUE CE QU'ON A DEMANDÉ À DÉPLIER. Une archive qui est le jeu
	// lui-même (`zipKeep`) suit le chemin ordinaire : renommage, comme un
	// fichier quelconque. Le `.part` disparaît dans les deux cas — il n'a
	// jamais été le résultat, seulement le brouillon.
	if isZip && policy == zipExpand {
		defer os.Remove(partial)

		return expandZipInto(partial, target)
	}

	if err := os.Rename(partial, target); err != nil {
		_ = os.Remove(partial)

		return 0, err
	}
	// LE FICHIER AUSSI, pas seulement son dossier (#146).
	//
	// Il manquait ici, et un vrai rapatriement l'a montré le 2026-09-17 :
	// `roms/gba/` arrivait bien à `1000:1000` — l'adoption du dossier, huit
	// lignes plus haut — mais le jeu dedans restait `0:0 644`, donc intouchable
	// pour la personne. `extractCore` et `writeGameFile` adoptaient déjà après
	// leur renommage ; ce chemin-ci avait été oublié, et seul un transfert réel
	// pouvait le dire.
	adoptParentOwner(target)

	return written, nil
}

// expandZipInto déplie une archive dans un DOSSIER, et rend le total écrit.
//
// Le dossier EST le jeu : c'est la forme que le scanner reconnaît (un `.cue`
// et ses pistes, un dump en plusieurs morceaux), et c'est aussi ce que
// `serveDirectoryAsZip` produit dans l'autre sens — les deux opérations sont
// donc symétriques, entrée par entrée.
//
// CHAQUE NOM D'ENTRÉE EST UNE ENTRÉE HOSTILE tant qu'il n'a pas été vérifié :
// il vient de l'archive d'un autre serveur, et « ../../databases » y passerait
// sans la garde ci-dessous (même raisonnement que `saveUserDir`, qui refuse
// tout ce qui n'est pas un nom simple). On compare le chemin nettoyé à la
// racine voulue : ce qui en sort est ignoré, pas deviné.
func expandZipInto(archivePath, targetDir string) (int64, error) {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, fmt.Errorf("archive illisible : %w", err)
	}
	defer archive.Close()

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return 0, fmt.Errorf("dossier impossible à créer : %w", err)
	}
	adoptParentOwner(targetDir)

	// CE QUE L'ARCHIVE ANNONCE, AVANT D'EN ÉCRIRE UN OCTET (#601) : elle vient
	// d'un autre serveur, et rien ne bornait ce qu'elle déplie.
	var announced uint64
	for _, entry := range archive.File {
		if !entry.FileInfo().IsDir() {
			announced += entry.UncompressedSize64
		}
	}
	if err := fitsOnDisk(targetDir, announced); err != nil {
		_ = os.Remove(targetDir)

		return 0, err
	}

	var total int64
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || retro_files.IsHidden(filepath.Base(entry.Name)) {
			continue
		}

		destination := filepath.Join(targetDir, filepath.FromSlash(entry.Name))
		relative, err := filepath.Rel(targetDir, destination)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return total, fmt.Errorf("dossier impossible à créer : %w", err)
		}
		adoptParentOwner(filepath.Dir(destination))

		// `extractCore` n'a de spécifique à un cœur que son nom : il ouvre une
		// entrée, l'écrit en `.part`, puis renomme. Le réemployer évite un
		// second exemplaire de cette écriture atomique.
		// Pas de vérificateur : ce sont des fichiers de jeu ou de BIOS, pas des
		// bibliothèques à charger.
		written, err := extractCore(entry, destination, nil)
		if err != nil {
			return total, err
		}
		total += written
	}

	if total == 0 {
		_ = os.RemoveAll(targetDir)

		return 0, errors.New("cette archive ne contenait aucun fichier utilisable")
	}

	return total, nil
}

// ── Où joindre un serveur enregistré ─────────────────────────────────────

// knownServer — l'adresse d'un serveur, et ce que la copie de son listing dit
// de lui.
//
// PAS SON JETON. Celui-ci ouvre le catalogue de quelqu'un d'autre et ne sort
// jamais de la base ; l'adresse, elle, est ce que quelqu'un a tapé pour
// l'ajouter, et la page en a besoin pour parler directement là-bas.
type knownServer struct {
	ID       int64        `json:"id"`
	Link     string       `json:"link"`
	Hostname string       `json:"hostname"`
	Listing  *peerListing `json:"listing,omitempty"`
	// Online — il a répondu à l'instant : seulement quand on vient de le lui
	// demander (handleKnownServer), jamais dans la liste, qui ne demande rien
	// à personne.
	Online *bool `json:"online,omitempty"`
	// Refused — il a répondu, et refuse le jeton du LIEN (chantier 20) : il
	// l'a révoqué. Ses jeux ne se lisent plus d'ici, quels que soient les
	// comptes de nos membres chez lui. Seulement avec Online, pour la même
	// raison.
	Refused bool `json:"refused,omitempty"`
	// Resources — ce qu'il offre à ses émulateurs de bureau, tel qu'il le
	// déclare (sessionstatus.Resources), relayé sans être relu : une partie
	// lancée chez lui joue avec. Avec Online seulement ; absent s'il ne le
	// déclare pas.
	Resources json.RawMessage `json:"resources,omitempty"`
}

// peerListing — ce que la copie du listing d'un ami dit de lui (chantier 15) :
// sa date, sa version, sa machine, et ce que notre jeton nous ouvre chez lui
// — « devices », c'est lancer ses jeux chez lui. Absent tant qu'on n'a pas
// de copie.
type peerListing struct {
	UpdatedAt time.Time        `json:"updatedAt"`
	Server    *listing.Server  `json:"server,omitempty"`
	Machine   *listing.Machine `json:"machine,omitempty"`
	Scopes    []string         `json:"scopes"`
}

// listingOf — l'en-tête de la copie d'un ami, à la forme de knownServer.
func (server *Server) listingOf(entry registeredServer) *peerListing {
	header, ok := server.copies().Header(peerOf(entry))
	if !ok {
		return nil
	}
	scopes := header.Scopes
	if scopes == nil {
		scopes = []string{}
	}

	return &peerListing{UpdatedAt: header.UpdatedAt, Server: header.Server, Machine: header.Machine, Scopes: scopes}
}

// handleKnownServer — GET /api/v1/servers/{id} : CE serveur, vérifié chez lui
// à l'instant — la date de son listing et ce que le jeton du lien nous y ouvre
// (`Copies.Check`, sans la minute), et s'il répond. La fiche d'un de ses jeux
// s'en sert avant Play : un serveur éteint, ou qui a révoqué notre lien, ne
// lance rien — et le dit (relevé par le porteur le 2026-10-13 : Play restait,
// et le lancement échouait chez lui). Un refus du jeton est une réponse : il
// répond, et ne nous ouvre plus rien.
func (server *Server) handleKnownServer(w http.ResponseWriter, r *http.Request) {
	entry, ok := server.registeredByID(w, r)
	if !ok {
		return
	}
	_, err := server.copies().Check(peerOf(entry))
	refused := errors.Is(err, remote.ErrDenied)
	online := err == nil || refused

	known := knownServer{
		ID: entry.ID, Link: entry.Link, Hostname: entry.Hostname, Listing: server.listingOf(entry),
		Online: &online, Refused: refused,
	}
	if online {
		if resources, ok := remote.PeerResources(peerOf(entry)); ok {
			known.Resources = resources
		}
	}

	writeJSON(w, http.StatusOK, known)
}

// handleKnownServers — GET /api/v1/servers.
//
// Réservée aux MEMBRES, pas aux administrateurs, et c'est la différence avec
// /api/v1/server-registry : ajouter un serveur est une décision de
// machine, emprunter une console est un geste de joueur. Un membre a besoin
// de savoir où joindre le serveur d'une console qu'on lui montre.
//
// Pourquoi une route plutôt que l'adresse dans l'URL de streaming : parce que
// la page enverrait alors le jeton de compte de CE serveur à l'adresse écrite
// dans l'URL. Une adresse forgée récolterait le jeton. L'identifiant, lui, ne
// se résout qu'ici.
func (server *Server) handleKnownServers(w http.ResponseWriter, r *http.Request) {
	registered, err := server.registeredServers()
	if err != nil {
		writeError(w, http.StatusBadGateway, "cannot read the server registry")
		return
	}

	servers := make([]knownServer, 0, len(registered))
	for _, entry := range registered {
		servers = append(servers, knownServer{
			ID: entry.ID, Link: entry.Link, Hostname: entry.Hostname, Listing: server.listingOf(entry),
		})
	}

	writeJSON(w, http.StatusOK, servers)
}
