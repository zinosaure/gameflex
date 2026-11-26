// Route : /settings/consoles/<console> — une console.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { SaveSection } from "../../bricks/SaveSection.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SwitchRow } from "../../bricks/SwitchRow.jsx";
import { consoleDefaults, listEmulatorParams, setConsoleSwitch } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { counted } from "../../libs/format.js";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../SettingsPage.jsx";
import { consolePath, loadSection, paramsLabel, paramsPath, useEmulated } from "../Consoles.jsx";

// /settings/consoles/<console> : ses émulateurs et ses BIOS, chacun sa lecture ; celui en service en couleur ; puis
// « Plus » — ses jeux (Games.jsx), ses paramètres (un par émulateur, grisés hors Streamex : seuls les
// émulateurs de bureau en ont), son apparence.
export default function Console() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const { consoles, countOf, markConsoleDisabled, capabilities } = useData();
    const { open } = useSettingsNavigation();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const registered = (consoles || []).find((item) => item.id === consoleId);
    // COUPER LA CONSOLE POUR TOUT LE SERVEUR (chantier 66) : personne ne la lance plus, ni d'ici ni par la fédération.
    const switchConsole = async ({ enabled }) => {
        const answer = await setConsoleSwitch(consoleId, enabled);

        if (answer.ok) {
            markConsoleDisabled(consoleId, !enabled);
        }

        return answer;
    };
    const [cores, setCores] = useState(undefined);
    const [bios, setBios] = useState(undefined);
    const [inUse, setInUse] = useState(null);
    const [params, setParams] = useState([]);
    // DEUX LISTES D'ÉMULATEURS (le porteur, 2026-11-25 : « scinder en 2 sections les émulateurs : "Emulators" et "Mobile
    // emulators". Puis par rapport à la capacité, tu grises la section qui ne matche pas ») : ceux de ce serveur (et les
    // cœurs du rétro), puis les paquets du téléphone (`package: "phone"`, une console récente seulement) ; celle que cet
    // hôte ne fait pas tourner (`platform` : android, le téléphone) est désactivée, sa raison dans son aide.
    const onPhone = capabilities?.emulatorPackage === "phone";
    const serverCores = cores === undefined || cores === null ? cores : cores.filter((item) => item.package !== "phone");
    const phoneCores = cores === undefined || cores === null ? cores : cores.filter((item) => item.package === "phone");
    const streamex = entry?.method === "streamex";
    const count = countOf(consoleId);

    useEffect(() => {
        let cancelled = false;
        const keep = (setter) => (value) => !cancelled && setter(value);

        setCores(undefined);
        setBios(undefined);
        loadSection("cores", consoleId).then(keep(setCores));
        loadSection("bios", consoleId).then(keep(setBios));
        consoleDefaults(consoleId).then((answer) => keep(setInUse)(answer.ok ? answer.data?.inUse || {} : {}));
        listEmulatorParams().then((answer) => keep(setParams)(answer.ok ? (answer.data?.emulators || []).filter((emulator) => emulator.consoleId === consoleId).sort((a, b) => String(a.system).localeCompare(String(b.system))) : []));

        return () => {
            cancelled = true;
        };
    }, [consoleId]);

    return (
        <SettingsPage title={entry?.name || consoleId} parent="/settings/consoles">
            {consoles && !entry ? <NotFound /> : !entry ? <Section><Loading /></Section> : (
                <>
                    {/* « UN INTERRUPTEUR POUR COUPER LA CONSOLE DIRECTEMENT » (le porteur, 2026-11-19, chantier 66) : pour tout le
                        serveur — ses joueurs, administrateur compris, et ceux de la fédération. Son bouton Jouer se grise
                        partout (« Console désactivée »). */}
                    <SaveSection id={`cs-switch-${consoleId}`} title={t("consoles.switchTitle")} saved={{ enabled: !registered?.disabled }} disabled={!registered}
                        onSave={switchConsole} success={({ enabled }) => t(enabled ? "consoles.switchedOn" : "consoles.switchedOff", { name: entry?.name || consoleId })}>
                        {(values, set) => (
                            <SwitchRow id={`cs-switch-${consoleId}-on`} label={t("consoles.switchLabel")} help={t("consoles.switchHelp")}
                                checked={values.enabled} disabled={!registered} onChange={(enabled) => set({ enabled })} />
                        )}
                    </SaveSection>
                    {/* SES ÉMULATEURS ET SES BIOS, CHACUN SA PAGE (le porteur, 2026-11-25 : « ce serait pas mieux de faire emulators,
                        mobile emulators et bios passer dans des routes spécifiques ? Au lieu de mettre les 3 listing sur une
                        seule page ») : une ligne chacun, son nombre en pastille, celui en service dessous. */}
                    <Section title={t("consoles.sections.emulators")} help={t("consoles.sections.emulatorsHelp")} sense="modify">
                        <Rows>
                            <NavRow id="cs-cores" icon="bi-cpu" label={t("consoles.emulators")} value={serverCores ? counted(t, "emulators", serverCores.length) : null}
                                help={onPhone ? t("consoles.emulatorsNotHere") : t("consoles.emulatorsRowHelp")} onOpen={() => open(`${consolePath(consoleId)}/cores`, "cs-cores")} />
                            {streamex && <NavRow id="cs-mobile-cores" icon="bi-phone" label={t("consoles.mobileEmulators")} value={phoneCores ? counted(t, "emulators", phoneCores.length) : null}
                                help={!onPhone ? t("consoles.mobileEmulatorsNotHere") : t("consoles.mobileEmulatorsRowHelp")} onOpen={() => open(`${consolePath(consoleId)}/mobile-cores`, "cs-mobile-cores")} />}
                            <NavRow id="cs-bios" icon="bi-fingerprint" label={t("consoles.bios")} value={bios ? counted(t, "files", bios.length) : null}
                                help={bios && !bios.length ? t("consoles.noBios") : t("consoles.biosRowHelp")} onOpen={() => open(`${consolePath(consoleId)}/bios`, "cs-bios")} />
                        </Rows>
                    </Section>
                    <Section title={t("consoles.sections.games")} help={t("consoles.sections.gamesHelp")} sense="play">
                        <Rows>
                            <NavRow id="cs-games" icon="bi-collection-play" label={t("consoles.games")} value={counted(t, "games", count)}
                                onOpen={() => open(`${consolePath(consoleId)}/games`, "cs-games")} />
                        </Rows>
                    </Section>
                    <Section title={t("consoles.sections.appearance")} help={t("consoles.sections.appearanceHelp")} sense="modify">
                        <Rows>
                            <NavRow id="cs-appearance" icon="bi-palette" label={t("consoles.appearance")} help={t("consoles.appearanceHelp")}
                                onOpen={() => open(`${consolePath(consoleId)}/appearance`, "cs-appearance")} />
                            <NavRow id="cs-status" icon="bi-patch-check" label={t("consoles.status")} help={t("consoles.statusRowHelp")}
                                onOpen={() => open(`${consolePath(consoleId)}/status`, "cs-status")} />
                        </Rows>
                    </Section>
                    <Section title={t("consoles.sections.settings")} help={t("consoles.paramsHelp")} sense="modify">
                        <Rows>
                            {streamex ? params.map((emulator) => (
                                <NavRow key={emulator.emulator} id={`cs-params-${emulator.emulator}`} icon="bi-sliders" label={paramsLabel(emulator, t)}
                                    onOpen={() => open(paramsPath(consoleId, emulator.emulator), `cs-params-${emulator.emulator}`)} />
                            )) : <NavRow id="cs-params" icon="bi-sliders" label={t("consoles.tabParams")} help={t("consoles.paramsStreamexOnly")} opens="info" disabled />}
                        </Rows>
                    </Section>
                    <Section title={t("more")} help={t("consoles.moreHelp")} sense="optional">
                        <Rows>
                            <NavRow id="cs-orphans" icon="bi-file-earmark-x" label={t("consoles.orphans")} help={t("consoles.orphansRowHelp")}
                                onOpen={() => open(`${consolePath(consoleId)}/metadata-orphans`, "cs-orphans")} />
                        </Rows>
                    </Section>
                    <GuideSection guides={[["consoles"], ["library", "3-get-the-emulators"]]} />
                </>
            )}
        </SettingsPage>
    );
}
