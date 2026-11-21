import { useCallback, useEffect, useRef } from "react";
import { getStoredToken } from "../../../profile/auth/AuthContext.jsx";
import { useGatewayWebrtcConnection } from "../../shared/hooks/use-gateway-webrtc-connection.js";
import { useManagedWebSocket } from "../../shared/hooks/use-managed-websocket.js";
import { useRetrogamingSaveStates } from "./use-retrogaming-save-states.js";
import { streamAuthToken, wsUrl } from "../../../libs/serverBase.js";

// Connects to the Go backend's server-side libretro emulation, streamed
// over WebRTC — signaling here is a single WebSocket carrying
// {type: start|spectate|join|error|...} messages (see
// golang/src/internal/retrogaming/stream_session.go), NOT the REST-based
// offer/answer/ICE endpoints use-streaming-connection.js (remote-play)
// polls — the two protocols are unrelated, hence a separate hook rather
// than trying to force-fit the same one. Input goes out over a WebRTC
// DataChannel as a batch of {button, value} pairs — RETRO_BUTTON_INDEX
// (gamepadOverlay.js) already matches the libretro RETRO_DEVICE_ID_JOYPAD_*
// indices the backend's inputStateCallback expects directly, no separate
// mapping table needed.
//
// Gateway-only since P7: media/ICE/audio live entirely in the mounted
// <GatewayWebRTC> the caller owns through screenRef — the inline
// RTCPeerConnection "legacy" path this file used to carry for the smaller
// <video>-based callers died with them (les vignettes du tableau de bord
// rendent GatewayWebRTC elles aussi, voir Dashboard.jsx). The child has no
// in-process WebRTC any more either (#257): it acks with "started", never an
// "offer".
//
// Slot requests are handled by the kernel over the façade WS now (P3-P5),
// so the child-level requestslot/slotgranted/cedeslot messages this file
// used to speak are gone — the unified contract (onSwapDevice/
// onSlotGiveaway in use-retrogaming-input.js) drives reconnect instead.
//
// onRumble — called on every "rumble" message from the backend (see
// stream_session.go's rumbleStateCallback), with the core's current
// weak/strong motor strengths (0-65535 each) for THIS connection's own
// controller port. Not handled generically inside this hook (like haptics
// dispatch) because only the CALLER (use-retrogaming-input.js) knows which
// physical gamepad index, if any, this
// connection is actually driving — this hook only knows its own
// WebSocket/port, nothing about input devices.
// `onProgress` — les jalons que le CONTENEUR annonce pendant qu'il charge
// (#191) : cœur résolu, cœur chargé, ROM prête, BIOS retenu, jeu chargé,
// géométrie et cadence. Ils traversent le relais sans qu'il les connaisse, et
// ce hook ne fait que les faire suivre — il ne décide rien avec.
export function useRetrogamingStreamingConnection({ screenRef, toast, onRumble, onProgress, t }) {
	const intentionalDisconnectRef = useRef(false);
	// LA REPRISE, EN SILENCE.
	//
	// Ce socket ne porte NI l'image NI la manette : la vidéo vient du gateway,
	// les boutons du DataChannel, le battement de cœur de la façade. Il ne
	// porte que les sauvegardes, les triches, les disques et la pause. Quand il
	// tombe en pleine partie — vu en vrai le 2026-09-08, `close 1006` sans
	// raison, la partie continuait parfaitement — le joueur n'a donc RIEN à
	// voir : il a juste, sans le savoir, perdu son menu.
	//
	// Avant, rien ne le rétablissait : le portillon de Streaming.jsx est un
	// verrou à sens unique qui ne se relâche que si la FAÇADE tombe. Le socket
	// restait mort jusqu'au prochain rafraîchissement de page — ce qui explique
	// pourquoi un F5 « réparait » le problème.
	//
	// On retente donc, tout seul, avec les mêmes arguments. Sans reprise de
	// contrôle forcée : quand ce socket meurt, le relais ferme aussitôt sa
	// jambe vers l'enfant, qui sait donc déjà que le propriétaire est parti
	// (`read error` dans son journal, à la seconde près) — la place est libre,
	// il n'y a rien à reprendre à personne.
	const reconnectRef = useRef({ args: null, timer: null, attempts: 0 });
	const everConnectedRef = useRef(false);
	// connect ne peut pas se citer lui-même dans son propre useCallback.
	const connectRef = useRef(null);
	const cancelReconnect = useCallback(() => {
		clearTimeout(reconnectRef.current.timer);
		reconnectRef.current.timer = null;
		reconnectRef.current.attempts = 0;
	}, []);

	const { wsRef, sendJSONRef, openManaged, closeManaged } = useManagedWebSocket();

	const {
		isConnected,
		isConnecting,
		connectionState,
		connectionStats,
		setIsConnected,
		setIsConnecting,
		setConnectionState,
		setStatsMonitoring,
		handleConnectionStateChange,
		handleStatsUpdate,
	} = useGatewayWebrtcConnection({ screenRef, t, intentionalDisconnectRef });

	// sendSignaling rides the managed socket's identity-guarded sender —
	// save-states extras and connect()/disconnect() all send through it.
	const sendSignaling = useCallback((msg) => {
		sendJSONRef.current?.(msg);
	}, [sendJSONRef]);

	const {
		cheatsSupported,
		rewindSupported,
		fastForwardSupported,
		initialCheats,
		applyStartedCapabilities,
		applyRewindSupport,
		handleMessage: handleSaveStatesMessage,
		saveState,
		loadState,
		setCheats,
		setFastForward,
		setRewind,
		setControllerType,
		queryDisks,
		changeDisk,
		setPaused,
		resetConsole,
	} = useRetrogamingSaveStates({ sendSignaling });

	// Drops THIS platform connection only — never the gateway viewer:
	// the PAGE owns the gateway viewer now (see
	// SessionStream): tearing it down from here kills the video for the
	// whole page, and the platform WS re-opening does NOT bring it back
	// (SessionStream stops retrying once attached). Confirmed live
	// 2026-08-24 in the gateway log — "viewer added" then "viewer removed"
	// ~310ms later, then no picture at all. The viewer is released by
	// GatewayWebRTC's own unmount effect, nowhere else.
	const cleanup = useCallback(() => {
		closeManaged();
		setIsConnected(false);
		setIsConnecting(false);
	}, [closeManaged, setIsConnected, setIsConnecting]);

	// saveProgress — defaults to false: saving is now the owner's deliberate
	// opt-in from the exit-confirm modal (Streaming.jsx's handleExit), never
	// the automatic path (2026-08-20). Sent as an explicit "exit" message
	// BEFORE closing the socket, since the backend only infers "the owner
	// left" from the connection closing and has no other way to learn the
	// user's choice (see stream_session.go's skipAutoSave doc).
	const disconnect = useCallback(
		(saveProgress = false) => {
			intentionalDisconnectRef.current = true;
			cancelReconnect();
			sendSignaling({ type: "exit", saveProgress });
			cleanup();
		},
		[cancelReconnect, cleanup, sendSignaling],
	);

	// leavePage — the page is navigating away, NOT quitting (roadmap P2):
	// mark this tab hidden (the emulator pauses once nobody is watching —
	// recomputePauseState server-side), then drop the transport WITHOUT the
	// "exit" message, so the session survives on the grace path for the
	// next device. Sending "exit" from here would mark the session
	// explicitExit (stream_session.go's "exit" case), skipping the 5-minute
	// grace and killing the child the instant this tab's WS closes —
	// exactly the behavior the reattach/"MySession" rejoin model exists to
	// avoid.
	const leavePage = useCallback(() => {
		sendSignaling({ type: "visibility", visible: false });
		intentionalDisconnectRef.current = true;
		cancelReconnect();
		cleanup();
	}, [cancelReconnect, cleanup, sendSignaling]);

	// A session is always created FIRST via a plain REST call
	// (createSession, gameflex/api/retrogaming.js — see CreateSession's doc,
	// golang/internal/retrogaming/handlers.go) — this hook only ever
	// ATTACHES to an already-existing sessionId, never starts one from
	// scratch itself. That split is what makes a copy-pasted or refreshed
	// streaming link safe: reopening the same URL re-attaches to the SAME
	// session instead of spawning a duplicate.
	//
	// role — "owner" (sends "start", server relays the STORED consoleId/
	// reference/core/username to the child — see stream_session.go's
	// HandleStreamWS, nothing about the game is sent from here anymore),
	// "join" (2nd/3rd player, own independent controller port — see
	// serveJoinerSession), or "spectate" (watch-only, no controller port,
	// unlimited — see serveSpectatorSession). All three go through the same
	// gateway-first flow; only the very first message's type differs.
	//
	// username — the logged-in user's own username (see AuthContext.jsx) —
	// not cryptographically verified by Go (a deliberate simplicity
	// tradeoff for a small trusted user base, see GAMEFLEX_JOURNAL.md).
	// Sent on "start"/"join" so the backend can enforce one active player-
	// session per user across sessions/platforms (see
	// internal/sessions.IsAlreadyPlaying) — unused for "spectate", which has
	// no per-user slot to enforce.

	const connect = useCallback(
		(sessionId, role, username, onConnectionError, { takeover = false } = {}) => {
			// Mémorisés pour la reprise silencieuse (voir reconnectRef) : elle
			// doit refaire EXACTEMENT la même demande, sinon un joueur
			// reviendrait spectateur, ou un spectateur réclamerait une place.
			reconnectRef.current.args = [sessionId, role, username, onConnectionError, { takeover }];
			clearTimeout(reconnectRef.current.timer);
			reconnectRef.current.timer = null;

			if (wsRef.current) {
				// A previous attempt's socket may still be closing (the child
				// closed it right after a rejection error) — replace it instead
				// of silently returning: the take-over retry depends on it.
				// The old socket's own close is identity-guarded in
				// openManaged so it can't clobber the new connection's state.
				intentionalDisconnectRef.current = true;
				closeManaged();
			}
			intentionalDisconnectRef.current = false;
			setIsConnecting(true);
			setConnectionState(t("streaming.connection.state.connecting"));

			// Token in the query string, not a header: a browser cannot set one on
			// a WebSocket handshake. The relay checks it before upgrading (see
			// handleRetroStreamWS), same contract as /api/v1/facade/ws.
			//
			// LE JETON DE L'HÔTE VISÉ, pas le nôtre (#42). `wsUrl` suit déjà la
			// surcharge d'hôte : ce socket part CHEZ LUI. Y présenter notre
			// jeton, qu'il ne sait pas lire, faisait de nous un anonyme — le
			// relais refusait le `start` (« not authorized to own this
			// session ») et l'émulateur, qui n'attend que lui, ne démarrait
			// jamais. Même règle que input.service.js, qui l'avait déjà.
			openManaged(
				wsUrl("/api/v1/retrogaming/stream/ws") +
					`?token=${encodeURIComponent(streamAuthToken() || getStoredToken() || "")}`,
				{
					onOpen: async (sendJSON) => {
						// The PAGE created the gateway viewer, once, from the
						// sessionId alone, before this socket was allowed to
						// open (Streaming.jsx gates onConnect on viewerReady) —
						// we only read its id to put on the wire. This hook
						// used to call createViewer here, which welded the
						// video's lifetime to this socket's: every role change
						// renegotiated the SDP (hasInput was role-dependent)
						// and restarted the picture, and a missed "started"
						// left the video dead for good while input still
						// flowed. See SessionStream's own doc.
						const viewerId = screenRef.current?.viewerId ?? undefined;
						if (role === "spectate") {
							sendJSON({ type: "spectate", sessionId, viewerId });
							return;
						}
						if (role === "join") {
							sendJSON({ type: "join", sessionId, username, viewerId });
							return;
						}
						sendJSON({ type: "start", sessionId, username, viewerId, takeover });
					},
					onError: () => {
						if (intentionalDisconnectRef.current) {
							return;
						}
						// Une panne AVANT la première connexion est une vraie
						// erreur : il n'y a rien à l'écran, il faut le dire.
						// APRÈS, la partie tourne toujours — on se tait et on
						// reprend (voir reconnectRef).
						if (!everConnectedRef.current) {
							onConnectionError?.(t("streaming.connection.errors.deviceNotReady"));
						}
					},
					onClose: () => {
						if (intentionalDisconnectRef.current) {
							return;
						}
						// PAS setIsConnected(false) — ET C'EST TOUT LE SUJET.
						//
						// `isConnected` décrit le FLUX (l'état de la
						// PeerConnection, voir useGatewayWebrtcConnection), pas
						// ce socket-ci. Les éteindre ensemble faisait dire à la
						// page « En attente de la vidéo… » par-dessus une image
						// qui tournait parfaitement, puisque la vidéo vient du
						// gateway et ne dépend pas d'ici — signalé en jouant,
						// PS2 comme PSP (2026-09-08). Le flux a son propre
						// verdict, il le rend tout seul.
						setIsConnecting(false);
						if (!everConnectedRef.current || !reconnectRef.current.args) {
							return;
						}
						// Palier court puis plafonné : la place est déjà libre
						// côté enfant, la première tentative suffit presque
						// toujours. Le plafond existe pour le cas où le
						// conteneur, lui, est vraiment parti — c'est la façade
						// qui tranche ça, pas ce socket.
						const attempt = reconnectRef.current.attempts + 1;
						reconnectRef.current.attempts = attempt;
						const delay = Math.min(1000 * attempt, 5000);
						clearTimeout(reconnectRef.current.timer);
						reconnectRef.current.timer = setTimeout(() => {
							const args = reconnectRef.current.args;
							if (args) {
								connectRef.current?.(...args);
							}
						}, delay);
					},
					onMessage: async (msg) => {
						if (handleSaveStatesMessage(msg)) return;

						switch (msg.type) {
							case "progress": {
								// UN JALON N'EST JAMAIS UN ÉTAT : il ne touche
								// ni la connexion, ni la reprise, ni la phase.
								// Une page qui l'ignore se comporte exactement
								// comme avant — c'est ce qui rend ce canal sûr
								// à ajouter sur un chemin de production.
								onProgress?.(msg.step, msg.detail || null);
								break;
							}
							case "started": {
								// LA connexion a abouti — c'est à partir
								// d'ici qu'une coupure devient une reprise
								// silencieuse plutôt qu'une erreur (voir
								// reconnectRef).
								everConnectedRef.current = true;
								reconnectRef.current.attempts = 0;
								// The child's ack: session/cheats payload —
								// media is already negotiated with gateway-webrtc.
								// No attach() here anymore: the page attached
								// the video as soon as the gateway had a stream,
								// with its own retry. Attaching on THIS message
								// meant one missed "started" = no picture ever,
								// while the input channel was perfectly up.
								applyStartedCapabilities(msg);
								break;
							}
							case "rewindsupport": {
								// Voir applyRewindSupport : la capacité se
								// mesure, elle ne s'annonce pas au démarrage.
								applyRewindSupport(msg);
								break;
							}
							case "rumble": {
								onRumble?.(msg.rumbleWeak || 0, msg.rumbleStrong || 0);
								break;
							}
							// Broadcast when ANY player (owner or a joiner,
							// port 0..N-1) connects/disconnects — see
							// stream_session.go's broadcast/
							// registerBroadcastTarget and child.go's
							// serveJoinerSession. Best-effort notice only:
							// it's up to whoever's still playing to decide
							// whether to keep going or wait — nothing here
							// changes the actual game/connection state, this
							// is purely informational (see the identical
							// reasoning on the controller-disconnect signal
							// server-side, which is itself best-effort and
							// may not visibly affect every core).
							// playerleft / playerjoined — the child still
							// broadcasts them, but the PAGE no longer builds
							// a notice from them: who joined and who left is
							// announced once, for all three platforms, from
							// the kernel's own joined/left façade events (see
							// handleKernelEvent in Streaming.jsx). Doing it
							// here made retro the only platform that said
							// anything, and made it say it twice whenever the
							// child tore down more than one port.
							case "playerleft":
							case "playerjoined":
								break;
							case "takeover": {
								// This device was the owner until another
								// device took the session over — the child is
								// about to close this connection. The kernel
								// push already flipped the local role (P3),
								// so there is nothing to hand over here —
								// only the state guard matters.
								intentionalDisconnectRef.current = true;
								break;
							}
							case "error": {
								// A savestate/loadstate/setcheats error is
								// consumed by handleSaveStatesMessage above
								// (its pending-callback path) — reaching here
								// means the error belongs to the connection
								// itself.
								setIsConnecting(false);
								onConnectionError?.(msg.error);
								break;
							}
							default:
								break;
						}
					},
				},
			);
		},
		[
			closeManaged,
			screenRef,
			t,
			toast,
			onRumble,
			setIsConnecting,
			setConnectionState,
			setIsConnected,
			handleSaveStatesMessage,
			applyStartedCapabilities,
			applyRewindSupport,
		],
	);
	// Toujours la dernière version, pour que la reprise silencieuse
	// (reconnectRef) rappelle le connect courant et non celui d'un rendu périmé.
	connectRef.current = connect;

	// Un démontage ne doit pas laisser une reprise programmée derrière lui.
	useEffect(() => cancelReconnect, [cancelReconnect]);

	// Sends one button transition (down=1/up=0) immediately over the
	// DataChannel — no polling interval needed the way the reference
	// client's keyboard/physical-gamepad handling does, since the virtual
	// controller already only fires on real press/release events.
	const sendButton = useCallback((buttonId, value) => {
		if (buttonId === undefined) return;
		screenRef.current?.sendMessage(JSON.stringify([{ button: buttonId, value }]));
	}, [screenRef]);

	// Sends an analog stick position — axis 0/1 matches libretro's
	// RETRO_DEVICE_INDEX_ANALOG_LEFT/RIGHT (see stream_session.go's
	// handleInput). x/y are normalized -1..1 from VirtualJoystick, scaled
	// here to the int16 range the backend's analogStates array expects
	// (RETRO_DEVICE_ID_ANALOG_X/Y), matching real libretro analog range.
	const sendAnalog = useCallback((axis, x, y) => {
		screenRef.current?.sendMessage(
			JSON.stringify([
				{ axis, x: Math.round(x * 32767), y: Math.round(y * 32767) },
			]),
		);
	}, [screenRef]);

	// L'ÉCRAN TACTILE — la Nintendo DS et les siens.
	//
	// x/y arrivent en -1..1 sur TOUTE l'image (le haut et le bas des deux
	// écrans compris), mis à l'échelle ici comme les axes juste au-dessus.
	// On n'essaie PAS de deviner où se trouve l'écran du bas : le core le
	// sait, selon la disposition qu'il applique, et un stylet posé hors de la
	// zone tactile doit rester hors d'elle.
	const sendPointer = useCallback((x, y, pressed) => {
		screenRef.current?.sendMessage(
			JSON.stringify([
				{
					pointer: pressed ? 1 : 0,
					x: Math.round(x * 32767),
					y: Math.round(y * 32767),
				},
			]),
		);
	}, [screenRef]);

	return {
		isConnected,
		isConnecting,
		connectionState,
		handleConnectionStateChange,
		handleStatsUpdate,
		connect,
		disconnect,
		leavePage,
		sendButton,
		sendAnalog,
		sendPointer,
		saveState,
		loadState,
		setFastForward,
		setRewind,
		cheatsSupported,
		rewindSupported,
		fastForwardSupported,
		initialCheats,
		setCheats,
		setControllerType,
		queryDisks,
		changeDisk,
		setPaused,
		resetConsole,
		connectionStats,
		setStatsMonitoring,
	};
}
