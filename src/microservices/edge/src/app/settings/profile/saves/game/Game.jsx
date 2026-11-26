// Route : /settings/profile/saves/<source>/game/<console>/<jeu>
import { useFeedback } from "../../../../bricks/useFeedback.jsx";
import React from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import { DangerSection } from "../../../../bricks/DangerSection.jsx";
import { Loading } from "../../../../bricks/Loading.jsx";
import NavRow from "../../../../bricks/NavRow.jsx";
import { Rows } from "../../../../bricks/Rows.jsx";
import { GuideSection } from "../../../../bricks/GuideSection.jsx";
import { Section } from "../../../../bricks/Section.jsx";
import { deleteGameSaves } from "../../../../libs/api.js";
import { counted, fact } from "../../../../libs/format.js";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../../SettingsPage.jsx";
import { childPath, PlaceBody, SaveGameSlots, SAVES_GUIDES, useConsoleName, useGameSlots, useGameTitle, usePlace } from "../../Saves.jsx";

export default function Game() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { consoleId, reference } = useParams();
    const place = usePlace();
    const { back } = useSettingsNavigation();
    const { open } = useSettingsNavigation();
    const location = useLocation();
    const found = useGameTitle(consoleId, reference, place.where);
    const [slots] = useGameSlots(consoleId, reference, place.where);
    const consoleName = useConsoleName()(consoleId);
    const parent = `${place.base}/console/${encodeURIComponent(consoleId)}`;
    const body = found === undefined ? <Section><Loading /></Section> : found === null ? <NotFound /> : (
        <>
            <Section title={t("saves.slotsTitle")} help={t("saves.slotsHelp")} sense="info">
                <SaveGameSlots consoleId={consoleId} reference={reference} title={found.title} where={place.where} slots={slots} />
            </Section>
            {(slots || []).length > 1 && (
                <Section title={t("more")} help={t("saves.gameMoreHelp")} sense="modify">
                    <Rows>
                        <NavRow id="sv-all" icon="bi-collection" label={t("saves.wholeGame")} value={fact(t("saves.slotsTitle"), counted(t, "slots", slots.length))}
                            onOpen={() => open(childPath(location, "all", "slotsPage"), "sv-all", { title: found.title })} />
                    </Rows>
                </Section>
            )}
            {/* TOUTES SES SAUVEGARDES D'UN GESTE (la ligne de la liste ne porte plus de geste) — ici ou dans une
                copie ; jamais chez un serveur fédéré. Fait, la page n'a plus rien à montrer : retour à la liste. */}
            {place.where?.kind !== "peer" && (
                <DangerSection id="sv-del-game" help={t("saves.deleteGameHelp")} label={t("saves.deleteGame")} question={t("saves.deleteGameConfirm", { title: found.title })}
                    onConfirm={async () => {
                        const answer = await deleteGameSaves(consoleId, reference, place.where.source);

                        if (answer.ok) {
                            feedback.success(t("saves.deleted"));
                            back(parent);
                        } else {
                            feedback.failed(t("saves.deleteFailed"));
                        }
                    }} />
            )}
        </>
    );

    return (
        <SettingsPage title={found?.title || reference} parent={parent} levels={{ ...place.levels, [parent]: consoleName }}>
            <PlaceBody place={place}>{body}</PlaceBody>
            <GuideSection guides={SAVES_GUIDES} />
        </SettingsPage>
    );
}
