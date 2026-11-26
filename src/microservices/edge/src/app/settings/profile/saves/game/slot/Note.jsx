// Route : /settings/profile/saves/copy-local/…/slot/<n>/note et …/game/<jeu>/note : LA NOTE D'UN BACKUP, écrite ou corrigée ;
// vidée, elle part. Le formulaire de Skeleton (Form.Longtext), son bouton au repos tant que rien n'a changé
// (`isReplaced`) ; enregistrée, retour au backup. Ailleurs que dans les backups, la page n'existe pas.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { Loading } from "../../../../../bricks/Loading.jsx";
import { Rows } from "../../../../../bricks/Rows.jsx";
import { GuideSection } from "../../../../../bricks/GuideSection.jsx";
import { Section } from "../../../../../bricks/Section.jsx";
import { SubmitRow } from "../../../../../bricks/SubmitRow.jsx";
import { TextRow } from "../../../../../bricks/TextRow.jsx";
import { useFeedback } from "../../../../../bricks/useFeedback.jsx";
import { writeSaveNote } from "../../../../../libs/api.js";
import { slotName } from "../../../../../screens/library/GameSettings.jsx";
import { Form, FormButton } from "../../../../../ui/Forms.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../../../SettingsPage.jsx";
import { gameNoteItem, isBackups, NOTE_MAX, noteKey, SAVES_GUIDES, slotNoteItem, streamexName, useBackupNotes, useConsoleGames, useConsoleName, useGameTitle, usePlace } from "../../../Saves.jsx";

export default function Note() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { consoleId, reference, slot, gameId } = useParams();
    const place = usePlace();
    const { back } = useSettingsNavigation();
    const notes = useBackupNotes(place.where);
    const consoleName = useConsoleName()(consoleId);
    const [draft, setDraft] = useState(null);
    const [pending, setPending] = useState(false);
    const streamex = gameId !== undefined;
    const item = streamex ? gameNoteItem(consoleId, gameId) : slotNoteItem(consoleId, reference, slot);
    const found = useGameTitle(consoleId, streamex ? null : reference, streamex ? null : place.where);
    const [games] = useConsoleGames(consoleId, streamex ? place.where : null);
    const savedGame = streamex ? (games || []).find((game) => game.id === gameId) : null;
    const consolePath = `${place.base}/console/${encodeURIComponent(consoleId)}`;
    const gamePath = `${place.base}/game/${encodeURIComponent(consoleId)}/${encodeURIComponent(reference)}`;
    const parent = streamex ? `${consolePath}/game/${encodeURIComponent(gameId)}` : `${gamePath}/slot/${slot}`;
    // Les niveaux au-dessus, nommés même par un lien direct : la console et le jeu (son titre, sinon son identifiant), ou
    // le jeu et l'emplacement.
    const levels = streamex
        ? { ...place.levels, [consolePath]: consoleName, [parent]: savedGame ? streamexName(savedGame, t) : gameId }
        : { ...place.levels, [consolePath]: consoleName, [gamePath]: found?.title, [parent]: slotName(Number(slot), t) };
    const saved = notes?.[noteKey(item)]?.note || "";
    const value = draft ?? saved;
    const isReplaced = value.trim() !== saved;

    const save = async () => {
        setPending(true);
        const answer = await writeSaveNote(item, value);
        setPending(false);

        if (answer.ok) {
            feedback.success(t(value.trim() ? "saves.noteSaved" : "saves.noteRemoved"));
            back(parent);
        } else {
            feedback.failed(answer.status === 404 ? t("saves.noteNoBackup") : answer.errorMessage || t("settings.saveFailed"));
        }
    };

    const body = !isBackups(place.where) ? <NotFound /> : notes === null ? <Section><Loading /></Section> : (
        <Section title={t("saves.noteTitle")} help={t("saves.notePageHelp")} sense="modify">
            <Form props={{
                onSubmit: (event) => {
                    event.preventDefault();

                    if (isReplaced && !pending) {
                        save();
                    }
                }
            }}>
                <Rows>
                    <TextRow id="sv-note" kind="longtext" label={t("saves.note")} help={t("saves.noteFieldHelp")} value={value}
                        onChange={(next) => setDraft(String(next || "").slice(0, NOTE_MAX))} props={{ maxLength: NOTE_MAX, rows: 5, disabled: pending }} />
                    <SubmitRow>
                        <FormButton id="sv-note-save" label={t("settings.save")} isReplaced={isReplaced} pending={pending} />
                    </SubmitRow>
                </Rows>
            </Form>
        </Section>
    );

    return (
        <SettingsPage title={t(saved ? "saves.noteEdit" : "saves.noteWrite")} parent={parent} levels={levels}>
            {body}
            <GuideSection guides={SAVES_GUIDES} />
        </SettingsPage>
    );
}
