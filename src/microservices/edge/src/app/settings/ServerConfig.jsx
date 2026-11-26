// Route : /settings/server-config — la configuration du serveur (chantier 81 ; le porteur, 2026-11-25 : « On peut mettre
// les varenv dans une section "Serveur config" dans le settings pour les administrateurs ? »). Ce qui peut changer sans
// relancer le serveur a quitté le compose et se règle ici, gardé par le kernel (api/server_config.go) : son nom, le GPU
// des parties, puis, pour chaque console Streamex, ses manettes et ses parties en même temps — un bouton par section
// (le porteur, 2026-11-25), la réponse en fenêtre. Le miroir est supprimé (le porteur, 2026-11-25 : « on n'utilise plus de mirror »). Puis
// « Détails » : ce que le compose fixe, en lecture seule et sans secret. Un serveur seulement
// (la capacité `serverConfig`) : le téléphone et le PC n'ont rien de tout cela.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { fetchServerConfig, saveServerConfig } from "../libs/api.js";
import { useData } from "../libs/data.jsx";
import { Ruleset } from "../ui/Forms.jsx";
import SettingsPage from "./SettingsPage.jsx";
import { InfoRow } from "../bricks/InfoRow.jsx";
import { Loading } from "../bricks/Loading.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { SaveSection } from "../bricks/SaveSection.jsx";
import { SelectRow } from "../bricks/SelectRow.jsx";
import { Section } from "../bricks/Section.jsx";
import { TextRow } from "../bricks/TextRow.jsx";

// Les règles des champs sont celles du kernel (config.Live, Normalized).
const PCI_ADDRESS = /^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$/;
// PAR CONSOLE STREAMEX, UN SÉLECTEUR (le porteur, 2026-11-25 : « mettre le champ "Games at the same time" en selectbox pour
// chacune des consoles » ; « Le champ "Controllers per game" c'est aussi par console. Les rétro sont toujours 2 ») : ses
// manettes, de 1 à 8 — par défaut, les places que le registre lui donne —, et ses parties en même temps, « Sans limite »
// puis de 1 à 8 (MaxPlayers, MaxConsoleLimit du kernel). Les consoles rétro gardent leurs places, fixes.
const MAX_PER_CONSOLE = 8;
const upTo = (count) => Array.from({ length: count }, (_, index) => ({ value: String(index + 1), label: String(index + 1) }));

// Ce qui est enregistré, tel que le kernel le reçoit : une section n'en change que sa part.
const payloadOf = (stored) => ({ serverName: stored.serverName || "", gpu: stored.gpu || "", players: stored.players || {}, consoleLimits: stored.consoleLimits || {} });
// Les places d'une console : son réglage, sinon celles du registre.
const seatsOf = (console) => console.maxPlayers || 4;

export default function ServerConfig() {
    const { t } = useTranslation();
    const [stored, setStored] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchServerConfig().then((answer) => !cancelled && setStored(answer.ok ? answer.data : null));

        return () => {
            cancelled = true;
        };
    }, []);

    return (
        <SettingsPage title={t("settings.items.serverConfig.label")} guides={[["install"], ["update"]]}>
            {stored === undefined ? <Section><Loading /></Section>
                : stored === null ? <Section title={t("settings.serverConfig.settings")} sense="modify"><p className="mb-0">{t("settings.loadFailed")}</p></Section>
                    : (
                        <>
                            <ServerConfigForms stored={stored} onSaved={setStored} />
                            <Section title={t("settings.serverConfig.details")} help={t("settings.serverConfig.detailsHelp")} sense="info">
                                <Rows>
                                    <InfoRow label={t("settings.serverConfig.publicHost")} value={stored.fixed?.publicHost || "-"} />
                                    <InfoRow label={t("settings.serverConfig.edgePort")} value={stored.fixed?.edgePort || "-"} />
                                    <InfoRow label={t("settings.serverConfig.storageDir")} value={stored.fixed?.storageDir || "-"} />
                                    <InfoRow label={t("settings.serverConfig.internalDisks")} value={String(stored.fixed?.internalDisks ?? 0)} />
                                    <InfoRow label={t("settings.serverConfig.stack")} value={stored.fixed?.stack || "-"} />
                                    <InfoRow label={t("settings.serverConfig.staging")} value={t(stored.fixed?.staging ? "switches.on" : "switches.off")}
                                        pill={stored.fixed?.staging ? "info" : "secondary"} />
                                </Rows>
                            </Section>
                        </>
                    )}
        </SettingsPage>
    );
}

// UN BOUTON PAR SECTION (le porteur, 2026-11-25 : « j'aurais mis un bouton par section au lieu de l'avoir à la fin de
// l'avant dernière section ») : chaque section est une SaveSection, et n'enregistre que ce qu'elle montre — le reste part
// tel qu'il est enregistré ; une valeur fausse se dit sous son champ, et le kernel la refuse aussi (sa réponse, en
// fenêtre). Une console aux places du registre n'a pas de réglage ; « Sans limite » non plus.
function ServerConfigForms({ stored, onSaved }) {
    const { t } = useTranslation();
    const { consoles } = useData();
    const streamex = (consoles || []).filter((console) => console.method === "streamex");
    const invalid = (key) => Ruleset.isInvalid(t(`settings.serverConfig.invalid.${key}`));
    const rules = {
        serverName: (data) => (data.serverName.trim().length > 40 ? invalid("serverName") : null),
        gpu: (data) => {
            const raw = data.gpu.trim();

            return raw === "" || PCI_ADDRESS.test(raw) ? null : invalid("gpu");
        },
    };
    const playerOptions = upTo(MAX_PER_CONSOLE);
    const limitOptions = [{ value: "0", label: t("settings.serverConfig.noLimit") }, ...upTo(MAX_PER_CONSOLE)];
    const saved = payloadOf(stored);
    const players = Object.fromEntries(streamex.map((console) => [console.id, saved.players[console.id] || seatsOf(console)]));
    const limits = Object.fromEntries(streamex.map((console) => [console.id, saved.consoleLimits[console.id] || 0]));

    const save = async (patch) => {
        const answer = await saveServerConfig({ ...saved, ...patch });

        if (answer.ok) {
            onSaved(answer.data);
        }

        return answer;
    };

    return (
        <>
            <SaveSection id="server-config-settings" title={t("settings.serverConfig.settings")} help={t("settings.serverConfig.settingsHelp")}
                saved={{ serverName: saved.serverName, gpu: saved.gpu }} rules={rules} success={t("settings.serverConfig.saved")}
                onSave={(values) => save({ serverName: values.serverName.trim(), gpu: values.gpu.trim() })}>
                {(values, set, form) => (
                    <>
                        <TextRow id="serverName" context={form} label={t("settings.serverConfig.serverName")} help={t("settings.serverConfig.serverNameHelp")}
                            props={{ maxLength: 40, placeholder: stored.fixed?.publicHost || "" }} />
                        <TextRow id="gpu" context={form} label={t("settings.serverConfig.gpu")} help={t("settings.serverConfig.gpuHelp")}
                            props={{ placeholder: "0000:c4:00.0" }} />
                    </>
                )}
            </SaveSection>
            <SaveSection id="server-config-players" title={t("settings.serverConfig.players")} help={t("settings.serverConfig.playersHelp")} saved={players}
                success={t("settings.serverConfig.saved")}
                onSave={(values) => save({ players: Object.fromEntries(streamex.filter((console) => values[console.id] !== seatsOf(console))
                    .map((console) => [console.id, values[console.id]])) })}>
                {(values, set) => streamex.map((console) => (
                    <SelectRow key={console.id} id={`players-${console.id}`} label={console.name} options={playerOptions}
                        value={String(values[console.id] || seatsOf(console))} onChange={(raw) => set({ [console.id]: Number(raw) })} />
                ))}
            </SaveSection>
            <SaveSection id="server-config-limits" title={t("settings.serverConfig.consoleLimits")} help={t("settings.serverConfig.consoleLimitsHelp")} saved={limits}
                success={t("settings.serverConfig.saved")}
                onSave={(values) => save({ consoleLimits: Object.fromEntries(streamex.filter((console) => values[console.id] > 0)
                    .map((console) => [console.id, values[console.id]])) })}>
                {(values, set) => streamex.map((console) => (
                    <SelectRow key={console.id} id={`limit-${console.id}`} label={console.name} options={limitOptions}
                        value={String(values[console.id] || 0)} onChange={(raw) => set({ [console.id]: Number(raw) })} />
                ))}
            </SaveSection>
        </>
    );
}
