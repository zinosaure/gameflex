import { createContext, useContext } from "react";

// LES CLASSES BOOTSTRAP QUE PLUSIEURS ÉCRANS COMPOSENT À L'IDENTIQUE — nommées une fois
// (l'homogénéité est la règle : un écart se définit une fois, jamais écran par écran).
// Tout le reste s'écrit directement en classes Bootstrap dans le JSX.

// L'INTITULÉ D'UN GROUPE, LE MÊME PARTOUT : celui de la direction C (validée par le porteur le 2026-11-13), en
// minuscules, `.gf-group-title` de launcher.css — sa taille, sa graisse, sa couleur et l'écart sous lui (mb-3, ou
// mb-1 quand une aide le suit). Les groupes des panneaux et des pages (Section, ui/Bricks.jsx) comme ceux
// des écrans (En direct, les séries de la liste des jeux) : le porteur, 2026-11-13, « Oui » à leur alignement sur C.
// Il n'y en a qu'un : le titre d'une section des Réglages (SECTION_HEADING), que le 8cf5e34d avait mis en capitales,
// est le même intitulé.
export const GROUP_HEADING = "gf-group-title";

// Une ligne d'aide ou d'information sous un contrôle : petite et atténuée.
export const MUTED = "small text-body-secondary";

// LES BOUTONS, PLEINS (le porteur, 2026-11-14, après la P7 du chantier 53 : « Malheureusement faudra remettre le
// background dans les boutons car sur certaines images on ne verra pas les boutons sinon. ») — et leur couleur, que le
// porteur a corrigée le même jour, après la P10 (le 7a99eb87 avait mis tout le reste des écrans en warning) : « J'avais
// demandé à ce que les couleurs succès et warning soient utilisées côté non settings. Ça voudrait pas dire qu'on
// n'utilise pas les autres couleurs sur ces pages. Le bouton retour, recherches, play et Settings passent donc en
// success. Et dans le panel recherches les boutons ne changent pas de couleur. Pour settings c'est primary et danger.
// Primary c'est pour valider les formulaires et danger c'est quand c'est disabled ou dangereux. Les radio group restent
// primary mais outline-primary les unchecked. »
//
// LES SORTES, et leur couleur selon le LIEU — définies ICI une fois :
//                écrans      Réglages
//   main         success     primary    le geste principal du lieu : Jouer, Bibliothèque, Se connecter, Rejoindre…,
//                                       Allumer et Regarder (Remote Play), valider un formulaire ;
//   round        success     primary    un rond à icône des écrans : Retour, la fermeture d'un panneau, la recherche,
//                                       l'engrenage (d'un jeu, des Réglages), la manette de l'en-tête, « + » (l'homogénéité : tous les
//                                       ronds d'un écran se ressemblent) — ceux de la barre du haut à contour
//                                       au repos (useBarToggle) ;
//   other        secondary   primary    le geste d'à côté : Plus tard, Annuler, Regarder près de Rejoindre, Détecter —
//                                       sa couleur d'avant 7a99eb87 ;
//   warning      warning     primary    ce qui avertit : Signaler une panne ;
//   danger       danger      danger     ce qui détruit (Tuer la partie) ; un bouton de formulaire au repos (« ••• »).
// Hors des Réglages, success et warning S'AJOUTENT aux couleurs : ils ne remplacent ni le secondary d'un geste d'à
// côté, ni le danger, ni la couleur d'un statut.
//
// UN BOUTON ENFONCÉ NE CHANGE PLUS DE COULEUR (la recherche qui restreint) : il garde celle de sa sorte, et `aria-pressed`
// dit son état — la pastille du compte suffit à la recherche. Deux exceptions, définies une fois plus bas : la case d'un
// choix segmenté (`choiceButton`), et les deux ronds de la barre du haut (`useBarToggle`).
// Le panneau de recherche de la bibliothèque n'a plus de boutons : des interrupteurs (chantier 53, P12).
//
// UN BOUTON NE CHOISIT JAMAIS SON JEU : il lit son lieu (ButtonPlace), que SettingsPage pose une fois autour de toutes
// ses pages ; tout le reste est un écran. `useButton()` rend `button(sorte)`, les classes du bouton ; `useButtonSet()`
// rend le jeu lui-même, pour un composant qui prend une variante (la Pagination de Skeleton).
export const BUTTON_SETS = {
    screens: { main: "success", round: "success", other: "secondary", warning: "warning", danger: "danger" },
    settings: { main: "primary", round: "primary", other: "primary", warning: "primary", danger: "danger" },
};
export const ButtonPlace = createContext("screens");

export const useButtonSet = () => BUTTON_SETS[useContext(ButtonPlace)];

export function useButton() {
    const set = useButtonSet();

    return (kind = "other") => `btn btn-${set[kind]}`;
}

// UN CHOIX SEGMENTÉ (des cases `btn-check` ou des puces `role="radio"`), PARTOUT, quel que soit le lieu (le porteur :
// « Les radio group restent primary mais outline-primary les unchecked ») : la case choisie pleine, les autres à contour.
export const choiceButton = (checked) => `btn ${checked ? "btn-primary" : "btn-outline-primary"}`;

// LES DEUX RONDS DE LA BARRE DU HAUT — les raccourcis de la manette, l'engrenage des Réglages — SONT DES BASCULES (le
// porteur, 2026-11-16 : « tu peux mettre les boutons de raccourcis et settings en outline et quand c'est activé c'est
// plein ») : à contour au repos, pleins quand leur panneau, ou les Réglages, sont ouverts ; `aria-pressed` le dit
// aussi. La couleur reste celle de leur sorte (`round`) et de leur lieu. Rend `toggle(on, sorte)`, les classes du bouton.
export function useBarToggle() {
    const set = useButtonSet();

    return (on, kind = "round") => `btn btn-${on ? "" : "outline-"}${set[kind]}`;
}

// Un bouton rond qui ne porte qu'une icône (retour, recherche, réglages, page suivante) : sa forme ; sa couleur vient
// de `button("round")` (`${ICON_BUTTON} ${button("round")}`).
export const ICON_BUTTON = "btn-icon rounded-circle";

// UN BOUTON CARRÉ QUI NE PORTE QU'UNE ICÔNE, à l'arrondi des boutons (6 px) : l'arrêt d'une partie dans En direct (le
// porteur, 2026-11-14 : « Le bouton power off doit être carré et non round »). Sa couleur vient de `button(sorte)`.
export const SQUARE_ICON_BUTTON = "btn-icon";

// Plus de bouton qui porte une icône ET un libellé : soit un texte, soit une icône (le porteur, 2026-11-13 ; la
// règle des boutons, ui/Bricks.jsx). WITH_ICON est parti avec le dernier.

// CE QUE LA CONSOLE PREND SANS CHOIX — l'émulateur en service, le BIOS par défaut — se lit à son libellé, en couleur
// et en gras (le porteur, 2026-11-13) : un seul par console et par liste.
export const IN_SERVICE = "text-primary fw-bold";

// La pastille d'un état (une partie, un appareil) : un petit rond de couleur, sans texte.
export const STATUS_DOT = "d-inline-block p-1 rounded-circle";
