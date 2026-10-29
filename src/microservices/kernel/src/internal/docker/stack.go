package docker

import "github.com/docker/docker/api/types/filters"

// labelStack — la pile à laquelle appartient un conteneur de partie.
//
// DEUX KERNELS SUR UN MÊME DÉMON SE VOLAIENT LEURS PARTIES. Adoption, balayage
// des conteneurs sortis et reprise au redémarrage ne filtraient que sur
// `gameflex.session.id` : un second kernel — une pile de release lancée à côté
// de la pile de dev pour la tester — aurait adopté les parties en cours de
// l'autre comme les siennes, puis les aurait arrêtées à la première inactivité
// qu'il croyait voir (relevé le 2026-09-15, avant le premier essai).
const labelStack = "gameflex.stack"

// sessionFilters — les conteneurs de partie de CETTE pile, et seulement eux.
// Toute liste de conteneurs de partie passe par ici ; un filtre écrit à la
// main ailleurs rouvrirait le vol.
func (dockerClient *Client) sessionFilters(extra ...filters.KeyValuePair) filters.Args {
	args := filters.NewArgs(
		filters.Arg("label", labelSessionID),
		filters.Arg("label", labelStack+"="+dockerClient.config.StackName),
	)
	for _, pair := range extra {
		args.Add(pair.Key, pair.Value)
	}

	return args
}
