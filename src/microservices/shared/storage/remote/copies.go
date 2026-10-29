package remote

// LES LISTINGS DES SERVEURS FÉDÉRÉS, GARDÉS EN COPIE (chantier 15, décidé par
// le porteur le 2026-10-13 ; partagé par les deux kernels).
//
// « Quand je rajoute un token sur mon serveur, c'est ce fichier que je récupère
// en copiant. Et quand il y a une mise à jour de ce fichier depuis le serveur
// distant, je n'ai qu'à appuyer sur le bouton rescan qui en vérité ne va que
// re-télécharger la dernière version de ce fichier » (le porteur).
//
// La copie se range dans `cache/listing/`, au format de tous les listings,
// sous une clé dérivée de l'adresse de l'ami (Key). Elle se prend à l'ajout de
// l'ami et par l'icône (Refresh) ; et quand on ouvre sa liste, sa DATE se
// demande — au plus une fois par minute — pour ne re-télécharger la copie que
// si elle a avancé (Fresh). Elle remplace le catalogue gardé cinq minutes en
// mémoire, qui se perdait à chaque redémarrage.
//
// UN REFUS N'EST PAS UNE PANNE. Un ami qui refuse notre jeton voit sa copie
// supprimée — on ne montre plus ce qu'il ne nous prête plus — et la page dit
// « ce serveur n'autorise pas l'accès » ; un ami qui ne répond pas se dit
// autrement (« ce serveur ne répond pas »), et sa copie attend son retour.

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"gameflex/consoles"
	"gameflex/storage/gamestatus"
	"gameflex/storage/listing"
)

// CheckEvery — au plus une question de date par ami et par minute : dix
// affichages en dix secondes ne font pas dix allers-retours.
const CheckEvery = time.Minute

// Peer — un serveur fédéré tel qu'on l'a enregistré : son adresse, notre jeton
// chez lui, et son identifiant dans notre registre.
type Peer struct {
	ID    int64
	Link  string
	Token string
}

// Key — la clé de la copie d'un ami : dérivée de son adresse, comme celle
// d'une ressource. Un autre ami ne peut pas la prendre.
func Key(peer Peer) string {
	return listing.Key(listing.PeerSource, peer.Link, "")
}

// Copies — les copies des listings des amis. Le zéro est prêt à servir dès que
// `Dir` est posé. Sur le serveur d'un kernel et non en variable de paquet :
// deux instances dans un test partageraient sinon leurs questions.
type Copies struct {
	// Dir — le cache des listings (`cache/listing/`).
	Dir string
	// Every — zéro : CheckEvery.
	Every time.Duration

	mu sync.Mutex
	// asked — la dernière question posée à chaque ami, et sa réponse.
	asked map[string]answer
	// busy — un ami à la fois : deux listes ouvertes ensemble ne
	// re-téléchargent pas deux fois la même copie.
	busy map[string]*sync.Mutex
}

func (copies *Copies) every() time.Duration {
	if copies.Every > 0 {
		return copies.Every
	}

	return CheckEvery
}

type answer struct {
	at  time.Time
	err error
}

func (copies *Copies) hold(key string) func() {
	copies.mu.Lock()
	if copies.busy == nil {
		copies.busy = map[string]*sync.Mutex{}
	}
	lock := copies.busy[key]
	if lock == nil {
		lock = &sync.Mutex{}
		copies.busy[key] = lock
	}
	copies.mu.Unlock()

	lock.Lock()

	return lock.Unlock
}

func (copies *Copies) remember(key string, err error) {
	copies.mu.Lock()
	defer copies.mu.Unlock()
	if copies.asked == nil {
		copies.asked = map[string]answer{}
	}
	copies.asked[key] = answer{at: time.Now(), err: err}
}

func (copies *Copies) lastAnswer(key string) (answer, bool) {
	copies.mu.Lock()
	defer copies.mu.Unlock()
	last, known := copies.asked[key]

	return last, known
}

// failed — un ami qui refuse notre jeton perd sa copie ; un ami muet la garde.
func (copies *Copies) failed(key string, err error) {
	if errors.Is(err, ErrDenied) {
		_ = listing.Remove(copies.Dir, key)
	}
	copies.remember(key, err)
}

// Refresh re-télécharge le listing d'un ami, maintenant : à son ajout, et par
// l'icône à côté du sélecteur de source. Rend l'en-tête de la copie.
func (copies *Copies) Refresh(peer Peer) (listing.Header, error) {
	key := Key(peer)
	defer copies.hold(key)()

	return copies.download(peer, key)
}

func (copies *Copies) download(peer Peer, key string) (listing.Header, error) {
	client, err := New(peer.Link, peer.Token)
	if err != nil {
		return listing.Header{}, err
	}
	got, err := client.Listing()
	if err != nil {
		copies.failed(key, err)

		return listing.Header{}, err
	}
	// LA COPIE DIT CE QU'ELLE EST : celle d'un ami. Ce que notre jeton nous y
	// ouvre (`Scopes`), c'est lui qui l'a posé dans l'en-tête qu'il nous sert.
	got.Header.Source = listing.PeerSource
	if err := listing.Save(copies.Dir, key, got); err != nil {
		return listing.Header{}, err
	}
	copies.remember(key, nil)

	return listing.LoadHeader(copies.Dir, key)
}

// Fresh — l'en-tête de la copie d'un ami, à jour : sa date se demande au plus
// une fois par minute, et la copie ne se re-télécharge que si elle a avancé,
// ou s'il n'y en a pas encore. L'erreur dit pourquoi il n'y a rien à montrer :
// ErrDenied s'il refuse notre jeton, une autre s'il ne répond pas — retenue la
// minute elle aussi, pour qu'un ami éteint ne fasse pas attendre chaque
// affichage.
func (copies *Copies) Fresh(peer Peer) (listing.Header, error) {
	return copies.fresh(peer, false)
}

// Check — comme Fresh, mais la date et la portée se demandent MAINTENANT, sans
// la minute : la fiche d'un de ses jeux décide de Play sur ce que l'ami nous
// ouvre, et une portée qu'il vient de retirer doit s'y voir aussitôt (relevé
// par le porteur le 2026-10-13 : « Consoles » retirée, Play restait).
func (copies *Copies) Check(peer Peer) (listing.Header, error) {
	return copies.fresh(peer, true)
}

func (copies *Copies) fresh(peer Peer, now bool) (listing.Header, error) {
	key := Key(peer)
	defer copies.hold(key)()

	header, err := listing.LoadHeader(copies.Dir, key)
	have := err == nil
	if last, known := copies.lastAnswer(key); !now && known && time.Since(last.at) < copies.every() {
		if last.err != nil {
			return listing.Header{}, last.err
		}
		if have {
			return header, nil
		}
	}

	client, err := New(peer.Link, peer.Token)
	if err != nil {
		return listing.Header{}, err
	}
	fresh, err := client.ListingUpdated()
	if err != nil {
		copies.failed(key, err)

		return listing.Header{}, err
	}
	if have && !fresh.UpdatedAt.After(header.UpdatedAt) {
		// UNE PORTÉE ACCORDÉE OU RETIRÉE ne change pas sa date : la copie
		// prend la nouvelle sans tout re-télécharger.
		if fresh.Scopes != nil && !sameScopes(fresh.Scopes, header.Scopes) {
			if err := listing.SaveHeader(copies.Dir, key, func(copied *listing.Header) {
				copied.Scopes = fresh.Scopes
			}); err == nil {
				header.Scopes = fresh.Scopes
			}
		}
		copies.remember(key, nil)

		return header, nil
	}

	return copies.download(peer, key)
}

func sameScopes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}

	return true
}

// Items — une section d'une console dans la copie d'un ami, à jour.
func (copies *Copies) Items(peer Peer, section, consoleID string) ([]listing.Item, error) {
	if _, err := copies.Fresh(peer); err != nil {
		return nil, err
	}

	return listing.LoadSection(copies.Dir, Key(peer), consoleID, section)
}

// SectionAll — une section entière de la copie d'un ami, à jour.
func (copies *Copies) SectionAll(peer Peer, section string) (map[string][]listing.Item, error) {
	if _, err := copies.Fresh(peer); err != nil {
		return nil, err
	}

	return listing.LoadSectionAll(copies.Dir, Key(peer), section)
}

// Game — ce que la copie d'un ami annonce pour CE jeu (son UUID), ou le refus
// à rendre : 403 s'il refuse notre jeton, 502 s'il ne répond pas, 404 s'il ne
// le connaît pas.
func (copies *Copies) Game(peer Peer, consoleID, id string) (listing.Item, error) {
	items, err := copies.Items(peer, listing.Roms, consoleID)
	if err != nil {
		return listing.Item{}, Refusal(err)
	}
	for _, item := range items {
		if id != "" && item.ID == id {
			return item, nil
		}
	}

	return listing.Item{}, unknownGame
}

// Header — l'en-tête de la copie d'un ami telle qu'elle est, sans rien lui
// demander : ce que l'écran Fédérations montre (la date, la version, la
// machine, ce qu'il nous accorde). Faux tant qu'on n'a pas de copie.
func (copies *Copies) Header(peer Peer) (listing.Header, bool) {
	header, err := listing.LoadHeader(copies.Dir, Key(peer))

	return header, err == nil
}

// Prune — les copies des amis qu'on n'a plus : un ami retiré emporte la sienne
// (listing.Prune — une copie prise dans la minute reste). Seules les copies
// d'amis sont touchées — ni ce serveur, ni une ressource.
func (copies *Copies) Prune(peers []Peer) []string {
	kept := map[string]bool{}
	for _, peer := range peers {
		kept[Key(peer)] = true
	}

	return listing.Prune(copies.Dir, kept, func(header listing.Header) bool {
		return header.Source == listing.PeerSource
	})
}

// RomItem, RomSection — les jeux d'un ami, console par console : la forme que
// l'écran Jeux de l'administration lit (`consoles[].roms[]`), la même sur les
// deux kernels.
type RomItem struct {
	Reference string   `json:"reference"`
	Title     string   `json:"title"`
	SizeBytes int64    `json:"sizeBytes"`
	Files     []string `json:"files,omitempty"`
	// Folder — ce jeu est un DOSSIER chez lui : il arrivera zippé, même avec
	// un seul fichier à sa racine (un `.iso` PSP).
	Folder bool `json:"folder,omitempty"`
	// Status — son statut chez l'ami (chantier 30), établi sur sa machine.
	Status *gamestatus.View `json:"status,omitempty"`
}

type RomSection struct {
	ConsoleID string    `json:"consoleId"`
	Console   string    `json:"console"`
	Roms      []RomItem `json:"roms"`
}

// RomSections — la section des jeux d'une copie, à cette forme : les consoles
// dans l'ordre du registre, puis celles qu'il ne connaît pas, sous leur
// identifiant. `statuses` : le statut de ses consoles (l'en-tête de la copie).
func RomSections(roms map[string][]listing.Item, statuses map[string]string) []RomSection {
	sections := []RomSection{}
	add := func(id, name string) {
		items := roms[id]
		if len(items) == 0 {
			return
		}
		section := RomSection{ConsoleID: id, Console: name, Roms: make([]RomItem, 0, len(items))}
		for _, item := range items {
			view := gamestatus.Resolve(id, statuses[id], item.Reports)
			section.Roms = append(section.Roms, RomItem{
				Reference: item.ID, Title: item.Name, SizeBytes: item.Size, Files: item.Files, Folder: item.Dir,
				Status: &view,
			})
		}
		sections = append(sections, section)
	}

	known := map[string]bool{}
	for _, console := range consoles.All() {
		known[console.ID] = true
		add(console.ID, console.Name)
	}
	unknown := []string{}
	for id := range roms {
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	sort.Strings(unknown)
	for _, id := range unknown {
		add(id, id)
	}

	return sections
}

// PatchItem, PatchSection — des correctifs, console par console : la forme de
// l'écran Correctifs (chantier 15), sur les deux kernels — ceux d'ici, ou ceux
// d'un ami avec `installed` quand on les a aussi.
type PatchItem struct {
	Name      string   `json:"name"`
	SizeBytes int64    `json:"sizeBytes"`
	Dir       bool     `json:"dir"`
	Files     []string `json:"files"`
	Installed bool     `json:"installed"`
}

type PatchSection struct {
	ConsoleID string      `json:"consoleId"`
	Console   string      `json:"console"`
	Items     []PatchItem `json:"items"`
}

// PatchSections — la section des correctifs d'un listing, à cette forme ;
// `here` dit si un correctif est aussi ici. Les consoles dans l'ordre du
// registre, puis celles qu'il ne connaît pas.
func PatchSections(patches map[string][]listing.Item, here func(consoleID, name string) bool) []PatchSection {
	sections := []PatchSection{}
	add := func(id, name string) {
		items := patches[id]
		if len(items) == 0 {
			return
		}
		section := PatchSection{ConsoleID: id, Console: name, Items: make([]PatchItem, 0, len(items))}
		for _, item := range items {
			files := item.Files
			if files == nil {
				files = []string{}
			}
			section.Items = append(section.Items, PatchItem{
				Name: item.Name, SizeBytes: item.Size, Dir: item.Dir, Files: files, Installed: here(id, item.Name),
			})
		}
		sections = append(sections, section)
	}

	known := map[string]bool{}
	for _, console := range consoles.All() {
		known[console.ID] = true
		add(console.ID, console.Name)
	}
	unknown := []string{}
	for id := range patches {
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	sort.Strings(unknown)
	for _, id := range unknown {
		add(id, id)
	}

	return sections
}

// PatchName — un nom de correctif utilisable comme chemin : une entrée de
// `patches/<console>/`, sans séparateur, ni « . », ni « .. », ni nom caché.
func PatchName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.HasPrefix(name, ".") &&
		!strings.ContainsAny(name, `/\`)
}

// ParamsSection — ce qu'un ami prête des paramètres d'une console : un prêt
// par émulateur dont il a choisi des valeurs, et un par préréglage perso —
// chacun tel que `offer` le dit vu d'ici.
type ParamsSection struct {
	ConsoleID string `json:"consoleId"`
	Console   string `json:"console"`
	Items     []any  `json:"items"`
}

// ParamsSections — la section des paramètres d'un listing, à cette forme. Les
// consoles dans l'ordre du registre ; celles qu'il ne connaît pas n'ont pas
// d'émulateur réglable ici, et ne paraissent pas.
func ParamsSections(params map[string][]listing.Item, offer func(consoleID, emulator, presetID, presetName string) any) []ParamsSection {
	sections := []ParamsSection{}
	for _, console := range consoles.All() {
		section := ParamsSection{ConsoleID: console.ID, Console: console.Name, Items: []any{}}
		for _, item := range params[console.ID] {
			if item.Values {
				section.Items = append(section.Items, offer(console.ID, item.Name, "", ""))
			}
			for _, preset := range item.Presets {
				section.Items = append(section.Items, offer(console.ID, item.Name, preset.ID, preset.Name))
			}
		}
		if len(section.Items) > 0 {
			sections = append(sections, section)
		}
	}

	return sections
}
