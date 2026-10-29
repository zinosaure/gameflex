// components/PlayStation3.jsx — the PS3 input surface for the unified
// streamex page: the GameFlex DualShock3 (controllers/DualShock3, same
// virtual-pad system as retro-gaming and remote-play) wired to streamex's
// wire protocol (kd/ku + st,<l|r>,x,y). With the uinput virtual gamepad
// active (the default), RPCS3 gets REAL analog sticks; the server falls
// back to keyboard emulation if /dev/uinput is missing. Mirrors
// NintendoSwitch.jsx — only the mappings differ.
//
// Contract (props from Streaming.jsx): profileButtons from /profile.json
// ({id,label,key} — gp:* codes when the virtual gamepad is active), enabled
// (serverRole === "player"), sendInput(payload) — the page owns the socket
// and the role gating, this component only ever decides WHICH message a
// button/input means.

import React from "react";
import { resolveProfileButton, makeCanDrawButton } from "./profile-buttons.js";
import { logThrottled } from "../../../shared/utils/log-store.js";
import { DualShock3 } from "../../../controllers/Controllers.jsx";

// DualShock3 button name → /profile.json button id (the preset names in
// stream/onscreen/Controllers.jsx vs the RPCS3 profile — ids are
// the same in keyboard and evdev mode, only the key codes differ).
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

export default function PlayStation3Pad({
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
		<DualShock3
			onButtonDown={handleButtonDown}
			onButtonUp={handleButtonUp}
			onAnalogMove={handleAnalogMove}
			canDrawButton={canDrawButton}
		/>
	);
}
