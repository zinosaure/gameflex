// Route : /settings/resources/disks/<nom> — un disque.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import { EmptyState } from "../../../bricks/EmptyState.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { listDisks, rescanDisk } from "../../../libs/api.js";
import { useData } from "../../../libs/data.jsx";
import { toast } from "../../../shell/Shell.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { ConsoleList } from "../../Consoles.jsx";

// UN DISQUE INTERNE : présent ou non, ses jeux par console, et le relire (une ligne de « Plus », grisée avec sa raison
// quand le disque est absent). Le disque : celui que la liste a passé, sinon relu (un lien direct, un rechargement).
export default function Disk() {
    const { t } = useTranslation();
    const { name } = useParams();
    const location = useLocation();
    const { capabilities } = useData();
    const { open } = useSettingsNavigation();
    const passed = location.state?.disk;
    const [disk, setDisk] = useState(() => (passed && passed.name === name ? passed : undefined));
    const [pending, setPending] = useState(false);
    const device = capabilities.internalDisks === "device";
    const text = (key, values) => t(`resources.disks.${key}${device ? "Device" : ""}`, values);

    const load = () => listDisks().then((answer) => {
        const found = (answer.ok ? answer.data || [] : []).find((entry) => entry.name === name);
        setDisk(found || false);
    });

    useEffect(() => {
        if (disk === undefined) {
            load();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [name]);

    const rescan = async () => {
        setPending(true);
        const answer = await rescanDisk(name);
        setPending(false);
        toast(answer.ok ? t("resources.disks.rescanned", { name }) : answer.errorMessage || t("resources.disks.rescanFailed"));

        if (answer.ok) {
            load();
        }
    };

    if (!disk) {
        return (
            <SettingsPage title={name} parent="/settings/resources/disks" guides={[["storage", "more-disks-of-games"]]}>
                {disk === false ? <NotFound /> : <Section><Loading /></Section>}
            </SettingsPage>
        );
    }

    // LE NOMBRE DE JEUX EN SOUS-TITRE, ET NON À CÔTÉ (le porteur, 2026-11-14 : « quand on ouvre une ressource interne,
    // mettre le nombre de jeux en sous-titre et non à côté ») : celui de chaque console sous son nom. Celui du disque
    // était sous son titre, jusqu'à ce que l'en-tête perde son sous-titre (le porteur, le même jour : l'en-tête ne porte
    // que le rond, le titre et le fil) : la page n'a pas de « Détails », il est le nombre que dit sa liste.
    //
    // UNE CONSOLE OUVRE SA PAGE DES RÉGLAGES (le porteur, 2026-11-14, chantier 53, P13 : « depuis Resources > disque.
    // Quand on clique sur une console, ça peut nous ramener sur la fiche console directement ? ») : Réglages › Consoles ›
    // la console (/settings/consoles/<id>), son fil d'Ariane le sien ; Retour revient au disque (l'historique).
    return (
        <SettingsPage title={disk.name} parent="/settings/resources/disks" guides={[["storage", "more-disks-of-games"]]}>
            {/* SES JEUX PAR CONSOLE : la liste des consoles des Réglages (ConsoleList), le nombre de jeux du disque sur
                chacune ; une console ouvre sa page des Réglages (chantier 53, P13). */}
            {!disk.present ? <Section title={t("resources.disks.contentsTitle")} sense="info"><EmptyState icon="bi-exclamation-triangle" text={text("absent")} /></Section> : (
                <ConsoleList countOf={(id) => disk.consoles?.[id] || 0}
                    onOpen={(entry, rowId) => open(`/settings/consoles/${encodeURIComponent(entry.id)}`, rowId)} />
            )}
            <Section title={t("more")} sense="modify">
                <Rows>
                    <NavRow id="dk-rescan" icon="bi-arrow-repeat" label={t("resources.disks.rescan")}
                        help={disk.present ? t("resources.disks.rescanHelp") : text("absent")} disabled={!disk.present} pending={pending} opens="action" onOpen={rescan} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}
