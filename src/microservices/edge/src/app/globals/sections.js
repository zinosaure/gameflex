import { createContext, useContext } from "react";

// LA COULEUR D'UNE SECTION EST SON SENS (chantier 81, §4 ; le porteur, 2026-11-25 : « juste par la couleur de la section
// on doit savoir ce que ça veut dire ») : la couleur dit ce que la section FAIT, pas de quoi elle parle, et les pastilles
// de ses lignes la prennent — une ligne ne choisit plus la sienne. Les variantes de Bootstrap, portées par les couleurs de
// launcher.css (`.tone-*`). Une action qui ne peut pas se faire change la couleur de SA ligne (NavRow, `blocked`).
export const SENSES = {
    danger: "rose", // détruire, retirer, se déconnecter partout (danger)
    modify: "amber", // modifier ce qui existe (warning)
    add: "indigo", // ajouter, créer (primary)
    info: "sky", // informer (info)
    optional: "slate", // facultatif : Plus, Guide (secondary)
    play: "green", // jouer, ouvrir, télécharger (success)
};

// La couleur de la section où se trouve une ligne : la sienne quand elle n'en donne pas.
export const SenseTone = createContext(null);
export const useSenseTone = (given) => given || useContext(SenseTone) || "slate";
