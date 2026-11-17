import React, { useCallback, useEffect, useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import { listMetadataServices, saveMetadataService, removeMetadataService } from "../../api.js";
import { SectionPanel, FormButton } from "../../../shared/templates/Essentials.jsx";

const Form = Skeleton.Form;

// LES COMPTES DES SERVICES DE MÉTADONNÉES (chantier 49, #659) — IGDB et
// SteamGridDB : ce qui donne aux jeux les fonds, jaquettes et logos qu'ils ne
// portent pas. ScreenScraper est parti (payant, le porteur, 2026-11-13).
//
// Les valeurs entrent et ne ressortent jamais : le service d'identité ne rend
// que les champs remplis. Un champ déjà enregistré s'affiche donc vide, avec
// « Enregistré » en indication ; le laisser vide garde sa valeur, le remplir
// la remplace.
const SERVICES = {
    igdb: {
        icon: "bi bi-controller",
        fields: [
            { name: "client_id", secret: false },
            { name: "client_secret", secret: true },
        ],
    },
    steamgriddb: {
        icon: "bi bi-grid-3x3-gap",
        fields: [{ name: "api_key", secret: true }],
    },
};

function ServiceSection({ service, status, onSaved, context, token, t }) {
    const spec = SERVICES[service];
    const blank = () => Object.fromEntries(spec.fields.map((field) => [field.name, ""]));
    const [values, setValues] = useState(blank);
    const [isSaving, setSaving] = useState(false);
    const [isRemoving, setRemoving] = useState(false);
    const filled = status?.fields || [];
    const isDirty = Object.values(values).some((value) => value.trim() !== "");
    const missing = (status?.required || []).filter((field) => !filled.includes(field) && !(values[field] || "").trim());

    const contextForm = {
        values,
        setValues,
        files: {},
        setFiles: () => { },
        rulesets: {},
    };

    const save = async () => {
        setSaving(true);
        try {
            const result = await saveMetadataService(token, service, values);
            if (result.ok) {
                setValues(blank());
                onSaved(result.items || []);
                context.toast.open(t("admin.metadataServices.saved"), { variant: "success" });
            } else {
                context.toast.open(t("admin.metadataServices.saveFailed"), { variant: "error" });
            }
        } catch (failure) {
            console.error("enregistrement d'un service de métadonnées", failure);
            context.toast.open(t("admin.metadataServices.saveFailed"), { variant: "error" });
        } finally {
            setSaving(false);
        }
    };

    const remove = async () => {
        setRemoving(true);
        try {
            const result = await removeMetadataService(token, service);
            if (result.ok) {
                onSaved(result.items || []);
                context.toast.open(t("admin.metadataServices.removed"), { variant: "success" });
            } else {
                context.toast.open(t("admin.metadataServices.saveFailed"), { variant: "error" });
            }
        } finally {
            setRemoving(false);
        }
    };

    return (
        <SectionPanel
            label={t(`admin.metadataServices.${service}.label`)}
            helpText={t(`admin.metadataServices.${service}.help`)}>
            <div className="mb-3">
                {status?.configured ? (
                    <bs.Badge bg="success"><i className="bi bi-check-circle-fill me-1" />{t("admin.metadataServices.configured")}</bs.Badge>
                ) : (
                    <bs.Badge bg="secondary"><i className={`${spec.icon} me-1`} />{t("admin.metadataServices.notConfigured")}</bs.Badge>
                )}
            </div>
            {spec.fields.map((field) => {
                const placeholder = filled.includes(field.name) ? t("admin.metadataServices.stored") : "";
                return (
                    <Form.Group
                        key={field.name}
                        label={t(`admin.metadataServices.${service}.${field.name}`)}
                        helpText={t(`admin.metadataServices.${service}.${field.name}Help`)}>
                        {field.secret ? (
                            <Form.Password name={field.name} context={contextForm} props={{ placeholder, autoComplete: "new-password" }} />
                        ) : (
                            <Form.Text name={field.name} context={contextForm} props={{ placeholder, autoComplete: "off" }} />
                        )}
                    </Form.Group>
                );
            })}
            <FormButton
                callback={save}
                label={t("app.save")}
                isDisabled={isSaving || !isDirty || missing.length > 0}
                isPending={isSaving} />
            {filled.length > 0 && (
                <FormButton
                    callback={remove}
                    label={t("admin.metadataServices.remove")}
                    variant="danger"
                    isDisabled={isRemoving}
                    isPending={isRemoving} />
            )}
        </SectionPanel>
    );
}

export default function MetadataServices({ context, token, t }) {
    const [statuses, setStatuses] = useState(null);

    const load = useCallback(() => {
        listMetadataServices(token).then((result) => {
            if (!result.ok) {
                context.toast.open(t("admin.metadataServices.loadFailed"), { variant: "error" });
                setStatuses([]);
                return;
            }
            setStatuses(result.items || []);
        }).catch(() => setStatuses([]));
    }, [token]);

    useEffect(() => {
        load();
    }, [load]);

    if (statuses === null) {
        return (
            <bs.Container className="py-2 text-center">
                <bs.Spinner animation="border" role="status" />
            </bs.Container>
        );
    }

    const byService = Object.fromEntries(statuses.map((status) => [status.service, status]));

    return (
        <>
            {Object.keys(SERVICES).map((service) => (
                <ServiceSection
                    key={service}
                    service={service}
                    status={byService[service]}
                    onSaved={setStatuses}
                    context={context}
                    token={token}
                    t={t} />
            ))}
        </>
    );
}
