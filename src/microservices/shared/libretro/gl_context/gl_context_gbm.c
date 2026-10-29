//go:build linux && !android

// LE CONTEXTE DE LINUX — celui du serveur : un affichage EGL sur un
// périphérique GBM, sans surface. Le FBO et la relecture sont communs aux deux
// plateformes (gl_context_fbo.c) ; Android a sa propre naissance du contexte
// (gl_context_android.c).

#include "gl_context_internal.h"

#include <EGL/eglext.h>
#include <gbm.h>

#include <fcntl.h>
#include <glob.h>
#include <unistd.h>
#include <stdlib.h>
#include <string.h>

void *gl_context_get_proc_address(const char *name) {
    return (void *)eglGetProcAddress(name);
}

gl_context *gl_context_create(int context_type, int major, int minor, const char **errOut) {
#define FAIL(msg)              \
    do {                       \
        if (errOut) *errOut = msg; \
        goto fail;             \
    } while (0)

    gl_context *ctx = calloc(1, sizeof(gl_context));
    if (!ctx) {
        if (errOut) *errOut = "calloc failed";
        return NULL;
    }
    ctx->fd = -1;

    // LE NŒUD DE RENDU QUE LE CONTENEUR A REÇU : le kernel ne lui donne que
    // celui du GPU des parties (chantier 39). « renderD128 » écrit ici ne
    // valait que pour le premier GPU de l'hôte.
    glob_t nodes;
    memset(&nodes, 0, sizeof(nodes));
    if (glob("/dev/dri/renderD*", 0, NULL, &nodes) != 0 || nodes.gl_pathc == 0) {
        globfree(&nodes);
        FAIL("no /dev/dri/renderD* node");
    }
    ctx->fd = open(nodes.gl_pathv[0], O_RDWR);
    globfree(&nodes);
    if (ctx->fd < 0) FAIL("open /dev/dri/renderD* failed");

    ctx->gbm = gbm_create_device(ctx->fd);
    if (!ctx->gbm) FAIL("gbm_create_device failed");

    ctx->display = eglGetPlatformDisplay(EGL_PLATFORM_GBM_KHR, ctx->gbm, NULL);
    if (ctx->display == EGL_NO_DISPLAY) FAIL("eglGetPlatformDisplay failed");

    EGLint eglMajor, eglMinor;
    if (!eglInitialize(ctx->display, &eglMajor, &eglMinor)) FAIL("eglInitialize failed");

    int isGles = (context_type == GLCONTEXT_TYPE_OPENGLES2 || context_type == GLCONTEXT_TYPE_OPENGLES3);
    if (!eglBindAPI(isGles ? EGL_OPENGL_ES_API : EGL_OPENGL_API)) FAIL("eglBindAPI failed");

    // No EGL_SURFACE_TYPE requirement — this context is surfaceless (see
    // this file's own doc), never binds to a real EGL window/pbuffer
    // surface at all, so requiring one here just risks rejecting configs
    // a GBM platform display doesn't happen to advertise a surface type
    // for.
    EGLint renderable = isGles
        ? (context_type == GLCONTEXT_TYPE_OPENGLES3 ? EGL_OPENGL_ES3_BIT : EGL_OPENGL_ES2_BIT)
        : EGL_OPENGL_BIT;
    EGLint configAttribs[] = {
        EGL_RENDERABLE_TYPE, renderable,
        EGL_RED_SIZE, 8, EGL_GREEN_SIZE, 8, EGL_BLUE_SIZE, 8, EGL_ALPHA_SIZE, 8,
        EGL_NONE,
    };
    EGLConfig config;
    EGLint numConfigs = 0;
    if (!eglChooseConfig(ctx->display, configAttribs, &config, 1, &numConfigs) || numConfigs < 1) {
        FAIL("eglChooseConfig failed (no matching config)");
    }

    EGLint contextAttribs[16];
    int n = 0;
    if (major > 0) {
        contextAttribs[n++] = EGL_CONTEXT_MAJOR_VERSION;
        contextAttribs[n++] = major;
        contextAttribs[n++] = EGL_CONTEXT_MINOR_VERSION;
        contextAttribs[n++] = minor;
    }
    if (context_type == GLCONTEXT_TYPE_OPENGL_CORE) {
        contextAttribs[n++] = EGL_CONTEXT_OPENGL_PROFILE_MASK;
        contextAttribs[n++] = EGL_CONTEXT_OPENGL_CORE_PROFILE_BIT;
    }
    contextAttribs[n++] = EGL_NONE;

    ctx->context = eglCreateContext(ctx->display, config, EGL_NO_CONTEXT, contextAttribs);
    if (ctx->context == EGL_NO_CONTEXT) FAIL("eglCreateContext failed");

    // Surfaceless (EGL_KHR_surfaceless_context) — no default framebuffer at
    // all, see this file's own header doc on why that's fine (we render
    // into our own FBO, never the EGL surface).
    if (!eglMakeCurrent(ctx->display, EGL_NO_SURFACE, EGL_NO_SURFACE, ctx->context)) {
        FAIL("eglMakeCurrent failed");
    }

    if (gl_context_load_functions(ctx) != 0) FAIL("failed to resolve required GL functions");

    return ctx;

fail:
    if (ctx) {
        if (ctx->context != EGL_NO_CONTEXT && ctx->display != EGL_NO_DISPLAY) {
            eglDestroyContext(ctx->display, ctx->context);
        }
        if (ctx->display != EGL_NO_DISPLAY) eglTerminate(ctx->display);
        if (ctx->gbm) gbm_device_destroy(ctx->gbm);
        if (ctx->fd >= 0) close(ctx->fd);
        free(ctx);
    }
    return NULL;
#undef FAIL
}

void gl_context_destroy(gl_context *ctx) {
    if (!ctx) return;
    gl_context_delete_framebuffer(ctx);
    eglMakeCurrent(ctx->display, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
    eglDestroyContext(ctx->display, ctx->context);
    eglTerminate(ctx->display);
    gbm_device_destroy(ctx->gbm);
    close(ctx->fd);
    free(ctx);
}
