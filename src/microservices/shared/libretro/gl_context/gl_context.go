// Package gl_context provides a headless EGL/GBM OpenGL context — the
// hardware-rendering path hardware-only libretro cores (N64/parallel_n64,
// PS2/play) require, since unlike every other core this backend runs, they
// have no software renderer to fall back to. See gl_context.h's own doc for
// the implementation approach (surfaceless GBM context, no libGL/libGLESv2
// link dependency, every GL call resolved via eglGetProcAddress).
//
// DEUX PLATEFORMES, UNE API (chantier 13). Le serveur ouvre son contexte sur
// GBM (gl_context_gbm.c) ; le téléphone sur l'affichage EGL d'Android, en GLES
// seulement (gl_context_android.c) — pas de GBM dans le NDK, pas de /dev/dri
// dans le bac à sable d'une application. Le FBO du cœur et la relecture de ses
// pixels sont communs (gl_context_fbo.c).
package gl_context

/*
#cgo linux,!android pkg-config: egl gbm
#cgo android LDFLAGS: -lEGL -ldl
#include "gl_context.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"runtime"
	"unsafe"
)

// Context libretro's own retro_hw_context_type numeric values (see
// ../libretro/libretro.h's enum retro_hw_context_type) — the caller
// (stream_session.go's environmentCallback) passes these straight through,
// this package has no dependency on the libretro package's cgo types.
const (
	ContextTypeOpenGL     = 1
	ContextTypeOpenGLES2  = 2
	ContextTypeOpenGLCore = 3
	ContextTypeOpenGLES3  = 4
)

// Context wraps the C gl_context — every method must be called from the
// SAME goroutine that created it (see New's own doc: EGL contexts are
// thread-bound, this package pins the creating goroutine to its current OS
// thread for exactly that reason).
type Context struct {
	ptr *C.gl_context
}

// New creates a headless GL context of the given type/version — version 0/0
// means "no preference," left for the driver to pick its own default
// (many cores don't set version_major/version_minor at all).
//
// Pins the calling goroutine to its current OS thread for the rest of the
// process's life (runtime.LockOSThread with no matching UnlockOSThread) —
// required because EGL contexts are bound to the OS thread that made them
// current, and Go's scheduler is otherwise free to migrate a goroutine
// between OS threads between any two function calls. Every retro-gaming
// session already runs in its own OS PROCESS (see supervisor.go's
// exec.Command re-exec), so permanently dedicating one thread within that
// process to the session's emulation loop (the only place this is called
// from) costs nothing meaningful.
//
// Sur le téléphone, les parties se succèdent dans LE MÊME processus,
// l'application. Le fil verrouillé n'y fuit pas pour autant : quand la
// goroutine de la partie se termine sans le déverrouiller, Go jette le fil
// avec elle. Il faut seulement avoir appelé Destroy avant.
func New(contextType, major, minor int) (*Context, error) {
	runtime.LockOSThread()

	var cErr *C.char
	ptr := C.gl_context_create(C.int(contextType), C.int(major), C.int(minor), &cErr)
	if ptr == nil {
		msg := "unknown error"
		if cErr != nil {
			msg = C.GoString(cErr)
		}
		return nil, fmt.Errorf("gl_context: %s", msg)
	}
	return &Context{ptr: ptr}, nil
}

// Resize (re)creates the FBO the core renders into, at the given size —
// call once up front and again any time the core's actual output geometry
// changes. Safe to call with the same size repeatedly (still recreates the
// FBO — this is not expected to happen every frame).
func (c *Context) Resize(width, height int) error {
	if C.gl_context_resize(c.ptr, C.int(width), C.int(height)) != 0 {
		return fmt.Errorf("gl_context: failed to create a complete framebuffer at %dx%d", width, height)
	}
	return nil
}

// FramebufferID returns the FBO name the core should render into — wire
// this up as struct retro_hw_render_callback's get_current_framebuffer
// result (stream_session.go's environmentCallback).
func (c *Context) FramebufferID() uint32 {
	return uint32(C.gl_context_framebuffer(c.ptr))
}

// GetProcAddress resolves a GL function by name — wire this up as struct
// retro_hw_render_callback's get_proc_address (same function a libretro
// core calls to get every GL entry point it uses internally).
func GetProcAddress(name string) unsafe.Pointer {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	return C.gl_context_get_proc_address(cName)
}

// ReadPixels reads the current frame back from the FBO as BGRA — the exact
// byte layout emitFrame (stream_session.go) already expects from every
// other (software) pixel format conversion path, so a hardware-rendered
// frame can be handed off to it identically. width/height are the CORE's
// own per-call values (videoCallback's own width/height args when data is
// the RETRO_HW_FRAME_BUFFER_VALID sentinel) — deliberately NOT always
// c.width/c.height from the last Resize: Resize sizes the FBO to the
// core's max_width/max_height once up front (see stream_session.go's own
// doc on why), but any individual frame's ACTUAL rendered resolution can be
// smaller than that ceiling, and reading back more than what this call
// asks for would just be stale/garbage pixels from a previous frame in the
// unused margin.
func (c *Context) ReadPixels(width, height int) ([]byte, error) {
	buf := make([]byte, width*height*4)
	if len(buf) == 0 {
		return buf, nil
	}
	if code := C.gl_context_read_pixels(c.ptr, C.int(width), C.int(height), unsafe.Pointer(&buf[0])); code != 0 {
		// Le code GL, pas seulement « ça a raté » — voir la doc du C.
		return nil, fmt.Errorf("gl_context: glReadPixels failed (GL 0x%04x, %dx%d)", int(code), width, height)
	}

	// RETOURNÉ, PARCE QU'OPENGL COMPTE À L'ENVERS.
	//
	// L'origine d'OpenGL est en bas à gauche : `glReadPixels` rend donc les
	// rangées de la DERNIÈRE à la première, quand tout le reste de la chaîne —
	// l'encodeur, et les cœurs qui rendent en logiciel — les attend de la
	// première à la dernière.
	//
	// Constaté à l'écran le 2026-09-08, et c'est la seule façon de le
	// constater : la première PS2 qui a produit une image l'a produite tête en
	// bas, logo et texte lisibles dans un miroir. Rien dans le journal ne peut
	// dire ça — un compteur d'images monte pareil dans les deux sens.
	//
	// Ici plutôt que chez l'appelant : c'est CETTE lecture qui introduit
	// l'inversion, et la rendre à l'endroit est son travail. Un appelant qui
	// devrait s'en souvenir finirait par l'oublier.
	stride := width * 4
	row := make([]byte, stride)
	for top, bottom := 0, height-1; top < bottom; top, bottom = top+1, bottom-1 {
		topRow := buf[top*stride : top*stride+stride]
		bottomRow := buf[bottom*stride : bottom*stride+stride]
		copy(row, topRow)
		copy(topRow, bottomRow)
		copy(bottomRow, row)
	}

	return buf, nil
}

// Destroy tears down the FBO, EGL context/display, GBM device and its
// render-node file descriptor. The Context must not be used afterward.
func (c *Context) Destroy() {
	C.gl_context_destroy(c.ptr)
	c.ptr = nil
}
