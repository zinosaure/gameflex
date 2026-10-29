import { useEffect, useState } from "react";
import { kernelReachable, kernelTarget, useKernelChannel, useKernelTopic } from "../../../libs/kernelChannel.js";

// useLaunchProgress — ce que le serveur a fait pendant qu'on attendait (#191).
//
// Entre le clic sur « jouer » et la première image, une dizaine d'étapes se
// déroulent côté serveur — dont l'attente qu'un conteneur réponde, jusqu'à
// quatre-vingt-dix secondes.
//
// UN SUJET DU CENTRE DE MESSAGES, `launch/<launchId>`. Le lancement lui-même
// est une commande sur le même canal : la page des jeux reçoit ses étapes en
// direct, puis navigue ici avec le `launchId` que le kernel a frappé. Le
// kernel garde l'histoire une minute après la fin et la REJOUE à
// l'abonnement — une page qui arrive après la dernière étape, ou qu'on
// rafraîchit, voit tout le lancement.
//
// Le téléphone rend un `launchId` (pour le fond de l'intro, chantier 35) mais
// ne sert pas ce sujet : sa progression passe par son flux de nouvelles
// (`deviceStream.js`), et l'abonnement refusé laisse la liste vide.
/**
 * withStep — la liste des étapes avec une de plus. Une étape republiée à la
 * suite remplace la dernière au lieu de s'empiler : la progression d'un
 * déballage (game.unpacking, « 40 % », « 50 % »…) tient sur une ligne.
 *
 * @param {Array} steps les étapes déjà reçues
 * @param {object} entry la nouvelle
 * @returns {Array} la liste suivante
 */
export function withStep(steps, entry) {
    const last = steps[steps.length - 1];

    return last && last.step === entry.step && last.state !== "error"
        ? [...steps.slice(0, -1), entry]
        : [...steps, entry];
}

export function useLaunchProgress(launchId, token) {
    const [steps, setSteps] = useState([]);
    const [failure, setFailure] = useState(null);
    const [sessionId, setSessionId] = useState(null);

    useEffect(() => {
        setSteps([]);
        setFailure(null);
        setSessionId(null);
    }, [launchId, token]);

    const channel = useKernelChannel(launchId && kernelReachable(token) ? kernelTarget(null, token) : null);

    useKernelTopic(channel, launchId ? `launch/${launchId}` : null, null, (message) => {
        if (!message) {
            return;
        }

        // L'ORDRE D'ARRIVÉE EST L'ORDRE DES ÉTAPES : le rejeu de l'histoire
        // se fait avant toute nouvelle publication. Seule une étape republiée
        // à la suite — une progression — remplace la précédente (withStep).
        setSteps((previous) => withStep(previous, {
            step: message.step,
            detail: message.detail || null,
            state: message.type === "failed" ? "error" : "done",
            at: message.at || null,
        }));

        if (message.type === "failed") {
            setFailure({ step: message.step, error: message.error || null });
        }
        if (message.type === "done" && message.sessionId) {
            setSessionId(message.sessionId);
        }
    });

    return { steps, failure, sessionId };
}
