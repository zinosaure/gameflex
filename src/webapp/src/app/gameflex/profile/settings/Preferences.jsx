import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import Skeleton from "../../../skeleton/skeleton.js";
import { SectionPanel } from "../../../gameflex/shared/templates/Essentials.jsx";
import { can } from "../../libs/capabilities.js";
import {
    HALF_SCREEN_MAX, HALF_SCREEN_MIN, getHalfScreenHeight, getScreenOrientation, setHalfScreenHeight, setScreenOrientation,
} from "../../libs/screenOrientation.js";
import { isMobileDevice, isTabletDevice } from "../../shared/utils/device-detection.js";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// PreferencesTab is a single form/section (Préférences) — it maps to a different backend concern (htmlTheme+language, 
// see users/routes.py's update_me) and there's no reason changing your name should require also re-typing your password.
// Logout lives in the left menu now (Profil section), not here — see index.js.
export default function Preferences({ context, t }) {
    const { i18n } = useTranslation();
    // L'état du thème est tenu ICI, en plus du contexte.
    //
    // `context.theme` est bien réactif (skeleton.js le tient en useState),
    // mais ce panneau vit dans un OFFCANVAS : son contenu est un élément
    // React construit une fois, à l'ouverture, et ses props sont un instantané
    // gelé. Le thème changeait donc réellement — la page basculait — sans que
    // l'interrupteur bouge, puisqu'il relisait une valeur figée. Signalé le
    // 2026-09-01 comme « pas un vrai setState » : c'était exactement ça, au
    // niveau du panneau et non du contexte.
    //
    // Même problème, même remède que FloatingPanel (voir son propre
    // commentaire) : ce qui doit bouger à l'écran a son état dans le
    // composant qui l'affiche.
    const [isDark, setDark] = useState(context.theme === "dark");
    // L'ORIENTATION DE L'ÉCRAN (#406) — même raison de tenir l'état ici :
    // ce panneau est un instantané figé à l'ouverture.
    //
    // Un contexte de formulaire, comme skeleton/examples/Form.jsx : l'orientation
    // et la hauteur du demi-écran (#410), que son ruleset ne montre que pour ce
    // choix. Chaque changement s'applique aussitôt — pas de bouton.
    const [screenValues, setScreenValues] = useState(() => ({
        screenOrientation: getScreenOrientation(),
        halfScreenHeight: getHalfScreenHeight(),
    }));
    // Dans l'application, l'activité tourne tout ; ailleurs, seul un appareil
    // qu'on tient en main a quelque chose à tourner (pendant une partie).
    const turnsTheApp = can("screenOrientation");
    const canTurn = turnsTheApp || isMobileDevice() || isTabletDevice();
    const screenRulesets = {};

    // LA HAUTEUR DU DEMI-ÉCRAN (le porteur, 2026-10-21) : de 30 à 70 % de
    // l'écran, 50 par défaut — la manette ne cache pas la même part sur tous
    // les téléphones.
    screenRulesets["halfScreenHeight"] = (data) => {
        if (data.screenOrientation !== "portrait-half") {
            return Ruleset.isHidden();
        }

        const height = Number(data.halfScreenHeight);
        if (!(height >= HALF_SCREEN_MIN && height <= HALF_SCREEN_MAX)) {
            return Ruleset.isInvalid(t("settings.halfScreenHeightInvalid", { min: HALF_SCREEN_MIN, max: HALF_SCREEN_MAX }));
        }

        return null;
    };

    const screenContext = {
        values: screenValues,
        setValues: (values) => {
            setScreenValues(values);
            if (values.screenOrientation !== screenValues.screenOrientation) {
                setScreenOrientation(values.screenOrientation).then((applied) => {
                    if (turnsTheApp && !applied) {
                        context.toast.open(t("settings.orientationFailed"), { variant: "error" });
                    }
                });
            }
            if (values.halfScreenHeight !== screenValues.halfScreenHeight) {
                // Une valeur hors bornes, en cours de frappe, n'est pas prise.
                setHalfScreenHeight(values.halfScreenHeight);
            }
        },
        rulesets: screenRulesets,
    };

    return (
        <SectionPanel>
            <Form.Group>
                <Form.SkeletonSwitch
                    reverse
                    label={t("profile.darkModeLabel")}
                    helpText={t("profile.darkModeHelp")}
                    name="htmlTheme"
                    context={{
                        values: { htmlTheme: isDark },
                        setValues: (values) => {
                            const dark = !!values.htmlTheme;
                            setDark(dark);
                            context.toggleTheme(dark ? "dark" : "light");
                        },
                    }}
                    onText={t("profile.darkModeOn")}
                    offText={t("profile.darkModeOff")}
                />
            </Form.Group>
            <Form.Group
                label={t("profile.languageLabel")}
                helpText={t("profile.languageHelp")}>
                <Skeleton.Form.Select
                    name="language"
                    context={{
                        values: { language: i18n.language },
                        setValues: (values) => {
                            const code = values.language;

                            i18n.changeLanguage(code);

                            try {
                                localStorage.setItem('lang', code);
                            } catch {
                                context.toast.open(t('language.switch_failed'), { variant: "error", duration: 3000, title: t('language.switch_failed_title') })
                            }
                        },
                    }}
                    options={context.availableLanguages}
                />
            </Form.Group>
            {/* L'ORIENTATION, POUR TOUTE L'APPLICATION (#406, le porteur,
                2026-10-19). Elle vivait dans le panneau d'une partie, pour
                cette partie seulement. Le demi-écran (#410) a son aide à lui :
                il ne fait rien tant qu'aucune manette n'est branchée. */}
            {canTurn && (
                <>
                    <Form.Group
                        label={t("settings.orientationLabel")}
                        helpText={t(screenValues.screenOrientation === "portrait-half"
                            ? "settings.orientationPortraitHalfHelp"
                            : turnsTheApp ? "settings.orientationHelpApp" : "settings.orientationHelpBrowser")}>
                        <Form.Select
                            name="screenOrientation"
                            context={screenContext}
                            options={[
                                { value: "auto", label: t("settings.orientationAuto") },
                                { value: "landscape-reverse", label: t("settings.orientationClockwise") },
                                { value: "landscape", label: t("settings.orientationAnticlockwise") },
                                { value: "portrait-half", label: t("settings.orientationPortraitHalf") },
                            ]}
                        />
                    </Form.Group>
                    <Form.Group
                        label={t("settings.halfScreenHeightLabel")}
                        helpText={t("settings.halfScreenHeightHelp")}
                        hidden={() => Ruleset(screenRulesets["halfScreenHeight"], screenValues, "halfScreenHeight").isHidden()}>
                        <Form.Range
                            name="halfScreenHeight"
                            context={screenContext}
                            min={HALF_SCREEN_MIN}
                            max={HALF_SCREEN_MAX}
                            step={1}
                        />
                    </Form.Group>
                </>
            )}
            {/* LE CACHE N'EST PLUS ICI (2026-09-09).
                Il vivait entre le thème et la langue, alors qu'un thème est
                une préférence personnelle et un plafond de cache un réglage
                de la machine. Il a rejoint « Gestion du stockage → Mémoire
                temporaire », avec les cinq autres sections du disque. */}
        </SectionPanel>
    );
}
