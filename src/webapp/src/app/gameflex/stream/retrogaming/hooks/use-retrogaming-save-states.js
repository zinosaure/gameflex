import { useCallback, useRef, useState } from "react";

// The retro-gaming-only actions that used to live inline in
// use-retro-streaming-connection.js — save/load state, cheats, fast-
// forward/rewind, and the PS1 controller-type debug switch. None of these
// exist on streamex or remote-play (see docs/wiggly-rolling-plum's "Hooks
// frontend" plan): streamex has no save states at all, remote-play's saves
// are the PS4/PS5's own OS-level ones, entirely outside this app.
//
// sendSignaling — the same WS sender use-retro-streaming-connection.js
// already owns; passed in rather than duplicated, since the WS itself is
// still that hook's (the connection stays a single WebSocket carrying both
// the session protocol and these messages, unlike streamex's slots which
// were judged NOT worth splitting out for the same reason — see the plan).
// registerMessageHandlers — lets this hook plug its own message types
// (savestate/loadstate/cheats/error's pending-callback cases) into the
// parent's ws.onmessage switch without the parent needing to know their
// shape; called once per connect(), returns nothing.
// Assez pour un aller-retour WS jusqu'au conteneur, bien trop court pour
// qu'on attende devant un menu.
const DISKS_ANSWER_TIMEOUT_MS = 2500;

export function useRetrogamingSaveStates({ sendSignaling }) {
	// Set right before sending, cleared (and called) by whichever of
	// "savestate"/"loadstate"/"error" comes back next — the backend only
	// ever has one request in flight at a time per session.
	const pendingStateCallbackRef = useRef(null);
	// Same one-at-a-time assumption, for "setcheats" instead of save/load.
	const pendingCheatsCallbackRef = useRef(null);
	// Idem pour les disques. Le menu ne pose qu'une question à la fois — il
	// ne s'ouvre qu'une fois.
	const pendingDisksCallbackRef = useRef(null);

	// Whether the loaded core exports retro_cheat_set at all (see the
	// "started" message's CheatsSupported field) — false until the
	// owner's offer arrives; always false for a joiner/spectator.
	const [cheatsSupported, setCheatsSupported] = useState(false);
	// The owner's persisted cheat list, as the offer/started message
	// carried it — CheatsPanel seeds its own local list from this once.
	const [initialCheats, setInitialCheats] = useState([]);
	// LE REMBOBINAGE NE VA PAS À TOUTES LES CONSOLES, et ce n'est pas au
	// menu de le deviner — c'est la partie qui le dit, sur ce même message.
	//
	// Rembobiner, c'est sérialiser l'état complet de la console une fois par
	// seconde. Un état PS2 pèse 44 Mio : la boucle d'émulation y perdait une
	// image par seconde, et une image perdue par seconde, c'est le son qui
	// se coupe (signalé le 2026-09-08). Le serveur écarte donc les consoles
	// dont l'état est trop gros (voir rewindStateSizeMax, stream_session.go)
	// et le DIT ici, plutôt que de laisser un interrupteur qui coûte cher
	// pour rien. Faux tant que le message n'est pas arrivé, et pour un
	// joueur/spectateur qui ne le reçoit jamais.
	const [rewindSupported, setRewindSupported] = useState(false);
	// L'AVANCE RAPIDE, DÉCLARÉE de même : le moteur GameFlex la sait (serveur
	// et téléphone), un invité de bureau du téléphone (RPCS3, xemu…) non — le
	// menu lui proposait un interrupteur sans effet (le porteur, 2026-10-28).
	const [fastForwardSupported, setFastForwardSupported] = useState(false);

	// Called by the parent hook's ws.onmessage from the "started" case, which
	// carries the capability fields.
	const applyStartedCapabilities = useCallback((msg) => {
		setCheatsSupported(!!msg.cheatsSupported);
		setRewindSupported(!!msg.rewindSupported);
		setFastForwardSupported(!!msg.fastForwardSupported);
		setInitialCheats(msg.cheats || []);
	}, []);

	// La réponse sur le rembobinage arrive UNE SECONDE APRÈS le démarrage,
	// pas avec "started" — le serveur ne la connaît pas plus tôt : il la
	// mesure sur sa première capture réelle plutôt que de la demander à un
	// cœur qui n'a pas encore tourné (voir decideRewindLocked côté Go, et le
	// plantage de dolphin qui a imposé ce report). Une reconnexion, elle,
	// retrouve la réponse sur "started" : à ce moment-là elle est connue.
	const applyRewindSupport = useCallback((msg) => {
		setRewindSupported(!!msg.rewindSupported);
	}, []);

	// Called by the parent hook's ws.onmessage — returns true if this hook
	// consumed the message (so the parent knows not to also handle it).
	const handleMessage = useCallback((msg) => {
		switch (msg.type) {
			case "savestate":
			case "loadstate":
				pendingStateCallbackRef.current?.(null, msg.info);
				pendingStateCallbackRef.current = null;
				return true;
			case "cheats":
				pendingCheatsCallbackRef.current?.(null, msg.cheats || []);
				pendingCheatsCallbackRef.current = null;
				return true;
			case "disks":
				pendingDisksCallbackRef.current?.({
					count: msg.disks || 0,
					current: msg.disk ?? -1,
					// Un refus se dit — sinon le sélecteur revient au disque
					// précédent sans un mot, indistinguable d'un clic perdu.
					...(msg.info === "denied" ? { error: msg.info } : {}),
				});
				pendingDisksCallbackRef.current = null;
				return true;
			case "error":
				if (pendingStateCallbackRef.current) {
					// Le message entier en troisième : un refus codé
					// (`state-refused`, #396) s'y lit avec ses versions.
					pendingStateCallbackRef.current(msg.error, null, msg);
					pendingStateCallbackRef.current = null;
					return true;
				}
				if (pendingCheatsCallbackRef.current) {
					pendingCheatsCallbackRef.current(msg.error, null);
					pendingCheatsCallbackRef.current = null;
					return true;
				}
				// Un changement de disque refusé : on rend « zéro disque »
				// plutôt que de laisser la promesse en l'air. Le menu se
				// referme sur cette réponse au lieu d'attendre pour toujours.
				if (pendingDisksCallbackRef.current) {
					pendingDisksCallbackRef.current({ count: 0, current: -1 });
					pendingDisksCallbackRef.current = null;
					return true;
				}
				return false;
			default:
				return false;
		}
	}, []);

	// onResult(error, info, answer) — error is the backend's message string
	// (or null on success), info is the ack string (e.g. "saved"/"loaded"),
	// answer the whole error message (its `code`, when it has one).
	// slot — 0-9; slot 0 is reserved for the automatic on-exit save and
	// rejected server-side for an explicit saveState call.
	const saveState = useCallback(
		(slot, onResult) => {
			pendingStateCallbackRef.current = onResult;
			sendSignaling({ type: "savestate", slot });
		},
		[sendSignaling],
	);

	const loadState = useCallback(
		(slot, onResult) => {
			pendingStateCallbackRef.current = onResult;
			sendSignaling({ type: "loadstate", slot });
		},
		[sendSignaling],
	);

	// Sends the FULL desired cheat list every time (not incremental) — the
	// backend resets and re-applies the whole list on every call, matching
	// how RetroArch itself re-syncs cheats. onResult(error, cheats) — cheats
	// is the list actually applied (echoed back), only meaningful on
	// success.
	const setCheats = useCallback(
		(cheats, onResult) => {
			pendingCheatsCallbackRef.current = onResult;
			sendSignaling({ type: "setcheats", cheats });
		},
		[sendSignaling],
	);

	// Fire-and-forget toggle — the backend echoes an ack, but the button
	// already reflects the requested state optimistically, so there's
	// nothing to reconcile it against here.
	const setFastForward = useCallback(
		(enabled) => sendSignaling({ type: "fastforward", enabled }),
		[sendSignaling],
	);

	// Mutually exclusive with fast-forward server-side — the caller is
	// responsible for not holding both active in its own UI state.
	const setRewind = useCallback(
		(enabled) => sendSignaling({ type: "rewind", enabled }),
		[sendSignaling],
	);

	// Temporary debug tool (Streaming.jsx's debugTab, PS1-only) — live
	// controller-device-id switching. deviceId is libretro's raw numeric
	// device id.
	const setControllerType = useCallback(
		(port, deviceId) => sendSignaling({ type: "setcontrollertype", port, deviceId }),
		[sendSignaling],
	);

	// LES DISQUES — combien, et lequel est monté.
	//
	// Une QUESTION, pas une capacité déclarée. Un enfant qui ne sait pas y
	// répondre (image plus ancienne que le cas 13 du runtime), un core sans
	// interface de disques, un jeu en un seul fichier : les trois rendent
	// « zéro disque », et le menu n'affiche alors aucun sélecteur. Rien à
	// tenir à jour des deux côtés, donc rien qui puisse finir par mentir.
	//
	// Le délai existe pour le premier de ces trois cas : un enfant qui ne
	// connaît pas le message ne répond RIEN du tout, et sans lui la promesse
	// resterait en suspens pour toute la durée de la partie.
	const askDisks = useCallback(
		(message) => new Promise((resolve) => {
			const timer = setTimeout(() => {
				if (pendingDisksCallbackRef.current) {
					pendingDisksCallbackRef.current = null;
					resolve({ count: 0, current: -1 });
				}
			}, DISKS_ANSWER_TIMEOUT_MS);
			pendingDisksCallbackRef.current = (state) => {
				clearTimeout(timer);
				resolve(state);
			};
			sendSignaling(message);
		}),
		[sendSignaling],
	);

	// GELER LA PARTIE, sans la décharger.
	//
	// Le core reste en mémoire, donc la reprise est instantanée : c'est toute
	// la différence avec « arrêter la session », qui libère la machine. Et
	// c'est le SEUL moment où une partie s'arrête d'elle-même — revenir jouer
	// la reprend tout seul, côté serveur.
	const setPaused = useCallback(
		(paused) => sendSignaling({ type: "pause", paused }),
		[sendSignaling],
	);

	// LE BOUTON RESET DE LA CONSOLE (chantier 13) : le jeu repart de son
	// démarrage. Sans accusé : un échec se dit dans le journal du serveur ou
	// du téléphone, jamais sur le canal d'erreur, qui couperait la partie.
	const resetConsole = useCallback(
		() => sendSignaling({ type: "reset" }),
		[sendSignaling],
	);

	const queryDisks = useCallback(() => askDisks({ type: "disks" }), [askDisks]);

	// Le serveur relit TOUJOURS l'état réel après coup et le renvoie : un
	// choix refusé revient donc de lui-même au disque précédent, sans que le
	// menu ait à deviner ce qui s'est passé.
	const changeDisk = useCallback(
		(index) => askDisks({ type: "changedisk", disk: index }),
		[askDisks],
	);

	return {
		cheatsSupported,
		rewindSupported,
		fastForwardSupported,
		initialCheats,
		applyStartedCapabilities,
		applyRewindSupport,
		handleMessage,
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
	};
}
