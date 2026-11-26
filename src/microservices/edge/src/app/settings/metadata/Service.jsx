// Route : /settings/metadata/<service> — un fournisseur et ses clés.
import { useFeedback } from "../../bricks/useFeedback.jsx";
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import { Section } from "../../bricks/Section.jsx";
import { removeMetadataService } from "../../libs/api.js";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../SettingsPage.jsx";
import { METADATA, METADATA_SERVICES, MetadataKeysForm, readMetadataServices } from "../Metadata.jsx";

// /settings/metadata/<service> : un service à compte. Son état vient de la liste qui l'a ouvert, sinon il se relit (un
// lien direct, un rechargement). Un service sans compte, ou inconnu, n'a pas de page.
export default function MetadataService() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { service: key } = useParams();
    const location = useLocation();
    const { back } = useSettingsNavigation();
    const service = METADATA_SERVICES.find((entry) => entry.key === key && entry.fields);
    const passed = location.state?.status;
    const [status, setStatus] = useState(() => (passed?.service === key ? passed : undefined));
    const name = service ? t(`settings.metadata.${service.key}.label`) : key;
    const stored = (status?.fields || []).length > 0;

    useEffect(() => {
        let cancelled = false;

        if (service && status === undefined) {
            readMetadataServices().then((items) => {
                if (cancelled) {
                    return;
                }

                if (items === null) {
                    toast(t("settings.metadataLoadFailed"));
                }
                setStatus((items || []).find((entry) => entry.service === key) || null);
            });
        }

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [key]);

    // La réponse d'un enregistrement est la liste relue : l'état du service en vient.
    const saved = (items) => {
        const next = (items || []).find((entry) => entry.service === key);

        if (next) {
            setStatus(next);
        }
    };

    return (
        <SettingsPage title={name} parent={METADATA} guides={[["library", "if-something-is-missing"]]}>
            {!service ? <NotFound /> : status === undefined ? <Section><Loading /></Section> : (
                <>
                    <MetadataKeysForm service={service} status={status} onSaved={saved} />
                    <DangerSection id="md-remove" label={t("settings.metadataRemove")} help={t(stored ? "settings.metadataRemoveHelp" : "settings.metadataNothingStored", { name })}
                        question={t("settings.metadataRemoveConfirm", { name })} disabled={!stored} onConfirm={async () => {
                            const answer = await removeMetadataService(service.key);

                            if (answer.ok) {
                                feedback.success(t("settings.metadataRemoved", { name }));
                                back(METADATA);
                            } else {
                                feedback.failed(t("settings.actionFailed"));
                            }
                        }} />
                </>
            )}
        </SettingsPage>
    );
}
