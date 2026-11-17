// LES VISUELS GÉNÉRÉS — ce qui s'affiche quand le backend n'a aucune image.
//
// GameFlex ne livre aucune illustration sous droits : une console a son
// visuel abstrait (des formes et sa couleur, jamais un logo), un jeu sans image
// son affiche générée (une teinte tirée de son identifiant, son titre, le nom
// court de sa console). Repris de la maquette de la proposition E
// (docs/reports/2026-11-11-launcher-propositions/E-une-console-par-ecran/).
// Le hasard est reproductible : une console, un jeu donnent toujours la même image — et la vitrine d'En direct,
// les mêmes jeux tout un jour (Live.jsx).

export const hash = (text) => {
    let value = 2166136261;

    for (let index = 0; index < text.length; index++) {
        value = Math.imul(value ^ text.charCodeAt(index), 16777619);
    }

    return value >>> 0;
};

export const randomFrom = (seed) => {
    let value = seed || 1;

    return () => (value = (Math.imul(value, 1664525) + 1013904223) >>> 0) / 4294967296;
};

const svgUrl = (viewBox, body, crisp = false) => {
    const svg = `<svg xmlns='http://www.w3.org/2000/svg' viewBox='${viewBox}' preserveAspectRatio='xMidYMid slice'${crisp ? " shape-rendering='crispEdges'" : ""}>${body}</svg>`;

    return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
};

// p = [fond sombre, fond clair, accent, lumière] ; toile 400 × 480.
const gradients = (p) => `<defs><linearGradient id='bg' x1='0' y1='0' x2='.4' y2='1'><stop offset='0' stop-color='${p[1]}'/><stop offset='1' stop-color='${p[0]}'/></linearGradient><radialGradient id='gl'><stop offset='0' stop-color='${p[2]}' stop-opacity='.75'/><stop offset='1' stop-color='${p[2]}' stop-opacity='0'/></radialGradient><linearGradient id='lt' x1='0' y1='0' x2='0' y2='1'><stop offset='0' stop-color='${p[3]}'/><stop offset='1' stop-color='${p[2]}'/></linearGradient></defs><rect width='400' height='480' fill='url(#bg)'/>`;

const MOTIFS = {
    fins: (p, r) => {
        let out = `<circle cx='250' cy='170' r='230' fill='url(#gl)' opacity='.6'/>`;

        for (let i = 0; i < 5; i++) {
            const x = 70 + i * 62 + r() * 10, h = 230 + r() * 120;
            out += `<path d='M${x} ${400 - h} q 34 ${h / 2} 0 ${h} q -16 ${-h / 2} 0 ${-h}z' fill='url(#lt)' opacity='${(0.25 + i * 0.13).toFixed(2)}'/>`;
        }

        return out;
    },
    stack: (p) => {
        let out = `<circle cx='200' cy='150' r='240' fill='url(#gl)' opacity='.5'/>`;

        for (let i = 0; i < 5; i++) {
            const y = 120 + i * 46;
            out += `<polygon points='${60 + i * 6},${y + 40} ${110 + i * 6},${y} ${350 - i * 6},${y} ${300 - i * 6},${y + 40}' fill='${i % 2 ? p[2] : p[3]}' opacity='${(0.18 + i * 0.12).toFixed(2)}'/>`;
        }

        return out;
    },
    arcs: (p) => {
        let out = `<circle cx='60' cy='420' r='260' fill='url(#gl)' opacity='.5'/>`;

        for (let i = 0; i < 7; i++) {
            out += `<circle cx='40' cy='440' r='${90 + i * 46}' fill='none' stroke='${i % 3 ? p[2] : p[3]}' stroke-opacity='${(0.55 - i * 0.06).toFixed(2)}' stroke-width='${i % 2 ? 3 : 10}'/>`;
        }

        return out;
    },
    split: (p) => `<circle cx='200' cy='190' r='220' fill='url(#gl)' opacity='.55'/><rect x='70' y='70' width='118' height='260' rx='52' fill='${p[2]}' transform='rotate(-10 130 200)'/><rect x='212' y='92' width='118' height='260' rx='52' fill='${p[3]}' opacity='.9' transform='rotate(-10 270 220)'/><circle cx='122' cy='150' r='18' fill='${p[0]}' opacity='.5'/><circle cx='282' cy='270' r='18' fill='${p[0]}' opacity='.5'/>`,
    tablet: (p) => `<circle cx='200' cy='200' r='230' fill='url(#gl)' opacity='.6'/><rect x='60' y='90' width='280' height='200' rx='44' fill='${p[3]}' opacity='.9'/><rect x='110' y='122' width='180' height='130' rx='14' fill='${p[2]}'/><circle cx='86' cy='190' r='10' fill='${p[0]}' opacity='.5'/><circle cx='314' cy='170' r='9' fill='${p[0]}' opacity='.5'/>`,
    orb: (p) => `<circle cx='200' cy='190' r='240' fill='url(#gl)'/><circle cx='200' cy='190' r='120' fill='url(#lt)' opacity='.9'/><circle cx='200' cy='190' r='150' fill='none' stroke='${p[3]}' stroke-opacity='.5' stroke-width='4'/><circle cx='160' cy='150' r='36' fill='#fff' opacity='.25'/>`,
    ringlight: (p) => {
        let out = `<circle cx='200' cy='190' r='230' fill='url(#gl)' opacity='.55'/>`;
        const rad = (deg) => deg * Math.PI / 180;

        for (let i = 0; i < 4; i++) {
            const a = i * 90 + 8, b = a + 74;
            out += `<path d='M${(200 + 120 * Math.cos(rad(a))).toFixed(1)} ${(190 + 120 * Math.sin(rad(a))).toFixed(1)} A120 120 0 0 1 ${(200 + 120 * Math.cos(rad(b))).toFixed(1)} ${(190 + 120 * Math.sin(rad(b))).toFixed(1)}' fill='none' stroke='${p[3]}' stroke-width='18' stroke-linecap='round' opacity='${i === 0 ? 1 : 0.4}'/>`;
        }

        return out + `<circle cx='200' cy='190' r='64' fill='${p[1]}' stroke='${p[2]}' stroke-width='3'/>`;
    },
    chip: (p, r) => {
        let out = `<circle cx='200' cy='200' r='230' fill='url(#gl)' opacity='.45'/><rect x='130' y='120' width='140' height='140' rx='18' fill='${p[1]}' stroke='${p[2]}' stroke-width='4'/>`;

        for (let i = 0; i < 6; i++) {
            const y = 136 + i * 22;
            out += `<path d='M130 ${y} H ${60 - r() * 30} V ${y + (r() > 0.5 ? 40 : -40)}' stroke='${p[2]}' stroke-width='3' fill='none' opacity='.6'/><path d='M270 ${y} H ${340 + r() * 30}' stroke='${p[3]}' stroke-width='3' opacity='.5'/>`;
        }

        return out;
    },
    pixels: (p, r) => {
        let out = "";

        for (let y = 0; y < 400; y += 40) {
            for (let x = 0; x < 400; x += 40) {
                const d = (x + (400 - y)) / 800 + r() * 0.25;

                if (d > 0.55) {
                    out += `<rect x='${x}' y='${y}' width='40' height='40' fill='${d > 0.85 ? p[3] : p[2]}' opacity='${Math.min(1, d - 0.3).toFixed(2)}'/>`;
                }
            }
        }

        return out;
    },
    dots: (p) => {
        let out = `<circle cx='280' cy='130' r='240' fill='url(#gl)' opacity='.5'/>`;

        for (let y = 20; y < 420; y += 30) {
            for (let x = 20; x < 420; x += 30) {
                const k = Math.max(0, 1 - Math.hypot(x - 300, y - 120) / 380);
                out += `<circle cx='${x}' cy='${y}' r='${(1 + k * 11).toFixed(1)}' fill='${k > 0.6 ? p[3] : p[2]}' opacity='${(0.3 + k * 0.6).toFixed(2)}'/>`;
            }
        }

        return out;
    },
    trio: (p) => `<circle cx='200' cy='200' r='230' fill='url(#gl)' opacity='.35'/><g style='mix-blend-mode:screen'><circle cx='150' cy='170' r='92' fill='${p[2]}' opacity='.85'/><circle cx='250' cy='170' r='92' fill='${p[3]}' opacity='.8'/><circle cx='200' cy='256' r='92' fill='#e0567a' opacity='.75'/></g>`,
    lcd: (p) => {
        let out = "";

        for (let y = 40; y < 420; y += 18) {
            for (let x = 20; x < 400; x += 18) {
                const hill = 300 - 90 * Math.sin(x / 70) - 40 * Math.cos(x / 31);
                const on = y > hill || (y < 120 && Math.hypot(x - 300, y - 90) < 40);
                out += `<rect x='${x}' y='${y}' width='14' height='14' rx='2' fill='${on ? p[2] : p[1]}' opacity='${on ? 0.95 : 0.35}'/>`;
            }
        }

        return out;
    },
    capsule: (p) => `<circle cx='200' cy='190' r='230' fill='url(#gl)' opacity='.5'/><rect x='30' y='130' width='340' height='130' rx='65' fill='${p[3]}' opacity='.9'/><rect x='110' y='150' width='180' height='90' rx='10' fill='${p[2]}'/><circle cx='72' cy='195' r='16' fill='${p[0]}' opacity='.45'/><circle cx='328' cy='195' r='16' fill='${p[0]}' opacity='.45'/>`,
    panes: (p) => `<circle cx='200' cy='200' r='230' fill='url(#gl)' opacity='.5'/><rect x='95' y='50' width='210' height='140' rx='18' fill='${p[3]}' opacity='.85'/><rect x='95' y='206' width='210' height='140' rx='18' fill='${p[2]}' opacity='.9'/><rect x='120' y='72' width='160' height='96' rx='6' fill='${p[0]}' opacity='.35'/>`,
    cube: (p) => `<circle cx='200' cy='200' r='230' fill='url(#gl)' opacity='.55'/><polygon points='200,70 320,140 200,210 80,140' fill='${p[3]}'/><polygon points='80,140 200,210 200,350 80,280' fill='${p[2]}'/><polygon points='320,140 200,210 200,350 320,280' fill='${p[2]}' opacity='.6'/><polygon points='200,150 250,180 200,210 150,180' fill='${p[0]}' opacity='.4'/>`,
    wave: (p) => {
        let out = `<circle cx='200' cy='180' r='230' fill='url(#gl)' opacity='.5'/>`;

        for (let i = 0; i < 9; i++) {
            let d = `M -10 ${120 + i * 26}`;

            for (let x = 0; x <= 420; x += 20) {
                d += ` L ${x} ${(120 + i * 26 + Math.sin(x / 50 + i * 0.6) * 22).toFixed(1)}`;
            }
            out += `<path d='${d}' fill='none' stroke='${i % 2 ? p[2] : p[3]}' stroke-width='${i === 4 ? 6 : 2.5}' opacity='${(0.35 + (i === 4 ? 0.6 : 0.05 * i)).toFixed(2)}'/>`;
        }

        return out;
    },
    stripes: (p) => {
        let out = `<circle cx='300' cy='120' r='230' fill='url(#gl)' opacity='.5'/>`;

        for (let i = 0; i < 6; i++) {
            out += `<polygon points='${-80 + i * 70},480 ${40 + i * 70},480 ${300 + i * 70},0 ${180 + i * 70},0' fill='${i % 2 ? p[2] : p[3]}' opacity='${(0.15 + i * 0.12).toFixed(2)}'/>`;
        }

        return out;
    },
    rings: (p) => `<circle cx='200' cy='190' r='230' fill='url(#gl)' opacity='.5'/><circle cx='200' cy='190' r='92' fill='url(#lt)'/><ellipse cx='200' cy='190' rx='170' ry='40' fill='none' stroke='${p[3]}' stroke-width='10' opacity='.75' transform='rotate(-18 200 190)'/><ellipse cx='200' cy='190' rx='196' ry='50' fill='none' stroke='${p[2]}' stroke-width='3' opacity='.6' transform='rotate(-18 200 190)'/>`,
    spiral: (p) => {
        let d = "M200 190";

        for (let a = 0; a < 18; a += 0.15) {
            const radius = 6 + a * 8.4;
            d += ` L ${(200 + Math.cos(a) * radius).toFixed(1)} ${(190 + Math.sin(a) * radius).toFixed(1)}`;
        }

        return `<circle cx='200' cy='190' r='230' fill='url(#gl)' opacity='.5'/><path d='${d}' fill='none' stroke='${p[2]}' stroke-width='12' stroke-linecap='round'/>`;
    },
    disc: (p) => {
        let out = `<circle cx='200' cy='190' r='230' fill='url(#gl)' opacity='.4'/><circle cx='200' cy='190' r='150' fill='${p[1]}'/>`;

        for (let i = 0; i < 9; i++) {
            out += `<circle cx='200' cy='190' r='${40 + i * 12}' fill='none' stroke='${p[3]}' stroke-opacity='${(0.08 + i * 0.03).toFixed(2)}' stroke-width='5'/>`;
        }

        return out + `<path d='M200 190 L 340 120 A150 150 0 0 1 350 200z' fill='${p[2]}' opacity='.45'/><circle cx='200' cy='190' r='26' fill='${p[0]}'/>`;
    },
    monolith: (p) => `<circle cx='240' cy='170' r='230' fill='url(#gl)' opacity='.6'/><polygon points='150,40 260,20 260,400 150,420' fill='${p[0]}' stroke='${p[1]}' stroke-width='2'/><polygon points='260,20 300,40 300,400 260,400' fill='${p[1]}'/><rect x='254' y='60' width='6' height='300' fill='${p[2]}'/><rect x='170' y='210' width='70' height='4' fill='${p[3]}' opacity='.4'/>`,
    neon: (p) => {
        let out = `<rect y='0' width='400' height='230' fill='${p[1]}'/><circle cx='200' cy='210' r='110' fill='url(#lt)'/>`;

        for (let i = 0; i < 6; i++) {
            out += `<rect x='80' y='${150 + i * 14}' width='240' height='${3 + i}' fill='${p[1]}'/>`;
        }
        out += `<rect y='228' width='400' height='252' fill='${p[0]}'/>`;

        for (let i = -10; i <= 10; i++) {
            out += `<line x1='200' y1='228' x2='${200 + i * 60}' y2='480' stroke='${p[2]}' stroke-width='2' opacity='.7'/>`;
        }

        for (let i = 1; i < 8; i++) {
            const y = 228 + Math.pow(i / 7, 2) * 252;
            out += `<line x1='0' y1='${y.toFixed(0)}' x2='400' y2='${y.toFixed(0)}' stroke='${p[2]}' stroke-width='2' opacity='.6'/>`;
        }

        return out;
    },
    blocks: (p, r) => {
        let out = `<circle cx='200' cy='190' r='230' fill='url(#gl)' opacity='.45'/>`;

        for (let i = 0; i < 14; i++) {
            const w = 60 + r() * 90, x = r() * 340, y = 40 + r() * 320;
            out += `<rect x='${x.toFixed(0)}' y='${y.toFixed(0)}' width='${w.toFixed(0)}' height='${(w * 0.42).toFixed(0)}' rx='8' fill='${r() > 0.5 ? p[2] : p[3]}' opacity='${(0.25 + r() * 0.6).toFixed(2)}'/>`;
        }

        return out;
    },
    bands: (p) => {
        let out = `<circle cx='200' cy='160' r='230' fill='url(#gl)' opacity='.4'/>`;

        [p[3], p[2], "#e0567a", "#7a5cff", "#3ccf8e"].forEach((color, i) => {
            out += `<rect x='0' y='${170 + i * 26}' width='400' height='16' fill='${color}' opacity='.85'/>`;
        });

        return out;
    },
    device: (p) => `<circle cx='200' cy='200' r='240' fill='url(#gl)' opacity='.55'/><rect x='70' y='120' width='260' height='160' rx='18' fill='none' stroke='${p[3]}' stroke-width='6' opacity='.75'/><path d='M150 320 h100' stroke='${p[3]}' stroke-width='6' stroke-linecap='round' opacity='.6'/><path d='M178 170 v60 l52-30z' fill='${p[2]}'/><path d='M300 70 a60 60 0 0 1 60 60 M300 92 a38 38 0 0 1 38 38' fill='none' stroke='${p[2]}' stroke-width='6' stroke-linecap='round'/>`,
};

// Les consoles de la maquette gardent leur motif et leur couleur ; une console
// que cette table ne connaît pas en reçoit un tiré de son identifiant. Le
// registre (`GET /api/v1/consoles`) reste la seule source de ce qu'EST une
// console : cette table ne dit que comment la dessiner.
const LOOKS = {
    ps5: ["fins", ["#0a0d1c", "#20284a", "#8fa8ff", "#f4f6ff"]],
    ps4: ["stack", ["#06111d", "#123050", "#4a8ae6", "#d3e6ff"]],
    ps3: ["arcs", ["#0c0c10", "#2a2a33", "#a3a3b5", "#eeeef4"]],
    switch: ["split", ["#1b1026", "#3d1f4f", "#f2b33d", "#ffe9b8"]],
    switch2: ["split", ["#0f1a26", "#1f3a4f", "#3dd0f2", "#d8f6ff"]],
    wiiu: ["tablet", ["#0a1b20", "#134650", "#59d0d6", "#e6fbff"]],
    xbox: ["orb", ["#140c07", "#3a2410", "#ff9a3d", "#ffe2c0"]],
    xbox360: ["ringlight", ["#0c0f14", "#232e3b", "#c9d4e3", "#ffffff"]],
    ports: ["chip", ["#0d0c14", "#26223a", "#a78bfa", "#ede9fe"]],
    nes: ["pixels", ["#171213", "#3a2b2c", "#e0645a", "#f1e3d3"]],
    snes: ["dots", ["#13111c", "#302a4a", "#9b8cff", "#e8e4ff"]],
    n64: ["trio", ["#0f0f16", "#272740", "#43c59e", "#ffd25a"]],
    gbc: ["lcd", ["#0e180e", "#2b3d1f", "#9bbc0f", "#e0f8d0"]],
    gba: ["capsule", ["#140f2a", "#2d2063", "#7b6cff", "#e9e4ff"]],
    nds: ["panes", ["#0f1418", "#283440", "#9ad0ff", "#f0f8ff"]],
    gc: ["cube", ["#120f24", "#30285e", "#8f7bff", "#f3f0ff"]],
    wii: ["wave", ["#0e1417", "#28343b", "#7fd3ff", "#ffffff"]],
    megadrive: ["stripes", ["#0b0b0b", "#2a2a2a", "#ff5a4d", "#f5f5f5"]],
    sms: ["blocks", ["#0f0f12", "#2c2c36", "#ff7a3d", "#ffe6d6"]],
    saturn: ["rings", ["#0c0e1a", "#24304f", "#7e8cff", "#e0e5ff"]],
    dreamcast: ["spiral", ["#121212", "#303030", "#ff8a3d", "#fbfbfb"]],
    ps1: ["disc", ["#121418", "#2e333c", "#c0c6d0", "#ffffff"]],
    ps2: ["monolith", ["#05060c", "#161c33", "#3e6dff", "#d9e3ff"]],
    psp: ["capsule", ["#0d0d10", "#26262f", "#9aa0b4", "#f0f2f8"]],
    neogeo: ["neon", ["#12061a", "#3a1450", "#ff4fd8", "#ffd6f5"]],
    pce: ["blocks", ["#0e1112", "#28343a", "#ff9f1c", "#fff3d6"]],
    atari2600: ["bands", ["#140c06", "#3a2410", "#ff7b2e", "#ffe0b2"]],
    c64: ["bands", ["#10162b", "#2a3561", "#8aa0ff", "#d5ddff"]],
};
const MOTIF_NAMES = Object.keys(MOTIFS).filter((name) => name !== "device");

const lookOf = (id, isDevice = false) => {
    if (isDevice) {
        return ["device", ["#0b1220", "#1c2c48", "#7fb2ff", "#e6f0ff"]];
    }

    if (LOOKS[id]) {
        return LOOKS[id];
    }
    const seed = hash(id);
    const hue = seed % 360;

    return [MOTIF_NAMES[seed % MOTIF_NAMES.length], [`hsl(${hue} 35% 8%)`, `hsl(${hue} 35% 22%)`, `hsl(${(hue + 20) % 360} 70% 64%)`, `hsl(${hue} 60% 92%)`]];
};

const artCache = new Map();

export const consoleArt = (id, isDevice = false) => {
    const key = (isDevice ? "device:" : "") + id;

    if (!artCache.has(key)) {
        const [motif, palette] = lookOf(id, isDevice);
        artCache.set(key, svgUrl("0 0 400 480", gradients(palette) + MOTIFS[motif](palette, randomFrom(hash(id))), motif === "pixels" || motif === "lcd"));
    }

    return artCache.get(key);
};

// L'affiche générée d'un jeu : une teinte tirée de son identifiant, toujours la même.
export const posterBackground = (seed) => {
    const hue = hash(seed + "poster") % 360;

    return `repeating-linear-gradient(135deg, rgba(255,255,255,.035) 0 2px, transparent 2px 16px), radial-gradient(120% 90% at 15% 0%, hsl(${hue} 42% 30%) 0%, transparent 62%), radial-gradient(90% 70% at 100% 100%, hsl(${(hue + 40) % 360} 40% 20%) 0%, transparent 70%), linear-gradient(160deg, hsl(${hue} 30% 14%), hsl(${(hue + 25) % 360} 28% 7%))`;
};

// « Contra (J) [o1] » : les mentions d'un nom de fichier passent en petit
// sous le titre, rien n'est retiré (plan du chantier 51).
export const splitTitle = (title) => {
    const match = /^(.*?)((?:\s*[([][^)\]]*[)\]])+)\s*$/.exec(title || "");

    return match && match[1] ? { main: match[1], tags: match[2].trim() } : { main: title || "", tags: "" };
};
