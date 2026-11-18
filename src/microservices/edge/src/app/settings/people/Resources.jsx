import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";
import { listDisks, rescanDisk } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { EmptyState, GuideSection, InfoRow, Listing, Note, Rows, Section } from "../../ui/Bricks.jsx";

// LES RESSOURCES : LES DISQUES INTERNES SEULEMENT (le porteur, 2026-11-14 : « on gère que les ressources internes,
// c'est-à-dire les disques ») — InternalDisks.jsx de la webapp. Une page, /settings/resources (`?disksPage=`), la liste
// feuilletée (Listing ; « Rajouter des pagination pour les listes asynchrone ») : les disques déclarés au compose
// (`/mnt/<nom>`) ; un disque s'ouvre sur sa page (/settings/resources/disks/<nom>) — son nombre de jeux en sous-titre,
// ses jeux par console, le relire.
//
// LES RESSOURCES DISTANTES (Samba, WebDAV, kDrive, FTP, SFTP) NE SONT PAS ICI : gameflex-federation les lit, et un
// appareil le rejoint comme un serveur fédéré (chantier 61). Le montage par appareil est parti (P6).
//
// Les pages se composent des briques de la direction C (Bricks.jsx) ; relire un disque est une ligne de « Plus ».

// LES DISQUES INTERNES : une ligne par disque, qui ouvre sa page ; une page à la fois (`?disksPage=`).
function Disks() {
    const { t } = useTranslation();
    const { capabilities } = useData();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ disksPage: "1" });
    const [disks, setDisks] = useState(null);
    const device = capabilities.internalDisks === "device";
    const text = (key, values) => t(`resources.disks.${key}${device ? "Device" : ""}`, values);

    useEffect(() => {
        listDisks().then((answer) => {
            if (!answer.ok) {
                toast(answer.errorMessage || t("resources.disks.loadFailed"));
            }
            setDisks(answer.ok ? answer.data || [] : []);
        });
    }, [t]);

    return (
        <Section title={t("resources.disks.title")} help={text("intro")}>
            <Listing items={disks || []} loading={disks === null} empty={text("empty")} page={read("disksPage")} onPage={(page) => update({ disksPage: page })}
                render={(disk) => (
                    <NavRow key={disk.name} id={`dk-${disk.name}`} icon={disk.present ? "bi-device-hdd" : "bi-exclamation-triangle"} tone={disk.present ? "indigo" : "amber"} label={disk.name}
                        help={disk.present ? t("resources.disks.present", { count: diskGames(disk) }) : text("absent")}
                        onOpen={() => open(`/settings/resources/disks/${encodeURIComponent(disk.name)}`, `dk-${disk.name}`, { disk })} />
                )} />
        </Section>
    );
}

const diskGames = (disk) => Object.values(disk.consoles || {}).reduce((sum, count) => sum + count, 0);

// UN DISQUE INTERNE : présent ou non, ses jeux par console, et le relire (une ligne de « Plus », grisée avec sa raison
// quand le disque est absent). Le disque : celui que la liste a passé, sinon relu (un lien direct, un rechargement).
function DiskPage() {
    const { t } = useTranslation();
    const { name } = useParams();
    const location = useLocation();
    const { capabilities, consoles } = useData();
    const { open } = useSettingsNavigation();
    const passed = location.state?.disk;
    const [disk, setDisk] = useState(() => (passed && passed.name === name ? passed : undefined));
    const [pending, setPending] = useState(false);
    const device = capabilities.internalDisks === "device";
    const text = (key, values) => t(`resources.disks.${key}${device ? "Device" : ""}`, values);
    const nameOf = (id) => (consoles || []).find((entry) => entry.id === id)?.name || id;
    // Une console que les Réglages connaissent (une console émulée du registre : Réglages › Consoles la liste) s'ouvre ;
    // un dossier qui n'en nomme aucune reste un renseignement.
    const known = (id) => (consoles || []).some((entry) => entry.id === id && entry.method !== "remoteplay");

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
            <SettingsPage title={name} parent="/settings/resources">
                {disk === false ? <NotFound /> : <Section><Note className="mt-0">{t("settings.loading")}</Note></Section>}
            </SettingsPage>
        );
    }
    const perConsole = Object.entries(disk.consoles || {}).filter(([, count]) => count > 0).sort((a, b) => b[1] - a[1]);

    // LE NOMBRE DE JEUX EN SOUS-TITRE, ET NON À CÔTÉ (le porteur, 2026-11-14 : « quand on ouvre une ressource interne,
    // mettre le nombre de jeux en sous-titre et non à côté ») : celui de chaque console sous son nom. Celui du disque
    // était sous son titre, jusqu'à ce que l'en-tête perde son sous-titre (le porteur, le même jour : l'en-tête ne porte
    // que le rond, le titre et le fil) : la page n'a pas de « Détails », il est le nombre que dit sa liste.
    //
    // UNE CONSOLE OUVRE SA PAGE DES RÉGLAGES (le porteur, 2026-11-14, chantier 53, P13 : « depuis Resources > disque.
    // Quand on clique sur une console, ça peut nous ramener sur la fiche console directement ? ») : Réglages › Consoles ›
    // la console (/settings/consoles/<id>), son fil d'Ariane le sien ; Retour revient au disque (l'historique).
    return (
        <SettingsPage title={disk.name} parent="/settings/resources">
            <Section title={t("resources.disks.contentsTitle")}>
                {!disk.present ? <EmptyState icon="bi-exclamation-triangle" text={text("absent")} /> : (
                    <Listing items={perConsole} found={t("resources.disks.present", { count: diskGames(disk) })} empty={t("resources.contentsEmpty")}
                        render={([id, count]) => (known(id)
                            ? <NavRow key={id} id={`dk-console-${id}`} icon="bi-joystick" tone="indigo" label={nameOf(id)} help={t("resources.disks.present", { count })}
                                onOpen={() => open(`/settings/consoles/${encodeURIComponent(id)}`, `dk-console-${id}`)} />
                            : <InfoRow key={id} icon="bi-joystick" tone="slate" label={nameOf(id)} value={t("resources.disks.present", { count })} />)} />
                )}
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="dk-rescan" icon="bi-arrow-repeat" tone={disk.present ? "indigo" : "slate"} label={t("resources.disks.rescan")}
                        help={disk.present ? t("resources.disks.rescanHelp") : text("absent")} disabled={!disk.present} pending={pending} opens="action" onOpen={rescan} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}

// La liste des disques, puis le guide : comment un disque s'ajoute — au compose d'un serveur, ou sur le téléphone.
function ResourcesPage() {
    const { t } = useTranslation();
    const { capabilities } = useData();

    return (
        <SettingsPage title={t("settings.items.resources.label")}>
            <Disks />
            <GuideSection guides={[capabilities.internalDisks === "device" ? ["android", "put-your-games-on-the-phone"] : ["storage", "more-disks-of-games"]]} />
        </SettingsPage>
    );
}

export function ResourcesRoutes() {
    return (
        <Routes>
            <Route index element={<ResourcesPage />} />
            <Route path="disks/:name" element={<DiskPage />} />
            <Route path="*" element={<Navigate to="/settings/resources" replace />} />
        </Routes>
    );
}
