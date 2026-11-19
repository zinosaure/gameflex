import { useEffect, useState } from "react";
import { can } from "../../../libs/capabilities.js";
import { requestFullscreen, isFullscreen, isPortrait } from "../../../shared/utils/device-detection.js";

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
 * TOURNER L'ÉCRAN — un seul exemplaire, pour toutes les pages qui streament.
 *
 * Il vivait dans `Streaming.jsx`. La page de test des manettes en a besoin
 * pour la même raison — regarder un pad de travers ne sert à rien — et
 * recopier cent lignes de verrou d'orientation, de repli CSS et d'appel à
 * l'hôte aurait fait deux implémentations à tenir d'accord. On en a déjà
 * payé le prix cette nuit, deux fois.
 *
 * @param {string} requested   "auto" | "landscape" | "landscape-reverse" | "portrait-half" —
 *        la préférence de l'appareil (libs/screenOrientation.js)
 * @param {object} options
 * @param {boolean} options.enabled   faux sur une machine qu'on ne tourne pas
 * @param {object} options.surfaceRef l'élément à passer en plein écran — le
 *        verrou d'orientation d'Android l'exige
 * @param {Function} options.onFullscreen prévenir l'appelant qu'on vient de
 *        l'allumer, pour que SON interrupteur le dise
 * @returns {null|"cw"|"ccw"} le sens de la rotation de REPLI, quand le verrou
 *        a été refusé — null quand l'OS a tourné pour de bon, ou qu'il n'y a
 *        rien à tourner.
 */
export function useScreenRotation(requested, { enabled = true, surfaceRef = null, onFullscreen = null } = {}) {
    // null quand l'écran ne tourne pas, sinon le SENS de la rotation de
    // repli — la transformation CSS n'est pas la même dans les deux sens.
    const [rotateSurface, setRotateSurface] = useState(null);

    // Turn the screen, on a device someone can turn.
    //
    // Asked for by the user (2026-08-25): a phone held upright shows a
    // landscape game as a letterboxed strip, and turning the phone is not
    // always an option (a stand, a case, orientation locked at the OS level).
    //
    // Two mechanisms, deliberately in this order. The Screen Orientation API
    // is the real thing — the OS rotates, so every overlay, every menu and
    // every touch follows without us doing anything. It also refuses often:
    // Chrome only grants it in fullscreen, and iOS Safari has no lock at all.
    // Only then do we rotate the surface ourselves, which is the fallback and
    // reads like one: a transform we have to maintain.
    useEffect(() => {
        if (!enabled) {
            setRotateSurface(null);

            return undefined;
        }

        let cancelled = false;

        // DANS L'APPLICATION, L'ACTIVITÉ A DÉJÀ TOURNÉ (#406, 2026-10-19).
        //
        // L'orientation est un réglage de TOUTE l'application, posé dans
        // Réglages › Paramètres de l'application (libs/screenOrientation.js) :
        // l'activité Android pivote, et tout suit — le menu, les touches, la
        // console d'accueil. Il n'y a plus rien à demander ici, ni à remettre
        // droit en partant : c'est ce que la personne a choisi pour l'appareil.
        // La transformation CSS ne tournait que la surface vidéo (signalé le
        // 2026-09-01) ; c'est l'hôte qui le DÉCLARE (`screenOrientation`).
        const rotation = requested || "auto";

        if (can("screenOrientation")) {
            setRotateSurface(null);

            return undefined;
        }

        // Qui suit le téléphone, ou le tient debout dans une manette
        // clipsée (le demi-écran, #410) : rien à verrouiller.
        if (rotation === "auto" || rotation === "portrait-half") {
            try {
                window.screen?.orientation?.unlock?.();
            } catch {
                // Nothing to unlock — the lock never took.
            }
            setRotateSurface(null);
            return undefined;
        }

        // TOURNER DEMANDE LE PLEIN ÉCRAN — sur Android, `orientation.lock()`
        // est refusé hors de lui. Demandé le 2026-09-05, et c'est aussi ce
        // qui rendait la rotation inopérante sur téléphone.
        //
        // On l'ALLUME, on ne l'éteint jamais : quelqu'un qui jouait déjà en
        // plein écran et qui tourne son écran ne demande pas à en sortir. Et
        // au retour à l'endroit on n'y touche pas non plus — c'est un réglage
        // à part, avec son propre interrupteur.
        if (!isFullscreen() && surfaceRef?.current) {
            onFullscreen?.();
            requestFullscreen(surfaceRef.current).catch(() => {
                // Refusé (pas de geste utilisateur reconnu) : on tourne
                // quand même, la surface CSS prendra le relais.
            });
        }

        // `landscape-primary` / `landscape-secondary` sont les deux sens de
        // l'API elle-même. On ne demande plus « landscape » tout court : ça
        // laissait le navigateur choisir, donc une seule des deux positions
        // était atteignable.
        const target = rotation === "landscape-reverse" ? "landscape-secondary" : "landscape-primary";
        const direction = rotation === "landscape-reverse" ? "ccw" : "cw";

        try {
            const locking = window.screen?.orientation?.lock?.(target);
            if (!locking) {
                setRotateSurface(direction);
            } else {
                Promise.resolve(locking)
                    .then(() => !cancelled && setRotateSurface(null))
                    .catch(() => !cancelled && setRotateSurface(direction));
            }
        } catch {
            setRotateSurface(direction);
        }

        return () => {
            cancelled = true;
        };
    }, [requested, enabled]);

    return rotateSurface;
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
