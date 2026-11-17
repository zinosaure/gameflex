import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { focusElement, navigable, useScreenInput } from "../libs/input.js";
import { ICON_BUTTON, useButton } from "../libs/ui.js";

// UN PANNEAU LATÉRAL : l'offcanvas de Bootstrap (ses classes), ouvert et fermé par le launcher
// lui-même — pas son JavaScript, qui ne connaît ni la manette ni le calque de la croix. La croix
// y circule (`data-nav-layer`), il défile avec elle (`data-scroller`), B ou son fond le referment.
//
// SON EN-TÊTE EST CELUI D'UNE PAGE DES RÉGLAGES (le porteur, 2026-11-14, chantier 53, P12 : « le offcanvas : rendre le
// titre comme pour les titres des pages settings. C'est à dire avoir le bouton à gauche et ensuite le titre (pas besoin
// de sous titres). Dans ce cas on aura (X) blablabla (et rajouter la même margin bottom que dans Settings) ») — défini
// ICI une fois, pour tous les panneaux : l'engrenage d'un jeu, la liste des jeux, le menu d'une partie, les filtres
// d'une liste (Listing, Bricks.jsx), les raccourcis :
//   - le rond de fermeture au bord gauche (ICON_BUTTON, la sorte `round` : success sur les écrans, primary dans les
//     Réglages — la couleur de son lieu, comme le Retour de PageHeader), la croix pour icône ; puis le titre, celui de
//     PageHeader (`.ltitle`) ;
//   - ni sous-titre, ni pastille, ni trait dessous ;
//   - L'ESPACEMENT EST CELUI DE BOOTSTRAP (le porteur : « le margin/padding bottom du titre dans offcanvas est trop gros
//     […] ça doit être normal », puis « dans search il faut laisser le padding top par défaut dans offcanvas body ») :
//     l'en-tête garde celui de `.offcanvas-header`, le corps son padding par défaut, et rien de plus — la première chose
//     du corps (une section, des tuiles) n'ajoute pas sa marge du haut (launcher.css, une fois).
// L'en-tête reste fixe au-dessus du corps, qui défile seul (`.offcanvas-body`). Au téléviseur, le rond est caché (Retour
// est sur la télécommande).
//
// IL GLISSE (le porteur, 2026-11-14 : « les offcanvas avec effet slide ») : la transition de Bootstrap elle-même
// (`.offcanvas-end` hors de l'écran, `.show` à sa place ; son fond en fondu, `.fade`). Le panneau naît sans `show` et le
// reçoit une image plus tard — il entre par la droite ; fermé par son rond, par B ou par son fond, il perd `show`,
// ressort, et ne disparaît (`onClose`) qu'à la fin de la transition. Un parent qui le retire lui-même le retire d'un coup.
const SLIDE_MS = 300;

// `badges` : des pastilles sous le titre (l'engrenage d'un jeu : « Expérimental » ; le porteur, 2026-11-14) — le seul
// sous-titre qu'un panneau porte.
export default function SidePanel({ label, className = "", firstKey, onClose, badges = null, children }) {
    const { t } = useTranslation();
    const button = useButton();
    const [shown, setShown] = useState(false);
    const closing = useRef(false);

    useEffect(() => {
        // Deux images : la première peint le panneau hors de l'écran, la seconde lance la transition.
        let frame = requestAnimationFrame(() => {
            frame = requestAnimationFrame(() => setShown(true));
        });

        return () => cancelAnimationFrame(frame);
    }, []);

    const close = () => {
        if (closing.current) {
            return;
        }
        closing.current = true;
        setShown(false);
        setTimeout(onClose, SLIDE_MS);
    };

    useEffect(() => {
        // La première commande voulue si elle se focalise (au téléviseur, le rond de fermeture est caché), sinon la première du panneau.
        // `firstKey` : sa clé (`data-key`), ou son id — les cartes d'un choix (RadioRow) reçoivent la leur, `<id>-<rang>`.
        const inPanel = navigable().filter((element) => element.closest(".npanel"));
        focusElement(inPanel.find((element) => element.dataset.key === firstKey || element.id === firstKey) || inPanel[0], true);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    useScreenInput({ back: close, switchTab: () => {} });

    return (
        <>
            <div className={`offcanvas-backdrop fade ${shown ? "show" : ""}`} onClick={close}></div>
            <aside className={`offcanvas offcanvas-end npanel ${shown ? "show" : ""} ${className}`} role="dialog" aria-modal="true" aria-label={label} data-nav-layer>
                <header className="offcanvas-header gap-3">
                    <button type="button" className={`${ICON_BUTTON} ${button("round")} gf-panel-close`} data-nav data-key="p-close" data-sound="back"
                        aria-label={t("close")} title={t("close")} onClick={close}>
                        <i className="bi bi-x-lg"></i>
                    </button>
                    {badges ? (
                        <div className="flex-grow-1 min-w-0">
                            <h1 className="ltitle m-0 text-break">{label}</h1>
                            <div className="d-flex flex-wrap gap-1 mt-1">{badges}</div>
                        </div>
                    ) : <h1 className="ltitle flex-grow-1 min-w-0 m-0 text-break">{label}</h1>}
                </header>
                <div className="offcanvas-body pb-4" data-scroller>{children}</div>
            </aside>
        </>
    );
}
