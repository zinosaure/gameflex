package remote

// LE QR D'UNE INVITATION (chantier 33) — dessiné ici en PNG, et relu ici dans
// une image déposée (une capture reçue par une messagerie, une photo) : une
// seule implémentation pour le serveur et le téléphone, et la page n'en charge
// aucune. gozxing (Apache-2.0), le portage Go de ZXing, fait les deux.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

const (
	// qrMaxBytes — une capture ou une photo de téléphone tient largement.
	qrMaxBytes = 16 << 20
	// qrMaxPixels — au-delà, l'image décodée pèserait des centaines de Mo :
	// une image piégée ne doit pas pouvoir épuiser la mémoire.
	qrMaxPixels = 40_000_000
)

// ErrNoQR — l'image ne porte pas de QR lisible.
var ErrNoQR = errors.New("aucun QR lisible dans cette image")

// InvitationPNG — le lien d'une invitation en QR : une image PNG carrée de
// `size` pixels, sa marge comprise. Correction « M » : une capture un peu
// compressée par une messagerie se relit encore.
func InvitationPNG(link string, size int) ([]byte, error) {
	if _, _, err := ParseInvitation(link); err != nil {
		return nil, err
	}
	matrix, err := qrcode.NewQRCodeWriter().Encode(link, gozxing.BarcodeFormat_QR_CODE, size, size,
		map[gozxing.EncodeHintType]any{gozxing.EncodeHintType_ERROR_CORRECTION: "M", gozxing.EncodeHintType_MARGIN: 2})
	if err != nil {
		return nil, err
	}
	// En niveaux de gris : quelques kilo-octets au lieu d'un RGBA.
	gray := image.NewGray(matrix.Bounds())
	draw.Draw(gray, gray.Bounds(), matrix, image.Point{}, draw.Src)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, gray); err != nil {
		return nil, err
	}

	return encoded.Bytes(), nil
}

// ReadInvitation — le lien d'invitation que porte le QR d'une image PNG ou
// JPEG. ErrNoQR sans QR lisible, ErrNotInvitation si ce n'est pas le nôtre.
func ReadInvitation(r io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r, qrMaxBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) > qrMaxBytes {
		return "", fmt.Errorf("image trop lourde (plus de %d Mo)", qrMaxBytes>>20)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return "", ErrNoQR
	}
	if config.Width*config.Height > qrMaxPixels {
		return "", fmt.Errorf("image trop grande (%d × %d)", config.Width, config.Height)
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", ErrNoQR
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(decoded)
	if err != nil {
		return "", ErrNoQR
	}
	result, err := qrcode.NewQRCodeReader().Decode(bitmap, map[gozxing.DecodeHintType]any{gozxing.DecodeHintType_TRY_HARDER: true})
	if err != nil {
		return "", ErrNoQR
	}
	if _, _, err := ParseInvitation(result.GetText()); err != nil {
		return "", err
	}

	return result.GetText(), nil
}
