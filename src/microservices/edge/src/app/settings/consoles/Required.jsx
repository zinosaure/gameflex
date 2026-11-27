// Route : /settings/consoles/<console>/cores/<nom> et …/bios/<nom> — un émulateur ou un BIOS (cores/Core.jsx, bios/File.jsx).
import { useFeedback } from "../../bricks/useFeedback.jsx";
import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { DangerRow } from "../../bricks/DangerRow.jsx";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { DownloadRow } from "../../bricks/DownloadRow.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Listing, useListSearch } from "../../bricks/Listing.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Note } from "../../bricks/Note.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { SaveSection } from "../../bricks/SaveSection.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SwitchRow } from "../../bricks/SwitchRow.jsx";
import { useConfirm } from "../../bricks/useConfirm.jsx";
import { consoleDefaults, deleteInstalled, fetchAvailability, fetchRequired, setConsoleDefaults } from "../../libs/api.js";
import { downloadOutcome, runDownload, useDownloads } from "../../libs/downloads.js";
import { fact, formatMegabytes } from "../../libs/format.js";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../SettingsPage.jsx";
import { consolePath, inService, loadSection, useEmulated } from "../Consoles.jsx";

// Une source de « Le récupérer », sa clé (celle du registre des téléchargements) et son nom.
const sourceKey = (source) => (source.source === "official" ? "official" : `server:${source.serverId}`);
const sourceLabel = (source, item, t) => (source.source === "official" ? t(item.package ? "consoles.sourceStreamex" : "consoles.sourceOfficial") : source.name || t("consoles.serverNamed", { id: source.serverId }));
// La raison d'une source qui ne l'a pas : injoignable, notre jeton refusé, ou elle ne l'a pas.
export const reasonOf = (reason, t) => t(reason === "unreachable" ? "consoles.sourceUnreachable" : reason === "denied" ? "consoles.sourceDenied" : "consoles.sourceLacks");
const hostOf = (address) => {
    try {
        return new URL(address).host;
    } catch {
        return null;
    }
};

// « LE RÉCUPÉRER » (le porteur, 2026-11-13) : la source officielle (un cœur rétro chez libretro, un émulateur de
// bureau chez son éditeur), puis chaque serveur fédéré, demandés à la source au moment où la page s'ouvre
// (storage_availability.go) ; grisés quand ils ne l'ont pas, avec leur raison. Toucher une source ouvre la fenêtre de
// confirmation, puis le téléchargement part en tâche de fond (libs/downloads.js) : la ligne de la source devient celle
// du téléchargement (DownloadRow : sa progression, son étape, « Annuler ») ; les autres sources grisées le temps qu'il
// dure. La taille qu'un
// serveur fédéré annonce se lit dans la copie de son listing. Les sources se feuillettent (Listing), la page tenue par
// la liste : une ligne n'ouvre pas de page ; demandées, elles sont des lignes fantômes.
function FetchSources({ section, consoleId, item }) {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const key = `${section}/${consoleId}/${item.name}`;
    const running = useDownloads().find((entry) => entry.key === key) || null;
    const [sources, setSources] = useState(undefined);
    const [failure, setFailure] = useState("");
    const [offered, setOffered] = useState({});

    useEffect(() => {
        let cancelled = false;

        fetchAvailability(consoleId, section, item.name).then((answer) => {
            if (cancelled) {
                return;
            }
            const found = answer.ok ? answer.data?.sources || [] : null;
            setSources(found);
            // La raison, dite (« Pas encore sur cet appareil » au téléphone, libs/api.js), plutôt qu'un échec brut.
            setFailure(answer.ok ? "" : answer.errorMessage);

            for (const source of (found || []).filter((candidate) => candidate.source === "server" && candidate.available)) {
                fetchRequired(section, `peer:${source.serverId}`).then((listed) => {
                    const theirs = (listed.ok ? listed.data?.consoles || [] : []).filter((listedConsole) => listedConsole.consoleId === consoleId).flatMap((listedConsole) => listedConsole.items || [])
                        .find((candidate) => candidate.name === item.name && candidate.installed);

                    if (!cancelled && theirs) {
                        setOffered((current) => ({ ...current, [source.serverId]: theirs }));
                    }
                });
            }
        });

        return () => {
            cancelled = true;
        };
    }, [section, consoleId, item.name]);

    const start = async (source) => {
        const total = Number(offered[source.serverId]?.sizeBytes) || 0;
        const command = source.source === "official"
            ? { route: "cores", body: { consoleId, core: item.name } }
            : { route: "server", server: String(source.serverId), section, body: { consoleId, name: item.name }, ...(total ? { total } : {}) };
        const answer = await runDownload({ key, label: item.name, source: sourceKey(source), command });
        toast(downloadOutcome(answer, item.name, t));
    };

    const ask = (source) => {
        const size = formatMegabytes(offered[source.serverId]?.sizeBytes);
        const from = sourceLabel(source, item, t);

        confirm({
            title: t("consoles.fetchFrom", { source: from }),
            message: t(size ? "consoles.fetchConfirmSized" : "consoles.fetchConfirm", { name: item.name, source: from, size }),
            confirmLabel: t("consoles.download"),
            onConfirm: () => start(source),
        });
    };

    // Les sources, nombreuses quand les serveurs fédérés le sont : un listing qui se cherche.
    const found = useListSearch(sources || [], (source) => sourceLabel(source, item, t));
    const row = (source) => {
        const own = running?.source === sourceKey(source);
        const theirs = offered[source.serverId];
        const official = source.source === "official";
        const facts = !source.available ? [official ? null : fact(t("facts.type"), t("consoles.federatedServer")), reasonOf(source.reason, t)]
            : official ? [fact(t("fed.urlLabel"), hostOf(item.source))]
                : [fact(t("facts.type"), t("consoles.federatedServer")), fact(t("consoles.version"), theirs?.version), fact(t("game.size"), formatMegabytes(theirs?.sizeBytes))];

        return (
            own ? <DownloadRow key={sourceKey(source)} entry={running} id={`cs-source-${sourceKey(source)}`} label={sourceLabel(source, item, t)} />
                : <NavRow key={sourceKey(source)} id={`cs-source-${sourceKey(source)}`} icon={official ? "bi-globe2" : "bi-arrow-left-right"}
                    label={sourceLabel(source, item, t)}
                    help={facts}
                    disabled={!source.available || Boolean(running)} opens="confirm" onOpen={() => ask(source)} />
        );
    };

    return (
        <Section title={t("consoles.fetchTitle")} help={t(section === "bios" ? "consoles.fetchHelpBios" : "consoles.fetchHelp")} sense="modify">
            {sources === null ? <Note className="mt-0">{failure || t("settings.actionFailed")}</Note>
                : <Listing items={found.found} loading={sources === undefined} empty={t("consoles.noSource")} render={row}
                    search={found.search} onSearch={found.onSearch} placeholder={t("gameAdmin.searchSource")} searchKey="cs-source-search" />}
        </Section>
    );
}

// /settings/consoles/<console>/cores|bios/<nom> : un émulateur ou un BIOS.
//   - Détails : son statut, son paquet (un émulateur), sa version, sa taille ;
//   - Utilisation, installé : « Utiliser cette version » (un émulateur de bureau : la version en service, qui fait de
//     son émulateur celui par défaut), « Émulateur par défaut » (une console rétro qui en a plusieurs), « BIOS par
//     défaut » (une console rétro qui en a plusieurs) — par le bouton de sa section, enregistré par la console
//     (console_defaults.go). La
//     fiche d'un jeu choisit toujours pour un lancement (le porteur, 2026-11-13, réponse 4 du chantier 53) ;
//   - Danger, installé : le supprimer de cette machine, puis retour à la console ;
//   - absent : « Le récupérer » (FetchSources).
// Un téléchargement fini relit la page : l'élément est là.
export function RequiredPage({ section }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { consoleId, name } = useParams();
    const { back } = useSettingsNavigation();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const running = useDownloads().some((download) => download.key === `${section}/${consoleId}/${name}`);
    const wasRunning = useRef(running);
    const [rows, setRows] = useState(undefined);
    const [defaults, setDefaults] = useState(null);
    const [epoch, setEpoch] = useState(0);
    const item = rows === undefined ? undefined : (rows || []).find((row) => row.name === name) || null;
    // Retour à sa liste (sa page à elle, chantier 81) : les BIOS, les émulateurs mobiles, ou ceux de ce serveur.
    const parent = `${consolePath(consoleId)}/${section === "bios" ? "bios" : item?.package === "phone" ? "mobile-cores" : "cores"}`;

    useEffect(() => {
        let cancelled = false;

        Promise.all([loadSection(section, consoleId), consoleDefaults(consoleId)]).then(([read, answer]) => {
            if (!cancelled) {
                setRows(read);
                setDefaults(answer.ok ? answer.data : null);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [section, consoleId, name, epoch]);

    useEffect(() => {
        if (wasRunning.current && !running) {
            setEpoch((value) => value + 1);
        }
        wasRunning.current = running;
    }, [running]);

    // LE CHOIX DE LA CONSOLE que cette page règle : ce que la ligne dit régler (`choose`, écrit par le kernel — chantier
    // 54 : au téléphone, la Switch est une console Streamex dont le choix est l'émulateur), et quand il a un sens. Une
    // version d'un émulateur de bureau du serveur (`emulatorFile`) se met en service, si l'image la reconnaît
    // (`tested`) ; un cœur, un émulateur du
    // téléphone ou un BIOS, seulement quand la console en a plusieurs — ou quand c'est déjà lui qui est enregistré.
    const field = item?.choose || null;
    const stored = (field && defaults?.stored?.[field]) || "";
    const several = (rows || []).filter((row) => row.choose === field && (section === "bios" ? row.installed : true)).length > 1;
    const usage = !item?.installed || !entry || !field ? null
        : field === "emulatorFile" ? { label: "consoles.useVersion", help: "consoles.useVersionHelp", known: item.tested !== undefined }
            : several || stored === item.name ? { label: section === "bios" ? "consoles.defaultBios" : "consoles.defaultEmulator", help: "consoles.defaultHelp", known: true } : null;
    const chosen = Boolean(item) && inService(section, item, defaults?.inUse);
    const implicit = chosen && stored !== item.name;

    // PAR LE BOUTON DE SA SECTION (le porteur, 2026-11-25 : « Faudra rajouter des boutons pour tous les formulaires qui
    // n'en ont pas ») : plus de confirmation à l'interrupteur — le bouton l'est —, la réponse en fenêtre (SaveSection).
    const choose = async ({ on }) => {
        const answer = await setConsoleDefaults(consoleId, { [field]: on ? item.name : "" });

        if (answer.ok) {
            setEpoch((value) => value + 1);
        }

        return answer;
    };

    const facts = item ? [
        t(item.installed ? "consoles.installed" : "consoles.missing"),
        item.tested === true ? t("consoles.tested") : null,
        item.tested === false ? t("consoles.untested") : null,
        item.installed && item.scoped === false ? t("consoles.unfiled") : null,
    ].filter(Boolean) : [];
    const phone = item?.package === "phone";

    return (
        <SettingsPage title={name} parent={parent} enter={usage ? "cs-default" : null} guides={[["consoles"], ["library", "3-get-the-emulators"]]}>
            {item === undefined ? <Section><Loading /></Section> : item === null ? <NotFound /> : (
                <>
                    <Section title={t("details")} sense="info">
                        <Rows>
                            <InfoRow icon={item.installed ? "bi-check2-circle" : "bi-x-circle"} label={t("games.status")} value={facts} />
                            {section === "cores" && <InfoRow icon={phone ? "bi-phone" : "bi-pc-display"} label={t("resources.typeLabel")} value={t(phone ? "consoles.packagePhone" : "consoles.packageServer")} />}
                            {item.installed && <InfoRow icon="bi-tag" label={t("consoles.version")} value={item.version} />}
                            {item.installed && <InfoRow icon="bi-hdd" label={t("game.size")} value={formatMegabytes(item.sizeBytes)} />}
                        </Rows>
                    </Section>
                    {usage && (
                        <SaveSection id="cs-default" title={t("consoles.usage")} saved={{ on: chosen }} disabled={!usage.known || implicit || defaults === null}
                            onSave={choose} success={({ on }) => t(on ? "consoles.defaultSaved" : "consoles.defaultCleared", { name: item.name })}>
                            {(values, set) => (
                                <SwitchRow id="cs-default-on" label={t(usage.label)} checked={values.on}
                                    disabled={!usage.known || implicit || defaults === null} onChange={(on) => set({ on })}
                                    help={!usage.known ? t("consoles.useVersionUnknown") : implicit ? t("consoles.defaultImplicit") : t(usage.help)} />
                            )}
                        </SaveSection>
                    )}
                    {!item.installed && <FetchSources section={section} consoleId={consoleId} item={item} />}
                    {item.installed && (
                        <DangerSection>
                            <DangerRow id="cs-remove" label={t(section === "bios" ? "consoles.removeBios" : "consoles.removeEmulator")} help={t("consoles.removeHelp")}
                                question={t("consoles.removeConfirm", { name: item.name })} confirmLabel={t("consoles.remove")} onConfirm={async () => {
                                    const answer = await deleteInstalled(section, consoleId, item.name);
                                    feedback.result(answer.ok, t("consoles.removed", { name: item.name }), answer.errorMessage || t("consoles.removeFailed"));

                                    if (answer.ok) {
                                        back(parent);
                                    }
                                }} />
                        </DangerSection>
                    )}
                </>
            )}
        </SettingsPage>
    );
}
