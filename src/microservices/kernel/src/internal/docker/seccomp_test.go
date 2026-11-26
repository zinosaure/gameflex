package docker

import (
	"encoding/json"
	"slices"
	"testing"
)

// LE PROFIL DE L'ÉMULATEUR QUI SUIT LA MÉMOIRE DU JEU : celui de Docker, userfaultfd en plus, et un JSON que le démon
// lira — un fichier abîmé ferait refuser chaque partie AstroHybrid à la création du conteneur.
func TestTheWriteWatchSeccompProfileAllowsUserfaultfd(t *testing.T) {
	var profile struct {
		DefaultAction string `json:"defaultAction"`
		Syscalls      []struct {
			Names  []string `json:"names"`
			Action string   `json:"action"`
		} `json:"syscalls"`
	}
	if err := json.Unmarshal([]byte(userfaultfdSeccomp), &profile); err != nil {
		t.Fatalf("seccomp-userfaultfd.json: %v", err)
	}
	if profile.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Errorf("defaultAction = %q, want Docker's SCMP_ACT_ERRNO", profile.DefaultAction)
	}
	if len(profile.Syscalls) == 0 || profile.Syscalls[0].Action != "SCMP_ACT_ALLOW" || !slices.Contains(profile.Syscalls[0].Names, "userfaultfd") {
		t.Error("userfaultfd is not in the profile's allow list")
	}
}

// SEUL ASTROHYBRID reçoit SYS_PTRACE et ce profil.
func TestOnlyAstroHybridWatchesGuestWrites(t *testing.T) {
	for console, emulators := range streamexEmulators {
		for _, emulator := range emulators {
			if emulator.writeWatch != (emulator.core == "AstroHybrid (streamex)") {
				t.Errorf("%s/%s: writeWatch = %v", console, emulator.core, emulator.writeWatch)
			}
		}
	}
}
