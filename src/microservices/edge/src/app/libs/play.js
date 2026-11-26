import { useCallback, useEffect, useRef, useState } from "react";
import { createSession } from "../engine/api/retrogaming.js";
import { launchSession, stopSession } from "../engine/api/spawnSessions.js";
import { can, loadHostCapabilities } from "./capabilities.js";
import { federatedId } from "./federatedHost.js";
import { withStep } from "../engine/stream/shared/hooks/use-launch-progress.js";
import { withNetworkBitrate } from "../engine/stream/shared/libs/network-probe.js";
import { connectedGamepads } from "../engine/libs/keyMapping.js";
import { menuGestureOf } from "../engine/libs/gameInput.js";
import { explained, readToken } from "./api.js";
import { getStoredUsername } from "./sharedHost.js";

// LANCER UNE PARTIE ET L'OUVRIR — la page de jeu du launcher (chantier 52), à la place de celle de la
// webapp.
//
//   /play/<session>             la page d'une partie : on y arrive en spectateur ;
//     ?play=1                   … pour y jouer : l'INTENTION de prendre la manette, demandée une
//                               fois à la session (le child en décide), puis retirée de l'adresse —
//                               un lien recopié de la barre reste un lien pour regarder ;
//     ?launch=<id>              … juste lancée d'ici : ses étapes, déjà commencées, s'y poursuivent
//                               jusqu'à la première image du jeu (#191), sur le fond de l'intro ;
//     ?state=<emplacement>      … l'état à charger une fois l'émulateur parti (rétro : le moteur le
//                               lit dans l'adresse, use-retrogaming-streaming-connection.js).
//
// D'où l'on vient voyage dans l'historique (`state.back`), pas dans l'adresse : quitter y ramène.

export function playPath(sessionId, { play = false, launch = null, state = null } = {}) {
    const params = new URLSearchParams();

    if (play) {
        params.set("play", "1");
    }
    if (launch) {
        params.set("launch", launch);
    }
    if (state != null && state !== "") {
        params.set("state", String(state));
    }
    const query = params.toString();

    return `/play/${encodeURIComponent(sessionId)}${query ? `?${query}` : ""}`;
}

// LE LANCEMENT, DÈS LE CLIC (#287) : la commande `launch` du canal du kernel, dont les étapes arrivent
// pendant qu'on attend sa réponse — un conteneur qui démarre se compte en secondes. L'écran les montre
// (screens/play/Launch.jsx), puis la page de jeu les reprend. Les mêmes appels que la webapp : `createSession`
// pour un jeu rétro, `launchSession` pour une console récente — sauf chez un hôte qui ne fait pas
// naître de conteneurs (un téléphone, `spawnsSessions` faux), où la console récente part comme un jeu
// rétro (son émulateur est embarqué).
//
// ANNULER pendant l'attente n'interrompt pas le kernel : la partie qui en naît est arrêtée dès sa
// réponse.
//
// `start({ console, rom, core, bios, settings, username, host })` rend `{ ok, sessionId, launchId }`, ou
// `{ ok: false, errorMessage, conflict }` — `conflict` : la partie qu'on joue déjà ailleurs.
//
// `settings` — le flux de la console dans ce navigateur (`streamOf`, GameSettings.jsx). Un débit « Auto » s'y mesure
// ici, l'écran du lancement déjà là, vers le serveur qui servira la partie : le nôtre, ou l'ami chez qui elle part
// (`withNetworkBitrate`, le porteur, 2026-11-16 : « Le auto s'align sur le débit du réseau »).
//
// `host` — LE JEU EST CHEZ UN AMI, C'EST LUI QUI LE LANCE (C9-P5) : `{serverId, origin, token, username}`,
// notre compte de chez lui. Même commande, à qui on la demande : la référence est LA SIENNE (sans notre
// préfixe `srv<id>:`), le nom celui qu'on porte chez lui, et la session qui en naît repart préfixée — deux
// serveurs frappent les mêmes identifiants. Pas de suivi du lancement (son kernel, pas le nôtre).
export function useLaunch() {
    const [screen, setScreen] = useState(null);
    const runRef = useRef(null);

    const start = useCallback(async ({ console, rom, core, bios, settings, username, host = null }) => {
        const run = { cancelled: false, failure: null };
        runRef.current = run;
        setScreen({ title: rom.title, steps: [], failure: null, backdrop: null });
        const update = (change) => {
            if (runRef.current === run) {
                setScreen((current) => (current ? { ...current, ...change(current) } : current));
            }
        };

        await loadHostCapabilities();
        const spawns = console.method === "streamex" && (Boolean(host) || can("spawnsSessions"));
        const platform = spawns ? "streamex" : "retrogaming";
        // LE FOND D'ABORD (chantier 35) : le premier message donne l'identifiant du lancement — l'écran
        // se branche sur son flux, où la passerelle tient l'image de l'intro.
        const progress = {
            onProgress: (message, frame) => {
                if (frame?.launchId) {
                    update((current) => (current.backdrop ? {} : { backdrop: { launch: { streamId: frame.launchId, host: host ? { origin: host.origin, token: host.token } : null, username: (host ? host.username : getStoredUsername()) || null } } }));
                }
                if (!message) {
                    return;
                }
                if (message.type === "failed") {
                    run.failure = { step: message.step, error: explained(message.errorCode || "", message.error || "") || null };
                }
                update((current) => ({
                    steps: withStep(current.steps, { step: message.step, detail: message.detail || null, state: message.type === "failed" ? "error" : "done", at: message.at || null }),
                    failure: message.type === "failed" ? run.failure : current.failure,
                }));
            },
        };
        const reference = host ? String(rom.reference).replace(/^srv:?\d+:/, "") : rom.reference;
        const stream = await withNetworkBitrate(settings, host);

        if (run.cancelled) {
            return { ok: false, cancelled: true };
        }
        const result = spawns
            ? await launchSession(rom.title, console.id, readToken(), stream, host, { ...progress, core })
            : await createSession(console.id, reference, rom.title, core || "", bios || "", host ? host.username : username, host, stream, progress);
        const sessionId = result?.ok ? (result.data?.sessionId || result.data?.session_id) : null;

        if (run.cancelled) {
            if (sessionId) {
                stopSession(platform, sessionId, host ? host.token : readToken(), host).catch(() => {});
            }

            return { ok: false, cancelled: true };
        }

        if (!sessionId) {
            // Une étape en erreur garde l'écran (la croix, « Signaler ») ; un refus sans étape se dit.
            const conflict = result?.activeSessionId ? { id: result.activeSessionId, platform: result.activeSessionPlatform, title: result.activeSessionTitle } : null;

            if (!run.failure) {
                update(() => ({ failure: { step: null, error: result?.errorMessage || null }, conflict }));
            }

            return { ok: false, errorMessage: result?.errorMessage || "", conflict };
        }

        return { ok: true, sessionId: host ? federatedId(host.serverId, sessionId) : sessionId, launchId: host ? null : result.launchId || null };
    }, []);

    const cancel = useCallback(() => {
        if (runRef.current) {
            runRef.current.cancelled = true;
        }
        runRef.current = null;
        setScreen(null);
    }, []);

    const close = useCallback(() => {
        runRef.current = null;
        setScreen(null);
    }, []);

    return { screen, start, cancel, close };
}

// LE MENU D'UNE PARTIE, À LA MANETTE (le geste : libs/gameInput.js, menuGestureOf) — Home seul en rétro, un appui ;
// L2 + R2 + Home tenus deux secondes pour streamex et le Remote Play. Tant que le geste est tenu, Home n'atteint pas le
// jeu (isMenuHome : les trois plateformes le lisent comme lâché).
//
// Rend l'avancement (0 à 1) d'un geste tenu, pour que la page le montre ; il faut relâcher pour réarmer. Toutes les
// manettes branchées comptent, pas les manettes virtuelles du serveur (connectedGamepads les écarte). Échap ouvre le
// menu sur-le-champ (PlayShell.jsx).
export function useMenuHotkey(enabled, onOpen, platform) {
    const [progress, setProgress] = useState(0);
    const onOpenRef = useRef(onOpen);
    onOpenRef.current = onOpen;

    useEffect(() => {
        if (!enabled) {
            setProgress(0);

            return undefined;
        }
        const { buttons, holdMs } = menuGestureOf(platform);
        let raf;
        let since = null;
        let armed = true;
        let shown = 0;
        const show = (value) => {
            if (value !== shown) {
                shown = value;
                setProgress(value);
            }
        };
        const tick = () => {
            const held = connectedGamepads().some((pad) => pad && buttons.every((index) => pad.buttons?.[index]?.pressed));
            const now = performance.now();

            if (!held) {
                since = null;
                armed = true;
                show(0);
            } else if (armed) {
                since ??= now;
                // Par pas de 2 % : cinquante positions pour l'anneau, pas un rendu par image. Un appui (holdMs 0) : aussitôt.
                const value = holdMs > 0 ? Math.min(1, Math.floor(((now - since) / holdMs) * 50) / 50) : 1;
                show(holdMs > 0 ? value : 0);

                if (value >= 1) {
                    armed = false;
                    since = null;
                    show(0);
                    onOpenRef.current();
                }
            }
            raf = requestAnimationFrame(tick);
        };
        raf = requestAnimationFrame(tick);

        return () => cancelAnimationFrame(raf);
    }, [enabled, platform]);

    return progress;
}
