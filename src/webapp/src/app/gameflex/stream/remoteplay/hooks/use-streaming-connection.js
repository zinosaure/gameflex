import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useGamepadInput, useGamepad } from "@/gameflex/hooks/use-gamepad";
import { streamingService } from "@/gameflex/stream/remoteplay/service/streaming.service";
import { viewerService } from "@/gameflex/stream/remoteplay/service/viewer.service";
import {
	remoteplayInputService as controllerService,
	remoteplayInputService as streamingHubService,
} from "@/gameflex/stream/remoteplay/service/input.service";
import { applyControllerRumbleToGamepads } from "@/gameflex/stream/remoteplay/service/rumble.service";
import { optimizeVideoForLowLatency } from "@/gameflex/stream/shared/webrtc-low-latency-sdp";
import { createKeyboardHandler } from "@/gameflex/stream/remoteplay/libs/keyboard-mapping";
import {
	GamepadButton,
	PS5_BUTTON_MAP,
} from "@/gameflex/stream/remoteplay/service/gamepad.service";
import { logThrottled, debugTrace } from "@/gameflex/shared/utils/log-store";
import {
	AXIS_DEADZONE,
	MAX_HEARTBEAT_INTERVAL_MS,
	SEND_INTERVAL_MS,
	TRIGGER_DEADZONE,
} from "./use-streaming-connection/constants";
import { markActivity } from "../../shared/activity-store.js";
// v1 (chantier-3): the client-side input priority gate is gone. It existed
// because every tab of a player kept its role, so the page had to decide
// locally which one was driving. Input is now gated where it belongs —
// server-side, on who holds the pad — and a pad is taken by an explicit
// POST, never inferred here.
import { useStickInputState } from "./use-streaming-connection/stick-input-state";
import { virtualJoystickActiveState } from "./use-streaming-connection/virtual-joystick-state";
import { useMouseRightStick } from "./use-streaming-connection/use-mouse-right-stick";
import { useGatewayWebrtcConnection } from "../../shared/hooks/use-gateway-webrtc-connection.js";
import { useConnectionStatsCollector } from "./use-streaming-connection/stats-collector";
import { usePrepareDevice } from "./use-streaming-connection/prepare-device";

export function useStreamingConnection({
	hostId,
	isLikelyLan,
	// videoRef — a read-only proxy onto GatewayWebRTC's own <video>
	// element (Player.jsx passes a `{get current(){return
	// screenRef.current?.videoElement}}` object, same pattern
	// use-retro-streaming-connection.js's legacy path uses) — every place
	// below that reads videoRef.current for a real DOM API (stall
	// detection, mouse-stick pointer lock, PiP) keeps working unchanged;
	// nothing here ever reassigns videoRef.current itself.
	videoRef,
	// screenRef — ref to the mounted <GatewayWebRTC>, which now owns
	// the RTCPeerConnection/ICE/offer-answer/getStats mechanics this hook
	// used to duplicate (see docs/wiggly-rolling-plum's "Étape 3" note).
	screenRef,
	toast,
	onConnectionError,
	attachSessionId,
	// username — le nom du compte connecté (voir AuthContext.jsx), envoyé dans
	// la commande `launch` du centre de messages, avec `platform:
	// "remoteplay"`. Le
	// kernel inscrit la session au pool avec un vrai Initiator et fait
	// respecter « une seule partie jouée à la fois », toutes plateformes
	// confondues (`launchRemoteplay`, kernel/internal/api/remoteplay_api.go).
	//
	// Il n'est qu'un REPLI : le kernel prend d'abord le nom dans le JWT et ne
	// lit celui-ci que s'il n'en a pas (launch_remoteplay.go). C'est la même
	// règle « le jeton fait foi » que côté rétro.
	username,
	// preCreatedSessionId — set when the unified Streaming page (gameflex/
	// Streaming.jsx) already created this session via startSession(hostId)
	// itself (before navigating here, so a copy-pasted/refreshed streaming
	// link re-attaches instead of re-triggering a fresh PS5 handshake — la
	// reprise est portée par `launchRemoteplay`, qui rend `existing: true`
	// quand une session tourne déjà pour cet appareil et fait alors SAUTER la
	// poignée de main, pour ne pas relancer Takion sous une partie en cours
	// (kernel/internal/api/remoteplay_api.go).
	// When set, connect() below skips its OWN startSession call and reuses
	// this id directly, then proceeds through the exact same offer/answer/
	// ICE flow as always. Distinct from attachSessionId (control-handoff,
	// attaches as a VIEWER via viewerService) — this is a fresh OWNER
	// connection to a session that merely happens to already be started.
	preCreatedSessionId,
	// gaveAwayControl — B2: the page flips this true after accepting a
	// kernel control request; the hook then stops sending input (the pad
	// now belongs to the requester). The kernel is the authority — see
	// Player.jsx's session.controlRequest prompt.
	gaveAwayControl = false,
}) {
	const { t } = useTranslation();
	// isConnected/isConnecting/connectionState/connectionStats/isStatsEnabled
	// — sourced from the same shared hook retro/streamex already use (see
	// its own doc), instead of reimplementing this slice locally as before
	// (docs/wiggly-rolling-plum, "Conventions de code + refacto phasé des
	// useXStreamingConnection"). handleConnectionStateChange/setStatsMonitoring
	// below wrap the shared hook's own versions to layer PS5-only side
	// effects on top — see each wrapper's own comment.
	const {
		isConnected,
		isConnecting,
		connectionState,
		connectionStats,
		isStatsEnabled,
		setIsConnected,
		setIsConnecting,
		setConnectionState,
		setConnectionStats,
		setStatsMonitoring: setSharedStatsMonitoring,
		handleConnectionStateChange: handleSharedConnectionStateChange,
	} = useGatewayWebrtcConnection({ screenRef, t });
	const [webrtcSessionId, setWebrtcSessionId] = useState(null);
	const [remotePlaySessionId, setRemotePlaySessionId] = useState(null);
	const [isStalling, setIsStalling] = useState(false);
	// Control handoff ("take control") — the owner starts in control; can be
	// handed off to a viewer via the kernel (B2 — see Player.jsx's
	// session.controlRequest prompt and gaveAwayControl above).
	// hasControlRef is what the actual input-sending code paths check
	// (avoids stale closures); hasControl (state) is only for rendering the
	// UI.
	const [hasControl, setHasControl] = useState(true);
	const hasControlRef = useRef(true);

	// Set only when this connection ATTACHES to an already-running session
	// (control-handoff "take control" flow, see connectAttached below) rather
	// than starting a brand-new one — governs which teardown path disconnect()
	// takes.
	const isAttachedRef = useRef(false);
	const viewerIdRef = useRef(null);
	const videoOptimizeCleanupRef = useRef(null);
	// Takes the <video> playback listeners back down — see where it is set, in
	// processVideoStream. Same shape as videoOptimizeCleanupRef above.
	const detachVideoListenersRef = useRef(null);
	const keyboardCleanupRef = useRef(null);
	const gamepadEnabledRef = useRef(false);
	const isConnectedRef = useRef(false);
	const hasAttemptedInitialConnectRef = useRef(false);
	const hasTriggeredErrorCallbackRef = useRef(false);

	// Push-based ICE candidates / health (Phase 3 WS migration) — unsubscribe
	// functions for the streamingHubService listeners registered once we
	// know our own webrtcSessionId, torn down in disconnect() below so a
	// reconnect doesn't stack duplicate listeners on the shared singleton.
	const iceCandidateUnsubscribeRef = useRef(null);
	const healthUpdateUnsubscribeRef = useRef(null);
	// Latest HealthUpdate pushed over the streaming hub — read by
	// handleStreamHealthCheck instead of polling GET .../health over HTTP.
	const pushedHealthRef = useRef(null);
	// Cumulative video packetsLost/packetsReceived from the last network-loss
	// sample, used to compute a windowed loss ratio to report to the backend
	// (see the reportNetworkLoss effect below) — separate from the stats
	// collector's rate anchors (now owned by useConnectionStatsCollector),
	// which only run while the Live Monitoring panel is open; this sampling
	// must run regardless, since bitrate adaptation is not a debug-only
	// feature.
	const lastNetworkLossSampleRef = useRef(null);

	const {
		getNormalizedState,
		snapshotGamepadAxes,
		handleGamepadAxis,
		setPointerLock,
		setMouseVelocity,
		setKeyboardLeftStick,
		setTriggerPressure,
		reset: resetStickInput,
	} = useStickInputState();

	const { setup: setupMouseRightStick, tearDown: tearDownMouseRightStick } =
		useMouseRightStick({
			videoRef,
			onPointerLockChange: setPointerLock,
			onMouseMove: setMouseVelocity,
		});

	const lastSentRef = useRef({
		leftX: 0,
		leftY: 0,
		rightX: 0,
		rightY: 0,
		l2: 0,
		r2: 0,
		timestamp: 0,
	});

	const stickProcessingActiveRef = useRef(false);
	const stickIntervalRef = useRef(null);
	const { isEnabled: isGamepadEnabled } = useGamepad();
	// Stats collection and the PS5 device gate live in their own files now
	// (P7.4) — see use-streaming-connection/stats-collector.js and
	// prepare-device.js. streamingStartTimeRef is stamped by
	// handleConnectionStateChange/onFirstVideoTrack below.
	const { streamingStartTimeRef, resetConnectionStats } =
		useConnectionStatsCollector({ screenRef, setConnectionStats, isStatsEnabled });
	const prepareDevice = usePrepareDevice({ hostId, t, setConnectionState });
	const webrtcSessionIdRef = useRef(null);
	const isStreamBoundRef = useRef(false);
	// Le minuteur de la première connexion, tenu hors du cycle de rendu — voir
	// l'effet d'auto-connexion tout en bas pour la raison.
	const initialConnectTimerRef = useRef(null);
	const hasVideoTrackRef = useRef(false);
	const keyframeMonitorIntervalRef = useRef(null);
	const lastVideoActivityRef = useRef(0);
	const lastDecodedFrameCountRef = useRef(null);
	const lastPlaybackPositionRef = useRef(null);
	const lastKeyframeRequestRef = useRef(0);
	const pendingKeyframeRequestRef = useRef(false);
	const initialKeyframeRequestedRef = useRef(false);
	const remotePlaySessionIdRef = useRef(null);
	const lastStreamHealthRef = useRef(null);
	const healthCheckInFlightRef = useRef(false);
	const lastHealthCheckAtRef = useRef(0);
	const lastNeutralHealthKeyframeRef = useRef(0);

	const KEYFRAME_REQUEST_COOLDOWN_MS = 8000;
	const HEALTH_CHECK_COOLDOWN_MS = 3000;
	// Previously used for the neutral keyframe auto-trigger cooldown (no longer in use)
	// const HEALTH_NEUTRAL_KEYFRAME_COOLDOWN_MS = 5000

	const requestKeyframe = useCallback(
		(reason) => {
			const now = Date.now();
			const sessionId = webrtcSessionIdRef.current || webrtcSessionId;
			if (!sessionId) {
				debugTrace("⚠️ Cannot request keyframe, missing SessionId", { reason });
				return false;
			}

			if (!isStreamBoundRef.current) {
				debugTrace("⚠️ Cannot request keyframe, session not bound", { reason });
				return false;
			}

			if (pendingKeyframeRequestRef.current) {
				debugTrace("⚠️ Keyframe request in progress, skipping", { reason });
				return false;
			}

			if (now - lastKeyframeRequestRef.current < KEYFRAME_REQUEST_COOLDOWN_MS) {
				debugTrace("⚠️ Keyframe request cooldown", {
					reason,
					elapsed: now - lastKeyframeRequestRef.current,
				});
				return false;
			}

			lastKeyframeRequestRef.current = now;
			pendingKeyframeRequestRef.current = true;

			console.warn("⚠️ Triggering keyframe request", {
				reason,
				sessionId,
			});

			const sendKeyframeRequest = async () => {
				try {
					const signalrResult =
						await streamingHubService.requestKeyframe(sessionId);
					if (!signalrResult) {
						console.warn("⚠️ SignalR keyframe failed, trying HTTP fallback", {
							reason,
						});
						const response = await streamingService.requestKeyframe(sessionId);
						if (!response.success) {
							console.warn("⚠️ HTTP keyframe request failed", {
								reason,
								message: response.message,
								error: response.errorMessage,
							});
						} else {
							debugTrace("🎯 HTTP keyframe request sent", { reason });
						}
					} else {
						debugTrace("🎯 SignalR keyframe request sent", { reason });
					}
				} catch (error) {
					console.error(
						"❌ Keyframe request failed, trying HTTP fallback",
						error,
						{
							reason,
						},
					);
					try {
						const response = await streamingService.requestKeyframe(sessionId);
						if (!response.success) {
							console.warn("⚠️ HTTP keyframe request failed", {
								reason,
								message: response.message,
								error: response.errorMessage,
							});
						} else {
							debugTrace("🎯 HTTP keyframe request sent", { reason });
						}
					} catch (httpError) {
						console.error("❌ HTTP keyframe request error:", httpError, {
							reason,
						});
					}
				} finally {
					pendingKeyframeRequestRef.current = false;
				}
			};

			void sendKeyframeRequest();

			return true;
		},
		[KEYFRAME_REQUEST_COOLDOWN_MS, webrtcSessionId],
	);

	// Externally exposed manual refresh method (requests a keyframe)
	const refreshStream = useCallback(() => {
		const ok = requestKeyframe("manual-refresh");
		// When connected, minimize toast interruptions; only show a brief notice on failure
		if (!ok) {
			try {
				// Only show a brief notice on failure, with a short display duration
				toast({
					title: "Refresh unavailable",
					description: "Session unavailable or still in cooldown",
					variant: "destructive",
					duration: 2000, // Auto-dismiss after 2 seconds
				});
			} catch {
				// ignore toast failure in environments without i18n/toast
			}
		} else {
			// On success, if already connected do not show a toast (avoid interrupting gameplay), only log to console
			if (isConnectedRef.current) {
				debugTrace("✅ Refresh request sent");
			} else {
				// Show a toast when not connected
				try {
					toast({
						title: "Refresh request sent",
						description: "Please wait, attempting to restore video",
						duration: 2000, // Auto-dismiss after 2 seconds
					});
				} catch {
					// ignore
				}
			}
		}
		return ok;
	}, [requestKeyframe, t, toast]);

	// Force reset ReorderQueue (user-triggered, to resolve a frozen frame)
	const forceResetReorderQueue = useCallback(async () => {
		const sessionId = webrtcSessionIdRef.current || webrtcSessionId;
		if (!sessionId) {
			console.warn("⚠️ Cannot reset queue, missing SessionId");
			try {
				toast({
					title: "Cannot Reset Queue",
					description: "Session unavailable",
					variant: "destructive",
					duration: 2000,
				});
			} catch {
				// ignore toast failure
			}
			return false;
		}

		if (!isStreamBoundRef.current) {
			console.warn("⚠️ Cannot reset queue, session not bound");
			try {
				toast({
					title: "Cannot Reset Queue",
					description: "Session not bound to remote stream",
					variant: "destructive",
					duration: 2000,
				});
			} catch {
				// ignore toast failure
			}
			return false;
		}

		console.warn("🔄 User manually triggered ReorderQueue reset", {
			sessionId,
		});

		try {
			const success =
				await streamingHubService.forceResetReorderQueue(sessionId);
			if (success) {
				debugTrace("✅ Queue reset successful");
				try {
					toast({
						title: "Video Queue Reset",
						description: "Requesting keyframe to restore video...",
						duration: 2000,
					});
				} catch {
					// ignore toast failure
				}
				return true;
			} else {
				console.warn("⚠️ Queue reset unsuccessful");
				try {
					toast({
						title: "Queue reset failed",
						description: "Please try again later",
						variant: "destructive",
						duration: 2000,
					});
				} catch {
					// ignore toast failure
				}
				return false;
			}
		} catch (error) {
			console.error("❌ Queue reset error:", error);
			try {
				toast({
					title: "Queue reset failed",
					description: "An error occurred, please try again",
					variant: "destructive",
					duration: 2000,
				});
			} catch {
				// ignore toast failure
			}
			return false;
		}
	}, [webrtcSessionId, toast]);

	const resolveWebrtcSessionId = useCallback(() => {
		if (webrtcSessionIdRef.current) {
			return webrtcSessionIdRef.current;
		}

		if (webrtcSessionId) {
			webrtcSessionIdRef.current = webrtcSessionId;
			return webrtcSessionId;
		}

		return null;
	}, [webrtcSessionId]);

	const resolveRemotePlaySessionId = useCallback(() => {
		if (remotePlaySessionIdRef.current) {
			return remotePlaySessionIdRef.current;
		}

		if (remotePlaySessionId) {
			remotePlaySessionIdRef.current = remotePlaySessionId;
			return remotePlaySessionId;
		}

		return null;
	}, [remotePlaySessionId]);

	const handleStreamHealthCheck = useCallback(
		async (reason, context) => {
			const streamSessionId = resolveRemotePlaySessionId();
			if (!streamSessionId) {
				return;
			}

			if (!resolveWebrtcSessionId()) {
				return;
			}

			const now = Date.now();
			if (healthCheckInFlightRef.current) {
				return;
			}

			const forceNeutral = context?.forceNeutral ?? false;

			if (now - lastHealthCheckAtRef.current < HEALTH_CHECK_COOLDOWN_MS) {
				const snapshot = lastStreamHealthRef.current;
				if (snapshot && snapshot.frozen > snapshot.recovered) {
					// On freeze, no longer auto-request a keyframe; only record last activity time for throttling
					lastVideoActivityRef.current = now;
					return;
				}

				if (forceNeutral) {
					// Within the cooldown period, neutral no longer auto-requests a keyframe
					// Only update last activity time to avoid repeated triggers in a short window
					lastVideoActivityRef.current = now;
				}
				return;
			}

			healthCheckInFlightRef.current = true;
			try {
				// ONLY what the child pushes over the input socket
				// (HealthUpdate, every 2 s — see the onHealthUpdate wiring
				// below). The GET …/health it fell back on, every 3 s, is
				// gone: the push never reached this page (the child filtered
				// on a hub name no connection carried), so the "fallback"
				// was the only path, and a poll. No fresh push, nothing to
				// judge.
				const PUSH_STALE_MS = 5000; // ~2 missed 2s server ticks
				const pushed = pushedHealthRef.current;
				if (!pushed || now - pushed.receivedAt >= PUSH_STALE_MS) {
					return;
				}
				const health = pushed.data;

				lastHealthCheckAtRef.current = Date.now();
				const previous = lastStreamHealthRef.current;

				// Merge backend debug fields (heartbeat/DATA_ACK/GMAC state)
				// into connectionStats for the Live Monitoring panel. Stored
				// as absolute anchor timestamps (now - agoMs) rather than the
				// precomputed "ago" delta itself, so the panel can recompute
				// "ago" at every render (the ~1s WebRTC stats tick already
				// re-renders regardless) instead of only updating once per
				// ~3s health-check poll.
				const healthReceivedAt = lastHealthCheckAtRef.current;
				setConnectionStats((prev) => ({
					...prev,
					debug: {
						heartbeatSentAt:
							health.heartbeatSentAgoMs != null
								? healthReceivedAt - health.heartbeatSentAgoMs
								: null,
						heartbeatReceivedAt:
							health.heartbeatReceivedAgoMs != null
								? healthReceivedAt - health.heartbeatReceivedAgoMs
								: null,
						dataAckAt:
							health.dataAckAgoMs != null
								? healthReceivedAt - health.dataAckAgoMs
								: null,
						dataAckCumulativeSeqNum: health.dataAckCumulativeSeqNum ?? null,
						keyPos: health.keyPos ?? null,
						gmacIndex: health.gmacIndex ?? null,
						nextExpectedTSN: health.nextExpectedTSN ?? null,
						congestionReceivedTotal: health.congestionReceivedTotal ?? null,
						congestionLostTotal: health.congestionLostTotal ?? null,
						status: health.status ?? null,
					},
				}));

				// Calculate delta values (based on previously stored values)
				const deltaFrozen = previous
					? Math.max(0, health.totalFrozenFrames - previous.frozen)
					: 0;
				const deltaRecovered = previous
					? Math.max(0, health.totalRecoveredFrames - previous.recovered)
					: 0;

				lastStreamHealthRef.current = {
					frozen: health.totalFrozenFrames,
					recovered: health.totalRecoveredFrames,
				};

				const hasNewFreeze = deltaFrozen > 0;
				const hasNewRecovery = deltaRecovered > 0;

				if (
					hasNewFreeze ||
					health.totalFrozenFrames > health.totalRecoveredFrames
				) {
					console.warn("⚠️ Stream freeze detected (auto keyframe disabled)", {
						reason,
						totalFrozenFrames: health.totalFrozenFrames,
						totalRecoveredFrames: health.totalRecoveredFrames,
						deltaFrozenFrames: deltaFrozen,
					});

					// No longer auto-request keyframe; only update activity time
					lastVideoActivityRef.current = Date.now();
					return;
				}

				let lastHandled = false;
				if (
					hasNewRecovery ||
					(previous && health.totalRecoveredFrames > previous.recovered)
				) {
					debugTrace("✅ Streaming frames recovered", {
						reason,
						totalRecoveredFrames: health.totalRecoveredFrames,
						totalFrozenFrames: health.totalFrozenFrames,
						deltaRecoveredFrames: deltaRecovered,
					});
					lastHandled = true;
					lastVideoActivityRef.current = Date.now();
				}

				if (!lastHandled && (health.totalFrozenFrames > 0 || forceNeutral)) {
					// No longer auto-request neutral keyframe; only update time and neutral timestamp
					lastVideoActivityRef.current = Date.now();
					lastNeutralHealthKeyframeRef.current = now;
				}
			} catch (error) {
				lastHealthCheckAtRef.current = Date.now();
				console.warn(
					"⚠️ Failed to fetch stream health (auto keyframe fallback disabled)",
					error,
				);
				if (
					error instanceof Error &&
					/does not exist or has ended/.test(error.message)
				) {
					remotePlaySessionIdRef.current = null;
				}
				// No longer auto-fallback to requesting a keyframe; only update activity time
				lastVideoActivityRef.current = Date.now();
			} finally {
				healthCheckInFlightRef.current = false;
			}
		},
		[
			HEALTH_CHECK_COOLDOWN_MS,
			resolveRemotePlaySessionId,
			resolveWebrtcSessionId,
			requestKeyframe,
			t,
			toast,
		],
	);

	// applyReceiverLatencyHints/reinforceLatencyHints — used to live here as
	// a hand-rolled copy of the same logic GatewayWebRTC now owns
	// internally (its own ontrack applies latency hints on every new
	// track, and its renegotiate() re-applies them to all receivers after
	// a device-pushed offer — see its own doc). Removed with the
	// RTCPeerConnection ownership itself.

	const stopStickProcessing = useCallback(() => {
		if (stickIntervalRef.current !== null) {
			clearInterval(stickIntervalRef.current);
			stickIntervalRef.current = null;
		}
		stickProcessingActiveRef.current = false;
		resetStickInput();
		lastSentRef.current = {
			leftX: 0,
			leftY: 0,
			rightX: 0,
			rightY: 0,
			l2: 0,
			r2: 0,
			timestamp: 0,
		};
	}, [resetStickInput]);

	const setupKeyboardControl = useCallback(() => {
		if (keyboardCleanupRef.current) {
			keyboardCleanupRef.current();
			keyboardCleanupRef.current = null;
		}

		const cleanup = createKeyboardHandler(
			async (buttonName, action) => {
				if (!hasControlRef.current) {
					return; // control handed off to a viewer
				}
				debugTrace("🎮 Keyboard control triggered:", buttonName, action, {
					isConnected: controllerService.isConnected(),
					buttonName,
					action,
				});

				try {
					// B2: with a gateway DataChannel sender there is no hub
					// socket to wait for — the key rides the channel directly.
					if (!controllerService.hasGatewaySender()) {
						let retries = 0;
						const maxRetries = 10;
						while (!controllerService.isConnected() && retries < maxRetries) {
							await new Promise((resolve) => setTimeout(resolve, 100));
							retries++;
						}

						if (!controllerService.isConnected()) {
							console.warn(
								"⚠️ Controller not ready, but attempting to send key:",
								buttonName,
								action,
							);
						}
					}

					// P6 — a real key press (this handler only fires on user
					// input): stamp the shared activity store.
					markActivity();
					debugTrace("📤 Sending button command:", buttonName, action);
					if (action === "press") {
						await controllerService.sendButton(buttonName, "press");
						debugTrace(
							"✅ Button command sent successfully:",
							buttonName,
							"press",
						);
					} else {
						await controllerService.sendButton(buttonName, "release");
						debugTrace(
							"✅ Button command sent successfully:",
							buttonName,
							"release",
						);
					}
				} catch (error) {
					console.error(
						"❌ Keyboard control failed:",
						error,
						"button:",
						buttonName,
						"action:",
						action,
					);
				}
			},
			{
				onLeftStickChange: (x, y) => {
					setKeyboardLeftStick(x, y);
				},
			},
		);

		keyboardCleanupRef.current = cleanup;
		debugTrace("✅ Keyboard control enabled");
	}, []);

	// connectController — the legacy SignalR-hub-based controller/input
	// path (as opposed to the gateway DataChannel's "input" channel) is
	// removed along with the rest of the non-gateway fallback (see
	// connect()/connectAttached()'s own doc — GAMEFLEX_GATEWAY_URL is a
	// required env var, confirmed dead in production).

	const startStickProcessing = useCallback(() => {
		if (stickProcessingActiveRef.current) {
			return;
		}

		stickProcessingActiveRef.current = true;
		lastSentRef.current.timestamp = 0;

		const readGamepadAxes = () => {
			try {
				const gamepads = navigator.getGamepads?.();
				if (!gamepads) {
					return;
				}

				for (let i = 0; i < gamepads.length; i++) {
					const gamepad = gamepads[i];
					if (!gamepad) {
						continue;
					}

					snapshotGamepadAxes(gamepad);
					break;
				}
			} catch (error) {
				logThrottled(
					"warn",
					"gamepad-read-state",
					5000,
					"⚠️ Failed to read gamepad state:",
					error,
				);
			}
		};

		const sendLatest = () => {
			// canSendInput(), not isConnected(): the transport is the gateway
			// DataChannel, and isConnected() only reports the raw socket.
			if (!isConnectedRef.current || !controllerService.canSendInput()) {
				return;
			}

			// This loop also carries the PHYSICAL gamepad and the keyboard
			if (!hasControlRef.current) {
				return; // control handed off to a viewer — stop sending our own input
			}

			// Check whether any virtual joystick is active (virtual joystick should work even without a physical gamepad)
			// Use the global virtual joystick state
			const hasVirtualJoystickActive =
				virtualJoystickActiveState.left || virtualJoystickActiveState.right;

			// If no physical gamepad is enabled and no virtual joystick is active, do not send
			// — this also gates keyboard-driven sticks (getNormalizedState folds
			// keyboard input in below), so a user toggling "gamepad enabled" off
			// silently stops keyboard movement too, not just a physical pad.
			if (
				(!gamepadEnabledRef.current || !isGamepadEnabled) &&
				!hasVirtualJoystickActive
			) {
				return;
			}

			readGamepadAxes();

			const now = performance.now();
			const normalized = getNormalizedState();
			const lastSent = lastSentRef.current;
			const stickDiff =
				Math.abs(normalized.leftX - lastSent.leftX) +
				Math.abs(normalized.leftY - lastSent.leftY) +
				Math.abs(normalized.rightX - lastSent.rightX) +
				Math.abs(normalized.rightY - lastSent.rightY);
			const triggerDiff =
				Math.abs(normalized.l2 - lastSent.l2) +
				Math.abs(normalized.r2 - lastSent.r2);
			const shouldHeartbeat =
				now - lastSent.timestamp >= MAX_HEARTBEAT_INTERVAL_MS;
			// Do NOT send unconditionally just because a virtual joystick is
			// "active" (held) — a steady, unchanged deflection should rely on
			// the shouldHeartbeat keep-alive like every other input path,
			// matching upstream's cadence. Sending on every SEND_INTERVAL_MS
			// tick while a key is held was flooding the PS5 (see
			// MAX_HEARTBEAT_INTERVAL_MS doc comment).
			const shouldSendSticks = stickDiff > AXIS_DEADZONE || shouldHeartbeat;
			const shouldSendTriggers =
				triggerDiff > TRIGGER_DEADZONE || shouldHeartbeat;
			// P6 — only REAL deflection marks activity: the shouldHeartbeat
			// branch of the same send is a transport keep-alive, not a
			// human.
			if (stickDiff > AXIS_DEADZONE || triggerDiff > TRIGGER_DEADZONE) {
				markActivity();
			}

			if (shouldSendSticks) {
				controllerService
					.sendSticks(
						normalized.leftX,
						normalized.leftY,
						normalized.rightX,
						normalized.rightY,
					)
					.catch((error) => {
						logThrottled(
							"error",
							"send-joystick-sendLatest",
							5000,
							"❌ Failed to send joystick input:",
							error,
						);
					});
			}

			if (shouldSendTriggers) {
				controllerService
					.sendTriggers(normalized.l2, normalized.r2)
					.catch((error) => {
						logThrottled(
							"error",
							"send-trigger-sendLatest",
							5000,
							"❌ Failed to send trigger pressure:",
							error,
						);
					});
			}

			if (shouldSendSticks || shouldSendTriggers) {
				lastSentRef.current = { ...normalized, timestamp: now };
			}
		};

		sendLatest();
		// Use a unified send interval (8ms), aligned between mobile and PC
		stickIntervalRef.current = window.setInterval(sendLatest, SEND_INTERVAL_MS);
	}, [getNormalizedState, isGamepadEnabled]);

	const handleGamepadInput = useCallback(
		async (event) => {
			// Only check basic connection state; do not strictly require controllerService.isConnected()
			// because sendButton internally handles connection state checks and reconnection
			if (
				!isConnectedRef.current ||
				!gamepadEnabledRef.current ||
				!isGamepadEnabled
			) {
				return;
			}
			if (!hasControlRef.current) {
				return; // control handed off to a viewer — stop sending our own input
			}

			// P6 — discrete gamepad events are real input (the poll loop
			// below marks only its own real deflection).
			markActivity();

			try {
				if (event.buttonIndex !== undefined && event.buttonState) {
					const buttonIndex = event.buttonIndex;
					const buttonState = event.buttonState;
					const isPressed = buttonState.pressed;
					const psButtonName = PS5_BUTTON_MAP[buttonIndex];

					if (buttonIndex === GamepadButton.LeftTrigger) {
						setTriggerPressure("l2", buttonState.value ?? 0);
					} else if (buttonIndex === GamepadButton.RightTrigger) {
						setTriggerPressure("r2", buttonState.value ?? 0);
					}

					if (psButtonName) {
						const action = isPressed ? "press" : "release";
						// Do not await sendButton to avoid blocking other key processing
						// sendButton internally handles connection state checks and reconnection
						controllerService
							.sendButton(psButtonName, action)
							.catch((error) => {
								// Only log non-connection-related errors
								if (
									error?.message &&
									!error.message.includes("connection") &&
									!error.message.includes("closed")
								) {
									logThrottled(
										"error",
										"send-button-face",
										5000,
										"❌ Failed to send gamepad button:",
										error,
									);
								}
							});
					} else if (buttonIndex >= 12 && buttonIndex <= 15) {
						const dpadMap = {
							12: "up",
							13: "down",
							14: "left",
							15: "right",
						};
						const dpadButton = dpadMap[buttonIndex];
						if (dpadButton) {
							const action = isPressed ? "press" : "release";
							// Do not await sendButton to avoid blocking other key processing
							controllerService
								.sendButton(dpadButton, action)
								.catch((error) => {
									// Only log non-connection-related errors
									if (
										error?.message &&
										!error.message.includes("connection") &&
										!error.message.includes("closed")
									) {
										logThrottled(
											"error",
											"send-button-dpad",
											5000,
											"❌ Failed to send gamepad button:",
											error,
										);
									}
								});
						}
					}
				}

				if (event.axisIndex !== undefined && event.axisValue !== undefined) {
					handleGamepadAxis(event.axisIndex, event.axisValue);

					const now = performance.now();
					const normalized = getNormalizedState();
					const lastSent = lastSentRef.current;
					const stickDiff =
						Math.abs(normalized.leftX - lastSent.leftX) +
						Math.abs(normalized.leftY - lastSent.leftY) +
						Math.abs(normalized.rightX - lastSent.rightX) +
						Math.abs(normalized.rightY - lastSent.rightY);
					const triggerDiff =
						Math.abs(normalized.l2 - lastSent.l2) +
						Math.abs(normalized.r2 - lastSent.r2);
					// Use the same heartbeat interval as the send logic (MAX_HEARTBEAT_INTERVAL_MS)
					const shouldHeartbeat =
						now - lastSent.timestamp >= MAX_HEARTBEAT_INTERVAL_MS;
					const shouldSendSticks = stickDiff > AXIS_DEADZONE || shouldHeartbeat;
					const shouldSendTriggers =
						triggerDiff > TRIGGER_DEADZONE || shouldHeartbeat;

					if (shouldSendSticks) {
						controllerService
							.sendSticks(
								normalized.leftX,
								normalized.leftY,
								normalized.rightX,
								normalized.rightY,
							)
							.catch((error) => {
								logThrottled(
									"error",
									"send-joystick-handleGamepadInput",
									5000,
									"❌ Failed to send joystick input:",
									error,
								);
							});
					}

					if (shouldSendTriggers) {
						controllerService
							.sendTriggers(normalized.l2, normalized.r2)
							.catch((error) => {
								logThrottled(
									"error",
									"send-trigger-handleGamepadInput",
									5000,
									"❌ Failed to send trigger pressure:",
									error,
								);
							});
					}

					if (shouldSendSticks || shouldSendTriggers) {
						lastSentRef.current = { ...normalized, timestamp: now };
					}
				}
			} catch (error) {
				logThrottled(
					"error",
					"gamepad-input-processing",
					5000,
					"❌ Gamepad input processing failed:",
					error,
				);
			}
		},
		[getNormalizedState, isGamepadEnabled, setTriggerPressure],
	);

	// explicit — true for a real "quit" (handleExit's confirm modal in
	// Player.jsx): tears down the SHARED backend session for everyone, right
	// now. false for the connect-effect's own cleanup, which runs on EVERY
	// unmount, including a plain navigation away from the page — passing
	// true there used to kill the PS4/PS5 session the instant the tab
	// changed, contradicting the comment a few lines below this function
	// ("a genuine close/navigation falls back to the backend's own hub-based
	// grace period") which was never actually wired to match. false skips
	// the stop-session/deleteSession calls below and lets the WS/hub
	// connection just drop, same as retro-gaming's leavePage.
	const disconnect = useCallback((explicit = true) => {
		stopStickProcessing();
		gamepadEnabledRef.current = false;
		tearDownMouseRightStick();
		// Phase B1: drop the gateway DataChannel transport — the channel dies
		// with its peer connection anyway, but the sender must not linger.
		controllerService.clearGatewayInputSender();

		isStreamBoundRef.current = false;
		hasVideoTrackRef.current = false;

		if (videoOptimizeCleanupRef.current) {
			videoOptimizeCleanupRef.current();
			videoOptimizeCleanupRef.current = null;
		}

		detachVideoListenersRef.current?.();

		if (keyframeMonitorIntervalRef.current !== null) {
			window.clearInterval(keyframeMonitorIntervalRef.current);
			keyframeMonitorIntervalRef.current = null;
		}
		lastVideoActivityRef.current = 0;
		lastDecodedFrameCountRef.current = null;
		lastPlaybackPositionRef.current = null;
		lastKeyframeRequestRef.current = 0;
		pendingKeyframeRequestRef.current = false;
		initialKeyframeRequestedRef.current = false;

		if (keyboardCleanupRef.current) {
			keyboardCleanupRef.current();
			keyboardCleanupRef.current = null;
		}

		controllerService.disconnect().catch(() => { });

		// Push-based ICE candidates / health (Phase 3 WS migration) —
		// unsubscribe from the shared singleton so a later session doesn't
		// stack a second listener on top of this one.
		if (iceCandidateUnsubscribeRef.current) {
			iceCandidateUnsubscribeRef.current();
			iceCandidateUnsubscribeRef.current = null;
		}
		if (healthUpdateUnsubscribeRef.current) {
			healthUpdateUnsubscribeRef.current();
			healthUpdateUnsubscribeRef.current = null;
		}
		pushedHealthRef.current = null;

		streamingHubService.disconnect().catch(() => { });

		// No screenRef.cleanup() here: the PAGE owns the gateway viewer
		// (see SessionStream), so releasing it from a platform disconnect
		// killed the picture for the whole page — and nothing brings it
		// back, since SessionStream stops retrying once attached. The
		// viewer goes away with GatewayWebRTC's own unmount, nowhere else.

		resetConnectionStats();
		setConnectionStats(null);

		// ARRÊTER LA PARTIE N'EST PLUS L'AFFAIRE DE CE HOOK. La page l'arrête
		// pour tout le monde par la commande `session.stop` du centre de
		// messages, que le child juge — seul le propriétaire le peut (voir
		// stopSessionForEveryone, Streaming.jsx). Ce hook appelait en plus
		// `/playstation/stop-session`, une route qui arrêtait n'importe quelle
		// session pour n'importe quel jeton : retirée le 2026-10-09 (#275).
		const currentWebrtcSessionId = webrtcSessionId;
		const wasAttached = isAttachedRef.current;
		isAttachedRef.current = false;
		viewerIdRef.current = null;
		setWebrtcSessionId(null);
		webrtcSessionIdRef.current = null;
		setRemotePlaySessionId(null);

		if (!wasAttached && explicit && currentWebrtcSessionId && hasControlRef.current) {
			streamingService
				.deleteSession(currentWebrtcSessionId)
				.then(() => {
					debugTrace("✅ WebRTC Session closed");
				})
				.catch((error) => {
					console.error("❌ Failed to close WebRTC Session:", error);
				});
		}

		setIsConnected(false);
		isConnectedRef.current = false;
		setIsConnecting(false);
		setConnectionState(t("streaming.connection.state.disconnected"));
		// On disconnect, reset the error callback flag and stalling state
		hasTriggeredErrorCallbackRef.current = false;
		setIsStalling(false);
	}, [screenRef, stopStickProcessing, t, webrtcSessionId]);

	// Warn before leaving/refreshing while a stream is active. Tearing down
	// the backend session on a mere tab-background used to live here too
	// (pagehide + sendBeacon), but that fires on backgrounding as much as on
	// a real close (confirmed: switching apps fires it on iOS Safari even
	// though the user comes right back) — that decision now belongs entirely
	// to the away-confirm modal (useAwayConfirm, wired at the component
	// level via usePictureInPictureOnHide's onVisibilityChange), which asks
	// instead of guessing. A genuine close/navigation still falls back to
	// the backend's own hub-based grace period (signalr_hub.go) exactly like
	// the no-control branch below already does.
	// The leave-confirmation prompt moved to Streaming.jsx (2026-08-24): it
	// was remote-play only, so refreshing a retro or streamex session
	// interrupted play with no warning at all. Nothing about it is
	// platform-specific.

	// shared session. GatewayWebRTC (screenRef) now owns the
	// RTCPeerConnection/ICE/offer-answer mechanics this used to duplicate
	// — the DataChannel/ontrack/onconnectionstatechange side effects that
	// used to live inline here moved into handleConnectionStateChange/
	// handleGatewayMessage below, shared with connect()'s own owner flow.
	const connectAttached = useCallback(
		async (sessionId) => {
			// Troisième occurrence de la même confusion (voir `connect()`) :
			// `isConnected` décrit le viewer de la passerelle, pas le flux de
			// la console. Reprendre une session déjà liée n'a pas de sens ;
			// la reprendre parce qu'un viewer est debout, si.
			if (isStreamBoundRef.current) {
				return;
			}
			isAttachedRef.current = true;
			setIsConnecting(true);
			setConnectionState(t("streaming.connection.state.connecting"));

			try {
				const offerResponse = await viewerService.createOffer(
					sessionId,
					"H264",
				);
				if (!offerResponse.success || !offerResponse.data) {
					throw new Error(
						offerResponse.errorMessage ||
						t("streaming.connection.errors.offerCreateFailed"),
					);
				}
				// username travels with the offer — the child resolves the
				// B2 input gate (viewerUsers) with it.
				// RETRIED, like SessionStream's own attach. A session that was
				// just launched has no stream registered on the gateway yet —
				// its child is still coming up — so the first try legitimately
				// answers 404. The Dashboard's preview has always retried
				// through that window; this page gave up on the first failure
				// and showed a fatal error for a session that was simply not
				// ready (reported live 2026-08-25). Same 2s cadence, bounded.
				// UNE PAGE, UN VIEWER — et celui-ci appartient à la PAGE.
				//
				// Ce hook en demandait un SECOND sur le même composant, ce que
				// retro et streamex ont cessé de faire il y a longtemps : le
				// getter `viewerId` de GatewayWebRTC porte l'avertissement
				// depuis, mot pour mot (« au lieu d'appeler createViewer une
				// seconde fois et de démolir la connexion »).
				//
				// Le second appel remplaçait la PeerConnection du composant,
				// donc l'élément vidéo se retrouvait branché sur un pair
				// démoli. Le son continuait — il n'avait pas besoin de
				// redémarrer un décodeur — et l'image, elle, n'avait plus
				// d'image-clé à laquelle se raccrocher. D'où un écran noir
				// avec du son, symptôme consigné le 2026-08-25 et laissé
				// ouvert faute de console pour le vérifier (voir
				// TROUBLESHOOTING.md, « Pas d'image sur la page remoteplay,
				// mais Live Now affiche la même session »).
				//
				// On ATTEND donc celui de la page plutôt que d'en créer un.
				// Il arrive avec sa propre boucle de reprise (SessionStream) :
				// une session qui vient d'être lancée n'a pas encore de flux
				// enregistré sur la passerelle.
				let viewerId = null;
				for (let attempt = 0; attempt < 8 && !viewerId; attempt += 1) {
					if (attempt > 0) {
						await new Promise((resolve) => setTimeout(resolve, 2000));
					}
					viewerId = screenRef.current?.viewerId || null;
				}
				if (!viewerId) {
					throw new Error(t("streaming.connection.errors.offerCreateFailed"));
				}
				const viewer = { viewerId };
				viewerIdRef.current = viewer.viewerId;

				setRemotePlaySessionId(sessionId);
				setWebrtcSessionId(sessionId);
				webrtcSessionIdRef.current = sessionId;

				// Health push subscription (handleStreamHealthCheck reads
				// pushedHealthRef instead of polling GET .../health) — the
				// ICE-candidate push/poll this used to sit alongside is gone
				// (GatewayWebRTC's own attach() receives the gateway's
				// candidates on its negotiation socket), health stays since
				// nothing else provides it.
				// The third argument is what the child's queue matches on —
				// iceKey cannot serve: for an owner it IS the sessionId.
				// controllerService and streamingHubService are the SAME object
				// under two aliases (see the imports) — so this one call binds
				// the input socket too. The third argument is what the child's
				// queue matches on; iceKey cannot serve, since for an owner it
				// IS the sessionId and could not tell two owner tabs apart.
				streamingHubService.joinSession(sessionId, viewer.viewerId, viewer.viewerId).catch(() => { });
				healthUpdateUnsubscribeRef.current = streamingHubService.onHealthUpdate(
					(health) => {
						pushedHealthRef.current = { data: health, receivedAt: Date.now() };
					},
				);
			} catch (err) {
				console.error("❌ Failed to attach to session:", err);
				toast({
					title: t("common.error"),
					description: err instanceof Error ? err.message : String(err),
					variant: "destructive",
				});
				setIsConnecting(false);
				// A failed ATTACH is not a quit — never stop the shared
				// session over our own signaling failure (it survives for
				// the next attempt and for other viewers).
				disconnect(false);
			}
		},
		[isConnecting, isConnected, t, toast, username, disconnect],
	);

	// handleConnectionStateChange — wired as GatewayWebRTC's own
	// onConnectionStateChange prop, shared by BOTH connect() and
	// connectAttached() (the same mounted <GatewayWebRTC>/screenRef
	// drives whichever one actually ran — this only cares that ITS
	// RTCPeerConnection reached "connected"/"failed"/etc, not which
	// caller got it there). Forwards to the shared hook's own
	// isConnected/isConnecting/connectionState handling first (same
	// compose pattern use-streamex-streaming-connection.js already uses),
	// then layers the PS5-only side effects (gamepad/keyboard/stick
	// enable, initial keyframe, hasControl) both entry points used to
	// duplicate with slightly different shapes.
	const handleConnectionStateChange = useCallback(
		(state) => {
			handleSharedConnectionStateChange(state);
			switch (state) {
				case "connected": {
					isConnectedRef.current = true;
					isStreamBoundRef.current = true;
					if (streamingStartTimeRef.current === null) {
						streamingStartTimeRef.current = Date.now();
					}
					hasTriggeredErrorCallbackRef.current = false;
					// connectAttached's own reason for existing: we navigated
					// here BECAUSE control was just granted (harmless no-op
					// for connect()'s own fresh-owner flow, which already
					// starts hasControl=true).
					hasControlRef.current = true;
					setHasControl(true);
					gamepadEnabledRef.current = true;
					// Phase B1: the gateway's "input" DataChannel carries
					// controller traffic — GatewayWebRTC's sendMessage
					// already no-ops until the channel is actually open, so
					// this can register unconditionally instead of waiting
					// for an onopen event the shared component doesn't
					// expose per-channel.
					controllerService.setGatewayInputSender({
						send: (message) => screenRef.current?.sendMessage(JSON.stringify(message), "input"),
					});
					setupKeyboardControl();
					startStickProcessing();
					if (!initialKeyframeRequestedRef.current) {
						if (requestKeyframe("post-bind-initial-video")) {
							initialKeyframeRequestedRef.current = true;
						}
					}
					break;
				}
				case "disconnected":
				case "failed":
				case "closed": {
					const wasConnected = isConnectedRef.current;
					isConnectedRef.current = false;
					if (wasConnected && onConnectionError && !hasTriggeredErrorCallbackRef.current) {
						hasTriggeredErrorCallbackRef.current = true;
						onConnectionError(
							state === "failed"
								? t("streaming.connection.errors.webrtcFailed", "WebRTC connection failed")
								: t("streaming.connection.errors.connectionClosed", "Connection closed"),
						);
					}
					break;
				}
				default:
					break;
			}
		},
		[handleSharedConnectionStateChange, onConnectionError, requestKeyframe, setupKeyboardControl, startStickProcessing, t],
	);

	// handleGatewayMessage — wired as GatewayWebRTC's onMessage prop.
	// label distinguishes the DataChannels a device-streaming platform can
	// open alongside the generic "input" every platform uses: "keepalive"
	// is remote-play's own ping/pong liveness channel (started for PS5,
	// generic enough that another console reusing the same pattern gets it
	// for free). retro/streamex never open a second channel, so their own
	// onMessage callers only ever see label==="input".
	const handleGatewayMessage = useCallback((data, label) => {
		if (label === "keepalive") {
			if (data instanceof ArrayBuffer) {
				const buffer = new Uint8Array(data);
				if (buffer.length > 0 && buffer[0] === 0x01) {
					// Received PING (0x01), respond PONG (0x02).
					screenRef.current?.sendMessage(new Uint8Array([0x02]).buffer, "keepalive");
				}
			}
			return;
		}
		// "input" (default label) — B2 downlink: rumble pushed from the
		// console to whichever viewer currently holds the pad.
		controllerService.dispatchGatewayMessage(data);
	}, []);

	// The low-latency answer-SDP rewrite used to be passed from here as a
	// remote-play-only prop. It never was platform-specific — it only adds
	// generic hints (a=x-google-flag:low-latency, a=minBufferedPlaybackTime)
	// that every game stream wants — so GatewayWebRTC now applies it to all
	// three platforms itself, on the browser-to-gateway leg it owns.

	// handleIceConnectionStateChange — wired as GatewayWebRTC's
	// onIceConnectionStateChange prop. A connection that comes back clears the
	// "error already reported" latch. An ICE restart is GatewayWebRTC's own
	// business (renegotiate on disconnect): the page also asked remoteplay for
	// a fresh offer, a path that answered nothing once its in-process WebRTC
	// was gone, and that left with it (#257).
	const handleIceConnectionStateChange = useCallback((iceState) => {
		if (iceState === "connected" || iceState === "completed") {
			hasTriggeredErrorCallbackRef.current = false;
		}
	}, []);

	// onFirstVideoTrack — wired as GatewayWebRTC's own prop, fires
	// once per attach() the first time a video track shows up.
	// GatewayWebRTC already owns the generic mute/fade-in/autoplay-
	// retry mechanics (audio on its own element now, like every other
	// platform — the single-<video> mode was dropped) — what's left here is
	// PS5-specific: the initial keyframe request, low-latency video tuning,
	// and the stall/keyframe-nudge DOM listeners (setIsStalling, "waiting"/
	// "stalled" re-requesting a keyframe). Wired once here instead of
	// inside GatewayWebRTC's own opaque ontrack, using the read-only
	// videoRef proxy (see this hook's own param doc).
	const onFirstVideoTrack = useCallback(() => {
		hasVideoTrackRef.current = true;
		if (isStreamBoundRef.current && !initialKeyframeRequestedRef.current) {
			if (requestKeyframe("initial-video-track")) {
				initialKeyframeRequestedRef.current = true;
			}
		}

		const video = videoRef.current;
		if (!video) {
			return;
		}

		if (videoOptimizeCleanupRef.current) {
			videoOptimizeCleanupRef.current();
		}
		videoOptimizeCleanupRef.current = optimizeVideoForLowLatency(video);
		video.playbackRate = 1.0;
		video.defaultPlaybackRate = 1.0;
		try {
			if (typeof video.latencyHint !== "undefined") {
				video.latencyHint = "interactive";
			}
		} catch {
			// best-effort
		}

		detachVideoListenersRef.current?.();
		const wired = [];
		const on = (type, handler) => {
			wired.push([type, handler]);
			video.addEventListener(type, handler);
		};
		detachVideoListenersRef.current = () => {
			wired.forEach(([type, handler]) => video.removeEventListener(type, handler));
			detachVideoListenersRef.current = null;
		};

		on("playing", () => {
			setIsConnecting(false);
			setIsConnected(true);
			isConnectedRef.current = true;
			setConnectionState(t("streaming.connection.state.connected"));
			if (streamingStartTimeRef.current === null) {
				streamingStartTimeRef.current = Date.now();
			}
			setIsStalling(false);
		});
		on("waiting", () => {
			setIsStalling(true);
			if (video.paused) {
				video.play().catch(() => { });
			}
			requestKeyframe("video-waiting");
		});
		on("stalled", () => {
			setIsStalling(true);
			if (video.paused) {
				video.play().catch(() => { });
			}
			requestKeyframe("video-stalled");
		});
		on("progress", () => {
			if (video.paused && video.readyState >= 2) {
				video.play().catch(() => { });
			}
		});
		on("error", (e) => {
			console.error("❌ Video playback error:", e, video.error);
		});
	}, [requestKeyframe, t, videoRef]);

	// connect — fresh OWNER connection (a brand-new or preCreatedSessionId
	// remote-play session). Dispatches to connectAttached for the
	// control-handoff/promotion case (attachSessionId set) — see that
	// function's own doc. GatewayWebRTC (screenRef) now owns the
	// RTCPeerConnection/ICE/offer-answer mechanics this used to duplicate;
	// what's left here is remote-play's own session bootstrap
	// (prepareDevice/startSession/createOffer — nothing to do with generic
	// WebRTC) plus the PS5 stream bind (connectToRemotePlaySession).
	const connect = useCallback(async () => {
		if (attachSessionId) {
			return connectAttached(attachSessionId);
		}
		if (!hostId) {
			toast({
				title: t("common.error"),
				description: t("streaming.connection.errors.missingDeviceInfo"),
				variant: "destructive",
			});
			return;
		}
		// LA MÊME CONFUSION QUE DANS L'EFFET D'AUTO-CONNEXION, ici aussi.
		//
		// Ce garde-fou testait `isConnecting || isConnected` — l'état du
		// VIEWER de la passerelle, monté par le composant vidéo partagé. Or
		// cette fonction-ci ne monte pas un viewer : elle demande à la console
		// d'ÉMETTRE (`createOffer` puis `connectToRemotePlaySession`). Le
		// viewer debout ne dit rien de ce travail-là.
		//
		// Sur le téléphone la passerelle est en processus : le viewer est déjà
		// connecté quand la temporisation de 500 ms expire, donc `connect()`
		// ressortait ici sans rien faire — écran noir, aucune trace de
		// `webrtc/offer` (2026-09-10, deux essais, deux adresses).
		//
		// La question juste est « suis-je DÉJÀ en train de lier, ou lié ? ».
		// `hasAttemptedInitialConnectRef` répond à la première (il est posé
		// avant la temporisation de l'effet, et ci-dessous pour un appel
		// direct), `isStreamBoundRef` à la seconde.
		if (isStreamBoundRef.current) {
			return;
		}
		hasAttemptedInitialConnectRef.current = true;

		setIsConnecting(true);
		setConnectionState(t("streaming.connection.state.connecting"));

		try {
			// preCreatedSessionId means the device was already discovered/
			// woken/handshaked moments ago by whoever created this session
			// (RemotePlay.jsx's own Connect button calls startSession()
			// itself before navigating here) — re-running the FULL
			// discover/wake sequence on arrival is redundant, and risky
			// right after a session that just succeeded: confirmed live
			// (2026-08-22) that landing here with a preCreatedSessionId
			// never even reached webrtc/offer (zero trace in
			// gateway-webrtc's logs across several attempts) — consistent
			// with prepareDevice()'s own status branches (STANDBY/OK/READY/
			// AVAILABLE) not accounting for "already streaming", the state
			// the PS5 is actually in at this exact point.
			const deviceReady = preCreatedSessionId ? true : await prepareDevice();
			if (!deviceReady) {
				throw new Error(t("streaming.connection.errors.deviceNotReady"));
			}

			setConnectionState(t("streaming.connection.state.creatingSession"));

			// preCreatedSessionId (see its own doc above) skips this REST
			// call entirely — la porte est idempotente de toute façon :
			// `launchRemoteplay` rend la session déjà en cours pour cet
			// appareil au lieu d'en relancer une.
			let sessionId = preCreatedSessionId;
			if (!sessionId) {
				const sessionResponse = await streamingService.startSession(hostId, username);
				if (!sessionResponse.success) {
					throw new Error(
						sessionResponse.errorMessage ||
						sessionResponse.message ||
						t("streaming.connection.errors.sessionCreateFailed"),
					);
				}
				const sessionData = sessionResponse.data || sessionResponse.result;
				if (!sessionData) {
					throw new Error(t("streaming.connection.errors.sessionDataMissing"));
				}
				sessionId = sessionData.id || sessionData.Id || sessionData.sessionId || sessionData.session_id;
			}
			setRemotePlaySessionId(sessionId);
			if (!sessionId) {
				throw new Error(t("streaming.connection.errors.sessionIdMissing"));
			}

			const offerResponse = await streamingService.createOffer({
				remotePlaySessionId: sessionId,
				preferLanCandidates: isLikelyLan,
			});
			if (!offerResponse.success) {
				throw new Error(
					offerResponse.errorMessage ||
					offerResponse.message ||
					t("streaming.connection.errors.offerCreateFailed"),
				);
			}
			const offerData = offerResponse.data || offerResponse.result;
			if (!offerData) {
				throw new Error(t("streaming.connection.errors.offerDataMissing"));
			}

			// L'IDENTIFIANT DU FLUX, PAS UN `sessionId` (2026-10-09). En mode
			// passerelle — le seul qui reste — l'offre répond `{gateway,
			// streamId, type}`, sur le serveur comme sur le téléphone. Lire
			// `sessionId` donnait undefined : le socket d'entrée ne s'ouvrait
			// jamais au premier lancement, et avec lui la santé du flux, la
			// vibration et les demandes d'image clé. Seule la reprise d'une
			// partie, qui passe par l'autre branche, l'ouvrait.
			const { streamId: webrtcSessionIdValue } = offerData;
			setWebrtcSessionId(webrtcSessionIdValue);
			webrtcSessionIdRef.current = webrtcSessionIdValue;

			// No gateway viewer created here anymore (2026-08-24). The PAGE
			// owns exactly one viewer for its whole life, created from
			// session.id alone with its own retry loop (see SessionStream),
			// and `sessionId` here IS session.id: use-remoteplay-input.js
			// always passes preCreatedSessionId and never attachSessionId,
			// so the branch above assigned `sessionId = preCreatedSessionId`.
			// Creating a second viewer at this point tore the page's one down
			// and restarted the picture. The page's retry picks the stream up
			// within ~2s of the offer below registering it on the gateway.

			// Health push subscription — handleStreamHealthCheck reads
			// pushedHealthRef instead of polling GET .../health. Keyed by
			// webrtcSessionIdValue, matching the pre-migration code (the
			// ICE-candidate push this used to sit alongside is gone,
			// GatewayWebRTC's own attach() receives the gateway's
			// candidates on its negotiation socket).
			// Le viewer de la page, s'il existe déjà : c'est lui que la file du
			// child reconnaît quand une commande passe par ce socket.
			streamingHubService.joinSession(webrtcSessionIdValue, undefined, screenRef.current?.viewerId || null).catch(() => { });
			healthUpdateUnsubscribeRef.current = streamingHubService.onHealthUpdate((health) => {
				pushedHealthRef.current = { data: health, receivedAt: Date.now() };
			});

			// Phase 3: gateway mode — the streamId IS the session id;
			// ConnectWebRTC resolves the gateway-mode StreamState by it.
			const connectResponse = await streamingService.connectToRemotePlaySession(sessionId, sessionId);
			if (!connectResponse.success) {
				throw new Error(
					connectResponse.errorMessage ||
					connectResponse.message ||
					t("streaming.connection.errors.connectRemotePlayFailed"),
				);
			}
			isStreamBoundRef.current = true;

			// LA LIGNE OUBLIÉE PAR LA MIGRATION, remise là où elle était.
			//
			// L'ancien code l'écrivait ici même, juste après la liaison du
			// flux : « gamepadEnabledRef.current = true » puis
			// « ✅ Gamepad input enabled ». Le passage au composant vidéo
			// partagé a gardé tout le reste du bloc — la demande d'image-clé,
			// l'état connecté — et perdu celle-là. Elle ne survivait plus que
			// dans connectAttached, le chemin qu'on emprunte en REPRENANT le
			// contrôle d'une session, jamais en démarrant la sienne.
			//
			// Ce que ça coûtait, sur toute partie PS5 lancée normalement :
			// l'entrée d'une manette PHYSIQUE (handleGamepadInput y est
			// gaté) et le traitement des sticks. Le joystick tactile, lui,
			// passait quand même par son propre contournement
			// (hasVirtualJoystickActive) — d'où une panne qui ressemblait à
			// « tout marche sauf la vibration » alors qu'elle était plus
			// large (2026-09-09).
			gamepadEnabledRef.current = true;
			// LA DEUXIÈME LIGNE DU MÊME BLOC, perdue avec la première.
			//
			// L'ancien code posait ici `isConnectedRef.current = true` juste
			// après. Aujourd'hui ce drapeau n'est plus mis à vrai que dans le
			// gestionnaire « playing » de l'élément vidéo — un ÉVÉNEMENT, qui
			// se rate : la lecture démarre dès que la piste est posée, et si
			// elle part avant que les écouteurs ne soient attachés, personne
			// ne le voit passer. Le drapeau reste alors faux pendant que
			// l'image joue.
			//
			// C'est ce que gardait le gestionnaire de vibration après le
			// correctif précédent, et c'est pourquoi la sonde disait « reçu
			// par la page » sans jamais dire « vibration appliquée »
			// (2026-09-09).
			//
			// Seul le REF est posé ici, pas l'état React : c'est lui que les
			// gardes lisent, et laisser « playing » décider de ce que le
			// joueur VOIT évite de lui annoncer une image qui n'est pas
			// encore là.
			isConnectedRef.current = true;

			// Requested REGARDLESS of hasVideoTrackRef/onFirstVideoTrack's
			// own attempt: onFirstVideoTrack can fire (video track arrives)
			// BEFORE this point — isStreamBoundRef.current was still false
			// then, so its own gated request was a no-op — and a page that
			// attached before the console's first IDR would otherwise sit
			// on black frames until a manual refresh. This was a live bug
			// found testing the migration on real hardware (2026-08-21):
			// dropping this fallback reintroduced exactly the chicken-and-
			// egg race the original code's own comment warned about.
			if (!initialKeyframeRequestedRef.current) {
				if (requestKeyframe("post-bind-initial-video")) {
					initialKeyframeRequestedRef.current = true;
				}
			}
		} catch (error) {
			console.error("Connection failed:", error);
			setConnectionState(t("streaming.connection.state.failed"));

			let errorReason = t("streaming.connection.errors.connectionFailed", "Connection failed");
			if (error instanceof Error) {
				const errorMessage = error.message.toLowerCase();
				if (errorMessage.includes("timeout")) {
					errorReason = t("streaming.connection.errors.connectionTimeout", "Connection timeout");
				} else if (errorMessage.includes("device")) {
					errorReason = error.message;
				} else if (errorMessage.includes("session")) {
					errorReason = t("streaming.connection.errors.sessionFailed", "Session creation failed");
				} else if (errorMessage.includes("offer")) {
					errorReason = t("streaming.connection.errors.offerFailed", "Offer creation failed");
				} else {
					errorReason = error.message || errorReason;
				}
			}

			if (onConnectionError && !hasTriggeredErrorCallbackRef.current) {
				hasTriggeredErrorCallbackRef.current = true;
				onConnectionError(errorReason);
			}

			// A failed CONNECT is not a quit — never stop the shared session
			// over our own signaling failure (it survives for the next
			// attempt and for other viewers).
			disconnect(false);
		} finally {
			setIsConnecting(false);
		}
	}, [
		attachSessionId,
		connectAttached,
		disconnect,
		hostId,
		isConnected,
		isConnecting,
		isLikelyLan,
		onConnectionError,
		prepareDevice,
		preCreatedSessionId,
		t,
		toast,
		username,
	]);

	useEffect(() => {
		if (!isConnected) {
			tearDownMouseRightStick();
			return;
		}

		setupMouseRightStick();
		return () => {
			tearDownMouseRightStick();
		};
	}, [isConnected, setupMouseRightStick, tearDownMouseRightStick]);

	useEffect(() => {
		const unsubscribe = controllerService.onRumble((event) => {
			// RECEVOIR UNE VIBRATION N'EST PAS ENVOYER UNE ENTRÉE.
			//
			// Ce gestionnaire recopiait la garde de handleGamepadInput —
			// `gamepadEnabledRef` et `isGamepadEnabled` — qui protège tout
			// autre chose : l'envoi d'entrée depuis une manette PHYSIQUE. Le
			// joystick tactile a d'ailleurs son propre contournement juste à
			// côté (hasVirtualJoystickActive), et c'est pour ça qu'on peut
			// jouer au doigt pendant que ce drapeau est faux.
			//
			// Or `gamepadEnabledRef` n'est mis à vrai QUE dans
			// connectAttached — le chemin qu'on emprunte en reprenant le
			// contrôle d'une session, jamais en démarrant la sienne. Un
			// joueur qui lance sa propre partie PS5 n'a donc jamais eu de
			// vibration, et le message arrivait pourtant jusqu'ici : le
			// gateway le livrait sans une erreur, le serveur journalisait
			// « envoyé à gino » (2026-09-09).
			//
			// Ce qui compte pour recevoir : être connecté. Le repli sur la
			// vibration du téléphone, lui, se débrouille sans manette (voir
			// applyControllerRumbleToGamepads).
			if (!isConnectedRef.current) {
				return;
			}

			// L'interrupteur et l'intensité sont lus par le moteur
			// partagé (libs/rumble.js), depuis l'unique magasin de
			// préférences. Les relire ici demandait de tenir un second
			// abonnement d'accord avec le premier — c'est justement ce qui
			// avait divergé.
			applyControllerRumbleToGamepads(event);
		});

		return () => {
			unsubscribe();
		};
	}, [isGamepadEnabled]);

	// Always enable useGamepadInput, check inside handleGamepadInput
	// This ensures event listeners update even when gamepadEnabledRef changes
	useGamepadInput(handleGamepadInput, true);

	useEffect(() => {
		webrtcSessionIdRef.current = webrtcSessionId;
	}, [webrtcSessionId]);

	useEffect(() => {
		remotePlaySessionIdRef.current = remotePlaySessionId;
	}, [remotePlaySessionId]);

	// onConnectionError is usually an inline closure in the page component
	// (new identity on every render) — the ref keeps the stall watchdog
	// below independent of it, so a render during a freeze can't tear the
	// interval down and reset the stall clock (which would defeat the 8 s
	// fuse and leave a permanent black screen).
	const onConnectionErrorRef = useRef(onConnectionError);
	useEffect(() => {
		onConnectionErrorRef.current = onConnectionError;
	}, [onConnectionError]);

	useEffect(() => {
		if (keyframeMonitorIntervalRef.current !== null) {
			window.clearInterval(keyframeMonitorIntervalRef.current);
			keyframeMonitorIntervalRef.current = null;
		}

		if (!isConnected) {
			return;
		}

		if (!resolveWebrtcSessionId()) {
			return;
		}

		const STALL_THRESHOLD_MS = 1500;
		// How long we tolerate zero video activity before treating the
		// stream as dead (console powered off, network cut, etc.) and
		// kicking the user back to the devices list — same signal as the
		// "isStalling" spinner above, just a longer fuse so a brief hiccup
		// doesn't bounce the user out of a session that's about to recover.
		const DISCONNECT_STALL_THRESHOLD_MS = 8000;
		const POSITION_EPSILON = 0.03;

		lastVideoActivityRef.current = Date.now();
		lastDecodedFrameCountRef.current = null;
		lastPlaybackPositionRef.current = null;

		const getDecodedFrameCount = (video) => {
			try {
				if (typeof video.getVideoPlaybackQuality === "function") {
					const quality = video.getVideoPlaybackQuality();
					if (
						quality &&
						typeof quality.totalVideoFrames === "number" &&
						quality.totalVideoFrames >= 0
					) {
						return quality.totalVideoFrames;
					}
					if (quality && typeof quality.presentedFrames === "number") {
						return quality.presentedFrames;
					}
				}
			} catch (error) {
				debugTrace("⚠️ Failed to read video playback quality:", error);
			}

			const videoAny = video;
			if (typeof videoAny?.webkitDecodedFrameCount === "number") {
				return videoAny.webkitDecodedFrameCount;
			}
			if (typeof videoAny?.mozParsedFrames === "number") {
				return videoAny.mozParsedFrames;
			}

			return null;
		};

		const checkStall = () => {
			const video = videoRef.current;
			if (!video || !isConnectedRef.current) {
				setIsStalling(false);
				return;
			}

			const now = Date.now();
			const noMedia = video.paused || video.readyState < 2 || !video.srcObject;
			const hasSeenFrames =
				lastDecodedFrameCountRef.current !== null ||
				lastPlaybackPositionRef.current !== null;
			// A paused/emptied element is only forgiven while the stream is
			// still booting (no frame seen yet). Once frames have flowed, it IS
			// the "console off" signal — resetting the clock here would leave a
			// permanent black screen (a dead <video> usually ends up paused
			// with readyState < 2) and the watchdog would never fire.
			if (noMedia && !hasSeenFrames) {
				lastVideoActivityRef.current = now;
				setIsStalling(false);
				return;
			}

			if (!noMedia) {
				const decodedFrames = getDecodedFrameCount(video);
				if (decodedFrames !== null) {
					if (
						lastDecodedFrameCountRef.current === null ||
						decodedFrames > lastDecodedFrameCountRef.current
					) {
						lastDecodedFrameCountRef.current = decodedFrames;
						lastVideoActivityRef.current = now;
						setIsStalling(false);
						return;
					}
				} else {
					const currentPosition = video.currentTime;
					if (
						lastPlaybackPositionRef.current === null ||
						Math.abs(currentPosition - lastPlaybackPositionRef.current) >
						POSITION_EPSILON
					) {
						lastPlaybackPositionRef.current = currentPosition;
						lastVideoActivityRef.current = now;
						setIsStalling(false);
						return;
					}
				}
			}

			const inactivity = now - lastVideoActivityRef.current;
			if (inactivity < STALL_THRESHOLD_MS) {
				setIsStalling(false);
				return;
			}

			// Stalling detected
			setIsStalling(true);
			void handleStreamHealthCheck("monitor-stall", { forceNeutral: true });

			// No video data at all for a sustained period — most likely the
			// console was turned off or the network dropped. The WebRTC
			// peer connection to OUR OWN backend can stay "connected" the
			// whole time (it never talks to the console directly), so
			// connectionstatechange/iceconnectionstatechange never fire for
			// this case — this frame-activity check is the only signal.
			if (
				inactivity >= DISCONNECT_STALL_THRESHOLD_MS &&
				onConnectionErrorRef.current &&
				!hasTriggeredErrorCallbackRef.current
			) {
				hasTriggeredErrorCallbackRef.current = true;
				onConnectionErrorRef.current(
					t(
						"streaming.connection.errors.noVideoData",
						"No video signal received — the console may be off or the connection was lost",
					),
				);
			}
		};

		keyframeMonitorIntervalRef.current = window.setInterval(checkStall, 1000);

		return () => {
			if (keyframeMonitorIntervalRef.current !== null) {
				window.clearInterval(keyframeMonitorIntervalRef.current);
				keyframeMonitorIntervalRef.current = null;
			}
		};
	}, [
		handleStreamHealthCheck,
		isConnected,
		resolveWebrtcSessionId,
		t,
		videoRef,
	]);

	// Always-on network-loss sampling for the PS5's native bitrate
	// adaptation (see streaming-hub.service.js reportNetworkLoss /
	// internal/api/signalr_hub.go ReportNetworkStats) — deliberately NOT
	// gated by isStatsEnabled like collectConnectionStats above: adapting the
	// stream to real network conditions should always be active, not only
	// while the debug panel happens to be open.
	useEffect(() => {
		if (!isConnected) {
			lastNetworkLossSampleRef.current = null;
			return undefined;
		}

		const sample = async () => {
			const statsReport = await screenRef.current?.getRawStats();
			if (!statsReport) {
				return;
			}
			try {
				let packetsLost = null;
				let packetsReceived = null;
				statsReport.forEach((report) => {
					if (
						report.type === "inbound-rtp" &&
						!report.isRemote &&
						report.kind === "video"
					) {
						if (typeof report.packetsLost === "number")
							packetsLost = report.packetsLost;
						if (typeof report.packetsReceived === "number")
							packetsReceived = report.packetsReceived;
					}
				});
				if (packetsLost === null || packetsReceived === null) {
					return;
				}
				const previous = lastNetworkLossSampleRef.current;
				lastNetworkLossSampleRef.current = { packetsLost, packetsReceived };
				if (!previous) {
					return;
				}
				const deltaLost = Math.max(0, packetsLost - previous.packetsLost);
				const deltaReceived = Math.max(
					0,
					packetsReceived - previous.packetsReceived,
				);
				const total = deltaLost + deltaReceived;
				if (total <= 0) {
					return;
				}
				streamingHubService
					.reportNetworkLoss(deltaLost / total)
					.catch(() => { });
			} catch {
				// best-effort, ignore
			}
		};

		void sample();
		const intervalId = window.setInterval(sample, 2000);
		return () => {
			window.clearInterval(intervalId);
		};
	}, [isConnected]);

	// handleStreamHealthCheck is recreated on many renders (it closes over
	// several other callbacks) — a ref lets the interval below depend only
	// on isConnected/isStatsEnabled, so it doesn't get torn down and
	// recreated (with an immediate extra call) on every unrelated render.
	const handleStreamHealthCheckRef = useRef(handleStreamHealthCheck);
	useEffect(() => {
		handleStreamHealthCheckRef.current = handleStreamHealthCheck;
	}, [handleStreamHealthCheck]);

	// The Live Monitoring panel (heartbeat/DATA_ACK/GMAC state) re-reads the
	// last snapshot the child PUSHED — a local read, no request leaves the
	// page. Independent of the stall-detection health check, which only fires
	// once video is already stuck. Tied to isStatsEnabled since it's only
	// useful while the panel is visible.
	useEffect(() => {
		if (!isStatsEnabled || !isConnected) {
			return undefined;
		}

		const intervalId = window.setInterval(() => {
			void handleStreamHealthCheckRef.current("live-monitor", {});
		}, HEALTH_CHECK_COOLDOWN_MS);

		return () => {
			window.clearInterval(intervalId);
		};
	}, [HEALTH_CHECK_COOLDOWN_MS, isConnected, isStatsEnabled]);

	// The initial IDR request, retried until it can actually go out.
	//
	// The PS5 only emits a keyframe on demand, so the very first connection
	// shows NOTHING until one is asked for (a reconnect works because the
	// gateway still has a cached keyframe — exactly the "first attempt black,
	// second attempt fine" reported live on 2026-08-24). The request used to
	// ride the PeerConnection's "connected" transition, which was safe while
	// this hook created the viewer itself. It no longer is: the PAGE attaches
	// the video first and the platform connection only starts afterwards
	// (Streaming.jsx gates it on the viewer being ready), so that request now
	// runs while webrtcSessionId is still null and the hub socket is still
	// closed — requestKeyframe drops it and nothing ever asks again.
	useEffect(() => {
		if (!isConnected || !webrtcSessionId) {
			return undefined;
		}

		if (!hasVideoTrackRef.current || initialKeyframeRequestedRef.current) {
			return undefined;
		}

		let attemptsLeft = 10;
		let timer = null;

		const askForKeyframe = () => {
			if (initialKeyframeRequestedRef.current) {
				return;
			}

			if (requestKeyframe("post-connect-initial-video")) {
				initialKeyframeRequestedRef.current = true;
				return;
			}

			attemptsLeft -= 1;

			if (attemptsLeft > 0) {
				timer = setTimeout(askForKeyframe, 1000);
			}
		};
		askForKeyframe();

		return () => clearTimeout(timer);
	}, [isConnected, webrtcSessionId, requestKeyframe]);

	const disconnectRef = useRef(disconnect);
	useEffect(() => {
		disconnectRef.current = disconnect;
	}, [disconnect]);

	useEffect(() => {
		hasAttemptedInitialConnectRef.current = false;
	}, [hostId, attachSessionId]);

	// LA VIDÉO QUI ARRIVE NE DIT PAS QUE LA CONSOLE A ÉTÉ PRIÉE D'ÉMETTRE.
	//
	// Ce garde-fou testait `!isConnected` — l'état du VIEWER de la passerelle,
	// que le composant vidéo partagé monte de son côté. Deux choses
	// différentes : le viewer peut être debout alors que personne n'a encore
	// demandé à la PS5 de streamer (c'est `connect()`, juste en dessous, qui
	// le fait via `createOffer` puis `connectToRemotePlaySession`).
	//
	// Sur un serveur la passerelle est un service distant : l'ICE prend le
	// temps d'un aller-retour réseau, l'effet s'évalue avant, et le hasard
	// jouait en notre faveur. SUR LE TÉLÉPHONE elle est en processus — l'ICE
	// se termine en cent cinquante millisecondes — donc `isConnected` était
	// déjà vrai au premier passage, et le flux n'était JAMAIS lancé : viewer
	// connecté, aucune image, « Video signal interrupted » par-dessus un
	// écran noir (2026-09-10, journal du téléphone à l'appui).
	//
	// La bonne question est « le flux est-il lié ? », et `isStreamBoundRef`
	// y répond déjà — il passe à vrai juste après la liaison réussie. La
	// répétition, elle, est retenue par `hasAttemptedInitialConnectRef`, qui
	// est posé AVANT la temporisation.
	// UN RE-RENDU NE DOIT PAS DÉFAIRE L'INTENTION DE SE CONNECTER.
	//
	// Cet effet posait son drapeau « j'ai tenté » AVANT la temporisation, et
	// rendait un nettoyage qui ANNULE le minuteur. Or `connect` est dans ses
	// dépendances et change d'identité à chaque rendu : le moindre re-rendu
	// dans les cinq cents millisecondes annulait le minuteur, et le drapeau
	// interdisait alors de le replanifier. `connect()` ne partait jamais.
	//
	// Sur un serveur la page se stabilise avant l'échéance et le hasard
	// jouait en notre faveur. Sur le téléphone la passerelle est en processus
	// — ICE bouclé en cent cinquante millisecondes, donc mises à jour d'état
	// et re-rendus dans cette même fenêtre : l'annulation était systématique
	// (2026-09-10, quatrième cause du même écran noir, les trois précédentes
	// étant des booléens confondus).
	//
	// Le minuteur vit donc dans un ref, et seul le DÉMONTAGE l'annule.
	useEffect(() => {
		if (
			!(hostId || attachSessionId) ||
			isStreamBoundRef.current ||
			hasAttemptedInitialConnectRef.current
		) {
			return;
		}
		hasAttemptedInitialConnectRef.current = true;
		initialConnectTimerRef.current = setTimeout(() => {
			initialConnectTimerRef.current = null;
			connect();
		}, 500);
	}, [connect, hostId, attachSessionId]);

	// Le seul endroit qui a le droit d'annuler : la page qui s'en va.
	useEffect(() => () => {
		if (initialConnectTimerRef.current) {
			clearTimeout(initialConnectTimerRef.current);
			initialConnectTimerRef.current = null;
		}
	}, []);

	useEffect(() => {
		return () => {
			disconnectRef.current(false);
		};
	}, []);

	// Control handoff (B2): the kernel is the authority — the page shows
	// the prompt when the pool snapshot carries session.controlRequest, and
	// flips gaveAwayControl after accepting. The hub event wiring that used
	// to live here is gone: granted/denied/requested now travel as snapshot
	// diffs (padSlots / controlRequest), and requesting control back is
	// done from the Watch page like any viewer.
	useEffect(() => {
		if (gaveAwayControl) {
			hasControlRef.current = false;
			setHasControl(false);
		}
	}, [gaveAwayControl]);

	return {
		hasControl,
		isConnected,
		isConnecting,
		connectionState,
		connect,
		disconnect,
		connectionStats,
		isStatsMonitoringEnabled: isStatsEnabled,
		setStatsMonitoring: setSharedStatsMonitoring,
		refreshStream,
		forceResetReorderQueue,
		webrtcSessionId,
		stopStickProcessing,
		startStickProcessing,
		isStalling,
		// Wired onto <GatewayWebRTC> by Player.jsx (ref={screenRef},
		// same pattern retro/streamex's Player.jsx already use, plus the
		// extra props this platform needs — see each callback's own doc).
		handleConnectionStateChange,
		handleGatewayMessage,
		handleIceConnectionStateChange,
		onFirstVideoTrack,
	};
}
