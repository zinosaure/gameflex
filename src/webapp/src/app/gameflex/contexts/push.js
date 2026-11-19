import { authApi, authHeaders } from "../../gameflex/profile/auth/AuthContext.jsx";

// Standard conversion required by pushManager.subscribe's
// applicationServerKey (it wants a Uint8Array, VAPID keys travel as
// base64url strings everywhere else — see push/vapid.py).
function urlBase64ToUint8Array(base64String) {
    const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
    const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
    const rawData = atob(base64);
    return Uint8Array.from([...rawData].map((c) => c.charCodeAt(0)));
}

// sameKey — la clé d'un abonnement (ArrayBuffer) est-elle celle-ci ?
function sameKey(buffer, expected) {
    if (!buffer) {
        return false;
    }
    const bytes = new Uint8Array(buffer);

    return bytes.length === expected.length && bytes.every((byte, index) => byte === expected[index]);
}

// pushSubscriptionState — cet appareil reçoit-il les notifications push ?
// "unsupported" quand le navigateur ne le peut pas ici : pas de service worker
// (une adresse http hors localhost, ou la page de développement, qui n'en
// installe pas) ou pas de PushManager. Sinon vrai ou faux, selon l'abonnement
// du navigateur.
export async function pushSubscriptionState() {
    if (!("serviceWorker" in navigator) || !("PushManager" in window)) {
        return "unsupported";
    }
    const registration = await navigator.serviceWorker.getRegistration();
    if (!registration) {
        return "unsupported";
    }

    return Boolean(await registration.pushManager.getSubscription());
}

// Explicit opt-in only (called from a switch in Paramètres) — never
// auto-prompted on login, an unsolicited permission dialog is almost always
// dismissed/annoying and browsers increasingly penalize sites that do this.
export async function subscribeToPush(token) {
    if (!("serviceWorker" in navigator) || !("PushManager" in window)) {
        return { ok: false, reason: "unsupported" };
    }

    const permission = await Notification.requestPermission();
    if (permission !== "granted") {
        return { ok: false, reason: "denied" };
    }

    const keyResult = await authApi("/api/v1/push/vapid-public-key");
    if (!keyResult.ok) {
        return { ok: false, reason: "vapid-key-fetch-failed" };
    }

    // getRegistration() first, THEN .ready: .ready never resolves at all
    // when no service worker is registered, and index.js deliberately
    // unregisters it outside production — so awaiting .ready directly would
    // hang this call forever in dev (switch stuck spinning, no toast, no
    // error). Once a registration does exist, .ready is still the right
    // wait, since a registered-but-not-yet-activated worker can't subscribe.
    if (!(await navigator.serviceWorker.getRegistration())) {
        return { ok: false, reason: "unsupported" };
    }

    const registration = await navigator.serviceWorker.ready;
    const applicationServerKey = urlBase64ToUint8Array(keyResult.item.publicKey);
    let subscription = await registration.pushManager.getSubscription();
    // UN ABONNEMENT PRIS CONTRE UNE AUTRE CLÉ NE REÇOIT RIEN. Le service
    // d'identité tirait la sienne au hasard à chaque démarrage, jusqu'au
    // 2026-10-15 : un abonnement de cette époque se reprend, au lieu de
    // renvoyer au serveur une adresse que le service de poussée refusera.
    if (subscription && !sameKey(subscription.options?.applicationServerKey, applicationServerKey)) {
        await subscription.unsubscribe();
        subscription = null;
    }
    if (!subscription) {
        subscription = await registration.pushManager.subscribe({
            userVisibleOnly: true,
            applicationServerKey,
        });
    }

    const result = await authApi("/api/v1/push/subscribe", {
        method: "POST",
        headers: { "Content-Type": "application/json", ...authHeaders(token) },
        body: JSON.stringify(subscription.toJSON()),
    });
    // LE NAVIGATEUR ET LE SERVEUR DISENT LA MÊME CHOSE. Un abonnement que le
    // serveur n'a pas gardé ne reçoit rien ; le laisser au navigateur ferait
    // afficher « abonné » à l'interrupteur au prochain passage (vu le
    // 2026-10-15, tant que le serveur refusait le corps du navigateur).
    if (!result.ok) {
        await subscription.unsubscribe().catch(() => { });
    }
    return { ok: result.ok };
}

// The other half of subscribeToPush — previously missing entirely, so the
// "Enable browser notifications" switch (Settings.jsx) could only ever
// turn ON (confirmed live: toggling it off just re-ran subscribeToPush,
// which re-granted an already-granted permission and left the switch back
// at "on"). Unsubscribes the BROWSER's own PushManager subscription first
// (so no more pushes are even attempted for this device) and then tells
// the backend to drop the matching row — best-effort on the backend call:
// a browser-side unsubscribe should still count as "off" in the UI even if
// the DELETE fails (a stale row just means one dead subscription
// send_push_to_user already self-heals on its own 404/410, see push/
// routes.py).
export async function unsubscribeFromPush(token) {
    if (!("serviceWorker" in navigator) || !("PushManager" in window)) {
        return { ok: true };
    }

    // getRegistration(), not .ready — see subscribeToPush's note above. This
    // path especially must never hang: it's the "turn it back off" half, so
    // hanging here would recreate the very bug it exists to fix.
    const registration = await navigator.serviceWorker.getRegistration();
    if (!registration) {
        return { ok: true };
    }

    const subscription = await registration.pushManager.getSubscription();
    if (!subscription) {
        return { ok: true };
    }

    const endpoint = subscription.endpoint;
    await subscription.unsubscribe();
    await authApi(`/api/v1/push/subscribe?endpoint=${encodeURIComponent(endpoint)}`, {
        method: "DELETE",
        headers: authHeaders(token),
    });
    return { ok: true };
}
