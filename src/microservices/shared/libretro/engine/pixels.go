package engine

import (
	"math"
	"unsafe"
)

// Les formats de pixel qu'un cœur annonce par SET_PIXEL_FORMAT (libretro.h,
// enum retro_pixel_format).
const (
	PixelFormat0RGB1555 = 0
	PixelFormatXRGB8888 = 1
	PixelFormatRGB565   = 2
)

// ToBGRA copie l'image qu'un cœur vient de rendre — `data`, valable seulement
// pendant l'appel de video_refresh — en BGRA compact, rangées de haut en bas,
// alpha opaque. C'est la forme que tout le reste attend : l'encodeur du serveur
// (ffmpeg, `-pixel_format bgra`), la relecture du rendu matériel
// (gl_context.ReadPixels), et la conversion NV12 du téléphone.
//
// Écrite pour le serveur (stream_session.go), sortie ici pour servir aussi le
// téléphone (chantier 13) : une seule conversion, pas deux qui divergent.
func ToBGRA(data unsafe.Pointer, w, h, pitch, format int) []byte {
	src := unsafe.Slice((*byte)(data), pitch*h)
	frame := make([]byte, w*h*4)

	switch format {
	case PixelFormat0RGB1555: // native (LE) byte layout [GGGBBBBB][0RRRRRGG]
		for y := 0; y < h; y++ {
			rowSrc := src[y*pitch : y*pitch+w*2]
			rowDst := frame[y*w*4 : (y+1)*w*4]
			for x := 0; x < w; x++ {
				lo, hi := uint16(rowSrc[x*2]), uint16(rowSrc[x*2+1])
				rgb := hi<<8 | lo
				r, g, b := (rgb>>10)&0x1F, (rgb>>5)&0x1F, rgb&0x1F
				rowDst[x*4] = byte(b<<3 | b>>2)
				rowDst[x*4+1] = byte(g<<3 | g>>2)
				rowDst[x*4+2] = byte(r<<3 | r>>2)
				rowDst[x*4+3] = 255
			}
		}

	case PixelFormatRGB565: // native (LE) byte layout [GGGBBBBB][RRRRRGGG]
		for y := 0; y < h; y++ {
			rowSrc := src[y*pitch : y*pitch+w*2]
			rowDst := frame[y*w*4 : (y+1)*w*4]
			for x := 0; x < w; x++ {
				lo, hi := uint16(rowSrc[x*2]), uint16(rowSrc[x*2+1])
				rgb := hi<<8 | lo
				r, g, b := (rgb>>11)&0x1F, (rgb>>5)&0x3F, rgb&0x1F
				rowDst[x*4] = byte(b<<3 | b>>2)
				rowDst[x*4+1] = byte(g<<2 | g>>4)
				rowDst[x*4+2] = byte(r<<3 | r>>2)
				rowDst[x*4+3] = 255
			}
		}

	default: // XRGB8888 (format 1 or unknown) — the value
		// is 0xXXRRGGBB as a native uint32; on every real (little-endian)
		// target that means the LEAST significant byte (Blue) is stored
		// FIRST in memory, giving byte order [B][G][R][X] — already BGR,
		// matching what emitFrame/ffmpeg expect (bgra). The previous R/B
		// byte swap here assumed the opposite ([X][R][G][B]) and was
		// wrong — confirmed via a real report of PSP (the only console
		// that actually exercises this path — N64/PS2 use the hw-render
		// path (gl_context), PS1/GBC/etc. report RGB1555/RGB565 instead) coming
		// out blue-tinted (R and B channels swapped). Only the alpha byte
		// (X, core-defined/unused) needs to be forced to opaque.
		for y := 0; y < h; y++ {
			rowSrc := src[y*pitch : y*pitch+w*4]
			rowDst := frame[y*w*4 : (y+1)*w*4]
			copy(rowDst, rowSrc)
			for i := 3; i < len(rowDst); i += 4 {
				rowDst[i] = 255
			}
		}
	}

	return frame
}

// FitBGRA met une image BGRA (fw×fh) dans le canevas de l'encodeur (encW×encH)
// AU FORMAT DU JEU, centrée, le reste en noir opaque (décision du porteur le
// 2026-10-15 : « pour les rétro, c'est bande noire »). `aspect` est le rapport
// largeur/hauteur d'affichage que le cœur annonce (Frame.Aspect) ; ≤ 0, celui
// de l'image. Plus proche voisin, comme avant.
//
// Elle ÉTIRAIT (ScaleBGRA) : dans le flux 16:9 du défaut (540p), une NES, une
// N64 ou une PS1 sortaient élargies d'un tiers, une DS écrasée — pendant que la
// Xbox (streamex) gardait ses bandes noires (#377).
//
// Un écart de moins de 1 % entre les deux formats remplit tout le canevas (la
// PSP, 480×272, contre 16:9) : un filet noir d'un pixel ne sert à personne.
// Rend `frame` tel quel quand il est déjà à la taille du canevas.
//
// Un tampon NEUF à chaque appel : le serveur garde l'image après coup (la
// vignette d'une sauvegarde, la file de l'encodeur). Qui la consomme aussitôt
// prend FitBGRAInto.
func FitBGRA(frame []byte, fw, fh, encW, encH int, aspect float64) []byte {
	return FitBGRAInto(nil, frame, fw, fh, encW, encH, aspect)
}

// FitBGRAInto — FitBGRA, écrite dans `dst` quand il est assez grand (sinon un
// tampon neuf). Pour qui convertit l'image aussitôt et n'en garde rien : le
// téléphone (EngineFrameNV12) n'alloue plus 2 Mo par image, soit ~124 Mo/s
// pour son ramasse-miettes à 60 images par seconde (BenchmarkFitBGRA).
//
// UNE LIGNE SOURCE NE SE CALCULE QU'UNE FOIS : agrandie (240 lignes NES vers
// 540), elle sert deux ou trois lignes du canevas, qui la recopient d'un bloc.
func FitBGRAInto(dst, frame []byte, fw, fh, encW, encH int, aspect float64) []byte {
	if fw == encW && fh == encH {
		return frame
	}
	if !PlausibleAspect(aspect) {
		aspect = float64(fw) / float64(fh)
	}
	if !PlausibleAspect(aspect) {
		aspect = float64(encW) / float64(encH)
	}
	cw, ch := encW, int(math.Round(float64(encW)/aspect))
	if ch > encH {
		ch = encH
		cw = int(math.Round(float64(encH) * aspect))
	}
	cw, ch = max(cw, 1), max(ch, 1)
	if encW-cw <= encW/100 && encH-ch <= encH/100 {
		cw, ch = encW, encH
	}
	need := encW * encH * 4
	fitted := dst
	if cap(fitted) < need {
		fitted = make([]byte, need)
	}
	fitted = fitted[:need]
	if cw < encW || ch < encH {
		blackOut(fitted)
	}
	x0, y0 := (encW-cw)/2, (encH-ch)/2
	columns := make([]int, cw)
	for dx := range columns {
		columns[dx] = dx * fw / cw * 4
	}
	previous, previousSy := -1, -1
	for dy := 0; dy < ch; dy++ {
		sy := dy * fh / ch
		row := ((y0+dy)*encW + x0) * 4
		dstRow := fitted[row : row+cw*4]
		if sy == previousSy {
			copy(dstRow, fitted[previous:previous+cw*4])

			continue
		}
		srcRow := frame[sy*fw*4 : (sy+1)*fw*4]
		for dx, off := range columns {
			copy(dstRow[dx*4:dx*4+4], srcRow[off:off+4])
		}
		previous, previousSy = row, sy
	}

	return fitted
}

// PlausibleAspect — un format d'affichage qui peut être celui d'un écran : un
// nombre fini entre 1:10 et 10:1 (l'écran haut d'une DS fait 2:3, un jeu
// d'arcade tourné 3:4). Un format qui n'est pas un nombre, infini, nul ou
// négatif ne laissait aucune ligne à dessiner : l'image sortait noire.
func PlausibleAspect(aspect float64) bool {
	return aspect >= 0.1 && aspect <= 10
}

// blackOut peint tout le tampon BGRA en noir opaque — l'alpha que ToBGRA
// promet —, par copies doublées plutôt que pixel à pixel.
func blackOut(pixels []byte) {
	if len(pixels) < 4 {
		return
	}
	pixels[0], pixels[1], pixels[2], pixels[3] = 0, 0, 0, 255
	for filled := 4; filled < len(pixels); filled *= 2 {
		copy(pixels[filled:], pixels[:filled])
	}
}
