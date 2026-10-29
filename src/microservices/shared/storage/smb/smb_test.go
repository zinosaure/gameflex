package smb

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/hirochachacha/go-smb2"

	"gameflex/storage"
)

func TestAddressComposesOneCanonicalForm(t *testing.T) {
	cases := []struct {
		host  string
		port  int
		share string
		want  string
	}{
		{"192.168.1.20", 0, "jeux", "smb://192.168.1.20:445/jeux"},
		{"nas", 1445, "jeux", "smb://nas:1445/jeux"},
		{"  nas  ", 445, " /jeux/ ", "smb://nas:445/jeux"},
		// Ce que Windows affiche, recopié tel quel dans le champ du serveur.
		{`\\nas\jeux`, 445, "", "smb://nas:445/jeux"},
		{`\\nas\jeux\roms`, 445, "", "smb://nas:445/jeux/roms"},
		// Ce que macOS et Linux écrivent — et son port l'emporte.
		{"smb://nas:1445/jeux", 445, "", "smb://nas:1445/jeux"},
		{"SMB://nas", 0, "jeux", "smb://nas:445/jeux"},
		{"nas", 0, "Mes Jeux", "smb://nas:445/Mes%20Jeux"},
		{"fe80::1", 0, "jeux", "smb://[fe80::1]:445/jeux"},
		{"[fe80::1]:1445", 0, "jeux", "smb://[fe80::1]:1445/jeux"},
		{"[fe80::1]", 0, "jeux", "smb://[fe80::1]:445/jeux"},
	}

	for _, tc := range cases {
		got, err := Address(tc.host, tc.port, tc.share)
		if err != nil {
			t.Errorf("Address(%q, %d, %q) : %v", tc.host, tc.port, tc.share, err)

			continue
		}
		if got != tc.want {
			t.Errorf("Address(%q, %d, %q) = %q, attendu %q", tc.host, tc.port, tc.share, got, tc.want)
		}
	}
}

func TestAddressRefusesWhatIsNotAShare(t *testing.T) {
	cases := []struct {
		host  string
		port  int
		share string
	}{
		{"", 445, "jeux"},
		{"nas", 445, ""},
		{"nas", 445, "jeux/roms"},
		{"nas", 70000, "jeux"},
		{"nas:abc", 445, "jeux"},
		{"nas:", 445, "jeux"},
		// Deux partages à la fois : lequel croire ? On ne choisit pas.
		{`\\nas\jeux`, 445, "autre"},
	}

	for _, tc := range cases {
		if got, err := Address(tc.host, tc.port, tc.share); !errors.Is(err, ErrAddress) {
			t.Errorf("Address(%q, %d, %q) = %q, %v — attendu ErrAddress", tc.host, tc.port, tc.share, got, err)
		}
	}
}

func TestNewReadsTheAddressAndTheFolder(t *testing.T) {
	source, err := New(Config{Name: "nas", Address: "smb://nas:1445/jeux/roms", Folder: "/nes/"})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	if source.hostport != "nas:1445" || source.share != "jeux" || source.root != "roms/nes" {
		t.Fatalf("hôte %q, partage %q, racine %q", source.hostport, source.share, source.root)
	}

	spaced, err := New(Config{Name: "nas", Address: "smb://nas/Mes%20Jeux"})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	if spaced.hostport != "nas:445" || spaced.share != "Mes Jeux" || spaced.root != "" {
		t.Fatalf("hôte %q, partage %q, racine %q", spaced.hostport, spaced.share, spaced.root)
	}
}

func TestNewRefusesAnAddressThatIsNotAShare(t *testing.T) {
	for _, address := range []string{"http://nas/jeux", "smb://nas", "smb://nas/", "smb:///jeux", "nas/jeux", ""} {
		if _, err := New(Config{Name: "nas", Address: address}); !errors.Is(err, ErrAddress) {
			t.Errorf("New(%q) : %v — attendu ErrAddress", address, err)
		}
	}
}

func TestNewRefusesAFolderThatClimbsOutOfTheShare(t *testing.T) {
	if _, err := New(Config{Name: "nas", Address: "smb://nas/jeux", Folder: "roms/../../etc"}); !errors.Is(err, storage.ErrOutsideSource) {
		t.Fatalf("attendu ErrOutsideSource, obtenu %v", err)
	}
}

func TestResolveStaysInsideTheMount(t *testing.T) {
	source, err := New(Config{Name: "nas", Address: "smb://nas/jeux", Folder: "roms"})
	if err != nil {
		t.Fatalf("New : %v", err)
	}

	cases := map[string]string{
		"":                  `roms`,
		"nes/Zelda.nes":     `roms\nes\Zelda.nes`,
		`nes\Zelda.nes`:     `roms\nes\Zelda.nes`,
		"/nes/./a/../b.nes": `roms\nes\b.nes`,
	}
	for name, want := range cases {
		got, err := source.resolve(name)
		if err != nil || got != want {
			t.Errorf("resolve(%q) = %q, %v — attendu %q", name, got, err, want)
		}
	}

	if _, err := source.resolve("../secret"); !errors.Is(err, storage.ErrOutsideSource) {
		t.Fatalf("une remontée doit être refusée, obtenu %v", err)
	}

	atRoot, _ := New(Config{Name: "nas", Address: "smb://nas/jeux"})
	if got, _ := atRoot.resolve(""); got != "" {
		t.Fatalf("la racine du partage s'écrit vide, obtenu %q", got)
	}
}

func TestClassifyNamesWhatTheServerMeant(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"mot de passe", &smb2.ResponseError{Code: statusLogonFailure}, ErrUnauthorized},
		{"compte désactivé", &smb2.ResponseError{Code: statusAccountDisabled}, ErrUnauthorized},
		{"partage inconnu", &smb2.ResponseError{Code: statusBadNetworkName}, ErrShareNotFound},
		{"droits", &os.PathError{Op: "open", Path: `roms`, Err: os.ErrPermission}, ErrForbidden},
		{"invité et signature", &smb2.InvalidResponseError{Message: "guest account doesn't support signing"}, ErrGuestRefused},
	}
	for _, tc := range cases {
		if got := classify(tc.err); !errors.Is(got, tc.want) {
			t.Errorf("%s : %v — attendu %v", tc.name, got, tc.want)
		}
	}

	// Un dossier absent RESTE un fichier absent : le scanner et les kernels
	// le reconnaissent à fs.ErrNotExist, et lui inventer une autre erreur
	// ferait passer un dossier renommé pour une panne.
	missing := classify(&os.PathError{Op: "open", Path: `roms\absent`, Err: os.ErrNotExist})
	if !errors.Is(missing, fs.ErrNotExist) || errors.Is(missing, ErrForbidden) {
		t.Fatalf("absent : %v", missing)
	}

	other := errors.New("autre chose")
	if classify(other) != other {
		t.Fatal("une erreur inconnue doit passer telle quelle")
	}
}

func TestOnlyABrokenLinkIsWorthRetrying(t *testing.T) {
	retry := []error{
		&smb2.TransportError{Err: io.EOF},
		io.ErrUnexpectedEOF,
		&smb2.ResponseError{Code: statusUserSessionDeleted},
		&smb2.ResponseError{Code: statusNetworkNameDeleted},
	}
	for _, err := range retry {
		if !brokenConnection(err) {
			t.Errorf("%v : la connexion est tombée, il faut rouvrir", err)
		}
	}

	final := []error{
		&smb2.ResponseError{Code: statusLogonFailure},
		&os.PathError{Op: "open", Path: "x", Err: os.ErrNotExist},
		os.ErrPermission,
	}
	for _, err := range final {
		if brokenConnection(err) {
			t.Errorf("%v : réessayer ne changerait rien", err)
		}
	}
}

func TestSourceSaysItIsASMBShare(t *testing.T) {
	source, err := New(Config{Name: "nas", Address: "smb://nas/jeux"})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	if source.Name() != "nas" || storage.KindOf(source) != storage.KindSMB {
		t.Fatalf("nom %q, nature %q", source.Name(), storage.KindOf(source))
	}
}

func TestAClosedSourceRefusesWorkWithoutDialing(t *testing.T) {
	// 192.0.2.0/24 est réservé à la documentation : si la source essayait
	// quand même de se connecter, le test attendrait le délai de connexion.
	source, err := New(Config{Name: "nas", Address: "smb://192.0.2.1/jeux"})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	_ = source.Close()

	started := time.Now()
	if _, err := source.List(""); !errors.Is(err, errClosed) {
		t.Fatalf("attendu errClosed, obtenu %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("une source fermée ne doit rien tenter sur le réseau")
	}
}

func TestAnUnreachableServerFailsFast(t *testing.T) {
	previous := connectTimeout
	connectTimeout = 300 * time.Millisecond
	defer func() { connectTimeout = previous }()

	source, err := New(Config{Name: "nas", Address: "smb://192.0.2.1/jeux"})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	defer source.Close()

	started := time.Now()
	if _, err := source.List(""); err == nil {
		t.Fatal("un serveur injoignable ne peut rien lister")
	}
	// Un seul essai de connexion, pas deux : l'échec d'un dial n'est pas une
	// connexion tombée, et réessayer doublerait l'attente.
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("échec en %v : trop lent", elapsed)
	}
}
