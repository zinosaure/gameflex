import React from "react";
import { useGameImage } from "../../libs/images.js";
import { posterBackground } from "../../libs/visuals.js";
import { Chip } from "../../ui/Bricks.jsx";

// LES IMAGES D'UN JEU, DÉFINIES ICI UNE FOIS : son visuel (la fiche, l'engrenage), son logo sur la fiche, sa jaquette en
// tête d'une ligne. Les sortes sont celles du kernel (background, screenshot, cover, icon, logo) ; rien n'est inventé.

// LE VISUEL D'UN JEU — la règle de repli du plan 51, celle de sa fiche : le fond (background), sinon une capture, sinon
// la jaquette ou l'icône floutée sur toute la surface, sinon un dégradé tiré de l'identifiant, le titre en filigrane.
// `front` : ce qui se pose au centre du visuel de l'engrenage — le logo, sinon l'icône, sinon la jaquette (le porteur,
// 2026-11-14, chantier 53, P12 : « en premier on va rajouter le background et par dessus le logo/icône du jeu au centre de
// celui-ci. Ce qui fait que même si on n'a que le background ou juste l'icône on verra toujours quelque chose. ») ; il
// n'est demandé qu'avec `withFront`. `source` : la source du jeu (libs/api.js, romSource).
export function useGameVisual(consoleId, reference, source, title, width, withFront = false) {
    const background = useGameImage(consoleId, reference, "background", width, true, source);
    const screenshot = useGameImage(consoleId, reference, "screenshot", width, background === null, source);
    const logo = useGameImage(consoleId, reference, "logo", 1280, true, source);
    const unsharp = background === null && screenshot === null;
    const frontless = withFront && logo === null;
    const cover = useGameImage(consoleId, reference, "cover", 512, unsharp || frontless, source);
    const icon = useGameImage(consoleId, reference, "icon", 512, (unsharp && cover === null) || frontless, source);
    const sharp = background || (background === null ? screenshot : null);
    const framed = cover || (cover === null ? icon : null);
    let visual = <div className="vis-bg" style={{ background: posterBackground(reference) }}><span className="wm">{title}</span></div>;

    if (sharp) {
        visual = <div className="vis-bg photo" style={{ backgroundImage: `url("${sharp}")` }}></div>;
    } else if (framed) {
        visual = <><div className="vis-bg blur" style={{ backgroundImage: `url("${framed}")` }}></div><div className="vis-panel" style={{ backgroundImage: `url("${framed}")` }}></div></>;
    }

    return { visual, sharp, framed, logo, front: logo || (logo === null ? icon || (icon === null ? cover : null) : null) };
}

// LE VISUEL EN TÊTE DE L'ENGRENAGE D'UN JEU : son fond sur toute la largeur (16:9), le logo ou l'icône au centre, par-dessus.
// Il se voit toujours : un fond seul, un logo seul (sur le dégradé du jeu), ou les deux — jamais un bloc vide.
export function GameBanner({ consoleId, reference, source, title }) {
    const { sharp, framed, front } = useGameVisual(consoleId, reference, source, title, 1280, true);
    let back = <div className="vis-bg" style={{ background: posterBackground(reference) }}>{!front && <span className="wm">{title}</span>}</div>;

    if (sharp) {
        back = <div className="vis-bg photo" style={{ backgroundImage: `url("${sharp}")` }}></div>;
    } else if (framed) {
        back = <div className="vis-bg blur" style={{ backgroundImage: `url("${framed}")` }}></div>;
    }

    return (
        <div className="gbanner ratio ratio-16x9 rounded-4 overflow-hidden" aria-hidden="true">
            <div>
                {back}
                {front && (
                    <div className="position-absolute top-0 start-0 w-100 h-100 d-flex align-items-center justify-content-center p-3">
                        <img className="gbanner-front" src={front} alt="" />
                    </div>
                )}
            </div>
        </div>
    );
}

// LE LOGO D'UN JEU SUR SA FICHE — sa place et ses bornes, définies ici une fois : en
// portrait, en tête du bloc (au-dessus du texte) ; en paysage, à droite, centré dans la
// hauteur libre. Bornes (styles/launcher.css, `.slogo`) : 60 % de la largeur et 20 % de
// la hauteur en portrait, 35 % de la largeur et la hauteur libre en paysage, les
// proportions gardées.
export default function HeroArt({ url, portrait }) {
    if (!url) {
        return null;
    }

    return (
        <div className={`slogo d-flex flex-column ${portrait ? "order-first align-items-start" : "flex-grow-1 align-items-end justify-content-center"}`}>
            <img className="flogo" src={url} alt="" />
        </div>
    );
}

// LA JAQUETTE D'UN JEU EN TÊTE D'UNE LIGNE (les jeux d'une console dans les Réglages, la liste de la recherche) : sa
// jaquette, sinon la pastille d'un disque. `game` : sa source (libs/api.js, romSource).
export function GameCover({ consoleId, reference, game }) {
    const image = useGameImage(consoleId, reference, "cover", 256, true, game);

    return image ? <img className="gf-cover rounded object-fit-cover" src={image} alt="" /> : <Chip icon="bi-disc" tone="slate" />;
}
