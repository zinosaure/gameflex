import NintendoSwitchPad from "./NintendoSwitch.jsx";
import PlayStation3Pad from "./PlayStation3.jsx";
import PlayStation4Pad from "./PlayStation4.jsx";
import XboxPad from "./Xbox.jsx";

// The console → on-screen-pad lookup streamex's own Player.jsx used to
// declare locally — moved here so stream/shared/controller-layout.js (the
// single resolver every platform's Player.jsx now goes through) can import
// it without reaching into Player.jsx itself.
export const CONSOLE_PADS = {
	switch: NintendoSwitchPad,
	ps3: PlayStation3Pad,
	// La PS4 a SA manette depuis le 2026-09-08 : elle empruntait celle de la
	// PS3, qui n'a ni SHARE ni OPTIONS — le joueur voyait donc SELECT et
	// START, des boutons qui ne sont pas sur sa manette. Voir
	// PlayStation4.jsx, y compris pour pourquoi le pavé tactile n'y est
	// toujours pas dessiné.
	ps4: PlayStation4Pad,
	// La PS5 reprend le pad de la PS4, et ce n'est pas un pis-aller : le
	// gabarit qu'il dessine s'appelle `dualsense` et se présente lui-même
	// comme « PS4 (DualShock 4) / PS5 (DualSense) » (presets.js) — c'est le
	// MÊME dessin pour les deux, boutons Create/Options compris, qui sont des
	// icônes (`•••`, `≡`) et non des mots, donc justes de part et d'autre.
	// Et PlayStation4.jsx mappe déjà SHARE/OPTIONS sur `select`/`start`,
	// exactement les identifiants que `ps5.json` déclare (il hérite du profil
	// PS3, comme celui de la PS4). Tout ce qui s'affiche envoie quelque chose.
	//
	// Sans cette ligne, `CONSOLE_PADS[...]` rendait null : le jeu tournait, la
	// manette 1/4 était bien prise, et l'écran n'avait aucun pad — le
	// commutateur du menu disait « Visible » et ne montrait rien (2026-09-10,
	// même famille que la Wii U juste en dessous).
	ps5: PlayStation4Pad,
	xbox: XboxPad,
	// La Xbox 360 a le même profil que la Xbox (xbox360.json déclare les
	// mêmes identifiants, profiles.go les range sous la même manette) — et
	// le dessin est déjà celui d'une manette de 360.
	xbox360: XboxPad,
	// La Wii U emprunte la manette de la Switch, et ce n'est pas un
	// à-peu-près : son GamePad porte les MÊMES boutons (A/B/X/Y aux mêmes
	// positions, L/R, ZL/ZR, +/-, deux sticks cliquables), et surtout
	// wiiu.json déclare exactement les mêmes identifiants que switch.json
	// (vérifié fichier contre fichier). Sans cette ligne, `CONSOLE_PADS[...]`
	// rendait null et la console n'avait AUCUNE manette à l'écran — signalé
	// dès la première partie Wii U (2026-09-08).
	wiiu: NintendoSwitchPad,
};

// What to CALL the pad in the settings panel ("Show the … controller on
// screen"). Keyed by console — except when the caller supplies its own pad
// (PerformStream previewing one specific layout), in which case it names it
// via controllerLabel/resolveControllerLayout's own overrideLabel instead.
export const CONSOLE_PAD_LABELS = {
	switch: "Nintendo Switch",
	ps3: "PS3 (DualShock 3)",
	ps4: "PS4 (DualShock 4)",
	ps5: "PS5 (DualSense)",
	xbox: "Xbox (360)",
	xbox360: "Xbox 360",
	wiiu: "Wii U (GamePad)",
};

// Gamepad API standard-layout index → this console's profile button id.
// Lived as a private GAMEPAD_ORDER const inside each pad component, which is
// also where the polling lived — see use-streamex-physical-input.js for why
// both had to leave. Keyed by console so the hook can be mounted by the
// session, not by the drawing.
//
// Index 16 is the standard layout's "guide" button (PS / Home / Xbox). Only
// the PS3 profile has anything to bind it to (`ps`); switch.json and
// xbox.json have no home/guide entry at all, so their tables stop at 15 —
// mapping it there would name a button the emulator does not know.
const STANDARD_FACE_TO_DPAD = ["dup", "ddown", "dleft", "dright"];

// Les deux SEULS ordres qui existent, écrits une fois.
//
// Ils étaient recopiés par console — cinq tableaux pour deux contenus — et
// c'est ce genre de table qu'on oublie d'allonger : la PS5 n'y était pas, et
// une manette physique branchée sur une session PS5 n'envoyait rien (le
// 2026-09-10, en même temps que trois autres listes du même acabit côté
// serveur). Une console qui partage le vocabulaire d'une autre partage
// désormais la MÊME constante, et l'oubli se voit.
const DUALSHOCK_ORDER = [
	"cross", "circle", "square", "triangle",
	"l1", "r1", "l2", "r2",
	"select", "start", "l3", "r3",
	...STANDARD_FACE_TO_DPAD,
	"ps",
];

// La Switch, la Wii U et la Xbox déclarent les mêmes identifiants (vérifié
// fichier contre fichier : switch.json, wiiu.json, xbox.json).
const ABXY_ORDER = [
	"a", "b", "x", "y",
	"l", "r", "zl", "zr",
	"minus", "plus", "lstick", "rstick",
	...STANDARD_FACE_TO_DPAD,
];

export const CONSOLE_GAMEPAD_ORDER = {
	switch: ABXY_ORDER,
	ps3: DUALSHOCK_ORDER,
	ps4: DUALSHOCK_ORDER,
	ps5: DUALSHOCK_ORDER,
	xbox: ABXY_ORDER,
	xbox360: ABXY_ORDER,
	wiiu: ABXY_ORDER,
};
