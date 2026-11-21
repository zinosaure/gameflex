// LES TRACES DE DÉVELOPPEMENT — vers la console du navigateur, et nulle part
// ailleurs.
//
// Ce module portait un tampon circulaire de 250 entrées et DÉTOURNAIT
// `console.log/warn/error/debug` pour tout y recopier, afin qu'un onglet
// « Logs » du panneau de performances puisse l'afficher. L'intention était
// bonne — voir les traces sans outils de développement, sur un téléphone.
//
// Ce que ça donnait en pratique, sur une capture d'écran du 2026-09-08 : le
// panneau était rempli d'avertissements de React Router sur des drapeaux de
// version 7, et de lignes « appui hors de tout bouton » à chaque contact avec
// la manette à l'écran. On y cherchait pourquoi une PS2 n'affiche pas d'image.
//
// Le panneau est retiré, donc le tampon n'a plus de lecteur, donc le
// détournement de `console` n'a plus d'objet. Restent les deux destinations
// que le code appelle vraiment — soixante-six fois — et qui vont là où l'on
// regarde quand on cherche : la console.

const VERBOSE_KEY = "rp_debug_logging";
const throttleState = new Map();

function isVerbose() {
    return typeof localStorage !== "undefined" && localStorage.getItem(VERBOSE_KEY) === "1";
}

/**
 * Une trace de développement — silencieuse par défaut.
 *
 * Elle n'apparaît que si l'on a demandé le mode bavard
 * (`localStorage.rp_debug_logging = "1"`). C'est ce qui permet d'en semer
 * beaucoup sans polluer la console de tout le monde.
 */
export function debugTrace(...args) {
    if (isVerbose()) {
        console.debug(...args);
    }
}

/**
 * Comme la console, mais au plus une fois par `windowMs` pour une même `key`.
 *
 * Pour les vrais chemins d'erreur qui vivent dans une boucle par image : sans
 * ça, une seule anomalie de manette écrit soixante lignes par seconde.
 */
export function logThrottled(level, key, windowMs, ...args) {
    const now = Date.now();
    const last = throttleState.get(key);

    if (last && now - last < windowMs) {
        return;
    }

    throttleState.set(key, now);
    const write = console[level] || console.log;
    write(...args);
}
