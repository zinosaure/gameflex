// Package domains lit la table des domaines de l'API : `api-domains.json`, la
// même que lit le serveur de développement (`src/webapp/src/webpack.config.js`).
//
// Une seule table pour les deux : deux listes de domaines tenues à la main
// finiraient par diverger, et un domaine oublié d'un côté ne se verrait qu'en
// production.
package domains

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Service — un service, et les domaines de /api/v1 qu'il possède.
type Service struct {
	// Service le nomme, pour les journaux et les erreurs.
	Service string `json:"service"`
	// Target nomme la VARIABLE d'environnement qui porte son adresse, jamais
	// l'adresse elle-même : elle change d'une installation à l'autre.
	Target string `json:"target"`
	// WebSocket — le service accepte des WebSocket sous ses domaines.
	WebSocket bool `json:"websocket"`
	// ForwardClientAddress — transmettre l'adresse du client
	// (X-Forwarded-For). Le service d'identité s'en sert pour distinguer les
	// tentatives de connexion ; l'en-tête reste déclaratif, jamais une preuve.
	ForwardClientAddress bool `json:"forwardClientAddress"`
	// Domains — le premier segment après /api/v1/.
	Domains []string `json:"domains"`
}

// Load lit la table et la vérifie.
func Load(path string) ([]Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var services []Service
	if err := json.Unmarshal(data, &services); err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}

	if err := Validate(services); err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}

	return services, nil
}

// Validate refuse une table où un domaine appartient à deux services, et une
// entrée qui ne nomme pas son service, sa variable ou au moins un domaine.
func Validate(services []Service) error {
	if len(services) == 0 {
		return errors.New("aucun service déclaré")
	}

	owners := map[string]string{}
	for _, service := range services {
		if service.Service == "" || service.Target == "" || len(service.Domains) == 0 {
			return fmt.Errorf("entrée incomplète : %+v", service)
		}

		for _, domain := range service.Domains {
			if domain == "" || strings.Contains(domain, "/") {
				return fmt.Errorf("domaine invalide %q (%s)", domain, service.Service)
			}

			if owner, taken := owners[domain]; taken {
				return fmt.Errorf("le domaine %q est déclaré par %s et par %s", domain, owner, service.Service)
			}

			owners[domain] = service.Service
		}
	}

	return nil
}
