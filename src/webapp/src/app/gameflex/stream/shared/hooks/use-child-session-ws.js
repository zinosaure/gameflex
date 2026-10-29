import { useCallback, useEffect, useRef, useMemo } from "react";
import { wsUrl } from "../../../libs/serverBase.js";

// useChildSessionWs — the socket a tab holds to the container that RUNS its
// session (chantier-4), reached through the kernel proxy so the page stays on
// one origin.
//
// It carries four things:
//   - PRESENCE: open means the tab is open; closed means it is gone. That is
//     the whole liveness story for a departure — nothing to announce;
//   - the BEAT, every 30s, with hadInput. The message arriving proves the
//     tab's JS still runs (a frozen tab keeps its socket open, so the socket
//     alone can never tell); hadInput proves a human is behind it. Two
//     meanings, one message, never conflated;
//   - the tab's GATEWAY VIEWER id, so the child can gate emulator input on
//     the players queue — a tab holds two identities and only it knows both;
//   - its INTENTS (P3, chantier 12): `{type:"intent", action, target}`. Still
//     declared, never inferred from a transport event — which is where nearly
//     every control bug of this project came from. They used to be POSTed
//     beside this socket, naming it by an id that travelled in the clear;
//     sent on it, they speak for this tab by construction.
//
// The RESULT of an intent comes back here too, because the state it produces
// has several recipients (the requester, the holder, every watcher) and a
// second copy travelling down another channel is exactly the divergence this
// chantier removes.
export const CHILD_BEAT_INTERVAL_MS = 30_000;

function childBase(sessionId) {
    return `/api/v1/sessions/${encodeURIComponent(sessionId)}/child`;
}

export function useChildSessionWs({ sessionId, token, enabled, onState, onResult, onAfk, onConnect, onDisconnect }) {
    const socketIdRef = useRef(null);
    const socketRef = useRef(null);
    // The tab can learn its gateway viewer id BEFORE the socket opens — on
    // the phone everything is in-process, the viewer is usually ready first.
    // A lost announce silently disables input (the child gates every
    // keystroke on the pairing), so remember it and flush on open.
    const pendingViewerIdRef = useRef(null);
    // Callbacks live in a ref so an unstable inline identity from the caller
    // cannot reconnect the socket on every render.
    const callbacksRef = useRef({ onState, onResult, onAfk, onConnect, onDisconnect });
    callbacksRef.current = { onState, onResult, onAfk, onConnect, onDisconnect };

    useEffect(() => {
        socketIdRef.current = null;
        socketRef.current = null;

        if (!enabled || !sessionId || !token) {
            return undefined;
        }

        const socket = new WebSocket(
            wsUrl(`${childBase(sessionId)}/v4/ws?token=${encodeURIComponent(token)}`),
        );
        socketRef.current = socket;

        socket.onopen = () => {
            const pending = pendingViewerIdRef.current;

            if (pending) {
                pendingViewerIdRef.current = null;
                socket.send(JSON.stringify({ type: "viewer", viewerId: pending }));
            }

            callbacksRef.current.onConnect?.();
        };

        socket.onmessage = (event) => {
            let message;

            try {
                message = JSON.parse(event.data);
            } catch {
                return;
            }

            if (message.type === "welcome") {
                // The tab learns its OWN socket id, which is what it puts in
                // its intents. It is never told anybody else's.
                socketIdRef.current = message.socketId;
                return;
            }
            if (message.type === "state") {
                callbacksRef.current.onState?.(message);
                return;
            }
            if (message.type === "result") {
                callbacksRef.current.onResult?.(message);
                return;
            }
            // « TOUJOURS LÀ ? » — adressé à CE joueur, et à lui seul.
            //
            // Le child l'envoie sur cette socket-ci parce que c'est cette
            // manette-là qui n'a rien touché depuis vingt-neuf minutes. Les
            // autres jouent peut-être très bien : prévenir la tablée entière
            // est le meilleur moyen d'apprendre à ignorer le message.
            //
            // Rien à renvoyer : toucher n'importe quoi remet `hadInput` à
            // vrai dans le prochain battement, et le child efface la question
            // tout seul. Le compte à rebours n'est qu'un affichage — c'est le
            // child qui exclut, à l'échéance, et lui seul.
            if (message.type === "afk") {
                callbacksRef.current.onAfk?.(message);
            }
        };

        socket.onclose = () => {
            socketIdRef.current = null;
            socketRef.current = null;
            callbacksRef.current.onDisconnect?.();
        };

        return () => {
            socketRef.current = null;
            socket.close();
        };
    }, [sessionId, token, enabled]);

    const send = useCallback((message) => {
        const socket = socketRef.current;

        if (socket && socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify(message));
        }
    }, []);

    // announceViewer tells the child which gateway viewer belongs to this
    // tab. Without it the child cannot tell whose keystrokes are whose, and
    // gates nothing.
    const announceViewer = useCallback((viewerId) => {
        if (!viewerId) {
            return;
        }

        pendingViewerIdRef.current = viewerId;
        send({ type: "viewer", viewerId });
    }, [send]);

    const beat = useCallback((hadInput) => send({ type: "beat", hadInput: !!hadInput }), [send]);

    // intent — ONE request to the child, on this tab's own socket. False when
    // the socket has not said hello yet: there is nobody to ask, and the
    // caller decides whether to try again once the state lands.
    const intent = useCallback((action, target = null) => {
        if (!socketIdRef.current) {
            return false;
        }
        send(target ? { type: "intent", action, target } : { type: "intent", action });

        return true;
    }, [send]);

    // socketId is read through a getter rather than returned as state: it
    // arrives one message after the socket opens, and a re-render for it would
    // reconnect nothing while making every consumer depend on the timing.
    const getSocketId = useCallback(() => socketIdRef.current, []);

    // MEMOISED, and that is load-bearing, not tidiness. Callers put this
    // object in effect dependency arrays; a fresh literal every render made
    // the 30s beat interval restart on every render, and the child pushes
    // state every 15s — so the interval never once reached 30s and the beat
    // NEVER FIRED. The child then evicted a present player at 90s for not
    // beating, the queue emptied, and the session ended itself a few minutes
    // later while somebody was sitting right there. Measured 2026-08-25:
    // 0 beats sent, socket closed 102s after joining.
    return useMemo(
        () => ({ getSocketId, announceViewer, beat, intent, send }),
        [getSocketId, announceViewer, beat, intent, send],
    );
}
