package storage

// LE TEMPS QU'ON ACCORDE À UNE SOURCE DISTANTE.
//
// Un `http.Client{Timeout: …}` en Go couvre TOUT l'échange, lecture du corps
// comprise. Trente secondes suffisent largement pour lister un dossier, et
// pas du tout pour descendre un jeu : un .iso de 855 Mo était coupé en plein
// vol, toujours au même endroit, quelle que soit la qualité du réseau
// (« unexpected end of stream » sur PSP et PS2, signalé le 2026-09-02).
//
// Ce qu'on veut borner, ce n'est pas la DURÉE d'un transfert — un gros jeu a
// le droit d'être long — c'est l'ATTENTE : le temps de joindre le serveur, et
// le temps qu'il met à répondre. Un transfert qui avance n'est jamais coupé ;
// un serveur muet l'est tout de suite.

import (
	"io"
	"net"
	"net/http"
	"time"
)

// NewTransferClient rend un client fait pour des fichiers, pas pour des
// requêtes courtes : aucune limite sur la durée totale, des limites sur
// l'attente.
func NewTransferClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 15 * time.Second,
			// Le serveur a trente secondes pour COMMENCER à répondre. Après,
			// c'est son affaire : tant que des octets arrivent, on lit.
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   4,
		},
	}
}

// SizedReadCloser — un flux qui sait ce qu'il pèse.
//
// La taille vient de la réponse elle-même, et c'est tout l'intérêt : la
// demander séparément voulait dire lister le dossier parent AVANT chaque
// téléchargement — 10 783 entrées paginées pour ouvrir une cartouche NES de
// 41 Ko, et un délai dépassé côté application (2026-09-02).
type SizedReadCloser interface {
	io.ReadCloser
	// Size est le poids annoncé, ou -1 quand la source ne le dit pas.
	Size() int64
}

type sizedBody struct {
	io.ReadCloser
	size int64
}

func (b sizedBody) Size() int64 { return b.size }

// WithSize habille un flux de sa taille annoncée.
func WithSize(body io.ReadCloser, size int64) SizedReadCloser {
	return sizedBody{ReadCloser: body, size: size}
}

// SizeOf — ce que ce flux annonce, ou -1.
func SizeOf(body io.ReadCloser) int64 {
	if sized, ok := body.(SizedReadCloser); ok {
		return sized.Size()
	}

	return -1
}

// CE QU'UNE SOURCE A APPRIS DE SON ARBORESCENCE.
//
// Certaines sources n'adressent pas par chemin. kDrive adresse par NUMÉRO :
// ouvrir « roms/nes/Mario.nes » suppose de savoir lequel, et le seul moyen de
// l'apprendre est de lister le dossier — 10 785 entrées paginées, une demi-
// minute, avant le premier octet.
//
// Le parcours de la bibliothèque paie déjà cette marche, pour tout le
// partage. Il ne la gardait qu'en mémoire vive : au redémarrage suivant, le
// premier téléchargement de chaque console la repayait entièrement (signalé
// le 2026-09-02, sur les 10 783 jeux NES et les 7 635 NDS).
//
// Cette interface laisse le kernel ranger cet acquis à côté du catalogue, et
// le redonner à la source qui reprend. Rien de tout ça ne vient d'une
// requête : ce sont des identifiants internes, et les laisser désigner par
// un client ouvrirait tout le drive.
type Memo interface {
	// ExportMemo rend ce que la source a appris, sous une forme qu'on peut
	// écrire en JSON.
	ExportMemo() map[string]string
	// ImportMemo lui rend cet acquis. Une source doit rester correcte si on
	// ne l'appelle jamais : c'est un raccourci, pas une dépendance.
	ImportMemo(learned map[string]string)
}
