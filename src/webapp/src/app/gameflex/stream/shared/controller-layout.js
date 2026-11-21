// resolveControllerLayout — the ONE place that decides which on-screen
// controller component a session renders, replacing 4 separate lookup
// mechanisms that used to exist: retro-gaming's RETRO_CONTROLLER_MAP (+ a
// PS1 special case switching live between two pads), streamex's own
// CONSOLE_PADS map (+ an injectable `override` prop), remote-play's single
// hardcoded <DualSense>, and PerformStream.jsx's own 4th lookup table
// (SEE_LAYOUT_CONTROLLER) for previewing a layout outside a real session.
//
// Returns { Component, label } together (not just the component) because
// retro-gaming's own controllerToggle needs BOTH derived from the exact
// same decision — a prior bug came from computing them via two separate,
// slightly different expressions that could disagree about which pad was
// actually on screen (see retrogaming/Player.jsx's own controllerToggle
// comment, still true of the logic moved here).
import {
	DefaultGamepad,
	DualSense,
	RETRO_CONTROLLER_PRESET_MAP,
	PRESET_COMPONENT_BY_NAME,
} from "../../controllers/Controllers.jsx";
import { CONTROLLER_PRESETS } from "../../controllers/presets.js";
import { CONSOLE_PADS as STREAMEX_CONSOLE_PADS, CONSOLE_PAD_LABELS as STREAMEX_CONSOLE_PAD_LABELS } from "../streamex/controllers/index.js";

// override — a caller-supplied component that takes precedence over any
// lookup (streamex's own `virtualPad` prop, e.g. Streaming.jsx's keytest
// case; PerformStream.jsx's own controller-preview harness). overrideLabel
// names it for the controllerToggle text when the caller doesn't already
// know a better one (streamex's own controllerLabel prop takes precedence
// over this when both are supplied — see streamex's own call site).
//
// ps1ControllerType — retro-gaming only: PS1 is the one console
// whose pad isn't fixed by consoleId alone (see the comment this replaced,
// retrogaming/Player.jsx). Ignored by the other two platforms.
export function resolveControllerLayout({
	session,
	override = null,
	overrideLabel = null,
	ps1ControllerType = null,
}) {
	if (override) {
		return { Component: override, label: overrideLabel };
	}

	// L'émulateur du téléphone est du rétro-gaming qui tourne ailleurs : même
	// console, même manette. Sans ce rapprochement, une NES s'affichait avec
	// un pad de PlayStation — le repli par défaut.
	if (session.platform === "retrogaming" || session.platform === "device") {
		// LA PS1 AVEC SA MANETTE D'ORIGINE, numérique, par défaut (décidé
		// par le porteur le 2026-10-11 : avec la DualShock, X ne passait pas
		// l'écran titre de Crash Team Racing). La DualShock ne s'affiche que
		// si le menu l'a explicitement choisie (type 517) ; tout le reste —
		// type 1, ou rien de connu — montre le pad numérique, comme ce que
		// la console a réellement. De août au 2026-10-11, c'était l'inverse.
		const presetName =
			session.consoleId === "ps1"
				? ps1ControllerType === 517
					? "dualshock"
					: "ps1"
				: RETRO_CONTROLLER_PRESET_MAP[session.consoleId] || "default";
		return {
			Component: PRESET_COMPONENT_BY_NAME[presetName] || DefaultGamepad,
			label: CONTROLLER_PRESETS[presetName]?.label,
		};
	}

	if (session.platform === "streamex") {
		return {
			Component: STREAMEX_CONSOLE_PADS[session.consoleId] || null,
			label: STREAMEX_CONSOLE_PAD_LABELS[session.consoleId] || null,
		};
	}

	// remote-play — no lookup needed, there is only ever one console family
	// (PS4/PS5) behind this platform.
	return { Component: DualSense, label: CONTROLLER_PRESETS.dualsense?.label };
}
