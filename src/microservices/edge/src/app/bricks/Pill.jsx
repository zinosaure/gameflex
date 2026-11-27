import React from "react";

// UNE VALEUR DE SOUS-TITRE, EN TEXTE COLORÉ (le porteur, 2026-11-25, des pastilles : « quand c'est actif c'est vert et
// quand ça ne l'est pas c'est rouge » ; le 2026-11-27, en texte : « oui », après l'essai du 2026-11-26,
// docs/reports/2026-11-26-sous-titres-en-texte/) : la couleur dit l'état, les valeurs d'une ligne se suivent, jointes
// par « • » (launcher.css, `.gf-pill`). `tone` : success (actif, installé, configuré), danger (inactif, manquant),
// info (un renseignement : un nombre, un nom) — défini ICI une fois. La valeur d'une ligne peut être une liste
// (« 23066 jeux », « 22 consoles »). `tone` peut aussi être une variante de Bootstrap (secondary, primary…) : le statut
// d'un jeu prend la sienne (STATUS_VARIANT, libs/format.js). `pill(texte, tone)` : un élément de sous-titre coloré,
// pour un listing (RowText).
const TONES = { success: "text-success-emphasis", danger: "text-danger-emphasis", warning: "text-warning-emphasis", info: "text-primary-emphasis",
    primary: "text-primary-emphasis", secondary: "text-body-secondary", cyan: "text-info-emphasis" };

// LE STATUT D'UN JEU, SA COULEUR, LA MÊME PARTOUT (le badge de ses détails : STATUS_VARIANT, libs/format.js).
export const STATUS_PILL = { playable: "success", patched: "cyan", glitches: "primary", crashes: "warning", unplayable: "danger", untested: "secondary" };

// RIEN, EN ROUGE (le porteur, 2026-11-25 : « Les pastilles qui sont à valeur 0 tu peux les mettre en rouge. Comme par exemple
// 0 jeux ») : une valeur de couleur par défaut dont le texte commence par le nombre zéro (« 0 jeu », « 0 », « 0 Go ») passe
// au rouge — une règle posée ici, une fois. Une couleur donnée par l'appelant (un statut) reste la sienne.
const ZERO = /^0(?![\d.,])/;
const isZero = (children) => (typeof children === "string" || typeof children === "number") && ZERO.test(String(children).trim());

// LA COULEUR PAR DÉFAUT SUIT LA PLACE (le porteur, 2026-11-25 : « tu peux les mettre par défaut couleur primary le premier
// et secondary le second ? Attention, les couleurs des states override les couleurs par défaut ») : sans couleur d'état
// (`tone` à DEFAULT_PILL), la première valeur d'une ligne est primary, les suivantes secondary (`index`).
export const DEFAULT_PILL = "default";
export const isStateTone = (tone) => Boolean(tone) && tone !== DEFAULT_PILL;
const placed = (tone, index) => (isStateTone(tone) ? tone : index === 0 ? "primary" : "secondary");

export function Pill({ tone = DEFAULT_PILL, index = 0, children }) {
    const base = placed(tone, index);
    const shown = (!isStateTone(tone) || tone === "info" || tone === "primary") && isZero(children) ? "danger" : base;

    return <span className={`gf-pill ${TONES[shown] || TONES.primary}`}>{children}</span>;
}

export const pill = (text, tone = DEFAULT_PILL) => (text === null || text === undefined || text === "" ? null : <Pill tone={tone}>{text}</Pill>);
