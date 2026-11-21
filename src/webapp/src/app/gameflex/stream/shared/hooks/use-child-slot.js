import { useCallback, useEffect, useRef, useState } from "react";
import { useChildSessionWs, CHILD_BEAT_INTERVAL_MS } from "./use-child-session-ws.js";

// useChildSlot — hold a pad on a child-managed session, and nothing else.
//
// Since chantier-4 the child gates input on its own queue: a tab that never
// claims a slot can be connected, negotiated and streaming, and every button
// it sends is dropped. That is not session bookkeeping a simple page can skip
// — it IS the input path. Keytest learned this the hard way: the picture
// played, the pad rendered, and nothing happened (reported live 2026-08-25).
//
// Streaming.jsx runs a richer version of this (hand-over prompts, AFK, phases,
// end-of-session). This is the part every page needs whatever else it does:
// open the socket, name your viewer, ask for a seat, keep beating.
export function useChildSlot({ sessionId, token, enabled = true, viewerIdOf }) {
    const [slot, setSlot] = useState(null);
    const [ready, setReady] = useState(false);
    const claimedRef = useRef(false);

    const onState = useCallback((message) => {
        setSlot(message.you?.slotId ?? null);
        setReady(true);
    }, []);

    const socket = useChildSessionWs({
        sessionId,
        token,
        enabled,
        onState,
        onResult: () => { },
    });

    // The tab holds TWO identities — a gateway viewer and this socket — and
    // only the tab knows both. Without the pairing the child cannot tell whose
    // keystrokes are whose, and gates everything.
    const announceViewer = socket.announceViewer;
    useEffect(() => {
        if (!enabled || !viewerIdOf) {
            return undefined;
        }

        // The viewer is created asynchronously by the video surface; poll
        // briefly rather than demand the caller thread a callback through.
        const timer = setInterval(() => {
            const viewerId = viewerIdOf();
            if (viewerId) {
                announceViewer(viewerId);
                clearInterval(timer);
            }
        }, 300);

        return () => clearInterval(timer);
    }, [enabled, viewerIdOf, announceViewer]);

    // Ask for a seat, once, after the child has said where we stand — asking
    // before that is asking blind, and asking twice is asking to be refused.
    useEffect(() => {
        if (!enabled || !ready || claimedRef.current || slot != null) {
            return;
        }

        if (!socket.intent("join")) {
            return; // no socket yet — this reruns when the next state lands
        }

        claimedRef.current = true;
    }, [enabled, ready, slot, socket]);

    // Through a ref, so the interval's life is "as long as this page is on a
    // child-managed session" and NOT "as long as this callback keeps its
    // identity" — an interval rebuilt every render never reaches its period.
    const beatRef = useRef(null);
    beatRef.current = socket.beat;
    useEffect(() => {
        if (!enabled) {
            return undefined;
        }

        const timer = setInterval(() => beatRef.current?.(false), CHILD_BEAT_INTERVAL_MS);
        return () => clearInterval(timer);
    }, [enabled]);

    return { slot, ready };
}
