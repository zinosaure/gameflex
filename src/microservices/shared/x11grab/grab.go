// Package x11grab lit l'écran d'un serveur X par le protocole, sans mémoire
// partagée — pour les endroits où la mémoire partagée SysV n'existe pas.
//
// POURQUOI PAS MIT-SHM, MESURÉ ET NON SUPPOSÉ (2026-09-12).
//
// La capture du serveur (runtimes/streamex/internal/capture) demande au serveur
// X d'écrire l'image dans un segment SysV partagé : zéro copie, et c'est le bon
// choix là où ça marche. Sur Android, trois constats l'excluent :
//
//  1. `# CONFIG_SYSVIPC is not set` — le noyau ne compile pas SysV IPC du tout.
//     Ce n'est donc pas une affaire de seccomp : `shmget` ne peut jamais
//     répondre, et le Go appelle l'appel système directement.
//  2. MIT-SHM 1.2 sait passer la mémoire par DESCRIPTEUR (ShmCreateSegment),
//     ce qui contournerait SysV — mais `jezek/xgb` ne lit jamais les données
//     auxiliaires d'une réponse (aucun SCM_RIGHTS dans son cœur ;
//     `CreateSegmentReply` ne porte que `Nfd`, le NOMBRE de descripteurs).
//  3. Et le Xvfb du bundle ne le propose pas non plus : il importe
//     `libandroid_shmget`/`shmat` (le shim Termux sur ashmem), pas
//     `memfd_create`.
//
// Reste donc `GetImage` par la socket : la requête est minuscule, la réponse
// porte la trame entière. Le champ de longueur d'une réponse X est un mot de
// 32 bits, donc une trame 720p (3,7 Mo) passe en un seul échange — BIG-REQUESTS
// ne borne que les requêtes, pas les réponses.
//
// Le prix est une copie par trame. L'optimisation, le jour où elle sera
// nécessaire : charger `libandroid-shmem.so` (déjà dans notre bundle, et il
// exporte `shmget`/`shmat` sous leurs noms nus) par cgo, et retrouver le
// chemin zéro copie que le serveur, lui, utilise déjà.
package x11grab

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Display est une connexion ouverte à un serveur X, avec de quoi convertir ses
// pixels.
type Display struct {
	conn *xgb.Conn
	root xproto.Drawable

	// Width/Height : la taille de l'écran, lue au raccordement.
	Width  int
	Height int

	// Décalages des composantes dans un pixel de 4 octets, DÉDUITS des masques
	// du visuel et non supposés. Un serveur X annonce ses masques ; les
	// deviner « BGRA parce que c'est l'usage » est exactement le genre de
	// supposition qui donne une image aux couleurs inversées sans erreur.
	redOff   int
	greenOff int
	blueOff  int

	// keysym → keycode, lu au premier geste injecté (voir input.go). Nil tant
	// que l'entrée n'a pas servi : capturer ne demande pas XTEST.
	keycodes map[uint32]byte

	// LES DEUX TEMPS D'UNE CAPTURE, SÉPARÉS — relevés par TakeSplit.
	//
	// Atomiques et non protégés par un verrou : le fil qui capture les écrit,
	// celui qui rapporte les échange, et aucun des deux ne doit attendre
	// l'autre pour une mesure.
	fetchNanos   atomic.Int64
	convertNanos atomic.Int64
	grabbed      atomic.Int64

	// L'EMPREINTE DE LA DERNIÈRE IMAGE CONVERTIE — voir FrameHash.
	lastHash atomic.Uint64

	// L'EMPREINTE DE LA DERNIÈRE IMAGE BRUTE, et si une conversion a eu lieu
	// depuis l'ouverture — voir GrabNV12. Le seul fil qui capture les touche.
	rawHash   uint64
	converted bool
}

// Open raccorde le serveur X dont la socket est `socket` — un chemin, ou un
// nom ABSTRAIT si elle commence par « @ » (net.Dial le traduit en octet nul de
// tête, la convention Linux). C'est le cas de l'écran de l'application :
// « @/tmp/.X11-unix/X0 », le nom standard, sans fichier — /tmp n'existe pas
// sur Android, et une socket abstraite n'a besoin d'aucun dossier.
//
// PAS `NewConnDisplay` : son analyse de chaîne d'affichage ne sait produire que
// `/tmp/.X11-unix/X<n>` (le chemin, codé en dur) ou la convention launchd
// `<chemin>:<n>` — jamais l'abstraite. On ouvre donc la socket soi-même et on
// la confie à `NewConnNet`, qui existe précisément pour ça.
func Open(socket string) (*Display, error) {
	netConn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("socket X %s : %w", socket, err)
	}
	conn, err := xgb.NewConnNet(netConn)
	if err != nil {
		_ = netConn.Close()

		return nil, fmt.Errorf("poignée de main X : %w", err)
	}

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)
	display := &Display{
		conn:   conn,
		root:   xproto.Drawable(screen.Root),
		Width:  int(screen.WidthInPixels),
		Height: int(screen.HeightInPixels),
	}
	if err := display.readVisual(setup, screen); err != nil {
		conn.Close()

		return nil, err
	}

	return display, nil
}

// readVisual trouve le visuel de la racine et en déduit où sont le rouge, le
// vert et le bleu dans un pixel.
func (d *Display) readVisual(setup *xproto.SetupInfo, screen *xproto.ScreenInfo) error {
	for _, depth := range screen.AllowedDepths {
		for _, visual := range depth.Visuals {
			if visual.VisualId != screen.RootVisual {
				continue
			}
			d.redOff = maskOffset(visual.RedMask)
			d.greenOff = maskOffset(visual.GreenMask)
			d.blueOff = maskOffset(visual.BlueMask)
			if d.redOff < 0 || d.greenOff < 0 || d.blueOff < 0 {
				return fmt.Errorf("visuel inattendu : masques r=%#x v=%#x b=%#x",
					visual.RedMask, visual.GreenMask, visual.BlueMask)
			}

			return nil
		}
	}

	return fmt.Errorf("visuel de la racine (%d) introuvable", screen.RootVisual)
}

// maskOffset rend l'octet où vit une composante, ou -1 si le masque n'est pas
// un octet aligné (un visuel 16 bits, par exemple : ce code ne le gère pas et
// doit le DIRE plutôt que rendre des couleurs fausses).
func maskOffset(mask uint32) int {
	switch mask {
	case 0x000000ff:
		return 0
	case 0x0000ff00:
		return 1
	case 0x00ff0000:
		return 2
	case 0xff000000:
		return 3
	default:
		return -1
	}
}

// Size rend la taille de l'écran.
func (d *Display) Size() (int, int) {
	return d.Width, d.Height
}

// GrabRGBA capture tout l'écran dans `dst`, en RGBA compact, PREMIÈRE RANGÉE EN
// HAUT — l'ordre que `GatewayEncoder.submitRgba` attend (sa documentation
// disait l'inverse ; ni son `enqueue` ni les producteurs existants ne
// retournent l'image).
//
// `dst` doit faire au moins Width*Height*4 octets. Rien n'est alloué ici : le
// même tampon se réutilise d'une trame à l'autre, comme la boucle de
// KeytestHost le fait déjà.
func (d *Display) GrabRGBA(dst []byte) error {
	need := d.Width * d.Height * 4
	if len(dst) < need {
		return fmt.Errorf("tampon de %d octets, %d nécessaires", len(dst), need)
	}
	src, err := d.fetch(need)
	if err != nil {
		return err
	}

	// Le seul travail : remettre les composantes dans l'ordre RGBA. Les rangées
	// ne bougent pas.
	for at := 0; at < need; at += 4 {
		dst[at] = src[at+d.redOff]
		dst[at+1] = src[at+d.greenOff]
		dst[at+2] = src[at+d.blueOff]
		dst[at+3] = 0xff
	}

	return nil
}

// NV12Size rend la taille du tampon que GrabNV12 demande : le plan Y, puis la
// chroma à demi-résolution dans les deux sens.
func (d *Display) NV12Size() int {
	return d.Width*d.Height + 2*((d.Width/2)*(d.Height/2))
}

// GrabNV12 capture l'écran en NV12 : le plan Y de Width*Height, puis UN SEUL
// plan de chroma où U et V ALTERNENT (u,v,u,v…), demi-largeur × demi-hauteur.
//
// POURQUOI ICI ET PAS EN JAVA, et c'est une mesure, pas un goût : sur l'A35, le
// 2026-09-12, la boucle tournait à 2 images par seconde avec **20 ms de capture
// et 420 ms de soumission** — l'encodeur matériel, lui, n'avait refusé aucune
// trame. Les 420 ms étaient la conversion RGB→YUV faite en Java, 921 600
// pixels un par un, bornes vérifiées à chaque accès. Ici, elle coûte quelques
// millisecondes.
//
// POURQUOI CE RANGEMENT PLUTÔT QUE L'I420, et c'est encore une mesure : le
// codec de l'A35 réclame du semi-planaire. Sa géométrie, demandée le
// 2026-09-12, répond `luma 1280/1, U 1280/2, V 1280/2` — pas de pixel 2 sur la
// chroma, donc entrelacé. Java devait alors poser 230 400 octets UN PAR UN,
// faute de pouvoir recopier en bloc, et la soumission tenait encore 62 ms par
// trame après le passage de la conversion en Go.
//
// Produit ici dans l'ordre que le codec attend, la chroma se verse par
// demi-rangées entières : 360 recopies au lieu de 230 400 écritures. Et comme
// son pas de rangée vaut exactement la demi-largeur × 2, ces rangées sont
// jointives des deux côtés.
//
// Les coefficients sont exactement ceux de GatewayEncoder.enqueue (BT.601 en
// virgule fixe), et le sous-échantillonnage prend le pixel en haut à gauche de
// chaque bloc 2×2. (Un prélèvement I420 a vécu à côté jusqu'au 2026-10-15 :
// essayé, il n'apportait rien — voir XserverHost.)
func (d *Display) GrabNV12(dst []byte) error {
	need := d.NV12Size()
	if len(dst) < need {
		return fmt.Errorf("tampon de %d octets, %d nécessaires", len(dst), need)
	}

	// LES DEUX TEMPS PRIS ICI : c'est le chemin que la boucle de l'application
	// emprunte.
	startedAt := time.Now()
	src, err := d.fetch(d.Width * d.Height * 4)
	if err != nil {
		return err
	}
	fetchedAt := time.Now()

	// UNE IMAGE QUI N'A PAS CHANGÉ NE SE CONVERTIT PAS (2026-10-26).
	//
	// La boucle prélève soixante fois par seconde, et un jeu lourd n'en dessine
	// que quelques-unes : sur le S25, *FF7 Remake* faisait 7 images neuves par
	// seconde, et cette conversion prenait 62 % du temps du code Go de
	// l'application — 115 % de processeur pour elle, sur un téléphone bridé par
	// la chaleur (profil simpleperf). L'empreinte se prend donc sur l'image
	// BRUTE, un pixel sur seize, avant de convertir ; inchangée, `dst` garde la
	// conversion précédente — la boucle repasse le même tampon à chaque tour
	// (XserverHost) — et `lastHash` reste celle de cette image : la boucle la
	// compte comme un doublon, comme avant. Même limite que l'empreinte
	// ci-dessous : une image qui ne différerait que dans les pixels sautés
	// passerait pour identique.
	raw := rawFrameHash(src[:d.Width*d.Height*4])
	if d.converted && raw == d.rawHash {
		d.fetchNanos.Add(fetchedAt.Sub(startedAt).Nanoseconds())
		d.grabbed.Add(1)

		return nil
	}
	d.rawHash, d.converted = raw, true

	ToNV12(src, d.Width, d.Height, d.redOff, d.greenOff, d.blueOff, dst)
	uvAt := d.Width * d.Height

	// L'EMPREINTE, ÉCHANTILLONNÉE : un octet de luma sur seize.
	//
	// Elle sert à savoir si l'image a CHANGÉ, pas à l'identifier. Hacher les
	// 460 800 octets coûterait la moitié de la conversion elle-même ; 19 200
	// suffisent à distinguer deux images d'un jeu qui bouge. Limite assumée et
	// nommée : une image qui ne différerait que dans les octets sautés
	// passerait pour identique.
	var hash uint64 = 1469598103934665603 // FNV-1a, base
	for at := 0; at < uvAt; at += 16 {
		hash ^= uint64(dst[at])
		hash *= 1099511628211
	}
	d.lastHash.Store(hash)

	d.fetchNanos.Add(fetchedAt.Sub(startedAt).Nanoseconds())
	d.convertNanos.Add(time.Since(fetchedAt).Nanoseconds())
	d.grabbed.Add(1)

	return nil
}

// rawFrameHash — l'empreinte d'une image de 32 bits par pixel, un pixel ENTIER
// sur seize (FNV-1a) : 57 600 lectures pour un écran de 1280×720, contre
// 921 600 pixels à convertir.
func rawFrameHash(src []byte) uint64 {
	var hash uint64 = 1469598103934665603
	for at := 0; at+3 < len(src); at += 64 {
		hash ^= uint64(src[at]) | uint64(src[at+1])<<8 | uint64(src[at+2])<<16 | uint64(src[at+3])<<24
		hash *= 1099511628211
	}

	return hash
}

// ToNV12 convertit une image de 32 bits par pixel — rangées jointives de
// width*4 octets, le rouge, le vert et le bleu aux décalages donnés — en NV12
// dans dst : le plan Y, puis U et V alternés, demi-largeur × demi-hauteur.
// dst doit tenir width*height*3/2 octets.
//
// Le cœur de GrabNV12, sorti de lui pour servir aussi les images du moteur
// libretro du téléphone (chantier 13) : une seule conversion BT.601 côté Go,
// avec les coefficients de GatewayEncoder.enqueue.
func ToNV12(src []byte, width, height, redOff, greenOff, blueOff int, dst []byte) {
	halfW := width / 2
	halfH := height / 2
	uvAt := width * height

	// PAR BANDES DE LIGNES, SUR PLUSIEURS FILS.
	//
	// Cette conversion était séquentielle, et c'est le poste le plus lourd de
	// notre chaîne sur un téléphone : 7 ms par image au repos, 17 à 19 sous
	// charge, mesurés sur un Galaxy A35 le 2026-09-13, quand la lecture de
	// l'écran n'en coûte que 3 à 10. Chaque ligne est indépendante des autres —
	// la luma lit une ligne source et écrit une ligne de destination, la chroma
	// une ligne sur deux — donc le découpage est sûr sans aucun verrou.
	//
	// Mesuré sur un banc à seize cœurs : 0,65 ms en séquentiel, 0,18 sur quatre
	// fils, 0,14 sur huit (x4,7). Le banc n'est PAS l'appareil et son gain ne
	// s'y transporte pas tel quel ; c'est la cadence rapportée par la boucle sur
	// le téléphone qui dira le vrai chiffre.
	//
	// Borné à huit : au-delà, les bandes deviennent plus petites que ce que la
	// mémoire sait alimenter, et un téléphone n'a pas seize cœurs à donner.
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	band := (height + workers - 1) / workers
	for w := 0; w < workers; w++ {
		from, to := w*band, (w+1)*band
		if to > height {
			to = height
		}
		if from >= to {
			continue
		}
		wg.Add(1)
		go func(from, to int) {
			defer wg.Done()
			for y := from; y < to; y++ {
				row := y * width * 4
				luma := y * width
				for x := 0; x < width; x++ {
					at := row + x*4
					r := int(src[at+redOff])
					g := int(src[at+greenOff])
					b := int(src[at+blueOff])
					dst[luma+x] = byte(((66*r + 129*g + 25*b + 128) >> 8) + 16)
				}
			}
		}(from, to)
	}

	bandC := (halfH + workers - 1) / workers
	for w := 0; w < workers; w++ {
		from, to := w*bandC, (w+1)*bandC
		if to > halfH {
			to = halfH
		}
		if from >= to {
			continue
		}
		wg.Add(1)
		go func(from, to int) {
			defer wg.Done()
			for y := from; y < to; y++ {
				row := 2 * y * width * 4
				chroma := uvAt + y*halfW*2
				for x := 0; x < halfW; x++ {
					at := row + 2*x*4
					r := int(src[at+redOff])
					g := int(src[at+greenOff])
					b := int(src[at+blueOff])
					dst[chroma+2*x] = byte(((-38*r - 74*g + 112*b + 128) >> 8) + 128)
					dst[chroma+2*x+1] = byte(((112*r - 94*g - 18*b + 128) >> 8) + 128)
				}
			}
		}(from, to)
	}
	wg.Wait()
}

// FrameHash rend l'empreinte de la dernière image convertie par GrabNV12, ou 0
// avant la première. Deux appels successifs qui rendent la même valeur veulent
// dire que l'écran n'a PAS changé entre les deux captures.
//
// À quoi ça sert : savoir combien d'images l'encodeur reçoit en double. Sur un
// téléphone, l'émulateur dessine moins vite que notre boucle ne capture — le
// banc lui compte 33 images neuves par seconde, l'appareil beaucoup moins — et
// chaque doublon encodé est du processeur pris à l'émulateur. On MESURE d'abord,
// on décidera de sauter ensuite : couper l'envoi a ses propres risques (un flux
// muet trop longtemps, un contrôle de débit qui s'affole), et ce compteur dit ce
// qu'il y aurait à gagner avant d'y toucher.
func (d *Display) FrameHash() uint64 {
	return d.lastHash.Load()
}

// TakeSplit rend les deux temps MOYENS d'une capture, en nanosecondes, et
// remet les compteurs à zéro — l'appelant les relève périodiquement.
//
// POURQUOI CETTE SÉPARATION, ET POURQUOI MAINTENANT. La boucle de
// l'application mesure « la capture » en un seul bloc : 57 à 90 ms par image
// sur un Galaxy A35, ce qui plafonne le flux à une dizaine d'images par seconde
// et explique exactement ce que le porteur voit (mesuré le 2026-09-13, dans son
// propre journal). Mais ce bloc contient DEUX choses de natures différentes :
// la lecture de l'écran par la socket X, et la conversion RGBA→NV12 ici même.
//
// Et la stratégie à choisir dépend entièrement de laquelle pèse. Un framebuffer
// mmappé — `Xvfb -fbdir`, que le binaire embarqué annonce et qui produit un
// fichier vivant de 1 232 032 octets pour un écran 640x480, en-tête XWD de
// 3 232 octets compris — supprimerait la LECTURE et ne toucherait pas la
// conversion. Mesurer avant de réécrire.
//
// Zéro image depuis le dernier relevé : rend trois zéros, et l'appelant se
// tait plutôt que d'afficher une moyenne qui n'existe pas.
func (d *Display) TakeSplit() (fetchNanos, convertNanos, frames int64) {
	frames = d.grabbed.Swap(0)
	fetchNanos = d.fetchNanos.Swap(0)
	convertNanos = d.convertNanos.Swap(0)
	if frames == 0 {
		return 0, 0, 0
	}

	return fetchNanos / frames, convertNanos / frames, frames
}

// fetch demande l'image entière et vérifie que la réponse la porte vraiment.
// Partagé par les deux captures : une réponse trop courte est la seule erreur
// que le protocole ne signale pas lui-même.
func (d *Display) fetch(need int) ([]byte, error) {
	reply, err := xproto.GetImage(d.conn, xproto.ImageFormatZPixmap, d.root,
		0, 0, uint16(d.Width), uint16(d.Height), 0xffffffff).Reply()
	if err != nil {
		return nil, fmt.Errorf("GetImage : %w", err)
	}
	if len(reply.Data) < need {
		return nil, fmt.Errorf("réponse de %d octets, %d attendus", len(reply.Data), need)
	}

	return reply.Data, nil
}

// Close ferme la connexion.
func (d *Display) Close() {
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
}
