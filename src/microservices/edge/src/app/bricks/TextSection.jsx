import React from "react";
import { MUTED } from "../libs/ui.js";
import { Section } from "./Section.jsx";
import { EXTERNAL } from "../libs/links.js";

// UNE SECTION DE TEXTE (le porteur, 2026-11-16 : « les lignes de bon à savoir peuvent être que du texte avec titres.
// Comme pour Guide » ; « Et pour les guides. Enleve l'icône link et enleve les puces. Juste titre et texte aussi. Pas de
// différence avec bon à savoir, juste que le titre est un lien ») : le titre du groupe et son icône, puis chaque élément,
// son titre en gras (jamais en italique) et son texte dessous, au bord gauche, sans encadré ni puce. Un élément qui a `href` a son titre en
// lien (un guide, `GuideSection`) ; les autres sont du texte (« Bon à savoir »). `items` : [{ key, title, text, href }].
export function TextSection({ title, icon, help = null, items }) {
    return (
        <Section title={title} help={help} badge={<i className={`bi ${icon}`} aria-hidden="true"></i>}>
            {items.map((item, index) => (
                <div key={item.key} className={index ? "mt-3" : ""}>
                    {item.href ? (
                        <a href={item.href} {...EXTERNAL} className="fw-bold d-inline-block" data-nav data-key={item.key}>{item.title}</a>
                    ) : (
                        <span className="fw-bold d-block">{item.title}</span>
                    )}
                    <span className={`d-block ${MUTED}`}>{item.text}</span>
                </div>
            ))}
        </Section>
    );
}
