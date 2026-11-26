import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { can, useData } from "../libs/data.jsx";
import { setLanguage } from "../i18n.js";
import { setSoundsEnabled, soundsEnabled } from "../libs/sounds.js";
import { PushSection, usePush } from "./application/Push.jsx";
import SettingsPage, { usePageText } from "./SettingsPage.jsx";
import { EmptyState } from "../bricks/EmptyState.jsx";
import { OrientationFailed } from "../bricks/OrientationFailed.jsx";
import { offersOrientation, OrientationRows, savedOrientation, saveOrientation } from "../bricks/OrientationRows.jsx";
import { useScreenOrientation } from "../libs/screenOrientation.js";
import { SaveSection } from "../bricks/SaveSection.jsx";
import { Section } from "../bricks/Section.jsx";
import { SelectRow } from "../bricks/SelectRow.jsx";
import { SwitchRow } from "../bricks/SwitchRow.jsx";
import { fact } from "../libs/format.js";

// APPLICATION (Settings.jsx en fait l'entrée et la route) : une seule page. Les Manettes ont leur entrée
// (Gamepads.jsx), leur disposition sa page (Layout.jsx) ; les notifications de cet appareil viennent de Push.jsx.
// Ce qui manque encore dans une page le dit dans la page même ; aucun faux contrôle. En direction C
// (bricks/) : des groupes titrés, leurs lignes, le bouton d'un formulaire dans l'encadré de son groupe (SubmitRow),
// un geste seul en ligne.

// LE PLEIN ÉCRAN : l'API Fullscreen, préfixée webkit en repli.
const fullscreenElement = () => document.fullscreenElement || document.webkitFullscreenElement || null;

const enterFullscreen = async () => {
    const root = document.documentElement;
    const request = root.requestFullscreen || root.webkitRequestFullscreen;

    if (!fullscreenElement() && request) {
        await request.call(root);
    }
};

const toggleFullscreen = async () => {
    try {
        if (fullscreenElement()) {
            await (document.exitFullscreen || document.webkitExitFullscreen)?.call(document);
        } else {
            await enterFullscreen();
        }
    } catch {
        // Un navigateur qui refuse : l'état affiché reste le vrai (fullscreenchange ne vient pas).
    }
};

function useFullscreen() {
    const [on, setOn] = useState(() => Boolean(fullscreenElement()));

    useEffect(() => {
        const update = () => setOn(Boolean(fullscreenElement()));
        document.addEventListener("fullscreenchange", update);
        document.addEventListener("webkitfullscreenchange", update);

        return () => {
            document.removeEventListener("fullscreenchange", update);
            document.removeEventListener("webkitfullscreenchange", update);
        };
    }, []);

    return on;
}

// LES SERVEURS FÉDÉRÉS DONT CE COMPTE VOIT LES JEUX (le porteur, 2026-11-16 : « Je demandais à ce qu'on voit la liste
// des serveurs fédérés en format switch pour ensuite choisir quel serveur on veut voir ou pas les jeux. […] Ça fait que
// chaque utilisateur choisit quel serveur fédéré il a envie de voir les jeux. ») : un interrupteur par serveur fédéré de
// ce GameFlex (`GET /servers`, son nom affiché), allumé par défaut — un serveur ajouté plus tard aussi. Éteint, ses jeux
// quittent les bibliothèques et leurs nombres (data.jsx, `shownServers`) ; le Catalogue et les Réglages des consoles les
// montrent toujours. Les interrupteurs s'enregistrent par le bouton de la section, sous le COMPTE (le service des
// comptes, hidden_servers.go) : le choix suit le joueur d'un appareil à l'autre ; la réponse en fenêtre.
// Sans serveur, la boîte reste, et sa ligne grisée le dit.
function FederatedServers() {
    const { t } = useTranslation();
    const { servers, hiddenServers, setServerShown } = useData();
    const saved = Object.fromEntries(servers.map((server) => [server.id, !hiddenServers.has(String(server.id))]));
    // PAR LE BOUTON DE LA SECTION (le porteur, 2026-11-25 : « Faudra rajouter des boutons pour tous les formulaires qui
    // n'en ont pas ») : chaque serveur changé part, le premier refus s'arrête là ; la réponse en fenêtre (SaveSection).
    const save = async (values) => {
        for (const server of servers) {
            if (values[server.id] !== saved[server.id]) {
                const answer = await setServerShown(server.id, values[server.id]);

                if (!answer.ok) {
                    return answer;
                }
            }
        }

        return { ok: true };
    };

    return servers.length ? (
        <SaveSection id="ap-servers" title={t("settings.federatedServersTitle")} help={t("settings.federatedServersHelp")} saved={saved} onSave={save}
            list={{
                items: servers, textOf: (server) => `${server.hostname || ""} ${server.link}`, placeholder: t("fed.searchServer"),
                render: (server, values, set) => (
                    <SwitchRow key={server.id} id={`ap-server-${server.id}`} label={server.hostname || server.link} texts="shown" help={server.hostname ? fact(t("fed.urlLabel"), server.link) : null}
                        checked={values[server.id]} onChange={(on) => set({ [server.id]: on })} />
                ),
            }} />
    ) : (
        <Section title={t("settings.federatedServersTitle")} help={t("settings.federatedServersHelp")} sense="modify">
            <EmptyState icon="bi-hdd-network" text={t("settings.federatedServersNone")} />
        </Section>
    );
}

// L'ORIENTATION, POUR TOUT L'APPAREIL (#406, #410 ; chantier 52 P7) : les lignes de bricks/ (OrientationRows), les
// mêmes que dans le menu d'une partie (#758). Ce sont des lignes du groupe « Apparence » (le porteur, 2026-11-14 :
// « language et orientation doivent être dans la même section "appearance" »).

// /settings/application (09-application des modèles) : en tuiles, ce qui s'allume et s'éteint — les sons, le
// plein écran, les consoles sans jeu, les notifications de cet appareil ; puis « Apparence » — la langue et
// l'orientation, dans le même groupe (le porteur, 2026-11-14) —, les serveurs fédérés dont on voit les jeux, et
// l'autorisation des notifications. Les Manettes n'y sont plus : une entrée des Réglages à elle (le porteur,
// 2026-11-13).
export function ApplicationPage() {
    const { t, i18n } = useTranslation();
    const page = usePageText("application");
    const { capabilities, showEmpty, setShowEmpty } = useData();
    const [sounds, setSounds] = useState(soundsEnabled);
    const [turnFailed, setTurnFailed] = useState(false);
    const fullscreen = useFullscreen();
    const orientation = useScreenOrientation();
    const turnsTheApp = can(capabilities, "screenOrientation");
    const notifies = can(capabilities, "accounts") && can(capabilities, "pushNotifications");
    const push = usePush(notifies);

    return (
        <SettingsPage title={page.title} guides={[["controllers"], ["remoteAccess", "keep-it-safe"]]}>
            {/* LES TUILES DEVIENNENT DES INTERRUPTEURS (le porteur, 2026-11-25 : « dans settings/application, enlève les tuiles
                pour les mettre en skeletonSwitch dans une section ») : les sons, le plein écran (dans un navigateur :
                l'application Android l'occupe déjà, l'hôte le dit), les consoles sans jeu (gardé dans ce navigateur,
                data.jsx), les notifications de cet appareil (bloquées par le navigateur : désactivé).
                CHAQUE SECTION A SON BOUTON (le porteur, 2026-11-26 : « les sections formulaire n'ont pas de FormButton ») :
                rien ne s'applique au changement d'un interrupteur ou d'un sélecteur ; le bouton applique, et répond en
                fenêtre (SaveSection). Le plein écran et l'orientation d'abord : il leur faut le geste du clic. */}
            <SaveSection id="ap-prefs" title={t("settings.prefsTitle")} help={t("settings.prefsHelp")} saved={{ sounds, fullscreen, showEmpty, push: push.device === true }}
                onSave={async (values) => {
                    if (values.fullscreen !== fullscreen) {
                        await toggleFullscreen();
                    }
                    setSoundsEnabled(values.sounds);
                    setSounds(values.sounds);
                    setShowEmpty(values.showEmpty);

                    return values.push !== (push.device === true) ? push.toggle(values.push) : { ok: true };
                }}>
                {(values, set) => (
                    <>
                        <SwitchRow id="ap-sounds" label={t("settings.items.sounds.label")} texts="enabled" help={t("settings.soundsHelp")} checked={values.sounds} onChange={(on) => set({ sounds: on })} />
                        {can(capabilities, "fullscreen") && <SwitchRow id="ap-fullscreen" label={t("settings.items.fullscreen.label")} texts="fullscreen" help={t("settings.fullscreenHelp")} checked={values.fullscreen} onChange={(on) => set({ fullscreen: on })} />}
                        <SwitchRow id="ap-show-empty" label={t("settings.showEmpty")} help={t("settings.showEmptyHelp")} checked={values.showEmpty} onChange={(on) => set({ showEmpty: on })} />
                        {notifies && push.device !== null && <SwitchRow id="ap-push" label={t("settings.items.push.label")} texts="enabled" help={push.blocked ? t("settings.pushBlocked") : t("settings.items.push.help")}
                            checked={values.push} disabled={push.blocked} onChange={(on) => set({ push: on })} />}
                    </>
                )}
            </SaveSection>
            <SaveSection id="ap-appearance" title={t("settings.items.appearance.label")} help={t("settings.appearanceHelp")} saved={{ language: i18n.language, ...savedOrientation(orientation) }}
                after={turnFailed && <OrientationFailed turnsTheApp={turnsTheApp} />}
                onSave={async (values) => {
                    const turned = offersOrientation(turnsTheApp) ? await saveOrientation(values, turnsTheApp) : { ok: true, applied: true };
                    setTurnFailed(!turned.applied);
                    setLanguage(values.language);

                    return turned;
                }}>
                {(values, set) => (
                    <>
                        <SelectRow id="ap-language" label={t("settings.items.language.label")} help={t("settings.items.language.help")} value={values.language} onChange={(language) => set({ language })}
                            options={["fr", "en"].map((language) => ({ value: language, label: t(`settings.languages.${language}`) }))} />
                        {offersOrientation(turnsTheApp) && <OrientationRows idPrefix="ap" turnsTheApp={turnsTheApp} value={values} onChange={set} />}
                    </>
                )}
            </SaveSection>
            <FederatedServers />
            {notifies && <PushSection push={push} />}
        </SettingsPage>
    );
}
