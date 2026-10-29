package session_test

import (
	"os/exec"
	"strings"
	"testing"
)

// The session model is the ONE piece of this repo an Android host could reuse
// as-is: the object, the invariants, the command queue and their transport,
// with no X11, no uinput, no PulseAudio anywhere near them. That is not a
// happy accident — it is what "no network, no docker, no clock of its own"
// bought, and chantier 4 paid for it without aiming at Android at all
// (docs/android-app.md, corrected 2026-08-25).
//
// It is also the kind of property that dies quietly. One import of a
// system-specific package in httphost.go and the whole asset is gone, months
// before anybody opens the Android plan again and discovers it. So the
// property is a test, not a paragraph: the thing that must stay true is
// checked by the same `go test` everything else runs through.
//
// CGO_ENABLED=0 is part of the claim. A build that needs cgo needs the NDK,
// which is exactly the risk the Android plan lists first.
func TestModelStillCompilesForAndroid(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no Go toolchain here — nothing to cross-compile with")
	}

	build := exec.Command("go", "build", "./...")
	build.Env = append(build.Environ(), "CGO_ENABLED=0", "GOOS=android", "GOARCH=arm64")

	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf(
			"the session model no longer builds for android/arm64 without cgo.\n"+
				"Something system-specific reached this package — see the note above.\n\n%s",
			strings.TrimSpace(string(output)),
		)
	}
}
