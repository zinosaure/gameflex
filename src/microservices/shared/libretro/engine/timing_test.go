package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// palCoreSource — un cœur qui fait ce que fait LRPS2 avec un jeu PAL : il se
// charge à la cadence NTSC (59,94, celle du BIOS), puis, dès que le jeu pose
// son mode vidéo, annonce la nouvelle par SET_SYSTEM_AV_INFO depuis
// retro_run. Chaque retro_run émule UNE image de la console et le son qu'elle
// dure à la fréquence annoncée : c'est le cœur qui fixe la vitesse du jeu,
// la boucle du moteur seulement le nombre d'images par seconde réelle.
const palCoreSource = `
#include <stdbool.h>
#include <stdint.h>
#include <string.h>
#include "libretro.h"

static retro_environment_t environ_cb;
static retro_video_refresh_t video_cb;
static retro_audio_sample_batch_t audio_batch_cb;
static double fps = 60.0 / 1.001;
static double rate = 48000.0;
static double owed;
static unsigned runs;
static uint32_t pixels[4 * 4];
static int16_t audio[2 * 4096];

void retro_set_environment(retro_environment_t cb) { environ_cb = cb; }
void retro_set_video_refresh(retro_video_refresh_t cb) { video_cb = cb; }
void retro_set_audio_sample(void* cb) { (void)cb; }
void retro_set_audio_sample_batch(retro_audio_sample_batch_t cb) { audio_batch_cb = cb; }
void retro_set_input_poll(void* cb) { (void)cb; }
void retro_set_input_state(void* cb) { (void)cb; }
void retro_init(void) {}
void retro_deinit(void) {}
unsigned retro_api_version(void) { return 1; }
void retro_get_system_info(struct retro_system_info* info) {
    memset(info, 0, sizeof *info);
    info->library_name = "pal";
    info->library_version = "1";
    info->need_fullpath = true;
}
void retro_get_system_av_info(struct retro_system_av_info* info) {
    memset(info, 0, sizeof *info);
    info->geometry.base_width = 4;
    info->geometry.base_height = 4;
    info->geometry.max_width = 4;
    info->geometry.max_height = 4;
    info->geometry.aspect_ratio = 4.0f / 3.0f;
    info->timing.fps = fps;
    info->timing.sample_rate = rate;
}
void retro_set_controller_port_device(unsigned port, unsigned device) { (void)port; (void)device; }
void retro_reset(void) {}
bool retro_load_game(const struct retro_game_info* game) {
    (void)game;
    unsigned format = RETRO_PIXEL_FORMAT_XRGB8888;
    environ_cb(RETRO_ENVIRONMENT_SET_PIXEL_FORMAT, &format);
    return true;
}
void retro_unload_game(void) {}
void retro_run(void) {
    runs++;
    if (runs == 2) {
        struct retro_system_av_info info;
        fps = NEW_FPS;
        rate = NEW_RATE;
        retro_get_system_av_info(&info);
        environ_cb(RETRO_ENVIRONMENT_SET_SYSTEM_AV_INFO, &info);
    }
    video_cb(pixels, 4, 4, 4 * sizeof(uint32_t));
    owed += rate / fps;
    size_t frames = (size_t)owed;
    owed -= (double)frames;
    audio_batch_cb(audio, frames);
}
`

// buildPALCore compile le faux cœur avec la cadence et la fréquence qu'il
// annoncera, ou saute le test sans compilateur C.
func buildPALCore(t *testing.T, fps, rate float64) string {
	t.Helper()
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("pas de compilateur C pour construire le faux cœur")
	}
	header, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "pal_core.c")
	if err := os.WriteFile(source, []byte(palCoreSource), 0o644); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(dir, "pal_core.so")
	command := exec.Command(compiler, "-shared", "-fPIC", "-I"+header,
		fmt.Sprintf("-DNEW_FPS=%g", fps), fmt.Sprintf("-DNEW_RATE=%g", rate), "-o", library, source)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("faux cœur : %v\n%s", err, out)
	}

	return library
}

// UN JEU PAL TOURNE À 50 IMAGES PAR SECONDE, MÊME QUAND LE CŒUR NE LE SAIT
// QU'APRÈS LE CHARGEMENT (le porteur, 2026-11-14 : Tekken 5 en accéléré, le
// son de Kingdom Hearts II en décalé). LRPS2 se charge à 59,94 et n'annonce 50
// qu'une fois le jeu parti : le moteur ignorait SET_SYSTEM_AV_INFO et
// continuait à 59,94 retro_run par seconde — le jeu tournait 20 % trop vite,
// et donnait 20 % de son de trop à un encodeur réglé sur l'horloge.
//
// Le moteur doit suivre la cadence annoncée — les images ET le son — et
// rendre le son à la fréquence du chargement, celle sur laquelle l'appelant a
// ouvert son encodeur, même quand le cœur change la sienne.
func TestEngineFollowsTheTimingTheCoreAnnounces(t *testing.T) {
	for _, scene := range []struct {
		name string
		rate float64
	}{
		{"PAL, son inchangé (LRPS2)", 48000},
		{"PAL, son changé", 32000},
	} {
		t.Run(scene.name, func(t *testing.T) {
			library := buildPALCore(t, 50, scene.rate)
			var frames, audioFrames atomic.Int64
			engine, err := Load(Config{
				CorePath:  library,
				Core:      "pal",
				ConsoleID: "test",
				Game:      Game{Path: filepath.Join(t.TempDir(), "jeu.iso")},
				OnFrame:   func(Frame) { frames.Add(1) },
				OnAudio:   func(samples []int16) { audioFrames.Add(int64(len(samples) / 2)) },
			})
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Stop()
			if loaded := engine.Info().Timing; loaded.FPS < 59.9 || loaded.FPS > 60 || loaded.SampleRate != 48000 {
				t.Fatalf("cadence du chargement : %+v, attendu 59,94 et 48000 Hz", loaded)
			}
			engine.Run()

			// Le cœur annonce 50 à sa deuxième image ; on laisse passer la
			// bascule, puis on compte deux secondes.
			time.Sleep(500 * time.Millisecond)
			frames.Store(0)
			audioFrames.Store(0)
			const window = 2 * time.Second
			time.Sleep(window)
			perSecond := float64(frames.Load()) / window.Seconds()
			soundPerSecond := float64(audioFrames.Load()) / window.Seconds()

			if perSecond < 46 || perSecond > 54 {
				t.Errorf("%.1f images par seconde après l'annonce de 50", perSecond)
			}
			if got := engine.OutputFPS(); got != 50 {
				t.Errorf("OutputFPS %.2f, attendu 50", got)
			}
			// Le son à l'horloge de l'encodeur : 48000 échantillons stéréo
			// par seconde, à 5 % près (les lots partent par dix millisecondes).
			if soundPerSecond < 45600 || soundPerSecond > 50400 {
				t.Errorf("%.0f échantillons de son par seconde, attendu 48000", soundPerSecond)
			}
			if engine.Info().Timing.SampleRate != 48000 {
				t.Errorf("Info rend %.0f Hz : la fréquence de l'encodeur ne doit pas changer", engine.Info().Timing.SampleRate)
			}
		})
	}
}

// LE SON RAMENÉ À LA FRÉQUENCE DE L'ENCODEUR, D'UN LOT À L'AUTRE : une rampe
// de 32000 Hz rendue à 48000 Hz reste une rampe — pas de saut à la jonction
// de deux lots, où le premier échantillon s'interpole avec le dernier du lot
// d'avant —, et la durée est gardée.
func TestSoundRateKeepsARampAcrossBatches(t *testing.T) {
	var sound soundRate
	sound.convert(32000, 48000)
	var out []int16
	value := int16(0)
	for batch := 0; batch < 50; batch++ {
		samples := make([]int16, 2*320) // 10 ms à 32000 Hz
		for frame := 0; frame < 320; frame++ {
			samples[2*frame], samples[2*frame+1] = value, -value
			value++
		}
		out = append(out, sound.resample(samples)...)
	}
	if frames := len(out) / 2; frames < 23990 || frames > 24001 {
		t.Fatalf("%d échantillons rendus pour 0,5 s, attendu 24000", frames)
	}
	for frame := 1; frame < len(out)/2; frame++ {
		if step := out[2*frame] - out[2*frame-2]; step < 0 || step > 1 {
			t.Fatalf("échantillon %d : la rampe saute de %d", frame, step)
		}
		if out[2*frame+1] != -out[2*frame] {
			t.Fatalf("échantillon %d : les deux voies se mêlent (%d, %d)", frame, out[2*frame], out[2*frame+1])
		}
	}

	// Sans changement de fréquence, le son passe tel quel.
	sound.convert(48000, 48000)
	samples := []int16{1, 2, 3, 4}
	if same := sound.resample(samples); &same[0] != &samples[0] {
		t.Fatal("un son à la bonne fréquence est recopié")
	}
}
