package api

// LE RELAIS DE LA FÉDÉRATION (chantier 83 ; le porteur, 2026-11-26 : « Quand on donne accès à notre serveur, on coche
// si on veut ou pas faire le relais des serveurs fédérés sur notre serveur. Le mieux ce serait par serveur »).
//
// Le porteur d'un accès d'ici voit aussi les serveurs fédérés qu'on lui relaie, un par un — à deux conditions, des
// deux côtés : cet accès les relaie (`relayed_servers`, choisi ici), et chacun d'eux nous le permet (« Peut me
// relayer », sur l'accès qu'il nous a donné : `relayAllowed` dans l'en-tête de la copie de son listing).
//
//   - SERVIR : l'en-tête de notre listing nomme ce qu'on relaie à ce porteur (`Header.Relayed`) ; ses demandes passent
//     par `/api/v1/federation/relay/{id}/…`, que ce fichier repasse au serveur d'origine avec NOTRE jeton chez lui —
//     les routes du catalogue seulement (voir et télécharger : le porteur, réponse 2), jamais une partie ni une console
//     Remote Play (réponse 3). Le porteur ne connaît ni l'adresse ni le jeton de l'origine.
//   - LIRE : un serveur qu'un ami nous relaie devient un pair VIRTUEL de notre registre (`relayedPeers`) — son adresse
//     est celle de l'ami suivie du préfixe du relais, son jeton celui qu'on a chez l'ami —, que le catalogue, les fiches
//     et les correctifs lisent comme un pair direct. Jouer et le Remote Play ne lisent que les pairs directs
//     (`directServers`).
//   - UN SEUL SAUT : ce qu'on relaie ne repart jamais plus loin — la copie relayée perd ses propres serveurs relayés,
//     et ses consoles (« rien à lancer » : y jouer demande un compte chez l'origine).

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"gameflex/storage/listing"
	"gameflex/storage/remote"

	"github.com/go-chi/chi/v5"
)

// relayedFor — les serveurs que l'accès du porteur relaie, et qui nous le permettent : ce que l'en-tête de notre
// listing lui nomme. Nil : aucun.
func (server *Server) relayedFor(r *http.Request) []listing.RelayedServer {
	ids, _ := r.Context().Value(tokenRelayedKey{}).([]int64)
	if len(ids) == 0 {
		return nil
	}
	registered, err := server.directServers()
	if err != nil {
		return nil
	}
	var relayed []listing.RelayedServer
	for _, entry := range registered {
		if !slices.Contains(ids, entry.ID) {
			continue
		}
		if header, ok := server.copies().Header(peerOf(entry)); ok && header.RelayAllowed {
			relayed = append(relayed, listing.RelayedServer{ID: entry.ID, Name: entry.Hostname})
		}
	}

	return relayed
}

// handleFederationRelay — GET|HEAD /api/v1/federation/relay/{id}/* : une route du catalogue d'un serveur qu'on relaie
// au porteur, repassée à ce serveur avec notre jeton. 403 quand cet accès ne le relaie pas (ou qu'il ne le permet
// plus), 404 pour une route qui ne se relaie pas, 502 quand l'origine ne répond pas.
func (server *Server) handleFederationRelay(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	path := "/" + chi.URLParam(r, "*")
	// Un pair virtuel (remote.RelayLink) demande le chemin entier, `/api/v1/federation/listing` : sous le relais, il
	// redevient `/listing`.
	path = strings.TrimPrefix(path, "/api/v1/federation")
	if err != nil || !remote.Relayable(path) {
		writeErrorCode(w, http.StatusNotFound, "not_found", "this route is not relayed")
		return
	}
	if !slices.ContainsFunc(server.relayedFor(r), func(one listing.RelayedServer) bool { return one.ID == id }) {
		writeErrorCode(w, http.StatusForbidden, "relay_refused", "this access does not relay that server")
		return
	}
	registered, _ := server.directServers()
	index := slices.IndexFunc(registered, func(entry registeredServer) bool { return entry.ID == id })
	if index < 0 {
		writeErrorCode(w, http.StatusForbidden, "relay_refused", "this access does not relay that server")
		return
	}
	origin, err := remote.New(registered[index].Link, registered[index].Token)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "peer_unreachable", err.Error())
		return
	}
	target := "/api/v1/federation" + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	response, err := origin.Forward(r.Context(), r.Method, target)
	if errors.Is(err, remote.ErrDenied) {
		writeErrorCode(w, http.StatusForbidden, "relay_refused", "the relayed server refuses our token")
		return
	}
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "peer_unreachable", err.Error())
		return
	}
	defer response.Body.Close()

	if (path == "/listing" || path == "/listing/updated") && r.Method == http.MethodGet {
		server.writeRelayedListing(w, path, response.Body)
		return
	}
	for _, header := range []string{"Content-Type", "Content-Length", "Content-Disposition", "Last-Modified", "ETag", "Cache-Control"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, response.Body)
	}
}

// writeRelayedListing — le listing (ou la fraîcheur) de l'origine, réécrit par remote.RelayedListing.
func (server *Server) writeRelayedListing(w http.ResponseWriter, path string, body io.Reader) {
	data, err := remote.RelayedListing(path, body)
	if err != nil {
		writeErrorCode(w, http.StatusBadGateway, "peer_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// relayedPeers — les serveurs que nos amis directs nous relaient (l'en-tête de la copie de leur listing), comme des
// pairs de notre registre : un identifiant négatif (remote.RelayID), l'adresse du relais suivie du préfixe, notre
// jeton chez lui, et le nom de l'ami qui relaie.
func (server *Server) relayedPeers(direct []registeredServer) []registeredServer {
	var relayed []registeredServer
	for _, entry := range direct {
		header, ok := server.copies().Header(peerOf(entry))
		if !ok {
			continue
		}
		for _, one := range header.Relayed {
			relayed = append(relayed, registeredServer{
				ID: remote.RelayID(entry.ID, one.ID), Link: remote.RelayLink(entry.Link, one.ID), Token: entry.Token,
				Hostname: one.Name, Identity: entry.Identity, RelayedBy: entry.Hostname,
			})
		}
	}

	return relayed
}

// directServers — les pairs directs seulement : jouer, le Remote Play et les invitations ne passent jamais par un
// relais.
func (server *Server) directServers() ([]registeredServer, error) {
	all, err := server.registeredServers()

	return slices.DeleteFunc(slices.Clone(all), func(entry registeredServer) bool { return entry.RelayedBy != "" }), err
}
