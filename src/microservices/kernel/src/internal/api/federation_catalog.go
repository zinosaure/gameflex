// Lire les jeux d'un serveur fédéré — chantier 6, revu aux chantiers 14 et 15.
//
// LES JEUX D'UN PAIR NE SE MÊLENT PLUS AUX NÔTRES (2026-10-12). Ils
// apparaissaient dans la liste d'ici, à leur place sous leur console, et se
// jouaient ici après une copie dans un cache. Ce cache était une seconde
// bibliothèque : « mettre en cache dans gameflex/cache/roms c'est exactement
// comme mettre dans gameflex/roms » (le porteur). Désormais la liste montre la
// bibliothèque, et son sélecteur de source montre UN pair à la fois
// (`servePeerRoms`) ; un jeu de chez lui se télécharge, s'ajoute à la
// bibliothèque par Réglages › Jeux, ou se joue chez lui.
//
// Deux pièges, tous deux réels, tous deux désamorcés ici.
//
//  1. LES RÉFÉRENCES SE COGNENT. Une référence de jeu est dérivée de la console
//     et du NOM SUR LE DISQUE, extension comprise (storage.RomID) : deux
//     serveurs qui rangent le même jeu sous le MÊME nom de fichier portent la
//     même référence. D'où le préfixe : un jeu de chez lui s'appelle
//     "srv3:<référence>". Il ne peut pas être confondu avec un jeu d'ici, et le
//     chemin local le refuse naturellement au lieu de servir un homonyme.
//
//  2. UN LISTING COÛTE CHER. Celui de cette machine pèse plusieurs mégaoctets
//     pour 23 000 jeux. Il se COPIE donc chez nous (`remote.Copies`, partagé
//     avec le téléphone, chantier 15) : à l'ajout de l'ami, par l'icône, et
//     quand sa date avance — demandée au plus une fois par minute.
//
// UN REFUS N'EST PAS UNE PANNE. Un pair qui refuse notre jeton le dit
// franchement — `remote.ErrDenied` —, et la page affiche « ce serveur
// n'autorise pas l'accès » au lieu d'une liste vide ; un pair éteint se dit
// autrement.
package api

import (
	"net/http"
	"strconv"
	"strings"

	"gameflex/kernel/internal/retro_files"
	"gameflex/storage"
	"gameflex/storage/gamestatus"
	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

// remotePrefix — ce qui distingue une référence distante d'une locale.
const remotePrefix = "srv"

// remoteReference — la référence telle qu'elle voyage dans NOTRE liste.
func remoteReference(serverID int64, reference string) string {
	return remotePrefix + strconv.FormatInt(serverID, 10) + ":" + reference
}

// parseRemoteReference défait le préfixe. ok est faux pour une référence
// locale, ce qui est le cas normal et pas une erreur.
func parseRemoteReference(reference string) (serverID int64, remote string, ok bool) {
	if !strings.HasPrefix(reference, remotePrefix) {
		return 0, "", false
	}
	rest := strings.TrimPrefix(reference, remotePrefix)
	colon := strings.Index(rest, ":")
	if colon <= 0 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(rest[:colon], 10, 64)
	if err != nil {
		return 0, "", false
	}

	return id, rest[colon+1:], true
}

// copies — les copies des listings des amis, rangées avec les autres listings.
// Le dossier se fixe au premier usage : les tests posent `listingCacheDir`
// avant de construire leur serveur.
func (server *Server) copies() *remote.Copies {
	server.copiesOnce.Do(func() { server.peerCopies.Dir = listingCacheDir })

	return &server.peerCopies
}

// peerOf — un serveur enregistré, tel que les copies le lisent.
func peerOf(entry registeredServer) remote.Peer {
	return remote.Peer{ID: entry.ID, Link: entry.Link, Token: entry.Token}
}

// peerRomEntry — un jeu d'un pair, sous la forme des nôtres : la liste n'a
// qu'une sorte d'entrée, et l'origine se lit dans `Kind`/`ServerID`/`Source`.
//
// Son statut est celui que l'ami a établi chez lui, sur sa machine
// (chantier 30) : ses signalements, et le statut de sa console.
func peerRomEntry(entry registeredServer, source, consoleID, consoleStatus string, item listing.Item) retro_files.RomEntry {
	// Jamais nil : une liste nulle fait tomber la liste des jeux (le piège
	// « System Volume Information » du 2026-09-03).
	files := item.Files
	if files == nil {
		files = []string{}
	}
	view := gamestatus.Resolve(consoleID, consoleStatus, item.Reports)

	return retro_files.RomEntry{
		Reference: remoteReference(entry.ID, item.ID),
		Title:     item.Name,
		Files:     files,
		Folder:    item.Dir,
		SizeBytes: item.Size,
		Source:    firstNonEmpty(source, entry.Hostname),
		Kind:      storage.KindServer,
		Identity:  entry.Identity,
		ServerID:  entry.ID,
		Status:    &view,
		Engine:    item.Engine,
	}
}

// servePeerRoms — les jeux d'UN pair pour UNE console : ce que montre la liste
// quand son sélecteur de source désigne ce serveur (chantier 14), lus dans la
// copie de son listing (chantier 15).
//
// Ouvert à tous les comptes, comme la liste elle-même : les droits ne changent
// pas — le listing d'un pair se lit avec le jeton que l'administrateur a
// enregistré, et jouer chez lui demande d'y être connecté.
//
// 403 quand il refuse notre jeton, 502 quand il ne répond pas : la page dit
// alors pourquoi il n'y a rien, au lieu d'une liste vide qui ne dit rien.
func (server *Server) servePeerRoms(w http.ResponseWriter, rawID, consoleID, query string) {
	entry, ok := server.registeredByIDValue(w, rawID)
	if !ok {
		return
	}

	peer := peerOf(entry)
	items, err := server.copies().Items(peer, listing.Roms, consoleID)
	if err != nil {
		refused := remote.Refusal(err)
		writeError(w, refused.Status, refused.Message)

		return
	}
	header, _ := server.copies().Header(peer)

	// Par titre : la copie l'est déjà, comme tout listing.
	needle := strings.ToLower(strings.TrimSpace(query))
	roms := []retro_files.RomEntry{}
	for _, item := range items {
		if needle != "" && !strings.Contains(strings.ToLower(item.Name), needle) {
			continue
		}
		roms = append(roms, peerRomEntry(entry, header.Name, consoleID, header.Statuses[consoleID], item))
	}

	writeJSON(w, http.StatusOK, roms)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
