import React from "react";

// UNE VALEUR EN PASTILLE (le porteur, 2026-11-25 : « pour les éléments avec sous-titres comme valeur, tu peux mettre les
// valeurs en pastille comme pour status dans la fiche des jeux ? Par exemple quand c'est actif c'est pastille verte et
// quand ça ne l'est pas c'est rouge. Pour ce qui est pastille info, tu mets en bleu ») : la pastille de Bootstrap, comme
// le statut d'un jeu. `tone` : success (actif, installé, configuré), danger (inactif, manquant), info (un renseignement :
// un nombre, un nom) — défini ICI une fois. PLUSIEURS VALEURS, PLUSIEURS PASTILLES SUR LA MÊME LIGNE (le porteur, 2026-11-25 :
// « soit mettre les 2 pastilles sur la même ligne ») : la valeur d'une ligne peut être une liste (« 23066 jeux », « 22 consoles »).
// `tone` peut aussi être une variante de Bootstrap (secondary, primary…) : le statut d'un jeu prend la sienne
// (STATUS_VARIANT, libs/format.js). `pill(texte, tone)` : un élément de sous-titre coloré, pour un listing (RowText).
export const PILL_TONES = { success: "text-bg-success", danger: "text-bg-danger", warning: "text-bg-warning", info: "text-bg-primary",
    primary: "text-bg-primary", secondary: "text-bg-secondary", cyan: "text-bg-info" };

// LE STATUT D'UN JEU, SA COULEUR, LA MÊME PARTOUT (le badge de ses détails : STATUS_VARIANT, libs/format.js).
export const STATUS_PILL = { playable: "success", patched: "cyan", glitches: "primary", crashes: "warning", unplayable: "danger", untested: "secondary" };

// RIEN, EN ROUGE (le porteur, 2026-11-25 : « Les pastilles qui sont à valeur 0 tu peux les mettre en rouge. Comme par exemple
// 0 jeux ») : une pastille de couleur par défaut dont le texte commence par le nombre zéro (« 0 jeu », « 0 », « 0 Go ») passe
// au rouge — une règle posée ici, une fois. Une couleur donnée par l'appelant (un statut) reste la sienne.
const ZERO = /^0(?![\d.,])/;
const isZero = (children) => (typeof children === "string" || typeof children === "number") && ZERO.test(String(children).trim());

// LA COULEUR PAR DÉFAUT SUIT LA PLACE (le porteur, 2026-11-25 : « tu peux les mettre par défaut couleur primary le premier
// et secondary le second ? Attention, les couleurs des states override les couleurs par défaut ») : sans couleur d'état
// (`tone` à DEFAULT_PILL), la première pastille d'une ligne est primary, les suivantes secondary (`index`).
export const DEFAULT_PILL = "default";
export const isStateTone = (tone) => Boolean(tone) && tone !== DEFAULT_PILL;
const placed = (tone, index) => (isStateTone(tone) ? tone : index === 0 ? "primary" : "secondary");

export function Pill({ tone = DEFAULT_PILL, index = 0, children }) {
    const base = placed(tone, index);
    const shown = (!isStateTone(tone) || tone === "info" || tone === "primary") && isZero(children) ? "danger" : base;

    return <span className={`badge rounded-pill gf-pill ${PILL_TONES[shown] || PILL_TONES.primary}`}>{children}</span>;
}

export const pill = (text, tone = DEFAULT_PILL) => (text === null || text === undefined || text === "" ? null : <Pill tone={tone}>{text}</Pill>);
