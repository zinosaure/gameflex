package sources

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpServer — un vrai serveur SSH qui sert `root` en SFTP, sur un port local ; rend son adresse.
func sftpServer(t *testing.T, root string) string {
	t.Helper()
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() == "gino" && string(password) == "secret" {
			return nil, nil
		}

		return nil, io.EOF
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				_, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					channel, reqs, err := incoming.Accept()
					if err != nil {
						continue
					}
					go func() {
						for req := range reqs {
							ok := req.Type == "subsystem" && strings.HasSuffix(string(req.Payload), "sftp")
							req.Reply(ok, nil)
							if ok {
								server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(root))
								if err == nil {
									_ = server.Serve()
								}
								channel.Close()
							}
						}
					}()
				}
			}()
		}
	}()

	return "sftp://" + listener.Addr().String()
}

func TestSFTPListsReadsAndPinsTheHostKey(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "jeux/roms/nes"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "jeux/roms/nes/Zelda.nes"), []byte("ZELDA"), 0o644)
	address := sftpServer(t, root)

	remembered := ""
	source, err := NewSFTP(SFTPConfig{Name: "nas", Address: address, Folder: "jeux", Username: "gino",
		Password: "secret", Remember: func(key string) { remembered = key }})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	entries, err := source.List("roms/nes")
	if err != nil || len(entries) != 1 || entries[0].Name != "Zelda.nes" || entries[0].Size != 5 {
		t.Fatalf("liste : %v %+v", err, entries)
	}
	file, err := source.Open("roms/nes/Zelda.nes")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(file)
	file.Close()
	if string(content) != "ZELDA" {
		t.Fatalf("lecture : %q", content)
	}
	if !strings.HasPrefix(remembered, "ssh-ed25519 ") {
		t.Fatalf("la clé du serveur doit être retenue à la première connexion : %q", remembered)
	}

	// Un mauvais mot de passe : refusé.
	wrong, _ := NewSFTP(SFTPConfig{Name: "nas", Address: address, Username: "gino", Password: "faux"})
	if _, err := wrong.List(""); err == nil {
		t.Fatal("un mauvais mot de passe doit être refusé")
	}

	// Une clé retenue qui ne correspond plus : la connexion est refusée.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	otherSigner, _ := ssh.NewSignerFromKey(other)
	impostor := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(otherSigner.PublicKey())))
	pinned, _ := NewSFTP(SFTPConfig{Name: "nas", Address: address, Username: "gino", Password: "secret", HostKey: impostor})
	if _, err := pinned.List(""); err == nil || !strings.Contains(err.Error(), "a changé") {
		t.Fatalf("une clé changée doit être refusée : %v", err)
	}
}

func TestAddressesNeedTheirScheme(t *testing.T) {
	if _, err := NewFTP(FTPConfig{Address: "sftp://nas"}); err == nil {
		t.Fatal("une adresse sftp:// n'est pas du FTP")
	}
	if host, err := hostPort("nas.local", "ftp", "21"); err != nil || host != "nas.local:21" {
		t.Fatalf("adresse nue : %q %v", host, err)
	}
	if host, err := hostPort("sftp://nas:2222", "sftp", "22"); err != nil || host != "nas:2222" {
		t.Fatalf("port : %q %v", host, err)
	}
}
