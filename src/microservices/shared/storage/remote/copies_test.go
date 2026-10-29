package remote_test

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gameflex/storage/gamestatus"
	"gameflex/storage/listing"
	"gameflex/storage/remote"
)

func peerAt(fake *fakePeer, id int64) remote.Peer {
	return remote.Peer{ID: id, Link: fake.URL, Token: "bon-jeton"}
}

// LA COPIE SE PREND UNE FOIS, PUIS SUIT LA DATE (chantier 15) : on ne
// re-télécharge un ami que si son listing a avancé.
func TestUneCopieSePrendUneFoisPuisSuitLaDate(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	copies := &remote.Copies{Dir: t.TempDir(), Every: time.Nanosecond}
	peer := peerAt(fake, 3)

	header, err := copies.Fresh(peer)
	if err != nil {
		t.Fatalf("première copie : %v", err)
	}
	// LA COPIE DIT CE QU'ELLE EST, et ce que notre jeton nous y ouvre.
	if header.Source != listing.PeerSource || header.Name != "nas-du-salon" || len(header.Scopes) != 2 {
		t.Errorf("en-tête de la copie = %+v", header)
	}
	if served, _ := fake.counts(); served != 1 {
		t.Fatalf("listings servis = %d, attendu 1", served)
	}

	if _, err := copies.Fresh(peer); err != nil {
		t.Fatal(err)
	}
	if served, asked := fake.counts(); served != 1 || asked != 2 {
		t.Fatalf("même date : %d listings, %d dates — attendu 1 et 2", served, asked)
	}

	fake.mu.Lock()
	fake.listing = sampleListing(t0.Add(time.Hour))
	fake.listing.Roms["nes"] = append(fake.listing.Roms["nes"], listing.Item{ID: "zelda", Name: "Zelda"})
	fake.listing.Header.Sections[listing.Roms] = listing.SectionInfo{UpdatedAt: t0.Add(time.Hour), Consoles: map[string]int{"nes": 2, "ps1": 1}}
	fake.mu.Unlock()
	items, err := copies.Items(peer, listing.Roms, "nes")
	if err != nil || len(items) != 2 {
		t.Fatalf("après sa mise à jour : %+v (%v)", items, err)
	}
	if served, _ := fake.counts(); served != 2 {
		t.Fatalf("listings servis = %d, attendu 2", served)
	}

	// L'icône re-télécharge toujours.
	if _, err := copies.Refresh(peer); err != nil {
		t.Fatal(err)
	}
	if served, _ := fake.counts(); served != 3 {
		t.Fatalf("après l'icône : %d listings, attendu 3", served)
	}
}

// UNE PORTÉE ACCORDÉE CHEZ L'AMI se voit dans la copie à la question suivante,
// sans tout re-télécharger : elle ne change pas la date de son listing.
func TestUnePorteeAccordeeSeVoitSansReTelecharger(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	fake.mu.Lock()
	fake.listing.Header.Scopes = []string{"catalog"}
	fake.mu.Unlock()
	copies := &remote.Copies{Dir: t.TempDir(), Every: time.Nanosecond}
	peer := peerAt(fake, 3)
	if header, err := copies.Fresh(peer); err != nil || len(header.Scopes) != 1 {
		t.Fatalf("première copie : %+v (%v)", header.Scopes, err)
	}

	fake.mu.Lock()
	fake.listing.Header.Scopes = []string{"catalog", "devices"}
	fake.mu.Unlock()
	header, err := copies.Fresh(peer)
	if err != nil || len(header.Scopes) != 2 {
		t.Fatalf("après « Consoles » accordée : %+v (%v)", header.Scopes, err)
	}
	if kept, _ := copies.Header(peer); len(kept.Scopes) != 2 {
		t.Fatalf("la copie garde %v", kept.Scopes)
	}
	if served, _ := fake.counts(); served != 1 {
		t.Fatalf("listings servis = %d, attendu 1 : la portée ne re-télécharge rien", served)
	}
}

// LA FICHE D'UN JEU DEMANDE MAINTENANT (Check) : une portée retirée chez l'ami
// dans la minute se voit tout de suite — là où la liste, elle, attend.
func TestCheckDemandeSansAttendreLaMinute(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	copies := &remote.Copies{Dir: t.TempDir()}
	peer := peerAt(fake, 3)
	if header, err := copies.Fresh(peer); err != nil || len(header.Scopes) != 2 {
		t.Fatalf("première copie : %+v (%v)", header.Scopes, err)
	}

	fake.mu.Lock()
	fake.listing.Header.Scopes = []string{"catalog"}
	fake.mu.Unlock()
	if header, _ := copies.Fresh(peer); len(header.Scopes) != 2 {
		t.Fatalf("la liste, dans la minute, ne redemande pas : %v", header.Scopes)
	}
	header, err := copies.Check(peer)
	if err != nil || len(header.Scopes) != 1 || header.Scopes[0] != "catalog" {
		t.Fatalf("la fiche voit la portée retirée : %+v (%v)", header.Scopes, err)
	}
	if served, asked := fake.counts(); served != 1 || asked != 2 {
		t.Fatalf("%d listings, %d dates — attendu 1 et 2 : la portée ne re-télécharge rien", served, asked)
	}
}

// LA QUESTION SE RETIENT LA MINUTE : dix affichages ne font pas dix
// allers-retours, et un ami éteint ne fait pas attendre chacun d'eux.
func TestLaQuestionSeRetientLaMinute(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	copies := &remote.Copies{Dir: t.TempDir()}
	peer := peerAt(fake, 3)

	for i := 0; i < 5; i++ {
		if _, err := copies.Fresh(peer); err != nil {
			t.Fatal(err)
		}
	}
	if served, asked := fake.counts(); served != 1 || asked != 1 {
		t.Fatalf("cinq affichages : %d listings, %d dates — attendu 1 et 1", served, asked)
	}

	silent := newFakePeer(t, "bon-jeton")
	silent.Close()
	other := peerAt(silent, 4)
	_, first := copies.Fresh(other)
	_, second := copies.Fresh(other)
	if first == nil || first.Error() != second.Error() {
		t.Fatalf("un ami éteint : %v puis %v", first, second)
	}
	if got := remote.Refusal(first); got.Status != http.StatusBadGateway {
		t.Errorf("un ami éteint se dit %d, attendu 502", got.Status)
	}
}

// UN AMI QUI REFUSE NOTRE JETON PERD SA COPIE ; un ami muet garde la sienne.
func TestUnRefusEffaceLaCopieUnSilenceLaGarde(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	copies := &remote.Copies{Dir: t.TempDir(), Every: time.Nanosecond}
	peer := peerAt(fake, 3)
	if _, err := copies.Fresh(peer); err != nil {
		t.Fatal(err)
	}

	fake.Close()
	if _, err := copies.Fresh(peer); err == nil || errors.Is(err, remote.ErrDenied) {
		t.Fatalf("un ami éteint = %v", err)
	}
	if _, kept := copies.Header(peer); !kept {
		t.Fatal("un ami éteint a perdu sa copie")
	}

	refusing := newFakePeer(t, "bon-jeton")
	other := peerAt(refusing, 4)
	if _, err := copies.Fresh(other); err != nil {
		t.Fatal(err)
	}
	refusing.mu.Lock()
	refusing.denied = true
	refusing.mu.Unlock()
	_, err := copies.Fresh(other)
	if !errors.Is(err, remote.ErrDenied) || remote.Refusal(err).Status != http.StatusForbidden {
		t.Fatalf("un ami qui refuse = %v", err)
	}
	if _, kept := copies.Header(other); kept {
		t.Fatal("un ami qui refuse notre jeton a gardé sa copie")
	}
}

// UN JEU D'UN AMI SE TROUVE PAR SON UUID dans sa copie ; un inconnu répond 404.
func TestUnJeuDUnAmiSeTrouveParSonUUID(t *testing.T) {
	fake := newFakePeer(t, "bon-jeton")
	copies := &remote.Copies{Dir: t.TempDir()}
	peer := peerAt(fake, 3)

	game, err := copies.Game(peer, "nes", marioID)
	if err != nil || game.Name != "Super Mario Bros" {
		t.Fatalf("Mario = %+v (%v)", game, err)
	}
	var refused *remote.StatusError
	if _, err := copies.Game(peer, "nes", "inconnu"); !errors.As(err, &refused) || refused.Status != http.StatusNotFound {
		t.Fatalf("un jeu inconnu = %v, attendu 404", err)
	}
	bios, err := copies.SectionAll(peer, listing.Bios)
	if err != nil || len(bios["ps1"]) != 1 {
		t.Fatalf("BIOS = %+v (%v)", bios, err)
	}
}

// UN AMI RETIRÉ EMPORTE SA COPIE — et rien d'autre : ni le listing de ce
// serveur, ni celui d'une ressource.
func TestPruneNeRetireQueLesCopiesDesAmisPartis(t *testing.T) {
	dir := t.TempDir()
	copies := &remote.Copies{Dir: dir}
	kept, gone := newFakePeer(t, "bon-jeton"), newFakePeer(t, "bon-jeton")
	for index, fake := range []*fakePeer{kept, gone} {
		if _, err := copies.Refresh(peerAt(fake, int64(index+1))); err != nil {
			t.Fatal(err)
		}
	}
	for key, source := range map[string]string{listing.SelfKey: listing.SelfSource, "partage": "webdav"} {
		if err := listing.Save(dir, key, listing.New(source, key)); err != nil {
			t.Fatal(err)
		}
	}

	// Prise à l'instant : elle reste, la liste comparée a pu être lue avant
	// l'ajout de cet ami.
	copies.Prune([]remote.Peer{peerAt(kept, 1)})
	if _, ok := copies.Header(peerAt(gone, 2)); !ok {
		t.Fatal("une copie prise à l'instant a été retirée")
	}
	old := time.Now().Add(-2 * listing.PruneGrace)
	if err := os.Chtimes(filepath.Join(dir, remote.Key(peerAt(gone, 2)), "listing.json"), old, old); err != nil {
		t.Fatal(err)
	}

	copies.Prune([]remote.Peer{peerAt(kept, 1)})

	if _, ok := copies.Header(peerAt(kept, 1)); !ok {
		t.Error("la copie d'un ami gardé a disparu")
	}
	if _, ok := copies.Header(peerAt(gone, 2)); ok {
		t.Error("la copie d'un ami retiré est restée")
	}
	for _, key := range []string{listing.SelfKey, "partage"} {
		if !listing.Scanned(dir, key) {
			t.Errorf("%s a été retiré avec les amis", key)
		}
	}
}

// LES JEUX D'UN AMI, À LA FORME DE L'ÉCRAN : console par console, la référence
// est l'UUID, un dossier le dit.
func TestRomSectionsALaFormeDeLEcran(t *testing.T) {
	sections := remote.RomSections(sampleListing(t0).Roms, nil)
	if len(sections) != 2 || sections[0].ConsoleID != "nes" || sections[0].Console == "nes" {
		t.Fatalf("sections = %+v", sections)
	}
	if mario := sections[0].Roms[0]; mario.Reference != marioID || mario.Title != "Super Mario Bros" || mario.Folder {
		t.Errorf("Mario = %+v", mario)
	}
	if ace := sections[1].Roms[0]; ace.Reference != aceID || !ace.Folder || len(ace.Files) != 2 {
		t.Errorf("Ace Combat = %+v", ace)
	}
}

// LE STATUT DES JEUX D'UN AMI (chantier 30) : celui qu'il a établi chez lui
// — ses signalements, et le statut de sa console pour les autres.
func TestRomSectionsPortentLeStatutDeLAmi(t *testing.T) {
	roms := sampleListing(t0).Roms
	roms["nes"][0].Reports = []gamestatus.Report{{Status: gamestatus.Crashes, Note: "plante au niveau 4", By: "alex", At: t0}}
	sections := remote.RomSections(roms, map[string]string{"nes": gamestatus.Playable})
	if mario := sections[0].Roms[0]; mario.Status == nil || mario.Status.Status != gamestatus.Crashes || len(mario.Status.Notes) != 1 {
		t.Fatalf("Mario = %+v", mario.Status)
	}
	if ace := sections[1].Roms[0]; ace.Status == nil || ace.Status.Status != gamestatus.Playable {
		t.Fatalf("un jeu d'une console rétro sans statut doit être jouable : %+v", ace.Status)
	}
}

// LES CORRECTIFS, À LA FORME DE L'ÉCRAN : console par console, et « aussi ici »
// quand on les a.
func TestPatchSectionsDisentCeQuiEstAussiIci(t *testing.T) {
	sections := remote.PatchSections(map[string][]listing.Item{
		"wiiu":    {{Name: "NewSuperMarioBrosU", Dir: true, Files: []string{"CrashFix"}, Size: 5}},
		"bizarre": {{Name: "x.bin", Size: 1}},
	}, func(consoleID, name string) bool { return name == "NewSuperMarioBrosU" })
	if len(sections) != 2 || sections[0].ConsoleID != "wiiu" || sections[1].ConsoleID != "bizarre" {
		t.Fatalf("sections = %+v", sections)
	}
	if pack := sections[0].Items[0]; !pack.Dir || !pack.Installed || pack.Files[0] != "CrashFix" {
		t.Fatalf("pack = %+v", pack)
	}
	for _, name := range []string{"", ".", "..", ".cache", "a/b", `a\b`} {
		if remote.PatchName(name) {
			t.Errorf("%q accepté comme nom de correctif", name)
		}
	}
	if !remote.PatchName("NewSuperMarioBrosU") {
		t.Error("un nom ordinaire refusé")
	}
}
