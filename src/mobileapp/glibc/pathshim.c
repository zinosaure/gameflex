// pathshim — le LD_PRELOAD des invités glibc : traduit les chemins que
// l'invité demande et qu'Android n'a pas, ou lui refuse.
//
// Un émulateur de bureau suppose un Linux ordinaire : /proc/sys/vm/
// overcommit_memory (RPCS3 le lit pour choisir sa mémoire — EACCES pour une
// app Android, et l'objet fichier nul devient « Null object », 2026-09-13),
// /dev/shm (shm_open), /tmp, et /proc/self/exe qui doit être LUI et non le
// chargeur par lequel on le lance. Le traceur ne voit que les appels REFUSÉS
// par seccomp ; ceux-ci sont permis et échouent plus loin. On les prend donc
// AVANT le noyau, dans la libc : les fonctions à chemin sont interposées, le
// chemin traduit par une table, puis l'appel réel suit.
//
//   GAMEFLEX_PATHMAP="/proc/sys/vm/overcommit_memory=<rt>/overcommit_memory;/dev/shm=<home>/shm;/tmp=<tmp>"
//   GAMEFLEX_EXE="<le binaire de l'invité>"        → readlink("/proc/self/exe")
//
// Préfixes : « /tmp » traduit « /tmp » et « /tmp/x », pas « /tmpfoo ». Le plus
// long préfixe gagne. Un chemin relatif n'est jamais touché. Compilé par
// assemble-glibc.sh dans le runtime (libpathshim.so), pour chaque ABI ; c'est
// la version mince de la traduction de chemins de proot — elle couvre tout ce
// qui passe par la libc, pas un appel système émis à la main.
#define _GNU_SOURCE
#include <dlfcn.h>
#include <fcntl.h>
#include <limits.h>
#include <stdarg.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <dirent.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>

static const char *translate(const char *path, char *out) {
    const char *map = getenv("GAMEFLEX_PATHMAP");
    if (!map || !path || path[0] != '/') return path;
    size_t bestLen = 0;
    const char *bestTo = NULL, *bestToEnd = NULL;
    const char *p = map;
    while (*p) {
        const char *eq = strchr(p, '=');
        if (!eq) break;
        const char *end = strchr(eq, ';');
        if (!end) end = eq + strlen(eq);
        size_t fromLen = (size_t)(eq - p);
        if (fromLen > bestLen && strncmp(path, p, fromLen) == 0
                && (path[fromLen] == '\0' || path[fromLen] == '/')) {
            bestLen = fromLen; bestTo = eq + 1; bestToEnd = end;
        }
        p = *end ? end + 1 : end;
    }
    if (!bestTo) return path;
    size_t toLen = (size_t)(bestToEnd - bestTo);
    if (toLen + strlen(path + bestLen) >= PATH_MAX) return path;
    memcpy(out, bestTo, toLen);
    strcpy(out + toLen, path + bestLen);
    return out;
}

#define REAL(name) \
    static typeof(name) *real_##name; \
    if (!real_##name) real_##name = (typeof(name) *)dlsym(RTLD_NEXT, #name)

int open(const char *path, int flags, ...) {
    mode_t mode = 0;
    if (flags & (O_CREAT | O_TMPFILE)) { va_list ap; va_start(ap, flags); mode = va_arg(ap, mode_t); va_end(ap); }
    char buf[PATH_MAX]; REAL(open);
    return real_open(translate(path, buf), flags, mode);
}
int open64(const char *path, int flags, ...) {
    mode_t mode = 0;
    if (flags & (O_CREAT | O_TMPFILE)) { va_list ap; va_start(ap, flags); mode = va_arg(ap, mode_t); va_end(ap); }
    char buf[PATH_MAX]; REAL(open64);
    return real_open64(translate(path, buf), flags, mode);
}
int openat(int dirfd, const char *path, int flags, ...) {
    mode_t mode = 0;
    if (flags & (O_CREAT | O_TMPFILE)) { va_list ap; va_start(ap, flags); mode = va_arg(ap, mode_t); va_end(ap); }
    char buf[PATH_MAX]; REAL(openat);
    return real_openat(dirfd, translate(path, buf), flags, mode);
}
int openat64(int dirfd, const char *path, int flags, ...) {
    mode_t mode = 0;
    if (flags & (O_CREAT | O_TMPFILE)) { va_list ap; va_start(ap, flags); mode = va_arg(ap, mode_t); va_end(ap); }
    char buf[PATH_MAX]; REAL(openat64);
    return real_openat64(dirfd, translate(path, buf), flags, mode);
}
FILE *fopen(const char *path, const char *mode) {
    char buf[PATH_MAX]; REAL(fopen);
    return real_fopen(translate(path, buf), mode);
}
FILE *fopen64(const char *path, const char *mode) {
    char buf[PATH_MAX]; REAL(fopen64);
    return real_fopen64(translate(path, buf), mode);
}
int stat(const char *path, struct stat *st) {
    char buf[PATH_MAX]; REAL(stat);
    return real_stat(translate(path, buf), st);
}
int lstat(const char *path, struct stat *st) {
    char buf[PATH_MAX]; REAL(lstat);
    return real_lstat(translate(path, buf), st);
}
int stat64(const char *path, struct stat64 *st) {
    char buf[PATH_MAX]; REAL(stat64);
    return real_stat64(translate(path, buf), st);
}
int lstat64(const char *path, struct stat64 *st) {
    char buf[PATH_MAX]; REAL(lstat64);
    return real_lstat64(translate(path, buf), st);
}
int access(const char *path, int mode) {
    char buf[PATH_MAX]; REAL(access);
    return real_access(translate(path, buf), mode);
}
DIR *opendir(const char *path) {
    char buf[PATH_MAX]; REAL(opendir);
    return real_opendir(translate(path, buf));
}
int mkdir(const char *path, mode_t mode) {
    char buf[PATH_MAX]; REAL(mkdir);
    return real_mkdir(translate(path, buf), mode);
}
int unlink(const char *path) {
    char buf[PATH_MAX]; REAL(unlink);
    return real_unlink(translate(path, buf));
}
int rmdir(const char *path) {
    char buf[PATH_MAX]; REAL(rmdir);
    return real_rmdir(translate(path, buf));
}
int chmod(const char *path, mode_t mode) {
    char buf[PATH_MAX]; REAL(chmod);
    return real_chmod(translate(path, buf), mode);
}
int rename(const char *from, const char *to) {
    char a[PATH_MAX], b[PATH_MAX]; REAL(rename);
    return real_rename(translate(from, a), translate(to, b));
}

// LES SOCKETS AUSSI, et elles n'étaient pas couvertes. Un serveur de rendu
// déporté (virgl : « /tmp/.virgl_test », codé en dur dans le pilote de Mesa,
// sans variable d'environnement pour le changer) ou un serveur de son se
// joignent par une socket de domaine UNIX sous /tmp — un dossier qu'Android ne
// donne pas à une app. Le chemin vit alors dans sun_path, jamais dans un
// argument de chemin : la carte ne pouvait rien pour eux, et l'échec était
// muet.
//
// Les sockets ABSTRAITES (sun_path[0] == '\0' — celle de notre écran X) n'ont
// aucun chemin de système de fichiers : on les laisse passer intactes.
static int mapUnix(const struct sockaddr *addr, socklen_t len, struct sockaddr_un *out, socklen_t *outLen) {
    if (!addr || addr->sa_family != AF_UNIX) return 0;
    if (len <= (socklen_t)offsetof(struct sockaddr_un, sun_path)) return 0;
    const struct sockaddr_un *un = (const struct sockaddr_un *)addr;
    if (un->sun_path[0] == '\0') return 0;
    char buf[PATH_MAX];
    const char *to = translate(un->sun_path, buf);
    if (to == un->sun_path || strlen(to) >= sizeof out->sun_path) return 0;
    memset(out, 0, sizeof *out);
    out->sun_family = AF_UNIX;
    strcpy(out->sun_path, to);
    *outLen = (socklen_t)(offsetof(struct sockaddr_un, sun_path) + strlen(to) + 1);
    return 1;
}
int connect(int fd, const struct sockaddr *addr, socklen_t len) {
    struct sockaddr_un mapped; socklen_t mappedLen; REAL(connect);
    if (mapUnix(addr, len, &mapped, &mappedLen)) {
        return real_connect(fd, (const struct sockaddr *)&mapped, mappedLen);
    }
    return real_connect(fd, addr, len);
}
int bind(int fd, const struct sockaddr *addr, socklen_t len) {
    struct sockaddr_un mapped; socklen_t mappedLen; REAL(bind);
    if (mapUnix(addr, len, &mapped, &mappedLen)) {
        return real_bind(fd, (const struct sockaddr *)&mapped, mappedLen);
    }
    return real_bind(fd, addr, len);
}

// /proc/self/exe : l'invité, pas le chargeur. RPCS3 s'en sert pour se
// retrouver (« Found exec path: …/libglibcloader.so », émulateur, 2026-09-13).
static ssize_t selfExe(const char *path, char *out, size_t size) {
    const char *exe = getenv("GAMEFLEX_EXE");
    if (!exe || !path || strcmp(path, "/proc/self/exe") != 0) return -2;
    size_t n = strlen(exe);
    if (n > size) n = size;
    memcpy(out, exe, n);
    return (ssize_t)n;
}
ssize_t readlink(const char *path, char *out, size_t size) {
    ssize_t self = selfExe(path, out, size);
    if (self != -2) return self;
    char buf[PATH_MAX]; REAL(readlink);
    return real_readlink(translate(path, buf), out, size);
}
ssize_t readlinkat(int dirfd, const char *path, char *out, size_t size) {
    ssize_t self = selfExe(path, out, size);
    if (self != -2) return self;
    char buf[PATH_MAX]; REAL(readlinkat);
    return real_readlinkat(dirfd, translate(path, buf), out, size);
}
