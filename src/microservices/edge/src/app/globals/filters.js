import { createContext } from "react";



// UNE LISTE ÉCARTÉE, gardée dans l'adresse (`?nostatus=crashes,untested`) : ses valeurs ; et la même après un interrupteur
// basculé, prête pour l'adresse (null : plus rien d'écarté).
export const hiddenOf = (value) => String(value || "").split(",").filter(Boolean);

// `value` : une valeur, ou plusieurs d'un coup (l'interrupteur « Tout » d'un groupe, switchGroup).
export const toggleHidden = (hidden, value, on) => {
    const values = [].concat(value);
    const next = on ? hidden.filter((item) => !values.includes(item)) : [...new Set([...hidden, ...values])];

    return next.join(",") || null;
};

// DANS UN PANNEAU DE FILTRES, UN INTERRUPTEUR N'A PAS DE DESCRIPTION (le porteur, 2026-11-25 : « Dans les filtres, retire
// les descriptions des switches. Ça crée trop d'espace ») : FilterGroups pose ce contexte, SwitchRow le lit.
export const InFilters = createContext(false);
