// draw-xcb — un client X GLIBC minimal qui dessine sur l'écran Xvfb DÉJÀ
// streamé, pour fermer la boucle glibc → X → capture → encodage → navigateur.
// Il sert deux fois : dans le banc Linux (bench/run.sh, contre un Xvfb Debian)
// et, livré dans glibc-<arch>.zip, dans la sonde de l'application — où le
// pixel central de la capture doit porter une de ses quatre couleurs.
//
// Pourquoi il ouvre la socket lui-même : libX11/libxcb résolvent :0 vers
// /tmp/.X11-unix/X0, un chemin codé en dur qui n'existe pas sur Android (la
// socket de l'app vit sous son propre dossier de données, chemin relocalisé).
// On ouvre donc l'AF_UNIX nous-mêmes (chemin en argv[1]) et on la donne à
// xcb_connect_to_fd — exactement ce que fait déjà /shared/x11grab côté capture.
//
// Il remplit la racine de couleurs qui changent : la capture (GetImage sur la
// racine) les voit, donc si l'image bouge dans le navigateur, un binaire glibc
// dessine bien à travers toute la chaîne.
//
// Compilé en Debian 12 (glibc 2.36, identique au bundle). Dépendances visées :
// libxcb + libXau + libXdmcp + libc — une poignée de .so, ~200 Ko.
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <stddef.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <xcb/xcb.h>

int main(int argc, char **argv) {
    if (argc < 2) { fprintf(stderr, "usage: draw-xcb <chemin-socket-x11>\n"); return 2; }

    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) { perror("socket"); return 3; }
    struct sockaddr_un addr;
    memset(&addr, 0, sizeof addr);
    addr.sun_family = AF_UNIX;
    socklen_t len = sizeof addr;
    if (argv[1][0] == '@') {
        // Espace de noms ABSTRAIT (Linux) : un octet nul, puis le nom — sans
        // fichier. C'est le nom standard de l'écran dans l'application
        // (@/tmp/.X11-unix/X0, voir Xserver.socketName), là où le banc Linux
        // passe un chemin.
        addr.sun_path[0] = '\0';
        strncpy(addr.sun_path + 1, argv[1] + 1, sizeof addr.sun_path - 2);
        len = (socklen_t)(offsetof(struct sockaddr_un, sun_path) + 1 + strlen(argv[1] + 1));
    } else {
        strncpy(addr.sun_path, argv[1], sizeof addr.sun_path - 1);
    }
    if (connect(fd, (struct sockaddr *)&addr, len) < 0) { perror("connect"); return 4; }

    xcb_connection_t *c = xcb_connect_to_fd(fd, NULL);
    if (xcb_connection_has_error(c)) { fprintf(stderr, "xcb_connect a échoué\n"); return 5; }

    const xcb_setup_t *setup = xcb_get_setup(c);
    xcb_screen_t *screen = xcb_setup_roots_iterator(setup).data;
    xcb_window_t root = screen->root;
    uint16_t w = screen->width_in_pixels, h = screen->height_in_pixels;

    xcb_gcontext_t gc = xcb_generate_id(c);
    uint32_t val[1] = { 0 };
    xcb_create_gc(c, gc, root, XCB_GC_FOREGROUND, val);

    fprintf(stderr, "draw-xcb: écran %ux%u, racine 0x%x — je dessine\n", w, h, root);
    uint32_t colors[] = { 0xC81E1E, 0x1EC81E, 0x1E1EC8, 0xC8C81E };
    for (int i = 0; i < 240; i++) {                 // ~8 s à 30 img/s
        val[0] = colors[i & 3];
        xcb_change_gc(c, gc, XCB_GC_FOREGROUND, val);
        xcb_rectangle_t r = { 0, 0, w, h };
        xcb_poly_fill_rectangle(c, root, gc, 1, &r);
        xcb_flush(c);
        usleep(33000);
    }
    fprintf(stdout, "draw-xcb-ok\n");
    xcb_disconnect(c);
    return 0;
}
