package libretro

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// exitFaultCoreSource — un cœur qui fait, à la fin du fil qui l'a initialisé,
// ce que peut faire un pilote graphique que le cœur a fait travailler sur ce fil
// (PPSSPP sonde Vulkan dans retro_init) : une faute qu'il ATTEND et rattrape
// lui-même — une lecture prudente, sous son propre gestionnaire de SIGSEGV —,
// dans le destructeur d'une clé de pthread posée par retro_init (#356).
const exitFaultCoreSource = `
#include <pthread.h>
#include <setjmp.h>
#include <signal.h>
#include <sys/mman.h>

static pthread_key_t key;
static sigjmp_buf back;

static void on_fault(int sig) { (void)sig; siglongjmp(back, 1); }

static void careful_read_at_exit(void* value) {
    (void)value;
    void* page = mmap(0, 4096, PROT_NONE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    struct sigaction mine, previous;
    mine.sa_handler = on_fault;
    mine.sa_flags = 0;
    sigemptyset(&mine.sa_mask);
    sigaction(SIGSEGV, &mine, &previous);
    if (sigsetjmp(back, 1) == 0) {
        (void)*(volatile char*)page;
    }
    sigaction(SIGSEGV, &previous, 0);
    munmap(page, 4096);
}

void retro_init(void) {
    pthread_key_create(&key, careful_read_at_exit);
    pthread_setspecific(key, (void*)1);
}
void retro_deinit(void) {}
unsigned retro_api_version(void) { return 1; }
void retro_get_system_info(void* info) {}
void retro_get_system_av_info(void* info) {}
void retro_set_controller_port_device(unsigned port, unsigned device) {}
void retro_reset(void) {}
void retro_run(void) {}
int retro_load_game(const void* game) { return 1; }
void retro_unload_game(void) {}
void retro_set_video_refresh(void* cb) {}
void retro_set_audio_sample(void* cb) {}
void retro_set_audio_sample_batch(void* cb) {}
void retro_set_input_poll(void* cb) {}
void retro_set_input_state(void* cb) {}
void retro_set_environment(void* cb) {}
`

const exitChildEnv = "GF_THREAD_EXIT_CHILD"

// LA FIN DU FIL DU MOTEUR SE PASSE COMME CELLE DE N'IMPORTE QUEL FIL (#356).
//
// Go termine le fil d'une goroutine qui finit verrouillée, mais bloque avant
// TOUS ses signaux ; la libc y exécute ensuite les destructeurs inscrits pour
// ce fil. Une faute qu'un destructeur attend et rattrape devient alors mortelle,
// sans gestionnaire ni rapport — le S25 mourait ainsi en quittant une partie
// PSP. Le témoin, sans rien inscrire, doit mourir d'un SIGSEGV (la scène est la
// bonne) ; avec UnblockFaultsAtThreadExit, le fil finit et le processus vit.
func TestAThreadExitRunsTheCoreDestructorsWithFaultsOpen(t *testing.T) {
	if mode := os.Getenv(exitChildEnv); mode != "" {
		runThreadExitChild(t, mode)

		return
	}
	library := buildCore(t, exitFaultCoreSource)

	for _, scene := range []struct {
		mode       string
		wantSignal bool
	}{
		{"témoin", true},
		{"rouvert", false},
	} {
		child := exec.Command(os.Args[0], "-test.run=^TestAThreadExitRunsTheCoreDestructorsWithFaultsOpen$", "-test.count=1")
		child.Env = append(os.Environ(), exitChildEnv+"="+scene.mode, "GF_EXIT_CORE="+library)
		out, err := child.CombinedOutput()
		killed := false
		if exit, ok := err.(*exec.ExitError); ok {
			if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() && status.Signal() == syscall.SIGSEGV {
				killed = true
			}
		}
		if scene.wantSignal && !killed {
			t.Fatalf("%s : le processus devait mourir d'un SIGSEGV à la fin du fil (%v)\n%s", scene.mode, err, out)
		}
		if !scene.wantSignal && (err != nil || !strings.Contains(string(out), "fin du fil passée")) {
			t.Fatalf("%s : la fin du fil a tué le processus (%v)\n%s", scene.mode, err, out)
		}
	}
}

// runThreadExitChild — dans le processus enfant : le cœur initialisé sur un fil
// verrouillé qui n'est pas m0, la goroutine qui finit, Go qui termine le fil.
func runThreadExitChild(t *testing.T, mode string) {
	// La mort attendue du témoin ne doit pas laisser de fichier core.
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{})
	if err := LoadCore(os.Getenv("GF_EXIT_CORE")); err != nil {
		t.Fatal(err)
	}
	thread := make(chan int)
	go func() {
		runtime.LockOSThread()
		if syscall.Gettid() != syscall.Getpid() {
			initThenFinish(mode, thread)

			return
		}
		// m0 : Go ne le termine jamais. Le garder le temps qu'une autre
		// goroutine verrouille son propre fil (comme onOwnThread du moteur).
		locked := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			close(locked)
			initThenFinish(mode, thread)
		}()
		<-locked
		runtime.UnlockOSThread()
	}()
	tid := <-thread
	for start := time.Now(); time.Since(start) < 5*time.Second; time.Sleep(2 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join("/proc/self/task", strconv.Itoa(tid))); os.IsNotExist(err) {
			fmt.Println("fin du fil passée")
			os.Exit(0)
		}
	}
	fmt.Println("le fil vit encore après 5 s")
	os.Exit(3)
}

func initThenFinish(mode string, thread chan<- int) {
	Init()
	if mode == "rouvert" && !UnblockFaultsAtThreadExit() {
		fmt.Println("inscription refusée")
	}
	thread <- syscall.Gettid()
}
