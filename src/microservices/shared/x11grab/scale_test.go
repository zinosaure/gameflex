package x11grab

import "testing"

// Une trame NV12 unie se réduit en la même couleur, par moitié comme en
// bilinéaire ; un dégradé garde son ordre (chantier 58, P3).
func TestScaleNV12(t *testing.T) {
	const w, h = 64, 36
	src := make([]byte, w*h*3/2)
	for i := 0; i < w*h; i++ {
		src[i] = 100
	}
	for i := w * h; i < len(src); i += 2 {
		src[i], src[i+1] = 60, 200
	}
	for _, size := range [][2]int{{32, 18}, {48, 26}} {
		dst := make([]byte, size[0]*size[1]*3/2)
		if err := ScaleNV12(src, w, h, dst, size[0], size[1]); err != nil {
			t.Fatal(err)
		}
		luma := size[0] * size[1]
		if dst[0] != 100 || dst[luma-1] != 100 || dst[luma] != 60 || dst[luma+1] != 200 || dst[len(dst)-1] != 200 {
			t.Errorf("%v : Y %d…%d, U %d, V %d…%d", size, dst[0], dst[luma-1], dst[luma], dst[luma+1], dst[len(dst)-1])
		}
	}
	// Un dégradé horizontal de la luma : croissant après réduction.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src[y*w+x] = byte(x * 4)
		}
	}
	dst := make([]byte, 48*26*3/2)
	if err := ScaleNV12(src, w, h, dst, 48, 26); err != nil {
		t.Fatal(err)
	}
	for x := 1; x < 48; x++ {
		if dst[x] < dst[x-1] {
			t.Fatalf("dégradé cassé en %d : %d < %d", x, dst[x], dst[x-1])
		}
	}
	if err := ScaleNV12(src, w, h, make([]byte, 10), 48, 26); err == nil {
		t.Error("une sortie trop courte doit être refusée")
	}
}
