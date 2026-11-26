import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { usePhysicalGamepad } from "../engine/libs/gamepadPresence.js";
import { canLockOrientation, chooseOrientation } from "../libs/orientation.js";
import { getHalfScreenHeight, HALF_SCREEN_MAX, HALF_SCREEN_MIN, SCREEN_ORIENTATIONS, setHalfScreenHeight, useScreenOrientation } from "../libs/screenOrientation.js";
import { RangeRow } from "./RangeRow.jsx";
import { SelectRow } from "./SelectRow.jsx";

// L'ORIENTATION DE L'ÉCRAN (#406, #410 ; #758) : les mêmes lignes dans Réglages › Application et dans le menu d'une partie —
// UNE valeur, gardée dans ce navigateur (libs/screenOrientation.js), que lit la page de jeu. Dans l'application Android
// (`turnsTheApp`), l'activité pivote tout ; dans un navigateur, choisir un paysage allume le plein écran et verrouille
// l'écran (libs/orientation.js) — un navigateur qui ne sait pas, ou qui refuse, le dit (`onFailed`) : seule l'image d'une
// partie tourne alors (use-screen-rotation.js). Choisi pendant une partie, il s'applique tout de suite, sans la couper.
// « Portrait, demi-écran » range l'interface dans le haut de l'écran dès qu'une manette est branchée, à la hauteur réglée
// sous lui. SANS MANETTE, LE DEMI-ÉCRAN NE SE CHOISIT PAS (le porteur, 2026-11-16) : son option est grisée, et l'aide dit
// d'en brancher une ; déjà choisi, il reste choisi (la manette peut revenir).
export const ORIENTATION_LABELS = { auto: "auto", "landscape-reverse": "clockwise", landscape: "anticlockwise", "portrait-half": "portraitHalf" };

// Qui a une orientation à choisir : l'application, dont l'activité tourne tout ; ailleurs, un appareil qu'on tient en main
// (un pointeur grossier : le doigt).
export const offersOrientation = (turnsTheApp) => turnsTheApp || matchMedia("(pointer: coarse)").matches;

// DANS UNE SECTION QUI S'ENREGISTRE (Réglages › Application, SaveSection ; le porteur, 2026-11-26 : « les sections
// formulaire n'ont pas de FormButton ») : `value` (`{ orientation, height }`) et `onChange(patch)` — les lignes ne font
// que changer la valeur, le bouton l'applique (`saveOrientation`). Sans eux (le menu d'une partie), le choix s'applique
// tout de suite.
// `onApplied` : l'orientation a tourné (le menu d'une partie se referme alors).
export function OrientationRows({ idPrefix, turnsTheApp, onFailed = () => {}, onApplied = null, value = null, onChange = null }) {
    const { t } = useTranslation();
    const stored = useScreenOrientation();
    const gamepad = usePhysicalGamepad();
    const [storedHeight, setHeight] = useState(getHalfScreenHeight);
    const orientation = value ? value.orientation : stored;
    const height = value ? value.height : storedHeight;
    const help = orientation === "portrait-half" ? (gamepad ? "portraitHalf" : "portraitHalfWaiting") : !gamepad ? "noGamepad"
        : turnsTheApp ? "app" : canLockOrientation() ? "browser" : "browserCannot";
    const choose = (mode) => {
        if (onChange) {
            onChange({ orientation: mode });

            return;
        }
        onFailed(false);
        chooseOrientation(mode, turnsTheApp).then((applied) => {
            onFailed(!applied);

            if (applied) {
                onApplied?.();
            }
        });
    };
    const resize = (next) => {
        if (onChange) {
            onChange({ height: Number(next) });

            return;
        }
        // Une valeur hors bornes (un chiffre en cours de frappe) n'est ni gardée ni appliquée.
        if (setHalfScreenHeight(next)) {
            setHeight(Number(next));
        }
    };

    return (
        <>
            <SelectRow id={`${idPrefix}-orientation`} label={t("settings.items.orientation.label")} help={t(`settings.orientationHelp.${help}`)} value={orientation} onChange={choose}
                options={SCREEN_ORIENTATIONS.map((mode) => ({ value: mode, label: t(`settings.orientations.${ORIENTATION_LABELS[mode]}`),
                    attributes: mode === "portrait-half" && !gamepad && orientation !== mode ? { disabled: true } : {} }))} />
            {orientation === "portrait-half" && (
                <RangeRow id={`${idPrefix}-half-height`} label={t("settings.items.halfScreenHeight.label")} help={t("settings.items.halfScreenHeight.help", { min: HALF_SCREEN_MIN, max: HALF_SCREEN_MAX })}
                    value={height} min={HALF_SCREEN_MIN} max={HALF_SCREEN_MAX} onChange={resize} />
            )}
        </>
    );
}

// LA VALEUR GARDÉE, pour une section qui s'enregistre : `{ orientation, height }`.
export const savedOrientation = (orientation) => ({ orientation, height: getHalfScreenHeight() });

// APPLIQUER LE CHOIX D'UNE SECTION, à son bouton (dans le geste du clic : le plein écran d'un paysage le demande) —
// `{ ok, applied }` : `ok` faux pour une hauteur hors bornes, `applied` faux quand le navigateur n'a pas tourné l'écran.
export async function saveOrientation({ orientation, height }, turnsTheApp) {
    if (orientation === "portrait-half" && !setHalfScreenHeight(height)) {
        return { ok: false, applied: true };
    }

    return { ok: true, applied: await chooseOrientation(orientation, turnsTheApp) };
}

