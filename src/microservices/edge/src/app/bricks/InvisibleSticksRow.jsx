import React from "react";
import { useTranslation } from "react-i18next";
import { setInvisibleSticks, useGamepadPreferences } from "../engine/libs/gamepadPreferences.js";
import { SwitchRow } from "./SwitchRow.jsx";

// LES STICKS INVISIBLES (#758) : la même ligne dans Réglages › Manettes et dans le menu d'une partie — un seul texte, une
// seule valeur (engine/libs/gamepadPreferences.js), gardée dans ce navigateur. La manette à l'écran la suit aussitôt, partie
// en cours comprise. `disabled` : grisée, jamais cachée, quand la manette à l'écran est coupée (Menu.jsx).
// Dans une section qui s'enregistre (Réglages › Manettes, SaveSection) : `checked` et `onChange` viennent d'elle, et son
// bouton l'applique ; sans eux (le menu d'une partie), le choix s'applique tout de suite.
export function InvisibleSticksRow({ id, disabled = false, checked = null, onChange = null }) {
    const { t } = useTranslation();
    const { invisibleSticks } = useGamepadPreferences();

    return <SwitchRow id={id} label={t("gamepads.invisibleSticks")} help={t("gamepads.invisibleSticksHelp")} checked={checked ?? invisibleSticks} disabled={disabled} onChange={onChange || setInvisibleSticks} />;
}
