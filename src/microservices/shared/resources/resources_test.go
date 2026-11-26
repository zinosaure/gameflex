package resources

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gameflex/storage"
	"gameflex/storage/listing"
)

// memorySource — un partage en mémoire : chemin → contenu (repris des tests de gameflex-federation).
type memorySource struct {
	name  string
	files map[string]string
}

func (m *memorySource) Name() string { return m.name }

func (m *memorySource) List(dir string) ([]storage.Entry, error) {
	dir = strings.Trim(dir, "/")
	seen := map[string]storage.Entry{}
	for file, content := range m.files {
		rel := file
		if dir != "" {
			if !strings.HasPrefix(file, dir+"/") {
				continue
			}
			rel = strings.TrimPrefix(file, dir+"/")
		}
		first, _, nested := strings.Cut(rel, "/")
		if nested {
			seen[first] = storage.Entry{Name: first, Dir: true}
		} else {
			seen[first] = storage.Entry{Name: first, Size: int64(len(content))}
		}
	}
	if len(seen) == 0 && dir != "" {
		return nil, os.ErrNotExist
	}
	entries := make([]storage.Entry, 0, len(seen))
	for _, e := range seen {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	return entries, nil
}

func (m *memorySource) Open(p string) (io.ReadCloser, error) {
	content, ok := m.files[strings.Trim(p, "/")]
	if !ok {
		return nil, os.ErrNotExist
	}

	return io.NopCloser(strings.NewReader(content)), nil
}

// fixture — un magasin, un catalogue, et des partages en mémoire rangés par adresse.
type fixture struct {
	t      *testing.T
	store  *Store
	cat    *Catalog
	shares map[string]*memorySource
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "databases"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, store: st, shares: map[string]*memorySource{}}
	f.cat = New(st, dir, func() string { return "ici" })
	f.cat.OpenSource = func(r Resource) (storage.Source, error) {
		share, ok := f.shares[r.Address]
		if !ok {
			return nil, errors.New("partage injoignable")
		}

		return share, nil
	}

	return f
}

// add — enregistre une ressource et attend la fin de son parcours.
func (f *fixture) add(r Resource) Resource {
	f.t.Helper()
	id, err := f.store.AddResource(r)
	if err != nil {
		f.t.Fatal(err)
	}
	r.ID = id
	f.cat.Rescan(r)
	deadline := time.Now().Add(5 * time.Second)
	for f.cat.States()[id].Scanning || f.cat.States()[id].ScannedAt.IsZero() {
		if time.Now().After(deadline) {
			f.t.Fatalf("%s : parcours sans fin", r.Name)
		}
		time.Sleep(10 * time.Millisecond)
	}

	return r
}

// LES MOTS DE PASSE SONT SCELLÉS : ni en clair dans le fichier, ni rendus faux après une réouverture ; une connexion
// passée en anonyme perd le sien, et un mot de passe vide garde l'ancien.
func TestPasswordsAreSealedInTheStore(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.AddResource(Resource{Name: "NAS", Kind: "smb", Address: "smb://nas/jeux", Username: "gino", Password: "très-secret"})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(dir, "resources.json"))
	if bytes.Contains(text, []byte("très-secret")) {
		t.Fatal("le mot de passe est en clair dans le fichier")
	}
	for _, name := range []string{"resources.json", "resources.key"} {
		if info, _ := os.Stat(filepath.Join(dir, name)); info.Mode().Perm() != 0o600 {
			t.Errorf("%s : mode %v, attendu 600", name, info.Mode().Perm())
		}
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := reopened.Resources()
	if len(list) != 1 || list[0].Password != "très-secret" {
		t.Fatalf("après réouverture : %+v", list)
	}
	if err := reopened.UpdateResource(Resource{ID: id, Name: "NAS 2", Kind: "smb", Address: "smb://nas/jeux", Username: "gino"}); err != nil {
		t.Fatal(err)
	}
	if list, _ = reopened.Resources(); list[0].Password != "très-secret" || list[0].Name != "NAS 2" {
		t.Fatalf("un mot de passe vide doit garder l'ancien : %+v", list[0])
	}
	if err := reopened.UpdateResource(Resource{ID: id, Name: "NAS 2", Kind: "smb", Address: "smb://nas/jeux"}); err != nil {
		t.Fatal(err)
	}
	if list, _ = reopened.Resources(); list[0].Password != "" {
		t.Fatal("passée en anonyme, la connexion garde un mot de passe")
	}
	if err := reopened.RemoveResource(id); err != nil {
		t.Fatal(err)
	}
	if err := reopened.RemoveResource(id); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retirer deux fois : %v", err)
	}
}

// LE SERVEUR REVALIDE LES RÈGLES DU FORMULAIRE.
func TestTheServerRevalidatesTheAddressRules(t *testing.T) {
	good := []Resource{
		{Name: "NAS", Kind: "smb", Address: "smb://nas/jeux"},
		{Name: "NAS", Kind: "smb", Address: "smb://192.168.1.20:445/jeux/retro"},
		{Name: "Cloud", Kind: "webdav", Address: "https://dav.example.com:8443/remote.php"},
		{Name: "Cloud", Kind: "webdav", Address: "http://[fe80::1]/dav"},
		{Name: "FTP", Kind: "ftp", Address: "ftp://box"},
		{Name: "SFTP", Kind: "sftp", Address: "sftp://box:2222/"},
		{Name: "kDrive", Kind: "kdrive", Address: "123456"},
	}
	for _, r := range good {
		if err := Validate(r); err != nil {
			t.Errorf("%s %s refusée : %v", r.Kind, r.Address, err)
		}
	}
	bad := []Resource{
		{Name: "", Kind: "smb", Address: "smb://nas/jeux"},
		{Name: "NAS", Kind: "nfs", Address: "nfs://nas/jeux"},
		{Name: "NAS", Kind: "smb", Address: "smb://nas"},
		{Name: "NAS", Kind: "smb", Address: "https://nas/jeux"},
		{Name: "NAS", Kind: "smb", Address: "smb://nas:99999/jeux"},
		{Name: "NAS", Kind: "smb", Address: "smb://300.1.1.1/jeux"},
		{Name: "NAS", Kind: "smb", Address: "smb://na_s/jeux"},
		{Name: "FTP", Kind: "ftp", Address: "ftp://box/jeux"},
		{Name: "kDrive", Kind: "kdrive", Address: "abc"},
		{Name: "NAS", Kind: "smb", Address: "nas/jeux"},
	}
	for _, r := range bad {
		if err := Validate(r); err == nil {
			t.Errorf("%s %q acceptée", r.Kind, r.Address)
		}
	}
}

// DEUX PARTAGES, UNE LIGNE PAR ÉLÉMENT : un jeu présent sur les deux porte ses deux sources, dans l'ordre (un petit jeu :
// FTP avant Samba) ; ce qu'un seul porte n'a que lui. Une ressource retirée emporte ce qu'elle portait.
func TestOneLinePerItemWithItsSourcesInOrder(t *testing.T) {
	f := newFixture(t)
	f.shares["smb://nas/jeux"] = &memorySource{name: "NAS", files: map[string]string{
		"roms/nes/Zelda.nes":    "ZELDA",
		"roms/nes/Metroid.nes":  "METROID",
		"bios/ps1/scph1001.bin": "BIOS",
	}}
	f.shares["ftp://box"] = &memorySource{name: "Box", files: map[string]string{
		"roms/nes/Zelda.nes": "ZELDA",
	}}
	nas := f.add(Resource{Name: "NAS", Kind: "smb", Address: "smb://nas/jeux"})
	box := f.add(Resource{Name: "Box", Kind: "ftp", Address: "ftp://box"})

	served := f.cat.Listing(nil)
	byName := map[string]listing.Item{}
	for _, item := range served.Roms["nes"] {
		byName[item.Name] = item
	}
	if len(served.Roms["nes"]) != 2 {
		t.Fatalf("nes : %+v, attendu Zelda et Metroid une fois chacun", served.Roms["nes"])
	}
	zelda := byName["Zelda"]
	if len(zelda.Sources) != 2 || zelda.Sources[0].Name != "Box" || zelda.Sources[1].Name != "NAS" {
		t.Fatalf("sources de Zelda : %+v, attendu Box puis NAS", zelda.Sources)
	}
	if len(byName["Metroid"].Sources) != 1 {
		t.Fatalf("sources de Metroid : %+v", byName["Metroid"].Sources)
	}
	if len(served.Bios["ps1"]) != 1 {
		t.Fatalf("BIOS : %+v", served.Bios)
	}

	f.cat.Forget(box.ID, true)
	if err := f.store.RemoveResource(box.ID); err != nil {
		t.Fatal(err)
	}
	served = f.cat.Listing(nil)
	for _, item := range served.Roms["nes"] {
		if item.Name == "Zelda" && (len(item.Sources) != 1 || item.Sources[0].Name != nas.Name) {
			t.Fatalf("après retrait de Box : %+v", item.Sources)
		}
	}
}

// SERVIR : un fichier depuis sa meilleure copie, un jeu-dossier zippé, et la copie suivante quand la première lâche.
func TestAnItemIsServedFromItsFirstCopyThatAnswers(t *testing.T) {
	f := newFixture(t)
	f.shares["ftp://box"] = &memorySource{name: "Box", files: map[string]string{
		"roms/nes/Zelda.nes":       "ZELDA",
		"roms/ps1/Crash/Crash.cue": "CUE",
		"roms/ps1/Crash/Crash.bin": "BIN",
	}}
	f.shares["smb://nas/jeux"] = &memorySource{name: "NAS", files: map[string]string{"roms/nes/Zelda.nes": "ZELDA"}}
	box := f.add(Resource{Name: "Box", Kind: "ftp", Address: "ftp://box"})
	f.add(Resource{Name: "NAS", Kind: "smb", Address: "smb://nas/jeux"})

	refOf := func(console, name string) string {
		for _, item := range f.cat.Listing(nil).Roms[console] {
			if item.Name == name {
				return item.ID
			}
		}
		t.Fatalf("%s/%s absent du listing", console, name)

		return ""
	}
	serve := func(console, ref string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/roms", nil)
		served := f.cat.Serve(recorder, request, f.cat.Candidates(listing.Roms, console, ref, 0), LocatorPath, func(status int, code, message string) {
			recorder.WriteHeader(status)
			_, _ = recorder.WriteString(code + ": " + message)
		})
		if !served {
			t.Fatalf("%s/%s : aucune copie", console, ref)
		}

		return recorder
	}

	if got := serve("nes", refOf("nes", "Zelda")); got.Body.String() != "ZELDA" || got.Header().Get("X-GameFlex-Source") == "" {
		t.Fatalf("Zelda : %d %q", got.Code, got.Body.String())
	}

	crash := serve("ps1", refOf("ps1", "Crash"))
	archive, err := zip.NewReader(bytes.NewReader(crash.Body.Bytes()), int64(crash.Body.Len()))
	if err != nil {
		t.Fatalf("Crash n'est pas un zip : %v", err)
	}
	names := []string{}
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "Crash.bin,Crash.cue" {
		t.Fatalf("zip de Crash : %v", names)
	}

	// Box tombe : Zelda vient de NAS, et Box passe derrière.
	zelda := refOf("nes", "Zelda")
	delete(f.shares, "ftp://box")
	f.cat.Forget(box.ID, false)
	if got := serve("nes", zelda); got.Body.String() != "ZELDA" {
		t.Fatalf("après la chute de Box : %d %q", got.Code, got.Body.String())
	}
	if f.cat.States()[box.ID].Error == "" {
		t.Fatal("Box devrait être marquée indisponible")
	}
}

// LUE, MAIS VIDE À SA RACINE (le porteur, 2026-11-23 : un kDrive rangé sous /gameflex/roms, « Dossier » laissé vide) :
// l'état le dit et nomme le dossier qui porte la bibliothèque ; la liste répond pendant et après (la suggestion se
// cherchait sous le verrou du catalogue, qu'elle reprenait : tout restait bloqué).
func TestAnEmptyResourceSuggestsTheFolderThatHoldsTheLibrary(t *testing.T) {
	f := newFixture(t)
	f.shares["123456"] = &memorySource{name: "kDrive", files: map[string]string{
		"gameflex/roms/nes/Zelda.nes": "ZELDA",
		"Photos/plage.jpg":            "JPG",
	}}
	done := make(chan Resource, 1)
	go func() { done <- f.add(Resource{Name: "kDrive", Kind: "kdrive", Address: "123456", Password: "jeton"}) }()
	var added Resource
	select {
	case added = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("le parcours d'une ressource vide reste bloqué")
	}
	state := f.cat.States()[added.ID]
	if !state.Empty || len(state.Suggest) != 1 || state.Suggest[0] != "gameflex" {
		t.Fatalf("état : %+v, attendu vide et la suggestion « gameflex »", state)
	}
	if _, err := f.cat.Views(); err != nil {
		t.Fatal(err)
	}

	// « Dossier » corrigé : les jeux arrivent, et l'avis part.
	added.Folder = "gameflex"
	f.shares["123456"] = &memorySource{name: "kDrive", files: map[string]string{"roms/nes/Zelda.nes": "ZELDA"}}
	if err := f.store.UpdateResource(added); err != nil {
		t.Fatal(err)
	}
	f.cat.Forget(added.ID, false)
	f.cat.Rescan(added)
	deadline := time.Now().Add(5 * time.Second)
	for f.cat.States()[added.ID].Scanning {
		if time.Now().After(deadline) {
			t.Fatal("seconde lecture sans fin")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state := f.cat.States()[added.ID]; state.Empty || state.Games != 1 {
		t.Fatalf("après correction : %+v", state)
	}
}

// « DOSSIER » QUI NOMME LE DOSSIER DES JEUX (`…/roms`) : son parent est suggéré ; un dossier absent se nomme.
func TestAFolderPointingAtRomsSuggestsItsParentAndAMissingOneIsNamed(t *testing.T) {
	f := newFixture(t)
	f.shares["sftp://box"] = &memorySource{name: "Box", files: map[string]string{"release/roms/nes/Zelda.nes": "ZELDA"}}
	f.cat.OpenSource = func(r Resource) (storage.Source, error) {
		share := f.shares[r.Address]
		if r.Folder == "absent" {
			return nil, os.ErrNotExist
		}
		if r.Folder != "" {
			return &memorySource{name: share.name, files: subtree(share.files, r.Folder)}, nil
		}

		return share, nil
	}
	pointing := f.add(Resource{Name: "Box", Kind: "sftp", Address: "sftp://box", Folder: "release/roms"})
	if state := f.cat.States()[pointing.ID]; !state.Empty || len(state.Suggest) != 1 || state.Suggest[0] != "release" {
		t.Fatalf("Dossier « release/roms » : %+v, attendu la suggestion « release »", state)
	}
	missing := f.add(Resource{Name: "Box 2", Kind: "sftp", Address: "sftp://box", Folder: "absent"})
	if state := f.cat.States()[missing.ID]; !strings.Contains(state.Error, `"absent"`) {
		t.Fatalf("dossier absent : %+v", state)
	}
}

// subtree — les fichiers sous `folder`, chemins relatifs à lui (une source ouverte sur un sous-dossier).
func subtree(files map[string]string, folder string) map[string]string {
	kept := map[string]string{}
	for file, content := range files {
		if rest, ok := strings.CutPrefix(file, strings.Trim(folder, "/")+"/"); ok {
			kept[rest] = content
		}
	}

	return kept
}

// UN PARCOURS « EN COURS » ÉCRIT AVANT UN ARRÊT NE BLOQUE PAS LA RELECTURE APRÈS LE REDÉMARRAGE.
func TestAScanInProgressDoesNotSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "databases"))
	if err != nil {
		t.Fatal(err)
	}
	stuck := `{"updatedAt":"2026-11-23T00:00:00Z","digest":"x","found":{},"states":{"5":{"scanning":true,"games":0}}}`
	if err := os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(stuck), 0o600); err != nil {
		t.Fatal(err)
	}
	if state := New(st, dir, func() string { return "ici" }).States()[5]; state.Scanning {
		t.Fatalf("relu : %+v, un parcours ne survit pas au redémarrage", state)
	}
}

// LA RÉFERENCE `res<id>:` se lit comme elle s'écrit ; toute autre est refusée.
func TestAResourceReferenceReadsAsItIsWritten(t *testing.T) {
	ref := Reference(5, "54d0569a-3a95-5df2-b2b9-299b1512e365")
	if id, inner, ok := ParseReference(ref); !ok || id != 5 || inner != "54d0569a-3a95-5df2-b2b9-299b1512e365" {
		t.Fatalf("%s → %d %q %v", ref, id, inner, ok)
	}
	for _, other := range []string{"srv5:abc", "res:abc", "res0:abc", "resx:abc", "res5:", "54d0569a"} {
		if _, _, ok := ParseReference(other); ok {
			t.Errorf("%q acceptée", other)
		}
	}
}

// LE GENRE D'UNE ERREUR DE SOURCE, pour le badge de la page.
func TestASourceErrorIsClassified(t *testing.T) {
	cases := map[string]error{
		"offline": errors.New(`Propfind "http://nas/": dial tcp: lookup nas: no such host`),
		"refused": errors.New("response 530: Login incorrect"),
		"folder":  os.ErrNotExist,
		"error":   errors.New("something else"),
	}
	for want, err := range cases {
		if got := classify(err); got != want {
			t.Errorf("%v → %q, attendu %q", err, got, want)
		}
	}
	if classify(nil) != "" {
		t.Error("pas d'erreur, pas de problème")
	}
}
