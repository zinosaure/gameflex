import React from "react";

// LE CONTENEUR ARRONDI D'UN GROUPE : une list-group de Bootstrap. Ce qu'il est se lit à ce qu'il contient, une fois,
// dans launcher.css (`.gf-list`), et aucune page ne le déclare (le porteur, 2026-11-14 : « quand une section est utilisée
// comme étant un formulaire alors on n'affiche pas les lignes de division mais on garde un mb-3. Et quand c'est pour les
// liens, les divisions doivent être remplies de gauche à droite. » ; puis « Certains formulaires ont toujours la
// séparation horizontale entre les form groups. Il faut faire comme les autres. » et « Dans manettes aussi il y a des
// traits dans le formulaire. ») :
//   - UN FORMULAIRE — tout groupe qui tient un contrôle (un interrupteur, un sélecteur, un champ, une glissière, un choix
//     segmenté ou des cases : un `input`, un `select`, un `textarea`) ou le bouton d'un formulaire (SubmitRow), qu'il
//     s'enregistre par un bouton ou qu'il s'applique sur place : ni trait entre ses lignes, un mb-3 de l'une à l'autre ;
//     ses interrupteurs sans icône (« évite de mettre l'icône à gauche quand c'est un Switch dans un formulaire ») ;
//   - UN GROUPE DE LIENS — fait seulement de liens (NavRow) et de renseignements (InfoRow), ou de leurs lignes fantômes et
//     de la ligne d'une liste vide : un trait entre deux lignes, d'un bord à l'autre ; ses lignes gardent leur icône.
// Un groupe qui mêle les deux est un formulaire (les candidats sous la recherche d'un service, le menu de la partie) : une
// page qui veut des traits à ses liens les pose dans un groupe à part (les touches d'une manette, sous le choix de
// l'appareil).
export function Rows({ className = "", children }) {
    return <div className={`list-group gf-list ${className}`}>{children}</div>;
}
