import React from "react";
import { helpLines, SUBTITLES } from "../globals/subtitles.jsx";

// LES TUILES : les gestes les plus fréquents en tête d'un panneau ou d'une page, deux par rangée ; un nombre impair
// de tuiles, et la dernière prend la rangée (`wide` la force). Une tuile pleine est un réglage allumé (`on`, annoncé
// par aria-pressed) ou le geste mis en avant (`primary`, Reprendre, Nouveau membre). Ce sont des boutons : 3 px, la
// croix y circule.
export function QuickTiles({ items, separator = SUBTITLES.separator, noKey = SUBTITLES.noKey }) {
    const shown = items.filter(Boolean);

    return (
        <div className="gf-tiles mt-3">
            {shown.map((item, index) => (
                <button key={item.id} type="button"
                    className={`btn gf-tile tone-${item.tone || "slate"} ${item.on || item.primary ? "on" : ""} ${item.wide || (shown.length % 2 === 1 && index === shown.length - 1) ? "wide" : ""}`}
                    data-nav data-key={item.id} aria-pressed={item.on === undefined ? undefined : Boolean(item.on)} disabled={item.disabled} onClick={item.onClick}>
                    <i className={`bi ${item.icon}`} aria-hidden="true"></i>
                    <span className="min-w-0">
                        <span className="gf-tile-label d-block">{item.label}</span>
                        {helpLines(item.value, Infinity, separator, noKey) && <span className="gf-tile-value d-block">{helpLines(item.value, Infinity, separator, noKey)}</span>}
                    </span>
                </button>
            ))}
        </div>
    );
}
