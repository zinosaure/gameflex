package docker

import (
	"errors"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
)

// LA BOUCLE DE LANCEMENT, SOUS UN DÉMON JOUÉ.
//
// Trois copies, et aucun test : or elle a déjà eu ses vrais défauts, dont un
// bloc UDP qui fuyait à chaque conflit de nom (les blocs sont partis avec le
// WebRTC des sessions, #257). Le démon est joué ; l'allocateur est le vrai,
// parce que c'est sa comptabilité — port banni, port rendu — que ces tests
// surveillent. La plage est haute pour que la sonde de bind passe.

var (
	errNameConflict = errors.New("Conflict. The container name /x-001 is already in use by container abc")
	errImageMissing = errors.New("No such image: gameflex-retrogaming:latest")
	errPortConflict = errors.New("Bind for 0.0.0.0:47000 failed: port is already allocated")
	errDaemon       = errors.New("daemon exploded")
)

// fakeDaemon rejoue un script : le n-ième create rend createErrs[n], le
// n-ième start rend startErrs[n] (nil au-delà), et chaque geste est noté.
type fakeDaemon struct {
	createErrs []error
	startErrs  []error

	created []string
	starts  int
	removed []string
	started []string
	missing []string
	built   []int // ports reçus par build, dans l'ordre
}

func (d *fakeDaemon) loop(t *testing.T, portRange string) spawnLoop {
	t.Helper()
	ports, err := newPortAllocator(portRange)
	if err != nil {
		t.Fatal(err)
	}

	return spawnLoop{
		create: func(name string, _ *container.Config, _ *container.HostConfig) (string, error) {
			d.created = append(d.created, name)
			if n := len(d.created) - 1; n < len(d.createErrs) && d.createErrs[n] != nil {
				return "", d.createErrs[n]
			}
			return "id-" + name, nil
		},
		start: func(string) error {
			n := d.starts
			d.starts++
			if n < len(d.startErrs) {
				return d.startErrs[n]
			}
			return nil
		},
		remove:         func(id string) { d.removed = append(d.removed, id) },
		ports:          ports,
		onStarted:      func(name string) { d.started = append(d.started, name) },
		onImageMissing: func(image string) bool { d.missing = append(d.missing, image); return true },
		portRange:      portRange,
	}
}

func (d *fakeDaemon) build(port int) (*container.Config, *container.HostConfig) {
	d.built = append(d.built, port)
	return &container.Config{Image: "gameflex-retrogaming:latest"}, &container.HostConfig{}
}

func held(allocator *PortAllocator, port int) bool { return allocator.blacklist[port] }

// Le cas nominal : le premier numéro, et exactement le port que build a reçu.
func TestSpawnStartsUnderTheFirstNumberWithWhatBuildReceived(t *testing.T) {
	d := &fakeDaemon{}
	loop := d.loop(t, "47000-47009")

	port, name, err := loop.run("retro-nes", 0, d.build)
	if err != nil {
		t.Fatalf("run : %v", err)
	}
	if name != "retro-nes-001" {
		t.Errorf("nom = %q, want retro-nes-001 (un index < 1 part de 1)", name)
	}
	if len(d.built) != 1 || d.built[0] != port {
		t.Errorf("build a reçu %v, run rend %d", d.built, port)
	}
	if len(d.started) != 1 || d.started[0] != name || len(d.removed) != 0 {
		t.Errorf("journaux suivis %v, conteneurs retirés %v", d.started, d.removed)
	}
}

// Deux lancements sur le même numéro : on prend le suivant.
func TestSpawnTakesTheNextNumberOnANameConflict(t *testing.T) {
	d := &fakeDaemon{createErrs: []error{errNameConflict}}
	loop := d.loop(t, "47000-47009")

	_, name, err := loop.run("retro-nes", 3, d.build)
	if err != nil {
		t.Fatalf("run : %v", err)
	}
	if got := strings.Join(d.created, ","); got != "retro-nes-003,retro-nes-004" || name != "retro-nes-004" {
		t.Errorf("noms essayés %s, obtenu %q", got, name)
	}
}

// Un port pris côté hôte : le conteneur est retiré, le port banni, et on
// recommence sous le même nom, puisque le conteneur n'existe plus.
func TestSpawnBansThePortOnAPortConflictThenRetries(t *testing.T) {
	d := &fakeDaemon{startErrs: []error{errPortConflict}}
	loop := d.loop(t, "47000-47009")

	port, name, err := loop.run("retro-nes", 1, d.build)
	if err != nil {
		t.Fatalf("run : %v", err)
	}
	firstPort := d.built[0]
	if len(d.removed) != 1 || d.removed[0] != "id-retro-nes-001" {
		t.Errorf("conteneurs retirés %v, want [id-retro-nes-001]", d.removed)
	}
	if !held(loop.ports, firstPort) {
		t.Errorf("le port %d en conflit doit être banni", firstPort)
	}
	if port == firstPort || name != "retro-nes-001" {
		t.Errorf("relance sur (%d, %q), want un port neuf sous le même nom", port, name)
	}
}

// L'image manque : le seul échec sur lequel quelqu'un peut agir. Il se dit par
// sa propre erreur, lance la reconstruction et ne démarre rien.
func TestSpawnReportsAMissingImage(t *testing.T) {
	d := &fakeDaemon{createErrs: []error{errImageMissing}}
	loop := d.loop(t, "47000-47009")

	_, _, err := loop.run("retro-nes", 1, d.build)
	if !errors.Is(err, ErrImageMissing) {
		t.Fatalf("err = %v, want ErrImageMissing", err)
	}
	if len(d.missing) != 1 || d.missing[0] != "gameflex-retrogaming:latest" {
		t.Errorf("reconstruction demandée pour %v", d.missing)
	}
	if d.starts != 0 {
		t.Errorf("%d démarrage(s) tenté(s)", d.starts)
	}
}

// Une installation sans sources ne reconstruit rien : l'erreur dit que l'image
// n'est pas installée — pas « reconstruite » (#136).
func TestSpawnSaysAnImageIsNotInstalledWhenNothingCanBuildIt(t *testing.T) {
	d := &fakeDaemon{createErrs: []error{errImageMissing}}
	loop := d.loop(t, "47000-47009")
	loop.onImageMissing = func(string) bool { return false }

	_, _, err := loop.run("retro-nes", 1, d.build)
	if !errors.Is(err, ErrImageNotInstalled) || errors.Is(err, ErrImageMissing) {
		t.Fatalf("err = %v, want ErrImageNotInstalled", err)
	}
	if d.starts != 0 {
		t.Errorf("%d démarrage(s) tenté(s)", d.starts)
	}
}

// Tout autre échec s'arrête là : une panne passagère du démon ne doit pas
// boucler.
func TestSpawnStopsOnAnyOtherFailure(t *testing.T) {
	for _, c := range []struct {
		name       string
		daemon     fakeDaemon
		wantErr    string
		wantRemove int
	}{
		{"à la création", fakeDaemon{createErrs: []error{errDaemon}}, "create container", 0},
		{"au démarrage", fakeDaemon{startErrs: []error{errDaemon}}, "start container", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := c.daemon
			loop := d.loop(t, "47000-47009")

			_, _, err := loop.run("retro-nes", 1, d.build)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want %q", err, c.wantErr)
			}
			if len(d.created) != 1 || len(d.removed) != c.wantRemove || len(d.started) != 0 {
				t.Errorf("créés %v, retirés %v, suivis %v", d.created, d.removed, d.started)
			}
		})
	}
}

// QUAND LES TENTATIVES S'ÉPUISENT, UN PORT BANNI RESTE BANNI : un port qu'on
// sait pris côté hôte ne doit pas revenir au pool, où le lancement suivant
// irait s'y heurter.
func TestSpawnKeepsAPortConflictBanWhenAttemptsRunOut(t *testing.T) {
	d := &fakeDaemon{startErrs: []error{errPortConflict, errPortConflict}}
	loop := d.loop(t, "47000-47001")

	if _, _, err := loop.run("retro-nes", 1, d.build); err == nil || !strings.Contains(err.Error(), "no free host port") {
		t.Fatalf("err = %v, want « no free host port »", err)
	}
	if len(d.built) != 2 {
		t.Fatalf("%d tentative(s), want 2", len(d.built))
	}
	for _, port := range d.built {
		if !held(loop.ports, port) {
			t.Errorf("le port %d, en conflit, est revenu au pool", port)
		}
	}
}

// CE QU'UNE SESSION RÉ-ADOPTÉE TIENT N'EST PLUS DISTRIBUÉ (#103). Au démarrage
// l'allocateur repart vide : sans réservation, la première session neuve
// recevait le port d'une ancienne encore vivante — et l'ancien enfant, qui y
// répond toujours, la déclarait prête à sa place.
func TestAnAdoptedSessionKeepsItsPort(t *testing.T) {
	ports, err := newPortAllocator("47200-47201")
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{allocator: ports}

	client.reserveAdopted(47200)

	if port, err := ports.Allocate(); err != nil || port != 47201 {
		t.Fatalf("Allocate = %d, %v ; want 47201 — 47200 appartient à la session adoptée", port, err)
	}

	// La session adoptée s'arrête : son port revient au pool.
	client.ReleasePort(47200)
	if held(ports, 47200) {
		t.Fatal("le port d'une session adoptée arrêtée est resté hors du pool")
	}
}

// UNE PARTIE A AU MOINS UNE MANETTE PAR PLACE (#443) : la Switch offre huit
// places, le plafond du conteneur est de quatre — sans quoi les joueurs 5 à 8
// tenaient une place sans manette.
func TestPadsCoverTheSeats(t *testing.T) {
	for _, c := range []struct{ ceiling, seats, want int }{
		{4, 8, 8}, // Switch
		{4, 2, 4}, // Wii U : le conteneur peut en porter plus
		{4, 0, 4}, // console inconnue du registre
	} {
		if got := padsFor(c.ceiling, c.seats); got != c.want {
			t.Errorf("padsFor(%d, %d) = %d, want %d", c.ceiling, c.seats, got, c.want)
		}
	}
}
