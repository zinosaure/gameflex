package saves

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path, content string, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s : %v", path, err)
	}

	return string(raw)
}

func missing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s ne devrait pas exister (%v)", path, err)
	}
}

// sfo — un PARAM.SFO réduit à son TITLE.
func sfo(title string) string {
	key := "TITLE\x00"
	value := title + "\x00"
	var out bytes.Buffer
	out.WriteString("\x00PSF")
	binary.Write(&out, binary.LittleEndian, uint32(0x101))
	binary.Write(&out, binary.LittleEndian, uint32(20+16))
	binary.Write(&out, binary.LittleEndian, uint32(20+16+len(key)))
	binary.Write(&out, binary.LittleEndian, uint32(1))
	binary.Write(&out, binary.LittleEndian, uint16(0))
	binary.Write(&out, binary.LittleEndian, uint16(0x0204))
	binary.Write(&out, binary.LittleEndian, uint32(len(value)))
	binary.Write(&out, binary.LittleEndian, uint32(len(value)))
	binary.Write(&out, binary.LittleEndian, uint32(0))
	out.WriteString(key)
	out.WriteString(value)

	return out.String()
}

// profiles — un profiles.dat dont le premier profil a cet UUID.
func profiles(low, high uint64) string {
	raw := make([]byte, 0x10+8*0xC8)
	binary.LittleEndian.PutUint64(raw[0x10:], low)
	binary.LittleEndian.PutUint64(raw[0x18:], high)

	return string(raw)
}

var (
	monday  = time.Date(2026, 10, 26, 20, 0, 0, 0, time.UTC)
	tuesday = time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)
)

// UNE COPIE NE SORT PAS DU DOSSIER DU COMPTE, et son nom vient de la
// fédération : `local` n'est jamais celui d'un serveur.
func TestTreesAndSourceNames(t *testing.T) {
	root := t.TempDir()
	if tree, err := Tree(root, "gino", ""); err != nil || tree != filepath.Join(root, "gino") {
		t.Fatalf("principal : %q %v", tree, err)
	}
	if tree, err := Tree(root, "gino", "192.0.2.11"); err != nil || tree != filepath.Join(root, "gino", "sources", "192.0.2.11") {
		t.Fatalf("copie : %q %v", tree, err)
	}
	for _, bad := range []string{"..", "../bob", ".cache", "a/b", " x"} {
		if _, err := Tree(root, "gino", bad); err == nil {
			t.Fatalf("%q accepté", bad)
		}
	}
	if _, err := Tree(root, "../bob", ""); err == nil {
		t.Fatal("compte « ../bob » accepté")
	}

	for name, want := range map[string]string{
		"S25 de Gino":  "S25 de Gino",
		"local":        "local_",
		"Local":        "Local_",
		"..nas":        "nas",
		"nas/salon":    "nas_salon",
		"  prod  ":     "prod",
		"a:b*c":        "a_b_c",
		"":             "",
		"192.0.2.11": "192.0.2.11",
	} {
		if got := SourceName(name); got != want {
			t.Errorf("SourceName(%q) = %q, attendu %q", name, got, want)
		}
	}

	for _, name := range []string{"prod", "local", "Alpha"} {
		os.MkdirAll(filepath.Join(root, "gino", "sources", name), 0o755)
	}
	os.MkdirAll(filepath.Join(root, "gino", "sources", ".incoming-1"), 0o755)
	names, err := Sources(root, "gino")
	if err != nil || len(names) != 3 || names[0] != "local" || names[1] != "Alpha" || names[2] != "prod" {
		t.Fatalf("sources : %v %v", names, err)
	}
	if names, err := Sources(root, "marc"); err != nil || len(names) != 0 {
		t.Fatalf("sans copie : %v %v", names, err)
	}
}

// fakeConsoles — un principal comme celui de la dev : des sauvegardes, et
// autour la configuration de chaque émulateur, qui ne se copie jamais.
func fakeConsoles(t *testing.T, tree string) {
	streamex := filepath.Join(tree, "streamex")
	ps3 := filepath.Join(streamex, "ps3", "config", "dev_hdd0", "home", "00000001", "savedata")
	write(t, filepath.Join(ps3, "MRTC00003-00", "PARAM.SFO"), sfo("Tekken 6"), monday)
	write(t, filepath.Join(ps3, "MRTC00003-01", "DATA.BIN"), "slot 1", tuesday)
	write(t, filepath.Join(ps3, "BLES00635", "DATA.BIN"), "god of war", monday)
	write(t, filepath.Join(streamex, "ps3", "config", "config.yml"), "renderer", monday)

	ps4 := filepath.Join(streamex, "ps4", "data", "home", "1000", "savedata")
	write(t, filepath.Join(ps4, "CUSA07010", "SAVEDATA00", "sce_sys", "param.sfo"), sfo("Kingdom Hearts III"), monday)
	write(t, filepath.Join(ps4, "CUSA07010", "SAVEDATA00", "data.bin"), "kh3", tuesday)

	write(t, filepath.Join(streamex, "ps5", "savedata", "PPSA01911", "ff7remake", "save.dat"), "ff7", monday)

	wiiu := filepath.Join(streamex, "wiiu", "mlc01", "usr", "save", "00050000", "10101e00")
	write(t, filepath.Join(wiiu, "meta", "meta.xml"), `<menu><shortname_en type="string" length="512">Mario Kart 8</shortname_en></menu>`, monday)
	write(t, filepath.Join(wiiu, "user", "80000001", "rp.dat"), "mk8", monday)

	write(t, filepath.Join(streamex, "xbox", "xemu", "xbox_hdd.qcow2"), "disk", monday)
	write(t, filepath.Join(streamex, "xbox", "xemu", "eeprom.bin"), "eeprom", monday)
	write(t, filepath.Join(streamex, "xbox", "xemu", "xemu.toml"), "config", monday)
}

func find(games []Game, console, id string) *Game {
	for index := range games {
		if games[index].Console == console && games[index].ID == id {
			return &games[index]
		}
	}

	return nil
}

// LES JEUX, PAS LES DOSSIERS : deux sauvegardes PS3 du même produit font un
// jeu, la Xbox un disque, et la configuration n'en est pas un.
func TestGamesListsEachGameOnce(t *testing.T) {
	tree := t.TempDir()
	fakeConsoles(t, tree)

	games := Games(tree)
	if len(games) != 6 {
		t.Fatalf("%d jeux : %+v", len(games), games)
	}
	tekken := find(games, "ps3", "MRTC00003")
	if tekken == nil || tekken.Title != "Tekken 6" || !tekken.UpdatedAt.Equal(tuesday) {
		t.Fatalf("Tekken : %+v", tekken)
	}
	if kh3 := find(games, "ps4", "CUSA07010"); kh3 == nil || kh3.Title != "Kingdom Hearts III" || kh3.SizeBytes == 0 {
		t.Fatalf("KH3 : %+v", kh3)
	}
	if mk8 := find(games, "wiiu", "10101e00"); mk8 == nil || mk8.Title != "Mario Kart 8" {
		t.Fatalf("MK8 : %+v", mk8)
	}
	if disk := find(games, "xbox", XboxDisk); disk == nil || disk.SizeBytes != int64(len("disk")+len("eeprom")) {
		t.Fatalf("Xbox : %+v", disk)
	}
	if find(games, "ps5", "PPSA01911") == nil || find(games, "ps3", "BLES00635") == nil {
		t.Fatalf("PS5 ou God of War absent : %+v", games)
	}
	if len(Games(t.TempDir())) != 0 {
		t.Fatal("un rangement vide a des jeux")
	}
}

// L'ARCHIVE D'UN JEU SE DÉPOSE DANS UNE COPIE, avec ses dates, à la place de
// la version d'avant — et rien que ce jeu.
func TestArchiveTravelsIntoACopy(t *testing.T) {
	principal := t.TempDir()
	fakeConsoles(t, principal)
	copy := filepath.Join(t.TempDir(), "sources", "prod")
	write(t, filepath.Join(copy, "streamex", "ps3", "config", "dev_hdd0", "home", "00000001", "savedata", "MRTC00003-02", "OLD.BIN"), "stale", monday)
	write(t, filepath.Join(copy, "streamex", "ps3", "config", "dev_hdd0", "home", "00000001", "savedata", "BLES01269-00", "DATA.BIN"), "other game", monday)

	var archive bytes.Buffer
	if err := WriteArchive(&archive, principal, "ps3", "MRTC00003"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tekken.zip")
	os.WriteFile(path, archive.Bytes(), 0o644)
	var announced uint64
	if err := Receive(path, copy, "ps3", "MRTC00003", func(size uint64) error { announced = size; return nil }); err != nil {
		t.Fatal(err)
	}

	savedata := filepath.Join(copy, "streamex", "ps3", "config", "dev_hdd0", "home", "00000001", "savedata")
	if read(t, filepath.Join(savedata, "MRTC00003-01", "DATA.BIN")) != "slot 1" || announced == 0 {
		t.Fatalf("Tekken non reçu (%d octets annoncés)", announced)
	}
	missing(t, filepath.Join(savedata, "MRTC00003-02"))
	if read(t, filepath.Join(savedata, "BLES01269-00", "DATA.BIN")) != "other game" {
		t.Fatal("un autre jeu de la copie a bougé")
	}
	missing(t, filepath.Join(copy, "streamex", "ps3", "config", "config.yml"))
	if game := find(Games(copy), "ps3", "MRTC00003"); game == nil || !game.UpdatedAt.Equal(tuesday) {
		t.Fatalf("la date de la partie s'est perdue : %+v", game)
	}

	if err := WriteArchive(&archive, principal, "ps3", "NONE00000"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("jeu absent : %v", err)
	}
	if err := Receive(path, copy, "ps3", "MRTC00003", func(uint64) error { return errors.New("disque plein") }); err == nil {
		t.Fatal("la place du disque n'a pas été jugée")
	}
}

// UNE ARCHIVE NE DÉPOSE QUE LA SAUVEGARDE DE SON JEU : ni la configuration de
// l'émulateur, ni un autre jeu, ni rien hors de la console.
func TestReceiveRefusesForeignEntries(t *testing.T) {
	for _, name := range []string{
		"config/config.yml",
		"config/dev_hdd0/home/00000001/savedata/BLES00635/DATA.BIN",
		"../../../etc/passwd",
		"config/dev_hdd0/home/00000001/savedata/MRTC00003-00/../../../../../config.yml",
		"config/dev_hdd0/home/.hidden/savedata/MRTC00003-00/x",
		"/config/dev_hdd0/home/00000001/savedata/MRTC00003-00/x",
	} {
		var buffer bytes.Buffer
		archive := zip.NewWriter(&buffer)
		writer, _ := archive.Create(name)
		writer.Write([]byte("x"))
		archive.Close()
		path := filepath.Join(t.TempDir(), "a.zip")
		os.WriteFile(path, buffer.Bytes(), 0o644)

		tree := t.TempDir()
		if err := Receive(path, tree, "ps3", "MRTC00003", nil); !errors.Is(err, ErrForeignEntry) {
			t.Errorf("%q : %v", name, err)
		}
		if games := Games(tree); len(games) != 0 {
			t.Errorf("%q a laissé %+v", name, games)
		}
	}
	if err := Receive("", t.TempDir(), "xbox360", "x", nil); err == nil {
		t.Fatal("la Xbox 360 n'est pas encore prise en charge")
	}
}

// RESTAURER REMPLACE LE JEU, ET LUI SEUL ; ce que le principal avait part dans
// `local`. Depuis `local` même, les deux versions s'échangent.
func TestRestoreReplacesOneGameAndKeepsABackup(t *testing.T) {
	root := t.TempDir()
	principal := filepath.Join(root, "gino")
	fakeConsoles(t, principal)
	prod := filepath.Join(principal, "sources", "prod")
	write(t, filepath.Join(prod, "streamex", "ps4", "data", "home", "1000", "savedata", "CUSA07010", "SAVEDATA00", "data.bin"), "kh3 de la prod", tuesday)

	if err := Restore(root, "gino", "prod", "ps4", "CUSA07010"); err != nil {
		t.Fatal(err)
	}
	kh3 := filepath.Join("streamex", "ps4", "data", "home", "1000", "savedata", "CUSA07010", "SAVEDATA00")
	if read(t, filepath.Join(principal, kh3, "data.bin")) != "kh3 de la prod" {
		t.Fatal("non restauré")
	}
	missing(t, filepath.Join(principal, kh3, "sce_sys"))
	local := filepath.Join(principal, "sources", "local")
	if read(t, filepath.Join(local, kh3, "data.bin")) != "kh3" {
		t.Fatal("l'ancienne version n'est pas dans local")
	}
	if read(t, filepath.Join(principal, "streamex", "ps5", "savedata", "PPSA01911", "ff7remake", "save.dat")) != "ff7" ||
		read(t, filepath.Join(principal, "streamex", "ps3", "config", "config.yml")) != "renderer" {
		t.Fatal("un autre jeu, ou la configuration, a bougé")
	}

	if err := Restore(root, "gino", Local, "ps4", "CUSA07010"); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(principal, kh3, "data.bin")) != "kh3" || read(t, filepath.Join(local, kh3, "data.bin")) != "kh3 de la prod" {
		t.Fatal("restaurer depuis local n'échange pas les deux versions")
	}

	if err := Restore(root, "gino", "prod", "ps4", "CUSA00001"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("jeu absent de la copie : %v", err)
	}
	if err := Restore(root, "gino", "", "ps4", "CUSA07010"); err == nil {
		t.Fatal("restaurer depuis le principal")
	}

	if err := Backup(root, "gino", "xbox", XboxDisk); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(local, "streamex", "xbox", "xemu", "xbox_hdd.qcow2")) != "disk" {
		t.Fatal("le disque de la Xbox n'est pas dans local")
	}
	missing(t, filepath.Join(local, "streamex", "xbox", "xemu", "xemu.toml"))
}

// LA SWITCH RANGE SOUS LE PROFIL D'ICI, quel que soit celui d'où vient la
// sauvegarde ; d'un nom tronqué par Citron et du vrai profil, le plus récent
// l'emporte.
func TestRestoreFilesSwitchSavesUnderThisProfile(t *testing.T) {
	root := t.TempDir()
	principal := filepath.Join(root, "gino")
	switchDir := filepath.Join(principal, "streamex", "switch")
	write(t, filepath.Join(switchDir, "nand", "system", "save", "8000000000000010", "su", "avators", "profiles.dat"),
		profiles(0x6FD0DCCA511C514B, 0xCB4433B032A5AC30), monday)
	if profile := SwitchProfile(principal); profile != "CB4433B032A5AC306FD0DCCA511C514B" {
		t.Fatalf("profil : %q", profile)
	}
	saves := filepath.Join("nand", "user", "save", "0000000000000000")
	write(t, filepath.Join(switchDir, saves, "CB4433B032A5AC306FD0DCCA511C514B", "0100564024AEA000", "save.bin"), "ici", monday)

	prod := filepath.Join(principal, "sources", "prod", "streamex", "switch")
	write(t, filepath.Join(prod, saves, "AAAAAAAAAAAAAAAABBBBBBBBBBBBBBBB", "0100564024AEA000", "save.bin"), "prod récente", tuesday)
	write(t, filepath.Join(prod, saves, "306FD0DCCA511C514B00000000000000", "0100564024AEA000", "save.bin"), "prod ancienne", monday)

	if err := Restore(root, "gino", "prod", "switch", "0100564024AEA000"); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(switchDir, saves, "CB4433B032A5AC306FD0DCCA511C514B", "0100564024AEA000", "save.bin")) != "prod récente" {
		t.Fatal("pas sous le profil d'ici, ou pas la plus récente")
	}
	missing(t, filepath.Join(switchDir, saves, "AAAAAAAAAAAAAAAABBBBBBBBBBBBBBBB"))
	if read(t, filepath.Join(principal, "sources", "local", "streamex", "switch", saves, "CB4433B032A5AC306FD0DCCA511C514B", "0100564024AEA000", "save.bin")) != "ici" {
		t.Fatal("l'ancienne sauvegarde n'est pas dans local")
	}
	if SwitchProfile(t.TempDir()) != "" {
		t.Fatal("un profil sans profiles.dat")
	}
}

// LE RÉTRO, EMPLACEMENT PAR EMPLACEMENT, sous la référence d'ici.
func TestRestoreSlotKeepsABackup(t *testing.T) {
	root := t.TempDir()
	principal := filepath.Join(root, "gino")
	states := filepath.Join(principal, "states", "nes")
	write(t, filepath.Join(states, "ici_slot1.state"), "ici", monday)
	write(t, filepath.Join(states, "ici_slot1.png"), "vignette d'ici", monday)
	prod := filepath.Join(principal, "sources", "prod", "states", "nes")
	write(t, filepath.Join(prod, "la-bas_slot1.state"), "prod", tuesday)

	if err := RestoreSlot(root, "gino", "prod", "nes", "la-bas", "ici", 1); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(states, "ici_slot1.state")) != "prod" {
		t.Fatal("non restauré")
	}
	missing(t, filepath.Join(states, "ici_slot1.png"))
	local := filepath.Join(principal, "sources", "local", "states", "nes")
	if read(t, filepath.Join(local, "ici_slot1.state")) != "ici" || read(t, filepath.Join(local, "ici_slot1.png")) != "vignette d'ici" {
		t.Fatal("l'ancien emplacement n'est pas dans local")
	}
	if info, _ := os.Stat(filepath.Join(states, "ici_slot1.state")); !info.ModTime().Equal(tuesday) {
		t.Fatalf("date : %v", info.ModTime())
	}

	if err := RestoreSlot(root, "gino", Local, "nes", "ici", "ici", 1); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(states, "ici_slot1.state")) != "ici" || read(t, filepath.Join(local, "ici_slot1.state")) != "prod" {
		t.Fatal("restaurer depuis local n'échange pas")
	}
	if err := RestoreSlot(root, "gino", "prod", "nes", "la-bas", "ici", 2); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("emplacement vide : %v", err)
	}

	if err := RememberTitle(filepath.Join(principal, "sources", "prod"), "nes", "la-bas", "Metroid"); err != nil {
		t.Fatal(err)
	}
	if Title(filepath.Join(principal, "sources", "prod"), "nes", "la-bas") != "Metroid" {
		t.Fatal("titre perdu")
	}
}
