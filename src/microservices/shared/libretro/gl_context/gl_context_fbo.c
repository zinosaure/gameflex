// LE FBO DU CŒUR ET LA RELECTURE DE SES PIXELS — communs à Linux et Android
// (voir gl_context_internal.h). Le contexte de chaque plateforme naît
// ailleurs ; ce qui se passe une fois qu'il est courant se passe ici.

#include "gl_context_internal.h"

#include <stddef.h>

int gl_context_load_functions(gl_context *ctx) {
#define LOAD(field, name)                                          \
    ctx->field = gl_context_get_proc_address(name);                \
    if (!ctx->field) return -1;
    LOAD(glGenFramebuffers, "glGenFramebuffers")
    LOAD(glBindFramebuffer, "glBindFramebuffer")
    LOAD(glGenRenderbuffers, "glGenRenderbuffers")
    LOAD(glBindRenderbuffer, "glBindRenderbuffer")
    LOAD(glRenderbufferStorage, "glRenderbufferStorage")
    LOAD(glFramebufferRenderbuffer, "glFramebufferRenderbuffer")
    LOAD(glCheckFramebufferStatus, "glCheckFramebufferStatus")
    LOAD(glViewport, "glViewport")
    LOAD(glClearColor, "glClearColor")
    LOAD(glClear, "glClear")
    LOAD(glReadPixels, "glReadPixels")
    LOAD(glFinish, "glFinish")
    LOAD(glGetError, "glGetError")
    LOAD(glDeleteFramebuffers, "glDeleteFramebuffers")
    LOAD(glDeleteRenderbuffers, "glDeleteRenderbuffers")
#undef LOAD
    return 0;
}

void gl_context_delete_framebuffer(gl_context *ctx) {
    if (!ctx->fbo) return;
    ctx->glDeleteFramebuffers(1, &ctx->fbo);
    ctx->glDeleteRenderbuffers(1, &ctx->color_rb);
    ctx->glDeleteRenderbuffers(1, &ctx->depth_rb);
    ctx->fbo = ctx->color_rb = ctx->depth_rb = 0;
}

int gl_context_resize(gl_context *ctx, int width, int height) {
    gl_context_delete_framebuffer(ctx);

    ctx->glGenRenderbuffers(1, &ctx->color_rb);
    ctx->glBindRenderbuffer(GL_RENDERBUFFER, ctx->color_rb);
    ctx->glRenderbufferStorage(GL_RENDERBUFFER, GL_RGBA8, width, height);

    ctx->glGenRenderbuffers(1, &ctx->depth_rb);
    ctx->glBindRenderbuffer(GL_RENDERBUFFER, ctx->depth_rb);
    ctx->glRenderbufferStorage(GL_RENDERBUFFER, GL_DEPTH24_STENCIL8, width, height);

    ctx->glGenFramebuffers(1, &ctx->fbo);
    ctx->glBindFramebuffer(GL_FRAMEBUFFER, ctx->fbo);
    ctx->glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, ctx->color_rb);
    ctx->glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_DEPTH_STENCIL_ATTACHMENT, GL_RENDERBUFFER, ctx->depth_rb);

    if (ctx->glCheckFramebufferStatus(GL_FRAMEBUFFER) != GL_FRAMEBUFFER_COMPLETE) {
        return -1;
    }

    ctx->glViewport(0, 0, width, height);
    ctx->glClearColor(0, 0, 0, 1);
    ctx->glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT | GL_STENCIL_BUFFER_BIT);
    ctx->width = width;
    ctx->height = height;
    return 0;
}

uint32_t gl_context_framebuffer(gl_context *ctx) {
    return ctx->fbo;
}

int gl_context_read_pixels(gl_context *ctx, int width, int height, void *buf) {
    ctx->glBindFramebuffer(GL_FRAMEBUFFER, ctx->fbo);
    ctx->glFinish();

    // VIDER LA FILE D'ERREURS AVANT DE LIRE — SINON ON ACCUSE LA LECTURE
    // D'UNE FAUTE COMMISE PAR LE CŒUR.
    //
    // glGetError() ne dit pas « l'appel précédent a échoué » : il dépile UNE
    // erreur de la file, quelle que soit la commande qui l'y a mise. Un cœur
    // qui laisse une erreur derrière lui — flycast le fait, et il n'est
    // sûrement pas le seul — la faisait donc attribuer à notre glReadPixels,
    // image après image.
    //
    // Effet observé : « hardware frame readback failed » 2667 fois sur une
    // seule session Dreamcast, zéro image encodée, alors que les pixels
    // étaient probablement lisibles (2026-09-08). La file est bornée, on la
    // vide entièrement plutôt qu'une entrée.
    while (ctx->glGetError() != 0) { }

    ctx->glReadPixels(0, 0, width, height, GL_RGBA, GL_UNSIGNED_BYTE, buf);

    // Et on REND le code, au lieu d'un -1 muet : 0x0502 (INVALID_OPERATION,
    // un FBO incomplet) et 0x0501 (INVALID_VALUE, une taille impossible) ne
    // se corrigent pas du tout pareil.
    GLenum err = ctx->glGetError();
    if (err != 0) return (int)err;

    // RGBA -> BGRA in place — see gl_context.h's doc on why: glReadPixels'
    // GL_RGBA is universally supported (unlike GL_BGRA on some GLES
    // drivers), and emitFrame (stream_session.go) already expects BGRA from
    // every OTHER pixel format conversion path.
    GLubyte *p = (GLubyte *)buf;
    size_t count = (size_t)width * (size_t)height;
    for (size_t i = 0; i < count; i++) {
        GLubyte r = p[i * 4 + 0];
        p[i * 4 + 0] = p[i * 4 + 2];
        p[i * 4 + 2] = r;
    }
    return 0;
}
