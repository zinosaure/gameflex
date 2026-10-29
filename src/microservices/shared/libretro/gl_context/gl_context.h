// Headless EGL/GBM OpenGL context — for hardware-rendered libretro cores
// (N64/parallel_n64, PS2/play) which have no software-rendering fallback,
// unlike every other core this backend runs. Deliberately does NOT link
// against libGL/libGLESv2 at all: every single GL function used here,
// including the handful this file's own FBO/readback code needs, is
// resolved at runtime via eglGetProcAddress — the exact same mechanism a
// libretro core itself uses to get its own GL functions (get_proc_address in
// struct retro_hw_render_callback, see ../libretro/libretro.h). That keeps
// this file's own dependency footprint to just EGL + GBM headers, and means
// it works identically whether the eventual context turns out to be desktop
// GL or GLES — no `#include <GL/gl.h>` (not installed, no libgl1-mesa-dev in
// this image) or `<GLES2/gl2.h>` needed for OUR side either.
//
// Deux implémentations derrière cette API (chantier 13) : GBM pour le serveur
// (gl_context_gbm.c), l'affichage EGL d'Android pour le téléphone
// (gl_context_android.c). Ce qui suit décrit le serveur ; les différences
// d'Android sont dans son fichier.
#ifndef RETROGAMING_GLCONTEXT_H
#define RETROGAMING_GLCONTEXT_H

#include <stdint.h>

// Mirrors enum retro_hw_context_type's numeric values (../libretro/libretro.h)
// — this package doesn't import that header (kept dependency-free of the
// libretro package, in case something else ever wants a headless GL context
// without pulling in the whole CGo libretro bridge), the caller translates.
#define GLCONTEXT_TYPE_OPENGL      1
#define GLCONTEXT_TYPE_OPENGLES2   2
#define GLCONTEXT_TYPE_OPENGL_CORE 3
#define GLCONTEXT_TYPE_OPENGLES3   4

typedef struct gl_context gl_context;

// gl_context_create opens the container's render node (/dev/dri/renderD*,
// the one GPU the kernel hands it — chantier 39), creates a GBM device, an EGL
// display over it (EGL_PLATFORM_GBM_KHR), and a surfaceless GL context
// (EGL_KHR_surfaceless_context — no default framebuffer at all; the core
// renders into an FBO we create in gl_context_resize instead, matching how
// every libretro frontend with HW render support actually works). Returns
// NULL on any failure — errOut, if non-NULL, gets a short static string
// describing which step failed (for the caller's own error message, no
// ownership transferred, never freed).
gl_context *gl_context_create(int context_type, int major, int minor, const char **errOut);

// gl_context_resize (re)creates the color+depth/stencil renderbuffers and
// the FBO they're attached to, at the given size — called once up front
// (GetSystemAVInfo's initial geometry) and again if the core requests a
// different size mid-session (RETRO_ENVIRONMENT_SET_GEOMETRY). Safe to call
// repeatedly; the previous attachment (if any) is deleted first. Returns 0
// on success.
int gl_context_resize(gl_context *ctx, int width, int height);

// gl_context_framebuffer returns the FBO name the core should render into —
// what get_current_framebuffer (struct retro_hw_render_callback) reports
// back to the core on every frame.
uint32_t gl_context_framebuffer(gl_context *ctx);

// gl_context_get_proc_address is handed to the core as-is for its own
// get_proc_address callback — a core resolves every GL function it uses
// (aside from the handful this file resolves for itself, see the .c file)
// through this exact function.
void *gl_context_get_proc_address(const char *name);

// gl_context_read_pixels reads the current FBO's color attachment back into
// `buf` (caller-allocated, width*height*4 bytes) as BGRA — the exact byte
// layout stream_session.go's emitFrame already expects from every OTHER
// (software) pixel format conversion path, so a HW-rendered frame can be
// hand off to it identically, no separate conversion needed on the Go side.
// Returns 0 on success.
int gl_context_read_pixels(gl_context *ctx, int width, int height, void *buf);

// gl_context_destroy tears down the FBO/renderbuffers, EGL context/display,
// GBM device, and the render node's file descriptor. ctx is invalid after
// this call.
void gl_context_destroy(gl_context *ctx);

#endif
