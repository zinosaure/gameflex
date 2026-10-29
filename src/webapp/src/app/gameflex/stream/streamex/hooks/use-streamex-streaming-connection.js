import { useCallback, useEffect, useRef, useState } from "react";
import { logThrottled } from "../../../shared/utils/log-store.js";
import { useGatewayWebrtcConnection } from "../../shared/hooks/use-gateway-webrtc-connection.js";
import { absoluteApiUrl } from "../../../libs/serverBase.js";
import { useManagedWebSocket } from "../../shared/hooks/use-managed-websocket.js";

const PING_INTERVAL_MS = 1000;

// Connects to streamex (the Go streaming server, src/microservices/
// runtimes/streamex) — WS /ws?viewerId carries this tab's hello, its
// fallback input and the ping-pong, while video/audio/ICE/stats go through
// the shared GatewayWebRTC (stream/shared/GatewayWebRTC.jsx) via screenRef —
// extracted out of streamex/Player.jsx, which used to build all of this
// inline.
//
// NO ROLE, NO TOKEN (P5, 2026-10-09). This socket used to open on the play
// or the watch link, whose token made the child hand out a role and a pad.
// Who drives a pad is the session queue's answer, read on the child socket
// (drivesPad below); the child only routes this tab's input if the queue
// seats the viewer it announced.
//
// Slot requests are handled by the kernel over the façade WS now (P4-P5),
// so the child-level slotrequested/slotgranted/requestslot messages this
// file used to speak are gone — the unified contract (onSwapDevice in
// use-streamex-input.js) drives takeover reconnects instead.
//
// streamex has no WebRTC of its own: it does not start without the gateway
// (docs/WEBRTC_GATEWAY.md), so no "offer" ever arrives on this WebSocket.
//
// Unlike retro-gaming, the session/chain-window protocol below is NOT
// split into a separate "extras" file — it rides the exact same WebSocket
// as connection setup (the "session" message is the child's hello), so
// splitting connection from protocol
// here would mean threading the one WS ref across two files for no real
// gain. Only the reactive connection-state slice (isConnected/isConnecting/
// connectionState/connectionStats) is shared, via useGatewayWebrtcConnection
// below — the same hook retro-gaming's connection file uses.
export function useStreamexStreamingConnection({
	screenRef,
	session,
	t,
	// drivesPad — do this tab's keys reach a pad? The CHILD's answer (its
	// `you.controls`), never this socket's legacy role: during a loan the
	// borrower watches on this socket yet drives the owner's pad, and the
	// owner plays on it yet drives nothing (#274).
	drivesPad = false,
	// enabled — the page's go (onConnect): the socket must carry this tab's
	// gateway viewer, which only exists once the page has created it.
	enabled = false,
	onConnectionError,
	onConnectionStateChange,
}) {
	const { wsRef, openManaged, closeManaged } = useManagedWebSocket();
	const drivesPadRef = useRef(drivesPad);
	drivesPadRef.current = drivesPad;
	// Server-clock offset (ms), refreshed on every server ping — used to
	// timestamp presses for the keytest console (see keytest/display.py).
	const serverOffsetMsRef = useRef(0);

	const [connectionError, setConnectionError] = useState(false);
	const [rttMs, setRttMs] = useState(null);
	// retryNonce — bumped 4s after a connection error to force the connect
	// effect below to re-run. A hard refresh never cleanly disconnects the
	// PREVIOUS tab's connection (no unmount fires, the socket just dies) —
	// the gateway/child can reject a fresh attempt for a few seconds while
	// it notices the old one is gone. Dashboard.jsx's own StreamexPreview
	// hits the exact same race and papers over it with an identical 4s
	// retry loop; this hook (unlike retro/remote-play's) has no external
	// connect() a caller can re-invoke — it auto-connects off socketUrl, so
	// the retry has to live in its own dependency array.
	const [retryNonce, setRetryNonce] = useState(0);

	const {
		isConnected,
		isConnecting,
		connectionState,
		connectionStats,
		setIsConnected,
		setStatsMonitoring,
		handleConnectionStateChange: handleGatewayConnectionStateChange,
		handleStatsUpdate,
	} = useGatewayWebrtcConnection({ screenRef, t });

	// The URL is built against L'ORIGINE DU SERVEUR, jamais l'adresse LAN du
	// conteneur : derrière un proxy HTTPS, celle-ci est injoignable et
	// bloquée pour contenu mixte — le kernel relaie /api/v1/streamex/<id>/ws
	// vers le conteneur de session.
	//
	// `absoluteApiUrl` ET PAS `window.location.origin`, et c'est tout le
	// défaut du 2026-09-07 : dans l'application, l'origine de la page est le
	// kernel du TÉLÉPHONE, qui sert bien l'interface mais ne connaît aucune
	// session streamex. Le profil de manette et le canal de contrôle partaient
	// donc chez lui et revenaient 404 — image parfaite, manette morte, sans
	// une erreur qui parle d'entrées. `serverOrigin()` (dedans) rend
	// exactement la même chose qu'avant dans un navigateur : rien, donc la
	// page. Ailleurs, il rend l'hôte que cette page vise.
	const socketUrl = session.id && enabled ? absoluteApiUrl(`/api/v1/streamex/${session.id}/ws`) : null;

	// Forwards to the shared hook's own connected/isConnecting/
	// connectionState handling, then also reports to the caller
	// (Player.jsx uses onConnectionStateChange for its own overlay text).
	const handleConnectionStateChange = useCallback(
		(state) => {
			handleGatewayConnectionStateChange(state);
			onConnectionStateChange?.(state);
		},
		[handleGatewayConnectionStateChange, onConnectionStateChange],
	);

	// Connect the WS and drive the WebRTC handshake.
	useEffect(() => {
		if (!session.id) {
			setConnectionError(true);
			return undefined;
		}
		if (!socketUrl) {
			// Not yet: the page has not given its go (no viewer so far).
			return undefined;
		}

		let cancelled = false;
		(async () => {
			// The PAGE created the gateway viewer, once, from the sessionId
			// alone, before this socket was allowed to open (Streaming.jsx
			// gates onConnect on viewerReady) — we only read its id for the
			// query (the child maps viewer->pad slot at connect). Creating it
			// here welded the video's lifetime to this socket's: hasInput was
			// role-dependent, so every role change renegotiated the SDP and
			// restarted the picture. See SessionStream's own doc.
			const viewer = screenRef?.current?.viewerId
				? { viewerId: screenRef.current.viewerId }
				: null;
			if (cancelled) return;

			const wsUrl = new URL(socketUrl);
			wsUrl.protocol = wsUrl.protocol === "https:" ? "wss:" : "ws:";
			// The viewer is how the child finds this tab's seat in the queue:
			// a hand-over changes the seat, never this socket.
			if (viewer?.viewerId) wsUrl.searchParams.set("viewerId", viewer.viewerId);

			const sendJSON = (message) => {
				wsRef.current?.send(JSON.stringify(message));
			};

			openManaged(wsUrl.toString(), {
				onOpen: () => {
					// UNE CONNEXION ÉTABLIE EFFACE UNE ERREUR PASSÉE. Sans ça,
					// l'erreur d'avant — un lien pas encore connu au premier
					// rendu — relançait ce socket 4 s plus tard alors qu'il
					// marchait, et le child retirait au passage le viewer de
					// l'image (2026-10-08).
					setConnectionError(false);
					sendJSON({ type: "ready" });
				},
				onError: () => {
					if (!cancelled) {
						setConnectionError(true);
					}
				},
				onClose: () => {
					setIsConnected(false);
					if (!cancelled) {
						setConnectionError(true);
					}
				},
				onMessage: async (message) => {
					switch (message.type) {
						case "session":
							// The child's hello — no role, no pad: who drives
							// what is the queue's answer (drivesPad).
							// Blackscreen.jsx's chain-window toggle starts at
							// 300ms client-side, but onChainWindowChange only
							// fires on a manual toggle — nothing was ever sent
							// on connect, so the keytest display app kept its
							// OWN internal default (50ms) until the user touched
							// the switch at least once. Sync it explicitly here.
							// Harmless for every other console: injectLine only
							// special-cases "cw," for the keytest adapter.
							sendMessage("cw,300");
							break;
						case "ping":
							// Server RTT probe — echoing keeps the server's rtt
							// stat live, and message.t (server UnixMilli) yields
							// the offset used to timestamp keytest presses on
							// the server's own clock.
							serverOffsetMsRef.current = message.t - performance.now();
							sendJSON({ type: "pong", t: message.t });
							break;
						case "pong":
							setRttMs(Math.max(0, Math.round(performance.now() - message.t)));
							break;
						case "error": {
							// Any child-side rejection now — the former "this
							// session already has an active connection" case was
							// removed (2026-08-23: multiple connections are
							// allowed, the latest bind wins).
							setConnectionError(true);
							break;
						}
						default:
							break;
					}
				},
			});
		})();

		return () => {
			cancelled = true;
			closeManaged();
			// No screenRef.cleanup() here — the PAGE owns the gateway viewer now (see
	// SessionStream): tearing it down from here kills the video for the
	// whole page, and the platform WS re-opening does NOT bring it back
	// (SessionStream stops retrying once attached). Confirmed live
	// 2026-08-24 in the gateway log — "viewer added" then "viewer removed"
	// ~310ms later, then no picture at all. The viewer is released by
	// GatewayWebRTC's own unmount effect, nowhere else.
			// This effect re-runs on socketUrl/retry, so it was the
			// worst offender: a dependency simply settling killed the video.
		};
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [socketUrl, retryNonce]);

	useEffect(() => {
		if (!connectionError) return;
		onConnectionError?.();
		const retry = setTimeout(() => {
			setConnectionError(false);
			setRetryNonce((n) => n + 1);
		}, 4000);
		return () => clearTimeout(retry);
	}, [connectionError, onConnectionError]);

	// Latency probe — streamex answers pong with the client's own t.
	useEffect(() => {
		const timer = setInterval(() => {
			const socket = wsRef.current;
			if (socket && socket.readyState === WebSocket.OPEN) {
				socket.send(JSON.stringify({ type: "ping", t: performance.now() }));
			}
		}, PING_INTERVAL_MS);
		return () => clearInterval(timer);
	}, []);

	// Sends one input token — the reference client's own wire format
	// (`kd,<code>`/`ku,<code>`/`md,<button>`/`mr,<dx>,<dy>`/`ms,<delta>`,
	// keytest's `cw,<ms>`), player-gated server-side too but checked here
	// first so a spectator's pad never even tries. Phase 2: DataChannel via
	// GatewayWebRTC when open, WS otherwise (mirrors sendButton/
	// sendAnalog's own fallback in use-retrogaming-streaming-connection.js).
	//
	// `ordered` — un TEXTE TAPÉ (#420) passe d'abord par le WebSocket : le
	// canal de données est sans ordre ni renvoi (gatewaystreams/bridge.go,
	// CreateDataChannel), juste pour une manette, dont l'état suivant efface
	// le précédent. Pour un mot, une lettre perdue ou un Maj relâché avant sa
	// lettre, et le nom tapé n'est plus le bon.
	const sendMessage = useCallback(
		(payload, { ordered = false } = {}) => {
			// A KEY only leaves a tab that drives a pad; a SETTING (the
			// keytest chain window, "cw,") always goes, the child decides.
			if (!drivesPadRef.current && !payload.startsWith("cw,")) return;
			// Keytest console: stamp presses/releases/pressure with the
			// server clock so the display computes latency on ONE clock.
			if (
				session.consoleId === "keytest" &&
				(payload.startsWith("kd,") || payload.startsWith("ku,") || payload.startsWith("ab,"))
			) {
				payload += `,${Math.round(performance.now() + serverOffsetMsRef.current)}`;
			}
			// DEUX CHEMINS, ET LE SECOND EST UN REPLI — pas une alternative
			// qu'on abandonne au premier échec.
			//
			// Le canal de données du gateway est préféré (moins de sauts),
			// mais il n'existe que si la négociation WebRTC a abouti ET que
			// ce viewer a demandé un canal d'entrée. `screenRef.current` est
			// TOUJOURS là depuis que la page possède son viewer : ce test
			// prenait donc systématiquement la première branche et rendait la
			// main, même canal fermé — le WebSocket, lui, était ouvert et
			// prêt. Toutes les touches disparaissaient sans un mot
			// (2026-09-07 ; « avant la refonte ça fonctionnait » : avant, il
			// n'y avait pas de screenRef, donc pas de première branche).
			const socket = wsRef.current;
			const socketOpen = socket?.readyState === WebSocket.OPEN;
			if (!(ordered && socketOpen) && screenRef?.current?.sendMessage(payload)) {
				return;
			}

			if (!socketOpen) {
				logThrottled("warn", "streamex-input-lost", 2000,
					"streamex: entrée perdue — ni canal de données ni WebSocket",
					payload);

				return;
			}

			socket.send(payload);
		},
		[session.consoleId, screenRef],
	);

	return {
		isConnected,
		isConnecting,
		connectionState,
		connectionError,
		rttMs,
		connectionStats,
		setStatsMonitoring,
		handleConnectionStateChange,
		handleStatsUpdate,
		sendMessage,
	};
}
