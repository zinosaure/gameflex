// Route : /settings/logs/settings — les réglages des journaux, sur leur propre route (le porteur, 2026-11-25 : « dans logs
// j'avais pas fait en sorte que les réglages soient dans une autre route. Faudra le faire »). Un hôte qui ne les sert pas
// n'en a pas l'entrée (la capacité `logSettings`).
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { fetchLogSettings, saveLogSettings } from "../../libs/api.js";
import SettingsPage from "../SettingsPage.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Note } from "../../bricks/Note.jsx";
import { SaveSection } from "../../bricks/SaveSection.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SwitchRow } from "../../bricks/SwitchRow.jsx";
import { LOGS } from "../Logs.jsx";

// Les niveaux, un interrupteur chacun, dans l'ordre du kernel (gameflex/logfile, LevelNames).
const LEVELS = ["debug", "info", "warn", "error"];

// CE QUI S'ÉCRIT (le porteur, 2026-11-14 : « Il faut que dans logs on puisse avoir la possibilité de l'activer ou pas et
// aussi choisir les level à afficher si on active le log. ») — deux interrupteurs, enregistrés par le bouton de leur section, le kernel les
// suivant dès sa ligne suivante (log_settings.go) : le journal du kernel, et, allumé, ses niveaux et les requêtes HTTP à
// part, quels que soient les niveaux allumés ; les journaux des parties, sans niveaux (la sortie d'un émulateur n'en a pas).
// LES SIGNALEMENTS (reports.log), leur interrupteur aussi (le porteur, 2026-11-14 : « Oui pour le reports.log ») :
// éteints, l'écran d'erreur d'un lancement ne propose plus « Signaler l'erreur » (la capacité `reports`).
// LES NIVEAUX, UN INTERRUPTEUR CHACUN (le porteur, 2026-11-14 : « change les checkbox en switch pour le choix des niveaux
// de log »), dans leur section, avec les requêtes HTTP — toujours là, grisés quand le journal du kernel est éteint (le
// porteur : « Au lieu de faire disparaitre le level to write, le mieux ce serait de mettre les switch en disabled et
// enabled quand c'est activé »).
// Les fichiers seulement : la sortie standard du kernel ne change pas. Grisé tant qu'il se lit ; chaque section part par son
// bouton, la réponse en fenêtre. Un hôte qui ne le sert pas ne le montre pas (`logSettings`).
// CE QUE L'HÔTE RÈGLE, ET RIEN D'AUTRE : une ligne pour chaque réglage que sa réponse porte — le téléphone a le journal
// de son kernel, ses niveaux et ses requêtes HTTP (`{kernel, levels, http}`, mobileapp/kernel/logging.go, 2026-11-17),
// pas les journaux des parties ni les signalements. Pendant la lecture, toutes, grisées.
function LogWriting() {
    const { t } = useTranslation();
    const [settings, setSettings] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchLogSettings().then((answer) => !cancelled && setSettings(answer.ok ? answer.data : null));

        return () => {
            cancelled = true;
        };
    }, []);

    // UN BOUTON PAR SECTION (le porteur, 2026-11-25 : « Faudra rajouter des boutons pour tous les formulaires qui n'en
    // ont pas ») : chaque section envoie ses réglages, la réponse en fenêtre (SaveSection) ; le kernel rend tout.
    const save = async (patch) => {
        const answer = await saveLogSettings(patch);

        if (answer.ok) {
            setSettings((current) => ({ ...current, ...(answer.data || patch) }));
        }

        return answer;
    };

    const known = Boolean(settings);
    const offers = (name) => !known || name in settings;
    const writing = Object.fromEntries(["kernel", "sessions", "reports"].filter((name) => known && name in settings).map((name) => [name, settings[name]]));
    const failed = settings === null && <Note className="mt-0">{t("logs.loadFailed")}</Note>;

    return (
        <>
            {failed ? <Section title={t("logs.writingTitle")} help={t("logs.writingHelp")} sense="modify">{failed}</Section> : (
                <SaveSection id="lg-writing" title={t("logs.writingTitle")} help={t("logs.writingHelp")} saved={writing} disabled={!known} onSave={save}>
                    {(values, set) => (
                        <>
                            <SwitchRow id="lg-kernel" label={t("logs.kernel")} texts="enabled" help={t("logs.kernelHelp")} checked={Boolean(values.kernel)} disabled={!known}
                                onChange={(kernel) => set({ kernel })} />
                            {offers("sessions") && (
                                <SwitchRow id="lg-sessions" label={t("logs.sessions")} texts="enabled" help={t("logs.sessionsHelp")} checked={Boolean(values.sessions)} disabled={!known}
                                    onChange={(sessions) => set({ sessions })} />
                            )}
                            {offers("reports") && (
                                <SwitchRow id="lg-reports" label={t("logs.reports")} texts="enabled" help={t("logs.reportsHelp")} checked={Boolean(values.reports)} disabled={!known}
                                    onChange={(reports) => set({ reports })} />
                            )}
                        </>
                    )}
                </SaveSection>
            )}
            {/* Les niveaux suivent le journal du kernel ENREGISTRÉ : éteint, ils restent là, grisés (jamais cachés). */}
            {known && "levels" in settings && (
                <SaveSection id="lg-levels" title={t("logs.levels")} help={t("logs.levelsHelp")} saved={{ levels: settings.levels || [], http: Boolean(settings.http) }}
                    disabled={!settings.kernel} onSave={save}>
                    {(values, set) => (
                        <>
                            {LEVELS.map((level) => (
                                <SwitchRow key={level} id={`lg-level-${level}`} label={t(`logs.level.${level}`)} texts="enabled" checked={values.levels.includes(level)} disabled={!settings.kernel}
                                    onChange={(on) => set({ levels: LEVELS.filter((other) => (other === level ? on : values.levels.includes(other))) })} />
                            ))}
                            <SwitchRow id="lg-http" label={t("logs.http")} texts="enabled" help={t("logs.httpHelp")} checked={values.http} disabled={!settings.kernel} onChange={(http) => set({ http })} />
                        </>
                    )}
                </SaveSection>
            )}
        </>
    );
}

export default function LogsSettings() {
    const { t } = useTranslation();

    // BAS depuis Retour : le premier interrupteur, que l'alignement de la croix sauterait (il est au bord droit, Retour au
    // bord gauche).
    return (
        <SettingsPage title={t("logs.settingsTitle")} parent={LOGS} enter="lg-kernel">
            <LogWriting />
            <GuideSection guides={[["storage", "logs"], ["troubleshooting", "find-out-what-happened"]]} />
        </SettingsPage>
    );
}
