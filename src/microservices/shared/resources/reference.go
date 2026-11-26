package resources

import (
	"net/http"
	"strconv"
	"strings"
)

// LA RÉFÉRENCE D'UN JEU D'UNE RESSOURCE, `res<id>:<sa référence>` — comme celle d'un jeu d'un ami (`srv<id>:`) : la
// liste d'une ressource la rend ainsi, le Catalogue la garde, et le ticket de téléchargement la reconnaît. Sans le
// préfixe, un jeu d'une ressource et le même jeu de la bibliothèque (le même identifiant) se confondraient.
const referencePrefix = "res"

// Reference — la référence d'un jeu de la ressource `id`.
func Reference(id int64, reference string) string {
	return referencePrefix + strconv.FormatInt(id, 10) + ":" + reference
}

// ParseReference — la ressource et la référence d'un jeu `res<id>:<référence>` ; faux pour toute autre référence.
func ParseReference(reference string) (int64, string, bool) {
	rest, ok := strings.CutPrefix(reference, referencePrefix)
	if !ok {
		return 0, "", false
	}
	idText, inner, ok := strings.Cut(rest, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if !ok || err != nil || id <= 0 || inner == "" {
		return 0, "", false
	}

	return id, inner, true
}

// Has — la ressource `id` porte-t-elle ce jeu (son dernier parcours) ?
func (c *Catalog) Has(id int64, consoleID, reference string) bool {
	items, _ := c.Games(id, consoleID)
	for _, item := range items {
		if item.ID == reference {
			return true
		}
	}

	return false
}

// ServeGame — un jeu servi depuis la ressource `id` et elle seule (le Catalogue : le ticket désigne sa ressource) ;
// faux si elle ne le porte pas.
func (c *Catalog) ServeGame(w http.ResponseWriter, r *http.Request, id int64, consoleID, reference string, fail func(status int, code, message string)) bool {
	only := []Copy{}
	for _, copy := range c.Candidates("roms", consoleID, reference, id) {
		if copy.Source == id {
			only = append(only, copy)
		}
	}

	return c.Serve(w, r, only, LocatorPath, fail)
}

// Lent — ce qu'un compte voit des ressources sans les administrer (le Catalogue) : leur nom, leur genre, leurs jeux par
// console — ni adresse, ni compte, ni état.
type Lent struct {
	ID       int64          `json:"id"`
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Consoles map[string]int `json:"consoles"`
}

// LentList — les ressources, à la forme de Lent.
func (c *Catalog) LentList() ([]Lent, error) {
	views, err := c.Views()
	if err != nil {
		return nil, err
	}
	list := make([]Lent, 0, len(views))
	for _, view := range views {
		list = append(list, Lent{ID: view.ID, Name: view.Name, Kind: view.Kind, Consoles: view.Consoles})
	}

	return list, nil
}
