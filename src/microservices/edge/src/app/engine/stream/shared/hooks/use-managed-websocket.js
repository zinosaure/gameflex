import { useCallback, useRef } from "react";

// useManagedWebSocket — the WebSocket plumbing retro-gaming's and
// streamex's connection hooks each hand-rolled (JSON send/parse, the
// identity guard that stops a replaced socket's onclose from clobbering
// the newer connection's state, silent teardown). Knows NOTHING about any
// platform protocol — every handler below is caller-supplied and the
// platform's own message switch stays in its own hook.
//
// openManaged — creates a WebSocket, records it as the current one, and
// wires the handlers. `sendJSON` is identity-guarded the same way: a
// superseded socket can never send again. Returns the socket so a caller
// that needs raw string frames (streamex's input tokens) can still reach
// it directly through wsRef.
//
// closeManaged — silent teardown of the CURRENT socket: handlers detached
// before close() so cleanup can't fire state transitions on a deliberate
// disconnect (retro's disconnect()/leavePage() and streamex's effect
// cleanup both rely on exactly that).
export function useManagedWebSocket() {
    const wsRef = useRef(null);
    const sendJSONRef = useRef(null);

    const openManaged = useCallback((url, { onOpen, onMessage, onClose, onError } = {}) => {
        const socket = new WebSocket(url);
        wsRef.current = socket;

        const sendJSON = (message) => {
            if (wsRef.current === socket && socket.readyState === WebSocket.OPEN) {
                socket.send(JSON.stringify(message));
            }
        };
        sendJSONRef.current = sendJSON;

        socket.onopen = () => onOpen?.(sendJSON);

        socket.onmessage = (event) => {
            let message = null;
            try {
                message = JSON.parse(event.data);
            } catch {
                return;
            }
            onMessage?.(message, sendJSON);
        };

        socket.onclose = () => {
            // Identity guard — a socket replaced mid-close (take-over
            // retry) must not fire the current connection's close path.
            if (wsRef.current === socket) wsRef.current = null;
            const replaced = wsRef.current !== null;
            if (!replaced) onClose?.();
        };

        socket.onerror = () => onError?.();

        return socket;
    }, []);

    const closeManaged = useCallback(() => {
        const socket = wsRef.current;
        if (!socket) return;
        wsRef.current = null;
        socket.onclose = null;
        socket.onerror = null;
        sendJSONRef.current = null;
        try {
            socket.close();
        } catch {
            // already closing — nothing to do
        }
    }, []);

    return { wsRef, sendJSONRef, openManaged, closeManaged };
}
