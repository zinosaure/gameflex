// Route : /settings/profile/saves/<source>/game/<console>/<jeu>/slot/<n> et …/all : UN emplacement, ou tous ceux du jeu
// (une ligne chacun : sa vignette, sa date ; seul, le titre du jeu, que le titre de la page ne dit pas), et leurs
// gestes ; un backup seul, sa note. Retiré, la page n'a plus rien à montrer : retour au jeu.
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { GameFacts } from "../../../../bricks/GameFacts.jsx";
import { InfoRow } from "../../../../bricks/InfoRow.jsx";
import { Listing, useListSearch } from "../../../../bricks/Listing.jsx";
import { Loading } from "../../../../bricks/Loading.jsx";
import { GuideSection } from "../../../../bricks/GuideSection.jsx";
import { Section } from "../../../../bricks/Section.jsx";
import { dateTime, fact } from "../../../../libs/format.js";
import { slotName, StateThumbnail } from "../../../../screens/library/GameSettings.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../../SettingsPage.jsx";
import { isBackups, noteKey, NoteSection, PlaceBody, SaveActions, SAVES_GUIDES, slotNoteItem, useBackupNotes, useConsoleName, useGameSlots, useGameTitle, usePlace, useSaveSources, withNote } from "../../Saves.jsx";

export default function Slots() {
    const { t, i18n } = useTranslation();
    const { consoleId, reference, slot: slotParam } = useParams();
    const place = usePlace();
    const { self } = useSaveSources();
    const { back } = useSettingsNavigation();
    const found = useGameTitle(consoleId, reference, place.where);
    const [slots, refresh] = useGameSlots(consoleId, reference, place.where);
    const notes = useBackupNotes(place.where);
    const consoleName = useConsoleName()(consoleId);
    const parent = `${place.base}/game/${encodeURIComponent(consoleId)}/${encodeURIComponent(reference)}`;
    const title = found?.title || reference;
    const all = slotParam === undefined;
    const chosen = (slots || []).filter((entry) => all || String(entry.slot) === slotParam);
    const items = chosen.map((entry) => ({ consoleId, reference, slot: entry.slot, title, timestamp: entry.timestamp }));
    const label = all ? t("saves.wholeGame") : slotName(Number(slotParam), t);
    const noteOf = (slot) => notes?.[noteKey(slotNoteItem(consoleId, reference, slot))] || null;
    const shown = useListSearch(chosen, (entry) => [slotName(entry.slot, t), noteOf(entry.slot)].join(" "));
    const backupNote = !all && isBackups(place.where) ? { path: `${parent}/slot/${slotParam}/note`, note: noteOf(slotParam) } : null;
    const body = slots === null || notes === null || found === undefined ? <Section><Loading /></Section> : !chosen.length ? <NotFound /> : (
        <>
            {/* UN EMPLACEMENT SEUL : SA FICHE (le porteur, 2026-11-23 : « il n'y a pas de fiche détails »), la forme de toute
                fiche (GameFacts) — sa vignette et son nom en tête, sa console, sa date. Tous : une ligne chacun. */}
            {all ? (
                <Section title={t("saves.slotsTitle")} help={t("saves.chosenHelp")} sense="info">
                    <Listing items={shown.found} search={shown.search} onSearch={shown.onSearch} placeholder={t("saves.search")} searchKey="sv-chosen-search"
                        render={(entry) => (
                        <InfoRow key={entry.slot} label={slotName(entry.slot, t)} value={withNote(fact(t("facts.date"), dateTime(entry.timestamp, i18n.language)), noteOf(entry.slot), t)}
                            lead={<StateThumbnail consoleId={consoleId} reference={reference} slot={entry.slot} source={place.where.source} host={place.where.host} version={entry.timestamp} />} />
                    )} />
                </Section>
            ) : (
                <Section title={t("details")} help={t("saves.detailsHelp")} sense="info">
                    <GameFacts complete={false} facts={{ console: consoleName, date: dateTime(chosen[0].timestamp, i18n.language) }}>
                        <InfoRow label={t("facts.slot")} value={slotName(chosen[0].slot, t)}
                            lead={<StateThumbnail consoleId={consoleId} reference={reference} slot={chosen[0].slot} source={place.where.source} host={place.where.host} version={chosen[0].timestamp} />} />
                    </GameFacts>
                </Section>
            )}
            {backupNote && <NoteSection note={backupNote.note} />}
            <SaveActions id={all ? "sv-all" : `sv-${slotParam}`} where={place.where} peers={place.peers} selfName={self} label={`${title} — ${label}`} slots={items}
                canDelete={all ? place.where.kind === "copy" : place.where.kind !== "peer"} notePath={backupNote?.path} note={backupNote?.note} onDone={refresh} onRemoved={() => back(parent)}
                game={all ? null : { consoleId, reference }} />
        </>
    );

    return (
        <SettingsPage title={label} parent={parent} levels={{ ...place.levels, [`${place.base}/console/${encodeURIComponent(consoleId)}`]: consoleName, [parent]: found?.title }}>
            <PlaceBody place={place}>{body}</PlaceBody>
            <GuideSection guides={SAVES_GUIDES} />
        </SettingsPage>
    );
}
