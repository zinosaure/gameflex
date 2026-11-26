import React, { useState } from "react";
import { Carousel } from "react-bootstrap";
import { useTranslation } from "react-i18next";
import { gamePath, useImage } from "../libs/images.js";

// LES IMAGES D'UN JEU, EN TÊTE DE SA PAGE (le porteur, 2026-11-25 : « tu as oublié de rajouter l'image du jeu à la place
// de TODO » ; sa mise en forme : « Preview of the game (e.g., cover image, basic info as a carousel) ») : le carrousel de
// Bootstrap — celui de l'accueil « En direct » (Live.jsx) —, une image par sorte que la fiche a (la jaquette d'abord,
// puis le fond, l'écran-titre, une capture, le logo), entière sur un fond sombre ; on glisse au doigt, ou on touche un des
// tirets dessous. Le temps de la lecture, la place vide ; sans aucune image, « Aucune image ». `game` : sa source
// (romSource).
const KINDS = ["cover", "background", "title", "screenshot", "logo"];
const WIDTH = 640;

export function GameImages({ consoleId, reference, game = {} }) {
    const { t } = useTranslation();
    const [active, setActive] = useState(0);
    // Une lecture par sorte, toujours dans le même ordre (les règles des hooks).
    // Un jeu sans adresse d'image (une ressource externe) n'en attend pas : « Aucune image », pas une attente sans fin.
    const images = KINDS.map((kind) => {
        const path = consoleId && reference ? gamePath(consoleId, reference, kind, WIDTH, game) : null;
        const image = useImage(path);

        return { kind, url: path ? (image ? image.url : image) : null };
    });
    const found = images.filter((image) => image.url);
    const reading = images.some((image) => image.url === undefined);
    const shown = Math.min(active, Math.max(found.length - 1, 0));

    return (
        <div className="gf-game-images mt-4">
            {found.length ? (
                <Carousel activeIndex={shown} onSelect={setActive} interval={null} controls={false} indicators={false} keyboard={false} touch={found.length > 1}>
                    {found.map((image) => (
                        <Carousel.Item key={image.kind}>
                            <div className="ratio ratio-16x9 rounded-4 overflow-hidden bg-black">
                                <img className="object-fit-contain" src={image.url} alt={t(`gameAdmin.meta.kinds.${image.kind}`)} />
                            </div>
                        </Carousel.Item>
                    ))}
                </Carousel>
            ) : (
                <div className="ratio ratio-16x9 rounded-4 overflow-hidden bg-body-secondary">
                    <span className="d-flex align-items-center justify-content-center small text-body-tertiary">{reading ? "" : t("appearance.noImage")}</span>
                </div>
            )}
            {found.length > 1 && (
                <div className="carousel-indicators position-static m-0 mt-3">
                    {found.map((image, index) => (
                        <button key={image.kind} type="button" data-bs-target="" className={index === shown ? "active" : ""} aria-current={index === shown || undefined}
                            aria-label={t(`gameAdmin.meta.kinds.${image.kind}`)} onClick={() => setActive(index)}></button>
                    ))}
                </div>
            )}
        </div>
    );
}
