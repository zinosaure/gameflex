import React, { useEffect, useRef, useState } from "react";
import { getHapticFeedback } from "../../libs/gamepadPreferences.js";

// LE PAVÉ TACTILE — une surface, pas un bouton.
//
// Le geste est celui de l'application PS Remote Play (choisi le 2026-09-11) :
// un écran ne sait pas « appuyer plus fort », donc
//   - un toucher BREF et SANS GLISSER est un clic du pavé ;
//   - tout le reste — glisser, ou laisser le doigt posé — est un doigt qui
//     bouge sur le pavé.
//
// LE DOIGT EST POSÉ DÈS LE CONTACT, même pour un clic. Sur la vraie manette on
// ne clique jamais sans toucher, et des jeux lisent OÙ le pavé a été cliqué
// (moitié gauche, moitié droite). Un clic envoie donc, dans cet ordre : doigt
// posé, appui, relâchement, doigt levé.
//
// Deux doigts au plus, la limite de la vraie manette. Ce composant ne parle à
// personne : il appelle `callback.ontouch(doigt, posé, x, y)` — x et y de 0 à 1
// depuis le coin haut gauche — et `onmousedown`/`onmouseup` pour le clic ;
// chaque plateforme traduit vers ce que sa cible sait lire.
//
// Pas de `data-btn-name` : la couche de balayage de ControllerLayout ne doit
// pas prendre la surface pour un bouton qu'un doigt traverse.

const DEFAULT_VIBRATION_DURATION = 15; // milliseconds
const FINGERS = [0, 1];
// Un toucher plus long, ou qui a glissé plus loin que TAP_MAX_TRAVEL (en
// fraction de la surface), n'est plus un clic.
const TAP_MAX_MS = 250;
const TAP_MAX_TRAVEL = 0.05;
// Le protocole veut un appui ET un relâchement distincts : un clic reste
// enfoncé ce temps-là.
const CLICK_HOLD_MS = 80;

export default function TouchpadSurface({ button, callback, isLandscape = false }) {
	// Les callbacks sont reconstruits à chaque rendu du pad ; les gestes, eux,
	// durent plus longtemps qu'un rendu. On lit toujours les plus récents.
	const callbackRef = useRef(callback);
	callbackRef.current = callback;
	// pointerId → le doigt qu'il tient, où il a commencé, où il est.
	const pointersRef = useRef(new Map());
	// Les doigts occupés. Un doigt reste occupé jusqu'à ce qu'il soit VRAIMENT
	// levé — après son clic, pas au relâchement de l'écran — sinon un second
	// contact rapide le reprendrait avant que la console ne l'ait vu partir.
	const busyRef = useRef(new Set());
	// Les clics en cours : timer → ce qu'il reste à faire.
	const clicksRef = useRef(new Map());
	const [touched, setTouched] = useState(false);

	const lift = (finger, x, y) => {
		busyRef.current.delete(finger);
		callbackRef.current.ontouch?.(finger, false, x, y);
		if (busyRef.current.size === 0) {
			setTouched(false);
		}
	};

	// Ne jamais laisser un doigt posé ni un clic enfoncé derrière soi : la
	// console les garderait pour le restant de la partie.
	useEffect(() => () => {
		for (const [timer, finish] of clicksRef.current) {
			clearTimeout(timer);
			finish();
		}
		for (const pointer of pointersRef.current.values()) {
			callbackRef.current.ontouch?.(pointer.finger, false, pointer.x, pointer.y);
		}
		pointersRef.current.clear();
	}, []);

	const locate = (event) => {
		const rect = event.currentTarget.getBoundingClientRect();

		return {
			x: Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width)),
			y: Math.max(0, Math.min(1, (event.clientY - rect.top) / rect.height)),
		};
	};

	const onPointerDown = (event) => {
		event.preventDefault();
		event.stopPropagation();
		const finger = FINGERS.find((index) => !busyRef.current.has(index));
		if (finger === undefined) {
			return;
		}
		const at = locate(event);
		// La capture garde le doigt à nous même s'il sort du cadre en
		// glissant — sans elle, un relâchement hors de la surface ne nous
		// parviendrait jamais et le doigt resterait posé.
		try {
			event.currentTarget.setPointerCapture(event.pointerId);
		} catch {
			// Refusée : on perd seulement les relâchements hors cadre, que
			// pointercancel rattrape la plupart du temps.
		}
		busyRef.current.add(finger);
		pointersRef.current.set(event.pointerId, {
			finger,
			startX: at.x,
			startY: at.y,
			startedAt: performance.now(),
			x: at.x,
			y: at.y,
			moved: false,
		});
		setTouched(true);
		if (getHapticFeedback()) navigator.vibrate?.(DEFAULT_VIBRATION_DURATION);
		callbackRef.current.ontouch?.(finger, true, at.x, at.y);
	};

	const onPointerMove = (event) => {
		const pointer = pointersRef.current.get(event.pointerId);
		if (!pointer) {
			return;
		}
		event.preventDefault();
		const at = locate(event);
		if (Math.hypot(at.x - pointer.startX, at.y - pointer.startY) > TAP_MAX_TRAVEL) {
			pointer.moved = true;
		}
		pointer.x = at.x;
		pointer.y = at.y;
		callbackRef.current.ontouch?.(pointer.finger, true, at.x, at.y);
	};

	// allowClick — un pointercancel ou une capture perdue ne sont jamais un
	// clic : le joueur n'a pas choisi de lever le doigt.
	const end = (event, allowClick) => {
		const pointer = pointersRef.current.get(event.pointerId);
		if (!pointer) {
			return;
		}
		pointersRef.current.delete(event.pointerId);
		const click = allowClick && !pointer.moved && performance.now() - pointer.startedAt <= TAP_MAX_MS;
		if (!click) {
			lift(pointer.finger, pointer.x, pointer.y);

			return;
		}
		callbackRef.current.onmousedown?.();
		const finish = () => {
			clicksRef.current.delete(timer);
			callbackRef.current.onmouseup?.();
			lift(pointer.finger, pointer.x, pointer.y);
		};
		const timer = setTimeout(finish, CLICK_HOLD_MS);
		clicksRef.current.set(timer, finish);
	};

	const size = isLandscape ? button.xl.size : button.size;

	return (
		<div
			title={button.text || button.name}
			style={{
				position: "absolute",
				left: `${(isLandscape ? button.xl.x : button.x) * 100}%`,
				top: `${(isLandscape ? button.xl.y : button.y) * 100}%`,
				// Plus large que haut, comme le vrai pavé : le doigt y a de la
				// place pour glisser de gauche à droite.
				width: size * 2,
				height: size,
				transform: "translate(-50%, -50%)",
				borderRadius: 12,
				border: "1px solid rgba(255, 255, 255, 0.5)",
				boxShadow: "1px 1px 5px rgba(0, 0, 0, 0.5)",
				background: touched ? "rgba(100, 100, 100, 0.35)" : "transparent",
				cursor: "pointer",
				userSelect: "none",
				touchAction: "none",
				// La bande de la manette ne reçoit rien : la surface, si.
				pointerEvents: "auto",
				zIndex: 2,
			}}
			onPointerDown={onPointerDown}
			onPointerMove={onPointerMove}
			onPointerUp={(event) => end(event, true)}
			onPointerCancel={(event) => end(event, false)}
			onLostPointerCapture={(event) => end(event, false)}
		/>
	);
}
