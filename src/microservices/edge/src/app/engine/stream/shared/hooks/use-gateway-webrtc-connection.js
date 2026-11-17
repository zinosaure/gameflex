import { useCallback, useEffect, useState } from "react";

// The connection-state slice that use-retro-streaming-connection.js and
// use-streamex-streaming-connection.js each redeclared on their own after
// GatewayWebRTC.jsx absorbed the actual RTCPeerConnection/ICE/stats
// work (docs/wiggly-rolling-plum plan) — isConnected/isConnecting/
// connectionState/connectionStats and the two callbacks GatewayWebRTC
// is wired to (onConnectionStateChange/onStatsUpdate) were byte-for-byte
// duplicated modulo streamex having quietly dropped isConnecting/
// connectionState along the way (an unintentional divergence, not a
// deliberate difference — both platforms restore the same shape here).
//
// What this hook does NOT own: the WS protocol itself (start/join/session/
// offer/ice — different on the wire per platform, see each hook's own
// doc), or when to call setIsConnecting/setConnectionState at the START of
// a connection attempt (platform-specific: retro's own connect() sets it
// before opening its WS, streamex's connect effect has no such moment) —
// both are left as plain setters for the caller to drive.
//
// intentionalDisconnectRef — optional. When provided, a disconnected/
// failed/closed state from GatewayWebRTC is ignored while the ref is
// true (retro's take-over-retry dance replaces its own WS/peer mid-flight
// and doesn't want that transient state to flip isConnected off). Omitted
// by streamex, which has no such moment — every disconnected/failed/closed
// report is applied unconditionally, same as before this extraction.
export function useGatewayWebrtcConnection({ screenRef, t, intentionalDisconnectRef = null }) {
	const [isConnected, setIsConnected] = useState(false);
	const [isConnecting, setIsConnecting] = useState(false);
	const [connectionState, setConnectionState] = useState("");
	const [connectionStats, setConnectionStats] = useState(null);
	const [isStatsEnabled, setIsStatsEnabled] = useState(false);

	const setStatsMonitoring = useCallback((enabled) => setIsStatsEnabled(enabled), []);

	// GatewayWebRTC does its own stats polling once told to — this just
	// forwards the enabled flag and clears the last reading when turned off,
	// same as both hooks did individually before.
	useEffect(() => {
		screenRef?.current?.setStatsMonitoring(isStatsEnabled);
		if (!isStatsEnabled) setConnectionStats(null);
	}, [isStatsEnabled, screenRef]);

	const handleConnectionStateChange = useCallback(
		(state) => {
			switch (state) {
				case "connected":
					setIsConnected(true);
					setIsConnecting(false);
					setConnectionState(t("streaming.connection.state.connected"));
					break;
				case "disconnected":
				case "failed":
				case "closed":
					if (!intentionalDisconnectRef?.current) {
						setIsConnected(false);
						setIsConnecting(false);
					}
					break;
				default:
					break;
			}
		},
		[t, intentionalDisconnectRef],
	);

	const handleStatsUpdate = useCallback((stats) => setConnectionStats(stats), []);

	return {
		isConnected,
		isConnecting,
		connectionState,
		connectionStats,
		isStatsEnabled,
		setIsConnected,
		setIsConnecting,
		setConnectionState,
		// setConnectionStats — a raw setter, same as the three above, for a
		// caller that merges its OWN data into the same stats object rather
		// than only ever replacing it wholesale via handleStatsUpdate (e.g.
		// remote-play's health-check poll merges backend debug fields
		// alongside GatewayWebRTC's own reported bitrate/fps).
		setConnectionStats,
		setStatsMonitoring,
		handleConnectionStateChange,
		handleStatsUpdate,
	};
}
