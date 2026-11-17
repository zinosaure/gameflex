package remote

// LA DISPONIBILITÉ PAR SOURCE, DEMANDÉE À LA SOURCE (chantier 53, P5, #672) — « Le récupérer », sur la page d'un
// émulateur ou d'un BIOS absent : la source officielle (un cœur libretro chez le buildbot, un émulateur chez son
// éditeur), puis chaque serveur fédéré, grisés quand ils ne l'ont pas, avec leur raison. Le même geste dans les deux
// kernels : le serveur (storage_availability.go) et le téléphone (le porteur, 2026-11-16 : « dans console/emulateur
// on peut pas récupérer depuis les serveurs fédérés. On ne voit même pas les serveurs »). Chaque kernel dit OÙ la
// source officielle publie le fichier, et QUEL fichier il prendrait chez un pair (le serveur : le cœur Linux ; le
// téléphone : sa variante Android, ou son paquet d'émulateur) ; la manière de demander est ici.
//
// Chaque source est interrogée au moment où la page s'ouvre : un HEAD chez la source officielle ; chez un pair, le
// fichier lui-même, demandé avec notre jeton, sa réponse refermée dès qu'elle commence (ce que le pair refuse ou ne
// connaît pas se dit alors avec sa raison). Chacune dans un délai borné.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AvailabilityTimeout — ce qu'une source a pour répondre.
const AvailabilityTimeout = 10 * time.Second

// Availability — une source, et si elle a le fichier.
type Availability struct {
	// Source : `official`, ou `server` (un serveur fédéré, ServerID et Name).
	Source    string `json:"source"`
	ServerID  int64  `json:"serverId,omitempty"`
	Name      string `json:"name,omitempty"`
	Available bool   `json:"available"`
	// Reason : `unreachable`, `denied`, `answered <statut>`, ou l'erreur du pair.
	Reason string `json:"reason,omitempty"`
}

// ProbeOfficial — un HEAD à l'adresse où la source officielle publie le fichier : 200 pour « disponible ».
func ProbeOfficial(ctx context.Context, url string) Availability {
	answer := Availability{Source: "official"}
	ctx, cancel := context.WithTimeout(ctx, AvailabilityTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		answer.Reason = err.Error()

		return answer
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		answer.Reason = "unreachable"

		return answer
	}
	response.Body.Close()
	answer.Available = response.StatusCode == http.StatusOK
	if !answer.Available {
		answer.Reason = fmt.Sprintf("answered %d", response.StatusCode)
	}

	return answer
}

// ProbePeer — le fichier demandé au pair (`open` : OpenCore, OpenAndroidCore, OpenEmulatorFile, OpenBios…), sa réponse
// refermée dès qu'elle commence. Le client du pair n'a pas de contexte : le délai se tient ici, et une réponse arrivée
// après lui se referme quand même. `name` : le nom du pair, tel que la page le montre.
func ProbePeer(peer Peer, name string, open func(*Server) (io.ReadCloser, error)) Availability {
	answer := Availability{Source: "server", ServerID: peer.ID, Name: name}
	client, err := New(peer.Link, peer.Token)
	if err != nil {
		answer.Reason = err.Error()

		return answer
	}
	type opened struct {
		body io.ReadCloser
		err  error
	}
	result := make(chan opened, 1)
	go func() {
		body, err := open(client)
		result <- opened{body, err}
	}()

	select {
	case got := <-result:
		if got.err != nil {
			answer.Reason = got.err.Error()
			if errors.Is(got.err, ErrDenied) {
				answer.Reason = "denied"
			}

			return answer
		}
		got.body.Close()
		answer.Available = true
	case <-time.After(AvailabilityTimeout):
		answer.Reason = "unreachable"
		go func() {
			if got := <-result; got.body != nil {
				got.body.Close()
			}
		}()
	}

	return answer
}
