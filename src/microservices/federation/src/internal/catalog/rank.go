package catalog

import (
	"os"
	"sort"
	"strconv"
	"strings"

	"gameflex/emuparams"
	"gameflex/federation/internal/library"
	"gameflex/federation/internal/store"
	"gameflex/storage"
	"gameflex/storage/listing"
)

// L'ORDRE DES SOURCES D'UN ÉLÉMENT (chantier 61 ; le porteur, 2026-11-18 : « l'ordre des priorités dépendra […] de la
// disponibilité, la qualité et la vitesse de chaque serveur » ; « pour un jeu PS4 déballé, SFTP ou FTP ne seront pas
// les meilleurs candidats » ; « Tant que ce sont des jeux retro avec un seul fichier de moins de 100mb max. On peut
// laisser passer les SFTP ou FTP » ; « Oui bonne idée pour épingler une source »). Dans cet ordre :
//
//  1. ÉPINGLÉE : l'administrateur l'a mise en tête de tout ;
//  2. DISPONIBLE : une source dont la dernière connexion a échoué passe derrière les autres ;
//  3. ADÉQUATE : un disque interne d'abord, il est sur la machine ; puis, pour un GROS élément (un dossier, des milliers
//     de fichiers, ou plus de 100 Mo) Samba, WebDAV, kDrive, SFTP, FTP — le coût par fichier compte ; pour un PETIT
//     élément (un seul fichier de moins de 100 Mo) FTP, SFTP, WebDAV, Samba, kDrive — ils laissent Samba et WebDAV
//     aux gros jeux qu'on sert peut-être en même temps ;
//  4. RAPIDE : à adéquation égale, le meilleur débit mesuré sur ses vrais transferts (State.Speed) ;
//  5. puis l'ordre d'ajout, pour un ordre stable.

// bigItem — au-delà, un élément d'un seul fichier est traité comme un gros.
const bigItem = 100 << 20

var (
	bigOrder   = map[string]int{"disk": 0, "smb": 1, "webdav": 2, "kdrive": 3, "sftp": 4, "ftp": 5}
	smallOrder = map[string]int{"disk": 0, "ftp": 1, "sftp": 2, "webdav": 3, "smb": 4, "kdrive": 5}
)

type sourceInfo struct {
	name, kind string
	pinned     bool
}

// kinds — le nom, le genre et l'épingle de chaque source connue.
func (c *Catalog) kinds() map[int64]sourceInfo {
	infos := map[int64]sourceInfo{}
	for _, r := range c.resources() {
		infos[r.ID] = sourceInfo{name: r.Name, kind: r.Kind, pinned: c.store.Setting(pinSetting(r.ID), "") == "1"}
	}

	return infos
}

func pinSetting(id int64) string { return "pinned_" + strconv.FormatInt(id, 10) }

// Pin — épingle une source en tête de tout, ou la libère.
func (c *Catalog) Pin(id int64, pinned bool) error {
	value := ""
	if pinned {
		value = "1"
	}
	if err := c.store.SetSetting(pinSetting(id), value); err != nil {
		return err
	}
	c.mu.Lock()
	c.wakeLocked()
	c.mu.Unlock()

	return nil
}

// Pinned — cette source est-elle épinglée ?
func (c *Catalog) Pinned(id int64) bool { return c.store.Setting(pinSetting(id), "") == "1" }

// sortSourcesLocked — trie des identifiants de sources, du meilleur au moins bon, pour un élément gros ou petit.
func (c *Catalog) sortSourcesLocked(ids []int64, infos map[int64]sourceInfo, big bool) {
	order := smallOrder
	if big {
		order = bigOrder
	}
	class := func(id int64) int {
		if rank, ok := order[infos[id].kind]; ok {
			return rank
		}

		return len(order)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		if infos[a].pinned != infos[b].pinned {
			return infos[a].pinned
		}
		if up, down := c.data.States[a].Error == "", c.data.States[b].Error == ""; up != down {
			return up
		}
		if class(a) != class(b) {
			return class(a) < class(b)
		}
		if c.data.States[a].Speed != c.data.States[b].Speed {
			return c.data.States[a].Speed > c.data.States[b].Speed
		}

		return a < b
	})
}

// key — ce qui fait qu'un élément est LE MÊME d'une source à l'autre : l'identifiant d'un jeu, le nom du reste.
func key(section string, item listing.Item) string {
	if section == listing.Roms {
		return item.ID
	}

	return item.Name
}

func isBig(item listing.Item) bool { return item.Dir || item.Size > bigItem }

// candidate — un élément dans une source.
type candidate struct {
	source int64
	item   listing.Item
	where  library.Locator
}

// gatherLocked — toutes les copies d'une section, par console et par élément.
func (c *Catalog) gatherLocked(section string) map[string]map[string][]candidate {
	gathered := map[string]map[string][]candidate{}
	for _, id := range c.idsLocked() {
		f := c.data.Found[id].Sections[section]
		for console, items := range f.Items {
			if gathered[console] == nil {
				gathered[console] = map[string][]candidate{}
			}
			for i, item := range items {
				if i >= len(f.Where[console]) {
					break
				}
				k := key(section, item)
				gathered[console][k] = append(gathered[console][k], candidate{source: id, item: item, where: f.Where[console][i]})
			}
		}
	}

	return gathered
}

// orderLocked — les copies d'un élément, de la meilleure à la moins bonne.
func (c *Catalog) orderLocked(copies []candidate, infos map[int64]sourceInfo) []candidate {
	if len(copies) < 2 {
		return copies
	}
	big := false
	for _, copy := range copies {
		big = big || isBig(copy.item)
	}
	ids := make([]int64, len(copies))
	byID := map[int64]candidate{}
	for i, copy := range copies {
		ids[i] = copy.source
		byID[copy.source] = copy
	}
	c.sortSourcesLocked(ids, infos, big)
	ordered := make([]candidate, len(ids))
	for i, id := range ids {
		ordered[i] = byID[id]
	}

	return ordered
}

func (c *Catalog) sourcesOf(copies []candidate, infos map[int64]sourceInfo) []listing.Source {
	list := make([]listing.Source, 0, len(copies))
	for _, copy := range copies {
		info := infos[copy.source]
		list = append(list, listing.Source{
			ID: strconv.FormatInt(copy.source, 10), Name: info.name, Kind: info.kind, Size: copy.where.Size,
			Pinned: info.pinned, Unavailable: c.data.States[copy.source].Error != "",
		})
	}

	return list
}

// Listing — le listing fondu, au format de la fédération : une ligne par élément, ses sources dans l'ordre.
func (c *Catalog) Listing(scopes []string) listing.Listing {
	infos := c.kinds()
	c.mu.Lock()
	defer c.mu.Unlock()

	served := listing.New(listing.PeerSource, c.name())
	served.Header.Server = &listing.Server{Hostname: c.name(), Version: os.Getenv("GAMEFLEX_VERSION"), CatalogOnly: true}
	served.Header.Scopes = scopes
	at := c.data.UpdatedAt
	for _, section := range []string{listing.Roms, listing.Bios, listing.Cores, listing.Patches} {
		items := map[string][]listing.Item{}
		for console, byKey := range c.gatherLocked(section) {
			for _, copies := range byKey {
				ordered := c.orderLocked(copies, infos)
				item := ordered[0].item
				item.Sources = c.sourcesOf(ordered, infos)
				items[console] = append(items[console], item)
			}
			sort.SliceStable(items[console], func(i, j int) bool {
				return strings.ToLower(items[console][i].Name) < strings.ToLower(items[console][j].Name)
			})
		}
		served.SetSection(section, items, at)
	}
	served.SetSection(listing.Params, c.paramsItemsLocked(infos), at)

	return served
}

// paramsItemsLocked — les paramètres prêtés, par console et émulateur, avec leurs sources.
func (c *Catalog) paramsItemsLocked(infos map[int64]sourceInfo) map[string][]listing.Item {
	type copyOf struct {
		item   listing.Item
		source int64
	}
	byKey := map[string][]copyOf{}
	consoleOf := map[string]string{}
	for _, id := range c.idsLocked() {
		root := c.data.Found[id].Params
		if root == "" {
			continue
		}
		for _, shared := range (emuparams.Store{Root: root}).Inventory() {
			item := listing.Item{Name: shared.Ref.Emulator, Values: shared.Values}
			for _, preset := range shared.Presets {
				item.Presets = append(item.Presets, listing.Preset{ID: preset.ID, Name: preset.Name})
			}
			k := shared.Ref.ConsoleID + "/" + shared.Ref.Emulator
			byKey[k] = append(byKey[k], copyOf{item: item, source: id})
			consoleOf[k] = shared.Ref.ConsoleID
		}
	}
	items := map[string][]listing.Item{}
	for k, copies := range byKey {
		candidates := make([]candidate, len(copies))
		for i, copy := range copies {
			candidates[i] = candidate{source: copy.source, item: copy.item}
		}
		ordered := c.orderLocked(candidates, infos)
		item := ordered[0].item
		item.Sources = c.sourcesOf(ordered, infos)
		items[consoleOf[k]] = append(items[consoleOf[k]], item)
	}

	return items
}

// Copy — un élément prêt à servir depuis une de ses sources.
type Copy struct {
	Source   int64
	Kind     string
	Item     listing.Item
	Where    library.Locator
	Resource store.Resource
	Open     func() (storage.Source, error)
}

// Candidates — les copies d'un élément d'une section, de la meilleure à la moins bonne ; `preferred` (l'identifiant
// d'une source, `?source=`) passe devant s'il en a une.
func (c *Catalog) Candidates(section, console, itemKey string, preferred int64) []Copy {
	infos := c.kinds()
	c.mu.Lock()
	copies := c.orderLocked(c.gatherLocked(section)[console][itemKey], infos)
	c.mu.Unlock()
	if preferred != 0 {
		for i, copy := range copies {
			if copy.source == preferred {
				copies = append([]candidate{copy}, append(copies[:i:i], copies[i+1:]...)...)

				break
			}
		}
	}
	result := make([]Copy, 0, len(copies))
	for _, copy := range copies {
		r, ok := c.resource(copy.source)
		if !ok {
			continue
		}
		result = append(result, Copy{Source: copy.source, Kind: r.Kind, Item: copy.item, Where: copy.where, Resource: r,
			Open: func() (storage.Source, error) { return c.open(r) }})
	}

	return result
}
