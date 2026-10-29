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

// Sourced from GAMEFLEX_GOOGLECAST_APPLICATION_ID (credentials.sh -> launch.sh ->
// docker-compose.yaml's gameflex-webapp service -> webpack.config.js's
// DefinePlugin — same chain GITHUB_COMMIT_SHA already uses), filled
// in once the receiver app is registered in the Google Cast SDK Developer
// Console (manual step, ~5$ one-time fee — see the plan's own doc; not
// something this code can do). Empty/missing is treated the same as
// "not registered yet": requestCastSession below fails fast with a clear
// error instead of silently trying to launch a receiver that doesn't exist.
const RECEIVER_APP_ID = process.env.GOOGLECAST_APPLICATION_ID || "";

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
        return Promise.reject(new Error("Le Cast n'est disponible que sur un navigateur Chrome/Chromium."));
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
                reject(new Error("Le SDK Cast n'est pas disponible dans ce navigateur — teste sur Google Chrome avec l'icône Cast présente dans la barre d'outils."));
                return;
            }
            setTimeout(() => waitForFramework(attemptsLeft - 1), 100);
        };

        window["__onGCastApiAvailable"] = (isAvailable) => {
            if (isAvailable) {
                waitForFramework(30);
            } else {
                sdkLoadPromise = null;
                reject(new Error("Le SDK Cast n'est pas disponible dans ce navigateur — teste sur Google Chrome avec l'icône Cast présente dans la barre d'outils."));
            }
        };
        const script = document.createElement("script");
        script.src = SENDER_SDK_URL;
        script.onerror = () => {
            sdkLoadPromise = null;
            reject(new Error("Failed to load Cast SDK"));
        };
        document.head.appendChild(script);
    });
    return sdkLoadPromise;
}

function getCastContext() {
    const context = cast.framework.CastContext.getInstance();
    // AUTO_ACCEPT_SESSION_ID lets Chrome resume a session already running
    // on a device without a fresh user gesture on THIS page — harmless
    // default here, no media-namespace autoplay concerns since our receiver
    // doesn't use PlayerManager at all.
    context.setOptions({
        receiverApplicationId: RECEIVER_APP_ID,
        autoJoinPolicy: chrome.cast.AutoJoinPolicy.ORIGIN_SCOPED,
    });
    return context;
}

// requestCastSession — call from a real user click (browsers require a
// gesture to open the Cast device picker). Resolves once a Cast session is
// actually established; the caller still has to call sendSessionId itself
// (kept separate so re-sending on e.g. a session change is just another
// call, not a fresh cast-session negotiation).
export async function requestCastSession() {
    if (!RECEIVER_APP_ID) {
        throw new Error("Cast receiver app not registered yet — set GAMEFLEX_GOOGLECAST_APPLICATION_ID in credentials.sh (see gameflex/stream/onscreen/cast.js's RECEIVER_APP_ID doc).");
    }
    await loadCastSdk();
    const context = getCastContext();
    await context.requestSession();
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
export async function sendSessionIdToReceiver(sessionId) {
    const context = cast.framework.CastContext.getInstance();
    const session = context.getCurrentSession();
    if (!session) throw new Error("No active Cast session.");
    await session.sendMessage(CUSTOM_NAMESPACE, { sessionId });
}

export function getCurrentCastSession() {
    if (!window.cast?.framework) return null;
    return cast.framework.CastContext.getInstance().getCurrentSession();
}

export function endCastSession() {
    const context = cast.framework.CastContext.getInstance();
    context.endCurrentSession(true);
}
