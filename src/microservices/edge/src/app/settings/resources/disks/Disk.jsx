// Route : /settings/resources/disks/<nom> — un disque.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import { EmptyState } from "../../../bricks/EmptyState.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { rescanDisk, setDiskEnabled } from "../../../libs/api.js";
import { toast } from "../../../shell/Shell.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { ConsoleList } from "../../Consoles.jsx";
import { SaveSection } from "../../../bricks/SaveSection.jsx";
import { SwitchRow } from "../../../bricks/SwitchRow.jsx";
import { useDiskText, useDisks } from "../../Resources.jsx";

// UN DISQUE INTERNE : présent ou non, ses jeux par console, et le relire (une ligne de « Plus », grisée avec sa raison
// quand le disque est absent). Le disque : celui que la liste a passé, puis celui que le sujet `disks` repousse — branché,
// débranché ou coupé, la page le suit (chantier 86).
export default function Disk() {
    const { t } = useTranslation();
    const { name } = useParams();
    const location = useLocation();
    const { open } = useSettingsNavigation();
    const passed = location.state?.disk;
    const disks = useDisks();
    const disk = disks ? disks.find((entry) => entry.name === name) || false : passed && passed.name === name ? passed : undefined;
    const [pending, setPending] = useState(false);
    const text = useDiskText();

    const rescan = async () => {
        setPending(true);
        const answer = await rescanDisk(name);
        setPending(false);
        toast(answer.ok ? t("resources.disks.rescanned", { name }) : answer.errorMessage || t("resources.disks.rescanFailed"));
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
            {/* COUPER LE DISQUE (chantier 86 ; le porteur, 2026-11-26 : « savoir si je peux temporairement débrancher un
                disque ») : coupé, GameFlex ne s'en sert plus et il peut se débrancher ; rebranché et réactivé, ses jeux
                reviennent. Refusé tant qu'une partie s'en sert. Seulement quand l'hôte dit l'état du disque (`enabled`). */}
            {typeof disk.enabled === "boolean" && (
                <SaveSection id={`dk-switch-${disk.name}`} title={t("resources.disks.switchTitle")} saved={{ enabled: disk.enabled }}
                    onSave={({ enabled }) => setDiskEnabled(disk.name, enabled)} success={({ enabled }) => t(enabled ? "resources.disks.switchedOn" : "resources.disks.switchedOff", { name: disk.name })}>
                    {(values, set) => (
                        <SwitchRow id={`dk-switch-${disk.name}-on`} label={t("resources.disks.switchLabel")} help={t("resources.disks.switchHelp")}
                            checked={values.enabled} onChange={(enabled) => set({ enabled })} />
                    )}
                </SaveSection>
            )}
            {!disk.present ? <Section title={t("resources.disks.contentsTitle")} sense="info"><EmptyState icon="bi-exclamation-triangle" text={disk.enabled === false ? t("resources.disks.disabled") : text("absent")} /></Section> : (
                <ConsoleList countOf={(id) => disk.consoles?.[id] || 0}
                    onOpen={(entry, rowId) => open(`/settings/consoles/${encodeURIComponent(entry.id)}`, rowId)} />
            )}
            <Section title={t("more")} sense="modify">
                <Rows>
                    <NavRow id="dk-rescan" icon="bi-arrow-repeat" label={t("resources.disks.rescan")}
                        help={disk.present ? t("resources.disks.rescanHelp") : disk.enabled === false ? t("resources.disks.disabled") : text("absent")} disabled={!disk.present} pending={pending} opens="action" onOpen={rescan} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}
