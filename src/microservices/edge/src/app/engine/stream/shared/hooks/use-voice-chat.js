import { useCallback, useEffect, useRef, useSyncExternalStore } from "react";
import { readStoredBool, store } from "../../../libs/gamepadPreferences.js";

// LA VOIX D'UNE PARTIE, CÔTÉ PAGE (chantier 71 ; le porteur, 2026-11-23 : « Le micro et le son des joueurs, c'est eux
// qui décident. Le micro et le son des spectateurs, c'est le owner qui décide s'ils peuvent interagir »).
//
// La file de la partie (gameflex/session, voice.go) décide et le dit dans son état : `you.voice` (mon micro, si
// j'entends, si je peux m'en servir et sinon pourquoi, et — au propriétaire — si je peux ouvrir la conversation aux
// spectateurs) et `voice` (la conversation ouverte aux spectateurs, qui parle). La passerelle ne relaie que ce que la
// file permet. La page :
//   - ouvre le micro (getUserMedia, avec l'annulation d'écho du navigateur) AVANT de le demander à la file, pour que la
//     permission du navigateur se pose d'abord ; refusé par la file, il se referme ;
//   - le branche sur sa piste sans renégocier (GatewayWebRTC.setMicrophone) ;
//   - le referme dès que la file le ferme (la conversation refermée, plus personne à qui parler) ;
//   - garde « Entendre les autres » par navigateur, vrai par défaut (le porteur, 2026-11-23), et le redit à la file.

const HEAR_KEY = "gameflex_voice_hear";
let hear = readStoredBool(HEAR_KEY, true);
const hearListeners = new Set();

function setStoredHear(on) {
    hear = Boolean(on);
    store(HEAR_KEY, hear);
    hearListeners.forEach((listener) => listener());
}

function subscribeHear(listener) {
    hearListeners.add(listener);

    return () => hearListeners.delete(listener);
}

// microphoneAvailable — le navigateur donne-t-il un micro à cette page ? Seulement sur une page sécurisée (https, ou
// l'application) : sur `http://<adresse>`, il le refuse avant même de demander.
export const microphoneAvailable = () => typeof window !== "undefined" && Boolean(window.isSecureContext && navigator.mediaDevices?.getUserMedia);

async function openMicrophone() {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } });

    return stream.getAudioTracks()[0] || null;
}

// useVoiceChat — `state` : la voix dite par le dernier état de la file (`{ room, you }`, null avant le premier) ;
// `intent(action, target)` : un geste à la file ; `screenRef` : la surface vidéo (setMicrophone) ; `onRefused(reason)` :
// dire pourquoi un geste n'a pas pris.
export function useVoiceChat({ enabled, state, intent, screenRef, onRefused }) {
    const hearing = useSyncExternalStore(subscribeHear, () => hear, () => true);
    const trackRef = useRef(null);
    const pendingRef = useRef(false);
    const toldHearRef = useRef(false);
    const you = state?.you || null;

    const closeMicrophone = useCallback(() => {
        trackRef.current?.stop();
        trackRef.current = null;
        screenRef.current?.setMicrophone?.(null);
    }, [screenRef]);

    // MA PRÉFÉRENCE « ENTENDRE » redite à la file dès son premier état : elle ouvre chaque onglet à « entendre ».
    useEffect(() => {
        if (!enabled || !you || toldHearRef.current) {
            return;
        }
        toldHearRef.current = true;
        if (you.hear !== hearing) {
            intent("voice-hear", hearing ? "on" : "off");
        }
    }, [enabled, you, hearing, intent]);

    // LA FILE A LE DERNIER MOT : un micro qu'elle ferme se ferme ici (sauf pendant mon propre geste, que son résultat
    // tranche) ; un micro qu'elle croit ouvert sans piste ici se referme chez elle.
    useEffect(() => {
        if (!you || pendingRef.current) {
            return;
        }
        if (!you.mic && trackRef.current) {
            closeMicrophone();
        } else if (you.mic && !trackRef.current) {
            intent("voice-mic", "off");
        }
    }, [you, intent, closeMicrophone]);

    useEffect(() => closeMicrophone, [closeMicrophone]);

    const toggleMic = useCallback(async (on) => {
        if (!on) {
            closeMicrophone();
            intent("voice-mic", "off");

            return;
        }
        if (!microphoneAvailable() || trackRef.current) {
            return;
        }
        pendingRef.current = true;
        try {
            trackRef.current = await openMicrophone();
        } catch (failure) {
            // LA VRAIE RAISON, pas « refusé » pour tout (le porteur, 2026-11-23 : le téléphone disait « le navigateur
            // n'a pas donné le micro » alors que la permission l'était — le micro ne DÉMARRAIT pas).
            pendingRef.current = false;
            onRefused?.(failure?.name === "NotAllowedError" || failure?.name === "SecurityError" ? "denied" : failure?.name === "NotFoundError" ? "missing" : "busy");

            return;
        }
        await screenRef.current?.setMicrophone?.(trackRef.current);
        if (!intent("voice-mic", "on")) {
            pendingRef.current = false;
            closeMicrophone();
        }
    }, [intent, screenRef, closeMicrophone, onRefused]);

    // LE RÉSULTAT DE MON GESTE (PlayShell, handleChildResult) : refusé, le micro se referme et la page dit pourquoi.
    const onResult = useCallback((message) => {
        if (message.action === "voice-mic") {
            pendingRef.current = false;
            if (message.status !== "granted") {
                closeMicrophone();
                onRefused?.(message.reason || "");
            }
        }
    }, [closeMicrophone, onRefused]);

    const toggleHear = useCallback((on) => {
        setStoredHear(on);
        intent("voice-hear", on ? "on" : "off");
    }, [intent]);

    const toggleSpectators = useCallback((on) => intent("voice-spectators", on ? "on" : "off"), [intent]);

    return { room: state?.room || null, you, hearing, microphone: microphoneAvailable(), toggleMic, toggleHear, toggleSpectators, onResult };
}
