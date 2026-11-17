package x11grab

import "fmt"

// LES PALIERS DU TÉLÉPHONE — chantier 58, P3. Une trame NV12 (le plan Y, puis
// UN plan où U et V alternent, ce que prend l'encodeur matériel) réduite à la
// taille d'un palier : 720p ou 540p tiré de l'image que la boucle de l'écran X
// vient de prélever, jamais d'une seconde capture (cinq à six fois plus chère
// sur le S25, journal 286). Bilinéaire, centres des pixels alignés ; par
// moyenne de quatre quand la sortie fait exactement la moitié (1080p → 540p).
//
// Les dimensions sont paires.
func ScaleNV12(src []byte, srcW, srcH int, dst []byte, dstW, dstH int) error {
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 || srcW%2 != 0 || srcH%2 != 0 || dstW%2 != 0 || dstH%2 != 0 {
		return fmt.Errorf("dimensions invalides : %dx%d → %dx%d (paires attendues)", srcW, srcH, dstW, dstH)
	}
	if len(src) < srcW*srcH*3/2 {
		return fmt.Errorf("source trop courte : %d < %d", len(src), srcW*srcH*3/2)
	}
	if len(dst) < dstW*dstH*3/2 {
		return fmt.Errorf("sortie trop courte : %d < %d", len(dst), dstW*dstH*3/2)
	}
	srcLuma, dstLuma := srcW*srcH, dstW*dstH
	if dstW*2 == srcW && dstH*2 == srcH {
		halvePlane(src[:srcLuma], srcW, srcH, 1, dst[:dstLuma])
		halvePlane(src[srcLuma:srcLuma*3/2], srcW/2, srcH/2, 2, dst[dstLuma:dstLuma*3/2])

		return nil
	}
	scaleNV12Plane(src[:srcLuma], srcW, srcH, 1, dst[:dstLuma], dstW, dstH)
	scaleNV12Plane(src[srcLuma:srcLuma*3/2], srcW/2, srcH/2, 2, dst[dstLuma:dstLuma*3/2], dstW/2, dstH/2)

	return nil
}

// halvePlane — un plan de w×h éléments de `channels` octets chacun, réduit de
// moitié par moyenne de quatre, canal par canal.
func halvePlane(src []byte, w, h, channels int, dst []byte) {
	stride, half := w*channels, w/2
	for row := 0; row < h/2; row++ {
		top := src[2*row*stride:]
		bottom := src[(2*row+1)*stride:]
		out := dst[row*half*channels:]
		for column := 0; column < half; column++ {
			for channel := 0; channel < channels; channel++ {
				a, b := 2*column*channels+channel, (2*column+1)*channels+channel
				sum := int(top[a]) + int(top[b]) + int(bottom[a]) + int(bottom[b])
				out[column*channels+channel] = byte((sum + 2) >> 2)
			}
		}
	}
}

// scaleNV12Plane — un plan de srcW×srcH éléments de `channels` octets vers
// dstW×dstH, en bilinéaire.
func scaleNV12Plane(src []byte, srcW, srcH, channels int, dst []byte, dstW, dstH int) {
	columns, columnWeights := taps(srcW, dstW)
	rows, rowWeights := taps(srcH, dstH)
	stride := srcW * channels
	for row := 0; row < dstH; row++ {
		top := src[rows[row]*stride:]
		bottom := top
		if rows[row]+1 < srcH {
			bottom = src[(rows[row]+1)*stride:]
		}
		down := rowWeights[row]
		out := dst[row*dstW*channels:]
		for column := 0; column < dstW; column++ {
			left := columns[column]
			right := min(left+1, srcW-1)
			across := columnWeights[column]
			for channel := 0; channel < channels; channel++ {
				l, r := left*channels+channel, right*channels+channel
				upper := int(top[l])*(256-across) + int(top[r])*across
				lower := int(bottom[l])*(256-across) + int(bottom[r])*across
				out[column*channels+channel] = byte((upper*(256-down) + lower*down + 32768) >> 16)
			}
		}
	}
}

// taps — pour chaque position de sortie, l'élément source de gauche (ou du
// haut) et le poids du suivant, sur 256.
func taps(srcSize, dstSize int) ([]int, []int) {
	first := make([]int, dstSize)
	weight := make([]int, dstSize)
	for index := range first {
		position := (float64(index)+0.5)*float64(srcSize)/float64(dstSize) - 0.5
		if position < 0 {
			position = 0
		}
		left := int(position)
		if left >= srcSize-1 {
			left, position = srcSize-1, float64(srcSize-1)
		}
		first[index] = left
		weight[index] = int((position - float64(left)) * 256)
	}

	return first, weight
}
