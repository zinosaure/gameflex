// controllers/PlayStation4.jsx — la surface d'entrée PS4 (shadPS4).
//
// LA PS4 N'EST PAS UNE PS3, et elle empruntait pourtant son dessin.
//
// Les deux manettes se ressemblent de loin — même croix/rond/carré/triangle,
// mêmes L1/R1/L2/R2 — mais la DualShock 4 a remplacé SELECT et START par
// SHARE et OPTIONS, et ajouté un pavé tactile cliquable que la DualShock 3
// n'a jamais eu. Afficher un pad de PS3 sur une session PS4, c'est montrer au
// joueur des boutons qui ne sont pas ceux de sa manette (signalé le
// 2026-09-08).
//
// LE PAVÉ TACTILE PASSE PAR LA SOURIS DE shadPS4, pas par la manette
// virtuelle. shadPS4 a un mode « souris vers pavé tactile » : la position du
// curseur dans sa fenêtre devient le doigt, le bouton gauche le pose, le droit
// clique le pavé (input_mouse.cpp, EmulateTouchpad). streamex l'allume à
// chaque lancement — adapter.json, `keys_on_window` : la touche Suppr, car
// shadPS4 ne l'expose que comme raccourci. Un seul doigt, donc : c'est une
// souris.
//
// La voie de la manette reste fermée : SDL n'expose un pavé tactile que pour
// une manette qu'il RECONNAÎT comme DualShock 4, et notre pad virtuel se
// présente avec un identifiant générique. L'imiter au niveau HID (uhid) est un
// autre chantier.
//
// Contrat identique à PlayStation3.jsx (profileButtons/enabled/sendInput) —
// seuls le vocabulaire des boutons et le dessin changent.
import React, { useRef } from "react";
import { resolveProfileButton, makeCanDrawButton } from "./profile-buttons.js";
import { logThrottled } from "../../../shared/utils/log-store.js";
import { DualSense } from "../../../controllers/Controllers.jsx";

const BUTTON_TO_PROFILE_ID = {
	CROSS: "cross",
	CIRCLE: "circle",
	SQUARE: "square",
	TRIANGLE: "triangle",
	L1: "l1",
	L2: "l2",
	L3: "l3",
	R1: "r1",
	R2: "r2",
	R3: "r3",
	// Le vocabulaire de la DualShock 4 D'ABORD, celui de la PS3 ensuite : le
	// gabarit dessine SHARE/OPTIONS, mais ps4.json — hérité du profil PS3 —
	// nomme encore ces deux boutons `select` et `start`. Les deux entrées
	// pointent donc au même endroit, plutôt que de renommer un profil servi à
	// des sessions en cours.
	SHARE: "select",
	OPTIONS: "start",
	START: "start",
	SELECT: "select",
	PS: "ps",
	DPAD_UP: "dup",
	DPAD_DOWN: "ddown",
	DPAD_LEFT: "dleft",
	DPAD_RIGHT: "dright",
	DPAD_UP_LEFT: "upleft",
	DPAD_UP_RIGHT: "upright",
	DPAD_DOWN_LEFT: "downleft",
	DPAD_DOWN_RIGHT: "downright",
};

// Stick which id on the wire (st,<l|r>,x,y with x,y ∈ [-1000,1000]) — the
// SERVER emulates per console (RPCS3: real analog via the uinput virtual
// gamepad; Eden: its cemuhook pads, or keys on the phone), so the
// client only forwards the normalized deflection. No key synthesis here.
const STICK_WHICH = { LEFT_STICK: "l", RIGHT_STICK: "r" };

export default function PlayStation4Pad({
	profileButtons,
	enabled,
	sendInput,
}) {
	// Keyboard and physical gamepad are NOT read here anymore.
	//
	// They were, and this component is removed from the tree the moment a real
	// controller is connected (Streaming.jsx hides the on-screen pad then), so
	// plugging a pad in silenced the only code reading it. They now live in
	// stream/streamex/hooks/use-streamex-physical-input.js, mounted by the
	// session — see that file for the whole story. What stays here is what this
	// component is actually for: drawing a pad and turning touches into input.

	const sendButtonProfile = (name, down) => {
		const buttonProfile = resolveProfileButton(profileButtons, BUTTON_TO_PROFILE_ID, name);

		// UN APPUI JETÉ EN SILENCE EST INDIAGNOSTICABLE.
		//
		// Deux raisons de ne rien envoyer, et elles n'appellent pas la même
		// action : ce bouton n'existe pas dans le profil de la console (rien
		// ne marchera pour lui), ou cette page n'a pas la manette (elle
		// regarde). Les deux donnaient le même écran — un pad qui s'allume
		// sous le doigt et ne fait rien.
		if (!buttonProfile) {
			logThrottled("warn", "pad-no-profile", 2000,
				"pad: bouton absent du profil de la console", name,
				`(${profileButtons.length} chargés)`);

			return;
		}

		if (!enabled) {
			logThrottled("warn", "pad-not-player", 2000,
				"pad: appui ignoré, cette page n'a pas la manette", name);

			return;
		}

		sendInput(down ? `kd,${buttonProfile.key}` : `ku,${buttonProfile.key}`);
	};

	// Le pavé tactile — voir l'en-tête. `ma` porte la position de 0 à 10000
	// dans la fenêtre de l'émulateur : c'est le serveur qui en connaît les
	// pixels. Poser un doigt demande la manette ; le lever passe toujours, sinon
	// le bouton gauche resterait enfoncé chez l'émulateur.
	const touchingRef = useRef(false);
	const handleTouch = (finger, down, x, y) => {
		// Un seul doigt : le second n'a pas de souris à qui parler.
		if (finger !== 0 || (down ? !enabled : !touchingRef.current)) return;
		sendInput(`ma,${Math.round(x * 10000)},${Math.round(y * 10000)}`);
		if (down !== touchingRef.current) {
			touchingRef.current = down;
			sendInput(down ? "md,1" : "mu,1");
		}
	};
	// Le clic du pavé : le bouton droit de la souris.
	const handleTouchpadClick = (down) => {
		if (down && !enabled) return;
		sendInput(down ? "md,3" : "mu,3");
	};

	const handleButtonDown = (name) => (name === "TOUCHPAD" ? handleTouchpadClick(true) : sendButtonProfile(name, true));
	const handleButtonUp = (name) => (name === "TOUCHPAD" ? handleTouchpadClick(false) : sendButtonProfile(name, false));
	const handleAnalogMove = (name, x, y) => {
		const which = STICK_WHICH[name];
		if (!which || !enabled) return;
		const clamp = (value) => Math.max(-1000, Math.min(1000, Math.round(value * 1000)));
		sendInput(`st,${which},${clamp(x)},${clamp(y)}`);
	};

	// Ne pas dessiner un bouton qui n'a nulle part où aller — voir
	// makeCanDrawButton. Les sticks et le pavé tactile sont exclus du test :
	// ils ne passent pas par les boutons du profil.
	const canDrawButton = makeCanDrawButton(
		profileButtons,
		BUTTON_TO_PROFILE_ID,
		[...Object.keys(STICK_WHICH), "TOUCHPAD"],
	);

	return (
		<DualSense
			onButtonDown={handleButtonDown}
			onButtonUp={handleButtonUp}
			onAnalogMove={handleAnalogMove}
			onTouch={handleTouch}
			canDrawButton={canDrawButton}
		/>
	);
}
