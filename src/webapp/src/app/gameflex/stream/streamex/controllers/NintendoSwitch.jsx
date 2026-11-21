// components/NintendoSwitch.jsx — la surface d'entrée Switch ET WII U du
// page streamex unifiée.
//
// LE NOM DU FICHIER NE DIT PLUS TOUT, et il vaut mieux l'écrire que le
// laisser surprendre : depuis le 2026-09-08 cette même surface sert la Wii U.
// Ce n'est pas un raccourci — le GamePad Wii U porte les mêmes boutons aux
// mêmes places (A/B/X/Y, L/R, ZL/ZR, +/-, deux sticks cliquables), et surtout
// wiiu.json déclare exactement les mêmes identifiants que switch.json. Le
// dessin est le même JoyCon, la traduction est la même table.
//
// L'original, pour mémoire : the Switch input surface for the
// unified streamex page: the GameFlex JoyCon (controllers/JoyCon, same
// virtual-pad system as retro-gaming and remote-play) wired to streamex's
// wire protocol (kd/ku + st,<l|r>,x,y via XTest/uinput on the server),
// plus keyboard pass-through and Gamepad API mapping (buttons AND analog
// axes). EVERY console gets its own component here (PlayStation3.jsx,
// Xbox.jsx, …) because the mapping is console-specific — the shared
// WebRTC/WS plumbing stays in ../Streaming.jsx.
//
// Contract (props from Streaming.jsx): profileButtons from /profile.json
// ({id,label,key}), enabled (serverRole === "player"), sendInput(payload) —
// the page owns the socket and the role gating, this component only ever
// decides WHICH message a button/input means.
//
// Sticks: the host emulates them (Eden's cemuhook pads on the server, keys
// on the phone) — the client sends raw deflections, no direction-key
// synthesis here.

import React from "react";
import { resolveProfileButton, makeCanDrawButton } from "./profile-buttons.js";
import { logThrottled } from "../../../shared/utils/log-store.js";
import { JoyCon } from "../../../controllers/Controllers.jsx";

// JoyCon button name → /profile.json button id (see the joycon preset in
// stream/onscreen/Controllers.jsx — L/R/ZL/ZR/A/B/X/Y/START/SELECT/
// DPAD_*/L3/R3 plus the two analog sticks).
const BUTTON_TO_PROFILE_ID = {
	A: "a",
	B: "b",
	X: "x",
	Y: "y",
	L: "l",
	R: "r",
	ZL: "zl",
	ZR: "zr",
	// DEUX VOCABULAIRES POUR LES MÊMES BOUTONS, et les deux sont acceptés.
	//
	// Le gabarit `joycon` dessine les siens à la Nintendo — `MINUS`, `PLUS`,
	// `LS`, `RS` — là où les gabarits Xbox et PlayStation utilisent les noms
	// génériques `SELECT`, `START`, `L3`, `R3`. Cette table ne connaissait que
	// les seconds : les quatre boutons Switch correspondants s'allumaient sous
	// le doigt et n'envoyaient rien, sans une erreur (2026-09-07).
	//
	// On garde les deux plutôt que de choisir : un gabarit qui change de
	// vocabulaire ne doit pas pouvoir tuer un bouton en silence, et la trace
	// posée dans le pad dit maintenant lequel n'a pas de correspondance.
	START: "plus",
	PLUS: "plus",
	SELECT: "minus",
	MINUS: "minus",
	L3: "lstick",
	LS: "lstick",
	R3: "rstick",
	RS: "rstick",
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

export default function NintendoSwitchPad({
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

	const handleButtonDown = (name) => sendButtonProfile(name, true);
	const handleButtonUp = (name) => sendButtonProfile(name, false);
	const handleAnalogMove = (name, x, y) => {
		const which = STICK_WHICH[name];
		if (!which || !enabled) return;
		const clamp = (value) => Math.max(-1000, Math.min(1000, Math.round(value * 1000)));
		sendInput(`st,${which},${clamp(x)},${clamp(y)}`);
	};

	// Ne pas dessiner un bouton qui n'a nulle part où aller — voir
	// makeCanDrawButton. Les sticks sont exclus du test : ils passent par
	// onAnalogMove, pas par les boutons du profil.
	const canDrawButton = makeCanDrawButton(
		profileButtons,
		BUTTON_TO_PROFILE_ID,
		Object.keys(STICK_WHICH),
	);

	return (
		<JoyCon
			onButtonDown={handleButtonDown}
			onButtonUp={handleButtonUp}
			onAnalogMove={handleAnalogMove}
			canDrawButton={canDrawButton}
		/>
	);
}
