package api

// LE CONTENU D'UNE RESSOURCE DISTANTE, tel qu'on le regarde.
//
// Trois gestes, ceux qu'on fait devant un partage qu'on vient d'ajouter :
// voir ce qu'il contient, le relire quand il a changé, et récupérer un
// élément à la main.
//
// Le troisième est le plus utile et le moins évident : jusqu'ici on ne
// pouvait prendre un BIOS ou un core chez quelqu'un d'autre qu'en le
// cherchant par le jeu qui en avait besoin. Ici on le prend directement.

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/auth"

	"gameflex/storage"
	"gameflex/storage/listing"
)

// mountsRouter — la surface du contenu d'une ressource, derrière le MÊME
// jeton que le reste : voir ce qu'il y a chez quelqu'un et en récupérer un
// fichier n'est pas public.
func (server *Server) mountsRouter() http.Handler {
	router := chi.NewRouter()
	router.Group(func(guarded chi.Router) {
		guarded.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(auth.Middleware(server.config.JWTSecret, localOnly(next.ServeHTTP)))
		})
		guarded.Get("/{id}/listing", server.handleMountListing)
		guarded.Post("/{id}/rescan", server.handleMountRescan)
		// RÉCUPÉRER UN FICHIER, à l'administrateur (#601) : le chemin est libre
		// et lisait tout le partage, pas seulement ses jeux — les documents
		// d'un NAS compris. Seul l'écran d'administration du contenu d'une
		// ressource s'en sert ; une partie lit le partage par le kernel.
		guarded.Get("/{id}/file", adminOnly(server.handleMountFetch))
		guarded.Get("/{id}/game", server.handleMountGameFiles)
		// Rapatrier un jeu dans la bibliothèque (#157) : `adminOnly` et non la
		// simple authentification de ses voisines — lire ce qu'un partage
		// contient est un geste de joueur, écrire dans la bibliothèque de tout
		// le monde en est un d'administration.
		guarded.Post("/{id}/import", adminOnly(server.refreshing(sectionNamed(listing.Roms), server.handleMountImport)))
	})

	return router
}

// handleMountListing — GET /api/v1/mounts/{id}/listing?section=roms&console=
//
// Les éléments d'une console, lus du seul fichier concerné — personne
// n'affiche vingt consoles à la fois. Le RÉSUMÉ (quand a-t-on parcouru,
// quelles consoles, un parcours tourne-t-il) n'est plus servi ici : c'est le
// sujet `mount/<id>` du centre de messages, poussé à chaque début et chaque
// fin de parcours, que la page relisait toutes les 4 s.
func (server *Server) handleMountListing(w http.ResponseWriter, r *http.Request) {
	mount, ok := server.mountByID(w, r)
	if !ok {
		return
	}

	section := strings.TrimSpace(r.URL.Query().Get("section"))

	// UN MONTAGE NE FOURNIT QUE DES ROMS (#153).
	//
	// Règle énoncée par l'utilisateur le 2026-09-17 : seuls les serveurs
	// fédérés peuvent fournir des cœurs et des BIOS — un partage Samba, un
	// dossier WebDAV ou un kDrive ne porte que des jeux. Les deux autres
	// sections étaient acceptées ici, donc un kDrive pouvait être parcouru
	// pour des BIOS, ce qui n'a pas de sens.
	//
	// La synchronisation des sauvegardes, quand elle arrivera, ajoutera une
	// section légitime — c'est le seul motif d'en rouvrir une.
	if section != "roms" {
		writeError(w, http.StatusBadRequest, "section must be roms")

		return
	}

	items, err := listing.LoadSection(
		listingCacheDir, resourceKey(mount), r.URL.Query().Get("console"), section,
	)
	if err != nil {
		// JAMAIS PARCOURUE : le premier parcours part d'ici, en fond. La liste
		// des jeux le lançait en passant, et elle ne montre plus les partages
		// (chantier 14) — c'est désormais cet écran, Réglages › Jeux, qui les
		// montre. Une console sans jeux, elle, ne relance rien.
		if !listing.Scanned(listingCacheDir, resourceKey(mount)) {
			server.startFirstScan(mount)
		}
		writeJSON(w, http.StatusOK, []listing.Item{})

		return
	}

	writeJSON(w, http.StatusOK, items)
}

// handleMountRescan — POST /api/v1/mounts/{id}/rescan.
//
// EN TÂCHE DE FOND, et ce n'est pas une facilité : parcourir un drive de dix
// mille jeux demande un aller-retour par dossier et dépasse largement les
// deux minutes (mesuré sur un vrai kDrive, 2026-09-02). Une requête qui
// attendrait la fin serait coupée avant d'aboutir, et le scan perdu avec
// elle.
//
// L'appel dit donc « c'est parti » ; l'état arrive par le sujet `mount/<id>`
// du centre de messages — `scanning` tant que ça tourne, l'erreur si ça a
// échoué —, poussé au début et à la fin.
func (server *Server) handleMountRescan(w http.ResponseWriter, r *http.Request) {
	mount, ok := server.mountByID(w, r)
	if !ok {
		return
	}

	// Un seul parcours à la fois par ressource : deux en parallèle
	// doubleraient les requêtes réseau pour écrire le même fichier.
	if !server.rescans.Start(resourceKey(mount)) {
		writeJSON(w, http.StatusOK, map[string]any{"scanning": true, "alreadyRunning": true})

		return
	}

	go func() {
		// « Relire » veut dire relire : ce qu'on avait mémorisé de
		// l'arborescence (les numéros de fichiers côté kDrive) peut ne plus
		// être vrai, et le parcours va de toute façon le réapprendre.
		forgetSource(mount)

		built, err := server.mountListing(mount, true)
		server.rescans.Finish(resourceKey(mount), err)
		if err != nil {
			slog.Warn("rescan failed", "resource", mount.Name, "error", err)

			return
		}

		slog.Info("rescan done", "resource", mount.Name, "consoles", len(built.Roms))
	}()

	writeJSON(w, http.StatusOK, map[string]any{"scanning": true})
}

// handleMountFetch — GET /api/v1/mounts/{id}/file?path=…
//
// Rend l'élément au NAVIGATEUR, en pièce jointe. C'est ce qui permet de
// récupérer un BIOS ou un core d'un serveur distant sans passer par un jeu
// qui en aurait besoin.
func (server *Server) handleMountFetch(w http.ResponseWriter, r *http.Request) {
	mount, ok := server.mountByID(w, r)
	if !ok {
		return
	}

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		writeError(w, http.StatusBadRequest, "path required")

		return
	}

	source, err := cachedSourceFor(mount)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())

		return
	}
	opener, canOpen := source.(storage.Opener)
	if !canOpen {
		writeError(w, http.StatusBadGateway, "this share cannot serve files")

		return
	}

	// Le chemin porte le nom de la source en tête — le scanner l'y met, et
	// la source, elle, part de sa propre racine.
	_, inside, found := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if !found {
		writeError(w, http.StatusBadRequest, "path must name a file inside the share")

		return
	}

	body, err := opener.Open(inside)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())

		return
	}
	defer body.Close()

	name := filepath.Base(inside)
	w.Header().Set("Content-Type", "application/octet-stream")
	// LA TAILLE, ANNONCÉE — et prise DANS LA RÉPONSE de la source.
	//
	// Sans elle la réponse part en « chunked » : le navigateur n'a rien à
	// remplir, et la barre ne s'affiche jamais. Mais la demander séparément
	// coûtait un listing du dossier parent avant chaque téléchargement
	// (2026-09-02) ; la source vient de la lire, elle la connaît.
	if size := storage.SizeOf(body); size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	// En pièce jointe et avec son nom : sans ça le navigateur affiche des
	// octets au lieu d'enregistrer un fichier.
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	if _, err := io.Copy(w, body); err != nil {
		// Trop tard pour un code d'erreur : l'en-tête est parti. On le note.
		slog.Warn("mount download cut short", "share", mount.Name, "error", err)
	}
}

// mountByID relit un partage enregistré, ou répond lui-même.
func (server *Server) mountByID(w http.ResponseWriter, r *http.Request) (mountedShare, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")

		return mountedShare{}, false
	}

	mount, found, err := server.findMount(id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())

		return mountedShare{}, false
	}
	if !found {
		writeError(w, http.StatusNotFound, "mount not found")

		return mountedShare{}, false
	}

	return mount, true
}

// findMount — un partage par son identifiant, hors de toute requête HTTP (le
// sujet `mount/<id>` du centre de messages en a besoin aussi).
//
// `find` relit le registre une fois si l'identifiant manque : un partage tout
// juste ajouté ne doit pas répondre « mount not found » (#230).
func (server *Server) findMount(id int64) (mountedShare, bool, error) {
	return server.mountRegistry.find(server.fetchRegisteredMounts, func(candidate mountedShare) bool {
		return candidate.ID == id
	})
}

// handleMountGameFiles — GET /api/v1/mounts/{id}/game?path=…&consoleId=…
//
// DE QUOI CE JEU EST FAIT. La même réponse que sur le téléphone, parce que
// c'est la même question : un jeu n'est pas toujours un fichier, et rien ne
// permettait de le savoir depuis l'interface.
//
// Elle sert à deux choses : montrer les fichiers d'un jeu dans le panneau
// d'une ressource, et vérifier — sans rien télécharger — que le partage
// répond ce qu'on croit.
func (server *Server) handleMountGameFiles(w http.ResponseWriter, r *http.Request) {
	mount, ok := server.mountByID(w, r)
	if !ok {
		return
	}

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		writeError(w, http.StatusBadRequest, "path required")

		return
	}

	game, err := mountGame(mount, strings.TrimSpace(r.URL.Query().Get("consoleId")), path)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())

		return
	}

	files := make([]map[string]any, 0, len(game.files))
	total := int64(0)
	for _, file := range game.files {
		files = append(files, map[string]any{"name": file.relative, "sizeBytes": file.size})
		total += file.size
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":       game.name,
		"folder":     game.folder,
		"files":      files,
		"totalBytes": total,
	})
}

// handleMountImport — POST /api/v1/mounts/{id}/import {consoleId, path}
//
// RAPATRIER un jeu d'un partage dans NOTRE bibliothèque (#157).
//
// C'est, depuis le chantier 14 (2026-10-12), le SEUL chemin d'un jeu de
// partage vers une partie : il ne se joue plus depuis un cache chaud, il entre
// dans le FROID — la bibliothèque —, sous le nom qu'il a sur le partage, donc
// sous le même UUID.
//
// LA FORME DU JEU VIENT DE NOUS, pas de la requête. `mountGame` la lit de notre
// propre listing (son commentaire l'énonce), donc un « dossier » choisi par le
// demandeur ne peut pas désigner un coin arbitraire du partage. La requête
// n'apporte que le chemin du jeu et sa console — et la console passe par le
// registre, parce qu'elle devient un segment de chemin sur le disque.
//
// Réservée aux administrateurs : on écrit dans la bibliothèque de tout le monde.
func (server *Server) handleMountImport(w http.ResponseWriter, r *http.Request) {
	mount, ok := server.mountByID(w, r)
	if !ok {
		return
	}

	var body struct {
		ConsoleID string `json:"consoleId"`
		Path      string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corps illisible")

		return
	}
	consoleID := strings.ToLower(strings.TrimSpace(body.ConsoleID))
	path := strings.TrimSpace(body.Path)
	if consoleID == "" || path == "" {
		writeError(w, http.StatusBadRequest, "consoleId et path sont requis")

		return
	}
	if !consoles.Exists(consoleID) {
		writeError(w, http.StatusBadRequest, "console inconnue : "+consoleID)

		return
	}

	game, err := mountGame(mount, consoleID, path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "ce partage ne l'a pas donné : "+err.Error())

		return
	}

	// UN JEU DÉJÀ LÀ N'EST PAS ÉCRASÉ, même mot que pour un cœur et pour un
	// rapatriement depuis un pair : un seul comportement à apprendre.
	target := filepath.Join(server.retroRomsRoot(), consoleID, game.name)
	if _, err := os.Stat(target); err == nil {
		writeError(w, http.StatusConflict,
			"ce jeu existe déjà : le supprimer d'abord pour en reprendre une copie")

		return
	}

	// UN JEU EST PARFOIS UN DOSSIER, et alors il faut tout prendre — un `.cue`
	// sans ses `.bin` est deux kilo-octets qui ne démarrent pas (leçon du
	// 2026-09-02).
	var total int64
	for _, file := range game.files {
		destination := target
		if game.folder {
			destination = filepath.Join(target, filepath.FromSlash(file.relative))
		}
		if err := importGameFile(file.open, destination, file.size); err != nil {
			writeError(w, http.StatusBadGateway, "copie interrompue : "+err.Error())

			return
		}
		total += file.size
	}

	// `sizeBytes` est ce que le partage ANNONÇAIT : c'est la seule taille qu'on
	// connaisse avant d'avoir tout relu, et `importGameFile` l'a déjà comparée
	// fichier par fichier.
	writeJSON(w, http.StatusOK, map[string]any{
		"consoleId": consoleID, "name": game.name, "folder": game.folder,
		"files": len(game.files), "sizeBytes": total,
	})
}
