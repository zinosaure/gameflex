// LA DISPOSITION — choisie au chargement et à chaque redimensionnement.
//
// Quatre dispositions qui ne changent QUE des jetons (styles/launcher.css),
// comme la table du README de la proposition E :
//   - tv       : l'agent dit téléviseur, ou aucun pointeur sur un écran ≥ 960 px.
//                Jamais la seule largeur : un PC en 1920 × 1080 n'est pas un téléviseur ;
//   - portrait : la hauteur l'emporte ;
//   - short    : paysage et moins de 540 px de haut (téléphone en paysage) ;
//   - land     : sinon, la disposition de référence.
// L'échelle (1rem) suit l'écran, bornée. `?layout=tv|land|short|portrait` force.

import { useEffect, useState } from "react";

const TV_AGENT = /Android TV|GoogleTV|Google TV|\bAFT[A-Z]|BRAVIA|SMART-TV|SmartTV|Tizen|webOS|HbbTV|CrKey/i;
const LAYOUTS = ["tv", "portrait", "short", "land"];
const clamp = (value, low, high) => Math.max(low, Math.min(high, value));

const forcedLayout = () => {
    const forced = new URLSearchParams(location.search).get("layout");

    if (LAYOUTS.includes(forced)) {
        try {
            sessionStorage.setItem("gameflex-edge-layout", forced);
        } catch {
            // Sans stockage, le forçage ne vaut que pour cette adresse.
        }

        return forced;
    }

    try {
        return sessionStorage.getItem("gameflex-edge-layout");
    } catch {
        return null;
    }
};

export function applyLayout() {
    const root = document.documentElement;
    const width = innerWidth, height = innerHeight;
    const noPointer = matchMedia("(pointer: none)").matches;
    const kind = forcedLayout() || (TV_AGENT.test(navigator.userAgent) || (noPointer && width >= 960)
        ? "tv"
        : height > width ? "portrait" : height < 540 ? "short" : "land");
    const scale = kind === "portrait" ? clamp(width / 390, 0.9, 1.4)
        : kind === "short" ? clamp(height / 390, 0.82, 1.15)
            : kind === "tv" ? 1.75 * Math.min(width / 1920, height / 1080)
                : clamp(Math.min(width / 1366, height / 768), 0.85, 2.1);

    const changed = !root.classList.contains(kind);
    LAYOUTS.forEach((name) => root.classList.toggle(name, name === kind));
    root.style.fontSize = (16 * scale).toFixed(2) + "px";

    // Au doigt (pointeur grossier), les aides des boutons disparaissent ; une
    // touche ou la manette les ramène (input.js).
    if (kind === "tv") {
        root.classList.remove("touch");
    } else if (matchMedia("(pointer: coarse)").matches) {
        root.classList.add("touch");
    }

    if (changed) {
        window.dispatchEvent(new CustomEvent(LAYOUT_EVENT, { detail: kind }));
    }

    return kind;
}

// La disposition, pour un composant qui change de STRUCTURE avec elle (la fiche
// d'un jeu : le logo au-dessus en portrait, à droite en paysage) — les jetons,
// eux, suivent seuls par les classes de <html>.
const LAYOUT_EVENT = "gameflex-edge:layout";
const currentKind = () => LAYOUTS.find((name) => document.documentElement.classList.contains(name)) || "land";

export function useLayoutKind() {
    const [kind, setKind] = useState(currentKind);

    useEffect(() => {
        const onChange = () => setKind(currentKind());
        window.addEventListener(LAYOUT_EVENT, onChange);
        onChange();

        return () => window.removeEventListener(LAYOUT_EVENT, onChange);
    }, []);

    return kind;
}

export const currentLayout = () => LAYOUTS.find((name) => document.documentElement.classList.contains(name)) || "land";
