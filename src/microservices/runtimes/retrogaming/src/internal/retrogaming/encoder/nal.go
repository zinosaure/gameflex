package encoder

// IsVCLNal reports whether nal (Annex-B, including its start code) is a
// video coding layer NAL unit (a coded slice, types 1 or 5), as opposed to
// a parameter set (SPS/PPS) or any other non-picture NAL. Only slices carry
// a frame duration when written to the WebRTC track — giving every NAL the
// frame duration makes the RTP clock run faster than real time (each
// keyframe adds 2+ phantom frame durations for its SPS/PPS), which the
// browser's jitter buffer then compensates by growing to seconds of delay.
// Found live on the external/Switch path (2026-08-13), which handed the
// frame duration to EVERY NAL while the libretro path already filtered.
// Lives in encoder (not in a consumer package) because both the libretro
// session and the external runtime need it, and neither can import the
// other.
func IsVCLNal(nal []byte) bool {
	off := 0
	switch {
	case len(nal) >= 4 && nal[0] == 0 && nal[1] == 0 && nal[2] == 0 && nal[3] == 1:
		off = 4
	case len(nal) >= 3 && nal[0] == 0 && nal[1] == 0 && nal[2] == 1:
		off = 3
	default:
		return false
	}
	if off >= len(nal) {
		return false
	}
	nalType := nal[off] & 0x1F
	return nalType == 1 || nalType == 5
}
