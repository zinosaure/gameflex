import React from "react";
import { GROUP_HEADING } from "../libs/ui.js";
import { SENSES, SenseTone } from "../globals/sections.js";

// UN GROUPE : son intitulé (mb-3 sous lui, ou sous son aide), son aide, puis son contenu — des lignes dans
// `Rows`, ou ce qui s'y pose seul (un texte, des boutons, des vignettes). `badge` : ce qui suit l'intitulé.
// NI TRAIT SOUS LA SECTION, NI INTITULÉ EN CAPITALES : le commit 8cf5e34d les avait posés avant le choix de la
// direction C ; le porteur a validé C telle que dessinée le 2026-11-13 (« très bien comme ça ») — les groupes
// sont des conteneurs arrondis, leur intitulé est en minuscules.
// `sense` : ce que fait la section (globals/sections.js) — la couleur des pastilles de ses lignes.
export function Section({ title = null, help = null, badge = null, sense = null, className = "", children }) {
    return (
        <section className={`mt-4 ${className}`}>
            {title && <h2 className={GROUP_HEADING}>{title}{badge && <span className="ms-2 align-middle">{badge}</span>}</h2>}
            {help && <p className="gf-group-help mb-3">{help}</p>}
            {sense ? <SenseTone.Provider value={SENSES[sense]}>{children}</SenseTone.Provider> : children}
        </section>
    );
}
