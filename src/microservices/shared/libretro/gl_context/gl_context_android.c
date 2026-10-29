//go:build android

// LE CONTEXTE D'ANDROID — l'équivalent de gl_context_gbm.c pour le téléphone
// (chantier 13). Pas de GBM dans le NDK, et pas de /dev/dri à ouvrir dans le
// bac à sable d'une application : l'affichage EGL par défaut, et un pbuffer
// d'un pixel pour rendre le contexte courant. Le cœur dessine dans notre FBO,
// comme sur le serveur (gl_context_fbo.c).
//
// GLES SEULEMENT. Android n'a pas d'OpenGL de bureau, et ses cœurs le savent :
// ils demandent GLES 2 ou GLES 3.

#include "gl_context_internal.h"

#include <EGL/eglext.h>

#include <dlfcn.h>
#include <stdlib.h>

#ifndef EGL_OPENGL_ES3_BIT_KHR
#define EGL_OPENGL_ES3_BIT_KHR 0x00000040
#endif
#ifndef EGL_CONTEXT_MINOR_VERSION_KHR
#define EGL_CONTEXT_MINOR_VERSION_KHR 0x30FB
#endif

static void *gles_library = NULL;

// eglGetProcAddress n'est tenu de rendre les fonctions de BASE de GLES que
// depuis EGL 1.5 (ou EGL_KHR_get_all_proc_addresses) ; avant, seulement les
// extensions. Les fonctions de base sont des symboles de libGLESv3 — publique
// dans le NDK —, qu'on va donc chercher là quand EGL ne les rend pas.
void *gl_context_get_proc_address(const char *name) {
    void *symbol = (void *)eglGetProcAddress(name);
    if (symbol) return symbol;

    if (!gles_library) {
        gles_library = dlopen("libGLESv3.so", RTLD_NOW | RTLD_LOCAL);
        if (!gles_library) gles_library = dlopen("libGLESv2.so", RTLD_NOW | RTLD_LOCAL);
    }
    return gles_library ? dlsym(gles_library, name) : NULL;
}

// create_es crée un contexte GLES `version` (2 ou 3), avec sa surface, et le
// rend courant. `minor` ne sert qu'en GLES 3, et seulement si le cœur l'a
// demandé ; un pilote qui refuse la version mineure a droit à un second essai
// sans elle. Rend 0 si le contexte est courant.
static int create_es(gl_context *ctx, int version, int minor) {
    EGLint configAttribs[] = {
        EGL_RENDERABLE_TYPE, version == 3 ? EGL_OPENGL_ES3_BIT_KHR : EGL_OPENGL_ES2_BIT,
        EGL_SURFACE_TYPE, EGL_PBUFFER_BIT,
        EGL_RED_SIZE, 8, EGL_GREEN_SIZE, 8, EGL_BLUE_SIZE, 8, EGL_ALPHA_SIZE, 8,
        EGL_NONE,
    };
    EGLConfig config;
    EGLint numConfigs = 0;
    if (!eglChooseConfig(ctx->display, configAttribs, &config, 1, &numConfigs) || numConfigs < 1) {
        return -1;
    }

    EGLint withMinor[] = {
        EGL_CONTEXT_CLIENT_VERSION, version,
        EGL_CONTEXT_MINOR_VERSION_KHR, minor,
        EGL_NONE,
    };
    EGLint withoutMinor[] = { EGL_CONTEXT_CLIENT_VERSION, version, EGL_NONE };

    if (version == 3 && minor > 0) {
        ctx->context = eglCreateContext(ctx->display, config, EGL_NO_CONTEXT, withMinor);
    }
    if (ctx->context == EGL_NO_CONTEXT) {
        ctx->context = eglCreateContext(ctx->display, config, EGL_NO_CONTEXT, withoutMinor);
    }
    if (ctx->context == EGL_NO_CONTEXT) return -1;

    EGLint pbufferAttribs[] = { EGL_WIDTH, 1, EGL_HEIGHT, 1, EGL_NONE };
    ctx->surface = eglCreatePbufferSurface(ctx->display, config, pbufferAttribs);
    if (ctx->surface == EGL_NO_SURFACE
        || !eglMakeCurrent(ctx->display, ctx->surface, ctx->surface, ctx->context)) {
        if (ctx->surface != EGL_NO_SURFACE) eglDestroySurface(ctx->display, ctx->surface);
        eglDestroyContext(ctx->display, ctx->context);
        ctx->surface = EGL_NO_SURFACE;
        ctx->context = EGL_NO_CONTEXT;
        return -1;
    }
    return 0;
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
    ctx->display = EGL_NO_DISPLAY;
    ctx->context = EGL_NO_CONTEXT;
    ctx->surface = EGL_NO_SURFACE;

    // Un cœur qui demande l'OpenGL de bureau n'est pas un cœur Android : le
    // dire, plutôt que de lui donner un contexte dont il ne saura rien faire.
    if (context_type != GLCONTEXT_TYPE_OPENGLES2 && context_type != GLCONTEXT_TYPE_OPENGLES3) {
        FAIL("desktop OpenGL is not available on Android: the core must request GLES");
    }

    ctx->display = eglGetDisplay(EGL_DEFAULT_DISPLAY);
    if (ctx->display == EGL_NO_DISPLAY) FAIL("eglGetDisplay failed");

    EGLint eglMajor, eglMinor;
    if (!eglInitialize(ctx->display, &eglMajor, &eglMinor)) FAIL("eglInitialize failed");
    if (!eglBindAPI(EGL_OPENGL_ES_API)) FAIL("eglBindAPI failed");

    // GLES 3 MÊME POUR UN CŒUR QUI DEMANDE GLES 2. GLES 3 contient GLES 2, et
    // les formats du FBO (RGBA8, DEPTH24_STENCIL8) n'y sont garantis qu'à
    // partir de GLES 3 — en GLES 2, seulement par extension : c'est ce qui fait
    // échouer GLES 2 dans le test de Linux. GLES 2 seul ne sert que sur un
    // appareil qui n'a pas mieux.
    int wantedMinor = (context_type == GLCONTEXT_TYPE_OPENGLES3 && major == 3) ? minor : 0;
    if (create_es(ctx, 3, wantedMinor) != 0) {
        if (context_type == GLCONTEXT_TYPE_OPENGLES3) FAIL("no GLES 3 context on this device");
        if (create_es(ctx, 2, 0) != 0) FAIL("no GLES context on this device");
    }

    if (gl_context_load_functions(ctx) != 0) FAIL("failed to resolve required GL functions");

    return ctx;

fail:
    if (ctx) {
        if (ctx->context != EGL_NO_CONTEXT) {
            eglMakeCurrent(ctx->display, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
            if (ctx->surface != EGL_NO_SURFACE) eglDestroySurface(ctx->display, ctx->surface);
            eglDestroyContext(ctx->display, ctx->context);
        }
        free(ctx);
    }
    return NULL;
#undef FAIL
}

// SANS eglTerminate, contrairement à Linux. L'affichage par défaut est celui
// de tout le processus : l'interface de l'application, l'encodeur MediaCodec
// et ses surfaces s'en servent aussi. Le terminer couperait leur EGL sous
// leurs pieds — sur les versions d'Android où il n'est pas compté.
void gl_context_destroy(gl_context *ctx) {
    if (!ctx) return;
    gl_context_delete_framebuffer(ctx);
    eglMakeCurrent(ctx->display, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
    eglDestroySurface(ctx->display, ctx->surface);
    eglDestroyContext(ctx->display, ctx->context);
    eglReleaseThread();
    free(ctx);
}
