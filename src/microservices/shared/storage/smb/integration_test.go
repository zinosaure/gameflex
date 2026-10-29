package smb

// Contre un VRAI serveur Samba — l'image de tools/bench/samba, avec son
// arbre de jeux intégré. Sans elle, ces tests s'effacent : un faux serveur
// SMB demanderait de réécrire la négociation, NTLM et la signature, c'est-à-
// dire exactement ce qu'on veut vérifier.
//
//	docker network create gf-smbnet
//	docker run -d --name gf-smbtest --network gf-smbnet gameflex-bench-samba
//	docker run -d --name gf-smbsigned --network gf-smbnet \
//	    -e SMB_SIGNING=mandatory gameflex-bench-samba
//	docker run --rm --network gf-smbnet \
//	    -e GF_SMB_TEST_HOST=gf-smbtest -e GF_SMB_TEST_SIGNED_HOST=gf-smbsigned \
//	    -v <dépôt>/src/microservices/shared:/shared:ro -w /shared/storage \
//	    gameflex-gameflex-kernel:latest go test ./smb/

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"

	"gameflex/storage"
)

// L'arbre que l'image du banc embarque (tools/bench/samba/Dockerfile).
const (
	benchGame      = "Bench (World).nes"
	benchGameBytes = "NES\x1abench"
	benchAccount   = "bench"
	benchPassword  = "bench-smb"
)

func benchSource(t *testing.T, env, share string, config Config) *Source {
	t.Helper()

	host := os.Getenv(env)
	if host == "" {
		t.Skipf("%s absent : pas de serveur Samba de banc", env)
	}
	address, err := Address(host, 0, share)
	if err != nil {
		t.Fatalf("adresse : %v", err)
	}
	config.Address = address
	if config.Name == "" {
		config.Name = "nas"
	}

	source, err := New(config)
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })

	return source
}

func names(entries []storage.Entry) map[string]storage.Entry {
	byName := map[string]storage.Entry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}

	return byName
}

func TestGuestReadsTheOpenShare(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "roms", Config{})

	root, err := source.List("")
	if err != nil {
		t.Fatalf("racine : %v", err)
	}
	if entry, ok := names(root)["roms"]; !ok || !entry.Dir {
		t.Fatalf("racine = %+v, attendu le dossier roms", root)
	}

	nes, err := source.List("roms/nes")
	if err != nil {
		t.Fatalf("roms/nes : %v", err)
	}
	game, ok := names(nes)[benchGame]
	if !ok || game.Dir || game.Size != int64(len(benchGameBytes)) {
		t.Fatalf("roms/nes = %+v, attendu %q de %d octets", nes, benchGame, len(benchGameBytes))
	}
}

func TestTheFolderBecomesTheRoot(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "roms", Config{Folder: "roms"})

	root, err := source.List("")
	if err != nil {
		t.Fatalf("racine : %v", err)
	}
	if _, ok := names(root)["nes"]; !ok {
		t.Fatalf("avec le dossier roms, la racine doit montrer nes : %+v", root)
	}
}

func TestOpenReadsTheBytes(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "roms", Config{})

	file, err := source.Open("roms/nes/" + benchGame)
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil || string(content) != benchGameBytes {
		t.Fatalf("contenu %q, %v", content, err)
	}
}

func TestTheScannerFindsTheGame(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "roms", Config{})

	games, err := storage.ScanConvention(source, "")
	if err != nil {
		t.Fatalf("scan : %v", err)
	}
	for _, game := range games {
		if game.Console == "nes" && game.Title == "Bench (World)" {
			return
		}
	}
	t.Fatalf("jeu absent du scan : %+v", games)
}

func TestAnAccountOpensThePrivateShare(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "prive", Config{Username: benchAccount, Password: benchPassword})

	if _, err := source.List("roms/nes"); err != nil {
		t.Fatalf("avec le bon compte : %v", err)
	}
}

func TestAWrongPasswordIsNamed(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "prive", Config{Username: benchAccount, Password: "pas-le-bon"})

	if _, err := source.List(""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("attendu ErrUnauthorized, obtenu %v", err)
	}
}

func TestAGuestIsTurnedAwayFromThePrivateShare(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "prive", Config{})

	if _, err := source.List(""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("attendu ErrForbidden, obtenu %v", err)
	}
}

func TestAnUnknownShareIsNamed(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "absent", Config{})

	if _, err := source.List(""); !errors.Is(err, ErrShareNotFound) {
		t.Fatalf("attendu ErrShareNotFound, obtenu %v", err)
	}
}

func TestAMissingFolderIsNotAnError(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "roms", Config{})

	if _, err := source.List("roms/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("attendu fs.ErrNotExist, obtenu %v", err)
	}
}

func TestADroppedConnectionIsReopened(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "roms", Config{})

	if _, err := source.List(""); err != nil {
		t.Fatalf("premier listing : %v", err)
	}

	// Ce que fait un NAS qui s'endort, vu d'ici : la connexion meurt.
	source.mu.Lock()
	_ = source.conn.Close()
	source.mu.Unlock()

	if _, err := source.List("roms/nes"); err != nil {
		t.Fatalf("après la coupure : %v", err)
	}
}

func TestClosingWaitsForAnOpenFile(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_HOST", "roms", Config{})

	file, err := source.Open("roms/nes/" + benchGame)
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}

	// Un « Relire » pendant un téléchargement.
	_ = source.Close()

	content, err := io.ReadAll(file)
	if err != nil || string(content) != benchGameBytes {
		t.Fatalf("le téléchargement a été coupé : %q, %v", content, err)
	}
	_ = file.Close()

	source.mu.Lock()
	defer source.mu.Unlock()
	if source.conn != nil {
		t.Fatal("le dernier usage rendu, la session devait se fermer")
	}
}

func TestProbeSeesTheFolders(t *testing.T) {
	host := os.Getenv("GF_SMB_TEST_HOST")
	if host == "" {
		t.Skip("GF_SMB_TEST_HOST absent : pas de serveur Samba de banc")
	}
	address, _ := Address(host, 0, "roms")

	seen, err := Probe(Config{Name: "probe", Address: address, Folder: "roms"})
	if err != nil {
		t.Fatalf("sonde : %v", err)
	}
	if len(seen.Folders) != 1 || seen.Folders[0] != "nes" || seen.Files != 0 {
		t.Fatalf("sonde = %+v", seen)
	}
}

// UN SAMBA QUI EXIGE LA SIGNATURE SERT QUAND MÊME UN INVITÉ.
//
// La supposition de départ était inverse — « une session invitée n'a pas de
// clé pour signer » — et ce test l'affirmait. Mesuré le 2026-09-15 contre
// Samba 4.17 réglé `server signing = mandatory` : la session invitée est
// acceptée ET signée, le listing aboutit. ErrGuestRefused reste pour les
// serveurs qui marquent la session comme invitée tout en exigeant la
// signature — la bibliothèque refuse alors, et la sonde sait le nommer. Ce
// cas-là n'est vérifié que par TestClassifyNamesWhatTheServerMeant : aucun
// serveur qui s'y comporte ainsi n'est à disposition ici.
func TestASigningSambaStillServesAGuest(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_SIGNED_HOST", "roms", Config{})

	if _, err := source.List("roms/nes"); err != nil {
		t.Fatalf("Samba signe la session invitée : %v", err)
	}
}

func TestASigningServerAcceptsAnAccount(t *testing.T) {
	source := benchSource(t, "GF_SMB_TEST_SIGNED_HOST", "prive", Config{Username: benchAccount, Password: benchPassword})

	if _, err := source.List("roms/nes"); err != nil {
		t.Fatalf("un compte signe ses messages : %v", err)
	}
}
