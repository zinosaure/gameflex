// Google Cast SENDER integration (Phase B2 of the "Mode Console +
// Chromecast WebRTC-natif" plan) — the OTHER half of public/cast-
// receiver.html (Phase B1, see that file's own top-of-file doc for why it's
// a separate static page, not part of this app's bundle). This module is
// plain JS, not a hook: the Cast Sender SDK is a global script (`window.cast`/
// `window.chrome.cast`), loaded lazily on first use rather than bundled, the
// same way every real Cast-integrated site does it.
//
// CUSTOM_NAMESPACE must match cast-receiver.html's own constant exactly —
// Cast's custom message channel is namespaced per string, a mismatch just
// silently never delivers anything.
const CUSTOM_NAMESPACE = "urn:x-cast:com.gameflex.session";

// LES ERREURS DU CAST SE TRADUISENT (la page de la partie, la page de cast) : un code, jamais un texte écrit ici —
// `unavailable` (ce navigateur ne caste pas), `noReceiver` (aucun récepteur réglé), `cancelled` (le sélecteur refermé
// sans choisir : pas une erreur à montrer), `failed` (le reste ; `detail`, ce que dit le SDK, pour la console).
function castError(code, detail = "") {
    return Object.assign(new Error(detail || code), { code, cancelled: code === "cancelled" });
}

export const castErrorKey = (error) => ({ unavailable: "play.castUnavailable", noReceiver: "play.castNoReceiver" })[error?.code] || "play.castFailed";

// L'IDENTIFIANT DU RÉCEPTEUR VIENT DE L'ADMINISTRATION (le porteur, 2026-11-24 : « ça devrait être dans
// l'administration directement et non plus dans le launch.sh ») : Réglages › Google Cast, gardé par le service des
// comptes (`GET /api/v1/cast`). La page de la partie le lit et le passe à requestCastSession ; sans lui, elle ne
// propose pas le cast.

// loadCastFramework=1 is required — without it the script only attaches the
// legacy window.chrome.cast namespace, never window.cast.framework (the CAF
// API this whole module is built on). Confirmed via real diagnostics: this
// exact omission produced isAvailable=true, hasChromeCast=true, but
// hasCastNamespace=false, indefinitely (not a timing gap — polling up to
// 15s never helped, because the framework layer was simply never requested).
const SENDER_SDK_URL = "https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1";

let sdkLoadPromise = null;

// Loads the Cast Sender SDK exactly once per page — the SDK calls a global
// callback (window['__onGCastApiAvailable']) once it's ready, rather than
// the script tag's own onload (the SDK does async internal setup after the
// script itself finishes loading).
//
// The "already loaded" check must test window.cast.framework specifically
// (what getCastContext below actually calls) — window.chrome.cast is a
// SEPARATE, older namespace the SDK also populates, but checking IT here
// was wrong: it can read truthy without cast.framework ever having loaded,
// which short-circuited past the script injection entirely and left
// `cast` undefined at the getCastContext call site ("cast is not defined").
function loadCastSdk() {
    if (window.cast?.framework?.CastContext) return Promise.resolve();
    if (sdkLoadPromise) return sdkLoadPromise;

    if (!window.chrome) {
        return Promise.reject(castError("unavailable"));
    }

    sdkLoadPromise = new Promise((resolve, reject) => {
        // __onGCastApiAvailable(true) can still fire a moment before
        // window.cast.framework.CastContext is actually attached even with
        // loadCastFramework=1 (SENDER_SDK_URL above) — poll briefly instead
        // of rejecting on the very first check. 3s/30 attempts is generous;
        // the indefinite hang seen before that fix was the missing
        // loadCastFramework=1 param, not a real timing gap.
        const waitForFramework = (attemptsLeft) => {
            if (window.cast?.framework?.CastContext) {
                resolve();
                return;
            }
            if (attemptsLeft <= 0) {
                sdkLoadPromise = null;
                reject(castError("unavailable"));
                return;
            }
            setTimeout(() => waitForFramework(attemptsLeft - 1), 100);
        };

        window["__onGCastApiAvailable"] = (isAvailable) => {
            if (isAvailable) {
                waitForFramework(30);
            } else {
                sdkLoadPromise = null;
                reject(castError("unavailable"));
            }
        };
        const script = document.createElement("script");
        script.src = SENDER_SDK_URL;
        script.onerror = () => {
            sdkLoadPromise = null;
            reject(castError("unavailable"));
        };
        document.head.appendChild(script);
    });
    return sdkLoadPromise;
}

function getCastContext(applicationId) {
    const context = cast.framework.CastContext.getInstance();
    // AUTO_ACCEPT_SESSION_ID lets Chrome resume a session already running
    // on a device without a fresh user gesture on THIS page — harmless
    // default here, no media-namespace autoplay concerns since our receiver
    // doesn't use PlayerManager at all.
    context.setOptions({
        receiverApplicationId: applicationId,
        autoJoinPolicy: chrome.cast.AutoJoinPolicy.ORIGIN_SCOPED,
    });
    return context;
}

// requestCastSession — call from a real user click (browsers require a
// gesture to open the Cast device picker). Resolves once a Cast session is
// actually established.
async function requestCastSession(applicationId) {
    if (!applicationId) {
        throw castError("noReceiver");
    }
    await loadCastSdk();
    const context = getCastContext(applicationId);
    try {
        await context.requestSession();
    } catch (error) {
        throw castError(error === "cancel" ? "cancelled" : "failed", String(error));
    }
    return context.getCurrentSession();
}

// Sends {sessionId} to the receiver over the custom message channel — the
// ONLY thing the receiver needs to know to attach its own spectate WebRTC
// connection (see cast-receiver.html's attachToSession).
//
// session.sendMessage returns a Promise — must be awaited: fire-and-forget
// here previously swallowed delivery failures silently (confirmed via real
// testing — the receiver launched and the sender reported success, but the
// message never arrived, with zero error surfaced anywhere).
async function sendSessionIdToReceiver(sessionId) {
    const session = cast.framework.CastContext.getInstance().getCurrentSession();
    if (!session) throw castError("failed");
    await session.sendMessage(CUSTOM_NAMESPACE, { sessionId });
}

function currentWebSession() {
    if (!window.cast?.framework) return null;
    return cast.framework.CastContext.getInstance().getCurrentSession();
}

// DEUX ENVOYEURS, UNE SEULE FAÇON DE S'EN SERVIR (#124 ; le porteur, 2026-11-26 : « le cast to tv c'est toutes les
// plateformes qui utilise ») — la page de la partie prend celui que l'hôte déclare (`castLink`) et ne connaît que ceci :
//   - `start(applicationId, sessionId)` : choisir la TV (une première fois), puis lui dire la partie ; une promesse ;
//   - `stop()` : arrêter le cast ;
//   - `casting()` : l'état connu tout de suite ;
//   - `watch(onChange)` : l'état quand il change hors de la page (la TV arrêtée depuis la notification du téléphone) ;
//     rend de quoi se désabonner.

// LE SDK CAST DE CHROME : tout navigateur Chrome, quel que soit l'hôte.
const webCaster = {
    async start(applicationId, sessionId) {
        if (!currentWebSession()) {
            await requestCastSession(applicationId);
        }
        await sendSessionIdToReceiver(sessionId);
    },
    stop() {
        if (currentWebSession()) {
            cast.framework.CastContext.getInstance().endCurrentSession(true);
        }
    },
    casting: () => Boolean(currentWebSession()),
    watch: () => () => {},
};

// LES FENÊTRES DE L'APPLICATION, qui n'ont pas le SDK de Chrome : la page leur parle par le lien réservé `gameflex://cast`,
// qu'elles interceptent — comme `gameflex://home`, il marche aussi chez un serveur rejoint. La WebView du téléphone
// caste par Android (TvCast.java) ; la fenêtre du PC ouvre la page de cast de l'hôte dans le navigateur du système
// (window/main.js, screens/access/Cast.jsx) et répond aussitôt qu'ici rien ne caste :
//   - `gameflex://cast?application=<id>&session=<id>` : le sélecteur de TV d'Android, puis la partie au récepteur ;
//   - `gameflex://cast?stop=1` ; `gameflex://cast?state=1` : redire l'état.
// L'application répond par l'évènement `gameflex-cast` de la fenêtre : `{ casting, error, cancelled }` — `cancelled`, le
// sélecteur refermé sans choisir de TV.
const APP_EVENT = "gameflex-cast";
const APP_LINK = "gameflex://cast";
let appCasting = false;

function askApp(query) {
    window.location.href = `${APP_LINK}?${new URLSearchParams(query)}`;
}

const appCaster = {
    start(applicationId, sessionId) {
        return new Promise((resolve, reject) => {
            const answered = (event) => {
                window.removeEventListener(APP_EVENT, answered);
                appCasting = Boolean(event.detail?.casting);
                if (appCasting) {
                    resolve();
                } else {
                    reject(castError(event.detail?.cancelled ? "cancelled" : "failed", event.detail?.error));
                }
            };
            window.addEventListener(APP_EVENT, answered);
            askApp({ application: applicationId, session: sessionId });
        });
    },
    stop() {
        askApp({ stop: "1" });
    },
    casting: () => appCasting,
    watch(onChange) {
        const changed = (event) => {
            appCasting = Boolean(event.detail?.casting);
            onChange(appCasting);
        };
        window.addEventListener(APP_EVENT, changed);
        askApp({ state: "1" });

        return () => window.removeEventListener(APP_EVENT, changed);
    },
};

export const castSender = (throughApp) => (throughApp ? appCaster : webCaster);
