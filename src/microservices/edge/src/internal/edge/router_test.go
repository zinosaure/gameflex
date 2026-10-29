package edge

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gameflex/edge/internal/domains"
)

// backend répond « <nom> <chemin> xff=<X-Forwarded-For> host=<Host> ».
func backend(t *testing.T, name string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s xff=%s host=%s", name, r.URL.RequestURI(), r.Header.Get("X-Forwarded-For"), r.Host)
	}))
	t.Cleanup(server.Close)

	return server
}

var page = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "page")
})

func TestNewNamesWhatIsMissing(t *testing.T) {
	services := []domains.Service{{Service: "kernel", Target: "KERNEL_TARGET", Domains: []string{"health"}}}

	_, err := New(services, func(string) string { return "" }, page)
	if err == nil || !strings.Contains(err.Error(), "KERNEL_TARGET manque") {
		t.Errorf("variable absente : %v", err)
	}

	_, err = New(services, func(string) string { return "gameflex-kernel:8080" }, page)
	if err == nil || !strings.Contains(err.Error(), "n'est pas une adresse http(s)") {
		t.Errorf("adresse sans schéma : %v", err)
	}
}

func TestRouting(t *testing.T) {
	kernel := backend(t, "kernel")
	credential := backend(t, "credential")
	env := map[string]string{"KERNEL": kernel.URL, "CREDENTIAL": credential.URL}
	handler, err := New([]domains.Service{
		{Service: "kernel", Target: "KERNEL", Domains: []string{"health", "sessions"}},
		{Service: "credential", Target: "CREDENTIAL", ForwardClientAddress: true, Domains: []string{"users"}},
	}, func(name string) string { return env[name] }, page)
	if err != nil {
		t.Fatal(err)
	}
	edge := httptest.NewServer(handler)
	defer edge.Close()

	cases := []struct {
		path   string
		status int
		want   []string
		reject []string
	}{
		{path: "/api/v1/health", status: 200, want: []string{"kernel /api/v1/health", "xff= ", "host=" + strings.TrimPrefix(kernel.URL, "http://")}},
		{path: "/api/v1/sessions/abc/child/v4/ws?token=x", status: 200, want: []string{"kernel /api/v1/sessions/abc/child/v4/ws?token=x"}},
		{path: "/api/v1/users/12", status: 200, want: []string{"credential /api/v1/users/12", "xff=127.0.0.1"}},
		{path: "/api/v1/healthz", status: 200, want: []string{"page"}},
		{path: "/api/v1/inconnu/x", status: 200, want: []string{"page"}},
		{path: "/retrogaming/nes", status: 200, want: []string{"page"}},
		{path: "/api/v1/sessions/", status: 200, want: []string{"kernel /api/v1/sessions/"}},
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, c := range cases {
		response, err := client.Get(edge.URL + c.path)
		if err != nil {
			t.Fatalf("%s : %v", c.path, err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()

		if response.StatusCode != c.status {
			t.Errorf("%s : statut %d, attendu %d", c.path, response.StatusCode, c.status)
		}
		for _, want := range c.want {
			if !strings.Contains(string(body), want) {
				t.Errorf("%s : %q ne contient pas %q", c.path, body, want)
			}
		}
	}

	// Un chemin qui remonte est renvoyé vers sa forme propre, jamais aiguillé tel quel.
	request, _ := http.NewRequest(http.MethodGet, edge.URL+"/api/v1/health/../users", nil)
	request.URL.Opaque = "/api/v1/health/../users"
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMovedPermanently || response.Header.Get("Location") != "/api/v1/users" {
		t.Errorf("remontée : %d vers %q, attendu 301 vers /api/v1/users", response.StatusCode, response.Header.Get("Location"))
	}
}

func TestUnreachableServiceAnswersInJSON(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	address := gone.URL
	gone.Close()

	handler, err := New([]domains.Service{{Service: "storage", Target: "STORAGE", Domains: []string{"storage"}}},
		func(string) string { return address }, page)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/storage/sections", nil))
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "storage is unreachable") {
		t.Errorf("service injoignable : %d %q", recorder.Code, recorder.Body.String())
	}
}

// echoWebSocket accepte la poignée de main puis renvoie ce qu'il reçoit — sans
// bibliothèque WebSocket : seul le passage de l'Upgrade par le frontal compte.
func echoWebSocket(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "upgrade attendu", http.StatusBadRequest)
			return
		}
		connection, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _ = buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = buffer.Flush()
		line, _ := buffer.ReadString('\n')
		_, _ = buffer.WriteString("echo " + line)
		_ = buffer.Flush()
	}))
	t.Cleanup(server.Close)

	return server
}

func dialUpgrade(t *testing.T, edgeURL, path string) (string, *bufio.Reader, net.Conn) {
	t.Helper()
	connection, err := net.Dial("tcp", strings.TrimPrefix(edgeURL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(connection, "GET %s HTTP/1.1\r\nHost: edge\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n", path)
	reader := bufio.NewReader(connection)
	status, _ := reader.ReadString('\n')

	return status, reader, connection
}

func TestWebSocketsPassOnlyWhereDeclared(t *testing.T) {
	socket := echoWebSocket(t)
	handler, err := New([]domains.Service{
		{Service: "kernel", Target: "KERNEL", WebSocket: true, Domains: []string{"facade"}},
		{Service: "storage", Target: "STORAGE", Domains: []string{"storage"}},
	}, func(string) string { return socket.URL }, page)
	if err != nil {
		t.Fatal(err)
	}
	edge := httptest.NewServer(handler)
	defer edge.Close()

	status, reader, connection := dialUpgrade(t, edge.URL, "/api/v1/facade/ws")
	defer connection.Close()
	if !strings.Contains(status, "101") {
		t.Fatalf("WebSocket déclaré : %q, attendu 101", status)
	}
	for line, _ := reader.ReadString('\n'); line != "\r\n" && line != ""; line, _ = reader.ReadString('\n') {
	}
	fmt.Fprint(connection, "ping\n")
	if echo, _ := reader.ReadString('\n'); echo != "echo ping\n" {
		t.Errorf("écho à travers le frontal : %q", echo)
	}

	status, _, refused := dialUpgrade(t, edge.URL, "/api/v1/storage/ws")
	defer refused.Close()
	if !strings.Contains(status, "404") {
		t.Errorf("WebSocket non déclaré : %q, attendu 404", status)
	}
}
