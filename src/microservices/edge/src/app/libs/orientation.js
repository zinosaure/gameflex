import { can } from "./capabilities.js";
import { getScreenOrientation, setScreenOrientation } from "./screenOrientation.js";

// L'ORIENTATION DANS UN NAVIGATEUR (le porteur, 2026-11-14 : « pour info les orientations ne fonctionnent pas encore,
// du moins pas sur navigateur du téléphone »).
//
// Le réglage est celui de la webapp (libs/screenOrientation.js, repris tel quel) : une valeur gardée dans ce
// navigateur, que l'application Android applique à toute son activité (`can("screenOrientation")`). Dans un
// navigateur, il ne faisait RIEN hors d'une partie : rien ne demandait à l'écran de tourner. Et dans une partie, la
// page de jeu demandait le plein écran au montage, longtemps après le geste qui avait lancé le jeu : Chrome le refuse
// sans geste récent, refuse alors aussi le verrou (`screen.orientation.lock()` n'est permis qu'en plein écran), et
// seule l'image du jeu tournait, en CSS (use-screen-rotation.js).
//
// Ce que fait le launcher, désormais, dans un navigateur :
//   - CHOISIR un paysage, du geste de la personne, allume le plein écran PUIS verrouille l'écran dans ce sens : tout
//     le launcher tourne, et une partie lancée ensuite le trouve déjà tourné ;
//   - allumer le plein écran par un autre geste (la tuile d'Application, le menu d'une partie) verrouille aussi ;
//   - « Suivre le téléphone » et le demi-écran déverrouillent ;
//   - OUVRIR UNE PARTIE NE TOURNE RIEN (le porteur, 2026-11-14, chantier 53, P12 : « le jeu ne doit pas basculer
//     automatiquement sur landscape […] C'est à l'utilisateur de décider l'orientation dans settings / application ») :
//     la page de jeu ne demande plus ni plein écran ni verrou à son montage (use-screen-rotation.js) ;
//   - quitter le plein écran rend l'écran au téléphone (le navigateur lève le verrou de lui-même) : le choix reste
//     gardé, et revient avec le prochain plein écran ;
//   - un navigateur qui ne sait pas tourner l'écran (Safari sur iPhone, un ordinateur) ou qui refuse : la page le
//     dit (`canLockOrientation`, et le `false` rendu par `chooseOrientation`), le plein écran allumé pour tourner
//     s'éteint — seule l'image d'une partie tournera.
// L'application Android n'est pas concernée : son activité tourne d'elle-même.
const LOCKS = { landscape: "landscape-primary", "landscape-reverse": "landscape-secondary" };

const fullscreenElement = () => document.fullscreenElement || document.webkitFullscreenElement || null;
// Un choix en cours verrouille lui-même (il lui faut la réponse) : le plein écran qu'il allume ne verrouille pas deux fois.
let choosing = false;

// Ce navigateur peut-il tourner l'écran ? Il lui faut le verrou ET le plein écran.
export const canLockOrientation = () => {
    const root = document.documentElement;

    return Boolean(window.screen?.orientation?.lock) && Boolean(root.requestFullscreen || root.webkitRequestFullscreen);
};

// Verrouiller dans le sens choisi, ou déverrouiller ; vrai si le navigateur l'a fait.
async function lockTo(mode) {
    const target = LOCKS[mode];

    try {
        if (!target) {
            window.screen?.orientation?.unlock?.();

            return true;
        }
        await window.screen.orientation.lock(target);

        return true;
    } catch {
        return false;
    }
}

// CHOISIR L'ORIENTATION, du geste de la personne (le plein écran d'un navigateur n'est permis que juste après un geste) :
// le réglage gardé et dit à l'application ; dans un navigateur, le plein écran puis le verrou. `turnsTheApp` : l'hôte
// est l'application (sa capacité `screenOrientation`). Rend vrai quand l'écran a pris le choix.
export async function chooseOrientation(mode, turnsTheApp) {
    if (turnsTheApp) {
        return setScreenOrientation(mode);
    }
    const root = document.documentElement;
    const request = root.requestFullscreen || root.webkitRequestFullscreen;
    const enters = Boolean(LOCKS[mode] && !fullscreenElement() && request);
    choosing = true;

    try {
        // Le plein écran se demande AVANT toute attente : le geste qui le permet ne dure pas.
        const entering = enters ? Promise.resolve(request.call(root)).then(() => true, () => false) : Promise.resolve(true);
        await setScreenOrientation(mode);

        if (!(await entering)) {
            return false;
        }
        const locked = await lockTo(mode);

        // Refusé : le plein écran allumé pour tourner n'a plus de raison d'être, il s'éteint (la page ne fait pas
        // croire à un écran tourné).
        if (!locked && enters && fullscreenElement()) {
            await (document.exitFullscreen || document.webkitExitFullscreen)?.call(document)?.catch?.(() => {});
        }

        return locked;
    } finally {
        choosing = false;
    }
}

// LE PLEIN ÉCRAN ALLUMÉ PAR UN AUTRE GESTE : le paysage choisi le suit.
if (typeof document !== "undefined") {
    const follow = () => {
        if (!choosing && fullscreenElement() && !can("screenOrientation") && LOCKS[getScreenOrientation()]) {
            lockTo(getScreenOrientation());
        }
    };
    document.addEventListener("fullscreenchange", follow);
    document.addEventListener("webkitfullscreenchange", follow);
}
