package storage

import (
	"bytes"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"path/filepath"
	"testing"
)

// testHeaderKey — une clé FABRIQUÉE pour ces tests : jamais celle d'une console.
var testHeaderKey = bytes.Repeat([]byte{0x5A, 0xC3}, 16)

// useTestKeys — une racine des BIOS qui range `testHeaderKey` comme le fait un
// `prod.keys` (`bios/switch/prod.keys`), le temps du test.
func useTestKeys(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "switch", "prod.keys"),
		[]byte("header_kek_source = 00\nheader_key = "+hex.EncodeToString(testHeaderKey)+"\nkey_area_key_application_00 = 11\n"))
	UseBiosRoot(func() string { return root })
	t.Cleanup(func() { UseBiosRoot(nil) })

	return root
}

// encryptSector — l'inverse de decryptSector, pour fabriquer un NCA chiffré comme
// les vrais.
func (header *ncaHeaderCipher) encryptSector(sector []byte, number uint64) {
	var tweak [16]byte
	binary.BigEndian.PutUint64(tweak[8:], number)
	header.tweak.Encrypt(tweak[:], tweak[:])
	for start := 0; start+16 <= len(sector); start += 16 {
		block := sector[start : start+16]
		subtle.XORBytes(block, block, tweak[:])
		header.data.Encrypt(block, block)
		subtle.XORBytes(block, block, tweak[:])
		nextTweak(&tweak)
	}
}

// fakeNCA — l'en-tête d'un NCA (0xC00 octets) de cette sorte et de ce program id,
// chiffré par `key`, secteur par secteur.
func fakeNCA(t *testing.T, key []byte, programID uint64, kind byte) []byte {
	t.Helper()
	header := make([]byte, 0xC00)
	copy(header[0x200:], "NCA3")
	header[0x205] = kind
	binary.LittleEndian.PutUint64(header[0x210:], programID)
	cipher, err := newNCAHeaderCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	for sector := 0; sector < len(header)/ncaSector; sector++ {
		cipher.encryptSector(header[sector*ncaSector:(sector+1)*ncaSector], uint64(sector))
	}

	return header
}

type fakeFile struct {
	name    string
	content []byte
}

// fakePartition — une partition PFS0 (entrées de 0x18 octets) ou HFS0 (0x40).
func fakePartition(magic string, entrySize int, files []fakeFile) []byte {
	names := []byte{}
	entries := make([]byte, len(files)*entrySize)
	data := []byte{}
	for index, file := range files {
		entry := entries[index*entrySize:]
		binary.LittleEndian.PutUint64(entry[0:], uint64(len(data)))
		binary.LittleEndian.PutUint64(entry[8:], uint64(len(file.content)))
		binary.LittleEndian.PutUint32(entry[16:], uint32(len(names)))
		names = append(append(names, file.name...), 0)
		data = append(data, file.content...)
	}
	header := make([]byte, 0x10)
	copy(header, magic)
	binary.LittleEndian.PutUint32(header[4:], uint32(len(files)))
	binary.LittleEndian.PutUint32(header[8:], uint32(len(names)))

	return append(append(append(header, entries...), names...), data...)
}

func fakeNSP(files ...fakeFile) []byte {
	return fakePartition("PFS0", 0x18, files)
}

// fakeXCI — une cartouche : « HEAD » à 0x100, la partition racine à 0x200, et ses
// NCA dans `secure`, après une partition `update` qui en a d'autres (ceux du
// système, qui ne disent rien du jeu).
func fakeXCI(update []fakeFile, secure ...fakeFile) []byte {
	header := make([]byte, 0x200)
	copy(header[0x100:], "HEAD")
	binary.LittleEndian.PutUint64(header[0x130:], 0x200)
	root := fakePartition("HFS0", 0x40, []fakeFile{
		{name: "update", content: fakePartition("HFS0", 0x40, update)},
		{name: "secure", content: fakePartition("HFS0", 0x40, secure)},
	})

	return append(header, root...)
}

// LE XTS DE LA NORME, au secteur 0 — le seul où le tweak de Nintendo (grand-boutiste)
// et celui de la norme (petit-boutiste) s'écrivent pareil : le vecteur 1 de
// l'IEEE 1619-2007, deux blocs (le second passe par la multiplication du tweak).
func TestTheNCAHeaderCipherIsTheStandardXTS(t *testing.T) {
	cipher, err := newNCAHeaderCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sector, _ := hex.DecodeString("917cf69ebd68b2ec9b9fe9a3eadda692cd43d2f59598ed858c02c2652fbf922e")
	cipher.decryptSector(sector, 0)
	if !bytes.Equal(sector, make([]byte, 32)) {
		t.Fatalf("vecteur 1 : %x", sector)
	}
}

// « CRASH BANDICOOT.XCI » NE DIT PAS SON IDENTIFIANT DANS SON NOM : il se lit dans
// l'en-tête de son NCA de programme, avec les clés de la console. Le NCA de
// métadonnées (sorte 1) ne compte pas, ni ceux de la partition `update`.
func TestASwitchCartridgeIsKnownByItsProgramID(t *testing.T) {
	useTestKeys(t)
	path := filepath.Join(t.TempDir(), "Crash Bandicoot", "Crash Bandicoot.xci")
	writeFile(t, path, fakeXCI(
		[]fakeFile{{name: "0123456789abcdef0123456789abcdef.nca", content: fakeNCA(t, testHeaderKey, 0x0100000000000809, ncaProgram)}},
		fakeFile{name: "69b96c1a57889c3a4b8ad43a3d19e889.cnmt.nca", content: fakeNCA(t, testHeaderKey, 0x0100000000000001, 1)},
		fakeFile{name: "e3e8422be6f2ef0106851eaf7b9ff265.nca", content: fakeNCA(t, testHeaderKey, 0x0100D1B006744000, ncaProgram)},
	))

	if got := GameTitleID(path); got != "0100D1B006744000" {
		t.Fatalf("la cartouche : %q", got)
	}
	if got := GameTitleID(filepath.Dir(path)); got != "0100D1B006744000" {
		t.Fatalf("son dossier : %q", got)
	}
}

// UN PAQUET AVEC SON TICKET SE LIT SANS CLÉ : les 16 premiers chiffres du nom du
// ticket. Celui d'une mise à jour (+0x800) dit le jeu de base.
func TestASwitchPackageIsKnownByItsTicketWithoutKeys(t *testing.T) {
	UseBiosRoot(nil)
	root := t.TempDir()
	base := filepath.Join(root, "FINAL FANTASY VII.nsp")
	writeFile(t, base, fakeNSP(
		fakeFile{name: "0100a5b00bdc60000000000000000007.cert", content: make([]byte, 0x700)},
		fakeFile{name: "0100a5b00bdc60000000000000000007.tik", content: make([]byte, 0x2C0)},
		fakeFile{name: "c1fe902c75c23f5901a21511f1b0c5a7.nca", content: make([]byte, 0xC00)},
	))
	if got := GameTitleID(base); got != "0100A5B00BDC6000" {
		t.Fatalf("le ticket : %q", got)
	}

	update := filepath.Join(root, "Mise à jour.nsp")
	writeFile(t, update, fakeNSP(fakeFile{name: "0100A5B00BDC68000000000000000007.tik", content: make([]byte, 0x2C0)}))
	if got := GameTitleID(update); got != "0100A5B00BDC6000" {
		t.Fatalf("le ticket d'une mise à jour : %q", got)
	}
}

// SANS TICKET, L'EN-TÊTE D'UN NCA ; SANS CLÉS, RIEN — et rien de gardé : des clés
// déposées ensuite valent aussitôt. Le programme d'une compilation (+1) dit le jeu
// de base.
func TestASwitchPackageWithoutTicketNeedsTheKeys(t *testing.T) {
	UseBiosRoot(nil)
	path := filepath.Join(t.TempDir(), "Kingdom Hearts.nsp")
	writeFile(t, path, fakeNSP(
		fakeFile{name: "03c8bf208b017977f8d80e0b843dd466.nca", content: fakeNCA(t, testHeaderKey, 0x010090C022978005, ncaProgram)},
		fakeFile{name: "1e6dbefc062bdb90d04734df84cc8ca9.nca", content: fakeNCA(t, testHeaderKey, 0x010090C022978001, ncaControl)},
	))
	if got := GameTitleID(path); got != "" {
		t.Fatalf("sans clés : %q", got)
	}

	useTestKeys(t)
	if got := GameTitleID(path); got != "010090C022978000" {
		t.Fatalf("avec les clés : %q", got)
	}

	// D'autres clés (celles d'une autre console) ne déchiffrent rien : aucune
	// magie, aucun identifiant.
	other := filepath.Join(t.TempDir(), "Autre.nsp")
	writeFile(t, other, fakeNSP(fakeFile{name: "00a0c40dd076744e43e0f2ab1a9669ed.nca",
		content: fakeNCA(t, bytes.Repeat([]byte{0x11}, 32), 0x01006B7014156000, ncaProgram)}))
	if got := GameTitleID(other); got != "" {
		t.Fatalf("d'autres clés : %q", got)
	}
}

// UN DOSSIER QUI RANGE LE JEU ET SON CONTENU ADDITIONNEL dit le jeu de base, quel
// que soit l'ordre des noms ; un dossier qui n'a que le contenu additionnel le dit
// aussi (+0x1000 et son numéro, effacés).
func TestASwitchFolderIsKnownByItsBaseGame(t *testing.T) {
	UseBiosRoot(nil)
	folder := filepath.Join(t.TempDir(), "Mario Kart 8 Deluxe")
	writeFile(t, filepath.Join(folder, "Mario Kart 8 Deluxe [DLC].nsp"),
		fakeNSP(fakeFile{name: "01001520000230010000000000000000.tik", content: make([]byte, 0x2C0)}))
	if got := GameTitleID(folder); got != "0100152000022000" {
		t.Fatalf("le contenu additionnel seul : %q", got)
	}
	writeFile(t, filepath.Join(folder, "Mario Kart 8 Deluxe.nsp"),
		fakeNSP(fakeFile{name: "01001520000220000000000000000004.tik", content: make([]byte, 0x2C0)}))
	writeFile(t, filepath.Join(folder, "notes.txt"), []byte("rien"))
	if got := GameTitleID(folder); got != "0100152000022000" {
		t.Fatalf("le jeu et son contenu additionnel : %q", got)
	}
}

// CE QUI N'EST PAS UN PAQUET SWITCH NE DIT RIEN, et ne casse rien : un `.xci`
// tronqué, un `.nsp` qui n'en est pas un, un en-tête qui annonce des milliers de
// fichiers.
func TestWhatIsNotASwitchPackageSaysNothing(t *testing.T) {
	useTestKeys(t)
	root := t.TempDir()

	truncated := filepath.Join(root, "Tronqué.xci")
	writeFile(t, truncated, []byte("HEAD"))
	if got := GameTitleID(truncated); got != "" {
		t.Fatalf("tronqué : %q", got)
	}

	notPFS0 := filepath.Join(root, "Faux.nsp")
	writeFile(t, notPFS0, bytes.Repeat([]byte("PFS1"), 64))
	if got := GameTitleID(notPFS0); got != "" {
		t.Fatalf("pas un PFS0 : %q", got)
	}

	huge := make([]byte, 0x40)
	copy(huge, "PFS0")
	binary.LittleEndian.PutUint32(huge[4:], 0xFFFFFFFF)
	hugePath := filepath.Join(root, "Énorme.nsp")
	writeFile(t, hugePath, huge)
	if got := GameTitleID(hugePath); got != "" {
		t.Fatalf("des milliers de fichiers annoncés : %q", got)
	}
}
