package encoder

import "testing"

func TestLevel40Canvas(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		fps           float64
		wantW, wantH  int
		wantCapped    bool
	}{
		{"720p60 reste valide", 1280, 720, 60, 1280, 720, false},
		{"1080p30 reste valide", 1920, 1080, 30, 1920, 1080, false},
		{"1080p60 devient 720p60", 1920, 1080, 60, 1280, 720, true},
		{"Dolphin historique reste valide", 640, 528, 60, 640, 528, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			width, height, capped := Level40Canvas(test.width, test.height, test.fps)
			if width != test.wantW || height != test.wantH || capped != test.wantCapped {
				t.Fatalf("Level40Canvas(%dx%d, %.2f) = %dx%d, %v; want %dx%d, %v", test.width, test.height, test.fps, width, height, capped, test.wantW, test.wantH, test.wantCapped)
			}
		})
	}
}
