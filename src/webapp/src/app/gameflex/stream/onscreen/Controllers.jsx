import React, { useEffect, useRef, useState } from "react";
import { useAuth } from "../../profile/auth/AuthContext.jsx";
import { listButtonPositioning } from "../../profile/api.js";
import { streamAuthToken } from "../../libs/serverBase.js";
import { getHapticFeedback } from "../../libs/gamepadPreferences.js";
import { logThrottled } from "../../shared/utils/log-store.js";
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
	onSelect,
	onDrag,
}) {
	const analog = button.analog || false;
	const diagonal = button.diagonal || false;

	// State for tracking dragging and offset for analog sticks
	const [dragging, setDragging] = useState(false);
	const [offset, setOffset] = useState({ x: 0, y: 0 });
	const size = isLandscape ? button.xl.size : button.size;
	const ringRadius = size * 0.6;
	const thumbSize = size * 0.6;
	const updateFromEvent = (e) => {
		const el = e.currentTarget;
		const rect = el.getBoundingClientRect();
		const cx = rect.left + rect.width / 2;
		const cy = rect.top + rect.height / 2;
		const radius = rect.width / 2;
		let dx = (e.clientX - cx) / radius;
		let dy = (e.clientY - cy) / radius;
		const mag = Math.hypot(dx, dy);

		if (mag > 1) {
			dx /= mag;
			dy /= mag;
		}

		setOffset({ x: dx, y: dy });
		callback.onmove?.(dx, dy);
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

	return (
		<div
			title={button.text || button.name}
			data-btn-name={button.name}
			data-btn-analog={analog ? "true" : undefined}
			style={{
				position: "absolute",
				left: `${isLandscape ? button.xl.x * 100 : button.x * 100}%`,
				top: `${isLandscape ? button.xl.y * 100 : button.y * 100}%`,
				width: isLandscape ? button.xl.size : button.size,
				height: isLandscape ? button.xl.size : button.size,
				transform: "translate(-50%, -50%)",
				display: "flex",
				alignItems: "center",
				justifyContent: "center",
				cursor: "grab",
				userSelect: "none",
				touchAction: "none",
				// La bande qui le porte ne reçoit rien (voir ControllerLayout) :
				// le bouton, lui, doit.
				pointerEvents: "auto",
				borderRadius: "50%",
				zIndex: editable && isSelected ? 3 : 2,
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
				if (analog) {
					// Sticks keep their own press/release (thumb tracking) — the
					// container swipe layer skips them via data-btn-analog.
					e.stopPropagation();
					callback.onmousedown?.();
					return;
				}
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
				if (!editable && !analog) return; // the container swipe layer released it
				e.stopPropagation();
				e.preventDefault();
				if (editable) {
					setDragging(false);
					return;
				}
				callback.onmouseup?.();
			}}
			onPointerCancel={() => {
				if (editable) setDragging(false);
				else if (analog) callback.onmouseup?.();
			}}
		>
			<div
				style={{
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
						borderRadius: "50%",
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
					// editable (ButtonPositioning): same ring+thumb VISUAL as a
					// real session (still centered, offset {0,0} — nothing
					// to deflect while just positioning it), but with no
					// interactive handlers of its own — pointerEvents: none
					// lets the drag pass straight through to the outer
					// PlaceButton div, which repositions the whole stick
					// (button.x/y) instead of deflecting its thumb.
					<div
						onPointerDown={
							editable
								? undefined
								: (e) => {
									e.preventDefault();
									e.stopPropagation();

									try {
										e.target.setPointerCapture?.(e.pointerId);
									} catch {
										/* noop */
									}

									if (getHapticFeedback()) navigator.vibrate?.(DEFAULT_VIBERATION_DURATION);
									setDragging(true);
									updateFromEvent(e);
								}
						}
						onPointerMove={
							editable
								? undefined
								: (e) => {
									if (!dragging) return;
									e.preventDefault();
									updateFromEvent(e);
								}
						}
						onPointerUp={
							editable
								? undefined
								: (e) => {
									if (!dragging) {
										return;
									}

									e.preventDefault();
									setDragging(false);
									setOffset({ x: 0, y: 0 });
									callback.onmove?.(0, 0);
									if (getHapticFeedback()) navigator.vibrate?.(DEFAULT_VIBERATION_DURATION);
								}
						}
						onPointerCancel={
							editable
								? undefined
								: (e) => {
									if (!dragging) {
										return;
									}

									e.preventDefault();
									setDragging(false);
									setOffset({ x: 0, y: 0 });
									callback.onmove?.(0, 0);
								}
						}
						style={{
							position: "absolute",
							left: "50%",
							top: "50%",
							width: size,
							height: size,
							transform: "translate(-50%, -50%)",
							pointerEvents: editable ? "none" : "auto",
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
								background: dragging
									? "rgba(100, 100, 100,0.75)"
									: "rgba(100, 100, 100,0.35)",
								pointerEvents: "none",
							}}
						/>
					</div>
				) : (
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
}

// LA LARGEUR D'UNE COLONNE DE LA MANETTE À L'ÉCRAN : les boutons s'y placent
// en fraction, quelle que soit la disposition — celle des préréglages ou
// celle qu'un joueur a enregistrée. Le bouton du menu de la page de jeu
// s'aligne dessus en paysage (use-streaming-menu.js).
export const PAD_COLUMN_WIDTH = 160;

// LA MANETTE ET LE BOUTON DU MENU, REMONTÉS EN PORTRAIT (le porteur,
// 2026-10-23 : « remonter les boutons et le menu d'au moins 60 px ; les sticks
// sont trop collés au bord du bas du téléphone ») — le geste de retour et la
// barre de navigation d'Android sont là. Pas en paysage : la bande de la
// manette y fait 300 px sur un écran d'environ 400, et ses gâchettes du haut
// sortiraient. Écrit une fois, pour la manette et pour le bouton du menu
// (use-streaming-menu.js).
export const PORTRAIT_LIFT = 60;

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
	onSelectButton,
	onButtonDrag,
	height = 300,
}) {
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
	opacity = 0.9,
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
	const [isLandscape, setIsLandscape] = useState(
		window.innerWidth > window.innerHeight,
	);
	const { token } = useAuth();
	const layout = layoutOverride || internalLayout;
	const drawn = canDrawButton ? layout.filter((b) => canDrawButton(b.name)) : layout;

	// ButtonPositioning (Phase 9, profile/ButtonPositioning.jsx) — a user's saved
	// DEFAULT slot for this virtual controller preset, if any, takes
	// priority over the hard-coded CONTROLLER_PRESETS layout below. Falls
	// back to (and stays on) the preset's own layout for a logged-out
	// caller or a user who never saved a positioning for this preset — this
	// is an opt-in override, not a required setup step, same pattern as
	// KeyMapping (gameflex/libs/keyMapping.js). Skipped entirely when
	// layoutOverride is provided — the editor is the one deciding what's
	// loaded/edited in that case.
	useEffect(() => {
		if (layoutOverride) return undefined;
		setInternalLayout(CONTROLLER_PRESETS[name].buttons);
		setFetchedOpacity(null);
		if (!token) return undefined;
		let cancelled = false;
		// LE JETON DE L'HÔTE QUAND LA PAGE EN VISE UN.
		//
		// La disposition part chez celui qui tient la partie — c'est là que
		// va cette requête (apiUrl suit la surcharge de la page). Y présenter
		// NOTRE jeton ne pouvait que revenir 401, en plus de l'envoyer à une
		// machine que ça ne regarde pas (lu dans la console d'un téléphone le
		// 2026-09-07). Sans compte là-bas, la disposition enregistrée du
		// preset s'applique, ce qui est déjà le repli prévu.
		listButtonPositioning(streamAuthToken() || token, name).then((result) => {
			if (cancelled || !result.ok) return;
			const defaultSlot = (result.items || []).find((s) => s.is_default);
			if (defaultSlot) {
				setInternalLayout(mergeSavedLayout(CONTROLLER_PRESETS[name].buttons, defaultSlot.layout));
				if (typeof defaultSlot.opacity === "number")
					setFetchedOpacity(defaultSlot.opacity);
			}
		});
		return () => {
			cancelled = true;
		};
	}, [name, token, layoutOverride]);

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
		const buttonName = buttonNameAtPoint(e.clientX, e.clientY);
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
	const height = 300;

	return (
		<div
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
			style={{ bottom: 20 + (isLandscape ? 0 : PORTRAIT_LIFT), left: 0, width: "100%", pointerEvents: "none" }}
		>
			<div className="d-flex flex-column">
				<div
					style={{
						position: "relative",
						width: "100%",
						height: height,
						opacity: fetchedOpacity ?? opacity,
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
							onSelectButton={onSelectButton}
							onButtonDrag={handleButtonDrag}
							height={height}
						/>
					</div>
					<div className="position-absolute bottom-0 start-50 translate-middle-x">
						<ColumnZone
							column="center"
							buttons={drawn}
							callbacks={callbacks}
							isLandscape={isLandscape}
							editable={editable}
							selectedName={selectedName}
							pressedNames={pressedNames}
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
