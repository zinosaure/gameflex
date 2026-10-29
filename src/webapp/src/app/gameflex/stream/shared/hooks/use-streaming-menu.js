import { useEffect, useRef, useState } from "react";
import { connectedGamepads } from "../../../libs/keyMapping.js";
import { MENU_CHORD } from "../../../libs/gameInput.js";
import { PAD_COLUMN_WIDTH, PORTRAIT_LIFT } from "../../onscreen/Controllers.jsx";

// LE MENU D'UNE PARTIE, À LA MANETTE ET SANS BARRE (#408, #409 — le porteur,
// 2026-10-19).

// L2 + HOME (libs/gameInput.js, MENU_CHORD). C'était L3 + R3 jusqu'au
// 2026-10-21, puis Select + Start, pour les manettes clipsées sans sticks
// (#410) ; le porteur l'a changé le 2026-10-28 : Start tenu éteint plusieurs
// manettes GameSir. Home n'est pas un geste de jeu tant que L2 est tenu (il
// n'atteint plus le jeu) : une seconde suffit, là où Select + Start, deux
// boutons de jeu, en demandaient cinq — et un Home tenu longtemps éteint, lui
// aussi, certaines manettes.
const MENU_HOLD_MS = 1000;

// LE BOUTON ROND D'UN ÉCRAN DE JEU — celui du menu, et la croix qui annule un
// lancement (le porteur, 2026-10-29 : « un ✖️ dans un cercle (comme pour le
// bouton du menu) »). Sa forme s'écrit ici, une fois ; chacun pose sa place.
export const ROUND_BUTTON_LOOK = {
    width: "48px",
    height: "48px",
    background: "rgba(0, 0, 0, 0.55)",
    border: "1px solid rgba(255, 255, 255, 0.35)",
};

// LE BOUTON DU MENU, EN BAS À GAUCHE (le porteur, 2026-10-22 : « Il faudra
// que le bouton menu passe en bas à gauche »). En haut à droite, la barre
// d'adresse de Chrome le cachait hors du plein écran. Sa place s'écrit ICI,
// une fois, pour la page de jeu et pour le test des touches : deux copies
// finissent par diverger.
const MENU_BUTTON_STYLE = {
    bottom: "max(12px, env(safe-area-inset-bottom))",
    left: "max(12px, env(safe-area-inset-left))",
    zIndex: 1003,
    ...ROUND_BUTTON_LOOK,
};

/**
 * LA PLACE DU BOUTON DU MENU : le coin en bas à gauche — sauf quand la manette
 * à l'écran est affichée en paysage. Sa colonne de gauche y met son stick, dans
 * ce coin même ; le bouton reste alors en bas, juste après elle. « Paysage »
 * se juge comme la manette le juge : la fenêtre plus large que haute.
 *
 * @param {boolean} padVisible la manette à l'écran est affichée
 * @returns {object} le style du bouton
 */
export function useMenuButtonStyle(padVisible) {
    const [landscape, setLandscape] = useState(() => window.innerWidth > window.innerHeight);
    useEffect(() => {
        const update = () => setLandscape(window.innerWidth > window.innerHeight);
        window.addEventListener("resize", update);

        return () => window.removeEventListener("resize", update);
    }, []);

    if (!landscape) {
        // En portrait, remonté avec la manette (PORTRAIT_LIFT).
        return {
            ...MENU_BUTTON_STYLE,
            bottom: `max(${12 + PORTRAIT_LIFT}px, calc(env(safe-area-inset-bottom) + ${PORTRAIT_LIFT}px))`,
        };
    }
    if (!padVisible) {
        return MENU_BUTTON_STYLE;
    }

    return { ...MENU_BUTTON_STYLE, left: `${PAD_COLUMN_WIDTH + 12}px` };
}

/**
 * L2 + HOME TENUS UNE SECONDE OUVRENT LE MENU — le raccourci que le porteur a
 * choisi, pour la manette qui n'a pas de doigt pour toucher le bouton.
 *
 * Rend l'avancement (0 à 1) pour que la page le montre : sans rien voir, on
 * lâche avant la fin. Il faut relâcher pour réarmer : un menu
 * refermé les deux boutons encore enfoncés ne se rouvre pas aussitôt.
 *
 * Toutes les manettes branchées comptent — celle du joueur 2 aussi : le menu
 * est celui de l'appareil. Les manettes virtuelles du serveur n'en sont pas
 * (connectedGamepads les écarte).
 *
 * @param {boolean} enabled faux quand un panneau est déjà ouvert
 * @param {Function} onOpen ouvrir le menu
 * @returns {number} l'avancement de l'appui, de 0 à 1
 */
export function useMenuHotkey(enabled, onOpen) {
    const [progress, setProgress] = useState(0);
    // Le rappel dans une référence : l'avancement re-rend la page à chaque
    // pas, et un rappel neuf en dépendance relancerait la boucle — l'appui
    // repartirait de zéro, et n'aboutirait jamais.
    const onOpenRef = useRef(onOpen);
    onOpenRef.current = onOpen;

    useEffect(() => {
        if (!enabled) {
            setProgress(0);

            return undefined;
        }

        let raf;
        let since = null;
        let armed = true;
        let shown = 0;
        const show = (value) => {
            if (value !== shown) {
                shown = value;
                setProgress(value);
            }
        };
        const tick = () => {
            const held = connectedGamepads().some((pad) => pad
                && MENU_CHORD.every((index) => pad.buttons?.[index]?.pressed));
            const now = performance.now();

            if (!held) {
                since = null;
                armed = true;
                show(0);
            } else if (armed) {
                since ??= now;
                // Par pas de 5 % : un rendu par image pour une barre qui n'a
                // que vingt positions utiles.
                const value = Math.min(1, Math.floor(((now - since) / MENU_HOLD_MS) * 20) / 20);
                show(value);
                if (value >= 1) {
                    armed = false;
                    since = null;
                    show(0);
                    onOpenRef.current();
                }
            }
            raf = requestAnimationFrame(tick);
        };
        raf = requestAnimationFrame(tick);

        return () => cancelAnimationFrame(raf);
    }, [enabled]);

    return progress;
}

// Trois secondes sans un geste, et le bouton du menu s'efface.
const IDLE_MS = 3000;

/**
 * LE BOUTON DU MENU S'EFFACE PENDANT QU'ON JOUE, et revient au premier geste
 * — la souris qui bouge, un doigt posé sur l'image. Pas un doigt posé sur la
 * manette À L'ÉCRAN : on ne la quitte pas, et le bouton ne reviendrait plus
 * jamais en jeu tactile. Un appui long sur L2 + Home, ou Échap, ouvrent le menu
 * sans lui.
 *
 * Qui regarde, ou attend sa manette, le garde affiché : c'est par lui qu'on
 * prend la manette ou qu'on s'en va.
 *
 * @param {boolean} playing vrai quand cette page tient une manette et
 *        qu'aucun panneau n'est ouvert
 * @returns {boolean} faut-il montrer le bouton
 */
export function useIdleVisibility(playing) {
    const [visible, setVisible] = useState(true);

    useEffect(() => {
        if (!playing) {
            setVisible(true);

            return undefined;
        }

        let timer = setTimeout(() => setVisible(false), IDLE_MS);
        const wake = (event) => {
            if (event.pointerType !== "mouse" && event.target?.closest?.("[data-streaming-strip]")) {
                return;
            }
            setVisible(true);
            clearTimeout(timer);
            timer = setTimeout(() => setVisible(false), IDLE_MS);
        };
        window.addEventListener("pointermove", wake, { passive: true });
        window.addEventListener("pointerdown", wake, { passive: true });

        return () => {
            clearTimeout(timer);
            window.removeEventListener("pointermove", wake);
            window.removeEventListener("pointerdown", wake);
        };
    }, [playing]);

    return visible;
}
