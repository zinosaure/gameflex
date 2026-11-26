// Route : /settings/resources/external/<id> — une ressource externe.
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import React from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { removeResource, rescanResource } from "../../../libs/api.js";
import { useData } from "../../../libs/data.jsx";
import { counted } from "../../../libs/format.js";
import { toast } from "../../../shell/Shell.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { consolesOfResource, EXTERNAL_GUIDES, KIND_OF, resourceGamesPath, STATUS_BADGE, statusOfResource, useExternalResources, when } from "../../Resources.jsx";

// UNE RESSOURCE EXTERNE (la page d'une connexion de gameflex-federation) : ses infos et son statut ; « Plus » — ses jeux,
// la relire, modifier la connexion (une page à part, le porteur, 2026-11-23 : « plus de section dans la page mais […]
// une page profonde ») ; la retirer.
export default function External() {
    const { t, i18n } = useTranslation();
    const feedback = useFeedback();
    const { id } = useParams();
    const navigate = useNavigate();
    const { consoles } = useData();
    const { open } = useSettingsNavigation();
    const list = useExternalResources();
    const resource = (list || []).find((entry) => String(entry.id) === id);

    if (list === undefined || !resource) {
        return (
            <SettingsPage title={resource?.name || t("resources.external.title")} parent="/settings/resources/external" guides={EXTERNAL_GUIDES}>
                {list === undefined ? <Section><Loading /></Section> : <NotFound />}
            </SettingsPage>
        );
    }
    const state = resource.state || {};
    const kind = KIND_OF[resource.kind] || {};
    const status = statusOfResource(t, state);
    const withGames = consolesOfResource(resource, consoles);
    // « JEUX » MÈNE À SES JEUX (le porteur, 2026-11-23) : la page des jeux de la console quand il n'y en a qu'une, la
    // liste de ses consoles sinon.
    const openGames = () => (withGames.length === 1
        ? open(resourceGamesPath(withGames[0].id, resource), "rx-games")
        : open(`/settings/resources/external/${resource.id}/games`, "rx-games"));
    const total = withGames.reduce((sum, entry) => sum + entry.count, 0);

    return (
        <SettingsPage title={resource.name} parent="/settings/resources/external" guides={EXTERNAL_GUIDES}>
            <Section title={t("details")} help={t("resources.external.detailsHelp")} sense="info">
                <Rows>
                    <InfoRow icon={kind.icon} label={t("fedService.fields.kind")} value={t(`fedService.kinds.${resource.kind}.label`)} />
                    <InfoRow icon="bi-link-45deg" label={t(`fedService.kinds.${resource.kind}.address`)} value={resource.address} mono />
                    {resource.folder && <InfoRow icon="bi-folder" label={t("fedService.fields.folder")} value={resource.folder} mono />}
                    <InfoRow icon="bi-activity" label={t("resources.external.statusTitle")}
                        value={<span className={`badge rounded-pill text-bg-${STATUS_BADGE[status.key]}`}>{t(`resources.external.status.${status.key}`)}</span>} help={status.reason} />
                    <InfoRow icon="bi-clock-history" label={t("fedService.connections.lastScan")} value={when(state.scannedAt, i18n.language)} />
                </Rows>
            </Section>
            {/* « CONTENU » (le porteur, 2026-11-25 : « mets games et rescan dans une section "contents", car il faut rajouter en
                deuxième dans contents le lien vers les "saves". Tu peux le mettre en grisé pour le moment ») : ses jeux, ses
                sauvegardes — pas encore lues d'une ressource : désactivée, NYI —, la relire. */}
            <Section title={t("resources.external.contentsTitle")} help={t("resources.external.contentsHelp")} sense="play">
                <Rows>
                    <NavRow id="rx-games" icon="bi-collection-play" label={t("consoles.games")}
                        value={[counted(t, "games", total), withGames.length > 1 ? counted(t, "consoles", withGames.length) : withGames[0]?.name]}
                        disabled={!total} onOpen={openGames} />
                    <NavRow id="rx-saves" icon="bi-shield-check" label={t("catalog.saves.label")} help={t("resources.external.savesNotYet")} disabled onOpen={() => {}} />
                    <NavRow id="rx-rescan" icon="bi-arrow-repeat" label={t("fedService.connections.rescan")} help={t("fedService.connections.rescanHelp")}
                        pending={Boolean(state.scanning)} opens="action" onOpen={async () => {
                            const answer = await rescanResource(resource.id);
                            if (!answer.ok) {
                                toast(answer.errorMessage || t("fedService.failed"));
                            }
                        }} />
                </Rows>
            </Section>
            <Section title={t("more")} help={t("resources.external.moreHelp")} sense="modify">
                <Rows>
                    <NavRow id="rx-edit" icon="bi-pencil" label={t("resources.external.edit")} help={t("resources.external.editHelp")}
                        onOpen={() => open(`/settings/resources/external/${resource.id}/edit`, "rx-edit")} />
                </Rows>
            </Section>
            <DangerSection id="rx-remove" label={t("fedService.connections.remove")} help={t("fedService.connections.removeHelp")}
                question={t("fedService.connections.removeQuestion", { name: resource.name })}
                onConfirm={async () => {
                    const answer = await removeResource(resource.id);
                    feedback.result(answer.ok, t("fedService.connections.removed"), answer.errorMessage || t("fedService.failed"));
                    if (answer.ok) {
                        navigate("/settings/resources/external", { replace: true });
                    }
                }} />
        </SettingsPage>
    );
}
