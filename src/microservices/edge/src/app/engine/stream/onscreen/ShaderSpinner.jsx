import React from "react";
import { useTranslation } from "react-i18next";
import { ROUND_BUTTON_LOOK } from "../shared/hooks/use-streaming-menu.js";

// LA ROUE DE COMPILATION DES SHADERS (chantier 50, `#658`) : quand l'émulateur compile, en haut à gauche de l'écran,
// un anneau pâle, un arc blanc qui tourne et le nombre de shaders déjà compilés dans la rafale — la même pour toutes
// les consoles, la partie la pousse par son socket (message `shaders`). Elle reprend la roue que Kyty dessinait dans
// l'image (tools/kyty 0058 : un arc sur 216°, un tour en 1,26 s), dans le rond des boutons de la page
// (ROUND_BUTTON_LOOK, celui du bouton du menu) : une forme définie une fois. Elle tourne par une animation CSS (celle
// des spinners de Bootstrap), sans rendu React par image, et même quand l'image du jeu est figée.
const RADIUS = 19;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;
const ARC = (216 / 360) * CIRCUMFERENCE;

export default function ShaderSpinner({ count, style }) {
    const { t } = useTranslation();
    const fontSize = count >= 1000 ? 13 : count >= 100 ? 16 : 19;

    return (
        <div role="status" aria-label={t("play.shaders", { count })} data-shader-spinner=""
            className="position-absolute pe-none rounded-circle d-flex align-items-center justify-content-center"
            style={{ ...ROUND_BUTTON_LOOK, ...style }}>
            <svg viewBox="0 0 48 48" className="position-absolute top-0 start-0 w-100 h-100" aria-hidden="true"
                style={{ animation: "spinner-border 1.26s linear infinite" }}>
                <circle cx="24" cy="24" r={RADIUS} fill="none" stroke="rgba(255, 255, 255, 0.25)" strokeWidth="3" />
                <circle cx="24" cy="24" r={RADIUS} fill="none" stroke="#fff" strokeWidth="3" strokeLinecap="round"
                    strokeDasharray={`${ARC} ${CIRCUMFERENCE}`} />
            </svg>
            <span className="fw-semibold text-white" style={{ fontSize: `${fontSize}px`, lineHeight: 1 }}>{count}</span>
        </div>
    );
}
