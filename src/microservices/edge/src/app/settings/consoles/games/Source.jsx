// Route : /settings/consoles/<console>/games/<source> — les jeux d'une source.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { sizeGroup, statusGroup } from "../../../bricks/FilterGroups.jsx";
import { Listing } from "../../../bricks/Listing.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Note } from "../../../bricks/Note.jsx";
import { PlaceholderRow } from "../../../bricks/PlaceholderRow.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { hiddenOf, toggleHidden } from "../../../globals/filters.js";
import { downloadState, useDownloads } from "../../../libs/downloads.js";
import { fact, formatSize, statusOf } from "../../../libs/format.js";
import { useDebounced } from "../../../libs/ui.js";
import { GameCover } from "../../../screens/library/HeroArt.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../../SettingsPage.jsx";
import { gamePagePath, gameSourceOf, gamesPath, isRemote, MEGABYTE, readGames, refusalOf, sourceOf, useLevels, useSourceNames } from "../Games.jsx";
import { pill, STATUS_PILL } from "../../../bricks/Pill.jsx";

export default function Source() {
    const { t } = useTranslation();
    const { consoleId, source: slug } = useParams();
    const { open } = useSettingsNavigation();
    const source = sourceOf(slug);
    const { name } = useSourceNames(slug);
    const [read, update] = useQueryState({ page: "1" });
    const search = read("q");
    const query = useDebounced(search.trim());
    const [answer, setAnswer] = useState(null);
    const levels = useLevels(consoleId, slug);
    const peer = isRemote(source);
    // Les jeux de cette console qui attendent leur tour ou descendent (la file des jeux) : leur ligne le dit.
    const downloads = useDownloads();
    const minSize = Math.max(0, Number(read("minSize")) || 0);
    const heaviest = read("sort") === "size";
    // LES STATUTS, UN INTERRUPTEUR CHACUN (le porteur, 2026-11-19), comme la bibliothèque d'une console : les écartés
    // dans `?nostatus=`, tous montrés par défaut.
    const hiddenStatuses = hiddenOf(read("nostatus"));
    const games = (answer?.games || []).filter((game) => (!minSize || (game.sizeBytes || 0) >= minSize * MEGABYTE)
        && !hiddenStatuses.includes(statusOf(game) || "untested"));

    if (heaviest) {
        games.sort((left, right) => (right.sizeBytes || 0) - (left.sizeBytes || 0));
    }

    useEffect(() => {
        if (!source) {
            return undefined;
        }
        let cancelled = false;
        setAnswer(null);
        readGames(consoleId, source, query).then((result) => !cancelled && setAnswer(result));

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, slug, query]);

    return (
        <SettingsPage title={name} parent={gamesPath(consoleId)} levels={levels} guides={[["consoles"], ["library", "browse-the-catalog-and-keep-favorites"]]}>
            {!source ? <NotFound /> : (
                <Section title={t("gameAdmin.listTitle")} sense="modify">
                    {answer && !answer.games ? <Note className="mt-0">{refusalOf(answer.failure, t)}</Note> : (
                        <Listing items={games} loading={answer === null} search={search} onSearch={(q) => update({ q })} page={read("page")} onPage={(page) => update({ page })}
                            placeholder={t("gameAdmin.search")} found={t("gameAdmin.found", { count: games.length })} empty={t(minSize ? "gameAdmin.noneHeavier" : "gameAdmin.none", { size: minSize })}
                            filterCount={(minSize ? 1 : 0) + (heaviest ? 1 : 0) + hiddenStatuses.length} filters={[
                                statusGroup({ t, id: "gs-status", hidden: hiddenStatuses, onChange: (status, on) => update({ nostatus: toggleHidden(hiddenStatuses, status, on), page: null }) }),
                                sizeGroup({ t, id: "gs", minSize, onMinSize: (value) => update({ minSize: value, page: null }), heaviest, onHeaviest: (on) => update({ sort: on ? "size" : null, page: null }) }),
                            ]}
                            placeholderRow={() => <PlaceholderRow lead="cover" />}
                            render={(game) => {
                                const key = `gl-${game.reference}`;
                                const downloading = peer ? downloads.find((entry) => entry.key === `roms/${consoleId}/${game.reference}`) : null;

                                return (
                                    <NavRow key={game.reference} id={key} icon="bi-disc" label={game.title} checked={peer && game.here}
                                        lead={<GameCover consoleId={consoleId} reference={game.reference} game={gameSourceOf(source)} />}
                                        help={[pill(t(`status.${statusOf(game) || "untested"}`), STATUS_PILL[statusOf(game) || "untested"]), downloadState(downloading, t) || fact(t("game.size"), formatSize(game.sizeBytes))]}
                                        onOpen={() => open(gamePagePath(consoleId, slug, game.reference), key, { game })} />
                                );
                            }} />
                    )}
                </Section>
            )}
        </SettingsPage>
    );
}
