// witness — le témoin glibc minimal, le même pour toutes les architectures.
//
// Si ces deux mots s'écrivent, c'est que le chargeur a projeté libc.so.6, que
// libc s'est initialisée (TLS, locale, stdio) et que main a tourné — tout ce
// que le seccomp de l'application tuait avant le traceur. Il remplace le
// `getent` de Debian qui servait de témoin en aarch64 : lancé sans argument il
// rendait 1 par sa propre logique, un « succès » qu'il fallait savoir lire.
#include <unistd.h>

int main(void) {
    write(1, "glibc-witness-ok\n", 17);
    return 0;
}
