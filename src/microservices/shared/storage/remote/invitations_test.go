package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

// LE LIEN D'UNE INVITATION, QU'IL SOIT COLLÉ OU LU DANS UN QR : l'adresse du
// serveur (préfixe compris) et un code de 22 signes ; le reste n'en est pas un.
func TestParseInvitation(t *testing.T) {
	const code = "AbCdEfGhIjKlMnOpQrSt_-"
	for text, want := range map[string]string{
		"https://gameflex.example.com/invite/" + code:        "https://gameflex.example.com",
		"  http://192.168.1.50:10200/invite/" + code + "/  ": "http://192.168.1.50:10200",
		"https://example.com/jeux/invite/" + code:            "https://example.com/jeux",
	} {
		link, got, err := ParseInvitation(text)
		if err != nil || link != want || got != code {
			t.Errorf("%q → %q %q %v, attendu %q", text, link, got, err, want)
		}
	}
	for _, text := range []string{
		"", code, "ftp://example.com/invite/" + code, "https://example.com/" + code,
		"https://example.com/invite/court", "https://example.com/invite/" + code + "x",
		"https://example.com/invite/AbCdEfGhIjKlMnOpQrSt+/",
	} {
		if _, _, err := ParseInvitation(text); !errors.Is(err, ErrNotInvitation) {
			t.Errorf("%q accepté (%v)", text, err)
		}
	}
}

// LE QR SE RELIT : dessiné ici, il rend son lien ; agrandi et posé sur une
// image plus grande, comme une capture d'écran, aussi. Une image sans QR, ou
// le QR d'autre chose, non.
func TestInvitationQRRoundTrip(t *testing.T) {
	link := "https://gameflex.example.com/invite/AbCdEfGhIjKlMnOpQrSt_-"
	encoded, err := InvitationPNG(link, 320)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ReadInvitation(bytes.NewReader(encoded)); err != nil || got != link {
		t.Fatalf("relu %q, %v", got, err)
	}

	qr, _ := png.Decode(bytes.NewReader(encoded))
	screen := image.NewRGBA(image.Rect(0, 0, 1080, 1600))
	draw.Draw(screen, screen.Bounds(), image.NewUniform(color.RGBA{30, 32, 38, 255}), image.Point{}, draw.Src)
	draw.Draw(screen, image.Rect(380, 600, 700, 920), qr, image.Point{}, draw.Src)
	var capture bytes.Buffer
	_ = jpeg.Encode(&capture, screen, &jpeg.Options{Quality: 70})
	if got, err := ReadInvitation(&capture); err != nil || got != link {
		t.Fatalf("dans une capture JPEG : %q, %v", got, err)
	}

	var blank bytes.Buffer
	_ = png.Encode(&blank, image.NewGray(image.Rect(0, 0, 200, 200)))
	if _, err := ReadInvitation(&blank); !errors.Is(err, ErrNoQR) {
		t.Fatalf("sans QR : %v", err)
	}
	if _, err := InvitationPNG("https://example.com/pas-une-invitation", 320); !errors.Is(err, ErrNotInvitation) {
		t.Fatalf("autre chose qu'une invitation dessinée : %v", err)
	}
}

// LE LIEN D'UN COMPTE (chantier 55) : le QR le dessine, mais il n'est pas une
// invitation de serveur — ParseInvitation le refuse, et relu dans une image il
// n'est pas pris pour l'une d'elles.
func TestAMemberLinkIsDrawnButNeverRedeemed(t *testing.T) {
	link := "http://192.168.1.50:11200/join/AbCdEfGhIjKlMnOpQrSt_-"
	if !IsMemberLink(link) || IsMemberLink("https://example.com/join/court") || IsMemberLink("https://example.com/invite/AbCdEfGhIjKlMnOpQrSt_-") {
		t.Fatal("IsMemberLink se trompe")
	}
	if _, _, err := ParseInvitation(link); !errors.Is(err, ErrNotInvitation) {
		t.Fatalf("le lien d'un compte pris pour une invitation : %v", err)
	}
	encoded, err := InvitationPNG(link, 320)
	if err != nil {
		t.Fatalf("le QR du lien d'un compte : %v", err)
	}
	if _, err := ReadInvitation(bytes.NewReader(encoded)); !errors.Is(err, ErrNotInvitation) {
		t.Fatalf("le QR d'un lien de compte relu comme une invitation : %v", err)
	}
}

// LE CODE D'APPAIRAGE (chantier 60) : six chiffres, espaces retirés ; une adresse
// tapée sans schéma reçoit http:// au réseau local, https:// ailleurs.
func TestPairingCodeAndTypedLink(t *testing.T) {
	for text, want := range map[string]string{"482913": "482913", " 482 913 ": "482913", "000042": "000042"} {
		if code, err := PairingCode(text); err != nil || code != want {
			t.Errorf("%q → %q %v", text, code, err)
		}
	}
	for _, text := range []string{"", "48291", "4829134", "48291a", "AbCdEfGhIjKlMnOpQrSt_-"} {
		if _, err := PairingCode(text); !errors.Is(err, ErrNotPairingCode) {
			t.Errorf("%q accepté (%v)", text, err)
		}
	}

	for text, want := range map[string]string{
		"192.168.1.10:11200":            "http://192.168.1.10:11200",
		"gamebox.local:10200/":          "http://gamebox.local:10200",
		"gamebox:10200":                 "http://gamebox:10200",
		"10.20.0.2:10200":               "http://10.20.0.2:10200",
		"gameflex.example.com":          "https://gameflex.example.com",
		"https://gameflex.example.com/": "https://gameflex.example.com",
		"http://203.0.113.4:10200":      "http://203.0.113.4:10200",
	} {
		if link, err := NormalizeLink(text); err != nil || link != want {
			t.Errorf("%q → %q %v, attendu %q", text, link, err, want)
		}
	}
	if _, err := NormalizeLink("  "); !errors.Is(err, ErrNoLink) {
		t.Errorf("adresse vide : %v", err)
	}
}

// UN SERVEUR HORS DU MODE DÉCOUVERTE RÉPOND 423 : c'est `pairingClosed`, que la
// page traduit ; notre nom part avec le code.
func TestRedeemSaysPairingClosed(t *testing.T) {
	var sent map[string]string
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(http.StatusLocked)
	}))
	defer peer.Close()
	distant, _ := New(peer.URL, "")
	_, err := distant.Redeem("482913", InvitationFederation, "Gamebox")
	if !errors.Is(err, ErrPairingClosed) || sent["from"] != "Gamebox" || sent["code"] != "482913" {
		t.Fatalf("%v, envoyé %v", err, sent)
	}
	if status, code, ok := RefusalCode(err); !ok || code != "pairingClosed" || status != http.StatusBadRequest {
		t.Fatalf("RefusalCode : %d %q %v", status, code, ok)
	}
	if _, code, _ := RefusalCode(ErrNotPairingCode); code != "notPairingCode" {
		t.Fatalf("RefusalCode d'un code mal formé : %q", code)
	}
}
