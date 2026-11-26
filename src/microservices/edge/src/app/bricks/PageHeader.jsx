import React, { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { ICON_BUTTON, useButton } from "../libs/ui.js";

// LE FIL D'ARIANE (le porteur, 2026-11-13 : « rajouter des fils d'Ariane dans les titres pour ne pas se perdre ») :
// le `breadcrumb` de Bootstrap, SOUS LE TITRE ET PLUS PETIT (le porteur, 2026-11-14, chantier 53, P13 : « pour
// essayer, peut-on mettre la fil d'Ariane en dessous du titre et en plus petit ? Voir ce que ça fait ? »), ses niveaux
// séparés par « / » — « Réglages / Consoles / PlayStation 4 ». Ce sont les pages AU-DESSUS de celle-ci, chacune un
// lien : la page elle-même est le titre, le fil ne la répète pas. Quand il ne tient pas sur la ligne, il se REPLIE au
// milieu, mesuré : la racine, « … », puis les deux derniers niveaux, ou le dernier seul ; et quand même le dernier seul
// ne tient pas (le long titre d'un jeu, au-dessus de sa page « Corriger les informations »), son nom PASSE À LA LIGNE
// (`gf-crumbs-wrap`, launcher.css) : un niveau n'est jamais coupé. `trail` : [{ label, to }], le plus haut d'abord. À la
// manette, B remonte d'un niveau comme toujours ; la croix atteint les niveaux du fil comme n'importe quel lien (BAS
// depuis Retour).
export function Crumbs({ trail }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const list = useRef(null);
    const [fold, setFold] = useState(0);
    // Les replis possibles : deux au-delà de deux niveaux, aucun sinon ; un de plus, et le dernier niveau passe à la ligne.
    const folds = trail.length >= 3 ? 2 : 0;
    const wrapped = fold > folds;
    const level = Math.min(fold, folds);
    const shown = level === 0 ? trail
        : level === 1 ? [trail[0], null, ...trail.slice(-2)]
            : [trail[0], null, trail[trail.length - 1]];

    // Replier tant que ça déborde ; tout redéplier quand la place change. Un niveau se réduit avant que la liste ne
    // déborde (`min-width: 0`, launcher.css) : un niveau réduit, c'est déjà trop long — sans ce second regard, le fil ne
    // se repliait jamais, il tronquait (vu au chantier 53, P8, sur les cinq niveaux d'un jeu).
    useLayoutEffect(() => {
        const node = list.current;
        const overflowing = node && (node.scrollWidth > node.clientWidth + 1 || Array.from(node.children).some((item) => item.scrollWidth > item.clientWidth + 1));

        if (overflowing && !wrapped) {
            setFold(fold + 1);
        }
    });
    // La place, c'est l'en-tête de la page (`.gf-page-header`), pas le fil : replié, le fil rétrécit, et le redéplier à
    // chaque repli le ferait battre.
    useEffect(() => {
        const node = list.current?.closest(".gf-page-header") || list.current;

        if (!node || typeof ResizeObserver === "undefined") {
            return undefined;
        }
        let width = node.clientWidth;
        const observer = new ResizeObserver(() => {
            if (Math.abs(node.clientWidth - width) > 1) {
                width = node.clientWidth;
                setFold(0);
            }
        });
        observer.observe(node);

        return () => observer.disconnect();
    }, []);

    return (
        <nav className={`gf-crumbs ${wrapped ? "gf-crumbs-wrap" : ""}`} aria-label={t("breadcrumb")}>
            <ol ref={list} className="breadcrumb">
                {shown.map((crumb, index) => (crumb === null
                    ? <li key="more" className="breadcrumb-item gf-crumb-more" aria-hidden="true">…</li>
                    : (
                        <li key={crumb.to} className="breadcrumb-item">
                            <a href={crumb.to} data-nav data-key={`crumb-${index}`} onClick={(event) => {
                                event.preventDefault();
                                navigate(crumb.to);
                            }}>{crumb.label}</a>
                        </li>
                    )))}
            </ol>
        </nav>
    );
}

// L'EN-TÊTE D'UNE PAGE DES RÉGLAGES, COMME CELUI DE LA FICHE DES JEUX (le porteur, 2026-11-14 : « faire comme dans la
// fiche des jeux, avoir le bouton "(<) Le titre" et ensuite en deuxième ligne le fil d'Ariane. Il faut que le titre
// soit intégré dans la page et non pas prendre le titre des offcanvas. Pas de border bottom sous le titre. ») : le rond
// de Retour (ICON_BUTTON, la sorte `round` : primary dans les Réglages) et le titre à côté, de la taille et du style
// de celui de la bibliothèque (`.ltitle`, Library.jsx) ; le fil d'Ariane SOUS LE TITRE, plus petit, dans la colonne
// du titre (le porteur, 2026-11-14, chantier 53, P13 : « pour essayer, peut-on mettre la fil d'Ariane en dessous du
// titre et en plus petit ? ») — le rond, au milieu de leur hauteur, à gauche des deux. Il était sur sa propre ligne,
// au bord gauche, sous le rond (P11). Aucun trait dessous.
//
// RIEN D'AUTRE (le porteur, 2026-11-14 : « le fil d'Ariane affiche "Streamex" après un retour à la ligne ») : PAS DE
// SOUS-TITRE. Ce qu'une page disait sous son fil (la façon dont une console se joue, la sorte d'une source, le nombre de
// jeux d'un disque) devient une ligne de sa section « Détails » quand elle en a une, ou disparaît.
//
// `enter` : le data-key de la commande où BAS mène depuis Retour (un saut déclaré, `data-down`, libs/input.js), quand
// la croix ne la trouverait pas d'elle-même — un interrupteur au bord droit sous des renseignements, qu'elle saute pour
// la ligne pleine largeur du dessous (sa règle : la même colonne d'abord).
// `remote` : le nom du serveur fédéré qu'on administre d'ici (chantier 73) — une pastille orange à côté du titre, pour
// ne jamais confondre ses réglages avec les nôtres.
export function PageHeader({ title, onBack, trail = [], enter = null, remote = "" }) {
    const { t } = useTranslation();
    const button = useButton();

    return (
        <header className="gf-page-header d-flex align-items-center gap-3">
            <button type="button" className={`${ICON_BUTTON} ${button("round")}`} data-nav data-key="sp-back" data-sound="back" data-down={enter || undefined}
                aria-label={t("back")} title={t("back")} onClick={onBack}>
                <i className="bi bi-chevron-left"></i>
            </button>
            <div className="flex-grow-1 min-w-0">
                <h1 className="ltitle m-0 text-break">{title}{remote && <span className="badge rounded-pill text-bg-warning ms-2 align-middle fs-6" style={{ textShadow: "none", WebkitTextStroke: 0 }}>{remote}</span>}</h1>
                {trail.length > 0 && <Crumbs trail={trail} />}
            </div>
        </header>
    );
}
