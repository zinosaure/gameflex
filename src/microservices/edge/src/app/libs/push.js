import { credential } from "./api.js";

// LES NOTIFICATIONS PUSH DE CET APPAREIL (contexts/push.js de la webapp) : un service worker sur
// l'origine de la page (public/service-worker.js), une clé VAPID demandée au service des comptes,
// un abonnement du navigateur, enregistré pour ce compte (`/push/subscribe`).
//
// LE NAVIGATEUR EXIGE UN CONTEXTE SÛR : https, ou localhost. Sur http://<adresse>:<port>, il n'y a
// ni `navigator.serviceWorker` ni PushManager — `secureContext()` le dit, et le panneau l'affiche
// tel quel plutôt que de faire semblant.

export const secureContext = () => window.isSecureContext === true;
const supported = () => "serviceWorker" in navigator && "PushManager" in window;

// Au démarrage de la page : le service worker, s'il peut exister ici.
export function registerServiceWorker() {
    if (!secureContext() || !("serviceWorker" in navigator)) {
        return;
    }
    navigator.serviceWorker.register("/service-worker.js").catch(() => {
        // Refusé (un navigateur strict) : le panneau dira « indisponible ».
    });
}

function urlBase64ToUint8Array(base64String) {
    const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
    const raw = atob((base64String + padding).replace(/-/g, "+").replace(/_/g, "/"));

    return Uint8Array.from([...raw].map((character) => character.charCodeAt(0)));
}

const sameKey = (buffer, expected) => {
    const bytes = buffer ? new Uint8Array(buffer) : null;

    return Boolean(bytes) && bytes.length === expected.length && bytes.every((byte, index) => byte === expected[index]);
};

// `true` / `false` : abonné ou non ; `"unsupported"` : ni service worker ni push ici.
export async function pushSubscriptionState() {
    if (!supported()) {
        return "unsupported";
    }
    const registration = await navigator.serviceWorker.getRegistration();

    return registration ? Boolean(await registration.pushManager.getSubscription()) : "unsupported";
}

export async function subscribeToPush() {
    if (!supported()) {
        return { ok: false, reason: "unsupported" };
    }

    if (await Notification.requestPermission() !== "granted") {
        return { ok: false, reason: "denied" };
    }
    const key = await credential("/push/vapid-public-key");

    if (!key.ok || !key.item?.publicKey) {
        return { ok: false, reason: "vapid-key-fetch-failed" };
    }

    if (!(await navigator.serviceWorker.getRegistration())) {
        return { ok: false, reason: "unsupported" };
    }
    const registration = await navigator.serviceWorker.ready;
    const applicationServerKey = urlBase64ToUint8Array(key.item.publicKey);
    let subscription = await registration.pushManager.getSubscription();

    // Une clé changée côté serveur : l'ancien abonnement ne recevrait plus rien.
    if (subscription && !sameKey(subscription.options?.applicationServerKey, applicationServerKey)) {
        await subscription.unsubscribe();
        subscription = null;
    }

    if (!subscription) {
        subscription = await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey });
    }
    const saved = await credential("/push/subscribe", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(subscription.toJSON()) });

    if (!saved.ok) {
        await subscription.unsubscribe().catch(() => {});
    }

    return { ok: saved.ok };
}

export async function unsubscribeFromPush() {
    if (!supported()) {
        return { ok: true };
    }
    const registration = await navigator.serviceWorker.getRegistration();
    const subscription = registration ? await registration.pushManager.getSubscription() : null;

    if (subscription) {
        const { endpoint } = subscription;
        await subscription.unsubscribe();
        await credential(`/push/subscribe?endpoint=${encodeURIComponent(endpoint)}`, { method: "DELETE" });
    }

    return { ok: true };
}
