package retro_files

import (
	"encoding/json"
	"strings"
	"testing"
)

// L'ORIGINE D'UN JEU VOYAGE EN CHAMPS, ET CE QU'IL N'A PAS DOIT RESTER ABSENT.
//
// Les deux identifiants d'origine (ServerID, MountID — #163) portent
// `omitempty`, et c'est précisément ce qui les rend sûrs : un jeu d'ici ne doit
// annoncer NI serverId NI mountId, sinon la page lirait « serveur 0 » ou
// « partage 0 » — deux origines qui n'existent pas. Ni le compilateur ni
// `go vet` ne vérifient une étiquette JSON ; ce test si.
//
// Il vaut aussi comme garde-fou au renommage : `mountId` est le nom que la
// webapp lit (libs/sources.js, sourceOf), donc le changer ici casse la page
// sans casser la compilation.
func TestRomEntryCarriesItsOriginAndOmitsWhatItHasNot(t *testing.T) {
	cases := []struct {
		name    string
		entry   RomEntry
		present []string
		absent  []string
	}{
		{
			name:   "un jeu d'ici ne déclare aucune origine",
			entry:  RomEntry{Reference: "Mario.zip", Title: "Mario"},
			absent: []string{"serverId", "mountId", "kind", "source"},
		},
		{
			name: "un jeu d'un partage monté déclare son mountId",
			entry: RomEntry{
				Reference: "mnt3:jeux/Mario.zip",
				Title:     "Mario",
				Kind:      "kdrive",
				Source:    "kDrive perso",
				MountID:   3,
			},
			present: []string{`"mountId":3`, `"kind":"kdrive"`},
			absent:  []string{"serverId"},
		},
		{
			name: "un jeu fédéré déclare son serverId",
			entry: RomEntry{
				Reference: "srv4:Mario.zip",
				Title:     "Mario",
				Kind:      "server",
				Source:    "la box de Gino",
				ServerID:  4,
			},
			present: []string{`"serverId":4`, `"kind":"server"`},
			absent:  []string{"mountId"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, err := json.Marshal(testCase.entry)
			if err != nil {
				t.Fatalf("encodage impossible : %v", err)
			}

			for _, wanted := range testCase.present {
				if !strings.Contains(string(encoded), wanted) {
					t.Errorf("%s manque dans %s", wanted, encoded)
				}
			}

			for _, unwanted := range testCase.absent {
				if strings.Contains(string(encoded), unwanted) {
					t.Errorf("%s ne devrait pas apparaître dans %s", unwanted, encoded)
				}
			}
		})
	}
}
