package resources

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// LE SERVEUR REVALIDE CE QUE LE FORMULAIRE VÉRIFIE (`#808`) — les règles de l'adresse de Réglages › Ressources
// (edge : settings/people/externalResources.jsx, `hostProblem` et `portProblem`), sur l'adresse recomposée qu'il envoie :
// le protocole du genre, un nom de serveur ou une adresse IP (IPv6 entre crochets), un port de 1 à 65535, le partage
// pour Samba, aucun chemin pour FTP et SFTP (le dossier a son champ), des chiffres pour kDrive.

// Schemes — les protocoles de chaque genre (kDrive n'en a pas : un numéro de drive).
var Schemes = map[string][]string{"smb": {"smb"}, "webdav": {"https", "http"}, "ftp": {"ftp"}, "sftp": {"sftp"}}

var (
	addressPattern = regexp.MustCompile(`^([a-z]+)://(\[[^\]]*\]|[^/:?#]*)(?::(\d*))?(.*)$`)
	hostPattern    = regexp.MustCompile(`^(?:\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*)$`)
	digits         = regexp.MustCompile(`^\d+$`)
)

// validHost — un nom de serveur, une IPv6 entre crochets, ou une IPv4 de quatre nombres de 0 à 255.
func validHost(host string) bool {
	if !hostPattern.MatchString(host) {
		return false
	}
	if strings.Trim(host, "0123456789.") == "" {
		parts := strings.Split(host, ".")
		if len(parts) != 4 {
			return false
		}
		for _, part := range parts {
			if value, err := strconv.Atoi(part); part == "" || err != nil || value > 255 {
				return false
			}
		}
	}

	return true
}

// Validate — la ressource est-elle enregistrable ? L'erreur dit quoi corriger.
func Validate(r Resource) error {
	if r.Name == "" || len(r.Name) > 60 {
		return errors.New("a resource needs a name of at most 60 characters")
	}
	if !slices.Contains(Kinds, r.Kind) {
		return errors.New("unknown kind of resource (smb, webdav, kdrive, ftp, sftp)")
	}
	if r.Kind == "kdrive" {
		if !digits.MatchString(r.Address) {
			return errors.New("a kDrive is named by its number")
		}

		return nil
	}
	found := addressPattern.FindStringSubmatch(r.Address)
	if found == nil {
		return errors.New("the address must be protocol://server[:port]")
	}
	scheme, host, port, rest := found[1], found[2], found[3], found[4]
	if !slices.Contains(Schemes[r.Kind], scheme) {
		return errors.New("the protocol does not match the kind of resource (" + strings.Join(Schemes[r.Kind], ", ") + ")")
	}
	if !validHost(host) {
		return errors.New("the server must be a host name or an IP address")
	}
	if port != "" {
		if value, err := strconv.Atoi(port); err != nil || value < 1 || value > 65535 {
			return errors.New("the port must be between 1 and 65535")
		}
	}
	switch r.Kind {
	case "smb":
		if strings.Split(strings.TrimLeft(rest, "/"), "/")[0] == "" {
			return errors.New("a Samba address names its share: smb://server/share")
		}
	case "ftp", "sftp":
		if strings.TrimRight(rest, "/") != "" {
			return errors.New("the address has no path: the folder has its own field")
		}
	}

	return nil
}
