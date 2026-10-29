import { can } from "../../libs/capabilities.js";
import { apiUrl } from "../../libs/serverBase.js";
// Sharing a session link — clipboard and native share, both of which are
// gated behind a SECURE CONTEXT and therefore absent on this app's most
// common origin.
//
// GameFlex is routinely reached over plain HTTP on the LAN
// (http://<host>:<port>), where `navigator.clipboard` and `navigator.share`
// are simply undefined. The previous copy button did
// `navigator.clipboard?.writeText(...).then(toast)` — on HTTP the optional
// chain short-circuited, so the button did nothing at all AND said nothing,
// which reads as a broken app rather than an unavailable capability.
// Everything here therefore reports success/failure instead of failing quiet.

// Copies text, falling back to the legacy execCommand path when the async
// Clipboard API is unavailable (plain-HTTP origin) or refused (permission).
// Returns true only when the text actually made it to the clipboard.
export async function copyTextToClipboard(text) {
    if (navigator.clipboard && window.isSecureContext) {
        try {
            await navigator.clipboard.writeText(text);

            return true;
        } catch {
            // Permission denied or a non-focused document — fall through to
            // the legacy path rather than giving up.
        }
    }

    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "");
    textarea.style.position = "fixed";
    textarea.style.top = "0";
    textarea.style.opacity = "0";
    // The streaming page is usually FULLSCREEN: a node appended to
    // document.body sits outside the fullscreen element, so it can neither
    // be focused nor selected and the copy silently fails. Append inside
    // whatever element currently owns the screen.
    const host = document.fullscreenElement || document.body;
    host.appendChild(textarea);

    try {
        textarea.select();
        // iOS Safari ignores select() on a readonly textarea.
        textarea.setSelectionRange(0, text.length);

        return document.execCommand("copy");
    } catch {
        return false;
    } finally {
        host.removeChild(textarea);
    }
}

// True when the OS share sheet is available (mobile, secure context). Used to
// decide whether to OFFER the action at all — a "Send link" button that can
// never work is worse than no button.
export function canShareLink() {
    // L'HÔTE DÉCLARE S'IL SAIT PARTAGER (C8-P5).
    //
    // Un WebView n'implémente pas `navigator.share` : le bouton disparaissait
    // donc exactement là où partager un lien est le geste le plus naturel
    // (signalé le 2026-09-02). L'application, elle, a une feuille de partage
    // depuis toujours — il suffit de la lui demander.
    //
    // `nativeShare` plutôt qu'un test de plateforme : la question est « cet
    // hôte a-t-il une feuille de partage ? », pas « est-ce Android ? ».
    return can("nativeShare")
        || (typeof navigator.share === "function" && window.isSecureContext);
}

// Opens the native share sheet. Returns false if unavailable or dismissed,
// so the caller can fall back to copying.
export async function shareLink({ url, title, text }) {
    if (can("nativeShare")) {
        try {
            const response = await fetch(apiUrl("/api/v1/device/share"), {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ url, title, text }),
            });

            return response.ok;
        } catch {
            return false;
        }
    }

    if (!canShareLink()) {
        return false;
    }

    try {
        await navigator.share({ url, title, text });

        return true;
    } catch {
        // AbortError (user dismissed the sheet) lands here too — same
        // outcome for the caller: nothing was shared.
        return false;
    }
}
