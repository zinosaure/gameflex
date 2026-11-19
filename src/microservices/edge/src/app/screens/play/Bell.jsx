import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ROUND_BUTTON_LOOK } from "../../engine/stream/shared/hooks/use-streaming-menu.js";

// LES NOTIFICATIONS D'UNE PARTIE (chantier 68 ; le porteur, 2026-11-19 : « pendant le streaming, quand on reçoit une
// demande […] un petit icône "notification bell" en haut à droite avec une opacité à 50% va apparaître avec un son de
// cloche. Cela évitera d'avoir directement le modal affiché sur l'écran pour ne pas déranger le joueur », puis « le
// notification bell remplace le toast qu'on a actuellement »).
//
// CE QUI PARLE DES AUTRES passe par ici, plus par un avis par-dessus le jeu : les demandes reçues (la manette, ma place
// demandée par mon autre appareil), les arrivées et les départs, les réponses à mes demandes. Chacune :
//   - fait sonner la cloche (si le son de la partie est ouvert) et la montre en haut à droite, à moitié transparente —
//     5 s pour une information, le temps qu'elle reste valable pour une demande (jusqu'à sa réponse, ou son délai) ;
//   - s'ajoute en tête de la section Notifications du menu, où elle reste toute la partie (5 par page).
// Une demande (`asks`) se répond depuis le menu : sa ligne ouvre la confirmation. Sans réponse à son échéance
// (`expiresAt`), `onExpire` la règle (garder ma place, par exemple) et la ligne dit « sans réponse ».

const INFO_MS = 5000;

// LE SON DE LA CLOCHE : deux notes synthétisées par la page, aucun fichier à servir.
let audio = null;
function chime() {
    try {
        audio = audio || new (window.AudioContext || window.webkitAudioContext)();
        const start = audio.currentTime;

        [1318.5, 1760].forEach((frequency, index) => {
            const at = start + index * 0.14;
            const tone = audio.createOscillator();
            const gain = audio.createGain();
            tone.type = "sine";
            tone.frequency.value = frequency;
            gain.gain.setValueAtTime(0.0001, at);
            gain.gain.exponentialRampToValueAtTime(0.2, at + 0.01);
            gain.gain.exponentialRampToValueAtTime(0.0001, at + 0.9);
            tone.connect(gain).connect(audio.destination);
            tone.start(at);
            tone.stop(at + 1);
        });
    } catch {
        // Pas de son (un navigateur qui le refuse) : la cloche se voit quand même.
    }
}

// `soundRef` : le son de la partie est-il ouvert (une référence : la page la tient à jour après avoir lu ses réglages).
export function useSessionNotifications(soundRef) {
    const [list, setList] = useState([]);
    const [infoUntil, setInfoUntil] = useState(0);
    const [, setTick] = useState(0);
    const seq = useRef(0);
    const listRef = useRef(list);
    listRef.current = list;

    // `entry` : { title, subtitle, asks?, expiresAt?, onConfirm?, onCancel?, onExpire?, confirm? } ; rend son id.
    const push = useCallback((entry) => {
        seq.current += 1;
        const id = seq.current;
        setList((current) => [{ id, at: Date.now(), state: entry.asks ? "pending" : "info", ...entry }, ...current]);

        if (!entry.asks) {
            setInfoUntil(Date.now() + INFO_MS);
        }
        if (soundRef.current !== false) {
            chime();
        }

        return id;
    }, []);
    // RÉGLER UNE DEMANDE EN ATTENTE (`state` : « accepted », « refused », « expired ») ; une demande déjà réglée ne change plus.
    const settle = useCallback((id, patch) => setList((current) => current.map((entry) => (entry.id === id && entry.state === "pending" ? { ...entry, ...patch } : entry))), []);
    const answer = useCallback((id, yes) => {
        const entry = listRef.current.find((item) => item.id === id);

        if (!entry || entry.state !== "pending") {
            return;
        }
        settle(id, { state: yes ? "accepted" : "refused" });
        (yes ? entry.onConfirm : entry.onCancel)?.();
    }, [settle]);

    // L'ÉCHÉANCE D'UNE DEMANDE : une minuterie par demande en attente, aucune cadence.
    const pending = list.filter((entry) => entry.state === "pending" && entry.expiresAt);
    const pendingKey = pending.map((entry) => `${entry.id}:${entry.expiresAt}`).join(",");
    useEffect(() => {
        const timers = pending.map((entry) => setTimeout(() => {
            settle(entry.id, { state: "expired" });
            entry.onExpire?.();
        }, Math.max(0, entry.expiresAt - Date.now())));

        return () => timers.forEach(clearTimeout);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [pendingKey, settle]);

    // LA CLOCHE : tant qu'une demande attend, ou 5 s après une information ; une minuterie la retire à l'heure dite.
    const asking = list.some((entry) => entry.state === "pending");
    const bellVisible = asking || Date.now() < infoUntil;
    useEffect(() => {
        const left = infoUntil - Date.now();

        if (left <= 0) {
            return undefined;
        }
        const timer = setTimeout(() => setTick((tick) => tick + 1), left + 20);

        return () => clearTimeout(timer);
    }, [infoUntil]);

    return useMemo(() => ({ list, push, settle, answer, bellVisible }), [list, push, settle, answer, bellVisible]);
}

// LA CLOCHE, en haut à droite, à 50 % : la toucher ouvre le menu, où sont les notifications. 35 % plus petite que le
// bouton du menu (le porteur, 2026-11-19 : « un peu plus petit […] -35 % de la taille actuelle ») : elle ne touche plus
// les gâchettes de la manette à l'écran.
const BELL_SIZE = "calc(3rem * 0.65)";
export function Bell({ visible, onOpen }) {
    const { t } = useTranslation();

    if (!visible) {
        return null;
    }

    return (
        <button type="button" className="play-round position-absolute" data-key="pw-bell" aria-label={t("play.notifications.title")} title={t("play.notifications.title")}
            style={{ ...ROUND_BUTTON_LOOK, top: "calc(12px + env(safe-area-inset-top, 0px))", right: "max(12px, env(safe-area-inset-right))", zIndex: 1003, opacity: 0.5, width: BELL_SIZE, height: BELL_SIZE, fontSize: "calc(1.4rem * 0.65)", background: undefined, border: undefined }}
            onClick={onOpen}>
            <i className="bi bi-bell-fill"></i>
        </button>
    );
}
