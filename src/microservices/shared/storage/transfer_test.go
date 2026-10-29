package storage

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Un transfert qui AVANCE n'est jamais coupé.
//
// La panne signalée le 2026-09-02 : PSP et PS2 s'arrêtaient sur
// « unexpected end of stream ». Les sources tournaient sur un
// `http.Client{Timeout: 30s}`, et en Go ce délai couvre TOUT l'échange,
// lecture du corps comprise : tout fichier qui mettait plus de trente
// secondes à descendre était coupé en plein vol, toujours au même endroit.
//
// Le test rejoue la panne en miniature : un serveur qui annonce son corps
// tout de suite puis l'égrène en petits morceaux. Le client borné dans le
// temps coupe au milieu ; NewTransferClient lit jusqu'au bout.
func TestATricklingBodyOutlivesAClientTimeout(t *testing.T) {
	const bodySize = 128

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "128")

		// Seize morceaux de huit octets, espacés : le corps met 2,4 s à
		// descendre — plus que le budget du client borné (1 s), moins qu'une
		// éternité pour la suite.
		for written := 0; written < bodySize; written += 8 {
			if _, err := io.WriteString(w, "abcdefgh"); err != nil {
				return
			}
			if flush, ok := w.(http.Flusher); ok {
				flush.Flush()
			}
			time.Sleep(150 * time.Millisecond)
		}
	}))
	defer server.Close()

	// LE CONTRE-EXEMPLE : le client qui a causé la panne. Une seconde de
	// budget total, un corps qui en met deux et demie : la LECTURE est
	// coupée en plein vol — le budget couvre l'échange entier, pas
	// seulement l'attente.
	short := &http.Client{Timeout: time.Second}
	shortResponse, err := short.Get(server.URL)
	if err != nil {
		t.Fatalf("requête courte : %v", err)
	}
	if _, err := io.ReadAll(shortResponse.Body); err == nil {
		_ = shortResponse.Body.Close()

		t.Fatal("le client borné dans le temps aurait dû couper le transfert")
	}
	_ = shortResponse.Body.Close()

	// LE CORRECTIF : aucune limite sur la durée totale, des limites sur
	// l'attente seulement.
	response, err := NewTransferClient().Get(server.URL)
	if err != nil {
		t.Fatalf("NewTransferClient : %v", err)
	}
	defer response.Body.Close()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("lecture : %v", err)
	}
	if len(content) != bodySize {
		t.Fatalf("%d octets lus, attendu %d — le transfert a été coupé", len(content), bodySize)
	}
}

// La taille annoncée voyage avec le flux : c'est elle qui fait la barre de
// progression. Un flux nu ne promet rien, et dit -1 plutôt que d'inventer.
func TestWithSizeAndSizeOf(t *testing.T) {
	body := io.NopCloser(strings.NewReader("ROM-BYTES"))
	if size := SizeOf(body); size != -1 {
		t.Fatalf("un flux nu annonce %d, attendu -1", size)
	}

	sized := WithSize(body, 131072)
	if size := SizeOf(sized); size != 131072 {
		t.Fatalf("le flux habillé annonce %d, attendu 131072", size)
	}

	// L'habillage ne change pas la lecture : il ne fait que répondre à une
	// question de plus.
	content, err := io.ReadAll(sized)
	if err != nil || string(content) != "ROM-BYTES" {
		t.Fatalf("lecture à travers WithSize = %q / %v", content, err)
	}
}
