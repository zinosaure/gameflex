package remote

// LES RESSOURCES D'UN PAIR (2026-10-23) — ce qu'il offre à ses émulateurs de
// bureau, tel qu'il le déclare dans /api/v1/capabilities (`resources`, voir
// sessionstatus.Resources). Un kernel les RELAIE à la page sans les relire :
// c'est la déclaration du pair, pas la nôtre.

import "encoding/json"

// Resources — les ressources que ce serveur déclare (une route publique).
// Faux s'il ne répond pas ou n'en déclare pas.
func (server *Server) Resources() (json.RawMessage, bool) {
	response, err := server.list("/api/v1/capabilities")
	if err != nil {
		return nil, false
	}
	defer response.Body.Close()

	var body struct {
		Capabilities struct {
			Resources json.RawMessage `json:"resources"`
		} `json:"capabilities"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || len(body.Capabilities.Resources) == 0 || string(body.Capabilities.Resources) == "null" {
		return nil, false
	}

	return body.Capabilities.Resources, true
}

// PeerResources — les ressources d'un pair, par son lien.
func PeerResources(peer Peer) (json.RawMessage, bool) {
	server, err := New(peer.Link, peer.Token)
	if err != nil {
		return nil, false
	}

	return server.Resources()
}
