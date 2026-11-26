// Package catalog — LES RESSOURCES DU SERVICE, VUES COMME UN SEUL SERVEUR FÉDÉRÉ (chantier 61).
//
// Chaque source — un disque monté (ressource interne) ou un partage (Samba, WebDAV, kDrive, FTP, SFTP) — est parcourue
// pour toutes ses sections, rangées comme chez GameFlex : ses jeux par le scan de GameFlex (`listing.Build`, la
// convention `roms/<console>/`), ses cœurs, BIOS et correctifs (internal/library), ses paramètres (recopiés dans le
// volume, d'où gameflex/emuparams les lit). Elles se fondent en UN listing, celui qu'un appareil copie chez lui comme
// celui de n'importe quel ami.
//
// PLUSIEURS SOURCES POUR UN MÊME ÉLÉMENT (le porteur, 2026-11-18) : une seule ligne par élément, qui porte ses sources
// (listing.Item.Sources), du meilleur au moins bon (rank.go) ; l'appareil prend la première ou en demande une autre, et
// le service bascule sur la suivante quand une source lâche.
//
// Pas de sondage : un partage ne prévient de rien, le parcours se fait à l'ajout d'une ressource, au démarrage pour un
// disque, et par le geste « Relire ». Le catalogue vit dans le volume de données : un redémarrage le relit.
package resources

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gameflex/emuparams"
	"gameflex/resources/library"
	"gameflex/resources/sources"
	"gameflex/resources/sources/kdrive"
	"gameflex/resources/sources/smb"
	"gameflex/resources/sources/webdav"
	"gameflex/storage"
	"gameflex/storage/listing"
)

// State — ce que l'administration voit d'une source : son dernier parcours, et son débit mesuré.
type State struct {
	Scanning  bool      `json:"scanning"`
	ScannedAt time.Time `json:"scannedAt,omitzero"`
	Games     int       `json:"games"`
	Error     string    `json:"error,omitempty"`
	// Speed — le débit réel de ses derniers transferts, en octets par seconde (moyenne glissante) ; 0 tant qu'elle n'a
	// rien servi.
	Speed float64 `json:"speedBytesPerSecond,omitempty"`
	// Empty — lue sans erreur, mais sans aucun dossier de bibliothèque (`roms/`, `bios/`, `cores/`, `patches/`) là où
	// elle commence ; Suggest — les dossiers, un niveau plus bas, qui en ont un : ce que « Dossier » devrait dire (le
	// porteur, 2026-11-23 : un kDrive rangé sous /gameflex/roms, Dossier laissé vide, « je ne trouve aucun jeu »).
	Empty   bool     `json:"empty,omitempty"`
	Suggest []string `json:"suggest,omitempty"`
	// Problem — ce que dit `Error`, rangé pour la page (le porteur, 2026-11-23 : « le statut de la ressource comme
	// badge : online, offline, unreachable, etc. ») : `offline` (le serveur ne répond pas), `refused` (le compte est
	// refusé), `folder` (le dossier n'existe pas), `error` (le reste). Vide quand tout va.
	Problem string `json:"problem,omitempty"`
}

// classify — le genre d'une erreur de source (State.Problem). Chaque protocole dit un refus à sa façon (401 en WebDAV et
// kDrive, 530 en FTP, STATUS_LOGON_FAILURE en Samba, « unable to authenticate » en SFTP) : on les reconnaît à leur texte.
func classify(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, os.ErrNotExist) {
		return "folder"
	}
	var netErr net.Error
	var opErr *net.OpError
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) || errors.As(err, &opErr) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return "offline"
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"connection refused", "no route to host", "i/o timeout", "no such host", "network is unreachable", "dial tcp", "eof"} {
		if strings.Contains(text, marker) {
			return "offline"
		}
	}
	for _, marker := range []string{"401", "403", "unauthorized", "forbidden", "unable to authenticate", "logon_failure", "logon failure", "530", "login", "permission denied", "access denied"} {
		if strings.Contains(text, marker) {
			return "refused"
		}
	}

	return "error"
}

// found — tout ce qu'une source porte, par section.
type found struct {
	Sections map[string]library.Found `json:"sections"`
	// Params — la racine d'où emuparams lit ses paramètres (le disque lui-même, ou le miroir dans le volume) ; vide :
	// aucun.
	Params string `json:"params,omitempty"`
}

// persisted — ce qui se garde dans le volume entre deux démarrages.
type persisted struct {
	UpdatedAt time.Time        `json:"updatedAt"`
	Digest    string           `json:"digest"`
	Found     map[int64]*found `json:"found"`
	States    map[int64]State  `json:"states"`
}

// Catalog — le listing fondu, ses emplacements, et les sources ouvertes.
type Catalog struct {
	store   *Store
	path    string
	dataDir string
	name    func() string

	// OpenSource — remplace l'ouverture d'une ressource (les tests y branchent une source en mémoire).
	OpenSource func(Resource) (storage.Source, error)

	mu      sync.Mutex
	data    persisted
	sources map[int64]storage.Source
	changed chan struct{}
}

// New — le catalogue de `dataDir`, relu s'il a déjà été fait. `name` rend le nom du serveur au moment de servir.
func New(st *Store, dataDir string, name func() string) *Catalog {
	c := &Catalog{
		store:   st,
		path:    filepath.Join(dataDir, "catalog.json"),
		dataDir: dataDir,
		name:    name,
		sources: map[int64]storage.Source{},
		changed: make(chan struct{}),
		data:    persisted{Found: map[int64]*found{}, States: map[int64]State{}},
	}
	if text, err := os.ReadFile(c.path); err == nil {
		var kept persisted
		if json.Unmarshal(text, &kept) == nil && kept.Found != nil {
			if kept.States == nil {
				kept.States = map[int64]State{}
			}
			// UN PARCOURS NE SURVIT PAS À UN REDÉMARRAGE : écrit « en cours » avant l'arrêt, il refusait sinon toute
			// relecture pour toujours (le 2026-11-23).
			for id, state := range kept.States {
				state.Scanning = false
				kept.States[id] = state
			}
			c.data = kept
		}
	}

	return c
}

// ── Les sources ────────────────────────────────────────────────────────────

// resources — toutes les sources connues : les ressources enregistrées, puis les disques présents.
func (c *Catalog) resources() []Resource {
	resources, err := c.store.Resources()
	if err != nil {
		slog.Warn("ressources illisibles", "error", err)
		resources = nil
	}

	return resources
}

func (c *Catalog) resource(id int64) (Resource, bool) {
	for _, r := range c.resources() {
		if r.ID == id {
			return r, true
		}
	}

	return Resource{}, false
}

// open — la source d'une ressource (gardée ouverte : une connexion SMB coûte cher à refaire).
func (c *Catalog) open(r Resource) (storage.Source, error) {
	c.mu.Lock()
	kept, ok := c.sources[r.ID]
	c.mu.Unlock()
	if ok {
		return kept, nil
	}
	var source storage.Source
	var err error
	if c.OpenSource != nil {
		source, err = c.OpenSource(r)
	} else {
		source, err = c.openResource(r)
	}
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.sources[r.ID] = source
	c.mu.Unlock()

	return source, nil
}

// openResource — la source d'une ressource, selon son genre.
func (c *Catalog) openResource(r Resource) (storage.Source, error) {
	var source storage.Source
	var err error
	switch r.Kind {
	case "kdrive":
		// kDrive parle son API REST : l'adresse est le NUMÉRO du drive, le « mot de passe » un jeton d'API.
		source, err = kdrive.New(kdrive.Config{Name: r.Name, DriveID: r.Address, Token: r.Password, Folder: r.Folder})
	case "webdav":
		// Le dossier fait partie de l'adresse : le scan part de la racine de la source.
		source, err = webdav.New(webdav.Config{Name: r.Name, BaseURL: storage.ShareRoot(r.Address, r.Folder),
			Username: r.Username, Password: r.Password})
	case "smb":
		source, err = smb.New(smb.Config{Name: r.Name, Address: r.Address, Folder: r.Folder,
			Username: r.Username, Password: r.Password})
	case sources.KindFTP:
		source, err = sources.NewFTP(sources.FTPConfig{Name: r.Name, Address: r.Address, Folder: r.Folder,
			Username: r.Username, Password: r.Password})
	case sources.KindSFTP:
		// La clé du serveur, retenue à la première connexion et vérifiée ensuite, vit dans les réglages du service.
		setting := sftpHostKeySetting(r.ID)
		source, err = sources.NewSFTP(sources.SFTPConfig{Name: r.Name, Address: r.Address, Folder: r.Folder,
			Username: r.Username, Password: r.Password, HostKey: c.store.Setting(setting, ""),
			Remember: func(key string) { _ = c.store.SetSetting(setting, key) }})
	default:
		err = errors.New("genre de ressource inconnu : " + r.Kind)
	}

	return source, err
}

// Forget — ferme la source d'une ressource changée ou retirée ; `drop` retire aussi ce qu'elle portait.
func (c *Catalog) Forget(id int64, drop bool) {
	c.mu.Lock()
	source := c.sources[id]
	delete(c.sources, id)
	if drop {
		delete(c.data.Found, id)
		delete(c.data.States, id)
		_ = os.RemoveAll(c.mirror(id))
		c.commitLocked()
	}
	c.mu.Unlock()
	if closer, ok := source.(io.Closer); ok {
		_ = closer.Close()
	}
}

// mirror — où se recopient les paramètres d'un partage.
func (c *Catalog) mirror(id int64) string {
	return filepath.Join(c.dataDir, "mirror", strconv.FormatInt(id, 10))
}

// ── Le parcours ────────────────────────────────────────────────────────────

// Rescan — parcourt une ressource en arrière-plan ; faux si un parcours est déjà en cours.
func (c *Catalog) Rescan(r Resource) bool {
	c.mu.Lock()
	if c.data.States[r.ID].Scanning {
		c.mu.Unlock()

		return false
	}
	state := c.data.States[r.ID]
	state.Scanning = true
	c.data.States[r.ID] = state
	c.wakeLocked()
	c.mu.Unlock()

	go c.scan(r)

	return true
}

func (c *Catalog) scan(r Resource) {
	started := time.Now()
	result, err := c.build(r)
	// LES DOSSIERS À SUGGÉRER, AVANT LE VERROU : suggest rouvre la source (c.open), qui prend ce verrou — sous lui, le
	// catalogue se bloquait pour de bon (le 2026-11-23, sur un kDrive vide à sa racine).
	var suggest []string
	if err == nil && isEmpty(result) {
		suggest = c.suggest(r)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.data.States[r.ID]
	state.Scanning, state.ScannedAt = false, time.Now().UTC()
	if err != nil {
		// Un partage injoignable garde ce qu'il portait : une coupure réseau n'efface pas un catalogue.
		state.Error = err.Error()
		// Un dossier qui n'existe pas se nomme (« file does not exist » ne disait pas lequel).
		if errors.Is(err, os.ErrNotExist) && strings.Trim(r.Folder, "/") != "" {
			state.Error = fmt.Sprintf("the folder %q does not exist on this resource", strings.Trim(r.Folder, "/"))
		}
		state.Problem = classify(err)
		slog.Warn("parcours d'une ressource en échec", "resource", r.Name, "error", err)
	} else {
		state.Error, state.Problem = "", ""
		c.data.Found[r.ID] = result
		state.Games = 0
		for _, list := range result.Sections[listing.Roms].Items {
			state.Games += len(list)
		}
		state.Empty, state.Suggest = isEmpty(result), suggest
		slog.Info("ressource parcourue", "resource", r.Name, "games", state.Games, "took", time.Since(started).Round(time.Millisecond))
	}
	c.data.States[r.ID] = state
	c.commitLocked()
}

// build — le parcours d'une ressource : toutes ses sections, et où retrouver chaque élément.
func (c *Catalog) build(r Resource) (*found, error) {
	source, err := c.open(r)
	if err != nil {
		return nil, err
	}
	// La source répond-elle ? Une connexion gardée peut être morte (le NAS a redémarré) : une seule nouvelle tentative,
	// sur une neuve.
	if _, err := source.List(""); err != nil {
		c.Forget(r.ID, false)
		if source, err = c.open(r); err != nil {
			return nil, err
		}
		if _, err := source.List(""); err != nil {
			return nil, err
		}
	}
	roms := library.Found{Items: map[string][]listing.Item{}, Where: map[string][]library.Locator{}}
	// Un partage sans `roms/` n'est pas une erreur : il peut n'apporter que des BIOS ou des correctifs.
	if built, err := listing.Build(source, time.Now()); err == nil {
		prefix := source.Name() + "/"
		for console, list := range built.Roms {
			for _, item := range list {
				where := library.Locator{Path: strings.TrimPrefix(item.Path, prefix), Size: item.Size}
				if item.Folder != "" {
					where = library.Locator{Path: strings.TrimPrefix(item.Folder, prefix), Dir: true, Size: item.Size}
				}
				// Le listing servi ne dit pas où est un jeu chez nous : un appareil le demande par son identifiant.
				item.Path, item.Folder = "", ""
				roms.Items[console] = append(roms.Items[console], item)
				roms.Where[console] = append(roms.Where[console], where)
			}
		}
	}
	result := &found{Sections: map[string]library.Found{
		listing.Roms:    roms,
		listing.Bios:    library.ScanBios(source),
		listing.Cores:   library.ScanCores(source),
		listing.Patches: library.ScanPatches(source),
	}}
	if has, err := library.MirrorParams(source, filepath.Join(c.mirror(r.ID), "params")); err != nil {
		slog.Warn("paramètres d'une ressource non recopiés", "resource", r.Name, "error", err)
	} else if has {
		result.Params = c.mirror(r.ID)
	}

	return result, nil
}

// commitLocked — refait l'empreinte ; si le contenu a changé, la date avance. Réveille les abonnés et écrit le volume.
func (c *Catalog) commitLocked() {
	sum := sha256.New()
	for _, id := range c.idsLocked() {
		text, _ := json.Marshal(c.data.Found[id])
		sum.Write([]byte(strconv.FormatInt(id, 10)))
		sum.Write(text)
	}
	if digest := hex.EncodeToString(sum.Sum(nil)); digest != c.data.Digest {
		c.data.Digest = digest
		c.data.UpdatedAt = time.Now().UTC()
	}
	c.saveLocked()
}

// saveLocked — réveille les abonnés et écrit le catalogue dans le volume.
func (c *Catalog) saveLocked() {
	c.wakeLocked()
	if text, err := json.Marshal(c.data); err == nil {
		tmp := c.path + ".tmp"
		if os.WriteFile(tmp, text, 0o600) == nil {
			_ = os.Rename(tmp, c.path)
		}
	}
}

func (c *Catalog) idsLocked() []int64 {
	ids := make([]int64, 0, len(c.data.Found))
	for id := range c.data.Found {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	return ids
}

// Changed — un canal qui se ferme au prochain changement (parcours commencé ou fini, listing changé).
func (c *Catalog) Changed() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.changed
}

func (c *Catalog) wakeLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// States — l'état de chaque source.
func (c *Catalog) States() map[int64]State {
	c.mu.Lock()
	defer c.mu.Unlock()
	states := make(map[int64]State, len(c.data.States))
	for id, state := range c.data.States {
		states[id] = state
	}

	return states
}

// UpdatedAt — la date du listing : ce qu'un appareil compare pour savoir s'il doit le recopier.
func (c *Catalog) UpdatedAt() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.data.UpdatedAt
}

// ── Ce qu'on apprend en servant ────────────────────────────────────────────

// Served — un transfert réussi depuis une source : son débit entre dans la moyenne glissante, et la source est
// disponible. Les petits transferts (moins de 1 Mo) ne mesurent que la latence : ils ne comptent pas dans le débit.
func (c *Catalog) Served(id int64, bytes int64, took time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.data.States[id]
	changed := state.Error != ""
	state.Error, state.Problem = "", ""
	if bytes >= 1<<20 && took > 0 {
		speed := float64(bytes) / took.Seconds()
		if state.Speed == 0 {
			state.Speed = speed
		} else {
			state.Speed = 0.7*state.Speed + 0.3*speed
		}
		changed = true
	}
	c.data.States[id] = state
	if changed {
		c.saveLocked()
	}
}

// Failed — une source n'a pas pu servir : elle passe en dernier jusqu'à son prochain succès.
func (c *Catalog) Failed(id int64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.data.States[id]
	state.Error, state.Problem = err.Error(), classify(err)
	c.data.States[id] = state
	slog.Warn("une source n'a pas pu servir", "source", id, "error", err)
	c.saveLocked()
}

// ── Les paramètres ─────────────────────────────────────────────────────────

// LendParams — les paramètres d'un émulateur, chez la première source qui les prête (`preferred` d'abord s'il est
// donné, puis les autres dans l'ordre des sources).
func (c *Catalog) LendParams(ref emuparams.Ref, preset string, preferred int64) (emuparams.Lent, error) {
	for _, root := range c.paramsRoots(preferred) {
		if lent, err := (emuparams.Store{Root: root}).Lend(ref, preset); err == nil {
			return lent, nil
		}
	}

	return emuparams.Lent{}, emuparams.ErrNoPreset
}

func (c *Catalog) paramsRoots(preferred int64) []string {
	kinds := c.kinds()
	c.mu.Lock()
	defer c.mu.Unlock()
	roots := []string{}
	if f, ok := c.data.Found[preferred]; ok && f.Params != "" {
		roots = append(roots, f.Params)
	}
	ids := c.idsLocked()
	c.sortSourcesLocked(ids, kinds, false)
	for _, id := range ids {
		if f := c.data.Found[id]; f.Params != "" && id != preferred {
			roots = append(roots, f.Params)
		}
	}

	return roots
}

// Watch — un réveil à chaque changement du catalogue (parcours commencé ou fini, une source tombée ou revenue), pour
// un abonné qui veut des signaux répétés (facade.PushOnChange) ; `stop` le libère.
func (c *Catalog) Watch() (<-chan struct{}, func()) {
	wake := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-c.Changed():
				select {
				case wake <- struct{}{}:
				default:
				}
			case <-done:
				return
			}
		}
	}()

	return wake, func() { close(done) }
}

// isEmpty — une ressource lue qui ne porte rien : ni jeu, ni BIOS, ni cœur, ni correctif, ni paramètres.
func isEmpty(f *found) bool {
	if f.Params != "" {
		return false
	}
	for _, section := range f.Sections {
		for _, items := range section.Items {
			if len(items) > 0 {
				return false
			}
		}
	}

	return true
}

// libraryFolders — les dossiers qui font d'un dossier une bibliothèque GameFlex.
var libraryFolders = map[string]bool{"roms": true, "bios": true, "cores": true, "patches": true}

// suggest — les dossiers, juste sous le début de la ressource, qui portent une bibliothèque : le « Dossier » à donner,
// le sien devant (un `Dossier` déjà rempli mais trop haut). Dix sous-dossiers regardés au plus : un partage peut en avoir
// des milliers, et un parcours vide ne doit pas devenir une exploration.
func (c *Catalog) suggest(r Resource) []string {
	// « Dossier » nomme déjà le dossier des jeux (`…/roms`) : c'est son parent qui porte la bibliothèque.
	folder := strings.Trim(r.Folder, "/")
	if base := path.Base(folder); folder != "" && libraryFolders[strings.ToLower(base)] {
		if parent := path.Dir(folder); parent != "." {
			return []string{parent}
		}

		return []string{"/"}
	}
	source, err := c.open(r)
	if err != nil {
		return nil
	}
	entries, err := source.List("")
	if err != nil {
		return nil
	}
	found := []string{}
	looked := 0
	for _, entry := range entries {
		if !entry.Dir || strings.HasPrefix(entry.Name, ".") || looked >= 10 {
			continue
		}
		looked++
		children, err := source.List(entry.Name)
		if err != nil {
			continue
		}
		for _, child := range children {
			if child.Dir && libraryFolders[strings.ToLower(child.Name)] {
				found = append(found, strings.Trim(strings.Trim(r.Folder, "/")+"/"+entry.Name, "/"))

				break
			}
		}
	}

	return found
}
