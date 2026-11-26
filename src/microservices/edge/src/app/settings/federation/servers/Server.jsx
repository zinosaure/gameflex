// Route : /settings/federation/servers/<id> — un serveur rejoint.
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { removeServerRegistry } from "../../../libs/api.js";
import { useData } from "../../../libs/data.jsx";
import { openExternal } from "../../../libs/links.js";
import { REMOTE_PAGES, remoteRoot, useAdminThere, useRemoteAdmin } from "../../../libs/remoteAdmin.js";
import SettingsPage, { useSettingsNavigation } from "../../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../../profile/Federation.jsx";
import { consolesSummary, dateTime, JOINED, loadServers, useLevels, usePassed, Waiting } from "../../Federation.jsx";
import { CONSOLE_ICON } from "../../../globals/icons.js";

// UN SERVEUR FÉDÉRÉ, EN SECTIONS D'INFORMATIONS COMME L'ENGRENAGE D'UN JEU (le porteur, 2026-11-16, 2.8 : « Le listing des
// informations du serveur fédéré en section comme les fiches des jeux ») : ce que la copie de son listing dit de lui —
// le serveur (son lien, sa version, la date de son listing, les jeux qu'il prête), sa machine (système, processeur,
// mémoire, cartes graphiques), ce que son jeton nous ouvre (les jeux, ses consoles à distance, ses fournisseurs de
// métadonnées). Pas de ligne par console : jouer demande toujours un compte chez lui, l'affaire de chaque utilisateur
// dans Réglages › Profil › Fédération (le porteur, 2026-11-16). Le retirer (« Danger »), puis le guide, en dernier.
export default function Server() {
    const { t, i18n } = useTranslation();
    const feedback = useFeedback();
    const { serverId } = useParams();
    const { refreshServers } = useData();
    const { back, open } = useSettingsNavigation();
    const levels = useLevels();
    const server = usePassed("server", serverId, () => loadServers(refreshServers, t, i18n.language));
    const remote = useRemoteAdmin();
    const adminThereNow = useAdminThere(remote ? null : server);
    const adminHere = !remote && adminThereNow;

    if (!server) {
        return <Waiting item={server} title={t("fed.joinedTitle")} parent={JOINED} />;
    }
    const facts = server.facts || {};
    const machine = facts.machine || {};
    const scopes = facts.scopes || [];
    const gigabytes = (bytes) => (bytes ? t("fed.gigabytes", { gb: Math.round(bytes / 2 ** 30) }) : null);
    const lent = Object.values(facts.consoles || {}).reduce((total, count) => total + count, 0);
    // Les mêmes mots que la page d'un accès délivré (« Autorisé », « Pas autorisé ») : une portée, deux pages.
    const opened = (scope) => t(scopes.includes(scope) ? "fed.allowed" : "fed.notAllowed");

    return (
        <SettingsPage title={server.label} parent={JOINED} levels={levels}>
            <Section title={t("fed.serverTitle")} help={t("fed.serverHelp")} sense="info">
                <Rows>
                    {/* SON ADRESSE S'OUVRE dans un autre onglet (le porteur, 2026-11-16 : « rend le lien cliquable pour ouvrir
                        dans un nouveau tab la page ») ; dans l'application Android, au navigateur du téléphone (EXTERNAL). */}
                    <NavRow id="fd-server-link" icon="bi-link-45deg" label={t("fed.linkLabel")} help={server.link} opens="link" onOpen={() => openExternal(server.link)} />
                    <InfoRow icon="bi-tag" label={t("fed.versionLabel")} value={facts.server?.version ? `GameFlex ${facts.server.version}` : null} />
                    <InfoRow icon="bi-clock-history" label={t("fed.listedLabel")} value={facts.updatedAt ? dateTime(facts.updatedAt, i18n.language) : t("fed.noListing")} />
                    <InfoRow icon="bi-collection-play" label={t("fed.gamesLabel")} value={server.facts ? t("fed.gamesCount", { count: lent, total: lent.toLocaleString(i18n.language) }) : null} />
                </Rows>
            </Section>
            {server.facts && (
                <Section title={t("fed.machineTitle")} help={t("fed.machineHelp")} sense="info">
                    <Rows>
                        <InfoRow icon="bi-phone" label={t("fed.deviceLabel")} value={machine.device} />
                        <InfoRow icon="bi-terminal" label={t("fed.osLabel")} value={machine.os} />
                        <InfoRow icon="bi-cpu" label={t("fed.cpuLabel")} value={machine.cpu} />
                        <InfoRow icon="bi-diagram-3" label={t("fed.coresLabel")} value={machine.cores ? String(machine.cores) : null} />
                        <InfoRow icon="bi-memory" label={t("fed.memoryLabel")} value={gigabytes(machine.ramBytes)} />
                        {(machine.gpus || []).map((gpu, index) => (
                            <InfoRow key={`${gpu.name}-${index}`} icon="bi-gpu-card" label={t("fed.gpuLabel")} value={[gpu.name, gigabytes(gpu.vramBytes)]} />
                        ))}
                    </Rows>
                </Section>
            )}
            {server.facts && (
                <Section title={t("fed.grantsTitle")} help={t("fed.grantsHelp")} sense="modify">
                    <Rows>
                        <InfoRow icon="bi-disc" label={t("fed.scope.catalog")} help={t("fed.catalogHelp")} value={opened("catalog")} />
                        {/* LES MÉTADONNÉES VIENNENT AVEC LES JEUX (le porteur, 2026-11-16 : « La métadonnée vient toujours du serveur.
                            C'est toujours autorisé comme les jeux. C'est l'utilisation des fournisseurs metadata du serveur qui doit
                            être autorisée ») : la même portée que les jeux, `catalog`. */}
                        <InfoRow icon="bi-card-text" label={t("fed.scopeMetadata")} help={t("fed.metadataHelp")} value={opened("catalog")} />
                        <InfoRow icon={CONSOLE_ICON} label={t("members.consolesTitle")} help={t("fed.consolesGrantHelp")}
                            value={consolesSummary(facts.granted, t)} />
                        <InfoRow icon="bi-images" label={t("fed.scopeProviders")} help={t("fed.providersGrantHelp")}
                            value={opened("metadata-providers")} />
                    </Rows>
                </Section>
            )}
            {/* PARTAGÉ AVEC VOUS EN TANT QU'ADMINISTRATEUR (chantier 73) : ses réglages, ouverts d'ici chez lui — seulement si mon
                compte de là-bas y est administrateur et l'a autorisé. JAMAIS depuis une page distante elle-même (le porteur :
                « éviter les cycles infinis ») : chez un serveur fédéré, ses propres serveurs rejoints n'en ont pas. */}
            {adminHere && (
                <Section title={t("remoteAdmin.section")} help={t("remoteAdmin.sectionHelp")} sense="modify">
                    <Rows>
                        {REMOTE_PAGES.map((entry) => (
                            <NavRow key={entry.key} id={`fd-admin-${entry.key}`} icon={entry.icon} label={t(`settings.items.${entry.key}.label`)}
                                help={t(`settings.items.${entry.key}.help`)} onOpen={() => open(`${remoteRoot(server.id)}/${entry.path}`, `fd-admin-${entry.key}`)} />
                        ))}
                    </Rows>
                </Section>
            )}
            <DangerSection id={`fd-del-${server.id}`} help={t("fed.removeHelp")} label={t("fed.remove")} question={`${t("fed.removeConfirm")} ${server.label}`}
                onConfirm={async () => {
                    const answer = await removeServerRegistry(server.id);
                    feedback.result(answer.ok, t("fed.removed", { name: server.label }), answer.detail || t("settings.actionFailed"));

                    if (answer.ok) {
                        await refreshServers();
                        back(JOINED);
                    }
                }} />
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}
