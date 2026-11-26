// Route : /settings/profile/saves/<source>/console/<console>/game/<jeu> et …/all : la sauvegarde d'UN jeu, ou celles de
// toute la console (une ligne chacune), et leurs gestes ; un backup seul, sa note. Retirée, retour à la console.
import React from "react";
import { useTranslation } from "react-i18next";
import { dateTime } from "../../../../libs/format.js";
import { useParams } from "react-router-dom";
import { GameFacts } from "../../../../bricks/GameFacts.jsx";
import { InfoRow } from "../../../../bricks/InfoRow.jsx";
import { Listing, useListSearch } from "../../../../bricks/Listing.jsx";
import { Loading } from "../../../../bricks/Loading.jsx";
import { GuideSection } from "../../../../bricks/GuideSection.jsx";
import { Section } from "../../../../bricks/Section.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../../SettingsPage.jsx";
import { gameNoteItem, isBackups, noteKey, NoteSection, PlaceBody, SaveActions, SAVES_GUIDES, streamexHelp, streamexName, streamexOrigin, useBackupNotes, useConsoleGames, useConsoleName, usePlace, useSaveSources } from "../../Saves.jsx";

export default function ConsoleGames() {
    const { t, i18n } = useTranslation();
    const { consoleId, gameId } = useParams();
    const place = usePlace();
    const { self } = useSaveSources();
    const { back } = useSettingsNavigation();
    const [games, refresh] = useConsoleGames(consoleId, place.where);
    const notes = useBackupNotes(place.where);
    const consoleName = useConsoleName()(consoleId);
    const parent = `${place.base}/console/${encodeURIComponent(consoleId)}`;
    const all = gameId === undefined;
    const chosen = (games || []).filter((game) => all || game.id === gameId);
    const label = all ? t("saves.wholeConsole", { name: consoleName }) : chosen[0] ? streamexName(chosen[0], t) : gameId;
    const noteOf = (id) => notes?.[noteKey(gameNoteItem(consoleId, id))] || null;
    const shown = useListSearch(chosen, (game) => [streamexName(game, t), game.id, noteOf(game.id)].join(" "));
    const backupNote = !all && isBackups(place.where) ? { path: `${parent}/game/${encodeURIComponent(gameId)}/note`, note: noteOf(gameId) } : null;
    const body = games === null || notes === null ? <Section><Loading /></Section> : !chosen.length ? <NotFound /> : (
        <>
            {/* UN JEU SEUL : SA FICHE (le porteur, 2026-11-23 : « Dans my saves/backup il n'y a pas de fiche détails »), la forme
                de toute fiche (GameFacts) — sa console, d'où vient son jeu, son identifiant, sa date, sa taille. Tous : une ligne
                chacun. */}
            {all ? (
                <Section title={t("saves.gamesTitle")} help={t("saves.chosenHelp")} sense="info">
                    <Listing items={shown.found} search={shown.search} onSearch={shown.onSearch} placeholder={t("saves.search")} searchKey="sx-chosen-search"
                        render={(game) => (
                        // Deux informations, comme la liste des jeux de la console (streamexHelp) : sa date, puis sa note sinon sa
                        // taille — l'identifiant ne dit rien du jeu (le porteur, 2026-11-23) ; le reste est dans sa fiche.
                        <InfoRow key={game.id} icon="bi-shield-check" label={streamexName(game, t)} value={streamexHelp(game, noteOf(game.id), t, i18n.language)} />
                    )} />
                </Section>
            ) : (
                <Section title={t("details")} help={t("saves.detailsHelp")} sense="info">
                    <GameFacts complete={false} facts={{
                        console: consoleName, origin: streamexOrigin(chosen[0], t), gameId: chosen[0].title ? chosen[0].id : null,
                        path: chosen[0].path, date: dateTime(chosen[0].updatedAt, i18n.language), sizeBytes: chosen[0].sizeBytes
                    }}>
                        {chosen[0].console === "xbox" && <InfoRow icon="bi-collection-play" label={t("saves.gamesTitle")} value={t("saves.xboxDiskHelp")} />}
                    </GameFacts>
                </Section>
            )}
            {backupNote && <NoteSection note={backupNote.note} />}
            <SaveActions id={all ? "sx-all" : `sx-${gameId}`} where={place.where} peers={place.peers} selfName={self} games={chosen} label={label}
                canDelete={all ? place.where.kind === "copy" : place.where.kind !== "peer"} notePath={backupNote?.path} note={backupNote?.note} onDone={refresh} onRemoved={() => back(parent)}
                game={all ? null : { consoleId, reference: chosen[0].reference || null }} />
        </>
    );

    return (
        <SettingsPage title={label} parent={parent} levels={{ ...place.levels, [parent]: consoleName }}>
            <PlaceBody place={place}>{body}</PlaceBody>
            <GuideSection guides={SAVES_GUIDES} />
        </SettingsPage>
    );
}
