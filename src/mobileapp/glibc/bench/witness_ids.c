// Le témoin des identifiants : « abandonner ses privilèges » pour les MÊMES
// identifiants, comme PulseAudio le fait au démarrage (pa_drop_root). Sous le
// filtre d'Android, setresuid/setresgid sont refusés ; le traceur doit
// répondre « fait » dans ce cas précis (voir identity() dans ptracer.c).
//
// Sortie : « ids-ok » si les deux appels rendent 0, sinon le premier échec.
#define _GNU_SOURCE
#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <unistd.h>

int main(void) {
    gid_t gid = getgid();
    uid_t uid = getuid();
    if (setresgid(gid, gid, gid) != 0) {
        printf("setresgid : %s\n", strerror(errno));
        return 1;
    }
    if (setresuid(uid, uid, uid) != 0) {
        printf("setresuid : %s\n", strerror(errno));
        return 1;
    }
    printf("ids-ok\n");
    return 0;
}
