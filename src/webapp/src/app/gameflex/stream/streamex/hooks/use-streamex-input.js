import React, { useCallback, useEffect, useRef, useState } from "react";
import { useAuth } from "../../../profile/auth/AuthContext.jsx";
import { useStreamexStreamingConnection } from "./use-streamex-streaming-connection.js";
import { absoluteApiUrl } from "../../../libs/serverBase.js";
import { useSessionStats } from "../../shared/hooks/use-session-stats.js";
import { resolveControllerLayout } from "../../shared/controller-layout.js";
import { markActivity } from "../../shared/activity-store.js";
// v1 (chantier-3): the client-side input priority gate is gone. It existed
// because every tab of a player kept its role, so the page had to decide
// locally which one was driving. Input is now gated where it belongs —
// server-side, on who holds the pad — and a pad is taken by an explicit
// POST, never inferred here.
import { useKeytestPad } from "../KeyTestPad.jsx";
import { CONTROLLER_PRESETS } from "../../../controllers/presets.js";
import { CONSOLE_GAMEPAD_ORDER } from "../controllers/index.js";
import { useStreamexPhysicalInput } from "./use-streamex-physical-input.js";
import { keysOfText } from "../keyboard-text.js";

// Une touche tapée par l'entrée « Clavier » : tenue ce temps-là, puis lâchée
// ce temps-là avant la suivante — plus qu'une image à 30 images/s.
const TYPING_STEP_MS = 40;

// useStreamexInput — the streamex half of the unified Streaming page's
// per-platform dispatcher (see stream/shared/hooks/use-streaming-input.js).
// Same pattern as use-retrogaming-input.js.
//
// preview — the keytest preset name (P2 §5, KeytestLive): the keytest pad
// decision lives HERE, not in Streaming.jsx's dispatcher. The wire
// consoleId is lowercase "keytest" (catalog.go's NewKeyTest); with a
// preset the pad renders that preset's controller on top of the input
// layer, without one (a Dashboard joiner) it renders the bare KeyTestPad.
// screenRef — the PAGE's one gateway viewer (Streaming.jsx owns it, see
// SessionStream's doc): input goes through it and its viewerId rides the WS
// query, but this hook never creates one of its own anymore.
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
export function useStreamexInput({ screenRef, session, t, options = {} }) {
	// drivesPad — the child's answer to "do my keys reach a pad?". It differs
	// from !isWatching during a loan only (#274), and a page without loans
	// (KeyTest) need not pass it.
	const { isWatching = false, drivesPad = !isWatching, preview = null } = options;
	const { token } = useAuth();
	const [profileButtons, setProfileButtons] = useState([]);
	// The page says when to connect (onConnect): once this tab's gateway
	// viewer exists, or after its deadline (StreamingShell's gate).
	const [connectRequested, setConnectRequested] = useState(false);

	// LA SOURIS — remise en service le 2026-09-09.
	//
	// Elle était partie sans bruit à la refonte (commit 5744bc8) : le
	// serveur écoute toujours `md`/`mu`/`mr`/`ms`
	// (streamex/cmd/streamex/main.go), mais plus personne ne les envoyait —
	// il n'en restait qu'une mention dans un commentaire. Les émulateurs qui
	// demandent un pointeur perdaient donc leur seul moyen d'être pilotés :
	// l'écran du GamePad Wii U, le tableau de bord Xbox, les menus de RPCS3
	// et de Eden. Trouvée par l'audit des affectations perdues, pas par un
	// test — rien ne la couvrait.
	//
	// Le protocole envoie un DÉPLACEMENT (`mr,<dx>,<dy>`), pas une position :
	// l'émulateur a son propre curseur, et lui pousser des coordonnées d'une
	// fenêtre de navigateur ne voudrait rien dire.
	const lastPointerRef = useRef({ x: null, y: null, dx: 0, dy: 0 });
	// Le doigt posé sur un ÉCRAN TACTILE (useTouchScreen) : le bouton gauche
	// n'est enfoncé qu'une fois, au premier contact, et relâché une fois.
	const touchDownRef = useRef(false);

	// La décision du pad KeyTest vit dans KeyTestPad.jsx (useKeytestPad) : le
	// téléphone sert la même console sans passer par ce hook, et une décision
	// enfermée ici lui serait restée invisible.
	const isKeyTest = session.consoleId === "keytest";
	const keytestPad = useKeytestPad(preview);
	const virtualPad = isKeyTest ? keytestPad : null;
	const controllerLabel = isKeyTest ? (preview ? (CONTROLLER_PRESETS[preview]?.label || preview) : t("streaming.controller.keyTest")) : null;
	const {
		isConnected,
		isConnecting,
		connectionState,
		connectionStats,
		setStatsMonitoring,
		handleConnectionStateChange,
		handleStatsUpdate,
		sendMessage,
	} = useStreamexStreamingConnection({
		screenRef,
		session,
		t,
		drivesPad,
		enabled: connectRequested,
	});

	// L'écran tactile (voir `touch` plus bas). STABLE : useTouchScreen se
	// rattache à la vidéo quand `send` change, et cette page se redessine
	// plusieurs fois par seconde (les relevés).
	const sendTouch = useCallback((x, y, pressed) => {
		const scale = (v) => Math.round(((v + 1) / 2) * 10000);
		sendMessage(`ma,${scale(x)},${scale(y)}`);
		if (pressed !== touchDownRef.current) {
			touchDownRef.current = pressed;
			sendMessage(pressed ? "md,1" : "mu,1");
		}
	}, [sendMessage]);

	useEffect(() => {
		if (!session.id) {
			return;
		}

		let cancelled = false;

		fetch(absoluteApiUrl(`/api/v1/streamex/${session.id}/profile.json`))
			.then((response) => response.json())
			.then((profile) => {
				if (!cancelled) {
					setProfileButtons(profile.buttons || []);
				}
			})
			.catch(() => { });

		return () => {
			cancelled = true;
		};
	}, [session.id]);

	// P6 — the pads only send on CHANGE (no continuous neutral polling —
	// see controllers/Xbox.jsx's own stick/button state guards), so a
	// stamp here is a real human touch; the shell drains the store onto
	// the façade WS at most once a minute. onChainWindowChange keeps the
	// raw sendMessage on purpose: a settings change, not an input.
	const sendInput = useCallback(
		(payload, options) => {
			// Only the tab holding the priority drives the pad (C2-P4d).
			// A RELEASE ("ku,") always goes through, even from a tab that
			markActivity();
			sendMessage(payload, options);
		},
		[sendMessage],
	);

	// LE CLAVIER DU TÉLÉPHONE (#420). Un téléphone n'a pas de clavier à
	// brancher, et la fenêtre « Create Profile » de Xenia ne se remplit qu'au
	// clavier : sans lui, pas de profil, et pas de sauvegarde sur la Xbox 360.
	// L'entrée « Clavier » du menu tape ici ce qu'on a écrit dans son champ,
	// touche par touche, puis Entrée.
	//
	// Chaque touche tenue un instant, puis un instant entre deux : un
	// émulateur qui lit l'état du clavier une fois par image ne verrait pas
	// une touche enfoncée et relâchée entre deux lectures.
	const typeText = useCallback(async (text) => {
		const { keys, missing } = keysOfText(text);
		const press = (code, down) => sendInput(`${down ? "kd" : "ku"},${code}`, { ordered: true });
		const pause = () => new Promise((resolve) => setTimeout(resolve, TYPING_STEP_MS));
		for (const { code, shift } of [...keys, { code: "Enter", shift: false }]) {
			if (shift) {
				press("ShiftLeft", true);
			}
			press(code, true);
			await pause();
			press(code, false);
			if (shift) {
				press("ShiftLeft", false);
			}
			await pause();
		}

		return { missing };
	}, [sendInput]);

	// The physical controller and the keyboard, read by the SESSION rather
	// than by the on-screen pad. They used to live inside the pad component,
	// which the page unmounts as soon as a real controller is connected — so
	// plugging one in silenced it (2026-08-25). KeyTest is excluded: its own
	// pad reads the Gamepad API with a richer table (chords, analog triggers)
	// and is always on screen, so it would double every press.
	useStreamexPhysicalInput({
		enabled: drivesPad && !isKeyTest,
		order: CONSOLE_GAMEPAD_ORDER[session.consoleId],
		profileButtons,
		sendInput,
	});

	// onConnect OPENS THE SOCKET, as on the other platforms. It used to be a
	// no-op: the socket opened by itself as soon as the play link arrived —
	// which happened to be after the gateway viewer existed. Without the link
	// (P5) it opened at once, BEFORE the viewer, and the child, which routes
	// a socket's input by the viewer it announced, dropped all of it (found
	// 2026-10-09). Error/close just clean up (a façade WS drop must not kill
	// the session for the others).
	// A façade WS drop must not kill the session for the others — and must
	// not kill the picture either: the PAGE owns the gateway viewer now (see
	// SessionStream): tearing it down from here kills the video for the
	// whole page, and the platform WS re-opening does NOT bring it back
	// (SessionStream stops retrying once attached). Confirmed live
	// 2026-08-24 in the gateway log — "viewer added" then "viewer removed"
	// ~310ms later, then no picture at all. The viewer is released by
	// GatewayWebRTC's own unmount effect, nowhere else.
	const onConnect = useCallback(() => setConnectRequested(true), []);
	const onDisconnect = useCallback(() => { }, []);

	// P4 — contract actions the shell triggers; the request messages ride
	// the child, as intents (sessionSend), so the hook only carries the platform-
	// side follow-ups.
	const onSlotGiveaway = useCallback(() => {
		// No-op by design: the queue moved the seat, and this tab's socket
		// stays as it is — nothing to tear down (unlike retro's relay, the
		// child never needs a cedeslot message from this side anymore).
	}, []);
	const hasFreeSlots = useCallback(() => session.openSlots ?? 0, [session.openSlots]);
	const onMonitoringResource = useCallback((open, powerOnly = false) => {
		const { serverStatus, devicePower } = useSessionStats({
			platform: "streamex",
			sessionId: session.id,
			startedAt: session.startedAt,
			// LE JETON MANQUAIT, et la route l'exige : la requête partait sans
			// en-tête, revenait en 401, et le panneau gardait ses tirets.
			token,
			setStatsMonitoring,
			intervalMs: 5000,
			open,
			powerOnly,
		});
		// `.data` — ET PAS L'ENVELOPPE. Le kernel répond `{success, data}` :
		// étaler la réponse entière donnait un objet à deux clés dont aucune
		// n'est celle que le panneau lit, donc un tiret sur CHAQUE ligne
		// mesurée alors que la mesure arrivait bel et bien (constaté le
		// 2026-09-09 sur une session PS4). Le rétro déballait déjà ; les deux
		// autres non.
		// `device` — le téléphone qu'on tient, quand la partie tourne
		// ailleurs (voir useSessionStats).
		return { ...connectionStats, ...serverStatus?.data, device: devicePower };
	}, [session.id, session.startedAt, token, setStatsMonitoring, connectionStats]);

	return {
		// streamex has no separate exit-message concept (unlike retro-
		// gaming's disconnect()/leavePage split) and no manual connect()
		// either: this connection auto-connects off its own socket effect.
		// Neither of these releases the gateway viewer — the PAGE owns it
		// (see SessionStream), and GatewayWebRTC's unmount effect is the only
		// thing allowed to let it go.
		// Same three verbs as the other platforms (see use-retrogaming-input's
		// doc). streamex has no exit message and no save states: leaving is
		// just dropping this connection, and ending the session for everyone
		// is the page's own stop-session REST call.
		disconnect: () => { },
		terminate: () => { },
		onConnect,
		onLeave: () => { },
		onDisconnect,
		onSlotGiveaway,
		hasFreeSlots,
		onMonitoringResource,
		title: session.title,
		// The video surface belongs to the page now (SessionStream) — no ref,
		// no sessionId here. streamex needs nothing on <GatewayWebRTC> beyond
		// the two state callbacks.
		gatewayProps: {
			onConnectionStateChange: handleConnectionStateChange,
			onStatsUpdate: handleStatsUpdate,
		},
		isConnected,
		isConnecting,
		connectionState,
		controller: () => {
			const { Component: ConsolePad, label: consolePadLabel } = resolveControllerLayout({
				session,
				override: virtualPad,
				overrideLabel: controllerLabel,
			});

			// No pad mapped for this console — render nothing rather than
			// <null />, which React rejects with "Element type is invalid…
			// got: null" and takes the WHOLE page down, prompts and all.
			// resolveControllerLayout returns null for any streamex console
			// missing from STREAMEX_CONSOLE_PADS, so this is reachable with
			// a real console, not just the blank test ones (seen live).
			if (!ConsolePad) {
				return { label: consolePadLabel, Component: null };
			}

			return {
				label: consolePadLabel,
				Component: (
					<ConsolePad
						profileButtons={profileButtons}
						enabled={drivesPad}
						sendInput={sendInput}
					/>
				),
			};
		},
		extraProps: {
			// Le relais souris/tactile. Propre à streamex : c'est la seule
			// plateforme dont l'invité est un bureau, avec un curseur à
			// bouger. La page pose une surface transparente par-dessus la
			// vidéo et y branche ces gestes — elle ne peut pas les mettre
			// sur le <video>, qui appartient à GatewayWebRTC.
			//
			// `null` pour un spectateur : regarder ne bouge pas la souris de
			// celui qui joue.
			pointerHandlers: drivesPad ? {
				onPointerDown: (event) => {
					// La capture suit le pointeur hors de la surface : sans
					// elle, un glissement qui sort du cadre s'arrête net au
					// bord, ce qui est exactement le geste qu'on veut faire
					// dans un menu.
					event.currentTarget.setPointerCapture(event.pointerId);
					lastPointerRef.current = { x: event.clientX, y: event.clientY, dx: 0, dy: 0 };
					sendMessage(`md,${event.button === 2 ? 3 : 1}`);
				},
				onPointerUp: (event) => {
					lastPointerRef.current = { x: null, y: null, dx: 0, dy: 0 };
					sendMessage(`mu,${event.button === 2 ? 3 : 1}`);
				},
				onPointerMove: (event) => {
					const last = lastPointerRef.current;
					if (last.x === null) {
						return;
					}
					// DES ENTIERS SUR LE FIL, ET LE RESTE GARDÉ.
					//
					// `clientX`/`clientY` sont fractionnaires dès que le ratio
					// de pixels ne vaut pas 1 — c'est-à-dire sur la plupart des
					// téléphones. Le serveur lit ces deltas avec `strconv.Atoi`
					// et jette l'erreur (`dx, _ :=`), donc une valeur
					// fractionnaire devenait un déplacement de ZÉRO : le
					// mouvement vertical était mort sans que rien ne le dise.
					//
					// On arrondit donc ici, et on REPORTE le reste : sans ça un
					// glissement lent, dont chaque pas fait moins d'un pixel,
					// n'avancerait jamais.
					const dx = last.dx + (event.clientX - last.x);
					const dy = last.dy + (event.clientY - last.y);
					const stepX = Math.trunc(dx);
					const stepY = Math.trunc(dy);
					lastPointerRef.current = {
						x: event.clientX,
						y: event.clientY,
						dx: dx - stepX,
						dy: dy - stepY,
					};
					if (stepX !== 0 || stepY !== 0) {
						sendMessage(`mr,${stepX},${stepY}`);
					}
				},
				// La molette aussi rend des fractions (défilement fluide,
				// pavé tactile).
				onWheel: (event) => sendMessage(`ms,${Math.round(-event.deltaY)}`),
				// Sinon le clic droit ouvre le menu du navigateur PAR-DESSUS
				// le jeu, au lieu d'atteindre l'émulateur.
				onContextMenu: (event) => event.preventDefault(),
			} : null,
			// L'ÉCRAN TACTILE d'une console qui en a un (`touch` au registre :
			// la Switch, #435), au serveur comme au téléphone : un doigt posé
			// sur l'image est un clic À CET ENDROIT de la fenêtre de
			// l'émulateur (`ma`, 0..10000, mis à l'échelle par le serveur),
			// et non le pavé tactile relatif ci-dessus, qui déplace un curseur
			// qu'on ne voit pas. C'est ce qui rend le clavier de saisie d'Eden
			// utilisable : la manette, elle, atteint aussi le jeu derrière lui
			// (cemuhook) — B y efface une lettre ET fait revenir le jeu en
			// arrière (le porteur, 2026-10-23). La page ne pose alors pas la
			// surface relative (Streaming.jsx).
			touch: drivesPad ? { send: sendTouch } : null,
			// L'entrée « Clavier » du menu (#420) — celui qui tient une
			// manette seulement, comme la souris.
			keyboard: drivesPad ? { typeText } : null,
			// Reduced to the genuinely streamex-only knob (P4) — join/
			// takeover/handover ride the façade WS + the contract fields
			// above now.
			// KeyTest ONLY. It is not a real emulator: it reads every button
			// on its own and cannot chain presses into combos, so it needs
			// the window pushed to it over the wire. Every other console
			// (PS3, Switch, Xbox, and the other platforms) runs a real
			// emulator that chains presses itself — sending `cw,` to those
			// is meaningless, and it was going out to all of them.
			onChainWindowChange: (windowMs) => {
				if (session.consoleId !== "keytest") {
					return;
				}

				sendMessage(`cw,${windowMs}`);
			},
		},
	};
}
