package resources

import (
	"strings"

	"gameflex/resources/library"
	"gameflex/storage/listing"
)

// LE MÊME CODE POUR LES DEUX SERVEURS (le kernel du serveur et celui du téléphone, `#808`) : la vue d'une ressource
// pour l'administration, et ses sections ajoutées au listing servi aux amis. Chaque kernel ne fait que les brancher.

// View — ce que l'administration voit d'une ressource : jamais son mot de passe, seulement s'il en a un.
type View struct {
	Resource
	HasPassword bool  `json:"hasPassword"`
	State       State `json:"state"`
	// Consoles — ses jeux par console (Réglages › Consoles › une console › Jeux, la ligne de la ressource).
	Consoles map[string]int `json:"consoles"`
}

// ViewOf — la vue d'une ressource.
func (c *Catalog) ViewOf(r Resource) View {
	return View{Resource: r, HasPassword: r.Password != "", State: c.States()[r.ID], Consoles: c.consolesOf(r.ID)}
}

// consolesOf — les jeux d'une ressource par console, d'après son dernier parcours.
func (c *Catalog) consolesOf(id int64) map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	counts := map[string]int{}
	if f := c.data.Found[id]; f != nil {
		for consoleID, items := range f.Sections[listing.Roms].Items {
			counts[consoleID] = len(items)
		}
	}

	return counts
}

// Games — les jeux d'une console qu'une ressource porte (son dernier parcours), avec leur emplacement.
func (c *Catalog) Games(id int64, consoleID string) ([]listing.Item, []library.Locator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f := c.data.Found[id]
	if f == nil {
		return nil, nil
	}
	section := f.Sections[listing.Roms]

	return append([]listing.Item(nil), section.Items[consoleID]...), append([]library.Locator(nil), section.Where[consoleID]...)
}

// Views — les ressources et l'état de leur dernier parcours.
func (c *Catalog) Views() ([]View, error) {
	list, err := c.store.Resources()
	if err != nil {
		return nil, err
	}
	states := c.States()
	views := make([]View, 0, len(list))
	for _, r := range list {
		views = append(views, View{Resource: r, HasPassword: r.Password != "", State: states[r.ID], Consoles: c.consolesOf(r.ID)})
	}

	return views, nil
}

// Find — une ressource par son identifiant, mot de passe descellé.
func (c *Catalog) Find(id int64) (Resource, bool) {
	return c.resource(id)
}

// MergeInto — les sections des ressources dans le listing servi à un ami, DERRIÈRE ce qu'il porte déjà (la bibliothèque
// et les disques internes) : un élément qu'il a déjà n'est pas repris — un jeu par sa référence, le reste par son nom ;
// ce qu'une ressource seule porte arrive avec ses sources, de la meilleure à la moins bonne.
func (c *Catalog) MergeInto(served *listing.Listing) {
	if c.UpdatedAt().IsZero() {
		return
	}
	extra := c.Listing(nil)
	for _, section := range []string{listing.Roms, listing.Bios, listing.Cores, listing.Patches, listing.Params} {
		added := extra.Section(section)
		if len(added) == 0 {
			continue
		}
		merged := map[string][]listing.Item{}
		lent := map[string]int{}
		seen := map[string]bool{}
		for consoleID, items := range served.Section(section) {
			merged[consoleID] = append([]listing.Item{}, items...)
			for _, item := range items {
				seen[consoleID+"/"+sameItem(section, item)] = true
			}
		}
		for consoleID, items := range added {
			for _, item := range items {
				if !seen[consoleID+"/"+sameItem(section, item)] {
					merged[consoleID] = append(merged[consoleID], item)
					lent[consoleID]++
				}
			}
		}
		at := served.Header.UpdatedAt
		if info, ok := served.Header.Sections[section]; ok {
			at = info.UpdatedAt
		}
		if extraAt := extra.Header.Sections[section].UpdatedAt; extraAt.After(at) {
			at = extraAt
		}
		served.SetSection(section, merged, at)
		// Les jeux prêtés sans être lancés, comptés à part (listing.SectionInfo.Lent).
		if section == listing.Roms {
			info := served.Header.Sections[section]
			info.Lent = lent
			served.Header.Sections[section] = info
		}
	}
}

// sameItem — ce qui fait qu'un élément est le même d'une source à l'autre : la référence d'un jeu, le nom du reste.
func sameItem(section string, item listing.Item) string {
	if section == listing.Roms {
		return item.ID
	}

	return strings.ToLower(item.Name)
}
