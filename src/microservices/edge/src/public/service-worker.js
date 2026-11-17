// LE SERVICE WORKER D'EDGE : les notifications push, et rien d'autre — pas de cache hors ligne (chantier 56, A8) : hors
// réseau, rien ne se joue dans un navigateur, et un cache figerait un ancien paquet. Une invitation (`type: invite`)
// s'affiche ; la toucher ouvre la partie sur la page de jeu (/play/<id>, chantier 52), qui sait la rejoindre — comme
// l'invitation d'En direct (Live.jsx).

self.addEventListener('install', () => self.skipWaiting());

// LES CACHES DE L'ANCIENNE INTERFACE (`gameflex-v<n>`, le service worker de la webapp) : edge prend son origine à la
// bascule (chantier 56, P5) — l'adresse du serveur, la WebView de l'application. En prenant sa
// place, celui-ci les efface : plus rien ne les lit, et un `bundle.js` d'avant n'a rien à y attendre.
self.addEventListener('activate', (event) => event.waitUntil(
    caches.keys()
        .then((keys) => Promise.all(keys.filter((key) => key.startsWith('gameflex-v')).map((key) => caches.delete(key))))
        .then(() => self.clients.claim()),
));

self.addEventListener('push', (event) => {
    let payload = {};

    try {
        payload = event.data ? event.data.json() : {};
    } catch {
        return;
    }

    if (payload.type !== 'invite') {
        return;
    }
    const from = payload.from?.displayName || 'Someone';
    event.waitUntil(self.registration.showNotification(`${from} vous invite`, {
        body: payload.reference,
        icon: '/images/icon-192.png',
        data: { url: `${self.location.origin}/play/${encodeURIComponent(payload.sessionId)}` },
    }));
});

self.addEventListener('notificationclick', (event) => {
    event.notification.close();
    const url = event.notification.data?.url;

    if (url) {
        event.waitUntil(self.clients.openWindow(url));
    }
});
