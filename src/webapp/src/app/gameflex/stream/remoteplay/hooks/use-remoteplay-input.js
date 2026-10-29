import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSessionStats } from "../../shared/hooks/use-session-stats.js";
import { resolveControllerLayout } from "../../shared/controller-layout.js";
import { remoteplayInputService as controllerService } from "../service/input.service.js";
import { useStreamingConnection } from "./use-streaming-connection.js";
import { setVirtualStick, setVirtualStickActive } from "./use-streaming-connection/virtual-joystick-state.js";
import { useAuth } from "../../../profile/auth/AuthContext.jsx";
import {
	getKeyboardDeviceId,
	getCachedKeyMapping,
	loadKeyMapping,
	subscribeKeyMapping,
} from "../../../libs/keyMapping.js";
import { saveControllerMappings, getDefaultControllerMappings } from "../../../../types/controller-mapping.js";
import { markActivity } from "../../shared/activity-store.js";
// v1 (chantier-3): the client-side input priority gate is gone. It existed
// because every tab of a player kept its role, so the page had to decide
// locally which one was driving. Input is now gated where it belongs —
// server-side, on who holds the pad — and a pad is taken by an explicit
// POST, never inferred here.

// See stream/remoteplay/Player.jsx's own former doc — remote-play's
// keyboard input already has its own proven, storage-event-driven local
// mapping (types/controller-mapping.js); this projects the user's
// KeyMapping into that same local storage shape once resolved, so the
// EXISTING keyboard handler picks it up unchanged.
function useRemotePlayKeyMapping(token) {
	const keyboardId = getKeyboardDeviceId();

	useEffect(() => {
		let cancelled = false;
		const project = (mapping) => {
			const next = getDefaultControllerMappings();
			for (const [code, buttonName] of Object.entries(mapping)) {
				if (next.buttons[buttonName]) {
					next.buttons[buttonName] = { button: buttonName, key: code };
				}
			}
			saveControllerMappings(next);
		};

		project(getCachedKeyMapping(keyboardId, "dualsense"));
		loadKeyMapping(token, keyboardId, "dualsense").then((m) => {
			if (!cancelled) {
				project(m);
			}
		});
		const unsubscribe = subscribeKeyMapping(keyboardId, "dualsense", project);
		return () => {
			cancelled = true;
			unsubscribe();
		};
	}, [keyboardId, token]);
}

function detectIsLikelyLan() {
	if (typeof window === "undefined") {
		return false;
	}
	const hostname = (window.location.hostname || "").toLowerCase();
	if (!hostname) {
		return false;
	}
	if (hostname === "localhost" || hostname === "127.0.0.1") {
		return true;
	}
	if (hostname.includes(":")) {
		return hostname.startsWith("fe80") || hostname.startsWith("fd") || hostname.startsWith("fc");
	}
	if (hostname.startsWith("10.") || hostname.startsWith("192.168.") || hostname.startsWith("169.254.")) {
		return true;
	}
	if (hostname.startsWith("172.")) {
		const second = Number(hostname.split(".")[1]);
		if (!Number.isNaN(second) && second >= 16 && second <= 31) {
			return true;
		}
	}
	return hostname.endsWith(".local");
}

// useRemotePlayInput — the remote-play half of the unified Streaming page's
// per-platform dispatcher (see stream/shared/hooks/use-streaming-input.js).
// Same pattern as use-retrogaming-input.js.
// screenRef — the PAGE's one gateway viewer (Streaming.jsx owns it, see
// SessionStream's doc). Remote-play no longer creates one of its own either:
// the belief that it could not (that it "only learns its gateway sessionId
// mid-flow") turned out to be false for this page — preCreatedSessionId is
// always session.id here and attachSessionId is never passed, so connect()
// was creating a second viewer against the very same id the page already
// held.
/**
 * LA MÊME SIGNATURE POUR LES TROIS PLATEFORMES.
 *
 *     useXInput({ screenRef, session, context, t, options })
 *
 * Les quatre premiers sont ce dont TOUTE plateforme a besoin : la surface
 * vidéo, la session, le contexte Skeleton, la traduction. Ce qui n'appartient
 * qu'à l'une descend dans `options`, jamais à plat à côté d'eux.
 *
 * Pourquoi cette règle : à plat, chaque hook affichait sa propre liste — neuf
 * paramètres pour le rétro, six pour streamex, quatre pour remote-play — et
 * l'appelant devait savoir laquelle avant d'appeler. Le dispatcher portait
 * donc trois lignes différentes pour trois hooks censés être
 * interchangeables, et ajouter un besoin à l'un obligeait à relire les trois.
 * Avec `options`, la ligne d'appel est la même partout et seul son contenu
 * change — ce qui est exactement ce qu'on veut dire.
 *
 * Cette règle vaut pour tout hook ou toute lib PARTAGÉE entre plateformes,
 * pas seulement pour ces trois-là.
 */
export function useRemotePlayInput({ screenRef, session, context, t, options = {} }) {
	// Aucune option propre à remote-play aujourd'hui. Le paramètre est là
	// quand même : c'est la signature qui est commune, pas son contenu.
	void options;
	// Read-only proxy onto GatewayWebRTC's own <video> element (see
	// useStreamingConnection's own param doc — it still expects this,
	// pre-dating the screenRef/GatewayWebRTC migration) — same pattern as
	// Streaming.jsx's own pipVideoRef. Never passing this crashed the
	// moment a track actually arrived: onFirstVideoTrack (wired to
	// GatewayWebRTC's ontrack) does `videoRef.current` unconditionally,
	// and `videoRef` itself was undefined, not just `.current` — confirmed
	// live (2026-08-22) via "Cannot read properties of undefined (reading
	// 'current')" at the exact point video started flowing.
	const videoRef = useRef({
		get current() {
			return screenRef.current?.videoElement ?? null;
		},
	}).current;
	const { token, user } = useAuth();
	const [gaveAwayControl, setGaveAwayControl] = useState(false);
	const isLikelyLan = useMemo(detectIsLikelyLan, []);

	// Stable identity (useCallback, not a plain arrow function recreated
	// every render) — connect() below has `onConnectionError` in its own
	// useCallback deps, and onConnectionError closes over this. An
	// unstable toast meant onConnectionError, then connect() itself, got a
	// NEW identity every single render — which fed straight into the
	// auto-connect effect's OWN deps ([connect, hostId, ...]), so it kept
	// clearing and rescheduling its 500ms timer on every re-render instead
	// of ever letting it fire. Confirmed live (2026-08-22): the page never
	// even reached "Connecting...", straight to stuck "Disconnected".
	const toast = useCallback(
		({ title, description, variant }) => {
			context.toast.open(description ? `${title} — ${description}` : title, {
				variant: variant === "destructive" ? "error" : "normal",
			});
		},
		[context],
	);

	const handleConnectionError = useCallback(
		(reason) => {
			toast({ title: t("streaming.connection.state.failed"), description: reason, variant: "destructive" });
		},
		[toast, t],
	);

	const {
		isConnected,
		isConnecting,
		isStalling,
		connectionState,
		connectionStats,
		hasControl,
		disconnect,
		setStatsMonitoring,
		handleConnectionStateChange,
		handleGatewayMessage,
		handleIceConnectionStateChange,
		onFirstVideoTrack,
	} = useStreamingConnection({
		hostId: session.reference,
		isLikelyLan,
		videoRef,
		screenRef,
		username: user?.username,
		// preCreatedSessionId (idempotent reuse of an already-started
		// session) only — NOT attachSessionId. attachSessionId makes
		// connect() skip straight to connectAttached(), the control-handoff
		// "attach as a promoted viewer" path (viewerService.createOffer,
		// see that function's own doc): it never wakes/handshakes with the
		// PS5 at all, it just tries to attach to a stream that's supposed
		// to already be live. Setting it unconditionally to session.id
		// meant EVERY connection — including the very first, real owner
		// connection that's supposed to DO the PS5 handshake — took this
		// dead-end path instead, so video never arrived (confirmed live,
		// 2026-08-22: stuck on "Disconnected" forever). There is no live
		// "take control" signal in the current architecture that should
		// set this yet (see extraProps.handover's own doc, use-remoteplay-
		// input.js) — leave unset until that flow actually exists.
		preCreatedSessionId: session.id,
		gaveAwayControl,
		onConnectionError: handleConnectionError,
	});

	useRemotePlayKeyMapping(token);

	const REMOTE_DIAGONALS = {
		DPAD_UP_LEFT: ["DPAD_UP", "DPAD_LEFT"],
		DPAD_UP_RIGHT: ["DPAD_UP", "DPAD_RIGHT"],
		DPAD_DOWN_LEFT: ["DPAD_DOWN", "DPAD_LEFT"],
		DPAD_DOWN_RIGHT: ["DPAD_DOWN", "DPAD_RIGHT"],
	};
	const handleRemoteButtonDown = (name) => {
		if (!hasControl || name === "LEFT_STICK" || name === "RIGHT_STICK") {
			return;
		}
		// P6 — real input stamps the activity store (drained by the shell).
		markActivity();
		for (const direction of REMOTE_DIAGONALS[name] || [name]) {
			controllerService.sendButton(direction, "press").catch(() => { });
		}
	};
	// No priority guard here on purpose: a RELEASE always goes through, even
	// from a tab that just lost the priority mid-press — dropping it would
	// leave the button stuck down on the console, and a stray release is
	// harmless.
	const handleRemoteButtonUp = (name) => {
		if (!hasControl || name === "LEFT_STICK" || name === "RIGHT_STICK") {
			return;
		}
		markActivity();
		for (const direction of REMOTE_DIAGONALS[name] || [name]) {
			controllerService.sendButton(direction, "release").catch(() => { });
		}
	};
	const handleRemoteAnalogMove = (name, x, y) => {
		if (!hasControl) {
			return;
		}
		markActivity();
		const side = name === "LEFT_STICK" ? "left" : name === "RIGHT_STICK" ? "right" : null;
		if (!side) {
			return;
		}
		setVirtualStickActive(side, x !== 0 || y !== 0);
		setVirtualStick(side, x, y);
	};
	// Le doigt sur le pavé tactile. Le clic, lui, passe par
	// handleRemoteButtonDown/Up (TOUCHPAD) comme avant : la surface
	// l'appelle au bon moment du geste.
	const handleRemoteTouch = (finger, down, x, y) => {
		if (!hasControl) {
			return;
		}
		markActivity();
		controllerService.sendTouch(finger, down, x, y).catch(() => { });
	};

	// P2 §1 — remote-play's transport lives in input.service.js, fully
	// independent of the façade WS (the WS only carries pushes) — all
	// three lifecycle callbacks are no-ops by design.
	const onConnect = useCallback(() => { }, []);
	const onLeave = useCallback(() => { }, []);
	const onDisconnect = useCallback(() => { }, []);

	// P5 — contract actions the shell triggers; the request messages ride
	// the child, as intents (sessionSend), so the hook only carries the platform-
	// side follow-ups. No swap (single pad, the owner IS the only slot)
	// and no free-slot join.
	const onSlotGiveaway = useCallback(() => {
		// THIS holder accepted a cede: the kernel already swapped the pool
		// and pushed the new holder to the child (input gate) — this page
		// just stops its own input attempts.
		setGaveAwayControl(true);
	}, []);
	const hasFreeSlots = useCallback(() => 0, []);
	const onMonitoringResource = useCallback((open, powerOnly = false) => {
		const { serverStatus, devicePower } = useSessionStats({
			platform: "remoteplay",
			sessionId: session.id,
			startedAt: session.startedAt,
			// Même oubli que streamex : sans jeton la route répond 401, et le
			// panneau garde ses tirets sans qu'on sache pourquoi.
			token,
			setStatsMonitoring,
			intervalMs: 5000,
			open,
			powerOnly,
		});
		// `.data` — le kernel répond `{success, data}`. Voir use-streamex-input,
		// où le même oubli donnait un tiret sur chaque ligne mesurée.
		// `device` — le téléphone qu'on tient, quand la partie tourne
		// ailleurs (voir useSessionStats).
		return { ...connectionStats, ...serverStatus?.data, device: devicePower };
	}, [session.id, session.startedAt, setStatsMonitoring, t, connectionStats]);

	return {
		// No `connect` exposed here — unlike retro-gaming, this
		// connection hook already auto-connects itself (its own
		// useEffect on hostId/attachSessionId/preCreatedSessionId, see
		// use-streaming-connection.js). Streaming.jsx calling connect()
		// again on top of that would race the hook's own attempt.
		// Three distinct verbs, no boolean parameters (2026-08-24). The old
		// disconnect(bool) meant `saveProgress` here and `explicit` on
		// remote-play — the SAME positional argument with opposite meanings,
		// which is how "quit without ending the session" ended it anyway.
		//   disconnect()  — leave quietly, the session survives on its grace
		//   terminate()   — deliberately end the session
		//   saveOnQuit()  — retro only: end it, saving progress first (the
		//                   save rides the "exit" message, it has no path of
		//                   its own; the menu's per-slot saveState is a
		//                   different feature)
		// remote-play has no save states, so saveOnQuit stays absent.
		disconnect: () => disconnect(false),
		terminate: () => disconnect(true),
		// leavePage — the page is navigating away, NOT quitting (roadmap P2):
		// there is nothing to pause on a PS5, and the connection hook's own
		// unmount effect already drops the transport — calling disconnect()
		// here too would tear down twice for nothing. The session survives
		// on its grace for the next device.
		onConnect,
		onLeave,
		onDisconnect,
		onSlotGiveaway,
		hasFreeSlots,
		onMonitoringResource,
		title: session.title,
		// The browser-to-gateway leg is UNIVERSAL: the gateway serves the same
		// fan-out contract to all three platforms, so nothing about talking to
		// it should be platform-specific. What IS specific to remote-play is
		// the buffer-service-to-console leg (Takion), which lives in
		// the buffer service, not here. Three former "PS5 settings" were
		// removed on that basis (2026-08-24):
		//   - combinedAudioVideo: put audio and video on ONE element, which
		//     makes Chrome slave the video to the audio clock (~1s of
		//     perceived input lag — see GatewayWebRTC's own note). A pre-
		//     gateway habit, and the worst possible trade for a game stream.
		//   - forceTurnRelay: confirmed unnecessary — all three platforms
		//     connect remotely with or without TURN.
		//   - rtcConfig: max-bundle and rtcp-mux are Chrome's own defaults.
		// The low-latency answer-SDP rewrite is not remote-play-specific
		// either; it now applies to every platform inside GatewayWebRTC.
		//
		// What remains here still reaches AROUND the gateway to speak to the
		// buffer service (keyframe request on first track, SignalR-driven ICE
		// restart, keepalive ping/pong) — pre-gateway leftovers that belong on
		// the buffer-to-gateway leg. Removing them needs a change in the
		// buffer service; tracked in chantier 2 (C2-P4c).
		gatewayProps: {
			// UN SEUL ÉLÉMENT : la console n'a qu'une source.
			//
			// La PS5 envoie l'image et le son ensemble et n'initialise son
			// flux qu'UNE fois. Les répartir sur deux éléments condamne la
			// vidéo dès que le son arrive le premier — elle n'a pas de
			// seconde chance. Le tampon entre la console et la passerelle
			// devait faire ce découpage ; tant qu'il ne le fait pas, c'est
			// ici que la contrainte se traduit.
			//
			// Retiré le 2026-08-24 pour économiser la seconde de latence
			// d'entrée que Chrome ajoute en asservissant la vidéo à
			// l'horloge audio. L'économie n'existait pas : elle se payait en
			// image absente sur navigateur mobile (2026-09-07).
			combinedAudioVideo: true,
			onConnectionStateChange: handleConnectionStateChange,
			onFirstVideoTrack,
			onIceConnectionStateChange: handleIceConnectionStateChange,
			onMessage: handleGatewayMessage,
		},
		isConnected,
		isConnecting,
		isStalling,
		connectionState,
		controller: () => {
			const { Component: ControllerComponent, label } = resolveControllerLayout({ session });

			return {
				label,
				Component: (
					<ControllerComponent
						onButtonDown={handleRemoteButtonDown}
						onButtonUp={handleRemoteButtonUp}
						onAnalogMove={handleRemoteAnalogMove}
						onTouch={handleRemoteTouch}
					/>
				),
			};
		},
		extraProps: null,
	};
}
