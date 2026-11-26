import { useEffect, useRef, useState } from "react";

// LE MENU D'UNE PARTIE, À LA MANETTE ET SANS BARRE (#408, #409 — le porteur,
// 2026-10-19). Le geste de la manette qui l'ouvre : libs/gameInput.js
// (menuGestureOf), suivi par libs/play.js (useMenuHotkey).

// LE BOUTON ROND D'UN ÉCRAN DE JEU — celui du menu, et la croix qui annule un
// lancement (le porteur, 2026-10-29 : « un ✖️ dans un cercle (comme pour le
// bouton du menu) »). Sa forme s'écrit ici, une fois ; chacun pose sa place.
export const ROUND_BUTTON_LOOK = {
    width: "48px",
    height: "48px",
    background: "rgba(0, 0, 0, 0.55)",
    border: "1px solid rgba(255, 255, 255, 0.35)",
};

// LE BOUTON DU MENU, EN BAS À DROITE (le porteur, 2026-11-17 : « au lieu de le mettre à gauche, rajoute le à droite
// avec slide vers la gauche. Ça fait plus classe et ça fait comme ci on ouvrait vraiment le menu »), et plus bas
// (« tu peux le baisser encore un plus bas […] il y a de la marge avec les boutons d'Android ») : 24 px du bas, au-dessus
// de la zone de sécurité quand l'appareil en a une. Il était en bas à gauche depuis le 2026-10-22 (« Il faudra que le
// bouton menu passe en bas à gauche ») : en haut à droite, la barre d'adresse de Chrome le cachait hors du plein écran.
//
// DANS LE COIN, EN PAYSAGE COMME EN PORTRAIT, la même marge (le porteur, 2026-11-17 : « en mode landscape, le bouton
// menu doit toujours resté dans en bas à droite. Comme avec le mode portrait »). En paysage, la colonne de droite de la
// manette à l'écran met son stick (ou le A de la N64) dans ce coin : le bouton passe devant (zIndex 1003 contre 1002),
// et un toucher sur lui reste à lui — il ne pose pas de stick. Il se fait glisser vers la gauche (useSlideToOpen).
//
// Sa place s'écrit ICI, une fois, pour le bouton, son rail, l'anneau de L2 + Home et les pastilles de l'appareil.
export const MENU_BUTTON_STYLE = {
    bottom: "calc(24px + env(safe-area-inset-bottom, 0px))",
    right: "max(12px, env(safe-area-inset-right))",
    zIndex: 1003,
    ...ROUND_BUTTON_LOOK,
};

// LA COURSE DE LA GLISSIÈRE : la poignée doit la parcourir toute pour ouvrir le menu.
export const MENU_SLIDE_DISTANCE = 160;

/**
 * LE BOUTON DU MENU EST UNE POIGNÉE : glissé vers la gauche le long du bas, il ouvre le menu (vers la droite jusqu'au
 * 2026-11-17, quand il était à gauche) ; un appui simple n'ouvre plus rien (le porteur, 2026-11-17, sur la proposition
 * « A : seul le glissement ouvre le menu, un appui simple ne fait plus rien » : « Oui A »). Un pouce posé pour le stick
 * flottant ne l'ouvre donc jamais par accident. Le clavier (Entrée sur le bouton, Échap) et la manette (L2 + Home)
 * l'ouvrent toujours.
 *
 * La poignée suit le doigt vers la gauche (`travel`, en px, de 0 à MENU_SLIDE_DISTANCE) ; arrivée au bout, le menu
 * s'ouvre ; lâchée avant, elle revient à sa place. Un seul doigt la tient.
 *
 * @param {Function} onOpen ouvrir le menu
 * @returns {{ travel: number, held: boolean, handlers: object }} la course, la poignée tenue, ses gestionnaires
 */
export function useSlideToOpen(onOpen) {
    const [travel, setTravel] = useState(null);
    const heldRef = useRef(null);
    const onOpenRef = useRef(onOpen);
    onOpenRef.current = onOpen;
    const release = (event) => {
        if (heldRef.current?.id !== event.pointerId) {
            return;
        }
        heldRef.current = null;
        setTravel(null);
    };

    return {
        travel: travel ?? 0,
        held: travel !== null,
        handlers: {
            onPointerDown: (event) => {
                if (heldRef.current) {
                    return;
                }
                event.preventDefault();
                heldRef.current = { id: event.pointerId, x: event.clientX };
                try {
                    event.currentTarget.setPointerCapture?.(event.pointerId);
                } catch {
                    /* noop */
                }
                setTravel(0);
            },
            onPointerMove: (event) => {
                const held = heldRef.current;

                if (held?.id !== event.pointerId) {
                    return;
                }
                const distance = Math.min(MENU_SLIDE_DISTANCE, Math.max(0, held.x - event.clientX));

                if (distance >= MENU_SLIDE_DISTANCE) {
                    heldRef.current = null;
                    setTravel(null);
                    onOpenRef.current();

                    return;
                }
                setTravel(distance);
            },
            onPointerUp: release,
            onPointerCancel: release,
            onLostPointerCapture: release,
        },
    };
}

// Trois secondes sans un geste, et le bouton du menu s'efface — à 10 %, jamais tout à fait (le porteur, 2026-11-17 :
// « Le bouton menu doit être toujours visible et au lieu de faire disparaitre complètement, tu le fais fade de 0.05 »,
// puis « pour le bouton menu, mets l'opacité à 10% »).
const IDLE_MS = 3000;
export const MENU_IDLE_OPACITY = 0.1;

/**
 * LE BOUTON DU MENU S'ESTOMPE PENDANT QU'ON JOUE (MENU_IDLE_OPACITY), et revient au premier geste
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
