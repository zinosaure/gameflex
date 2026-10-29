package remote

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
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
