// witness-mt — un témoin glibc MULTI-THREAD, pour prouver que le traceur
// attrape le SIGSYS de CHAQUE thread. Dans le banc Linux (bench/run.sh) et,
// livré dans glibc-<arch>.zip, dans la sonde de l'application.
//
// Pourquoi c'est le bon test : glibc, à la naissance de chaque thread, appelle
// set_robust_list et enregistre rseq — les deux appels que le seccomp de l'app
// refuse. Le traceur v1 ne suivait que le processus principal ; un thread né
// ensuite serait mort en silence. Si ce témoin imprime « glibc-mt-witness-ok »
// sous v2, c'est que PTRACE_O_TRACECLONE a bien attaché chaque thread à temps.
#define _GNU_SOURCE
#include <stdio.h>
#include <string.h>
#include <pthread.h>
#include <unistd.h>

#define N 6

static void *work(void *arg) {
    long id = (long)arg;
    char buf[32];
    int n = snprintf(buf, sizeof buf, "thread %ld vivant\n", id);
    // un vrai appel après le démarrage du thread : prouve qu'il a dépassé
    // son set_robust_list/rseq de naissance et exécute du code utile.
    write(2, buf, n);
    return NULL;
}

int main(void) {
    pthread_t t[N];
    for (long i = 0; i < N; i++) {
        if (pthread_create(&t[i], NULL, work, (void *)i) != 0) {
            write(2, "pthread_create a échoué\n", 24);
            return 2;
        }
    }
    for (int i = 0; i < N; i++) pthread_join(t[i], NULL);
    write(1, "glibc-mt-witness-ok\n", 20);
    return 0;
}
