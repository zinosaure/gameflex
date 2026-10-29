// CE QUE LES DEUX CONTEXTES PARTAGENT — celui de Linux (GBM,
// gl_context_gbm.c) et celui d'Android (gl_context_android.c). Seule la
// naissance du contexte diffère d'une plateforme à l'autre : l'affichage EGL,
// la surface, la façon de trouver une fonction GL. Le FBO dans lequel le cœur
// dessine, et la relecture de ses pixels, sont les mêmes
// (gl_context_fbo.c) — écrits une fois, pour que les deux ne divergent pas.
//
// Toujours aucun en-tête GL (voir gl_context.h) : les quelques types,
// constantes et signatures dont le code du FBO a besoin sont ici.
#ifndef RETROGAMING_GLCONTEXT_INTERNAL_H
#define RETROGAMING_GLCONTEXT_INTERNAL_H

#include "gl_context.h"

#include <EGL/egl.h>

typedef unsigned int GLenum;
typedef unsigned int GLuint;
typedef unsigned int GLbitfield;
typedef int GLint;
typedef int GLsizei;
typedef unsigned char GLubyte;

#define GL_FRAMEBUFFER              0x8D40
#define GL_RENDERBUFFER             0x8D41
#define GL_COLOR_ATTACHMENT0        0x8CE0
#define GL_DEPTH_STENCIL_ATTACHMENT 0x821A
#define GL_RGBA8                    0x8058
#define GL_DEPTH24_STENCIL8         0x88F0
#define GL_FRAMEBUFFER_COMPLETE     0x8CD5
#define GL_RGBA                     0x1908
#define GL_UNSIGNED_BYTE            0x1401
#define GL_COLOR_BUFFER_BIT         0x00004000
#define GL_DEPTH_BUFFER_BIT         0x00000100
#define GL_STENCIL_BUFFER_BIT       0x00000400

typedef void (*PFNGENFRAMEBUFFERS)(GLsizei n, GLuint *ids);
typedef void (*PFNBINDFRAMEBUFFER)(GLenum target, GLuint fb);
typedef void (*PFNGENRENDERBUFFERS)(GLsizei n, GLuint *ids);
typedef void (*PFNBINDRENDERBUFFER)(GLenum target, GLuint rb);
typedef void (*PFNRENDERBUFFERSTORAGE)(GLenum target, GLenum internalformat, GLsizei width, GLsizei height);
typedef void (*PFNFRAMEBUFFERRENDERBUFFER)(GLenum target, GLenum attachment, GLenum rbtarget, GLuint rb);
typedef GLenum (*PFNCHECKFRAMEBUFFERSTATUS)(GLenum target);
typedef void (*PFNVIEWPORT)(GLint x, GLint y, GLsizei w, GLsizei h);
typedef void (*PFNCLEARCOLOR)(float r, float g, float b, float a);
typedef void (*PFNCLEAR)(GLbitfield mask);
typedef void (*PFNREADPIXELS)(GLint x, GLint y, GLsizei w, GLsizei h, GLenum format, GLenum type, void *pixels);
typedef void (*PFNFINISH)(void);
typedef GLenum (*PFNGETERROR)(void);
typedef void (*PFNDELETEFRAMEBUFFERS)(GLsizei n, const GLuint *ids);
typedef void (*PFNDELETERENDERBUFFERS)(GLsizei n, const GLuint *ids);

struct gbm_device;

struct gl_context {
#ifdef __ANDROID__
    // Un pbuffer d'un pixel, seulement pour rendre le contexte courant :
    // le cœur dessine dans NOTRE FBO, jamais dans la surface.
    EGLSurface surface;
#else
    int fd;
    struct gbm_device *gbm;
#endif
    EGLDisplay display;
    EGLContext context;

    GLuint fbo, color_rb, depth_rb;
    int width, height;

    PFNGENFRAMEBUFFERS glGenFramebuffers;
    PFNBINDFRAMEBUFFER glBindFramebuffer;
    PFNGENRENDERBUFFERS glGenRenderbuffers;
    PFNBINDRENDERBUFFER glBindRenderbuffer;
    PFNRENDERBUFFERSTORAGE glRenderbufferStorage;
    PFNFRAMEBUFFERRENDERBUFFER glFramebufferRenderbuffer;
    PFNCHECKFRAMEBUFFERSTATUS glCheckFramebufferStatus;
    PFNVIEWPORT glViewport;
    PFNCLEARCOLOR glClearColor;
    PFNCLEAR glClear;
    PFNREADPIXELS glReadPixels;
    PFNFINISH glFinish;
    PFNGETERROR glGetError;
    PFNDELETEFRAMEBUFFERS glDeleteFramebuffers;
    PFNDELETERENDERBUFFERS glDeleteRenderbuffers;
};

// gl_context_load_functions résout les fonctions GL du FBO par
// gl_context_get_proc_address — celle de la plateforme. Le contexte doit être
// courant. Rend 0 si toutes sont là.
int gl_context_load_functions(gl_context *ctx);

// gl_context_delete_framebuffer supprime le FBO et ses deux renderbuffers,
// s'il y en a. Le contexte doit être courant.
void gl_context_delete_framebuffer(gl_context *ctx);

#endif
