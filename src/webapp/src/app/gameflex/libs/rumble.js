import { getRumbleEnabled, getRumbleStrength } from "./gamepadPreferences.js";

// LE MOTEUR DE VIBRATION — un seul, pour les trois plateformes.
//
// Il y en avait deux, et ils faisaient la même chose : trouver le moteur
// d'une manette, l'actionner, et à défaut faire vibrer le téléphone. L'un
// servait le rétro (`triggerRumble`), l'autre remote-play
// (`applyControllerRumbleToGamepads`, dans son propre service). Ils avaient
// chacun leur magasin de réglages, et ils avaient déjà divergé : le rétro
// exigeait `typeof playEffect === "function"` avant de renoncer, remote-play
// connaissait en plus l'ancien `hapticActuators.pulse` — donc une manette
// ancienne vibrait sur PS5 et pas sur PS1, sans que rien ne le dise.
//
// Ce fichier garde l'UNION de ce que les deux savaient faire. Ce qui reste
// propre à chaque plateforme est sa PLAGE d'entrée, et rien d'autre :
// libretro compte de 0 à 65535, la PS5 de 0 à 255. Les deux adaptateurs
// convertissent et appellent ici.
//
// Deux moteurs derrière un seul appel, parce que lequel s'applique dépend
// entièrement de ce qui est branché :
//
// - une VRAIE manette (Gamepad API) a ses propres moteurs, joignables par
//   `GamepadHapticActuator.playEffect('dual-rumble', …)` — Chrome
//   seulement aujourd'hui, mais c'est déjà le seul navigateur que le chemin
//   d'entrée manette de cette application suppose ;
// - pas de manette (manette virtuelle tactile) : `navigator.vibrate()`, le
//   moteur du TÉLÉPHONE — Chrome Android seulement, Safari iOS n'a jamais
//   implémenté l'API Vibration.

function isBrowser() {
    return typeof window !== "undefined" && typeof navigator !== "undefined";
}

function clamp01(value) {
    if (!Number.isFinite(value) || value <= 0) return 0;

    return value >= 1 ? 1 : value;
}

function getGamepads() {
    if (!isBrowser() || typeof navigator.getGamepads !== "function") {
        return [];
    }

    return Array.from(navigator.getGamepads.call(navigator) ?? [], (pad) => pad ?? null);
}

// L'actionneur d'une manette, ancienne API comprise.
//
// `vibrationActuator` est la forme actuelle ; `hapticActuators[0]` est celle
// d'avant, encore la seule que certaines manettes exposent. Le moteur rétro
// ignorait la seconde, celui de remote-play la connaissait — c'est
// exactement le genre d'écart que deux implémentations produisent en
// silence.
function actuatorOf(pad) {
    return pad?.vibrationActuator || (pad?.hapticActuators ?? [])[0] || null;
}

function playWithActuator(actuator, strong, weak, duration) {
    try {
        if (typeof actuator.playEffect === "function") {
            void actuator.playEffect("dual-rumble", {
                startDelay: 0,
                duration,
                strongMagnitude: strong,
                weakMagnitude: weak,
            });

            return true;
        }
        if (typeof actuator.pulse === "function") {
            void actuator.pulse.call(actuator, Math.max(strong, weak), duration);

            return true;
        }
    } catch (error) {
        // `playEffect` rend une promesse qui PEUT être rejetée (manette
        // débranchée entre-temps, effet refusé par le pilote), et l'appel
        // lui-même peut lever. Non attrapée, l'erreur remontait en
        // « unhandled rejection » dans la console d'un joueur qui n'a rien
        // demandé.
        rumbleTrace("manette : effet refusé", error?.message || error);
    }

    return false;
}

function stopActuator(actuator) {
    return playWithActuator(actuator, 0, 0, 1);
}

/**
 * Faire vibrer, à partir de magnitudes DÉJÀ normalisées entre 0 et 1.
 *
 * Normalisées, parce que c'est la seule chose que les plateformes ne
 * partagent pas : leur plage d'entrée. Tout le reste — l'interrupteur
 * global, l'intensité, la résolution de l'actionneur, le repli sur le
 * téléphone — est le même besoin, et vit donc ici.
 *
 * @param {object} request
 * @param {number} request.strong      moteur lourd, 0..1
 * @param {number} request.weak        moteur léger, 0..1
 * @param {number} [request.durationMs] durée de l'effet manette
 * @param {number|null} [request.gamepadIndex] une manette précise, ou null
 * @param {"first"|"all"} [request.targets] à qui l'envoyer quand aucun index
 *        n'est donné. « first » (rétro) s'arrête à la première manette qui
 *        vibre ; « all » (remote-play) les prend toutes. Explicite plutôt
 *        que choisi ici : les deux existaient, et en imposer un changerait
 *        le comportement d'une plateforme sans que personne l'ait demandé.
 * @param {boolean} [request.allowDeviceVibration] autoriser le repli sur le
 *        moteur du téléphone. Faux pour un gabarit de manette sans moteurs
 *        (NES, PSP) : le téléphone ne doit pas vibrer pour une console qui
 *        n'a jamais pu.
 * @param {number|null} [request.deviceVibrationMs] durée du repli téléphone,
 *        ou null pour la déduire de l'intensité.
 * @returns {boolean} vrai si quelque chose a effectivement vibré.
 */
export function applyRumble({
    strong,
    weak,
    durationMs = 100,
    gamepadIndex = null,
    targets = "first",
    allowDeviceVibration = true,
    deviceVibrationMs = null,
}) {
    // L'interrupteur global (page Manette) vaut pour LES DEUX moteurs : les
    // actionneurs de manette et le repli téléphone.
    if (!getRumbleEnabled()) {
        return false;
    }

    const scale = clamp01(getRumbleStrength());
    const strongMagnitude = clamp01(strong) * scale;
    const weakMagnitude = clamp01(weak) * scale;
    const shouldStop = strongMagnitude === 0 && weakMagnitude === 0;

    let applied = false;
    for (const pad of getGamepads()) {
        if (!pad) continue;
        if (typeof gamepadIndex === "number" && pad.index !== gamepadIndex) {
            continue;
        }
        const actuator = actuatorOf(pad);
        if (!actuator) continue;

        applied = (shouldStop
            ? stopActuator(actuator)
            : playWithActuator(actuator, strongMagnitude, weakMagnitude, durationMs)) || applied;

        if (typeof gamepadIndex === "number" || (targets === "first" && applied)) {
            break;
        }
    }

    if (applied) {
        rumbleTrace("manette", `fort=${strongMagnitude.toFixed(2)} faible=${weakMagnitude.toFixed(2)}`);

        return true;
    }

    // PAS DE MANETTE N'EST PAS UNE RAISON DE NE RIEN FAIRE.
    //
    // Une garde `gamepads.length === 0` a longtemps rendu ce repli
    // inatteignable : sur un écran tactile il n'y a par définition aucune
    // manette branchée, donc on rendait la main avant d'y arriver. La PS5 ne
    // faisait donc jamais vibrer le téléphone, alors que ce repli avait été
    // écrit exactement pour ça (2026-09-08).
    if (shouldStop) {
        // `navigator.vibrate(0)` est un no-op défini par la spécification :
        // un « arrête-toi » n'a donc rien à faire ici.
        return false;
    }
    if (!allowDeviceVibration) {
        rumbleTrace("ignoré", "ce gabarit de manette n'a pas de moteurs");

        return false;
    }
    if (typeof navigator.vibrate !== "function") {
        rumbleTrace("ignoré", "ce navigateur ne sait pas faire vibrer l'appareil");

        return false;
    }

    const intensity = Math.max(strongMagnitude, weakMagnitude);
    const milliseconds = deviceVibrationMs ?? Math.round(intensity * 100);
    if (milliseconds <= 0) {
        return false;
    }
    navigator.vibrate(milliseconds);
    rumbleTrace("téléphone", `intensité=${intensity.toFixed(2)} durée=${milliseconds}ms`);

    return true;
}

/**
 * L'adaptateur LIBRETRO — plage 0..65535.
 *
 * C'est celle que `stream_session.go`'s `rumbleStateCallback` envoie ; la
 * mise à l'échelle est le travail de ce côté-ci, pas celui du serveur.
 */
export function triggerRumble(gamepadIndex, weak, strong, { virtualControllerRumble = true } = {}) {
    // Durée courte et fixe, délibérément : libretro envoie un NOUVEL appel
    // `set_rumble_state` à chaque changement (« arrête » compris, force 0),
    // donc il suffit de couvrir l'intervalle jusqu'à la mise à jour
    // suivante, pas de modéliser l'enveloppe complète d'un effet.
    return applyRumble({
        strong: strong / 65535,
        weak: weak / 65535,
        durationMs: 100,
        gamepadIndex,
        targets: "first",
        allowDeviceVibration: virtualControllerRumble,
    });
}

// UNE VIBRATION QUI NE VIENT PAS EST INDIAGNOSTICABLE.
//
// Rien ne bouge : le serveur n'a pas envoyé ? le message n'est pas arrivé ?
// l'appel est parti et le navigateur n'a rien fait ? Les trois donnent
// exactement le même résultat — c'est-à-dire aucun. Le serveur journalise
// déjà chacun de ses appels ; ce côté-ci n'en disait rien.
//
// Au plus une ligne par seconde et par motif : la vibration change à chaque
// image quand elle est active, et une trace par appel noierait la console.
const rumbleTraceAt = new Map();
function rumbleTrace(what, detail) {
    const now = Date.now();
    if (now - (rumbleTraceAt.get(what) || 0) < 1000) return;
    rumbleTraceAt.set(what, now);
    // eslint-disable-next-line no-console
    console.info(`[rumble] ${what} — ${detail}`);
}
