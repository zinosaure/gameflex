import { useEffect, useRef } from "react";
import { SAMPLE_MS, adaptQuality, createAdaptiveQuality, readVideoHealth } from "../../libs/adaptive-quality.js";

// LE DOUBLE FLUX DE LA PAGE (chantier 31) — le mode choisi dans la catégorie
// « Vidéo » du menu de la partie, appliqué au spectateur de la page :
//
//   auto     la page mesure son réseau toutes les deux secondes et bascule
//            (libs/adaptive-quality.js) ;
//   current  le flux de la partie, tel que son propriétaire l'a lancé ;
//   low      le 540p à 30.
//
// `quality` — ce que la passerelle a dit sur le canal « control » :
// `{available, current, want}`. Sans second flux (`available` faux), rien ne
// se fait : le menu ne propose d'ailleurs rien.
//
// `paused` — pas avant le jeu (chantier 35) : l'image tenue de l'intro arrive
// une fois par seconde, et ses relevés compteraient chacun pour un gel — le
// spectateur descendrait en 540p avant d'avoir vu une image du jeu. Les
// relevés repartent de zéro quand il y entre.
export function useAdaptiveQuality({ screenRef, mode, quality, paused = false }) {
    const available = !!quality?.available;
    const currentRef = useRef(quality?.current || "hd");
    currentRef.current = quality?.current || "hd";

    // Un choix à la main fixe le flux, et arrête la bascule.
    useEffect(() => {
        if (!available) {
            return;
        }
        if (mode === "current" && quality?.want !== "hd") {
            screenRef.current?.setQuality("hd");
        }
        if (mode === "low" && quality?.want !== "sd") {
            screenRef.current?.setQuality("sd");
        }
    }, [mode, available, quality?.want, screenRef]);

    useEffect(() => {
        if (mode !== "auto" || !available || paused) {
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
            const step = adaptQuality(state, sampled.reading, currentRef.current, Date.now());
            state = step.state;
            if (step.want) {
                screenRef.current?.setQuality(step.want);
            }
        }, SAMPLE_MS);

        return () => window.clearInterval(intervalId);
    }, [mode, available, paused, screenRef]);
}
