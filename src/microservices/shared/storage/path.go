package storage

import "strings"

// Les chemins virtuels sont toujours en "/", jamais en séparateur natif : ils
// ne désignent rien sur le disque de la machine qui exécute ce code. Un
// partage SMB vu depuis un téléphone n'a pas de chemin Windows à respecter, et
// mélanger les deux conventions est le genre de détail qui ne casse que chez
// l'utilisateur.

func trimPath(p string) string {
	return strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/")
}

// splitPath sépare le premier segment du reste : "/smb1/jeux/ps1" →
// ("smb1", "jeux/ps1").
func splitPath(p string) (string, string) {
	clean := trimPath(p)
	if clean == "" {
		return "", ""
	}
	if slash := strings.IndexByte(clean, '/'); slash >= 0 {
		return clean[:slash], clean[slash+1:]
	}

	return clean, ""
}

// joinPath assemble sans jamais produire de "//" ni de "/" en tête.
func joinPath(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if clean := trimPath(part); clean != "" {
			kept = append(kept, clean)
		}
	}

	return strings.Join(kept, "/")
}
