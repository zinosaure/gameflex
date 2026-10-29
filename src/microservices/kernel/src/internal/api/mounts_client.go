package api

// Les partages montés (WebDAV, kDrive, SMB) vus comme des jeux de la
// bibliothèque — chantier « ressources externes ».
//
// Le pendant de federation_catalog.go, et la différence tient en une phrase :
// un serveur GameFlex a DÉJÀ classé ses jeux et rend un catalogue, un partage
// est un arbre de dossiers qu'il faut scanner. Le scan lui-même n'est pas
// réécrit — c'est celui de shared/storage, le même que le téléphone utilise
// pour son propre stockage, et la source WebDAV s'y branche sans qu'il
// change d'une ligne.
//
// Le résultat entre dans la liste d'une console À CÔTÉ des jeux d'ici, avec
// sa mention d'origine, comme les jeux fédérés : on cherche un jeu, pas un
// protocole.

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"gameflex/kernel/internal/config"
	"gameflex/storage"
	"gameflex/storage/kdrive"
	"gameflex/storage/listing"
	"gameflex/storage/smb"
	"gameflex/storage/webdav"
)

type mountedShare struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	BaseURL  string `json:"base_url"`
	Folder   string `json:"folder"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// fetchRegisteredMounts — les partages enregistrés, mots de passe COMPRIS.
//
// Route interne du service d'identité : c'est le kernel qui monte, et il ne
// peut pas le faire avec un mot de passe qu'on lui cache. La liste
// d'administration, elle, ne les rend jamais. Mémorisée dix secondes par
// `mountRegistry` (#190), même raison que son jumeau côté fédération.
func (server *Server) fetchRegisteredMounts() ([]mountedShare, error) {
	if server.config.CredentialURL == "" || server.config.InternalSecret == "" {
		return nil, nil
	}

	request, err := http.NewRequest(http.MethodGet, server.config.CredentialURL+"/internal/storage/mounts", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)

	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, errors.New("cannot read the mount registry: " + response.Status)
	}

	var envelope struct {
		Items []mountedShare `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	forgetDepartedResources(envelope.Items)

	return envelope.Items, nil
}

// forgetDepartedResources — une ressource retirée emporte son listing et sa
// source ouverte (chantier 15) : le registre relu est celui qui fait foi. Un
// listing pris dans la minute reste (listing.Prune), et ceux d'un format
// d'avant partent avec.
func forgetDepartedResources(mounts []mountedShare) {
	kept := map[string]bool{}
	for _, mount := range mounts {
		kept[resourceKey(mount)] = true
	}
	for _, key := range listing.Prune(listingCacheDir, kept, isResourceListing) {
		slog.Info("listing of a departed resource removed", "key", key)
	}

	sourcesMu.Lock()
	departed := []storage.Source{}
	for key, source := range sourcesByID {
		if !kept[key] {
			departed = append(departed, source)
			delete(sourcesByID, key)
		}
	}
	sourcesMu.Unlock()
	for _, source := range departed {
		closeSource(source)
	}
}

// isResourceListing — le listing d'une ressource : ni celui de ce serveur, ni
// la copie de celui d'un ami.
func isResourceListing(header listing.Header) bool {
	return header.Source != listing.SelfSource && header.Source != listing.PeerSource
}

// sourceFor — la source de bibliothèque d'un partage.
// UNE SOURCE PAR PARTAGE, GARDÉE.
//
// Un kDrive n'adresse pas ses fichiers par chemin mais par NUMÉRO : ouvrir
// un fichier suppose donc de savoir lequel, et sans mémoire il faut lister
// tout le dossier parent pour le trouver — 10 783 entrées paginées pour une
// cartouche NES de 41 Ko. On construisait une source NEUVE à chaque requête,
// donc cette mémoire repartait vide à chaque fois : trente-quatre secondes
// pour ouvrir un fichier de quarante kilo-octets, et un délai dépassé côté
// application (signalé le 2026-09-02).
//
// Gardée par clé de RESSOURCE, pas par nom : c'est la même règle que le cache
// de listings — un drive supprimé puis remplacé sous le même nom ne doit pas
// hériter des numéros de l'autre.
var (
	sourcesMu   sync.Mutex
	sourcesByID = map[string]storage.Source{}
)

func cachedSourceFor(mount mountedShare) (storage.Source, error) {
	key := resourceKey(mount)

	sourcesMu.Lock()
	defer sourcesMu.Unlock()

	if kept, ok := sourcesByID[key]; ok {
		return kept, nil
	}
	source, err := sourceFor(mount)
	if err != nil {
		return nil, err
	}
	// CE QU'ELLE AVAIT APPRIS, rendu tout de suite : sans quoi le premier
	// téléchargement de chaque console repaie la marche chemin→numéro
	// entière après chaque redémarrage.
	listing.LoadMemo(listingCacheDir, key, source)
	sourcesByID[key] = source

	return source, nil
}

// forgetSource — après un « Relire » ou une modification du partage : ce
// qu'on avait mémorisé de son arborescence peut ne plus être vrai.
func forgetSource(mount mountedShare) {
	key := resourceKey(mount)

	sourcesMu.Lock()
	kept := sourcesByID[key]
	delete(sourcesByID, key)
	sourcesMu.Unlock()

	closeSource(kept)
}

// closeSource — une source qui TIENT une session (SMB) la rend. Sans quoi
// chaque « Relire » laissait une connexion ouverte jusqu'à ce que le serveur
// s'en lasse. La source attend elle-même la fin de ses lectures en cours :
// un téléchargement lancé juste avant n'est pas coupé.
func closeSource(source storage.Source) {
	if closer, ok := source.(io.Closer); ok {
		_ = closer.Close()
	}
}

func sourceFor(mount mountedShare) (storage.Source, error) {
	switch mount.Kind {
	case "kdrive":
		// kDrive parle son API REST, pas WebDAV : il ne l'ouvre qu'aux offres
		// Team et Pro, et sur les autres le drive refuse tout sans même
		// demander qui vous êtes. L'adresse enregistrée est ici le NUMÉRO du
		// drive, et le « mot de passe » un jeton d'API.
		return kdrive.New(kdrive.Config{
			Name:    mount.Name,
			DriveID: mount.BaseURL,
			Token:   mount.Password,
			Folder:  mount.Folder,
		})
	case "webdav":
		return webdav.New(webdav.Config{
			Name: mount.Name,
			// LE DOSSIER FAIT PARTIE DE L'ADRESSE.
			//
			// Le scanner part toujours de la racine de la source : lui donner
			// un partage entier revenait à parcourir un drive complet —
			// photos et documents compris — pour y chercher des jeux. C'est
			// long, et ça ne trouve rien si les jeux sont rangés ailleurs.
			BaseURL:  shareRoot(mount),
			Username: mount.Username,
			Password: mount.Password,
		})
	case "smb":
		// L'adresse est `smb://hôte:port/partage`, composée à
		// l'enregistrement. Le dossier reste À PART, comme pour kDrive : un
		// partage SMB se monte par son nom, pas par un chemin.
		return smb.New(smb.Config{
			Name:     mount.Name,
			Address:  mount.BaseURL,
			Folder:   mount.Folder,
			Username: mount.Username,
			Password: mount.Password,
		})
	default:
		return nil, errors.New("unsupported mount kind: " + mount.Kind)
	}
}

// resourceKey — l'identité de cache d'un partage.
//
// Dérivée de CE QU'IL EST, jamais de son nom : le nom est choisi par son
// propriétaire et peut être réutilisé, si bien qu'un nouveau drive héritait
// du cache de l'ancien (voir listing.Key).
func resourceKey(mount mountedShare) string {
	return listing.Key(mount.Kind, mount.BaseURL, mount.Folder)
}

// shareRoot — l'adresse à partir de laquelle on scanne.
//
// La composition elle-même a déménagé dans `storage.ShareRoot` : elle vivait
// ici, non exportée, et les deux chemins du téléphone ne l'appliquaient pas —
// un montage WebDAV avec un dossier y scannait le partage entier. Une règle,
// un endroit ; ce wrapper ne fait plus que dénouer la structure.
func shareRoot(mount mountedShare) string {
	return storage.ShareRoot(mount.BaseURL, mount.Folder)
}

// listingCacheDir — où le cache des listings vit : `cache/` du dossier.
var listingCacheDir = filepath.Join(config.StorageDir, "cache", "listing")

// startFirstScan lance le premier parcours d'une ressource, une seule fois.
func (server *Server) startFirstScan(mount mountedShare) {
	if !server.rescans.Start(resourceKey(mount)) {
		return
	}

	slog.Info("first scan of a resource", "resource", mount.Name)
	go func() {
		_, err := server.mountListing(mount, true)
		server.rescans.Finish(resourceKey(mount), err)
		if err != nil {
			slog.Warn("first scan failed", "resource", mount.Name, "error", err)
		}
	}()
}

// mountListing rend le listing d'un partage, du CACHE si on en a un.
//
// Le réseau n'est touché qu'au premier appel, ou sur demande explicite
// (rescan) : parcourir un drive coûte un aller-retour par dossier, et le
// repayer à chaque affichage rendait la liste des jeux lente pour un contenu
// qui n'avait pas bougé (signalé le 2026-09-02).
func (server *Server) mountListing(mount mountedShare, force bool) (listing.Listing, error) {
	if !force {
		if cached, err := listing.Load(listingCacheDir, resourceKey(mount)); err == nil {
			return cached, nil
		}
	}

	// La source GARDÉE, pas une jetable : le parcours voit tous les fichiers
	// du partage, et c'est lui qui doit réchauffer la mémoire des numéros —
	// sinon elle est jetée avec la source dès la fin du scan.
	source, err := cachedSourceFor(mount)
	if err != nil {
		return listing.Listing{}, err
	}

	built, err := listing.Build(source, time.Now())
	if err != nil {
		return listing.Listing{}, err
	}
	if err := listing.Save(listingCacheDir, resourceKey(mount), built); err != nil {
		// Écrire le cache peut échouer (disque plein, droits) sans que le
		// listing lui-même soit faux : on le rend quand même, et on le dit.
		slog.Warn("cannot write the listing cache", "resource", mount.Name, "error", err)
	}
	// APRÈS le catalogue, jamais avant : listing.Save efface le dossier de la
	// ressource avant d'écrire, et emporterait cette mémoire avec.
	//
	// Ce que la source vient d'apprendre en parcourant est le même travail
	// que celui qu'on refaisait à chaque redémarrage — une demi-minute par
	// console, avant le premier octet.
	if err := listing.SaveMemo(listingCacheDir, resourceKey(mount), source); err != nil {
		slog.Warn("cannot write the resource memo", "resource", mount.Name, "error", err)
	}

	return built, nil
}
