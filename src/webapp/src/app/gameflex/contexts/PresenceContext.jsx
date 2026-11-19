import React, { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { useAuth } from "../profile/auth/AuthContext.jsx";
import { homeWsUrl } from "../libs/serverBase.js";

// ONE persistent WebSocket for the whole app session (mounted once in
// index.js's AppShell, not per-page) — carries both directions: the server
// pushes the live "who's online/playing" list, and any component can send
// this user's own state changes over the SAME connection via
// reportPlaying()/reportIdle() (see Streaming.jsx). No REST
// counterpart, no polling — see python/src/app/presence/routes.py.
const PresenceContext = createContext(null);

export function PresenceProvider({ children }) {
    const { token, isAuthenticated } = useAuth();
    const [users, setUsers] = useState([]);
    // Invitations received LIVE over this same connection (see
    // push/routes.py's send_invite — the WS delivery half, Web Push is the
    // separate closed-tab fallback). Accumulated, not replaced — several can
    // arrive before any is acted on. Each gets a client-side `id` (not
    // server-provided, these are ephemeral and never persisted) so
    // NotificationBell can dismiss one specifically.
    const [invites, setInvites] = useState([]);
    const wsRef = useRef(null);

    useEffect(() => {
        if (!isAuthenticated || !token) {
            setUsers([]);
            return;
        }

        let cancelled = false;
        let ws = null;
        let reconnectTimer = null;

        const connect = () => {
                        // Query param, not a header — browsers can't set custom headers
            // on a WebSocket handshake (see presence/routes.py's own doc).
            ws = new WebSocket(homeWsUrl(`/api/v1/presence/ws?token=${encodeURIComponent(token)}`));
            wsRef.current = ws;

            ws.onmessage = (evt) => {
                let msg;
                try {
                    msg = JSON.parse(evt.data);
                } catch {
                    return;
                }
                if (msg.type === "list") setUsers(msg.users || []);
                if (msg.type === "invite") {
                    setInvites((prev) => [...prev, { ...msg, id: `${Date.now()}-${Math.random()}` }]);
                }
            };

            ws.onclose = () => {
                if (wsRef.current === ws) wsRef.current = null;
                if (cancelled) return;
                reconnectTimer = setTimeout(connect, 3000);
            };
        };

        connect();

        return () => {
            cancelled = true;
            clearTimeout(reconnectTimer);
            ws?.close();
            wsRef.current = null;
        };
    }, [isAuthenticated, token]);

    // Best-effort — if the connection happens to be mid-reconnect right
    // when this is called, the report is just dropped (presence is a live
    // status display, not persisted data; PlayHistory, which IS persisted,
    // is reported separately over plain REST — see profile/routes.py).
    // role — "player" (default, an owner or a joiner) or "spectator" (a
    // watch-only connection) — see presence/registry.py's PresenceEntry.role.
    const reportPlaying = useCallback((session, isWatching) => {
        if (wsRef.current?.readyState === WebSocket.OPEN) {
            wsRef.current.send(JSON.stringify({
                type: "playing",
                sessionId: session.id,
                platform: session.platform,
                consoleId: session.consoleId,
                reference: session.reference || null,
                role: isWatching ? "spectator" : "player"
            }));
        }
    }, []);

    const reportIdle = useCallback(() => {
        if (wsRef.current?.readyState === WebSocket.OPEN) {
            wsRef.current.send(JSON.stringify({ type: "idle" }));
        }
    }, []);

    // Called once the user has acted on (or dismissed) an invite — see
    // NotificationBell (index.js).
    const dismissInvite = useCallback((id) => {
        setInvites((prev) => prev.filter((i) => i.id !== id));
    }, []);

    return (
        <PresenceContext.Provider value={{ users, invites, reportPlaying, reportIdle, dismissInvite }}>
            {children}
        </PresenceContext.Provider>
    );
}

export function usePresence() {
    return useContext(PresenceContext);
}
