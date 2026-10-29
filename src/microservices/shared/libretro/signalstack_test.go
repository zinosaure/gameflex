package libretro

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// fakeCoreSource — un cœur qui, à chacun de ses arrêts, fait ce que fait
// PPSSPP en retirant son gestionnaire de fautes : il laisse au fil appelant la
// pile de signaux d'un autre fil, dont la mémoire est déjà rendue (#356).
const fakeCoreSource = `
#include <signal.h>
#include <sys/mman.h>

static void leave_a_dead_signal_stack(void) {
    size_t size = 64 * 1024;
    void* region = mmap(0, size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    stack_t ss;
    ss.ss_sp = region;
    ss.ss_size = size;
    ss.ss_flags = 0;
    sigaltstack(&ss, 0);
    munmap(region, size);
}

void retro_init(void) {}
void retro_deinit(void) { leave_a_dead_signal_stack(); }
unsigned retro_api_version(void) { return 1; }
void retro_get_system_info(void* info) {}
void retro_get_system_av_info(void* info) {}
void retro_set_controller_port_device(unsigned port, unsigned device) {}
void retro_reset(void) { leave_a_dead_signal_stack(); }
void retro_run(void) {}
int retro_load_game(const void* game) { return 1; }
void retro_unload_game(void) { leave_a_dead_signal_stack(); }
void retro_set_video_refresh(void* cb) {}
void retro_set_audio_sample(void* cb) {}
void retro_set_audio_sample_batch(void* cb) {}
void retro_set_input_poll(void* cb) {}
void retro_set_input_state(void* cb) {}
void retro_set_environment(void* cb) {}
`

// buildFakeCore compile le faux cœur, ou saute le test sans compilateur C.
func buildFakeCore(t *testing.T) string {
	t.Helper()

	return buildCore(t, fakeCoreSource)
}

// buildCore compile un cœur de test depuis sa source C, ou saute le test sans
// compilateur C.
func buildCore(t *testing.T, coreSource string) string {
	t.Helper()
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("pas de compilateur C pour construire le faux cœur")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "fake_core.c")
	if err := os.WriteFile(source, []byte(coreSource), 0o644); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(dir, "fake_core.so")
	if out, err := exec.Command(compiler, "-shared", "-fPIC", "-o", library, source).CombinedOutput(); err != nil {
		t.Fatalf("faux cœur : %v\n%s", err, out)
	}

	return library
}

// LE FIL DU MOTEUR GARDE SA PILE DE SIGNAUX QUAND LE CŒUR S'ARRÊTE (#356).
//
// Sur le S25, le téléphone mourait au hasard en quittant une partie PSP :
// PPSSPP laissait au fil du moteur la pile de signaux de son fil de chargement,
// terminé depuis, et le signal suivant (la préemption de Go) ne pouvait plus
// être servi — le noyau tuait le processus d'un SIGSEGV, sans rapport. Chaque
// arrêt du cœur doit rendre au fil la pile qu'il avait avant, et le fil doit
// ensuite survivre à un signal.
func TestCoreStopsGiveTheThreadItsSignalStackBack(t *testing.T) {
	library := buildFakeCore(t)

	// Jamais déverrouillé : si le test échoue, Go jette ce fil.
	runtime.LockOSThread()
	if err := LoadCore(library); err != nil {
		t.Fatal(err)
	}
	defer UnloadCore()
	NoteSignalStack()

	for _, stop := range []struct {
		name string
		call func()
	}{
		{"retro_unload_game", UnloadGame},
		{"retro_deinit", Deinit},
		{"retro_reset", Reset},
	} {
		stop.call()

		replaced := SignalStackReplaced()
		if !strings.Contains(replaced, "NON MAPPÉE") {
			t.Fatalf("%s : la pile laissée par le cœur n'a pas été vue ni remise (%q)", stop.name, replaced)
		}
		if drift := SignalStackDrift(); drift != "" {
			t.Fatalf("%s : le fil n'a pas retrouvé sa pile de signaux : %s", stop.name, drift)
		}
		if again := SignalStackReplaced(); again != "" {
			t.Fatalf("%s : le compte rendu est resservi : %q", stop.name, again)
		}

		// Le signal de préemption de Go, sur CE fil : sans la pile remise, le
		// processus de test mourrait ici d'un SIGSEGV.
		if err := syscall.Tgkill(syscall.Getpid(), syscall.Gettid(), syscall.SIGURG); err != nil {
			t.Fatalf("%s : tgkill : %v", stop.name, err)
		}
	}
}

// UN FIL QUE LE CŒUR NE TOUCHE PAS N'A RIEN À REMETTRE : pas de compte rendu,
// pas d'écart.
func TestAnUntouchedSignalStackIsLeftAlone(t *testing.T) {
	runtime.LockOSThread()
	NoteSignalStack()
	if drift := SignalStackDrift(); drift != "" {
		t.Fatalf("écart sans cœur : %s", drift)
	}
	if replaced := SignalStackReplaced(); replaced != "" {
		t.Fatalf("compte rendu sans arrêt : %q", replaced)
	}
}
