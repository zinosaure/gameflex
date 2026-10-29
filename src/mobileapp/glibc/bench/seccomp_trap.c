// seccomp-trap — la pièce maîtresse du banc Linux du traceur (bench/run.sh).
//
// Installe SUR LUI-MÊME un filtre seccomp qui rend SIGSYS (SECCOMP_RET_TRAP)
// pour les appels que le filtre d'application d'Android x86_64 refuse — ceux
// qu'on a mesurés (poll, access, set_robust_list, rseq, clone3) et la famille
// héritée qu'il refuse à coup sûr — puis exécute le programme donné. Le filtre
// survit à l'exec, comme celui du zygote : sous Linux, on reproduit ainsi le
// mur de l'émulateur SANS l'émulateur, et chaque essai du traceur prend une
// seconde au lieu d'un tour d'APK.
//
//   ./ptracer ./seccomp-trap ./draw-xcb /tmp/.X11-unix/X7
//
// Contrôle négatif : sans ptracer, le même programme doit mourir en 159.
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <stddef.h>
#include <unistd.h>
#include <sys/prctl.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <linux/audit.h>

// x86_64 : poll access open stat lstat pipe dup2 select rename mkdir rmdir
// unlink readlink chmod epoll_wait — puis set_robust_list rseq clone3, et
// setresuid setresgid (refusés à une app : PulseAudio y mourait, 2026-10-02).
static const int BLOCKED[] = { 7, 21, 2, 4, 6, 22, 33, 23, 82, 83, 84, 87, 89, 90, 232,
                               273, 334, 435, 117, 119 };

int main(int argc, char **argv) {
    if (argc < 2) { fprintf(stderr, "usage: seccomp-trap <prog> [args...]\n"); return 2; }
    int n = sizeof BLOCKED / sizeof BLOCKED[0];
    struct sock_filter *f = calloc(2 * n + 6, sizeof *f);
    int k = 0;
    // Pas d'autre architecture que la nôtre : un filtre par numéro n'a de sens
    // que pour une table d'appels donnée.
    f[k++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(struct seccomp_data, arch));
    f[k++] = (struct sock_filter)BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, AUDIT_ARCH_X86_64, 1, 0);
    f[k++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_KILL_PROCESS);
    f[k++] = (struct sock_filter)BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(struct seccomp_data, nr));
    for (int i = 0; i < n; i++) {
        f[k++] = (struct sock_filter)BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, BLOCKED[i], 0, 1);
        f[k++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_TRAP);
    }
    f[k++] = (struct sock_filter)BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ALLOW);
    struct sock_fprog prog = { .len = (unsigned short)k, .filter = f };

    if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)) { perror("no_new_privs"); return 3; }
    if (prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &prog)) { perror("seccomp"); return 4; }
    execv(argv[1], &argv[1]);
    perror("execv");
    return 127;
}
