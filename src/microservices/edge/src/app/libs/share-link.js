import qrcode from "qrcode-generator";
import { can } from "./capabilities.js";
import { homeOrigin } from "./serverBase.js";
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

// OUVRIR LA FEUILLE DE PARTAGE. Rend "sent" (elle s'est ouverte), "cancelled" (la personne l'a refermée sans rien
// choisir : rien d'autre à faire) ou "failed" (elle n'a pas pu s'ouvrir : l'appelant copie le lien à la place — un geste
// qui ne fait rien et ne dit rien passe pour une panne).
export async function shareLink({ url, title, text }) {
    if (can("nativeShare")) {
        try {
            // LA FEUILLE DE PARTAGE EST CELLE DE CET APPAREIL : la demande va à SON kernel (`homeOrigin`), jamais à l'hôte
            // que vise la page de jeu (`apiUrl` suit `setStreamBase`). Une partie jouée au téléphone chez un serveur fédéré
            // l'envoyait au serveur, qui n'a pas cette route (404) : « Envoyer le lien » ne faisait rien (le porteur,
            // 2026-11-16 : « sur mobileapp, le lien partager le lien ne fonctionne pas »).
            const response = await fetch(`${homeOrigin()}/api/v1/device/share`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ url, title, text }),
            });

            return response.ok ? "sent" : "failed";
        } catch {
            return "failed";
        }
    }

    if (!canShareLink()) {
        return "failed";
    }

    try {
        await navigator.share({ url, title, text });

        return "sent";
    } catch (error) {
        return error?.name === "AbortError" ? "cancelled" : "failed";
    }
}

// LE QR CODE D'UN LIEN (le porteur, 2026-11-14 : « Peut on rajouter le QR code qui permet de rejoindre la session ? ») :
// une image SVG (qrcode-generator, MIT), à l'échelle de sa place, correction moyenne. Écrit ICI une fois : le lien d'une
// partie (screens/play/Menu.jsx) et l'adresse de ce serveur (Réglages › Inviter, settings/application/Invite.jsx) ; on le
// pose dans `.gf-qr` (launcher.css), sur blanc.
export const qrImage = (text) => {
    const qr = qrcode(0, "M");
    qr.addData(text);
    qr.make();

    return `data:image/svg+xml;utf8,${encodeURIComponent(qr.createSvgTag({ cellSize: 4, margin: 4, scalable: true }))}`;
};
