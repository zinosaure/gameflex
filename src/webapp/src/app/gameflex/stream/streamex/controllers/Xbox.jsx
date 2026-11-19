// components/Xbox.jsx — the Xbox input surface for the unified streamex
// page: the GameFlex Xbox pad (controllers/Xbox, 360-style layout) wired
// to streamex's wire protocol (kd/ku + st,<l|r>,x,y via uinput on the
// server), plus keyboard pass-through and Gamepad API mapping. The Xbox
// runtime (streamex + xemu) serves the SAME pad profile as the Switch's
// SDL mode (see defaultSwitchPadProfile in streamex/cmd/streamex/main.go),
// so the profile ids below are the same gp:* family as NintendoSwitch.jsx —
// only the button NAMES differ (Xbox diamond, LB/RB/LT/RT, BACK/START).
//
// Contract (props from Streaming.jsx): profileButtons from /profile.json
// ({id,label,key}), enabled (serverRole === "player"), sendInput(payload).
import React from "react";
import { resolveProfileButton, makeCanDrawButton } from "./profile-buttons.js";
import { logThrottled } from "../../../shared/utils/log-store.js";
import { Xbox } from "../../../controllers/Controllers.jsx";

// Xbox pad button name → /profile.json button id (gp:* family — the xemu
// container serves defaultSwitchPadProfile, same ids as the Switch pad).
const BUTTON_TO_PROFILE_ID = {
	A: "a",
	B: "b",
	X: "x",
	Y: "y",
	LB: "l",
	RB: "r",
	LT: "zl",
	RT: "zr",
	START: "plus",
	// Le gabarit Xbox dessine `SELECT` ; cette table n'attendait que `BACK`,
	// et ce bouton-là ne partait donc jamais. Même défaut que sur la Switch,
	// même correctif : les deux noms mènent au même identifiant de profil.
	BACK: "minus",
	SELECT: "minus",
	// Le gabarit Xbox dessine `LS`/`RS` (le clic des sticks) ; cette table
	// n'attendait que `L3`/`R3`, donc ces deux boutons n'envoyaient RIEN —
	// exactement le défaut de BACK/SELECT juste au-dessus, trouvé de la même
	// façon (le 2026-09-10, en cherchant pourquoi un TOUCHPAD sans
	// destination était dessiné sur le pad PS5). Le profil, lui, a bien
	// `lstick`/`rstick`.
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
// SERVER drives the uinput virtual gamepad (real analog via SDL2).
const STICK_WHICH = { LEFT_STICK: "l", RIGHT_STICK: "r" };

export default function XboxPad({ profileButtons, enabled, sendInput }) {
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
		<Xbox
			onButtonDown={handleButtonDown}
			onButtonUp={handleButtonUp}
			onAnalogMove={handleAnalogMove}
			canDrawButton={canDrawButton}
		/>
	);
}
