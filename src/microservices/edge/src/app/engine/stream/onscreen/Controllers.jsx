import React, { createContext, useContext, useEffect, useRef, useState } from "react";
import { listButtonPositioning, readToken } from "../../../libs/api.js";
import { getHapticFeedback, useGamepadPreferences } from "../../libs/gamepadPreferences.js";
import { DEFAULT_PAD_OPACITY, forgetPadOpacity, publishPadOpacity, usePadOpacity } from "./padOpacity.js";
import { logThrottled } from "../../../libs/log-store.js";
import { CONTROLLER_PRESETS } from "../../controllers/presets.js";
import TouchpadSurface from "./TouchpadSurface.jsx";

// Haptic feedback on virtual button press — navigator.vibrate is Android
// Chrome only (iOS Safari has never implemented the Vibration API at all,
// not something a feature-check can work around), hence the guard rather
// than assuming it exists. Short/subtle on purpose: this fires on every
// single button press during active gameplay, a long buzz would be
// obnoxious fast.

const DEFAULT_VIBERATION_DURATION = 15; // milliseconds

// editable/isSelected/onSelect/onDrag — ButtonPositioning (Phase 9,
// profile/ButtonPositioning.jsx) only: repositions THIS button (button.x/y)
// instead of the normal onmousedown/onmouseup/onmove callbacks, which don't
// apply while editing a layout (there's no session to send input to).
// Deliberately no visual change beyond a thin selection ring (same border
// style, just a different color) — the whole point of editing through this
// component instead of a separate mock-up is that what you see here IS
// exactly what a real session renders.
// UNE DISPOSITION ENREGISTRÉE DÉPLACE DES BOUTONS, ELLE N'EN DÉFINIT PAS LA LISTE.
//
// Elle remplaçait l'ensemble du gabarit. Tant que les deux vocabulaires
// coïncidaient, personne ne le voyait — mais les callbacks, eux, sont
// construits à partir du PRESET (voir buildCallbacks) : le jour où un gabarit
// renomme un bouton, ou qu'une disposition date d'un vocabulaire antérieur, la
// manette dessine des boutons que rien ne câble. Ils s'allument sous le doigt
// et n'envoient rien.
//
// C'est ce qui distinguait deux comptes sur la même console le 2026-09-07 :
// celui SANS disposition enregistrée jouait, celui qui en avait une non.
//
// On repart donc toujours du gabarit — c'est lui qui dit quels boutons
// existent — et on n'emprunte à l'enregistrement que ce qu'il a le droit de
// dire : où ils sont et quelle taille ils font. Un nom inconnu est ignoré, un
// bouton absent de l'enregistrement garde sa place par défaut.
function mergeSavedLayout(presetButtons, savedLayout) {
	if (!Array.isArray(savedLayout) || savedLayout.length === 0) {
		return presetButtons;
	}

	const savedByName = new Map(
		savedLayout.filter((button) => button?.name).map((button) => [button.name, button]),
	);

	return presetButtons.map((button) => {
		const saved = savedByName.get(button.name);
		if (!saved) {
			return button;
		}

		return {
			...button,
			...(typeof saved.x === "number" ? { x: saved.x } : {}),
			...(typeof saved.y === "number" ? { y: saved.y } : {}),
			...(typeof saved.size === "number" ? { size: saved.size } : {}),
			...(saved.xl ? { xl: { ...button.xl, ...saved.xl } } : {}),
		};
	});
}

// LE DESSIN DES BOUTONS, ET LUI SEUL (chantier 52, option A du porteur) : une page peut donner aux
// boutons sa propre apparence — le launcher, le sien. Le placement, les dispositions enregistrées,
// l'appui et le balayage restent ceux de ce fichier ; absent, rien ne change.
//
//   face({ pressed, selected, diagonal }) — le style du disque d'un bouton (couleur, fond, bordure,
//     ombres) ; un bouton diagonal de la croix n'a pas de disque ;
//   glyph(button) — ce qui s'écrit dedans ;
//   thumb(dragging) — le fond du pouce d'un stick.
export const PadLookContext = createContext(null);

function PlaceButton({
	button,
	callback = {},
	isLandscape = false,
	editable = false,
	isSelected = false,
	// Pressé SOUS LE DOIGT — vient du calque de balayage du conteneur, qui
	// est le seul à savoir quel bouton un pointeur tient (voir
	// ControllerLayout). Un bouton numérique ne peut pas le déduire seul :
	// son appui ne passe jamais par ses propres gestionnaires.
	isPressed = false,
	// LE STICK EST TOUJOURS FLOTTANT (#754, #758) : voir sa zone plus bas. `floatZone` : ses morceaux, déjà placés
	// dans la colonne du stick (floatingZones, plus bas). `invisible` : les sticks invisibles (Réglages › Manettes),
	// aucun stick dessiné au repos.
	invisible = false,
	floatZone = null,
	onSelect,
	onDrag,
}) {
	const analog = button.analog || false;
	const diagonal = button.diagonal || false;
	const look = useContext(PadLookContext);
	// Le stick fixe n'existe plus (le porteur, 2026-11-17 : « maintenant, on aura toujours les sticks flottant ») :
	// en jeu, un stick flotte toujours ; l'éditeur de disposition, lui, le déplace.
	const floats = analog && !editable;

	// State for tracking dragging and offset for analog sticks
	const [dragging, setDragging] = useState(false);
	const [offset, setOffset] = useState({ x: 0, y: 0 });
	// Flottant : où le pouce s'est posé — dans la colonne (anchor, en px, pour le dessin) et à l'écran (le
	// ref, pour la déviation), avec le doigt qui le tient. Rien de posé : le stick est à sa place.
	const [anchor, setAnchor] = useState(null);
	const floatRef = useRef(null);
	const size = isLandscape ? button.xl.size : button.size;
	// LE PAVÉ TACTILE DESSINÉ COMME UN BOUTON (la plateforme ne sait servir que son clic : la PS4 et la PS5 du téléphone,
	// l'éditeur de disposition) a la forme de la surface — deux fois plus large que haut, coins arrondis, sans glyphe —,
	// et non un disque (le porteur, 2026-11-17 : « Le touchpad n'est pas comme sur le serveur »).
	const padShape = button.name === "TOUCHPAD" && !analog;
	const ringRadius = size * 0.6;
	const thumbSize = size * 0.6;
	// La déviation, en px depuis le centre du stick, ramenée à son rayon et bornée au cercle.
	const deflect = (px, py) => {
		const radius = size / 2;
		let dx = px / radius;
		let dy = py / radius;
		const mag = Math.hypot(dx, dy);

		if (mag > 1) {
			dx /= mag;
			dy /= mag;
		}

		setOffset({ x: dx, y: dy });
		callback.onmove?.(dx, dy);
	};
	// Le relâchement du doigt qui tient le stick flottant : le stick revient à sa place, à zéro.
	const releaseFloating = (e) => {
		if (floatRef.current?.id !== e.pointerId) return;
		floatRef.current = null;
		setDragging(false);
		setOffset({ x: 0, y: 0 });
		setAnchor(null);
		callback.onmove?.(0, 0);
		callback.onmouseup?.();
		if (getHapticFeedback()) navigator.vibrate?.(DEFAULT_VIBERATION_DURATION);
	};

	// Position drag (editable mode) — resolves the new x/y as a fraction of
	// this button's own ColumnZone box (same coordinate space button.x/y
	// already use), clamped to stay inside it.
	const updatePositionFromEvent = (e) => {
		const zone = e.currentTarget.parentElement;
		if (!zone) return;
		const rect = zone.getBoundingClientRect();
		const x = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width));
		const y = Math.min(1, Math.max(0, (e.clientY - rect.top) / rect.height));
		onDrag?.(button.name, x, y);
	};

	// LE STICK FLOTTANT, SOUS les boutons (zIndex 1 contre 2) — un toucher sur un bouton reste un bouton ; sur sa zone
	// ou sur le stick dessiné, il prend le stick. Le point touché devient son centre, le pouce le dévie à partir de là ;
	// un seul doigt le tient, capturé ici : il peut sortir de la zone sans rien presser au passage. Le centre se compte
	// depuis la colonne, où le stick se dessine, quel que soit le morceau touché.
	const floatHandlers = floats ? {
		"data-floating-stick": button.name,
		onPointerDown: (e) => {
			e.preventDefault();
			e.stopPropagation();
			if (floatRef.current) return;
			try {
				e.currentTarget.setPointerCapture?.(e.pointerId);
			} catch {
				/* noop */
			}
			const column = e.currentTarget.parentElement.getBoundingClientRect();
			floatRef.current = { id: e.pointerId, x: e.clientX, y: e.clientY };
			setAnchor({ x: e.clientX - column.left, y: e.clientY - column.top });
			setOffset({ x: 0, y: 0 });
			setDragging(true);
			callback.onmousedown?.();
			if (getHapticFeedback()) navigator.vibrate?.(DEFAULT_VIBERATION_DURATION);
		},
		onPointerMove: (e) => {
			const held = floatRef.current;
			if (held?.id !== e.pointerId) return;
			e.preventDefault();
			deflect(e.clientX - held.x, e.clientY - held.y);
		},
		onPointerUp: (e) => {
			e.stopPropagation();
			releaseFloating(e);
		},
		onPointerCancel: releaseFloating,
		onLostPointerCapture: releaseFloating,
	} : {};

	const drawn = (
		<div
			title={button.text || button.name}
			data-btn-name={button.name}
			data-btn-analog={analog ? "true" : undefined}
			style={{
				position: "absolute",
				left: anchor ? `${anchor.x}px` : `${isLandscape ? button.xl.x * 100 : button.x * 100}%`,
				top: anchor ? `${anchor.y}px` : `${isLandscape ? button.xl.y * 100 : button.y * 100}%`,
				width: padShape ? size * 2 : size,
				height: size,
				transform: "translate(-50%, -50%)",
				display: "flex",
				alignItems: "center",
				justifyContent: "center",
				cursor: "grab",
				userSelect: "none",
				touchAction: "none",
				// La bande qui le porte ne reçoit rien (voir ControllerLayout) :
				// le bouton, lui, doit. Le stick flottant aussi : le toucher là où on le voit le prend.
				pointerEvents: "auto",
				borderRadius: padShape ? 12 : "50%",
				zIndex: editable && isSelected ? 3 : 2,
				// Les sticks invisibles : rien de dessiné au repos, le stick apparaît sous le pouce. Il reste touchable là.
				opacity: floats && invisible && !anchor ? 0 : undefined,
			}}
			onClick={(e) => {
				e.stopPropagation();
				e.preventDefault();
				if (!editable) callback.onclick?.();
			}}
			onPointerDown={(e) => {
				if (editable) {
					e.stopPropagation();
					e.preventDefault();
					onSelect?.(button.name);
					setDragging(true);
					try {
						e.target.setPointerCapture?.(e.pointerId);
					} catch {
						/* noop */
					}
					return;
				}
				// Sticks: their own handlers (floatHandlers, spread below) replace these.
				// Digital buttons: press/release live in the container's swipe
				// layer (a finger rolls across several buttons — touch captures
				// the FIRST element implicitly, so per-button handlers would
				// never see the next ones). No capture here: the event bubbles.
			}}
			onPointerMove={(e) => {
				if (!editable || !dragging) return;
				e.preventDefault();
				updatePositionFromEvent(e);
			}}
			onPointerUp={(e) => {
				if (!editable) return; // the container swipe layer released it
				e.stopPropagation();
				e.preventDefault();
				setDragging(false);
			}}
			onPointerCancel={() => {
				if (editable) setDragging(false);
			}}
			{...floatHandlers}
		>
			<div
				style={look ? {
					width: "100%",
					height: "100%",
					fontSize: Math.max(isLandscape ? button.xl.size * 0.4: button.size * 0.4, 15),
					display: "flex",
					alignItems: "center",
					justifyContent: "center",
					cursor: "grab",
					userSelect: "none",
					touchAction: "none",
					...look.face({ pressed: isPressed, selected: editable && isSelected, diagonal }),
					...(padShape ? { borderRadius: 12 } : {}),
				} : {
					// Le glyphe s'éclaircit sous l'appui : le fond pressé est
					// de la MÊME teinte que lui, il y disparaîtrait sinon.
					color: isPressed ? "rgba(255, 255, 255, 0.95)" : "rgba(100, 100, 100, 1)",
					width: "100%",
					height: "100%",
					fontSize: Math.max(isLandscape ? button.xl.size * 0.4: button.size * 0.4, 15),
					display: "flex",
					alignItems: "center",
					justifyContent: "center",
					cursor: "grab",
					userSelect: "none",
					touchAction: "none",
					shapeOutside: "circle()",
					...(!diagonal ? {
						boxShadow: "1px 1px 5px rgba(0, 0, 0, 0.5)",
						textShadow: "1px 1px 5px rgba(0, 0, 0, 0.5)",
						borderRadius: padShape ? 12 : "50%",
						// Pressé : la même teinte, plus dense. C'est déjà le
						// langage du pouce du stick juste en dessous (0,35 →
						// 0,75), plutôt qu'une couleur étrangère au pad. Sans
						// transition : un retour d'appui se veut instantané,
						// et il n'y a donc aucun mouvement à réduire.
						background: isPressed ? "rgba(100, 100, 100, 0.75)" : undefined,
						border:
							editable && isSelected
								? "2px solid #6ea8fe"
								: isPressed
									? "1px solid rgba(255, 255, 255, 0.9)"
									: "1px solid rgba(255, 255, 255, 0.5)",
					} : {}),
				}}
				className={`groupname-${button.group}`}
			>
				{analog ? (
					// The ring+thumb VISUAL. No handlers of its own: in game the
					// stick floats (floatHandlers on the outer div and its zone),
					// and in the editor (ButtonPositioning) the drag passes
					// straight through to the outer PlaceButton div, which
					// repositions the whole stick (button.x/y).
					<div
						style={{
							position: "absolute",
							left: "50%",
							top: "50%",
							width: size,
							height: size,
							transform: "translate(-50%, -50%)",
							pointerEvents: "none",
							touchAction: "none",
							cursor: "grab",
							zIndex: 3,
						}}
					>
						<div
							style={{
								position: "absolute",
								top: `calc(50% + ${(editable ? 0 : offset.y) * ringRadius}px)`,
								left: `calc(50% + ${(editable ? 0 : offset.x) * ringRadius}px)`,
								width: thumbSize,
								height: thumbSize,
								borderRadius: "50%",
								transform: "translate(-50%, -50%)",
								background: look
									? look.thumb(dragging)
									: dragging
										? "rgba(100, 100, 100,0.75)"
										: "rgba(100, 100, 100,0.35)",
								pointerEvents: "none",
							}}
						/>
					</div>
				) : padShape ? null : look ? look.glyph(button) : (
					<>
						{button.icon ? (
							<i className={`${button.icon}`}></i>
						) : (
							button.text || button.name
						)}
					</>
				)}
			</div>
		</div>
	);

	if (!floats) {
		return drawn;
	}

	// Sa zone (#758, floatingZones) : sous les boutons, jusqu'en bas de l'écran.
	return (
		<>
			{(floatZone || []).map((piece) => (
				<div key={`${piece.left}:${piece.top}`} {...floatHandlers}
					style={{ ...piece, position: "absolute", zIndex: 1, pointerEvents: "auto", touchAction: "none" }} />
			))}
			{drawn}
		</>
	);
}

// LA LARGEUR D'UNE COLONNE DE LA MANETTE À L'ÉCRAN : les boutons s'y placent
// en fraction, quelle que soit la disposition — celle des préréglages ou
// celle qu'un joueur a enregistrée. La vue discrète de la page de jeu
// s'aligne dessus en paysage (PlayShell.jsx).
export const PAD_COLUMN_WIDTH = 160;

// LA MANETTE, REMONTÉE EN PORTRAIT (le porteur, 2026-10-23 : « remonter les boutons et le menu d'au moins 60 px ; les
// sticks sont trop collés au bord du bas du téléphone ») — le geste de retour et la barre de navigation d'Android sont
// là. Le bouton du menu ne suit plus cette marge depuis le 2026-11-17 : il descend en bas à droite (use-streaming-menu.js).
const PORTRAIT_LIFT = 60;

// LA MANETTE, AU-DESSUS DE LA RANGÉE DU MENU (le porteur, 2026-11-03 : « les
// boutons menus sont intercalés »). Le bouton et ses pastilles font 48 px : à
// 64 px de plus, le bas des sticks passe au-dessus d'eux.
//
// PUIS TOUTE LA MANETTE REMONTÉE, SANS RIEN REDIMENSIONNER (le porteur, 2026-11-17 : « Le but était de remonter les
// boutons, pas de redimensionner. C'était juste le margin bottom qu'il fallait augmenter ») : 60 px de plus en
// portrait. En paysage, rien au-dessus des 20 px du bord (le porteur, 2026-11-17 : « En landscape, les boutons sont
// trop remontés » — L et R sortaient par le haut du S25).
// Puis 20 px plus bas (le porteur, 2026-11-17 : « en mode portrait, de baisser le block des boutons de 20px à peu près.
// Juste pour que ça soit dessous du texte GameFlex et les stats »).
// Puis 56 px plus bas encore (le porteur, 2026-11-18, sur le S25 avec FF7 Remake : « le bloc des boutons n'est toujours
// pas descendu de quelques pixels […] c'est superposé sur les stats ») : la manette se pose depuis le BAS de l'écran, et
// dans l'application l'écran visible est plus court que dans Chrome (les barres d'Android) — elle montait d'autant. Mesuré
// sur sa capture (384 px de large) : le haut du pavé tactile à ~270 px, le bas de « GameFlex » sous la vue discrète à
// ~314 ; 56 px laissent un écart, et le bas de la manette reste à ~200 px du bord.
const PORTRAIT_PAD_LIFT = PORTRAIT_LIFT + 64 - 16;
const LANDSCAPE_PAD_LIFT = 0;
const PAD_BOTTOM = 20;
const PAD_HEIGHT = 300;

// LA ZONE DU STICK FLOTTANT (#758 ; le porteur, 2026-11-17, ses dessins dans
// docs/reports/2026-11-17-menu-manette/zones/ : « Je t'ai rajouté les zone sticks sur les écrans. Ce qui sera différent
// dépendant en taille des manettes ») : toute la largeur de l'écran, sous les boutons, jusqu'en bas de l'écran.
//   - en portrait, du dessous du bouton le plus bas ;
//   - en paysage, du dessous des boutons de chaque côté ; au milieu, entre les deux groupes, elle monte jusqu'au bas
//     du bouton le plus proche du milieu de chaque côté, jamais au-dessus de la moitié de la manette (l'image garde le
//     haut), et jamais sur le pavé tactile.
// Elle se calcule depuis les boutons dessinés — le gabarit de la console ou la disposition enregistrée —, jamais en
// chiffres par console. Un stick prend toute la zone ; deux sticks se la partagent, la moitié gauche au stick de
// gauche. Les boutons passent devant elle (zIndex 2 contre 1). Renvoie, par stick, ses morceaux placés dans SA colonne.
const FLOAT_ZONE_GAP = 12;

function floatingZones(buttons, { isLandscape, width, below, isSurface }) {
	const columnLeft = { left: 0, center: (width - PAD_COLUMN_WIDTH) / 2, right: width - PAD_COLUMN_WIDTH };
	const boxes = buttons.map((button) => {
		const at = isLandscape ? button.xl : button;
		const x = (columnLeft[button.column] ?? 0) + at.x * PAD_COLUMN_WIDTH;
		// Le pavé tactile est deux fois plus large que haut (TouchpadSurface.jsx).
		const halfWidth = isSurface(button) ? at.size : at.size / 2;

		return { button, x, left: x - halfWidth, right: x + halfWidth, bottom: at.y * PAD_HEIGHT + at.size / 2 };
	});
	const sticks = boxes.filter((box) => box.button.analog).sort((a, b) => a.x - b.x);
	const keys = boxes.filter((box) => !box.button.analog);
	const lowest = (list, floor = 0) => list.reduce((bottom, box) => Math.max(bottom, box.bottom), floor) + FLOAT_ZONE_GAP;
	let bands = [{ left: 0, right: width, top: lowest(keys) }];

	if (isLandscape) {
		const leftKeys = keys.filter((box) => box.button.column === "left");
		const rightKeys = keys.filter((box) => box.button.column === "right");
		const leftEdge = leftKeys.length ? Math.max(...leftKeys.map((box) => box.right)) + FLOAT_ZONE_GAP : 0;
		const rightEdge = rightKeys.length ? Math.min(...rightKeys.map((box) => box.left)) - FLOAT_ZONE_GAP : width;
		const inner = [
			leftKeys.reduce((best, box) => (!best || box.right > best.right ? box : best), null),
			rightKeys.reduce((best, box) => (!best || box.left < best.left ? box : best), null),
		].filter(Boolean);
		const surfaces = keys.filter((box) => isSurface(box.button) && box.right > leftEdge && box.left < rightEdge);

		bands = [
			{ left: 0, right: leftEdge, top: lowest(leftKeys) },
			{ left: leftEdge, right: rightEdge, top: lowest([...inner, ...surfaces], PAD_HEIGHT / 2) },
			{ left: rightEdge, right: width, top: lowest(rightKeys) },
		];
	}

	return Object.fromEntries(sticks.map((stick, index) => {
		const from = (width * index) / sticks.length;
		const to = (width * (index + 1)) / sticks.length;
		const origin = columnLeft[stick.button.column] ?? 0;
		const pieces = bands
			.map((band) => ({ left: Math.max(band.left, from), right: Math.min(band.right, to), top: band.top }))
			.filter((piece) => piece.right > piece.left)
			.map((piece) => ({ left: piece.left - origin, width: piece.right - piece.left, top: piece.top, bottom: -below }));

		return [stick.button.name, pieces];
	}));
}

// LE PAVÉ TACTILE RESTE QUAND UNE VRAIE MANETTE CACHE CELLE DE L'ÉCRAN (le
// porteur, 2026-11-03 : « en transparence et un peu plus grand »). La page dit
// comment sa surface est tournée : `portrait`, `landscape`, ou rien pour la
// manette entière. Sans pavé dans le gabarit, ou sans plateforme qui sache le
// servir, rien ne s'affiche.
export const TouchpadOnlyContext = createContext(null);

// Sa taille, seul : celle du pavé de la manette entière, agrandie d'un cinquième (60 en portrait, 90 en paysage).
// Fixée ici, et non tirée du gabarit : le pavé de la manette entière en paysage est bien plus grand (#758, presets.js)
// et, seul, il couvrirait la partie qu'on joue à la vraie manette.
// Plus gros qu'en paysage avec la manette à l'écran (180 de haut, presets.js) : à côté d'une vraie manette, le pavé
// est la seule chose à toucher (le porteur, 2026-11-18). En portrait, la largeur de l'écran le borne.
const TOUCHPAD_ALONE_SIZE = { portrait: 120, landscape: 220 };
// En portrait, sous la vidéo, la vue discrète quand elle est allumée, et la
// ligne qui la suit : il n'y a qu'un demi-écran sous une manette clipsée
// (#410). La hauteur de la vue n'est définie qu'à un endroit, sa constante
// (STATS_STRIP_PORTRAIT_HEIGHT, FloatingPanel.jsx), que la page passe par
// --gameflex-stats-strip-height. La rangée du menu y remonte à sa hauteur :
// app.css décale alors le pavé à droite de ses pastilles. En paysage, là où la
// manette entière le met.
//
// JAMAIS SOUS LE BAS DE LA PARTIE VISIBLE (8 px au moins) : dans Chrome, le
// demi-écran ne laisse qu'environ 290 px de haut au S25 (680 − 390, calculé),
// et la vue discrète poussait alors plus de la moitié du pavé sous la manette
// clipsée. Il remonte plutôt sur la vue et sur « GameFlex », des textes qui ne
// captent rien, et il reste à moitié transparent, sans fond.
const TOUCHPAD_ALONE_PLACE = {
	portrait: (size) => ({
		left: "var(--gameflex-touchpad-left, 50%)",
		bottom: `max(8px, calc(var(--gameflex-visible-height) - 56.25vw - var(--gameflex-stats-strip-height, 0px) - ${size + 32}px))`,
		transform: "translateX(-50%)",
	}),
	landscape: (size) => ({ left: "50%", bottom: Math.max(8, 110 - size / 2), transform: "translateX(-50%)" }),
};

function ColumnZone({
	column,
	buttons,
	callbacks = {},
	isLandscape = false,
	editable = false,
	selectedName = null,
	// A SET of names when a whole group is selected. Kept alongside
	// selectedName rather than replacing it: every other caller highlights
	// exactly one button, and a Set of one would be a worse way to say that.
	selectedNames = null,
	// Les noms tenus par un doigt, pour la couleur d'appui. Un Set comme
	// selectedNames juste au-dessus, et ici la forme est la bonne par
	// nature : plusieurs pouces pressent plusieurs boutons à la fois.
	pressedNames = null,
	invisible = false,
	// Par stick, les morceaux de sa zone (floatingZones).
	floatZones = null,
	onSelectButton,
	onButtonDrag,
	height = PAD_HEIGHT,
}) {
	const look = useContext(PadLookContext);

	return (
		<div
			className="position-relative"
			style={{
				width: PAD_COLUMN_WIDTH,
				height: height,
				touchAction: "none",
			}}
		>
			{buttons
				.filter((b) => b.column === column)
				.map((b) => (!editable && callbacks[b.name]?.ontouch ? (
					// Un pavé tactile que la plateforme sait servir : une
					// surface, pas un bouton. L'éditeur de disposition, lui,
					// continue de le déplacer comme n'importe quel bouton.
					<TouchpadSurface
						key={b.name}
						button={b}
						callback={callbacks[b.name]}
						isLandscape={isLandscape}
						look={look}
					/>
				) : (
					<PlaceButton
						key={b.name}
						button={b}
						callback={callbacks[b.name] || {}}
						isLandscape={isLandscape}
						editable={editable}
						isSelected={selectedNames ? selectedNames.has(b.name) : selectedName === b.name}
						isPressed={pressedNames ? pressedNames.has(b.name) : false}
						invisible={invisible}
						floatZone={floatZones?.[b.name]}
						onSelect={onSelectButton}
						onDrag={onButtonDrag}
					/>
				)))}
		</div>
	);
}

// editable/layoutOverride/onLayoutChange/selectedName/onSelectButton — all
// ButtonPositioning-only (profile/ButtonPositioning.jsx, Phase 9). layoutOverride
// makes this a CONTROLLED layout (the editor owns the array being edited)
// instead of the normal fetch-your-own-default behavior below — passing it
// skips that fetch entirely, and the editor's own `opacity` prop is used
// as-is (fetchedOpacity, set only by that fetch, stays null). This is
// deliberately the SAME component real sessions render (Controllers.jsx's
// makeController), not a separate mock-up, so a saved layout (position AND
// opacity) is guaranteed to look exactly like what a player sees.
export default function ControllerLayout({
	name = "default",
	callbacks = {},
	opacity = DEFAULT_PAD_OPACITY,
	editable = false,
	layoutOverride = null,
	onLayoutChange = null,
	selectedName = null,
	onSelectButton = null,
	// When set, dragging any button moves its whole group (presets.js's
	// `group`). Groups have no coordinates of their own — they are a way to
	// move, not a thing to edit.
	dragGroup = false,
	// canDrawButton — un prédicat sur le nom d'un bouton du gabarit.
	//
	// Un gabarit sert plusieurs consoles, et toutes ne savent pas tout : le
	// dessin `dualsense` porte un TOUCHPAD que ni ps4.json ni ps5.json ne
	// déclarent, donc un bouton qui s'allumait sous le doigt sans rien
	// envoyer — et une ligne d'avertissement à chaque appui. Celui qui
	// connaît le profil de la console répond ici, plutôt que de laisser le
	// joueur le découvrir.
	//
	// Un prédicat plutôt qu'une liste : le pad sait résoudre un nom, il n'a
	// pas à énumérer un gabarit qu'il ne possède pas.
	//
	// Ne concerne que l'AFFICHAGE : la disposition sauvegardée, l'éditeur et
	// les groupes continuent de voir tous les boutons (l'éditeur ne passe pas
	// ce prop).
	canDrawButton = null,
}) {
	name = name in CONTROLLER_PRESETS ? name : "default";
	const [internalLayout, setInternalLayout] = useState(
		() => CONTROLLER_PRESETS[name].buttons,
	);
	// fetchedOpacity stays null (falls back to the `opacity` prop) whenever
	// layoutOverride is active — the effect below returns early in that
	// case, so an editor-controlled opacity always just passes straight
	// through via the `opacity` prop itself, no separate override needed.
	const [fetchedOpacity, setFetchedOpacity] = useState(null);
	const touchpadOnly = useContext(TouchpadOnlyContext);
	const look = useContext(PadLookContext);
	// LA LARGEUR DE LA BANDE DE LA MANETTE, suivie (ResizeObserver : la taille de mise en page, que l'image tournée en
	// repli ne fausse pas) — la zone du stick flottant en dépend (floatingZones).
	const stripRef = useRef(null);
	const [stripWidth, setStripWidth] = useState(0);
	useEffect(() => {
		const strip = stripRef.current;
		if (!strip || typeof ResizeObserver === "undefined") return undefined;
		const observer = new ResizeObserver(([entry]) => setStripWidth(entry.contentRect.width));
		observer.observe(strip);
		return () => observer.disconnect();
	}, [touchpadOnly]);
	// LES STICKS INVISIBLES, suivis (#758) : basculés dans le menu d'une partie, ils s'appliquent à la manette déjà
	// dessinée. Jamais dans l'éditeur de disposition : on doit y voir les sticks pour les déplacer.
	const { invisibleSticks: invisiblePreference } = useGamepadPreferences();
	const invisibleSticks = !editable && invisiblePreference;
	const [isLandscape, setIsLandscape] = useState(
		window.innerWidth > window.innerHeight,
	);
	const token = readToken();
	const layout = layoutOverride || internalLayout;
	const drawn = canDrawButton ? layout.filter((b) => canDrawButton(b.name)) : layout;
	const padLift = PAD_BOTTOM + (isLandscape ? LANDSCAPE_PAD_LIFT : PORTRAIT_PAD_LIFT);
	const floatZones = !editable && stripWidth ? floatingZones(drawn, {
		isLandscape,
		width: stripWidth,
		below: padLift,
		// La forme du pavé, surface ou bouton (padShape) : la même boîte.
		isSurface: (button) => button.name === "TOUCHPAD",
	}) : null;

	// ButtonPositioning (Phase 9, profile/ButtonPositioning.jsx) — a user's saved
	// DEFAULT slot for this virtual controller preset, if any, takes
	// priority over the hard-coded CONTROLLER_PRESETS layout below. Falls
	// back to (and stays on) the preset's own layout for a logged-out
	// caller or a user who never saved a positioning for this preset — this
	// is an opt-in override, not a required setup step, same pattern as
	// KeyMapping (engine/libs/keyMapping.js). Skipped entirely when
	// layoutOverride is provided — the editor is the one deciding what's
	// loaded/edited in that case.
	useEffect(() => {
		if (layoutOverride) return undefined;
		setInternalLayout(CONTROLLER_PRESETS[name].buttons);
		setFetchedOpacity(null);
		if (!token) return undefined;
		let cancelled = false;
		// LA DISPOSITION SE LIT CHEZ NOUS, avec notre jeton (libs/api.js, chantier 85) : là où Réglages › Manettes l'écrit,
		// même dans une partie chez un serveur fédéré (elle y partait avec le jeton de l'hôte, sur notre adresse : un 401).
		listButtonPositioning(name).then((result) => {
			if (cancelled) return;
			// LA DISPOSITION NE SE LIT PAS (un invité sans compte, le serveur injoignable) : la manette garde celle par défaut,
			// et l'opacité se règle quand même depuis le menu, pour cette page seulement (rien ne s'enregistre ; le porteur,
			// 2026-11-17 : « dans mobileapp je ne vois pas […] l'opacité des boutons »).
			if (!result.ok) {
				publishPadOpacity({ name, opacity, slot: null, slots: [], saves: false });

				return;
			}
			const defaultSlot = (result.items || []).find((s) => s.is_default);
			if (defaultSlot) {
				setInternalLayout(mergeSavedLayout(CONTROLLER_PRESETS[name].buttons, defaultSlot.layout));
				if (typeof defaultSlot.opacity === "number")
					setFetchedOpacity(defaultSlot.opacity);
			}
			// Le menu d'une partie règle cette opacité-là (padOpacity.js) : ce qui a été lu, pour lui.
			publishPadOpacity({
				name,
				opacity: typeof defaultSlot?.opacity === "number" ? defaultSlot.opacity : opacity,
				slot: defaultSlot || null,
				slots: result.items || [],
				saves: true,
			});
		});
		return () => {
			cancelled = true;
			forgetPadOpacity(name);
		};
	}, [name, token, layoutOverride]);
	// Réglée depuis le menu, elle vaut tout de suite — réglée pendant que la manette était cachée aussi (`lastName`).
	const tuned = usePadOpacity();
	const tunedOpacity = !layoutOverride && (tuned.name || tuned.lastName) === name ? tuned.opacity : null;

	useEffect(() => {
		const handleResize = () => {
			setIsLandscape(window.innerWidth > window.innerHeight);
		};
		window.addEventListener("resize", handleResize);
		return () => window.removeEventListener("resize", handleResize);
	}, []);

	// Writes to x/y (portrait) or xl.x/xl.y (landscape) depending on the
	// CURRENT window shape (isLandscape, tracked above) — buttons render at
	// genuinely different positions per orientation (see PlaceButton's own
	// isLandscape ? button.xl... : button... read), so a drag performed
	// while the editor happens to be in one shape must only ever update
	// THAT shape's coordinates, never the other one's.
	// Where a button sits in the CURRENT orientation, with the same fallback
	// the renderer uses — a button with no landscape entry shows its portrait
	// position, so that is the position a drag moves it from.
	const positionOf = (button) => (isLandscape
		? { x: button.xl?.x ?? button.x, y: button.xl?.y ?? button.y }
		: { x: button.x, y: button.y });

	const handleButtonDrag = (buttonName, x, y) => {
		const dragged = layout.find((b) => b.name === buttonName);
		const group = dragGroup && dragged?.group ? dragged.group : null;

		if (!group) {
			onLayoutChange?.(
				layout.map((b) => {
					if (b.name !== buttonName) return b;
					return isLandscape ? { ...b, xl: { ...b.xl, x, y } } : { ...b, x, y };
				}),
			);
			return;
		}

		// The whole group moves by the SAME delta the dragged button moved —
		// a group keeps its shape, otherwise it is not a group, it is eight
		// buttons that happened to be near each other.
		const from = positionOf(dragged);
		const members = layout.filter((b) => b.group === group);

		// CLAMPED so no member leaves the play area. Without this, dragging
		// the d-pad into a corner pushes its far side off the surface, where
		// nothing can be tapped and nothing can be dragged back — the group
		// would have to be repaired one button at a time, or by reset.
		let dx = x - from.x;
		let dy = y - from.y;
		for (const member of members) {
			const at = positionOf(member);
			dx = Math.min(Math.max(dx, -at.x), 1 - at.x);
			dy = Math.min(Math.max(dy, -at.y), 1 - at.y);
		}

		onLayoutChange?.(
			layout.map((b) => {
				if (b.group !== group) return b;
				const at = positionOf(b);
				// Rounded for the same reason the panel rounds: a dragged
				// group is a chain of deltas, and the drift compounds.
				const moved = {
					x: Math.round((at.x + dx) * 10000) / 10000,
					y: Math.round((at.y + dy) * 10000) / 10000,
				};
				return isLandscape ? { ...b, xl: { ...b.xl, ...moved } } : { ...b, ...moved };
			}),
		);
	};
	// ── Touch swipe across buttons ─────────────────────────────────────
	// One finger can roll across the d-pad (down → down-right → right,
	// Tekken-style): the pointer is captured on THIS container and each
	// move hit-tests the button under the finger — press the new one,
	// release the previous. Touch would otherwise implicitly capture the
	// first button and never see the next ones.
	//
	// Tracking is PER POINTER (a map, not a single slot): two thumbs at
	// once (d-pad + face button) is the norm on a virtual pad — with one
	// shared slot, the second finger overwrote the first one's tracking and
	// its button stayed visually AND logically pressed until the second
	// finger lifted (reported as stuck keys).
	const swipeRef = useRef({});
	// L'APPUI VISIBLE EST DÉRIVÉ DU SUIVI, PAS TENU EN DOUBLE.
	//
	// `swipeRef` porte la logique : un ref, synchrone, qui ne provoque aucun
	// rendu — la couleur ne pouvait donc pas en venir. L'ensemble ci-dessous
	// est RECALCULÉ depuis lui à chaque appui et chaque relâchement, plutôt
	// que maintenu en parallèle : deux états qui se répondent finissent par
	// diverger, et ça se voit comme un bouton resté allumé.
	//
	// Filtré par « a un câblage » : un bouton dessiné sans callback n'envoie
	// rien, et l'allumer quand même recréerait exactement le symptôme que
	// pressSwipePointer met en garde plus bas (il s'allume sous le doigt et
	// n'envoie rien). Le ref, lui, le suit malgré tout — sinon un doigt qui
	// roule depuis ce bouton vers un bouton câblé ne presserait jamais.
	const [pressedNames, setPressedNames] = useState(() => new Set());
	const syncPressedNames = () => {
		setPressedNames(
			new Set(
				Object.values(swipeRef.current).filter(
					(name) => callbacks[name]?.onmousedown,
				),
			),
		);
	};
	const releaseSwipePointer = (pointerId) => {
		const pressedName = swipeRef.current[pointerId];
		if (!pressedName) return;
		delete swipeRef.current[pointerId];
		syncPressedNames();
		callbacks[pressedName]?.onmouseup?.();
	};
	const pressSwipePointer = (pointerId, buttonName) => {
		swipeRef.current[pointerId] = buttonName;
		syncPressedNames();
		const press = callbacks[buttonName]?.onmousedown;

		// LE BOUTON DESSINÉ ET LE BOUTON CÂBLÉ NE VIENNENT PAS DU MÊME ENDROIT.
		//
		// Ce qui s'affiche vient de `layout` — la disposition enregistrée du
		// joueur si elle existe, le preset sinon. Les callbacks, eux, sont
		// construits à partir du PRESET seul. Deux tables, une seule clé : le
		// nom du bouton. Quand elles divergent, le pad s'allume sous le doigt
		// et n'envoie rien, sans une erreur (2026-09-07 : « les touches ne
		// fonctionnent pas », des heures pour arriver jusqu'ici).
		if (!press) {
			logThrottled("warn", "pad-no-callback", 2000,
				"pad: bouton dessiné sans câblage", buttonName,
				`(${Object.keys(callbacks).length} câblés)`);

			return;
		}

		press();
		if (getHapticFeedback()) navigator.vibrate?.(DEFAULT_VIBERATION_DURATION);
	};
	const buttonNameAtPoint = (clientX, clientY) => {
		const hit = document
			.elementFromPoint(clientX, clientY)
			?.closest?.("[data-btn-name]");
		if (!hit || hit.dataset.btnAnalog === "true") return null;
		return hit.dataset.btnName;
	};
	const handleSwipePointerDown = (e) => {
		if (editable) return;
		// LE BOUTON QUE CHROME A CHOISI, quand le point exact n'en touche aucun : sur un écran tactile, il
		// rapproche un doigt posé à côté d'un bouton de ce bouton (l'ajustement du toucher), et l'appui
		// arrive à lui. Le seul point exact le perdait — sous le stick flottant (#754), le doigt ne
		// pressait alors ni le bouton ni le stick.
		const targeted = e.target?.closest?.("[data-btn-name]");
		const buttonName = buttonNameAtPoint(e.clientX, e.clientY)
			?? (targeted && targeted.dataset.btnAnalog !== "true" ? targeted.dataset.btnName : null);
		// PAS DE JOURNAL ICI : un doigt qui touche entre deux boutons est un
		// geste normal, pas un défaut. Ça remplissait la console à chaque
		// contact (demandé le 2026-09-10).
		if (!buttonName) {
			return;
		}
		e.preventDefault();
		try {
			e.currentTarget.setPointerCapture?.(e.pointerId);
		} catch {
			/* noop */
		}
		pressSwipePointer(e.pointerId, buttonName);
	};
	const handleSwipePointerMove = (e) => {
		if (editable) return;
		const pressedName = swipeRef.current[e.pointerId];
		if (!pressedName) return;
		const buttonName = buttonNameAtPoint(e.clientX, e.clientY);
		if (buttonName && buttonName !== pressedName) {
			releaseSwipePointer(e.pointerId);
			pressSwipePointer(e.pointerId, buttonName);
		}
	};
	const handleSwipePointerEnd = (e) => {
		if (editable) return;
		releaseSwipePointer(e.pointerId);
	};
	const height = PAD_HEIGHT;

	if (touchpadOnly) {
		const touchpad = drawn.find((b) => b.name === "TOUCHPAD");

		return touchpad && callbacks[touchpad.name]?.ontouch ? (
			<TouchpadSurface
				button={touchpad}
				callback={callbacks[touchpad.name]}
				isLandscape={touchpadOnly === "landscape"}
				alone={{ size: TOUCHPAD_ALONE_SIZE[touchpadOnly], place: TOUCHPAD_ALONE_PLACE[touchpadOnly], opacity: tunedOpacity ?? fetchedOpacity ?? opacity }}
				look={look}
			/>
		) : null;
	}

	return (
		<div
			ref={stripRef}
			className="position-absolute"
			// LA BANDE DE LA MANETTE LAISSE PASSER LES DOIGTS.
			//
			// 300 px de haut sur toute la largeur : sur un téléphone tenu à
			// l'horizontale, c'est presque tout l'écran. Elle prenait chaque
			// toucher, y compris ENTRE les boutons, et rien n'atteignait ce qui
			// est dessous — ni l'écran tactile de la DS, ni la souris de
			// streamex (relevé le 2026-09-11 : 15 points sondés dans la moitié
			// basse d'un téléphone à l'horizontale, aucun sur la vidéo).
			//
			// `pointer-events` s'hérite : seuls les boutons et le pavé tactile
			// le rallument. Le balayage d'un bouton à l'autre ne change pas —
			// l'appui part d'un bouton et remonte jusqu'au calque qui le suit.
			style={{ bottom: padLift, left: 0, width: "100%", pointerEvents: "none" }}
		>
			<div className="d-flex flex-column">
				<div
					style={{
						position: "relative",
						width: "100%",
						height: height,
						opacity: tunedOpacity ?? fetchedOpacity ?? opacity,
						// In `style`, not as a bare `touchAction` attribute:
						// React does not map that to the CSS property, it tries
						// to set a DOM attribute of that name (and warns), so
						// the rule was never applied at all. This container is
						// the one capturing the pointer for the swipe roll
						// below — without touch-action:none the browser is free
						// to claim the gesture for scrolling/zooming mid-roll.
						touchAction: "none",
					}}
					onPointerDown={handleSwipePointerDown}
					onPointerMove={handleSwipePointerMove}
					onPointerUp={handleSwipePointerEnd}
					onPointerCancel={handleSwipePointerEnd}
					onLostPointerCapture={handleSwipePointerEnd}
				>
					<div className="position-absolute top-0 start-0 h-100 d-flex align-items-center">
						<ColumnZone
							column="left"
							buttons={drawn}
							callbacks={callbacks}
							isLandscape={isLandscape}
							editable={editable}
							selectedName={selectedName}
							pressedNames={pressedNames}
							invisible={invisibleSticks}
							floatZones={floatZones}
							onSelectButton={onSelectButton}
							onButtonDrag={handleButtonDrag}
							height={height}
						/>
					</div>
					{/* zIndex 2 : sa translation en fait un contexte d'empilement, qui passait SOUS la zone du stick flottant
					    (zIndex 1) — ses boutons doivent passer devant, comme tous les boutons (#758). */}
					<div className="position-absolute bottom-0 start-50 translate-middle-x" style={{ zIndex: 2 }}>
						<ColumnZone
							column="center"
							buttons={drawn}
							callbacks={callbacks}
							isLandscape={isLandscape}
							editable={editable}
							selectedName={selectedName}
							pressedNames={pressedNames}
							invisible={invisibleSticks}
							floatZones={floatZones}
							onSelectButton={onSelectButton}
							onButtonDrag={handleButtonDrag}
							height={height}
						/>
					</div>
					<div className="position-absolute top-0 end-0 h-100 d-flex align-items-center">
						<ColumnZone
							column="right"
							buttons={drawn}
							callbacks={callbacks}
							isLandscape={isLandscape}
							editable={editable}
							selectedName={selectedName}
							pressedNames={pressedNames}
							invisible={invisibleSticks}
							floatZones={floatZones}
							onSelectButton={onSelectButton}
							onButtonDrag={handleButtonDrag}
							height={height}
						/>
					</div>
				</div>
			</div>
		</div>
	);
}
