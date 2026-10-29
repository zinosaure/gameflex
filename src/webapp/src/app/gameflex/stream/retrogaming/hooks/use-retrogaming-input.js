import React, { useCallback, useEffect, useRef, useState } from "react";
import { RETRO_BUTTON_INDEX, RETRO_DIAGONAL_BUTTONS, RETRO_CONTROLLER_PRESET_MAP } from "../../../controllers/Controllers.jsx";
import { resolveControllerLayout } from "../../shared/controller-layout.js";
import { useConsoles } from "../../../contexts/ConsolesContext.jsx";
import { useRetroPhysicalGamepadInput } from "./use-retrogaming-physical-gamepad.js";
import { useRetroKeyboardInput } from "./use-retrogaming-keyboard-input.js";
import { useRetrogamingStreamingConnection } from "./use-retrogaming-streaming-connection.js";
import { useAuth } from "../../../profile/auth/AuthContext.jsx";
import { triggerRumble } from "../../../libs/rumble.js";
import { CONTROLLER_PRESETS } from "../../../controllers/presets.js";
import { useSessionStats } from "../../shared/hooks/use-session-stats.js";
import { markActivity } from "../../shared/activity-store.js";
// v1 (chantier-3): the client-side input priority gate is gone. It existed
// because every tab of a player kept its role, so the page had to decide
// locally which one was driving. Input is now gated where it belongs —
// server-side, on who holds the pad — and a pad is taken by an explicit
// POST, never inferred here.

// useRetrogamingInput — the retro-gaming half of the unified Streaming
// page's per-platform dispatcher (see stream/shared/hooks/use-session-
// input.js). Returns the exact StreamingShell prop set retro-gaming's own
// (now-deleted) Player.jsx used to build and render directly — the shared
// Streaming.jsx component just spreads this straight onto <StreamingShell>.
// Every genuinely retro-only feature (save states, cheats, PS1 dual-
// controller, rewind/fast-forward, N64 C-buttons) lives entirely in here;
// StreamingShell already treats an undefined prop as "no section" (see its
// own doc), so the other two platforms never see any of it.
// screenRef — the PAGE's one gateway viewer (Streaming.jsx owns it, see
// SessionStream's doc): this hook sends input through it and reads its
// viewerId for the start/join/spectate message, but never creates one of
// its own anymore. Creating a second viewer inside the WS onOpen is what
// restarted the video on every role change.
// virtualPad / virtualPadLabel — un pad fourni par l'appelant, qui l'emporte
// sur la console (KeytestLive : le téléphone sert KeyTest comme une session
// « device », donc par CE hook). `resolveControllerLayout` prévoyait déjà le
// cas — seul le chemin pour l'y amener manquait de ce côté.
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
export function useRetrogamingInput({ screenRef, session, context, t, options = {} }) {
	const {
		isWatching,
		childSlot = null,
		onConnectionError,
		// Un pad qui vient de la PAGE et non de la console — KeyTest hébergé
		// par le téléphone, qui n'a pas de manette à déclarer.
		virtualPad = null,
		virtualPadLabel = null,
		// Les jalons que le CONTENEUR annonce pendant qu'il charge (#191) —
		// cœur, ROM, BIOS, jeu, géométrie. Ce hook ne fait que les laisser
		// passer : il n'en décide rien, et l'écran qui les montre est le même
		// que celui des étapes du kernel.
		onProgress = null,
	} = options;
	// This page is player-only now — watching (spectating) went to
	// gameflex/Watch.jsx, which handles all 3 platforms' spectate flows.
	const { token, user } = useAuth();
	const [isFastForwarding, setIsFastForwarding] = useState(false);
	const [isRewinding, setIsRewinding] = useState(false);
	// PS1 controller type — 1, la manette d'origine (numérique) : c'est ce
	// que le moteur monte au démarrage depuis le 2026-10-11, décision du
	// porteur. Le sélecteur part donc d'accord avec la console, sans
	// attendre de réponse. (517, la DualShock, jusqu'à cette date.)
	const [selectedControllerType, setSelectedControllerType] = useState(1);
	const consoles = useConsoles();
	const consoleMeta = consoles.find((c) => c.id === session.consoleId);

	// The connection hook reports player joins/leaves through a {title,
	// variant} toast shape (slot requests ride the façade WS now, P3) —
	// this adapter maps that shape onto Skeleton's toast.open.
	const toast = useCallback(({ title, action, variant }) => {
		context.toast?.open?.(
			action ? (
				<div className="d-flex flex-column gap-2">
					<span>{title}</span>
					{action}
				</div>
			) : title,
			{ variant: variant === "error" ? "error" : "normal", duration: action ? 0 : undefined }
		);
	}, [context]);

	// P2 §1 / P3 — the reconnect role comes from the KERNEL's own push,
	// not from this page's previous state: a spectate→player transition
	// MEANS a slot got granted (freeSlot join or handover) and the platform
	// connection follows with a joiner reconnect. A spectate→owner
	// transition is the watch-first OWNER taking their own pad — that
	// reconnect must send "start" (retro_relay.go's identity check), never
	// "join": an owner "join" on a maxPlayers=1 console (PSP) is rejected
	// by the relay with "session is full" before the child ever sees it,
	// and on any other console it would grab a joiner port instead of
	// starting the session. The mount case never triggers the effect: the
	// ref starts at the current value.
	// chantier-4: the role follows the PAD, and the pad is the queue's answer
	// — me.padSlot, not a name. Slot 1 dials "owner" (the message that boots
	// the emulator), any other slot dials "join". A tab holding nothing
	// spectates.
	//
	// Keyed on the SLOT rather than on me.role because that is what the child
	// gates input on: deriving the two from different fields is how the on-
	// screen pad and the mapping profile ended up disagreeing once already.
	// childSlot wins when present: on a child-managed session it IS the
	// authority's answer, while session.me arrives several hops later. Dialing
	// with the stale one made the page announce itself as a spectator moments
	// after taking the pad — and the child then refused, because nobody was
	// playing yet.
	const slot = childSlot ?? session.me?.padSlot;
	const connectRole = isWatching
		? "spectate"
		: slot === 1
			? "owner"
			: slot > 1
				? "join"
				: session.me?.role === "owner"
					? "owner"
					: session.me?.role === "player"
						? "join"
						: "spectate";
	const previousWatchingRef = useRef(isWatching);
	// Set by onConnect below — the façade gate owns the FIRST connection.
	// Without this, a fresh session opened two relay sockets ~19ms apart:
	// the page starts with isWatching=true, the kernel's first private push
	// flips it to false, and that flip fired the reconnect below at the same
	// moment the gate fired onConnect. The second socket replaced the first,
	// the child saw the owner's socket die, Players emptied, and the role
	// fell back to spectate for good — "retro drops me into watch as soon as
	// I launch" (reported live 2026-08-24). This effect only ever handles a
	// LATER watch→play transition; there is nothing to reconnect before the
	// first connection has been made.
	const hasConnectedRef = useRef(false);

	const {
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
	} = useRetrogamingStreamingConnection({
		screenRef,
		t,
		toast,
		onProgress,
		onRumble: (weak, strong) =>
			triggerRumble(null, weak, strong, {
				virtualControllerRumble: CONTROLLER_PRESETS[RETRO_CONTROLLER_PRESET_MAP[session.consoleId]]?.rumble === true,
			}),
	});

	// P6 — every REAL input send stamps the shared activity store (the
	// shell drains it onto the façade WS at most once a minute). Wrapped
	// ONCE here so the virtual pad, the keyboard hook and the physical-
	// gamepad poller — which all route through sendButton/sendAnalog —
	// stay untouched.
	const sendButtonWithActivity = useCallback(
		(index, value) => {
			// Only the tab holding the priority drives the pad (C2-P4d).
			// Presses only. A RELEASE always goes through, even from a tab
			markActivity();
			sendButton(index, value);
		},
		[sendButton],
	);
	const sendAnalogWithActivity = useCallback(
		(index, x, y) => {
			markActivity();
			sendAnalog(index, x, y);
		},
		[sendAnalog],
	);

	// DÉPLACÉ SOUS LA DÉCLARATION DE `connect`.
	//
	// Cet effet le lisait dans son tableau de dépendances une dizaine de
	// lignes AVANT que la destructuration ne le définisse — une lecture en
	// zone morte temporelle, que le linter signale (no-use-before-define).
	// Elle ne lève pas aujourd'hui par accident : Babel transpile ce `const`
	// en `var`, hissé et valant `undefined`. Le jour où la cible de
	// compilation change, ça devient une ReferenceError à chaque rendu.
	useEffect(() => {
		const wasWatching = previousWatchingRef.current;
		previousWatchingRef.current = isWatching;
		if (wasWatching && !isWatching && hasConnectedRef.current) {
			connect(session.id, connectRole, user?.username, (reason) => {
				context.toast?.open?.(reason || t("streaming.connection.state.failed"), { variant: "error" });
			});
		}
	}, [isWatching, connect, connectRole, session.id, user?.username, context, t]);

	useRetroPhysicalGamepadInput(isConnected, sendButtonWithActivity, sendAnalogWithActivity, session.consoleId, token);
	useRetroKeyboardInput(isConnected, sendButtonWithActivity, session.consoleId, token);

	// N64's C-buttons have no digital RetroPad bit in mupen64plus_next's
	// default mapping — the core expects them as RIGHT ANALOG STICK
	// deflection instead.
	const cStateRef = useRef({ up: false, down: false, left: false, right: false });
	const N64_C_BUTTON_NAMES = {
		C_UP: "up",
		C_DOWN: "down",
		C_LEFT: "left",
		C_RIGHT: "right",
	};
	const sendN64CState = () => {
		const c = cStateRef.current;
		const x = (c.right ? 1 : 0) - (c.left ? 1 : 0);
		const y = (c.down ? 1 : 0) - (c.up ? 1 : 0);
		sendAnalogWithActivity(1, x, y);
	};
	const handleButtonDown = (name) => {
		if (name in RETRO_DIAGONAL_BUTTONS) {
			for (const direction of RETRO_DIAGONAL_BUTTONS[name]) {
				sendButtonWithActivity(RETRO_BUTTON_INDEX[direction], 1);
			}
			return;
		}
		if (name in N64_C_BUTTON_NAMES) {
			cStateRef.current = { ...cStateRef.current, [N64_C_BUTTON_NAMES[name]]: true };
			sendN64CState();
			return;
		}
		sendButtonWithActivity(RETRO_BUTTON_INDEX[name], 1);
	};
	const handleButtonUp = (name) => {
		if (name in RETRO_DIAGONAL_BUTTONS) {
			for (const direction of RETRO_DIAGONAL_BUTTONS[name]) {
				sendButtonWithActivity(RETRO_BUTTON_INDEX[direction], 0);
			}
			return;
		}
		if (name in N64_C_BUTTON_NAMES) {
			cStateRef.current = { ...cStateRef.current, [N64_C_BUTTON_NAMES[name]]: false };
			sendN64CState();
			return;
		}
		sendButtonWithActivity(RETRO_BUTTON_INDEX[name], 0);
	};
	const dpadStateRef = useRef({ up: false, down: false, left: false, right: false });
	// LA ZONE MORTE SE MESURE SUR L'AMPLITUDE, PAS SUR CHAQUE AXE.
	//
	// Le stick borne sa course en cercle (Controllers.jsx : `mag > 1` divise
	// x et y), mais le seuil était appliqué aux deux axes séparément — un
	// test carré sur une entrée ronde. Une diagonale répartit la poussée :
	// à 45° il fallait 0,566 d'amplitude (0,4 ÷ cos 45°) pour franchir 0,4
	// sur les DEUX axes, contre 0,4 pour un haut ou un droite. Et dans la
	// bande intermédiaire, RIEN ne partait — le pouce poussé plus loin que
	// le seuil d'une direction franche, et la croix qui lisait le neutre.
	// C'était ça, « les diagonales ne prennent pas » (2026-09-17).
	//
	// L'amplitude décide donc SI la croix bouge, l'angle décide ensuite de
	// quelles directions : huit secteurs de 45°, ce qui met une diagonale à
	// la même poussée qu'une direction franche.
	const STICK_DPAD_DEADZONE = 0.4;
	const stickToDpad = (x, y) => {
		if (Math.hypot(x, y) < STICK_DPAD_DEADZONE) {
			return { up: false, down: false, left: false, right: false };
		}
		// Secteur de 45° : 0 = droite, puis dans le sens des aiguilles (y
		// croît vers le bas, comme à l'écran). Un secteur pair est une
		// direction franche, un impair les deux qui l'encadrent.
		const octant = (Math.round((Math.atan2(y, x) * 4) / Math.PI) + 8) % 8;
		return {
			up: octant >= 5 && octant <= 7,
			down: octant >= 1 && octant <= 3,
			left: octant >= 3 && octant <= 5,
			right: octant === 0 || octant === 1 || octant === 7,
		};
	};
	// LE STICK À L'ÉCRAN EXISTE PARTOUT ; CE QU'IL ENVOIE DÉPEND DE LA CONSOLE.
	//
	// Sur une NES il devient des appuis de croix — un stick est plus
	// confortable qu'une croix tactile, et c'est tout l'intérêt de le
	// dessiner là. Sur une PS2 il doit envoyer la déflexion telle quelle,
	// sinon on joue au tout-ou-rien.
	//
	// La liste des consoles analogiques était écrite ICI, en dur, et elle
	// n'en connaissait que deux : psp et ps1. PS2, N64, GameCube, Wii et
	// Dreamcast — qui ont toutes un vrai stick — recevaient donc des appuis
	// de croix (2026-09-08). C'est un fait de console : il vit maintenant
	// dans le registre des consoles (`analog`), avec les extensions et le
	// nombre de manettes.
	const handleAnalogMove = (name, x, y) => {
		if (session.consoleId === "psp") {
			// La PSP n'a QU'UN nub : ses deux sticks à l'écran pointent donc
			// le même axe, plutôt qu'un second que la console n'a pas.
			sendAnalogWithActivity(0, x, y);
			return;
		}
		if (consoleMeta?.analog || consoleMeta?.method === "streamex") {
			sendAnalogWithActivity(name === "RIGHT_STICK" ? 1 : 0, x, y);
			return;
		}
		const next = stickToDpad(x, y);
		const prev = dpadStateRef.current;
		for (const dir of ["up", "down", "left", "right"]) {
			if (next[dir] === prev[dir]) {
				continue;
			}
			sendButtonWithActivity(RETRO_BUTTON_INDEX[`DPAD_${dir.toUpperCase()}`], next[dir] ? 1 : 0);
		}
		dpadStateRef.current = next;
	};

	// P2 §1 — the façade WS drives the platform connection: onConnect at
	// WS open, onLeave/onDisconnect at error/close. connectRole (defined
	// above, shared with the spectate→player/owner reconnect effect) rides
	// the kernel-pushed role — a reconnecting PLAYER must send "join",
	// never "owner" (retro_relay.go's identity check rejects it).
	const onConnect = useCallback(() => {
		hasConnectedRef.current = true;
		connect(session.id, connectRole, user?.username, (reason) => {
			onConnectionError?.(reason || t("streaming.connection.state.failed"));
		});
	}, [connect, session.id, connectRole, user?.username, onConnectionError, t]);
	// leavePage (pause, grace) — a façade WS drop must NEVER end the game
	// for the other players (roadmap decision: leavePage ≠ disconnect).
	const onLeave = useCallback(() => {
		leavePage();
	}, [leavePage]);
	const onDisconnect = useCallback(() => {
		leavePage();
	}, [leavePage]);

	// P3 — contract actions the shell triggers; the request MESSAGES
	// themselves ride the child, as intents (sessionSend), so the hook only carries
	// the platform-side follow-ups.
	const onSlotGiveaway = useCallback(() => {
		// THIS holder accepted a cede: the kernel already swapped the pool,
		// but the child still serves this connection as owner — reconnect
		// as a plain spectator (delayed so the cede flushes first).
		setTimeout(() => {
			connect(session.id, "spectate", user?.username, () => { });
		}, 1500);
	}, [connect, session.id, user?.username]);
	const hasFreeSlots = useCallback(() => session.openSlots ?? 0, [session.openSlots]);
	const onMonitoringResource = useCallback((open, powerOnly = false) => {
		const { serverStatus, devicePower } = useSessionStats({
			platform: "retrogaming",
			sessionId: session.id,
			startedAt: session.startedAt,
			token,
			setStatsMonitoring,
			intervalMs: 1000,
			open,
			powerOnly,
		});
		// `device` — le téléphone qu'on tient, quand la partie tourne
		// ailleurs (voir useSessionStats).
		return { ...connectionStats, ...serverStatus?.data, device: devicePower };
	}, [session.id, session.startedAt, token, setStatsMonitoring, connectionStats]);

	return {
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
		disconnect: () => leavePage(),
		terminate: () => disconnect(false),
		// An ordinary save on slot 0, awaited — INDEPENDENT of how the player
		// then leaves (2026-08-24). It used to be a flavour of the exit
		// message, so saving was only possible while ending the session; and
		// the child saved by itself on every teardown, which piled up a full
		// state for games merely opened for ten seconds. Nothing is written
		// now unless the player asks, and asking works whichever way they go.
		saveOnQuit: () => new Promise((resolve) => saveState(0, resolve)),
		onConnect,
		onLeave,
		onDisconnect,
		onSlotGiveaway,
		hasFreeSlots,
		onMonitoringResource,
		// Le nom de la console SI ON LA CONNAÎT, et rien sinon. Le gabarit
		// posait le tiret d'abord : une console absente du registre — ce qui
		// arrive quand la session ne la nomme pas — affichait « Mario —
		// undefined » en titre de partie (vu le 2026-09-08).
		title: consoleMeta?.name ? `${session.title} — ${consoleMeta.name}` : session.title,
		// The video surface belongs to the page now (SessionStream, mounted
		// once per page from the sessionId alone) — no ref, no sessionId
		// here. gatewayProps carries only what THIS platform needs on the
		// <GatewayWebRTC> itself; retro needs nothing beyond the two
		// state callbacks.
		gatewayProps: {
			onConnectionStateChange: handleConnectionStateChange,
			onStatsUpdate: handleStatsUpdate,
		},
		isConnected,
		isConnecting,
		connectionState,
		controller: () => {
			const { Component: ControllerComponent, label: controllerLabel } = resolveControllerLayout({
				session,
				ps1ControllerType: selectedControllerType,
				override: virtualPad,
				overrideLabel: virtualPadLabel,
			});

			return {
				label: controllerLabel,
				Component: (
					<ControllerComponent
						onButtonDown={handleButtonDown}
						onButtonUp={handleButtonUp}
						onAnalogMove={handleAnalogMove}
					/>
				),
			};
		},
		extraProps: {
			// Reduced to the genuinely retro-only sections (P3) — join/
			// takeover/handover ride the façade WS + the contract fields
			// above now.
			timeTravelProps: {
				isFastForwarding,
				isRewinding,
				// DÉCLARÉ par la partie, pas déduit de la console — voir
				// rewindSupported (use-retrogaming-save-states.js). Le menu
				// n'affiche l'interrupteur que si le cœur peut vraiment
				// tenir un historique d'états sans coûter le son.
				rewindSupported,
				fastForwardSupported,
				setTravelMode: (mode) => {
					const wantsRewind = mode === "rewind";
					const wantsFastForward = mode === "fastForward";
					setIsRewinding(wantsRewind);
					setIsFastForwarding(wantsFastForward);
					setRewind(wantsRewind);
					setFastForward(wantsFastForward);
				},
			},
			saves: {
				saveState,
				loadState,
			},
			// LE BOUTON RESET, au propriétaire seul, dans la section Session
			// du menu, à côté des sauvegardes (FloatingPanel.jsx).
			reset: resetConsole,
			cheats: {
				cheatsSupported,
				initialCheats,
				setCheats,
			},
			// LES DISQUES, à demander — jamais à déclarer. Voir queryDisks
			// (use-retrogaming-save-states.js) : zéro disque est la réponse
			// que rendent l'enfant trop ancien, le core sans interface et le
			// jeu en un seul fichier, et le menu n'affiche alors rien.
			disks: {
				list: queryDisks,
				change: changeDisk,
			},
			// L'ÉCRAN TACTILE. La page décide OÙ est le doigt, ce hook
			// seulement comment le dire au core.
			touch: {
				send: sendPointer,
			},
			// La pause DÉLIBÉRÉE, celle qu'on demande en quittant. Voir
			// setPaused (use-retrogaming-save-states.js).
			// UN VERBE SANS PARAMÈTRE (CODE_CONVENTIONS, « Nommer les
			// intentions ») : `pause` était exposé comme `setPaused`, donc la
			// page passait un booléen positionnel à travers la frontière de
			// plateforme — la forme même que la règle condamne, et dans le
			// chemin de sortie où le bug d'origine vivait. Il n'a jamais été
			// appelé qu'avec `true` : geler avant de partir. Le jour où il
			// faudra dégeler, ce sera `resume()`, pas un booléen de retour.
			pause: () => setPaused(true),
			...(session.consoleId === "ps1" ? {
				PS1: {
					standard: {
						id: 1,
						load: () => {
							setControllerType(0, 1);
							setSelectedControllerType(1);
						},
					},
					dualshock: {
						id: 517,
						load: () => {
							setControllerType(0, 517);
							setSelectedControllerType(517);
						},
					},
				},
			} : {}),
		},
	};
}
