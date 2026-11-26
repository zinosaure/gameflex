import React from "react";
import { RELAY_ICON } from "../globals/icons.js";
import { originOf } from "../libs/format.js";

// LA PROVENANCE D'UN JEU, À L'ÉCRAN (chantier 83 ; le porteur, 2026-11-26 : « la provenance on écrit : Gameflex (avec
// un icône relais) ») : originOf, suivie de l'icône de relais quand le serveur nous est relayé par un ami — son nom au
// survol. Les messages (un refus, une panne) gardent originOf, en texte.
export const relayOf = (game, servers) => (game.peer != null ? (servers || []).find((entry) => String(entry.id) === String(game.peer))?.relayedBy || null : null);

export function originView(game, servers, t) {
    const text = originOf(game, servers, t);
    const by = relayOf(game, servers);

    if (!by) {
        return text;
    }
    const said = t("fed.relayedBy", { name: by });

    return <span className="text-nowrap">{text} <i className={`bi ${RELAY_ICON}`} title={said} aria-label={said}></i></span>;
}
