package remote

// L'ARCHIVE DES MÉTADONNÉES D'UN JEU D'UN PAIR (le Catalogue des Réglages ; le porteur, 2026-11-16 : « télécharger le
// metadata ») — `GET /api/v1/federation/metadata/{consoleId}/{ref}/archive` chez lui : le dossier de sa fiche, tel qu'il
// le garde, en .zip. Le kernel qui affiche le jeu la relaie au NAVIGATEUR qui la télécharge (RelayMetadataArchive) :
// rien ne s'écrit ici, et le jeton du pair ne sort pas du kernel. Ses refus sont ceux de la fiche (fetchMetadata,
// metadata.go) : notre jeton refusé, 403 ; un jeu sans métadonnées, ou un pair qui n'a pas cette route, 404 ; muet, 502.

import (
	"context"
	"errors"
	"io"
	"net/http"
)

// metadataArchivePath — l'archive du jeu `reference` de la console chez le pair.
func metadataArchivePath(consoleID, reference string) string {
	return metadataPath(consoleID, reference) + "/archive"
}

// metadataRefusal — le refus d'une lecture des fiches, tel que la page le lit (relayMetadata).
func metadataRefusal(err error) *StatusError {
	switch {
	case errors.Is(err, ErrDenied):
		return refusedAccess
	case errors.Is(err, ErrNotLent):
		return notLent
	}

	return notAnswering
}

// CheckMetadataArchive — LE PAIR PRÊTE-T-IL L'ARCHIVE DE CE JEU ? Demandé avant d'émettre le ticket du navigateur : un
// lien qui ne mène à rien ne s'émet pas. Rend nil, ou le refus à répondre (le corps n'est pas lu : le pair cesse de
// l'écrire quand la lecture s'arrête).
func CheckMetadataArchive(ctx context.Context, peer Peer, consoleID, reference string) *StatusError {
	client, err := New(peer.Link, peer.Token)
	if err != nil {
		return notAnswering
	}
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	response, err := client.fetchMetadata(ctx, metadataArchivePath(consoleID, reference), "")
	if err != nil {
		return metadataRefusal(err)
	}
	response.Body.Close()

	return nil
}

// RelayMetadataArchive — L'ARCHIVE, relayée telle quelle : son type, sa taille, son nom (`Content-Disposition`). Le
// transfert n'a pas d'échéance (des images pèsent) : la requête du navigateur le borne — l'abandonner coupe aussi la
// lecture chez le pair. Une *StatusError rendue AVANT le premier octet est à répondre par l'appelant ; une autre erreur
// arrive après : il n'y a plus qu'à la noter.
func RelayMetadataArchive(w http.ResponseWriter, r *http.Request, peer Peer, consoleID, reference string) error {
	client, err := New(peer.Link, peer.Token)
	if err != nil {
		return notAnswering
	}
	response, err := client.fetchMetadata(r.Context(), metadataArchivePath(consoleID, reference), "")
	if err != nil {
		return metadataRefusal(err)
	}
	defer response.Body.Close()

	for _, header := range []string{"Content-Type", "Content-Length", "Content-Disposition", "Last-Modified"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return nil
	}
	_, err = io.Copy(w, response.Body)

	return err
}
