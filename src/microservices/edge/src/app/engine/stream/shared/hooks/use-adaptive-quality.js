import { useEffect, useRef } from "react";
import { SAMPLE_MS, adaptQuality, createAdaptiveQuality, readVideoHealth } from "../../libs/adaptive-quality.js";

// LES PALIERS DE LA PAGE (chantiers 31 et 58) — le mode choisi dans la catégorie
// « Vidéo » du menu de la partie, appliqué au spectateur de la page :
//
//   auto     la page mesure son réseau toutes les deux secondes et passe d'un
//            palier à l'autre (libs/adaptive-quality.js), au plus jusqu'au flux
//            de la partie ;
//   "0"…     un flux fixé : 0 celui de la partie, puis ses paliers.
//
// `quality` — ce que la passerelle a dit sur le canal « control » :
// `{streams, current, want}`. Un seul flux : rien ne se fait.
//
// `paused` — pas avant le jeu (chantier 35) : l'image tenue de l'intro arrive
// une fois par seconde, et ses relevés compteraient chacun pour un gel — le
// spectateur descendrait avant d'avoir vu une image du jeu. Les relevés
// repartent de zéro quand il y entre.
export function useAdaptiveQuality({ screenRef, mode, quality, paused = false }) {
    const lowest = Math.max(0, (quality?.streams?.length || 1) - 1);
    const currentRef = useRef(quality?.current || 0);
    currentRef.current = quality?.current || 0;

    // Un choix à la main fixe le flux, et arrête la bascule.
    useEffect(() => {
        if (lowest === 0 || mode === "auto") {
            return;
        }
        const fixed = Math.min(Number(mode) || 0, lowest);
        if (quality?.want !== fixed) {
            screenRef.current?.setQuality(fixed);
        }
    }, [mode, lowest, quality?.want, screenRef]);

    useEffect(() => {
        if (mode !== "auto" || lowest === 0 || paused) {
            return undefined;
        }
        let state = createAdaptiveQuality();
        let counters = null;
        const intervalId = window.setInterval(async () => {
            const report = await screenRef.current?.getRawStats();
            if (!report) {
                return;
            }
            const sampled = readVideoHealth(report, counters);
            counters = sampled.counters;
            if (!sampled.reading) {
                return;
            }
            const step = adaptQuality(state, sampled.reading, currentRef.current, lowest, Date.now());
            state = step.state;
            if (step.want !== null) {
                screenRef.current?.setQuality(step.want);
            }
        }, SAMPLE_MS);

        return () => window.clearInterval(intervalId);
    }, [mode, lowest, paused, screenRef]);
}
