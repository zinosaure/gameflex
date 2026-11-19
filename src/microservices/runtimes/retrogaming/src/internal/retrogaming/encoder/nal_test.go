package encoder

import (
	"bytes"
	"testing"
)

func TestAppendNALToAccessUnitEmitsOneCompleteFrame(t *testing.T) {
	aud := []byte{0, 0, 0, 1, 9, 0x10}
	sps := []byte{0, 0, 0, 1, 7, 0x42, 0xc0, 0x28}
	pps := []byte{0, 0, 0, 1, 8, 0xcb, 0x8f, 0x20}
	sei := []byte{0, 0, 0, 1, 6, 1, 2, 3}
	idr := []byte{0, 0, 0, 1, 5, 0xaa, 0xbb}

	var pending, frame []byte
	for _, nal := range [][]byte{aud, sps, pps, sei} {
		pending, frame = appendNALToAccessUnit(pending, nal)
		if frame != nil {
			t.Fatalf("prefix NAL emitted a frame: %x", frame)
		}
	}
	pending, frame = appendNALToAccessUnit(pending, idr)

	want := bytes.Join([][]byte{aud, sps, pps, sei, idr}, nil)
	if !bytes.Equal(frame, want) {
		t.Fatalf("complete frame = %x, want %x", frame, want)
	}
	if len(pending) != 0 {
		t.Fatalf("pending bytes after coded slice = %d, want 0", len(pending))
	}

	p := []byte{0, 0, 0, 1, 1, 0xcc}
	pending, frame = appendNALToAccessUnit(pending, aud)
	pending, frame = appendNALToAccessUnit(pending, p)
	if !bytes.Equal(frame, append(append([]byte(nil), aud...), p...)) {
		t.Fatalf("second frame = %x", frame)
	}
}
