// Package libretro provides CGo bindings to dynamically load and interact
// with Libretro emulator cores (.so files).
//
// It wraps dlopen/dlsym to load a core at runtime and exposes Go-friendly
// functions for the full emulation lifecycle: init, load game, run frames,
// and capturing audio/video output through callbacks.
package libretro

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <string.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <unistd.h>
#include <signal.h>
#include <pthread.h>
#include "libretro.h"

// ============================================================================
// Global function pointers to the loaded core
// ============================================================================
static void* core_handle = NULL;

static retro_init_t                         fp_retro_init;
static retro_deinit_t                       fp_retro_deinit;
static retro_api_version_t                  fp_retro_api_version;
static retro_get_system_info_t              fp_retro_get_system_info;
static retro_get_system_av_info_t           fp_retro_get_system_av_info;
static retro_set_controller_port_device_t   fp_retro_set_controller_port_device;
static retro_reset_t                        fp_retro_reset;
static retro_run_t                          fp_retro_run;
static retro_load_game_t                    fp_retro_load_game;
static retro_unload_game_t                  fp_retro_unload_game;
static retro_serialize_t                    fp_retro_serialize;
static retro_unserialize_t                  fp_retro_unserialize;
static retro_serialize_size_t               fp_retro_serialize_size;

// LA SAUVEGARDE DE LA CARTOUCHE — celle que le JEU écrit, pas la nôtre.
//
// Ces deux-là manquaient, et c'est le trou le plus coûteux qu'on ait eu :
// dans libretro, la SRAM (la pile d'une cartouche, la memory card d'une PS1)
// n'est PAS écrite par le cœur. Il l'expose en mémoire, et c'est au FRONTEND
// de la relire et de la ranger. GET_SAVE_DIRECTORY ne sert qu'aux cœurs qui
// gèrent leurs propres fichiers — PPSSPP, melonDS — et ceux-là marchaient.
// Tous les autres perdaient la partie sauvegardée À CHAQUE FIN DE SESSION :
// aucun .srm sur le disque, sur huit consoles jouées (constaté le
// 2026-09-08). Personne ne l'avait vu parce que nos créneaux de sauvegarde
// rapide, eux, marchent — tout le monde s'en servait sans se demander
// pourquoi la sauvegarde du jeu ne revenait pas.
//
// Déclarés ici : le libretro.h embarqué est allégé et ne les porte pas.
typedef void  *(*retro_get_memory_data_t)(unsigned id);
typedef size_t (*retro_get_memory_size_t)(unsigned id);

static retro_get_memory_data_t              fp_retro_get_memory_data;
static retro_get_memory_size_t              fp_retro_get_memory_size;
static retro_cheat_reset_t                  fp_retro_cheat_reset;
static retro_cheat_set_t                    fp_retro_cheat_set;

static retro_set_video_refresh_t            fp_retro_set_video_refresh;
static retro_set_audio_sample_t             fp_retro_set_audio_sample;
static retro_set_audio_sample_batch_t       fp_retro_set_audio_sample_batch;
static retro_set_input_poll_t               fp_retro_set_input_poll;
static retro_set_input_state_t              fp_retro_set_input_state;
static retro_set_environment_t              fp_retro_set_environment;

// ============================================================================
// Helper: resolve a symbol from the core
// ============================================================================
static void* resolve_symbol(const char* name) {
    return dlsym(core_handle, name);
}

#define LOAD_SYMBOL(fp, name) \
    fp = (typeof(fp))resolve_symbol(name); \
    if (!fp) { \
        fprintf(stderr, "ERROR: failed to resolve symbol: %s (%s)\n", name, dlerror()); \
        return -1; \
    }

// ============================================================================
// Optional symbols (may not exist in all cores)
// ============================================================================
#define LOAD_SYMBOL_OPTIONAL(fp, name) \
    fp = (typeof(fp))resolve_symbol(name)

// ============================================================================
// Load a Libretro core from the given .so path.
// Returns 0 on success, -1 on failure.
// ============================================================================
static int core_load(const char* path) {
    if (core_handle) {
        fprintf(stderr, "ERROR: a core is already loaded\n");
        return -1;
    }

    core_handle = dlopen(path, RTLD_LAZY | RTLD_LOCAL);
    if (!core_handle) {
        fprintf(stderr, "ERROR: dlopen failed: %s\n", dlerror());
        return -1;
    }

    // Resolve all required symbols
    LOAD_SYMBOL(fp_retro_init,                   "retro_init");
    LOAD_SYMBOL(fp_retro_deinit,                 "retro_deinit");
    LOAD_SYMBOL(fp_retro_api_version,            "retro_api_version");
    LOAD_SYMBOL(fp_retro_get_system_info,        "retro_get_system_info");
    LOAD_SYMBOL(fp_retro_get_system_av_info,     "retro_get_system_av_info");
    LOAD_SYMBOL(fp_retro_set_controller_port_device, "retro_set_controller_port_device");
    LOAD_SYMBOL(fp_retro_reset,                  "retro_reset");
    LOAD_SYMBOL(fp_retro_run,                    "retro_run");
    LOAD_SYMBOL(fp_retro_load_game,              "retro_load_game");
    LOAD_SYMBOL(fp_retro_unload_game,            "retro_unload_game");
    LOAD_SYMBOL(fp_retro_set_video_refresh,      "retro_set_video_refresh");
    LOAD_SYMBOL(fp_retro_set_audio_sample,       "retro_set_audio_sample");
    LOAD_SYMBOL(fp_retro_set_audio_sample_batch, "retro_set_audio_sample_batch");
    LOAD_SYMBOL(fp_retro_set_input_poll,         "retro_set_input_poll");
    LOAD_SYMBOL(fp_retro_set_input_state,        "retro_set_input_state");
    LOAD_SYMBOL(fp_retro_set_environment,        "retro_set_environment");

    // Optional symbols
    LOAD_SYMBOL_OPTIONAL(fp_retro_serialize,      "retro_serialize");
    LOAD_SYMBOL_OPTIONAL(fp_retro_unserialize,    "retro_unserialize");
    LOAD_SYMBOL_OPTIONAL(fp_retro_serialize_size, "retro_serialize_size");
    // Optionnels au même titre : un cœur sans mémoire de sauvegarde (un
    // lecteur de musique, un test) n'a aucune raison de les exporter.
    LOAD_SYMBOL_OPTIONAL(fp_retro_get_memory_data, "retro_get_memory_data");
    LOAD_SYMBOL_OPTIONAL(fp_retro_get_memory_size, "retro_get_memory_size");
    // retro_cheat_reset/retro_cheat_set are technically REQUIRED exports per
    // the libretro spec (every core must define them, even as a no-op) —
    // loaded optional anyway, same defensive stance as serialize above: a
    // core that's missing/misbehaving here shouldn't take down core_load
    // over a feature (cheats) nobody's forced to use.
    LOAD_SYMBOL_OPTIONAL(fp_retro_cheat_reset,    "retro_cheat_reset");
    LOAD_SYMBOL_OPTIONAL(fp_retro_cheat_set,      "retro_cheat_set");

    return 0;
}

// ============================================================================
// Unload the currently loaded core. `close` à 0 : l'oublier SANS le retirer de
// la mémoire — un fil qui l'a exécuté vit encore (engine.unloadAfter).
// ============================================================================
static void core_unload(int close) {
    if (core_handle) {
        if (close) {
            dlclose(core_handle);
        }
        core_handle = NULL;
    }
    fp_retro_init = NULL;
    fp_retro_deinit = NULL;
    fp_retro_api_version = NULL;
    fp_retro_get_system_info = NULL;
    fp_retro_get_system_av_info = NULL;
    fp_retro_set_controller_port_device = NULL;
    fp_retro_reset = NULL;
    fp_retro_run = NULL;
    fp_retro_load_game = NULL;
    fp_retro_unload_game = NULL;
    fp_retro_serialize = NULL;
    fp_retro_unserialize = NULL;
    fp_retro_serialize_size = NULL;
    fp_retro_get_memory_data = NULL;
    fp_retro_get_memory_size = NULL;
    fp_retro_cheat_reset = NULL;
    fp_retro_cheat_set = NULL;
    fp_retro_set_video_refresh = NULL;
    fp_retro_set_audio_sample = NULL;
    fp_retro_set_audio_sample_batch = NULL;
    fp_retro_set_input_poll = NULL;
    fp_retro_set_input_state = NULL;
    fp_retro_set_environment = NULL;
}

// ============================================================================
// Wrappers that call the core's function pointers.
// Safe to call even if core is not loaded (they are no-ops then).
// ============================================================================
// ============================================================================
// LA PILE DE SIGNAUX DU FIL DU MOTEUR (2026-10-14, #356)
//
// Un cœur peut laisser au fil qui l'appelle une pile de signaux (sigaltstack)
// qui n'est plus la sienne. PPSSPP installe son gestionnaire de fautes depuis
// son fil de CHARGEMENT et y note la pile de signaux de ce fil ; il le retire
// depuis le fil qui arrête le jeu — le nôtre — et y remet la pile notée : celle
// d'un fil terminé depuis longtemps, dont la mémoire est rendue. Le signal
// suivant que reçoit notre fil (la préemption de Go, un SIGCHLD…) ne peut plus
// être servi : le noyau tue le processus d'un SIGSEGV, sans gestionnaire, donc
// sans pile ni rapport — Android dit « tuée par un signal, statut 11 ». C'est
// la mort du téléphone en quittant une partie PSP (#317, #328, #356), qui
// tombait au hasard du prochain signal : la plupart des arrêts passaient.
//
// Même famille que lightrec sur le serveur, qui désactivait la pile de son fil
// (« non-Go code disabled sigaltstack », journal du 2026-07).
//
// Le fil du moteur note donc sa pile avant de charger le cœur ; les appels qui
// arrêtent le cœur (retro_unload_game, retro_deinit, retro_reset) se font
// signaux asynchrones retenus, puis la pile notée est remise si le cœur l'a
// changée — avant de rendre la main à Go. Les signaux synchrones (une vraie
// faute) passent toujours. Pendant la partie, rien n'est remis : un cœur peut
// avoir posé sa pile pour son propre gestionnaire ; l'écart est seulement
// mesuré (gf_signal_stack_drift).
// ============================================================================
static __thread stack_t gf_noted_ss;
static __thread int gf_noted = 0;
static __thread stack_t gf_left_ss;   // ce que le cœur avait laissé, au dernier remplacement
static __thread int gf_replaced = 0;

static void gf_note_signal_stack(void) {
    gf_noted = sigaltstack(NULL, &gf_noted_ss) == 0;
}

static int gf_same_stack(const stack_t* a, const stack_t* b) {
    int a_off = (a->ss_flags & SS_DISABLE) != 0;
    int b_off = (b->ss_flags & SS_DISABLE) != 0;
    if (a_off || b_off) return a_off == b_off;
    return a->ss_sp == b->ss_sp && a->ss_size == b->ss_size;
}

// gf_signal_stack_drift — la pile de signaux du fil diffère-t-elle de la
// notée ? Sans rien remettre. `now` reçoit la pile actuelle.
static int gf_signal_stack_drift(stack_t* now) {
    if (!gf_noted || sigaltstack(NULL, now) != 0) return 0;
    return !gf_same_stack(now, &gf_noted_ss);
}

static void gf_restore_signal_stack(void) {
    stack_t now;
    if (!gf_signal_stack_drift(&now)) return;
    stack_t want = gf_noted_ss;
    want.ss_flags &= SS_DISABLE;
    if (sigaltstack(&want, NULL) == 0) {
        gf_left_ss = now;
        gf_replaced = 1;
    }
}

// gf_take_replaced — 1 si un arrêt du cœur a dû remettre la pile notée, avec
// ce que le cœur avait laissé et ce qui a été remis ; remis à zéro par la
// lecture.
static int gf_take_replaced(stack_t* left, stack_t* noted) {
    if (!gf_replaced) return 0;
    *left = gf_left_ss;
    *noted = gf_noted_ss;
    gf_replaced = 0;
    return 1;
}

static void gf_guarded(void (*call)(void)) {
    sigset_t block, previous;
    sigfillset(&block);
    sigdelset(&block, SIGSEGV);
    sigdelset(&block, SIGBUS);
    sigdelset(&block, SIGILL);
    sigdelset(&block, SIGFPE);
    sigdelset(&block, SIGTRAP);
    sigdelset(&block, SIGABRT);
    sigdelset(&block, SIGSYS);
    pthread_sigmask(SIG_BLOCK, &block, &previous);
    call();
    gf_restore_signal_stack();
    pthread_sigmask(SIG_SETMASK, &previous, NULL);
}

// ============================================================================
// LA FIN DU FIL DU MOTEUR, COMME CELLE DE N'IMPORTE QUEL FIL (2026-10-15, #356)
//
// Go termine le fil d'une goroutine qui finit verrouillée, et avant de le
// rendre à la libc il y bloque TOUS les signaux (mexit → sigblock(true)) —
// SIGSEGV compris. La libc exécute ensuite, sur ce fil, les destructeurs que le
// code y a inscrits : variables `thread_local` du C++, clés de pthread — ceux
// du cœur, et ceux des pilotes que le cœur a fait travailler sur ce fil
// (PPSSPP sonde Vulkan dans son retro_init). Une faute dans l'un d'eux, même
// une que son auteur attend et rattrape, tue alors le processus sans appeler
// personne : ni rapport, ni tombstone — Android ne dit que « tuée par un
// signal, statut 11 ». C'était la mort du S25 en quittant une partie PSP une
// fois la pile de signaux remise : entre « partie arrêtée » et la fin du fil,
// sans rien d'autre ; jamais avec LibretroDroid, dont le fil du cœur était un
// fil Java, qui se termine signaux ouverts.
//
// Le fil du moteur inscrit donc, après son dernier appel au cœur, un
// destructeur à lui : la libc les exécute du dernier inscrit au premier, et
// les clés de pthread après eux — il passe avant tous. Il rouvre les signaux
// synchrones, ceux d'une faute ; les autres restent fermés, le fil n'est plus
// à Go. Il note ce qu'il a trouvé (SIGSEGV fermé ?) et, si une trace est
// demandée, l'écrit : un processus qui meurt après elle est mort pendant les
// destructeurs, signaux ouverts — le rapport natif doit alors exister.
// ============================================================================
extern int __cxa_thread_atexit_impl(void (*func)(void*), void* arg, void* dso_handle);

static char gf_exit_trace_path[512];
static volatile int gf_exit_found_faults_blocked = -1;  // -1 : aucune fin vue

static void gf_open_faults_at_exit(void* unused) {
    (void)unused;
    sigset_t faults, now;
    sigemptyset(&faults);
    sigaddset(&faults, SIGSEGV);
    sigaddset(&faults, SIGBUS);
    sigaddset(&faults, SIGILL);
    sigaddset(&faults, SIGFPE);
    sigaddset(&faults, SIGTRAP);
    sigaddset(&faults, SIGABRT);
    sigaddset(&faults, SIGSYS);
    if (pthread_sigmask(SIG_UNBLOCK, &faults, &now) == 0) {
        gf_exit_found_faults_blocked = sigismember(&now, SIGSEGV) == 1;
    }
    if (gf_exit_trace_path[0] != 0) {
        int fd = open(gf_exit_trace_path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0644);
        if (fd >= 0) {
            static const char line[] = "fin du fil du moteur : ses destructeurs commencent, signaux synchrones rouverts\n";
            ssize_t written = write(fd, line, sizeof line - 1);
            (void)written;
            close(fd);
        }
    }
}

// Le troisième argument désigne la bibliothèque du destructeur : la libc la
// cherche par cette adresse (glibc la déréférence, bionic s'arrête si elle
// n'est dans aucune). Celle du destructeur lui-même, donc.
static int gf_unblock_faults_at_thread_exit(void) {
    return __cxa_thread_atexit_impl(gf_open_faults_at_exit, NULL, (void*)&gf_open_faults_at_exit);
}

static void gf_set_exit_trace(const char* path) {
    strncpy(gf_exit_trace_path, path, sizeof gf_exit_trace_path - 1);
    gf_exit_trace_path[sizeof gf_exit_trace_path - 1] = 0;
}

static int gf_take_exit_found_faults_blocked(void) {
    int found = gf_exit_found_faults_blocked;
    gf_exit_found_faults_blocked = -1;
    return found;
}

static void wrap_retro_init(void) {
    fprintf(stderr, "[libretro] retro_init()\n");
    if (fp_retro_init) fp_retro_init();
    fprintf(stderr, "[libretro] retro_init() done\n");
}

static void call_retro_deinit(void) {
    if (fp_retro_deinit) fp_retro_deinit();
}

static void wrap_retro_deinit(void) {
    gf_guarded(call_retro_deinit);
}

static void wrap_retro_get_system_info(struct retro_system_info* info) {
    if (fp_retro_get_system_info) fp_retro_get_system_info(info);
}

static void wrap_retro_get_system_av_info(struct retro_system_av_info* info) {
    if (fp_retro_get_system_av_info) fp_retro_get_system_av_info(info);
}

static void wrap_retro_set_controller_port_device(unsigned port, unsigned device) {
    if (fp_retro_set_controller_port_device) fp_retro_set_controller_port_device(port, device);
}

static void wrap_retro_run(void) {
    if (fp_retro_run) fp_retro_run();
}

static void call_retro_reset(void) {
    if (fp_retro_reset) fp_retro_reset();
}

static void wrap_retro_reset(void) {
    gf_guarded(call_retro_reset);
}

static bool wrap_retro_load_game(const struct retro_game_info* game) {
    if (fp_retro_load_game) {
        // Debug: verify file can be opened
        if (game->path) {
            FILE* f = fopen(game->path, "rb");
            if (!f) {
                fprintf(stderr, "[libretro] ERROR: cannot open file: %s (errno=%d)\n", game->path, errno);
            } else {
                fseek(f, 0, SEEK_END);
                long sz = ftell(f);
                fclose(f);
                fprintf(stderr, "[libretro] retro_load_game(path=%s, data=%p, size=%zu) file_on_disk=%ld\n",
                    game->path, game->data, game->size, sz);
            }
        } else {
            fprintf(stderr, "[libretro] retro_load_game(path=nil, data=%p, size=%zu)\n",
                game->data, game->size);
        }
        bool ok = fp_retro_load_game(game);
        fprintf(stderr, "[libretro] retro_load_game returned %s\n", ok ? "true" : "false");
        return ok;
    }
    return false;
}

static void call_retro_unload_game(void) {
    if (fp_retro_unload_game) fp_retro_unload_game();
}

static void wrap_retro_unload_game(void) {
    gf_guarded(call_retro_unload_game);
}

static bool wrap_retro_serialize(void* data, size_t size) {
    if (fp_retro_serialize) return fp_retro_serialize(data, size);
    return false;
}

static bool wrap_retro_unserialize(const void* data, size_t size) {
    if (fp_retro_unserialize) return fp_retro_unserialize(data, size);
    return false;
}

static size_t wrap_retro_serialize_size(void) {
    if (fp_retro_serialize_size) return fp_retro_serialize_size();
    return 0;
}

static void *wrap_retro_get_memory_data(unsigned id) {
    if (fp_retro_get_memory_data) return fp_retro_get_memory_data(id);
    return NULL;
}

static size_t wrap_retro_get_memory_size(unsigned id) {
    if (fp_retro_get_memory_size) return fp_retro_get_memory_size(id);
    return 0;
}

// Go-side "is this optional function pointer set" checks go through a
// wrapper like this one instead of comparing the static C.fp_retro_xxx
// variable to nil directly from Go — confirmed necessary: Serialize/
// Unserialize's direct `C.fp_retro_serialize == nil` checks (never actually
// exercised before nothing called them) failed at LINK time with
// "undefined reference to fp_retro_serialize", while every other fp_retro_*
// var is only ever read from within a wrap_* C function like this one,
// which always linked fine.
static bool core_supports_serialize(void) { return fp_retro_serialize != NULL; }
static bool core_supports_unserialize(void) { return fp_retro_unserialize != NULL; }
static bool core_supports_cheats(void) { return fp_retro_cheat_set != NULL; }

static void wrap_retro_cheat_reset(void) {
    if (fp_retro_cheat_reset) fp_retro_cheat_reset();
}

static void wrap_retro_cheat_set(unsigned index, bool enabled, const char* code) {
    if (fp_retro_cheat_set) fp_retro_cheat_set(index, enabled, code);
}

// ============================================================================
// Hardware rendering (RETRO_ENVIRONMENT_SET_HW_RENDER) — see
// ../gl_context/gl_context.go's own doc for the headless EGL/GBM context this
// wires up to. g_hw_render points at the CORE's own struct (memory it owns,
// typically static storage inside the core — we never allocate or free it),
// captured so context_reset/context_destroy can be invoked later from Go,
// once the actual GL context exists.
// ============================================================================
static struct retro_hw_render_callback *g_hw_render = NULL;

extern uintptr_t goHWGetCurrentFramebuffer(void);
extern void* goHWGetProcAddress(char *sym);

// hw_get_current_framebuffer/hw_get_proc_address are what the CORE calls,
// via the function pointers wrap_set_hw_render writes into its own struct
// below — every GL function a hardware-rendered core uses internally is
// resolved through get_proc_address, same as this backend's OWN GL calls
// (see gl_context.c's own doc on why neither links against libGL directly).
static uintptr_t hw_get_current_framebuffer(void) {
    return goHWGetCurrentFramebuffer();
}
static retro_proc_address_t hw_get_proc_address(const char *sym) {
    // Cast away const — goHWGetProcAddress only reads the string (copied
    // into a Go string via C.GoString), never modifies it.
    return (retro_proc_address_t)goHWGetProcAddress((char *)sym);
}

static void wrap_set_hw_render(struct retro_hw_render_callback *hw) {
    g_hw_render = hw;
    hw->get_current_framebuffer = hw_get_current_framebuffer;
    hw->get_proc_address = hw_get_proc_address;
}

static int hw_render_context_type(struct retro_hw_render_callback *hw) { return (int)hw->context_type; }
static unsigned hw_render_version_major(struct retro_hw_render_callback *hw) { return hw->version_major; }
static unsigned hw_render_version_minor(struct retro_hw_render_callback *hw) { return hw->version_minor; }

// wrap_hw_context_reset/destroy call the CORE's own callbacks (captured in
// g_hw_render by wrap_set_hw_render above) — context_reset once the real GL
// context is current and ready (the core does its one-time GL resource
// setup inside this call, resolving its own GL functions via
// get_proc_address), context_destroy once on session shutdown, before the
// GL context itself is torn down.
static void wrap_hw_context_reset(void) {
    if (g_hw_render && g_hw_render->context_reset) g_hw_render->context_reset();
}
static void wrap_hw_context_destroy(void) {
    if (g_hw_render && g_hw_render->context_destroy) g_hw_render->context_destroy();
    g_hw_render = NULL;
}

static void wrap_retro_set_video_refresh(retro_video_refresh_t cb) {
    if (fp_retro_set_video_refresh) fp_retro_set_video_refresh(cb);
}

static void wrap_retro_set_audio_sample(retro_audio_sample_t cb) {
    if (fp_retro_set_audio_sample) fp_retro_set_audio_sample(cb);
}

static void wrap_retro_set_audio_sample_batch(retro_audio_sample_batch_t cb) {
    if (fp_retro_set_audio_sample_batch) fp_retro_set_audio_sample_batch(cb);
}

static void wrap_retro_set_input_poll(retro_input_poll_t cb) {
    if (fp_retro_set_input_poll) fp_retro_set_input_poll(cb);
}

static void wrap_retro_set_input_state(retro_input_state_t cb) {
    if (fp_retro_set_input_state) fp_retro_set_input_state(cb);
}

static void wrap_retro_set_environment(retro_environment_t cb) {
    if (fp_retro_set_environment) fp_retro_set_environment(cb);
}

// ============================================================================
// Forward declarations of Go-exported callback functions.
// These are resolved at link time — no need to pass them as arguments.
// ============================================================================
extern void goVideoRefresh(void*, unsigned, unsigned, size_t);
extern void goAudioSample(int16_t, int16_t);
extern size_t goAudioSampleBatch(void*, size_t);
extern void goInputPoll(void);
extern int16_t goInputState(unsigned, unsigned, unsigned, unsigned);
extern bool goEnvironment(unsigned, void*);

// Register all Go callbacks with the loaded Libretro core in one shot.
// Called from Go AFTER all individual callback variables have been set.
static void register_all_callbacks(void) {
    fprintf(stderr, "[libretro] register_all_callbacks: video=%p audio=%p input_poll=%p input_state=%p env=%p\n",
        (void*)fp_retro_set_video_refresh, (void*)fp_retro_set_audio_sample_batch,
        (void*)fp_retro_set_input_poll, (void*)fp_retro_set_input_state, (void*)fp_retro_set_environment);
    if (fp_retro_set_video_refresh)
        fp_retro_set_video_refresh((retro_video_refresh_t)goVideoRefresh);
    if (fp_retro_set_audio_sample_batch)
        fp_retro_set_audio_sample_batch((retro_audio_sample_batch_t)goAudioSampleBatch);
    // L'ÉCHANTILLON SEUL AUSSI. Il n'était jamais donné au cœur : un cœur qui
    // rend son son un échantillon à la fois appelait un pointeur nul, et
    // s'abattait. Aucun de ceux du serveur ne le fait ; LibretroDroid, que ce
    // pont remplacera sur le téléphone, donne les deux (chantier 13).
    if (fp_retro_set_audio_sample)
        fp_retro_set_audio_sample((retro_audio_sample_t)goAudioSample);
    if (fp_retro_set_input_poll)
        fp_retro_set_input_poll((retro_input_poll_t)goInputPoll);
    if (fp_retro_set_input_state)
        fp_retro_set_input_state((retro_input_state_t)goInputState);
    if (fp_retro_set_environment)
        fp_retro_set_environment((retro_environment_t)goEnvironment);
    fprintf(stderr, "[libretro] register_all_callbacks: done\n");
}

// ============================================================================
// Environment paths (system dir, save dir) — allocated once at startup.
// ============================================================================
static char* env_system_dir = NULL;
static char* env_save_dir   = NULL;

// systemDir/saveDir are caller-supplied (BIOS_PATH/SAVES_PATH) since both
// must point at persistent, host-mounted folders — systemDir for BIOS files
// some cores need (e.g. PS1's scph1001.bin), saveDir for in-game saves
// (battery/SRAM saves the core writes on its own, via GET_SAVE_DIRECTORY —
// distinct from Libretro save-STATES, which aren't wired up at all yet, see
// webrtcso STATUS.md "Pas encore implémenté"). Neither can be an ephemeral
// /tmp path, or both are silently lost on every container restart.
static void init_env_paths(const char* systemDir, const char* saveDir) {
    mkdir(systemDir, 0755);
    mkdir(saveDir, 0755);
    // Le téléphone enchaîne les parties dans LE MÊME processus (chantier 13) :
    // les chemins de la précédente se libèrent. Son cœur est déchargé, plus
    // personne ne les tient.
    free(env_system_dir);
    free(env_save_dir);
    env_system_dir = strdup(systemDir);
    env_save_dir   = strdup(saveDir);
}

static void* get_system_dir(void) { return env_system_dir; }
static void* get_save_dir(void)   { return env_save_dir; }

// ============================================================================
// Log callback (RETRO_ENVIRONMENT_GET_LOG_INTERFACE) — some cores (e.g.
// PPSSPP's retro_init) skip their own internal init entirely when this
// environment call fails, then unconditionally use the log manager anyway,
// crashing on a null function pointer. Returning a real callback here (not
// just `false`) avoids that whole code path.
// ============================================================================
#include <stdarg.h>

static void core_log_printf(enum retro_log_level level, const char* fmt, ...) {
    va_list args;
    va_start(args, fmt);
    fprintf(stderr, "[core-log] ");
    vfprintf(stderr, fmt, args);
    va_end(args);
}

static void* get_core_log_cb(void) { return (void*)core_log_printf; }

// ============================================================================
// Rumble callback (RETRO_ENVIRONMENT_GET_RUMBLE_INTERFACE) — the core calls
// this whenever it wants to change a controller's motor state (separate
// calls for the STRONG/low-frequency and WEAK/high-frequency motors, per
// libretro's retro_rumble_effect enum: 0=STRONG, 1=WEAK). Forwarded straight
// into Go (goSetRumbleState) rather than handled here, since the actual
// destination is a specific player's WebSocket connection — routing that
// belongs in Go (see stream_session.go's environmentCallback), same
// division of labor as goHWGetCurrentFramebuffer/goHWGetProcAddress above.
// ============================================================================
extern bool goSetRumbleState(unsigned port, int effect, uint16_t strength);

static bool wrap_set_rumble_state(unsigned port, enum retro_rumble_effect effect, uint16_t strength) {
    return goSetRumbleState(port, (int)effect, strength);
}

static void* get_rumble_cb(void) { return (void*)wrap_set_rumble_state; }

// ============================================================================
// Disk control (RETRO_ENVIRONMENT_SET_DISK_CONTROL_INTERFACE) — a multi-disc
// game (a .m3u and its .cue files) hands the frontend seven function
// pointers and expects to be TOLD which image to mount. Nothing about it is
// automatic: reaching "insert disc 2" with no frontend that speaks this is
// the end of the game.
//
// The struct is COPIED, not kept by pointer: libretro's own doc says the
// data a core passes to an environment call is only guaranteed valid for the
// duration of that call. Keeping the pointer worked right up until a core
// put it on its stack.
//
// Every entry is null-checked on the way out, not just on the way in: the
// interface is a struct of seven optional pointers, and a core that only
// implements five is conforming, not broken.
// ============================================================================
static struct retro_disk_control_callback disk_control;
static int disk_control_ready = 0;

static void store_disk_control(const struct retro_disk_control_callback* incoming) {
    if (!incoming) {
        return;
    }
    disk_control = *incoming;
    disk_control_ready = 1;
}

// forget_core_state oublie ce qu'un cœur a CONFIÉ au pont et qui pointe dans
// sa mémoire : son interface de disques, sa structure de rendu matériel. Après
// dlclose, ce sont des pointeurs dans le vide. Sur le serveur, une partie par
// processus, personne ne les relisait ; sur le téléphone, qui enchaîne les
// parties, le DiskCount() de la suivante aurait appelé le cœur d'avant.
static void forget_core_state(void) {
    memset(&disk_control, 0, sizeof(disk_control));
    disk_control_ready = 0;
    g_hw_render = NULL;
}

static int has_disk_control(void) {
    return disk_control_ready && disk_control.get_num_images != NULL;
}

static int disk_image_count(void) {
    if (!has_disk_control()) {
        return 0;
    }
    return (int)disk_control.get_num_images();
}

static int disk_image_index(void) {
    if (!disk_control_ready || !disk_control.get_image_index) {
        return -1;
    }
    return (int)disk_control.get_image_index();
}

// La séquence complète, et elle n'est pas facultative : un core ne regarde
// l'index que pendant que le lecteur est OUVERT. Poser set_image_index seul
// ne fait rien du tout — le disque suivant n'est jamais monté.
//
// Si le choix échoue, on referme quand même : laisser le lecteur ouvert
// laisserait le jeu bloqué sur son écran « insérez le disque », ce qui est
// pire que de n'avoir rien tenté.
static int disk_select(unsigned index) {
    if (!disk_control_ready || !disk_control.set_eject_state || !disk_control.set_image_index) {
        return 0;
    }
    if (!disk_control.set_eject_state(true)) {
        return 0;
    }
    if (!disk_control.set_image_index(index)) {
        disk_control.set_eject_state(false);
        return 0;
    }
    return disk_control.set_eject_state(false) ? 1 : 0;
}
*/
import "C"
import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"gameflex/libretro/gl_context"
)

// ---------------------------------------------------------------------------
// Go-side types (mirrors of C structs — must match layout)
// ---------------------------------------------------------------------------

// SystemInfo describes the loaded Libretro core.
type SystemInfo struct {
	LibraryName     string
	LibraryVersion  string
	ValidExtensions string
	NeedFullpath    bool
	BlockExtract    bool
}

// GameGeometry describes the video frame dimensions.
type GameGeometry struct {
	BaseWidth   uint32
	BaseHeight  uint32
	MaxWidth    uint32
	MaxHeight   uint32
	AspectRatio float32
}

// SystemTiming describes the target FPS and audio sample rate.
type SystemTiming struct {
	FPS        float64
	SampleRate float64
}

// GeometryAt lit la géométrie qu'un cœur passe à SET_GEOMETRY
// (`const struct retro_game_geometry *`).
func GeometryAt(data unsafe.Pointer) GameGeometry {
	geometry := (*C.struct_retro_game_geometry)(data)

	return GameGeometry{
		BaseWidth:   uint32(geometry.base_width),
		BaseHeight:  uint32(geometry.base_height),
		MaxWidth:    uint32(geometry.max_width),
		MaxHeight:   uint32(geometry.max_height),
		AspectRatio: float32(geometry.aspect_ratio),
	}
}

// SystemAVInfo combines geometry and timing.
type SystemAVInfo struct {
	Geometry GameGeometry
	Timing   SystemTiming
}

// GameInfo describes the ROM to load.
type GameInfo struct {
	Path string
	Data []byte
	Meta string
}

// InputStateCallback is called by the core to poll controller state.
// It receives (port, device, index, buttonID) and returns the button value.
type InputStateCallback func(port, device, index, buttonID uint32) int16

// VideoRefreshCallback is called by the core each frame with pixel data.
type VideoRefreshCallback func(data unsafe.Pointer, width, height uint32, pitch uint32)

// AudioSampleCallback is called for each stereo sample pair.
type AudioSampleCallback func(left, right int16)

// AudioSampleBatchCallback is called with batches of audio samples.
type AudioSampleBatchCallback func(data unsafe.Pointer, frames uint32) uint32

// EnvironmentCallback handles environment commands from the core.
type EnvironmentCallback func(cmd uint32, data unsafe.Pointer) bool

// RumbleStateCallback is called by the core (via RETRO_ENVIRONMENT_GET_
// RUMBLE_INTERFACE's set_rumble_state) to change a controller motor's
// intensity — effect is libretro's retro_rumble_effect (0=STRONG/low-
// frequency motor, 1=WEAK/high-frequency motor), strength is 0-65535.
// Two independent calls arrive per "rumble event" (one per motor), not one
// combined call — the caller is responsible for tracking per-port state if
// it wants to report both motors together.
type RumbleStateCallback func(port uint32, effect int32, strength uint16) bool

// ---------------------------------------------------------------------------
// Global Go callback storage (referenced from C callbacks via cgo)
// ---------------------------------------------------------------------------
var (
	onVideoRefresh     VideoRefreshCallback
	onAudioSample      AudioSampleCallback
	onAudioSampleBatch AudioSampleBatchCallback
	onInputPoll        func()
	onInputState       InputStateCallback
	onEnvironment      EnvironmentCallback
	onRumbleState      RumbleStateCallback
)

// ---------------------------------------------------------------------------
// Exported cgo callback trampolines (called from C)
// ---------------------------------------------------------------------------

//export goVideoRefresh
func goVideoRefresh(data unsafe.Pointer, width C.unsigned, height C.unsigned, pitch C.size_t) {
	if onVideoRefresh != nil {
		onVideoRefresh(data, uint32(width), uint32(height), uint32(pitch))
	}
}

//export goAudioSample
func goAudioSample(left C.int16_t, right C.int16_t) {
	if onAudioSample != nil {
		onAudioSample(int16(left), int16(right))
	}
}

//export goAudioSampleBatch
func goAudioSampleBatch(data unsafe.Pointer, frames C.size_t) C.size_t {
	if onAudioSampleBatch != nil {
		return C.size_t(onAudioSampleBatch(data, uint32(frames)))
	}
	return 0
}

//export goInputPoll
func goInputPoll() {
	if onInputPoll != nil {
		onInputPoll()
	}
}

//export goInputState
func goInputState(port C.unsigned, device C.unsigned, index C.unsigned, id C.unsigned) C.int16_t {
	if onInputState != nil {
		return C.int16_t(onInputState(uint32(port), uint32(device), uint32(index), uint32(id)))
	}
	return 0
}

//export goEnvironment
func goEnvironment(cmd C.unsigned, data unsafe.Pointer) C.bool {
	if onEnvironment != nil {
		return C.bool(onEnvironment(uint32(cmd), data))
	}
	// Fallback default handler (when no Go callback is set)
	switch uint32(cmd) {
	case 1: // SET_ROTATION
		return true
	case 2: // GET_OVERSCAN
		return false
	case 3: // GET_CAN_DUPE
		return true
	case 10: // SET_PIXEL_FORMAT
		format := *(*uint32)(data)
		return format == 1
	case 15: // GET_VARIABLE
		return false
	case 18: // SET_SUPPORT_NO_GAME
		return true
	default:
		return false
	}
}

//export goSetRumbleState
func goSetRumbleState(port C.unsigned, effect C.int, strength C.uint16_t) C.bool {
	if onRumbleState != nil {
		return C.bool(onRumbleState(uint32(port), int32(effect), uint16(strength)))
	}
	return C.bool(false)
}

// activeGLContext is the ONE hardware-render GL context for this process —
// same "one core loaded per process" assumption the whole rest of this
// bridge already makes (see LoadCore's own doc), extended to hardware
// rendering: a process only ever runs one retro-gaming session (see
// supervisor.go's per-session re-exec), so a package-level global here
// carries no cross-session interference risk.
var activeGLContext *gl_context.Context

//export goHWGetCurrentFramebuffer
func goHWGetCurrentFramebuffer() C.uintptr_t {
	if activeGLContext == nil {
		return 0
	}
	return C.uintptr_t(activeGLContext.FramebufferID())
}

//export goHWGetProcAddress
func goHWGetProcAddress(sym *C.char) unsafe.Pointer {
	return gl_context.GetProcAddress(C.GoString(sym))
}

// ---------------------------------------------------------------------------
// Hardware rendering (RETRO_ENVIRONMENT_SET_HW_RENDER) — public API
// ---------------------------------------------------------------------------

// HWRenderRequest is what a core asked for via SET_HW_RENDER, translated out
// of struct retro_hw_render_callback (data) into plain Go — the caller
// (stream_session.go's environmentCallback) decides whether to accept
// (create a real GL context of this type) using this, without needing any
// C struct access of its own.
type HWRenderRequest struct {
	// ContextType mirrors enum retro_hw_context_type's numeric values —
	// compare against gl_context.ContextTypeOpenGL etc. (kept as plain ints
	// on both sides rather than a shared type, so gl_context stays free of
	// any dependency on this package's cgo types).
	ContextType                int
	VersionMajor, VersionMinor uint32
}

// HandleSetHWRender reads the core's struct retro_hw_render_callback and
// wires OUR get_current_framebuffer/get_proc_address into it — called
// REGARDLESS of whether the caller ends up accepting the request, since
// filling in those two fields is harmless if the core is never actually
// told to proceed (environmentCallback returning false for an unsupported
// ContextType means the core won't call context_reset at all, so they're
// simply never invoked).
func HandleSetHWRender(data unsafe.Pointer) HWRenderRequest {
	hw := (*C.struct_retro_hw_render_callback)(data)
	C.wrap_set_hw_render(hw)
	return HWRenderRequest{
		ContextType:  int(C.hw_render_context_type(hw)),
		VersionMajor: uint32(C.hw_render_version_major(hw)),
		VersionMinor: uint32(C.hw_render_version_minor(hw)),
	}
}

// SetActiveGLContext stores the live GL context goHWGetCurrentFramebuffer
// reports to the core — call once the context has been created (and, if
// the core requests a specific geometry, resized) but BEFORE
// NotifyHWContextReset, so the core's own context_reset (which typically
// calls get_current_framebuffer immediately) sees a real value.
func SetActiveGLContext(ctx *gl_context.Context) {
	activeGLContext = ctx
}

// NotifyHWContextReset calls the core's own context_reset callback — the
// core does its one-time GL resource setup inside this call, resolving its
// own GL functions via get_proc_address. Must be called after
// SetActiveGLContext, exactly once per session, before the first Run().
func NotifyHWContextReset() {
	C.wrap_hw_context_reset()
}

// NotifyHWContextDestroy calls the core's own context_destroy callback (a
// no-op if it never set one) — call once on session shutdown, before the GL
// context itself is destroyed.
func NotifyHWContextDestroy() {
	C.wrap_hw_context_destroy()
	activeGLContext = nil
}

// IsHWFrameBufferValid reports whether a videoCallback `data` argument is
// the RETRO_HW_FRAME_BUFFER_VALID sentinel — a hardware-rendered core
// passes this instead of a real pixel buffer pointer, meaning "the frame is
// already in whatever FBO get_current_framebuffer last reported, go read it
// yourself" (see gl_context.Context.ReadPixels).
func IsHWFrameBufferValid(data unsafe.Pointer) bool {
	return data == C.RETRO_HW_FRAME_BUFFER_VALID
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// LoadCore loads a Libretro core from the given .so file path.
func LoadCore(path string) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	if C.core_load(cPath) != 0 {
		return fmt.Errorf("failed to load core: %s", path)
	}
	return nil
}

// UnloadCore unloads the currently loaded core.
func UnloadCore() {
	C.core_unload(1)
	C.forget_core_state()
}

// ForgetCore — le cœur oublié mais laissé en mémoire (pas de dlclose) : quand
// un fil qui a exécuté son code vit encore, le décharger le tuerait au premier
// retour dans ce code. Un autre cœur peut ensuite être chargé.
func ForgetCore() {
	C.core_unload(0)
	C.forget_core_state()
}

// NoteSignalStack — sur le fil du moteur, AVANT de charger le cœur : la pile
// de signaux que ce fil doit garder. Les arrêts du cœur (UnloadGame, Deinit,
// Reset) la remettent si le cœur l'a changée (voir gf_guarded).
func NoteSignalStack() {
	C.gf_note_signal_stack()
}

// SignalStackReplaced — si le dernier arrêt du cœur avait laissé au fil une
// autre pile de signaux : ce qu'il avait laissé, et la pile remise. "" sinon.
// Sur le fil du moteur, comme les arrêts.
func SignalStackReplaced() string {
	var left, noted C.stack_t
	if C.gf_take_replaced(&left, &noted) == 0 {
		return ""
	}

	return describeSignalStack(left) + " ; celle du fil remise : " + describeSignalStack(noted)
}

// SignalStackDrift — la pile de signaux du fil diffère-t-elle, maintenant, de
// celle notée avant le cœur ? "" si non. Rien n'est remis.
func SignalStackDrift() string {
	var now C.stack_t
	if C.gf_signal_stack_drift(&now) == 0 {
		return ""
	}

	return describeSignalStack(now)
}

// UnblockFaultsAtThreadExit — sur le fil du moteur, après son dernier appel au
// cœur : quand Go terminera ce fil, ses destructeurs (ceux du cœur, ceux des
// pilotes) tourneront signaux synchrones ouverts, comme sur n'importe quel
// fil. Voir gf_open_faults_at_exit. Faux si la libc a refusé l'inscription.
func UnblockFaultsAtThreadExit() bool {
	return C.gf_unblock_faults_at_thread_exit() == 0
}

// TraceThreadExits — le fichier où la fin d'un fil du moteur écrit qu'elle a
// commencé ; "" n'écrit rien. Le moteur l'efface une fois le fil fini
// (ClearThreadExitTrace) : resté sur le disque, il dit qu'un processus est mort
// pendant ces destructeurs.
func TraceThreadExits(path string) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	C.gf_set_exit_trace(cPath)
	exitTracePath.Store(path)
}

var exitTracePath atomic.Value

// ClearThreadExitTrace — le fil du moteur a fini : sa trace n'a plus rien à
// dire.
func ClearThreadExitTrace() {
	if path, _ := exitTracePath.Load().(string); path != "" {
		_ = os.Remove(path)
	}
}

// ThreadExitFoundFaultsBlocked — à la dernière fin d'un fil du moteur, SIGSEGV
// était-il fermé quand ses destructeurs ont commencé : "oui", "non", ou "" si
// aucune fin n'a été vue depuis la dernière lecture.
func ThreadExitFoundFaultsBlocked() string {
	switch C.gf_take_exit_found_faults_blocked() {
	case 1:
		return "oui"
	case 0:
		return "non"
	}

	return ""
}

// describeSignalStack — « désactivée », ou l'adresse, la taille, et si cette
// mémoire est encore celle du processus (/proc/self/maps) : la pile d'un fil
// terminé ne l'est plus.
func describeSignalStack(stack C.stack_t) string {
	if stack.ss_flags&C.SS_DISABLE != 0 {
		return "désactivée"
	}
	start := uintptr(unsafe.Pointer(stack.ss_sp))

	return fmt.Sprintf("%#x (%d Kio, %s)", start, uint64(stack.ss_size)/1024, mappedState(start))
}

// mappedState — « mappée » si l'adresse est dans une région de
// /proc/self/maps, « NON MAPPÉE » sinon ; « ? » si le fichier ne se lit pas.
func mappedState(address uintptr) string {
	maps, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return "?"
	}
	for _, line := range strings.Split(string(maps), "\n") {
		bounds, _, _ := strings.Cut(line, " ")
		low, high, found := strings.Cut(bounds, "-")
		if !found {
			continue
		}
		from, errLow := strconv.ParseUint(low, 16, 64)
		to, errHigh := strconv.ParseUint(high, 16, 64)
		if errLow == nil && errHigh == nil && uint64(address) >= from && uint64(address) < to {
			return "mappée"
		}
	}

	return "NON MAPPÉE"
}

// Init initializes the core. Must be called after LoadCore and after setting
// the environment callback.
func Init() {
	C.wrap_retro_init()
}

// Deinit deinitializes the core.
func Deinit() {
	C.wrap_retro_deinit()
}

// GetSystemInfo returns information about the loaded core.
func GetSystemInfo() SystemInfo {
	var info C.struct_retro_system_info
	C.wrap_retro_get_system_info(&info)
	return SystemInfo{
		LibraryName:     C.GoString(info.library_name),
		LibraryVersion:  C.GoString(info.library_version),
		ValidExtensions: C.GoString(info.valid_extensions),
		NeedFullpath:    bool(info.need_fullpath),
		BlockExtract:    bool(info.block_extract),
	}
}

// GetSystemAVInfo returns the audio/video configuration for the loaded game.
func GetSystemAVInfo() SystemAVInfo {
	var avInfo C.struct_retro_system_av_info
	C.wrap_retro_get_system_av_info(&avInfo)
	return SystemAVInfo{
		Geometry: GameGeometry{
			BaseWidth:   uint32(avInfo.geometry.base_width),
			BaseHeight:  uint32(avInfo.geometry.base_height),
			MaxWidth:    uint32(avInfo.geometry.max_width),
			MaxHeight:   uint32(avInfo.geometry.max_height),
			AspectRatio: float32(avInfo.geometry.aspect_ratio),
		},
		Timing: SystemTiming{
			FPS:        float64(avInfo.timing.fps),
			SampleRate: float64(avInfo.timing.sample_rate),
		},
	}
}

// SetControllerPortDevice configures the controller type for a given port.
// device should be RETRO_DEVICE_JOYPAD (1) for standard gamepads.
func SetControllerPortDevice(port, device uint32) {
	C.wrap_retro_set_controller_port_device(C.unsigned(port), C.unsigned(device))
}

// Run advances the emulation by one frame. Call this in a loop.
func Run() {
	C.wrap_retro_run()
}

// Reset remet la console à zéro, comme son bouton (retro_reset). Même
// discipline que Run : depuis le fil d'émulation, entre deux images.
func Reset() {
	C.wrap_retro_reset()
}

// gameData holds the C-allocated copy of ROM data, freed in UnloadGame.
var gameData unsafe.Pointer

// LoadGame loads a ROM into the core.
//
// The Libretro API allows cores to receive ROM data either via a file path
// (path field) or via a raw memory buffer (data+size fields). Some cores
// (like fceumm/NES) only use the path, others (like Gambatte/GB) only use
// data+size. To support all cores, we ALWAYS pass both when available.
//
// The ROM data is copied into C-allocated memory (Go 1.21+ cgo safe).
func LoadGame(info GameInfo) error {
	var cInfo C.struct_retro_game_info

	if info.Path == "" && len(info.Data) == 0 {
		return errors.New("no ROM path or data provided")
	}

	// Always set path when available (some cores need it for save files, etc.)
	if info.Path != "" {
		cInfo.path = C.CString(info.Path)
		defer C.free(unsafe.Pointer(cInfo.path))
	}

	// Always set data+size when available (some cores ONLY read from memory)
	if len(info.Data) > 0 {
		gameData = C.malloc(C.size_t(len(info.Data)))
		if gameData == nil {
			return errors.New("malloc failed for ROM data")
		}
		copy(unsafe.Slice((*byte)(gameData), len(info.Data)), info.Data)
		cInfo.data = gameData
		cInfo.size = C.size_t(len(info.Data))
	}

	if !C.wrap_retro_load_game(&cInfo) {
		if gameData != nil {
			C.free(gameData)
			gameData = nil
		}
		return errors.New("retro_load_game returned false")
	}
	return nil
}

// UnloadGame unloads the currently loaded game and frees C-allocated ROM data.
func UnloadGame() {
	C.wrap_retro_unload_game()
	if gameData != nil {
		C.free(gameData)
		gameData = nil
	}
}

// Serialize saves the current state into a buffer. Returns nil if unsupported.
func Serialize() ([]byte, error) {
	if !bool(C.core_supports_serialize()) {
		return nil, errors.New("serialize not supported by this core")
	}
	size := C.wrap_retro_serialize_size()
	if size == 0 {
		return nil, errors.New("serialize size is 0")
	}
	buf := make([]byte, size)
	if !C.wrap_retro_serialize(unsafe.Pointer(&buf[0]), C.size_t(size)) {
		return nil, errors.New("serialize failed")
	}
	return buf, nil
}

// ErrUnserializeFailed — the core itself refused the state (retro_unserialize
// answered false), as opposed to a core that cannot load states at all. The
// engine tells the player why (engine.StateRefusedError, #396).
var ErrUnserializeFailed = errors.New("unserialize failed")

// Unserialize restores a previously saved state.
func Unserialize(data []byte) error {
	if !bool(C.core_supports_unserialize()) {
		return errors.New("unserialize not supported by this core")
	}
	if len(data) == 0 {
		return errors.New("empty state data")
	}
	if !C.wrap_retro_unserialize(unsafe.Pointer(&data[0]), C.size_t(len(data))) {
		return ErrUnserializeFailed
	}
	return nil
}

// CheatSupported reports whether the loaded core exports retro_cheat_set at
// all — cheats are best-effort per the libretro spec: a core is free to
// implement these as no-ops (many modern/complex cores do), in which case
// CheatSet below silently does nothing rather than erroring, same as
// Serialize/Unserialize's own core_supports_* gate.
func CheatSupported() bool {
	return bool(C.core_supports_cheats())
}

// CheatReset clears every cheat previously applied via CheatSet — called
// once before re-applying the full enabled set (see stream_session.go's
// handleSetCheats), matching how RetroArch itself re-syncs its cheat list
// rather than trying to diff against whatever was applied before.
func CheatReset() {
	C.wrap_retro_cheat_reset()
}

// CheatSet applies (or removes) one cheat code at the given index. code is
// in whatever raw format the CORE itself expects for its system (e.g. Game
// Genie/Pro Action Replay strings for the NES/SNES/Genesis cores) — there's
// no cheat database/lookup here, the caller supplies a raw code as-is.
func CheatSet(index uint, enabled bool, code string) {
	cCode := C.CString(code)
	defer C.free(unsafe.Pointer(cCode))
	C.wrap_retro_cheat_set(C.uint(index), C.bool(enabled), cCode)
}

// ---------------------------------------------------------------------------
// Callback registration — Go-side setters just store the callback.
// C.register_all_callbacks() (called via RegisterCallbacks()) wires them
// into the Libretro core using extern declarations (avoids CGo type issues).
// ---------------------------------------------------------------------------

// SetVideoRefreshCallback stores the video refresh callback.
func SetVideoRefreshCallback(cb VideoRefreshCallback) {
	onVideoRefresh = cb
}

// SetAudioSampleCallback stores the per-sample audio callback.
func SetAudioSampleCallback(cb AudioSampleCallback) {
	onAudioSample = cb
}

// SetAudioSampleBatchCallback stores the batch audio callback.
func SetAudioSampleBatchCallback(cb AudioSampleBatchCallback) {
	onAudioSampleBatch = cb
}

// SetInputPollCallback stores the input poll callback.
func SetInputPollCallback(cb func()) {
	onInputPoll = cb
}

// SetInputStateCallback stores the input state callback.
func SetInputStateCallback(cb InputStateCallback) {
	onInputState = cb
}

// SetEnvironmentCallback stores the environment callback.
func SetEnvironmentCallback(cb EnvironmentCallback) {
	onEnvironment = cb
}

// SetRumbleStateCallback stores the rumble callback.
func SetRumbleStateCallback(cb RumbleStateCallback) {
	onRumbleState = cb
}

// InitEnvPaths creates the system (BIOS) and save directories and stores C
// string pointers for use by environment callbacks. Call once at startup.
// Both should be persistent, host-mounted folders (BIOS_PATH/SAVES_PATH) —
// cores read them via the GET_SYSTEM_DIRECTORY/GET_SAVE_DIRECTORY
// environment callbacks (system dir for BIOS files like PS1's
// scph1001.bin, save dir for in-game battery/SRAM saves).
func InitEnvPaths(systemDir, saveDir string) {
	cSystemDir := C.CString(systemDir)
	defer C.free(unsafe.Pointer(cSystemDir))
	cSaveDir := C.CString(saveDir)
	defer C.free(unsafe.Pointer(cSaveDir))
	C.init_env_paths(cSystemDir, cSaveDir)
}

// RegisterCallbacks wires all stored Go callbacks into the Libretro core.
// Must be called AFTER all Set*Callback calls and BEFORE retro_init().
func RegisterCallbacks() {
	C.register_all_callbacks()
}

// GetSystemDir returns the C string pointer for the system directory.
func GetSystemDir() unsafe.Pointer {
	return C.get_system_dir()
}

// GetSaveDir returns the C string pointer for the save directory.
func GetSaveDir() unsafe.Pointer {
	return C.get_save_dir()
}

// GetCoreLogCallback returns a C function pointer matching
// retro_log_printf_t, for use as the `log` field of a struct
// retro_log_callback in response to RETRO_ENVIRONMENT_GET_LOG_INTERFACE.
func GetCoreLogCallback() unsafe.Pointer {
	return C.get_core_log_cb()
}

// GetRumbleCallback returns a C function pointer matching
// retro_set_rumble_state_t, for use as the `set_rumble_state` field of a
// struct retro_rumble_interface in response to
// RETRO_ENVIRONMENT_GET_RUMBLE_INTERFACE.
func GetRumbleCallback() unsafe.Pointer {
	return C.get_rumble_cb()
}

// ---------------------------------------------------------------------------
// Disk control (RETRO_ENVIRONMENT_SET_DISK_CONTROL_INTERFACE)
// ---------------------------------------------------------------------------

// HandleSetDiskControl copies the core's struct retro_disk_control_callback
// out of `data` and keeps it — the answer to environment command 13, which
// this switch simply did not handle before (a PS1 game in several discs was
// therefore unfinishable: nothing could ever mount disc 2).
//
// Reports whether the interface is USABLE, not merely present: a core is
// allowed to leave any of the seven pointers null, and one without
// get_num_images tells us nothing we can act on.
func HandleSetDiskControl(data unsafe.Pointer) bool {
	if data == nil {
		return false
	}
	C.store_disk_control((*C.struct_retro_disk_control_callback)(data))

	return C.has_disk_control() != 0
}

// DiskCount is how many images the current game holds — 0 when the core
// never registered the interface, or when the game is a single file. The
// COUNT IS ONLY MEANINGFUL ONCE THE GAME IS LOADED: a core registers the
// interface at retro_init, but only parses the .m3u in retro_load_game, so
// asking too early legitimately answers 0.
func DiskCount() int {
	return int(C.disk_image_count())
}

// DiskIndex is the image currently mounted, or -1 when unknown.
func DiskIndex() int {
	return int(C.disk_image_index())
}

// DiskSelect mounts image `index` — eject, choose, close, which is the only
// sequence a core honours (see disk_select's own doc). MUST run on the
// emulation goroutine, never concurrently with an in-flight retro_run: these
// are direct calls into the core, the same thread-safety class as
// retro_serialize/retro_cheat_set.
func DiskSelect(index int) bool {
	if index < 0 {
		return false
	}

	return C.disk_select(C.unsigned(index)) != 0
}

// CoreVariable mirrors one entry of the classic struct retro_variable list a
// core hands to RETRO_ENVIRONMENT_SET_VARIABLES — Value here is the core's
// OWN "description; possible|values" string (libretro's packed format for
// this older environment call, distinct from the newer SET_CORE_OPTIONS
// structured API), not a resolved setting.
type CoreVariable struct {
	Key   string
	Value string
}

// ListCoreVariables parses the NULL-key-terminated struct retro_variable
// array a core passes to SET_VARIABLES — added purely to find out, from the
// core itself, what option keys/values it actually exposes (e.g. a PS1
// core's own pad-type option) instead of guessing, since this vendored
// libretro.h has no accompanying core-options metadata file to read that
// from statically.
func ListCoreVariables(data unsafe.Pointer) []CoreVariable {
	var out []CoreVariable
	if data == nil {
		return out
	}
	const entrySize = unsafe.Sizeof(C.struct_retro_variable{})
	for i := 0; ; i++ {
		entry := (*C.struct_retro_variable)(unsafe.Add(data, uintptr(i)*entrySize))
		if entry.key == nil {
			break
		}
		out = append(out, CoreVariable{Key: C.GoString(entry.key), Value: C.GoString(entry.value)})
	}
	return out
}

// GetVariableKey reads the `key` field the core already set on a single
// struct retro_variable passed to RETRO_ENVIRONMENT_GET_VARIABLE — the
// caller needs this to know WHICH option is being asked for before deciding
// whether to override it.
func GetVariableKey(data unsafe.Pointer) string {
	if data == nil {
		return ""
	}
	entry := (*C.struct_retro_variable)(data)
	if entry.key == nil {
		return ""
	}
	return C.GoString(entry.key)
}

// SetVariableValue writes an overridden value into a single struct
// retro_variable for RETRO_ENVIRONMENT_GET_VARIABLE (paired with
// GetVariableKey above) — the leaked C string is intentional, same
// justification as GetSystemDir/GetSaveDir's persistent strings: the core
// keeps reading this pointer for the rest of the session (in practice, for
// the rest of the process, since one core loads per process here), freeing
// it would be a use-after-free, not a real per-call leak.
func SetVariableValue(data unsafe.Pointer, value string) {
	// UNE SEULE COPIE C PAR VALEUR. Chaque réponse allouait la sienne, jamais
	// rendue : sans conséquence tant que seules deux options étaient forcées,
	// une fuite dès que le moteur répond à toutes (chantier 13). Le cœur lit
	// la valeur sans la modifier : la partager est sûr, et l'ensemble reste
	// petit — les valeurs distinctes des options d'un cœur.
	internedValuesMu.Lock()
	cValue, known := internedValues[value]
	if !known {
		cValue = C.CString(value)
		internedValues[value] = cValue
	}
	internedValuesMu.Unlock()

	entry := (*C.struct_retro_variable)(data)
	entry.value = cValue
}

var (
	internedValuesMu sync.Mutex
	internedValues   = map[string]*C.char{}
)

// SaveRAM rend une COPIE de la mémoire de sauvegarde du jeu — la pile d'une
// cartouche, la memory card d'une PS1 — ou nil si ce cœur n'en a pas.
//
// Une copie, et pas la tranche elle-même : le tampon appartient au cœur, qui
// écrit dedans à chaque image. Le rendre tel quel donnerait à l'appelant une
// vue qui bouge sous lui pendant qu'il l'écrit sur le disque.
//
// À N'APPELER QUE DEPUIS LA GOROUTINE D'ÉMULATION, entre deux retro_run() —
// même discipline que Serialize, et pour la même raison : c'est de la mémoire
// que le cœur est en train de faire vivre.
func SaveRAM() []byte {
	size := C.wrap_retro_get_memory_size(C.RETRO_MEMORY_SAVE_RAM)
	if size == 0 {
		return nil
	}
	ptr := C.wrap_retro_get_memory_data(C.RETRO_MEMORY_SAVE_RAM)
	if ptr == nil {
		return nil
	}

	return C.GoBytes(ptr, C.int(size))
}

// RestoreSaveRAM recopie une sauvegarde relue du disque DANS la mémoire du
// cœur. À appeler après retro_load_game (le tampon n'existe pas avant) et
// avant la première image.
//
// Une taille qui ne correspond pas n'est pas une erreur fatale : on recopie
// ce qui tient. Un fichier plus COURT arrive légitimement (une pile encore
// vierge, un cœur qui a grandi entre deux versions) ; un fichier plus long
// est tronqué plutôt que d'écrire hors des clous.
func RestoreSaveRAM(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	size := C.wrap_retro_get_memory_size(C.RETRO_MEMORY_SAVE_RAM)
	if size == 0 {
		return errors.New("ce cœur n'expose pas de mémoire de sauvegarde")
	}
	ptr := C.wrap_retro_get_memory_data(C.RETRO_MEMORY_SAVE_RAM)
	if ptr == nil {
		return errors.New("mémoire de sauvegarde absente")
	}

	n := len(data)
	if n > int(size) {
		n = int(size)
	}
	C.memcpy(ptr, unsafe.Pointer(&data[0]), C.size_t(n))

	return nil
}

// SaveRAMSize dit la taille que le cœur annonce, sans rien copier — de quoi
// journaliser ou décider sans payer la copie.
func SaveRAMSize() int { return int(C.wrap_retro_get_memory_size(C.RETRO_MEMORY_SAVE_RAM)) }

// ControllerType — un type de manette que le cœur DÉCLARE savoir émuler, avec
// l'identifiant à passer à retro_set_controller_port_device pour l'obtenir.
type ControllerType struct {
	Description string
	ID          uint32
}

// ListControllerTypes lit le tableau de retro_controller_info que le cœur
// pousse via SET_CONTROLLER_INFO.
//
// ON NE LE LISAIT PAS, et c'est ce qui rendait la vibration indiagnosticable.
// Chaque console était enregistrée en RETRO_DEVICE_JOYPAD — une manette
// générique, qui n'a pas de moteurs. Un cœur n'a alors aucune raison
// d'appeler set_rumble_state, et il ne l'appelait pas : quatre lignes à zéro
// au démarrage, plus rien ensuite, sur une session PS2 de sept minutes
// (2026-09-08). Le cœur disait pourtant, à cet endroit précis, quels types il
// sait faire et sous quel numéro.
//
// Le tableau est terminé par une entrée de description nulle, comme les
// variables de cœur juste au-dessus. `data` appartient au cœur : on copie
// tout ce qu'on garde.
func ListControllerTypes(data unsafe.Pointer) []ControllerType {
	if data == nil {
		return nil
	}
	infos := (*C.struct_retro_controller_info)(data)
	out := []ControllerType{}
	// SET_CONTROLLER_INFO rend un tableau de retro_controller_info, un par
	// PORT, terminé par une entrée dont `types` est nul.
	for port := 0; port < 16; port++ {
		info := (*C.struct_retro_controller_info)(unsafe.Pointer(
			uintptr(unsafe.Pointer(infos)) + uintptr(port)*unsafe.Sizeof(*infos)))
		if info.types == nil {
			break
		}
		for i := 0; i < int(info.num_types); i++ {
			desc := (*C.struct_retro_controller_description)(unsafe.Pointer(
				uintptr(unsafe.Pointer(info.types)) + uintptr(i)*unsafe.Sizeof(*info.types)))
			if desc.desc == nil {
				continue
			}
			out = append(out, ControllerType{
				Description: C.GoString(desc.desc),
				ID:          uint32(desc.id),
			})
		}
		// Les ports suivants répètent presque toujours la même liste : une
		// seule suffit à savoir quoi demander.
		break
	}

	return out
}
