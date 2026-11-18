package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"gameflex/federation/internal/catalog"
	"gameflex/federation/internal/library"
	"gameflex/federation/internal/store"
	"gameflex/storage"
	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

// memorySource — un partage en mémoire : des chemins relatifs et leur contenu.
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
		first, rest, nested := strings.Cut(rel, "/")
		if nested {
			_ = rest
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

type fixture struct {
	t      *testing.T
	server *httptest.Server
	store  *store.Store
	cat    *catalog.Catalog
	shares map[string]*memorySource
	// session — la session de l'administrateur, ouverte par newFixture.
	session string
	dir     string
	disks   string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	_ = st.SetSetting(SettingServerName, "Mon NAS")
	name := func() string { return st.Setting(SettingServerName, "") }
	f := &fixture{t: t, store: st, shares: map[string]*memorySource{}, dir: dir}
	f.cat = catalog.New(st, dir, name)
	f.cat.OpenSource = func(r store.Resource) (storage.Source, error) {
		if r.Kind == "disk" {
			return library.NewLocal(r.Name, r.Address)
		}
		share, ok := f.shares[r.Address]
		if !ok {
			return nil, errors.New("partage injoignable")
		}

		return share, nil
	}
	f.disks = t.TempDir()
	served := &Server{Store: st, Catalog: f.cat, AdminUsername: "gino", AdminPassword: "secret", Name: name, DataDir: dir, DisksDir: f.disks}
	f.cat.Disks = served.DiskResources
	f.server = httptest.NewServer(served.Router())
	t.Cleanup(f.server.Close)
	if status, _, _ := f.do("POST", "/api/v1/admin/login", "", map[string]string{"username": "gino", "password": "faux"}); status != 401 {
		t.Fatalf("un mauvais mot de passe doit être refusé : %d", status)
	}
	_, body, _ := f.do("POST", "/api/v1/admin/login", "", map[string]string{"username": "gino", "password": "secret"})
	f.session = body["data"].(map[string]any)["token"].(string)

	return f
}

func (f *fixture) do(method, url, token string, body any) (int, map[string]any, []byte) {
	f.t.Helper()
	var reader io.Reader
	if body != nil {
		text, _ := json.Marshal(body)
		reader = bytes.NewReader(text)
	}
	request, _ := http.NewRequest(method, f.server.URL+url, reader)
	if token == "admin" {
		token = f.session
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)

	return response.StatusCode, decoded, raw
}

// waitScanned — attend la fin du parcours d'une ressource (réveil par le catalogue, jamais de sondage à l'aveugle).
func (f *fixture) waitScanned(id int64) catalog.State {
	f.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		changed := f.cat.Changed()
		if state := f.cat.States()[id]; !state.Scanning && !state.ScannedAt.IsZero() {
			return state
		}
		select {
		case <-changed:
		case <-deadline:
			f.t.Fatal("parcours jamais fini")
		}
	}
}

func TestFederationSurfaceServesSharesAsOneServer(t *testing.T) {
	f := newFixture(t)
	f.shares["smb://nas/jeux"] = &memorySource{name: "nas", files: map[string]string{
		"roms/nes/Zelda.nes":            "ZELDA",
		"roms/ps1/FF7/FF7 (Disc 1).bin": "DISC1",
		"roms/ps1/FF7/FF7 (Disc 1).cue": "CUE1",
		"roms/ps1/FF7/FF7.m3u":          "FF7 (Disc 1).cue",
		"documents/notes.txt":           "rien",
	}}

	// Sans jeton, ou avec un faux : refusé, comme chez un kernel.
	if status, body, _ := f.do("GET", "/api/v1/federation/listing", "", nil); status != 401 || body["errorCode"] != "token_refused" {
		t.Fatalf("sans jeton : %d %v", status, body)
	}
	if status, _, _ := f.do("GET", "/api/v1/federation/listing", "gff_faux", nil); status != 401 {
		t.Fatalf("faux jeton : %d", status)
	}
	// L'administration exige son mot de passe.
	if status, _, _ := f.do("GET", "/api/v1/admin/resources", "", nil); status != 401 {
		t.Fatalf("administration sans mot de passe : %d", status)
	}

	status, body, _ := f.do("POST", "/api/v1/admin/resources", "admin", map[string]string{
		"name": "nas", "kind": "smb", "address": "smb://nas/jeux", "username": "gino", "password": "motdepasse"})
	if status != 201 {
		t.Fatalf("ajout : %d %v", status, body)
	}
	id := int64(body["data"].(map[string]any)["id"].(float64))
	if _, leaked := body["data"].(map[string]any)["password"]; leaked {
		t.Fatal("le mot de passe d'une ressource ne doit jamais sortir de l'API")
	}
	if state := f.waitScanned(id); state.Games != 2 || state.Error != "" {
		t.Fatalf("parcours : %+v", state)
	}

	_, body, _ = f.do("POST", "/api/v1/admin/tokens", "admin", map[string]string{"label": "S25"})
	value := body["data"].(map[string]any)["value"].(string)

	_, body, _ = f.do("GET", "/api/v1/federation/identity", value, nil)
	identity := body["data"].(map[string]any)
	if identity["hostname"] != "Mon NAS" || identity["identity"] != "S25" {
		t.Fatalf("identité : %v", identity)
	}

	_, _, raw := f.do("GET", "/api/v1/federation/listing", value, nil)
	var envelope struct {
		Data struct {
			Header struct {
				Source string `json:"source"`
				Name   string `json:"name"`
			} `json:"header"`
			Roms map[string][]struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Dir    bool   `json:"dir"`
				Path   string `json:"path"`
				Folder string `json:"folder"`
			} `json:"roms"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Header.Source != "gameflex" || envelope.Data.Header.Name != "Mon NAS" {
		t.Fatalf("en-tête : %+v", envelope.Data.Header)
	}
	nes, psx := envelope.Data.Roms["nes"], envelope.Data.Roms["ps1"]
	if len(nes) != 1 || len(psx) != 1 || !psx[0].Dir {
		t.Fatalf("jeux : %+v", envelope.Data.Roms)
	}
	if nes[0].Path != "" || psx[0].Folder != "" {
		t.Fatal("le listing servi ne dit pas où est un jeu dans un partage")
	}

	// Un jeu d'un seul fichier : son contenu, sous son nom.
	request, _ := http.NewRequest("GET", f.server.URL+"/api/v1/federation/roms/nes/"+nes[0].ID, nil)
	request.Header.Set("Authorization", "Bearer "+value)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(content) != "ZELDA" || !strings.Contains(response.Header.Get("Content-Disposition"), "Zelda.nes") {
		t.Fatalf("jeu : %q, %q", content, response.Header.Get("Content-Disposition"))
	}

	// Un jeu en dossier : zippé, ses fichiers relatifs au dossier.
	_, _, zipped := f.do("GET", "/api/v1/federation/roms/ps1/"+psx[0].ID, value, nil)
	archive, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, file := range archive.File {
		names = append(names, path.Clean(file.Name))
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "FF7 (Disc 1).bin,FF7 (Disc 1).cue,FF7.m3u" {
		t.Fatalf("zip : %v", names)
	}

	// LE CLIENT DES APPAREILS (shared/storage/remote, celui du kernel et du téléphone) lit le service comme un ami :
	// rien ne change de leur côté.
	client, err := remote.New(f.server.URL, value)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := client.Check()
	if err != nil || checked.Hostname != "Mon NAS" {
		t.Fatalf("Check : %v %+v", err, checked)
	}
	copied, err := client.Listing()
	if err != nil || len(copied.Roms["nes"]) != 1 || len(copied.Roms["ps1"]) != 1 {
		t.Fatalf("Listing : %v %+v", err, copied.Roms)
	}
	fresh, err := client.ListingUpdated()
	if err != nil || fresh.UpdatedAt.IsZero() {
		t.Fatalf("ListingUpdated : %v %+v", err, fresh)
	}
	rom, err := client.OpenRom("nes", copied.Roms["nes"][0].ID)
	if err != nil {
		t.Fatalf("OpenRom : %v", err)
	}
	got, _ := io.ReadAll(rom)
	rom.Close()
	if string(got) != "ZELDA" {
		t.Fatalf("OpenRom : %q", got)
	}

	// LA FICHE D'UN JEU D'UN PARTAGE (P3) : faite par le service, dans son volume ; sa clé est la référence du listing.
	status, body, _ = f.do("GET", "/api/v1/federation/metadata/nes/"+nes[0].ID, value, nil)
	if sheet, _ := body["data"].(map[string]any); status != 200 || sheet == nil || sheet["gameId"] != nes[0].ID {
		t.Fatalf("fiche d'un jeu de partage : %d %v", status, body)
	}

	// La date du listing : elle n'avance que si le contenu change.
	before := f.cat.UpdatedAt()
	f.do("POST", "/api/v1/admin/resources/1/rescan", "admin", nil)
	f.waitScanned(id)
	if !f.cat.UpdatedAt().Equal(before) {
		t.Fatal("un parcours sans changement ne doit pas avancer la date")
	}
	f.shares["smb://nas/jeux"].files["roms/nes/Metroid.nes"] = "METROID"
	f.do("POST", "/api/v1/admin/resources/1/rescan", "admin", nil)
	f.waitScanned(id)
	if !f.cat.UpdatedAt().After(before) {
		t.Fatal("un jeu de plus doit avancer la date")
	}

	// Un partage devenu injoignable garde ses jeux, et dit pourquoi.
	delete(f.shares, "smb://nas/jeux")
	f.cat.Forget(id, false)
	f.do("POST", "/api/v1/admin/resources/1/rescan", "admin", nil)
	if state := f.waitScanned(id); state.Error == "" || state.Games != 3 {
		t.Fatalf("partage injoignable : %+v", state)
	}

	// Un jeton révoqué ne lit plus rien.
	f.do("DELETE", "/api/v1/admin/tokens/1", "admin", nil)
	if status, _, _ := f.do("GET", "/api/v1/federation/listing", value, nil); status != 401 {
		t.Fatalf("jeton révoqué : %d", status)
	}
}

func TestPasswordsAreSealedInTheStore(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddResource(store.Resource{Name: "nas", Kind: "webdav", Address: "https://nas/dav",
		Password: "motdepasse-très-secret"}); err != nil {
		t.Fatal(err)
	}
	st.Close()
	raw, _ := os.ReadFile(dir + "/federation.db")
	if bytes.Contains(raw, []byte("motdepasse-très-secret")) {
		t.Fatal("le mot de passe est en clair dans la base")
	}
	again, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	resources, err := again.Resources()
	if err != nil || len(resources) != 1 || resources[0].Password != "motdepasse-très-secret" {
		t.Fatalf("relecture : %v %+v", err, resources)
	}
	if _, err := again.AddResource(store.Resource{Name: "x", Kind: "gopher", Address: "gopher://x"}); err == nil {
		t.Fatal("un genre inconnu doit être refusé")
	}
}

func TestInvitationsAndPairingCodesGiveADeviceItsToken(t *testing.T) {
	f := newFixture(t)
	client, _ := remote.New(f.server.URL, "")

	// Une invitation : échangée une fois par le client des appareils, refusée la seconde.
	_, body, _ := f.do("POST", "/api/v1/admin/invitations", "admin", map[string]string{"label": "Android TV"})
	code := body["data"].(map[string]any)["code"].(string)
	link, parsed, err := remote.ParseInvitation(f.server.URL + "/invite/" + code)
	if err != nil || parsed != code || link != f.server.URL {
		t.Fatalf("le lien d'invitation doit se lire comme celui d'un serveur GameFlex : %v %q %q", err, link, parsed)
	}
	// L'autre sorte (un compte) : refusée sans être consommée.
	if _, err := client.Redeem(code, remote.InvitationAccount, "S25"); !errors.Is(err, remote.ErrInvitationKind) {
		t.Fatalf("une invitation de compte : %v", err)
	}
	redeemed, err := client.Redeem(code, remote.InvitationFederation, "S25")
	if err != nil || redeemed.Token == "" || redeemed.Hostname != "Mon NAS" || redeemed.Identity != "Android TV" {
		t.Fatalf("échange : %v %+v", err, redeemed)
	}
	if _, err := client.Redeem(code, remote.InvitationFederation, "S25"); !errors.Is(err, remote.ErrInvitationRefused) {
		t.Fatalf("une invitation déjà servie : %v", err)
	}
	if status, _, _ := f.do("GET", "/api/v1/federation/identity", redeemed.Token, nil); status != 200 {
		t.Fatalf("le jeton reçu doit ouvrir le catalogue : %d", status)
	}

	// Un code d'appairage : refusé mode fermé (423), accepté mode ouvert, une seule fois.
	if _, err := client.Redeem("123456", remote.InvitationFederation, "S25"); !errors.Is(err, remote.ErrPairingClosed) {
		t.Fatalf("mode fermé : %v", err)
	}
	if status, _, _ := f.do("POST", "/api/v1/admin/pairing/code", "admin", map[string]string{"label": "S25"}); status != 423 {
		t.Fatalf("un code mode fermé : %d", status)
	}
	f.do("POST", "/api/v1/admin/pairing/discovery", "admin", map[string]any{"open": true, "link": f.server.URL})
	_, body, _ = f.do("POST", "/api/v1/admin/pairing/code", "admin", map[string]string{"label": "S25"})
	pin := body["data"].(map[string]any)["code"].(string)
	if !isPairingCode(pin) {
		t.Fatalf("code : %q", pin)
	}
	paired, err := client.Redeem(pin, remote.InvitationFederation, "S25")
	if err != nil || paired.Token == "" || paired.Identity != "S25" {
		t.Fatalf("appairage : %v %+v", err, paired)
	}
	if _, err := client.Redeem(pin, remote.InvitationFederation, "S25"); !errors.Is(err, remote.ErrInvitationRefused) {
		t.Fatalf("un code déjà servi : %v", err)
	}
	// LA FORME DU SUJET DE GAMEFLEX (B2) : le code a son numéro, et sa fin dit qui l'a servi.
	codeID := body["data"].(map[string]any)["id"].(float64)
	_, state, _ := f.do("GET", "/api/v1/admin/pairing", "admin", nil)
	ended := state["data"].(map[string]any)["ended"].([]any)
	if last := ended[len(ended)-1].(map[string]any); last["id"] != codeID || last["reason"] != "used" || last["by"] != "S25" {
		t.Fatalf("la fin du code : %v", ended)
	}
	// Un code annulé ne vaut plus, et le dit.
	_, body, _ = f.do("POST", "/api/v1/admin/pairing/code", "admin", map[string]string{"label": "TV"})
	cancelled := body["data"].(map[string]any)
	if status, _, _ := f.do("DELETE", fmt.Sprintf("/api/v1/admin/pairing/code/%v", cancelled["id"]), "admin", nil); status != 204 {
		t.Fatalf("annuler : %d", status)
	}
	if _, err := client.Redeem(cancelled["code"].(string), remote.InvitationFederation, "TV"); err == nil {
		t.Fatal("un code annulé a servi")
	}

	// Au cinquième code faux (le servi et l'annulé comptent), le mode se ferme.
	for i := 0; i < 2; i++ {
		_, _ = client.Redeem("000000", remote.InvitationFederation, "S25")
	}
	if !f.serverPairingOpen() {
		t.Fatal("quatre codes faux : le mode reste ouvert")
	}
	_, _ = client.Redeem("000001", remote.InvitationFederation, "S25")
	if f.serverPairingOpen() {
		t.Fatal("au cinquième code faux, le mode doit se fermer")
	}
	_, state, _ = f.do("GET", "/api/v1/admin/pairing", "admin", nil)
	closed := state["data"].(map[string]any)["closed"].([]any)
	if last := closed[len(closed)-1].(map[string]any); last["reason"] != "too_many_failures" || last["scope"] != "server" {
		t.Fatalf("la fermeture : %v", closed)
	}
}

func (f *fixture) serverPairingOpen() bool {
	_, body, _ := f.do("GET", "/api/v1/admin/pairing", "admin", nil)

	return body["data"].(map[string]any)["server"] != nil
}

func TestLogsAreListedReadAndRedacted(t *testing.T) {
	f := newFixture(t)
	day := f.dir + "/logs/20261118"
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"time":"2026-11-18T03:00:00Z","level":"INFO","msg":"http","path":"/api/v1/admin/events?token=gff_secret_value","status":200}` + "\n" +
		`{"time":"2026-11-18T03:00:01Z","level":"WARN","msg":"parcours d'une ressource en échec","resource":"NAS"}` + "\n"
	_ = os.WriteFile(day+"/federation.log", []byte(line), 0o644)

	_, body, _ := f.do("GET", "/api/v1/admin/logs", "admin", nil)
	entries := body["data"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["path"] != "20261118/federation.log" {
		t.Fatalf("liste : %v", entries)
	}
	_, body, _ = f.do("GET", "/api/v1/admin/logs/tail?path=20261118/federation.log", "admin", nil)
	lines := body["data"].([]any)
	if len(lines) != 2 || lines[1].(map[string]any)["level"] != "WARN" {
		t.Fatalf("lecture : %v", lines)
	}
	_, _, raw := f.do("GET", "/api/v1/admin/logs/download?path=20261118/federation.log&token="+f.session, "", nil)
	if strings.Contains(string(raw), "gff_secret_value") || !strings.Contains(string(raw), "parcours") {
		t.Fatalf("le téléchargement doit masquer les secrets : %s", raw)
	}
	if status, _, _ := f.do("GET", "/api/v1/admin/logs/tail?path=../federation.db", "admin", nil); status != 400 {
		t.Fatalf("un chemin hors du dossier des journaux : %d", status)
	}
	if status, _, _ := f.do("GET", "/api/v1/admin/logs/download?path=20261118/federation.log&token=faux", "", nil); status != 401 {
		t.Fatalf("un téléchargement sans session : %d", status)
	}
	_, body, _ = f.do("PUT", "/api/v1/admin/logs/settings", "admin", map[string]any{"enabled": true, "levels": []string{"warn", "error"}, "http": true})
	if got := body["data"].(map[string]any); got["http"] != true || len(got["levels"].([]any)) != 2 {
		t.Fatalf("réglages : %v", got)
	}
	f.do("DELETE", "/api/v1/admin/logs/file?path=20261118/federation.log", "admin", nil)
	if _, err := os.Stat(day + "/federation.log"); !os.IsNotExist(err) {
		t.Fatal("le journal supprimé est encore là")
	}
}

func TestInternalDisksLendEverySection(t *testing.T) {
	f := newFixture(t)
	disk := f.disks + "/nas"
	for path, content := range map[string]string{
		"roms/nes/Zelda.nes":                         "ZELDA",
		"bios/ps1/scph1001.bin":                      "BIOS",
		"patches/wiiu/NewSuperMarioBrosU/CrashFix/a": "PATCH",
		"patches/wiiu/NewSuperMarioBrosU/CrashFix/b": "PATCH2",
	} {
		_ = os.MkdirAll(path2dir(disk+"/"+path), 0o755)
		_ = os.WriteFile(disk+"/"+path, []byte(content), 0o644)
	}
	_ = os.MkdirAll(f.disks+"/vide", 0o755) // un dossier sans section n'est pas un disque présent

	_, body, _ := f.do("GET", "/api/v1/admin/disks", "admin", nil)
	disks := body["data"].([]any)
	if len(disks) != 2 || disks[0].(map[string]any)["present"] != true || disks[1].(map[string]any)["present"] != false {
		t.Fatalf("disques : %v", disks)
	}
	nas := library.Disk{Name: "nas", Dir: disk, Present: true}
	f.do("POST", "/api/v1/admin/disks/nas/rescan", "admin", nil)
	if state := f.waitScanned(nas.ID()); state.Games != 1 {
		t.Fatalf("parcours du disque : %+v", state)
	}

	_, body, _ = f.do("POST", "/api/v1/admin/tokens", "admin", map[string]string{"label": "PC"})
	pcToken := body["data"].(map[string]any)["value"].(string)
	client, _ := remote.New(f.server.URL, pcToken)
	copied, err := client.Listing()
	if err != nil || len(copied.Roms["nes"]) != 1 || len(copied.Bios["ps1"]) != 1 || len(copied.Patches["wiiu"]) != 1 {
		t.Fatalf("listing : %v roms=%v bios=%v patches=%v", err, copied.Roms, copied.Bios, copied.Patches)
	}
	rom, err := client.OpenRom("nes", copied.Roms["nes"][0].ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rom)
	rom.Close()
	bios, err := client.OpenBios("ps1", "scph1001.bin")
	if err != nil {
		t.Fatal(err)
	}
	gotBios, _ := io.ReadAll(bios)
	bios.Close()
	patch, err := client.OpenPatch("wiiu", "NewSuperMarioBrosU")
	if err != nil {
		t.Fatal(err)
	}
	zipped, _ := io.ReadAll(patch)
	patch.Close()
	archive, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if string(got) != "ZELDA" || string(gotBios) != "BIOS" || err != nil || len(archive.File) != 2 {
		t.Fatalf("fichiers : %q %q %v %d", got, gotBios, err, len(archive.File))
	}
	// La fiche d'un jeu de disque se lit (le fichier est sur cette machine).
	if status, body, _ := f.do("GET", "/api/v1/federation/metadata/nes/"+copied.Roms["nes"][0].ID, pcToken, nil); status != 200 || body["data"] == nil {
		t.Fatalf("fiche : %d %v", status, body)
	}
}

func path2dir(path string) string { return path[:strings.LastIndex(path, "/")] }

func TestOneLinePerItemWithItsSourcesInOrder(t *testing.T) {
	f := newFixture(t)
	big := strings.Repeat("X", 2048)
	shared := map[string]string{
		"roms/ps4/FF7 Remake/eboot.bin":         big,
		"roms/ps4/FF7 Remake/sce_sys/param.sfo": "SFO",
		"roms/nes/Zelda.nes":                    "ZELDA",
		"patches/ps4/CUSA07211/settings.json":   "{}",
	}
	copyOf := func() map[string]string {
		files := map[string]string{}
		for k, v := range shared {
			files[k] = v
		}

		return files
	}
	f.shares["smb://nas/jeux"] = &memorySource{name: "Samba", files: copyOf()}
	f.shares["ftp://nas"] = &memorySource{name: "FTP", files: copyOf()}
	for _, body := range []map[string]string{
		{"name": "Samba", "kind": "smb", "address": "smb://nas/jeux"},
		{"name": "FTP", "kind": "ftp", "address": "ftp://nas"},
	} {
		_, answer, _ := f.do("POST", "/api/v1/admin/resources", "admin", body)
		f.waitScanned(int64(answer["data"].(map[string]any)["id"].(float64)))
	}
	_, answer, _ := f.do("POST", "/api/v1/admin/tokens", "admin", map[string]string{"label": "TV"})
	token := answer["data"].(map[string]any)["value"].(string)
	client, _ := remote.New(f.server.URL, token)
	sourcesOf := func(items []listing.Item) []string {
		names := []string{}
		for _, source := range items[0].Sources {
			names = append(names, source.Name)
		}

		return names
	}

	copied, err := client.Listing()
	if err != nil || len(copied.Roms["ps4"]) != 1 || len(copied.Roms["nes"]) != 1 || len(copied.Patches["ps4"]) != 1 {
		t.Fatalf("une ligne par élément : %v %+v %+v", err, copied.Roms, copied.Patches)
	}
	// Un jeu en dossier (des milliers de fichiers) : Samba d'abord. Une petite ROM d'un seul fichier : FTP d'abord, pour
	// laisser Samba aux gros jeux.
	if got := sourcesOf(copied.Roms["ps4"]); strings.Join(got, ",") != "Samba,FTP" {
		t.Fatalf("gros jeu : %v", got)
	}
	if got := sourcesOf(copied.Roms["nes"]); strings.Join(got, ",") != "FTP,Samba" {
		t.Fatalf("petit jeu : %v", got)
	}
	// Un correctif d'un partage distant se sert (il n'était servi que depuis un disque).
	patch, err := client.OpenPatch("ps4", "CUSA07211")
	if err != nil {
		t.Fatalf("correctif d'un partage : %v", err)
	}
	patch.Close()

	// Épingler FTP le met en tête de tout, même pour un gros jeu.
	f.do("PUT", "/api/v1/admin/resources/2/pin", "admin", map[string]bool{"pinned": true})
	copied, _ = client.Listing()
	if got := sourcesOf(copied.Roms["ps4"]); got[0] != "FTP" || !copied.Roms["ps4"][0].Sources[0].Pinned {
		t.Fatalf("épinglée : %v", got)
	}
	f.do("PUT", "/api/v1/admin/resources/2/pin", "admin", map[string]bool{"pinned": false})

	// L'appareil peut demander une source précise.
	request, _ := http.NewRequest("GET", f.server.URL+"/api/v1/federation/roms/nes/"+copied.Roms["nes"][0].ID+"?source=1", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, _ := http.DefaultClient.Do(request)
	response.Body.Close()
	if response.Header.Get("X-GameFlex-Source") != "1" {
		t.Fatalf("?source=1 : servi par %q", response.Header.Get("X-GameFlex-Source"))
	}

	// FTP tombe : la petite ROM est servie par Samba, et FTP passe derrière dans le listing.
	f.cat.Forget(2, false)
	delete(f.shares, "ftp://nas")
	rom, err := client.OpenRom("nes", copied.Roms["nes"][0].ID)
	if err != nil {
		t.Fatalf("bascule : %v", err)
	}
	got, _ := io.ReadAll(rom)
	rom.Close()
	if string(got) != "ZELDA" {
		t.Fatalf("bascule : %q", got)
	}
	copied, _ = client.Listing()
	if names := sourcesOf(copied.Roms["nes"]); names[0] != "Samba" || !copied.Roms["nes"][0].Sources[1].Unavailable {
		t.Fatalf("une source tombée passe derrière : %v %+v", names, copied.Roms["nes"][0].Sources)
	}
}

// LES COMPTES DES FOURNISSEURS (P3) : ils entrent, se scellent, ne ressortent jamais — seuls les noms des champs remplis
// se lisent ; « configuré » quand tous les champs requis le sont ; oubliés d'un geste.
func TestMetadataAccountsGoInAndNeverComeOut(t *testing.T) {
	f := newFixture(t)
	if status, _, _ := f.do("GET", "/api/v1/admin/metadata", "", nil); status != 401 {
		t.Fatalf("sans session : %d", status)
	}
	status, _, raw := f.do("PUT", "/api/v1/admin/metadata/igdb", "admin", map[string]string{"client_id": "mon-client", "client_secret": "  "})
	if status != 200 || strings.Contains(string(raw), "mon-client") {
		t.Fatalf("enregistrer : %d %s", status, raw)
	}
	var listed struct {
		Data []struct {
			Service    string   `json:"service"`
			Configured bool     `json:"configured"`
			Fields     []string `json:"fields"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &listed)
	if len(listed.Data) != 2 || listed.Data[0].Service != "igdb" || listed.Data[0].Configured || strings.Join(listed.Data[0].Fields, ",") != "client_id" {
		t.Fatalf("liste après un champ : %+v", listed.Data)
	}
	f.do("PUT", "/api/v1/admin/metadata/igdb", "admin", map[string]string{"client_secret": "mon-secret"})
	accounts, err := f.store.MetadataAccounts()
	if err != nil || accounts["igdb"]["client_id"] != "mon-client" || accounts["igdb"]["client_secret"] != "mon-secret" {
		t.Fatalf("comptes : %v %v", accounts, err)
	}
	_, _, raw = f.do("GET", "/api/v1/admin/metadata", "admin", nil)
	_ = json.Unmarshal(raw, &listed)
	if !listed.Data[0].Configured || strings.Contains(string(raw), "mon-secret") {
		t.Fatalf("configuré : %s", raw)
	}
	if status, _, _ := f.do("PUT", "/api/v1/admin/metadata/inconnu", "admin", map[string]string{"x": "y"}); status != 404 {
		t.Fatalf("service inconnu : %d", status)
	}
	f.do("DELETE", "/api/v1/admin/metadata/igdb", "admin", nil)
	if accounts, _ := f.store.MetadataAccounts(); len(accounts["igdb"]) != 0 {
		t.Fatalf("après l'oubli : %v", accounts)
	}
}

// LE PARTAGE DES FOURNISSEURS (chantier 61, B) : la portée `metadata-providers` se choisit à la création (un jeton, une
// invitation qui la transmet), se change ensuite ; sans elle, le jeu d'un fournisseur est refusé, avec elle il est
// demandé à ses comptes (aucun ici : « pas configuré », jamais une clé) ; la recherche dans ses fiches répond toujours.
func TestTheMetadataProvidersAreSharedOnlyWithTheirScope(t *testing.T) {
	f := newFixture(t)
	_, body, _ := f.do("POST", "/api/v1/admin/tokens", "admin", map[string]any{"label": "Gamebox", "scopes": []string{"catalog", "metadata-providers", "devices"}})
	created := body["data"].(map[string]any)
	value, token := created["value"].(string), created["token"].(map[string]any)
	if scopes := fmt.Sprint(token["scopes"]); scopes != "[catalog metadata-providers]" {
		t.Fatalf("portées à la création : %s", scopes)
	}
	if status, body, _ := f.do("GET", "/api/v1/federation/metadata/nes/providers/igdb/1022", value, nil); status != 409 || body["errorCode"] != "service_not_configured" {
		t.Fatalf("un fournisseur partagé, sans compte : %d %v", status, body)
	}
	if status, _, _ := f.do("GET", "/api/v1/federation/metadata/nes/search?q=Zelda", value, nil); status != 200 {
		t.Fatalf("la recherche dans ses fiches : %d", status)
	}
	id := fmt.Sprint(token["id"])
	if status, body, _ := f.do("PATCH", "/api/v1/admin/tokens/"+id, "admin", map[string]any{"scopes": []string{"catalog"}}); status != 200 ||
		fmt.Sprint(body["data"].(map[string]any)["scopes"]) != "[catalog]" {
		t.Fatalf("retirer le partage : %d %v", status, body)
	}
	if status, _, _ := f.do("GET", "/api/v1/federation/metadata/nes/providers/igdb/1022", value, nil); status != 403 {
		t.Fatalf("un fournisseur non partagé : %d", status)
	}
	if status, _, _ := f.do("PATCH", "/api/v1/admin/tokens/999", "admin", map[string]any{"scopes": []string{"catalog"}}); status != 404 {
		t.Fatalf("un jeton inconnu : %d", status)
	}

	// L'invitation transmet ses portées au jeton qu'elle donne.
	_, body, _ = f.do("POST", "/api/v1/admin/invitations", "admin", map[string]any{"label": "S25", "scopes": []string{"catalog", "metadata-providers"}})
	code := body["data"].(map[string]any)["code"].(string)
	client, _ := remote.New(f.server.URL, "")
	redeemed, err := client.Redeem(code, remote.InvitationFederation, "S25")
	if err != nil {
		t.Fatal(err)
	}
	if status, body, _ := f.do("GET", "/api/v1/federation/identity", redeemed.Token, nil); status != 200 ||
		fmt.Sprint(body["data"].(map[string]any)["scopes"]) != "[catalog metadata-providers]" {
		t.Fatalf("les portées de l'invitation : %d %v", status, body)
	}
}
