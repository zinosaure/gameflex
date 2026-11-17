import { useEffect, useState } from "react";
import { can } from "../../../../libs/capabilities.js";
import { isPortrait } from "../../../../libs/device-detection.js";

// L'écran est-il debout ? Un appareil tenu droit est toute la raison d'être
// de la rotation, et il cesse de l'être dès qu'on le tourne pour de bon — on
// surveille donc, on ne lit pas une fois.
export function usePortraitViewport() {
    const [portrait, setPortrait] = useState(() => isPortrait());

    useEffect(() => {
        const check = () => setPortrait(isPortrait());
        window.addEventListener("resize", check);
        window.addEventListener("orientationchange", check);

        return () => {
            window.removeEventListener("resize", check);
            window.removeEventListener("orientationchange", check);
        };
    }, []);

    return portrait;
}

/**
 * TOURNER L'IMAGE D'UNE PARTIE, jamais l'écran — un seul exemplaire, pour toutes les pages qui streament.
 *
 * L'ORIENTATION SE DÉCIDE DANS LES RÉGLAGES, PAS EN OUVRANT UNE PARTIE (le porteur, 2026-11-14, chantier 53, P12 :
 * « quand on ouvre une session, le jeu ne doit pas basculer automatiquement sur landscape. La raison est que si on joue
 * avec la manette Gamesir Taco Pocket. Notre écran doit rester en portrait. C'est à l'utilisateur de décider
 * l'orientation dans settings / application. »). Ce hook demandait, au montage de la page de jeu, le plein écran puis
 * le verrou de l'orientation choisie : la partie basculait d'elle-même, à l'ouverture, sans geste de la personne. Ce
 * n'est plus lui qui tourne l'écran :
 *   - dans l'APPLICATION (`can("screenOrientation")`), l'activité a déjà tourné, pour toute l'application (#406) ;
 *   - dans un NAVIGATEUR, le launcher entier tourne quand la personne CHOISIT un paysage dans Réglages › Application
 *     (le plein écran puis le verrou, de son geste), et chaque fois qu'elle rallume le plein écran (libs/orientation.js).
 *     Chrome n'accorde `screen.orientation.lock()` qu'en plein écran, et le plein écran qu'à un geste récent : une page
 *     qui le demande seule, à son montage, n'a rien à faire valoir — et quand il lui était accordé, c'était cette bascule
 *     non voulue ;
 *   - « Suivre le téléphone » et « Portrait, demi-écran » : rien ne bouge, ni l'écran ni l'image.
 * Reste LE REPLI d'un paysage choisi quand l'écran n'a pas tourné (un navigateur hors du plein écran, ou qui refuse) :
 * l'image de la partie tourne en CSS — ce que Réglages › Application annonce (« Hors du plein écran, seule l'image d'une
 * partie tourne »). Il ne s'applique qu'à un écran resté debout (rotationSurfaceStyle).
 *
 * @param {string} requested   "auto" | "landscape" | "landscape-reverse" | "portrait-half" —
 *        la préférence de l'appareil (libs/screenOrientation.js)
 * @param {object} options
 * @param {boolean} options.enabled   faux sur une machine qu'on ne tourne pas
 * @returns {null|"cw"|"ccw"} le sens de la rotation de REPLI d'un paysage choisi — null quand il n'y a rien à
 *        tourner.
 */
export function rotationFallback(requested, { enabled = true } = {}) {
    if (!enabled || can("screenOrientation")) {
        return null;
    }

    if (requested === "landscape-reverse") {
        return "ccw";
    }

    return requested === "landscape" ? "cw" : null;
}

/**
 * La transformation de repli, prête à étaler dans un `style`.
 *
 * Vide quand l'OS a tourné pour de bon : il n'y a alors rien à corriger, et
 * ajouter une rotation par-dessus remettrait le jeu debout.
 */
export function rotationSurfaceStyle(rotateSurface, isPortraitViewport) {
    if (!rotateSurface || !isPortraitViewport) {
        return {};
    }

    return {
        position: "fixed",
        top: 0,
        left: 0,
        width: "100vh",
        height: "100vw",
        transformOrigin: "top left",
        // Le décalage n'est pas le même dans les deux sens : une rotation
        // autour du coin haut-gauche sort la boîte de l'écran par la gauche
        // dans un sens et par le haut dans l'autre. On la ramène par l'axe
        // qui correspond, sinon la moitié de l'image est hors cadre.
        transform: rotateSurface === "ccw"
            ? "translateY(100vh) rotate(-90deg)"
            : "translateX(100vw) rotate(90deg)",
    };
}
