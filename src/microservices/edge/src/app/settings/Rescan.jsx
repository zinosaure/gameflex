import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { fetchListing, rescanListing } from "../libs/api.js";
import { toast } from "../shell/Shell.jsx";
import SettingsPage from "./SettingsPage.jsx";
import { GuideSection } from "../bricks/GuideSection.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";

// LES PAGES D'ADMINISTRATION PORTÉES DE LA WEBAPP (profile/settings/admin) : le même
// comportement et les mêmes routes d'API, à la manière du launcher — une page des réglages
// chacune (SettingsPage.jsx), composée des briques de la direction C (bricks/) : ses groupes, ses
// lignes, ses champs, sa recherche et sa pagination ceux de Skeleton (Forms.jsx), la croix y circule, FR/EN.
// Un geste qui retire quelque chose est une ligne de « Danger », en bas de sa page, et se confirme dans la
// fenêtre de Skeleton, comme dans la webapp ; un geste qui n'appartient à aucun groupe est une ligne de « Plus »
// (le porteur, 2026-11-13 : « Les boutons orphelins […] ne doivent pas être des boutons mais des liens »).

const dateTime = (iso, language) => (iso ? new Date(iso).toLocaleString(language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "");

// TOUT RESCANNER (40 des modèles) : le listing de cette machine, sa date en aide, et le geste qui le refait — une
// ligne de son groupe, pas un bouton ; son tourniquet à la place de la valeur le temps du travail.
export function RescanPage() {
    const { t, i18n } = useTranslation();
    const [updatedAt, setUpdatedAt] = useState(undefined);
    const [pending, setPending] = useState(false);

    useEffect(() => {
        fetchListing().then((answer) => setUpdatedAt(answer.ok ? answer.data?.updatedAt || null : null));
    }, []);

    const rescan = async () => {
        setPending(true);
        const answer = await rescanListing();
        setPending(false);

        if (answer.ok) {
            setUpdatedAt(answer.data?.updatedAt || null);
            toast(t("settings.rescanned"));
        } else {
            toast(answer.errorMessage || t("settings.actionFailed"));
        }
    };

    return (
        <SettingsPage title={t("settings.items.rescan.label")}>
            <Section title={t("settings.listingTitle")}
                help={updatedAt === undefined ? t("settings.loading") : updatedAt ? t("settings.listingFrom", { at: dateTime(updatedAt, i18n.language) }) : t("settings.listingNever")} sense="info">
                <Rows>
                    <NavRow id="sc-rescan" icon="bi-arrow-repeat" label={t("settings.rescanAll")} help={t("settings.rescanAllHelp")} pending={pending} opens="action"
                        onOpen={rescan} />
                </Rows>
            </Section>
            <GuideSection guides={[["library", "4-tell-gameflex-to-look"]]} />
        </SettingsPage>
    );
}
