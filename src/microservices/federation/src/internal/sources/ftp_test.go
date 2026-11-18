package sources

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ftpServer — un serveur FTP minimal sur `root` : USER/PASS, FEAT, TYPE, EPSV, LIST (format Unix), RETR, QUIT. Assez
// pour le client du service, rien de plus.
func ftpServer(t *testing.T, root string) string {
	t.Helper()
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
			go serveFTP(conn, root)
		}
	}()

	return "ftp://" + listener.Addr().String()
}

func serveFTP(conn net.Conn, root string) {
	defer conn.Close()
	reply := func(format string, args ...any) { fmt.Fprintf(conn, format+"\r\n", args...) }
	reply("220 prêt")
	var data net.Listener
	user := ""
	lines := bufio.NewScanner(conn)
	for lines.Scan() {
		command, argument, _ := strings.Cut(lines.Text(), " ")
		switch strings.ToUpper(command) {
		case "USER":
			user = argument
			reply("331 mot de passe")
		case "PASS":
			if user == "gino" && argument == "secret" {
				reply("230 connecté")
			} else {
				reply("530 refusé")
			}
		case "FEAT":
			reply("211 rien")
		case "TYPE":
			reply("200 ok")
		case "EPSV":
			data, _ = net.Listen("tcp", "127.0.0.1:0")
			reply("229 passif (|||%d|)", data.Addr().(*net.TCPAddr).Port)
		case "LIST", "RETR":
			target := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(argument, "-a "), "/")))
			if data == nil {
				reply("425 pas de connexion de données")

				continue
			}
			reply("150 voilà")
			channel, err := data.Accept()
			data.Close()
			data = nil
			if err != nil {
				continue
			}
			if strings.ToUpper(command) == "LIST" {
				entries, _ := os.ReadDir(target)
				for _, e := range entries {
					info, _ := e.Info()
					mode, size := "-rw-r--r--", info.Size()
					if e.IsDir() {
						mode, size = "drwxr-xr-x", 0
					}
					fmt.Fprintf(channel, "%s 1 owner group %d Jan  1 00:00 %s\r\n", mode, size, e.Name())
				}
			} else if file, err := os.Open(target); err == nil {
				_, _ = io.Copy(channel, file)
				file.Close()
			}
			channel.Close()
			reply("226 fini")
		case "QUIT":
			reply("221 au revoir")

			return
		default:
			reply("502 inconnu")
		}
	}
}

func TestFTPListsAndReads(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "jeux/roms/nes/Zelda Pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "jeux/roms/nes/Metroid.nes"), []byte("METROID"), 0o644)
	address := ftpServer(t, root)

	source, err := NewFTP(FTPConfig{Name: "nas", Address: address, Folder: "jeux", Username: "gino", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	entries, err := source.List("roms/nes")
	if err != nil || len(entries) != 2 {
		t.Fatalf("liste : %v %+v", err, entries)
	}
	byName := map[string]bool{}
	for _, e := range entries {
		byName[e.Name] = e.Dir
	}
	if dir, ok := byName["Zelda Pack"]; !ok || !dir {
		t.Fatalf("un dossier au nom espacé : %+v", entries)
	}
	file, err := source.Open("roms/nes/Metroid.nes")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(file)
	file.Close()
	if string(content) != "METROID" {
		t.Fatalf("lecture : %q", content)
	}

	wrong, _ := NewFTP(FTPConfig{Name: "nas", Address: address, Username: "gino", Password: "faux"})
	if _, err := wrong.List(""); err == nil {
		t.Fatal("un mauvais mot de passe doit être refusé")
	}
}
