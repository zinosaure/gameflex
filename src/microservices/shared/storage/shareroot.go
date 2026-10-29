package storage

import "strings"

// ShareRoot — l'adresse à partir de laquelle on scanne un partage distant.
//
// UNE RÈGLE, UN ENDROIT. Elle vivait en fonction non exportée du kernel
// (`shareRoot`, mounts_client.go) et les deux chemins du TÉLÉPHONE ne
// l'appliquaient pas : `mobile.Scan` et le `sourceOf` de mobilekernel
// passaient l'adresse nue. Un montage WebDAV avec un dossier scannait donc le
// partage ENTIER sur l'appareil — photos et documents compris —, ce que le
// commentaire du kernel décrivait déjà comme le défaut à éviter : « c'est
// long, et ça ne trouve rien si les jeux sont rangés ailleurs ».
//
// Pourquoi ici : `webdav.Config` n'a PAS de champ `Folder` — contrairement à
// `kdrive.Config`, dont l'API prend le dossier séparément. Pour WebDAV le
// dossier fait partie de l'adresse, et cette composition est la seule chose
// qui le sait. Les trois appelants (kernel, pont gomobile, kernel du
// téléphone) importent tous ce paquet.
//
// Le scanner part toujours de la racine de la source : c'est pour ça que le
// dossier doit être replié en amont plutôt que passé au scan.
func ShareRoot(baseURL, folder string) string {
	trimmed := strings.Trim(strings.TrimSpace(folder), "/")
	if trimmed == "" {
		return baseURL
	}

	return strings.TrimRight(baseURL, "/") + "/" + trimmed
}
