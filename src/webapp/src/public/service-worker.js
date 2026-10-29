/**
 * Service Worker minimal pour le mode hors-ligne (PWA).
 *
 * Stratégie : Cache-First pour les assets statiques,
 * Network-First pour les requêtes API.
 */

// Named for THIS app, not the template it came from: caches are keyed by name
// and the activate handler below deletes every key that is not the current one.
// A rename is therefore also the migration — the old skeleton-* caches are
// swept on the first activation after this ships.
// À INCRÉMENTER à chaque release qui change le bundle : le navigateur ne
// réinstalle le service worker que sur un octet de différence. Vécu le
// 2026-08-29 : la version navigateur du PWA restait sur l'écran de connexion
// pendant que l'application installée servait déjà le bon bundle.
const SW_VERSION = "5";

const CACHE_NAME = `gameflex-v${SW_VERSION}`;
const STATIC_ASSETS = [
    '/',
    '/bundle.js',
    '/manifest.json',
];

// ─── Installation : pré-cache des assets statiques ──────
self.addEventListener('install', (event) => {
    console.log('[SW] Installing...');

    event.waitUntil(
        caches.open(CACHE_NAME).then((cache) => {
            console.log('[SW] Caching static assets');

            return cache.addAll(STATIC_ASSETS);
        }).then(() => {
            console.log('[SW] Installed, skipping wait');

            return self.skipWaiting();
        })
    );
});

// ─── Activation : nettoyage des anciens caches ──────────
self.addEventListener('activate', (event) => {
    console.log('[SW] Activating...');
    event.waitUntil(
        caches.keys().then((keys) => {
            return Promise.all(
                keys.filter((key) => key !== CACHE_NAME).map((key) => caches.delete(key))
            );
        }).then(() => {
            console.log('[SW] Activated, claiming clients');
            return self.clients.claim();
        })
    );
});

// ─── Push : invitations (voir le service auth, /api/v1/push/*) ───────────
// Only the "app closed/backgrounded" safety net — a WS-connected tab gets
// the same {type:"invite",...} payload instantly over /api/v1/presence/ws
// (see PresenceContext.jsx), never through this path.
//
// Touching this file forces every client's service worker to reinstall
// (byte-diff check on navigation), which rebuilds the pre-cache — the only
// way a cache-first asset strategy ever picks up a new bundle.
self.addEventListener('push', (event) => {
    let payload = {};
    try {
        payload = event.data ? event.data.json() : {};
    } catch {
        return;
    }
    if (payload.type !== 'invite') return;

    const from = payload.from?.displayName || 'Someone';
    // LA MÊME ADRESSE QUE LA CLOCHE (index.js, buildStreamingUrl) : une partie
    // se désigne par son seul identifiant, quelle que soit sa plateforme, et
    // la page décide qui regarde ou joue. Les adresses d'avant
    // (/retrogaming/streaming?…, /remoteplay/watch?…) ne menaient plus nulle
    // part (2026-10-15).
    const url = `/streaming?sessionId=${encodeURIComponent(payload.sessionId)}`;

    event.waitUntil(
        self.registration.showNotification(`${from} vous invite`, {
            body: payload.reference,
            icon: '/images/icon-192.png',
            data: { url },
        })
    );
});

self.addEventListener('notificationclick', (event) => {
    event.notification.close();
    const url = event.notification.data?.url;
    if (!url) return;

    event.waitUntil(
        self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
            for (const client of clients) {
                if (new URL(client.url).pathname === new URL(url, self.location.origin).pathname && 'focus' in client) {
                    return client.focus();
                }
            }
            return self.clients.openWindow(url);
        })
    );
});

// ─── Fetch : stratégie hybride ──────────────────────────
self.addEventListener('fetch', (event) => {
    const { request } = event;
    const url = new URL(request.url);

    // API requests → Network-First (pas de cache)
    if (url.pathname.startsWith('/api/')) {
        return; // Laisser le navigateur gérer normalement
    }

    // Navigations et bundle non versionné → Network-First, cache en repli.
    //
    // Ces deux-là sont les seuls fichiers qui changent SANS changer de nom :
    // en cache-first, un client peut les servir périmés indéfiniment tant que
    // le service worker lui-même n'a pas changé d'octet. Vécu le 2026-08-29 :
    // le PWA du téléphone montrait encore l'écran de connexion après la mise
    // à jour de l'APK, parce que le navigateur continuait de servir l'ancien
    // bundle depuis son pré-cache. En network-first, le prochain chargement
    // prend le bundle frais — et hors ligne, le cache répond quand même.
    // Les autres statiques (chunks, polices, images) restent en cache-first :
    // leur nom change avec leur contenu. Ce n'était PAS vrai des chunks
    // jusqu'au 2026-09-01 — webpack les nommait « 265.bundle.js » et
    // réutilisait l'identifiant d'un build à l'autre, donc cette branche
    // servait éternellement l'écran d'une version précédente. Corrigé à la
    // source (webpack.config.js, output.chunkFilename porte l'empreinte) :
    // la supposition ci-dessus est enfin exacte.
    const isNavigation = request.mode === 'navigate';
    const isUnversionedBundle = url.pathname === '/bundle.js';

    if (isNavigation || isUnversionedBundle) {
        event.respondWith(
            fetch(request).then((response) => {
                if (response.ok) {
                    const clone = response.clone();

                    caches.open(CACHE_NAME).then((cache) => {
                        cache.put(request, clone);
                    });
                }

                return response;
            }).catch(() =>
                caches.match(request).then((cached) => cached || caches.match('/'))
            )
        );

        return;
    }

    // Static assets → Cache-First, avec fallback réseau
    event.respondWith(
        caches.match(request).then((cached) => {
            if (cached) {
                return cached;
            }

            return fetch(request).then((response) => {
                // Ne cache que les réponses valides GET
                if (request.method === 'GET' && response.status === 200) {
                    const clone = response.clone();

                    caches.open(CACHE_NAME).then((cache) => {
                        cache.put(request, clone);
                    });
                }
                return response;
            }).catch(() => {
                // Fallback hors-ligne pour les pages HTML
                if (request.destination === 'document') {
                    return caches.match('/');
                }

                // Pour les autres ressources, renvoyer une erreur muette
                return new Response('Offline', { status: 503 });
            });
        })
    );
});
