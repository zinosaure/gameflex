package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
)

// LA BOUCLE DE LANCEMENT, écrite une fois (#48, 2026-09-14).
//
// Streamex, rétro et jeu à distance la recopiaient à l'identique : trois
// fonctions de 558, 224 et 140 lignes dont le squelette était le même au mot
// près — réserver un port TCP (et, jusqu'à #257, un bloc UDP pour les pairs
// pion de chaque partie), créer le conteneur sous le premier numéro libre, le
// démarrer, recommencer sur les deux conflits que le démon sait signaler. Le script qui l'a extraite a d'abord vérifié que les
// trois copies étaient identiques, instruction par instruction.
//
// Ce qui diffère réellement d'un genre de session à l'autre — environnement,
// montages, image, capacité — reste dans chaque CreateAndStart* : c'est
// `build`, à qui la boucle donne le port qu'elle vient de réserver.
// Le store de session avait été unifié avant, et la couche de lancement
// laissée de côté avec sa raison écrite : ces réglages divergent vraiment. Ce
// fichier n'y touche pas — seulement à ce qui, lui, ne divergeait pas.

// spawnLoop — ce dont la boucle a besoin. Les appels au démon sont des
// fonctions pour qu'un test puisse jouer le démon ; l'allocateur est le vrai,
// parce que sa comptabilité est précisément ce qui doit rester juste.
type spawnLoop struct {
	create func(name string, config *container.Config, host *container.HostConfig) (id string, err error)
	start  func(id string) error
	remove func(id string)

	ports *PortAllocator

	onStarted      func(containerName string)
	onImageMissing func(image string) bool // vrai : une reconstruction est partie
	portRange      string
}

// spawn — la boucle branchée sur le vrai démon.
func (dockerClient *Client) spawn(ctx context.Context) spawnLoop {
	return spawnLoop{
		create: func(name string, config *container.Config, host *container.HostConfig) (string, error) {
			created, err := dockerClient.dockerClient.ContainerCreate(ctx, config, host, nil, nil, name)

			return created.ID, err
		},
		start: func(id string) error {
			return dockerClient.dockerClient.ContainerStart(ctx, id, container.StartOptions{})
		},
		remove: func(id string) {
			_ = dockerClient.dockerClient.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
		},
		ports: dockerClient.allocator,
		// Sa voix, gardée sur le disque tant qu'il parle.
		onStarted:      dockerClient.FollowLogs,
		onImageMissing: dockerClient.EnsureImageBuilt,
		portRange:      dockerClient.config.PortRange,
	}
}

// run lance UN conteneur nommé « <base>-<nnn> », en partant du numéro index
// (1 au minimum). Il rend le port publié et le nom réellement obtenu — pas
// forcément celui demandé.
func (loop spawnLoop) run(base string, index int, build func(port int) (*container.Config, *container.HostConfig)) (port int, containerName string, err error) {
	nameIndex := index
	if nameIndex < 1 {
		nameIndex = 1
	}
	containerName = fmt.Sprintf("%s-%03d", base, nameIndex)

	for attempt := 0; attempt < loop.ports.size(); attempt++ {
		candidatePort, err := loop.ports.Allocate()
		if err != nil {
			return 0, "", err
		}

		containerConfig, hostConfig := build(candidatePort)

		id, err := loop.create(containerName, containerConfig, hostConfig)
		if err != nil {
			if isNameConflict(err) && nameIndex < 999 {
				// Deux lancements ont pris le même numéro, ou un conteneur
				// arrêté le garde encore : on passe au suivant.
				nameIndex++
				containerName = fmt.Sprintf("%s-%03d", base, nameIndex)
				continue
			}
			if isImageMissing(err) {
				// Lance une reconstruction en arrière-plan (EnsureImageBuilt
				// déduplique avec la passe de CheckImages au démarrage, et avec
				// un second joueur tombé sur la même image) : ce lancement
				// échoue maintenant, mais le SUIVANT — le 503 dit au joueur de
				// réessayer — a une vraie chance d'aboutir quelques minutes plus
				// tard, au lieu d'exiger à jamais un `docker compose build`.
				// Sans sources — une installation publique —, rien ne la
				// reconstruira : l'erreur dit qu'elle n'est pas installée (#136).
				if loop.onImageMissing(containerConfig.Image) {
					return 0, "", fmt.Errorf("%w: %s", ErrImageMissing, containerConfig.Image)
				}
				return 0, "", fmt.Errorf("%w: %s", ErrImageNotInstalled, containerConfig.Image)
			}
			return 0, "", fmt.Errorf("create container: %w", err)
		}

		if err := loop.start(id); err != nil {
			loop.remove(id)
			if isPortConflict(err) {
				// Le port est pris côté hôte : il est banni, et on recommence
				// avec un neuf.
				loop.ports.blacklistPort(candidatePort)
				continue
			}
			return 0, "", fmt.Errorf("start container: %w", err)
		}
		loop.onStarted(containerName)

		return candidatePort, containerName, nil
	}

	return 0, "", fmt.Errorf("no free host port in range %s", loop.portRange)
}
