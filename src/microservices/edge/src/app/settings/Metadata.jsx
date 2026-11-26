// LES PAGES D'ADMINISTRATION PORTÉES DE LA WEBAPP (profile/settings/admin) : le même
// comportement et les mêmes routes d'API, à la manière du launcher — une page des réglages
// chacune (SettingsPage.jsx), composée des briques de la direction C (bricks/) : ses groupes, ses
// lignes, ses champs, sa recherche et sa pagination ceux de Skeleton (Forms.jsx), la croix y circule, FR/EN.
// Un geste qui retire quelque chose est une ligne de « Danger », en bas de sa page, et se confirme dans la
// fenêtre de Skeleton, comme dans la webapp ; un geste qui n'appartient à aucun groupe est une ligne de « Plus »
// (le porteur, 2026-11-13 : « Les boutons orphelins […] ne doivent pas être des boutons mais des liens »).

import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { InfoRow } from "../bricks/InfoRow.jsx";
import { Listing } from "../bricks/Listing.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { PlaceholderRow } from "../bricks/PlaceholderRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { SaveSection } from "../bricks/SaveSection.jsx";
import { Section } from "../bricks/Section.jsx";
import { SubmitRow } from "../bricks/SubmitRow.jsx";
import { SwitchRow } from "../bricks/SwitchRow.jsx";
import { TextRow } from "../bricks/TextRow.jsx";
import { useFeedback } from "../bricks/useFeedback.jsx";
import { fetchMetadataSettings, listMetadataServices, saveMetadataService, saveMetadataSettings } from "../libs/api.js";
import { useData } from "../libs/data.jsx";
import { counted } from "../libs/format.js";
import { toast } from "../shell/Shell.jsx";
import { Form, FormButton, formContext } from "../ui/Forms.jsx";
import SettingsPage, { useSettingsNavigation } from "./SettingsPage.jsx";

// ─── Métadonnées (chantier 53, P10) ─────────────────────────────────────────
// LES SERVICES RÉELLEMENT ÉCRITS dans le kernel (internal/metadata), et rien d'autre (décision 12 du chantier 53) :
//   - IGDB et SteamGridDB, qui demandent un compte (igdb.go, steamgriddb.go ; remote.go ne les interroge pas sans lui) ;
//   - libretro-database, le texte de la fiche d'un jeu rétro (libretro_database.go, info.go) ;
//   - libretro-thumbnails, des jaquettes, des captures, des écrans-titres et des logos (libretro.go) ;
//   - les fichiers du jeu, l'icône et le fond que porte un jeu PS3, PS4 ou PS5 (embedded.go).
// ScreenScraper est parti (le porteur, 2026-11-13 : « Oui, en plus c'est payant »). Aucune route ne liste les services
// sans compte : ils sont déclarés ici, et cette liste suit le kernel quand il en gagne ou en perd un.
//   - les serveurs fédérés (chantier 57, P3) : une source de plus pour la recherche des fiches de mes jeux, après les
//     fournisseurs. Leur ligne dit combien il y en a (`/api/v1/servers`, la liste que garde la page), sans page à ouvrir :
//     ils se rejoignent dans Réglages › Fédération.
//
// /settings/metadata, « Fournisseur de métadonnées » (le porteur, 2026-11-14 : « Remplace "metadata" par "metadata
// provider" ») : la liste (35 des modèles) — un service à compte dit s'il est configuré et ouvre sa page ; un service
// sans compte se lit sur sa ligne, sans page à ouvrir (les modèles n'en dessinent pas) —, puis « Où les garder » :
// l'interrupteur « Écrire les métadonnées sur le disque du jeu » (MetadataStorage).
// /settings/metadata/<service> : ses clés (36), puis « Danger » — retirer le compte, dans la fenêtre de Skeleton. LES
// VALEURS ENTRENT ET NE RESSORTENT JAMAIS : un champ déjà enregistré s'affiche vide, « Enregistré » en indication ; le
// laisser vide garde sa valeur, le remplir la remplace, et seuls les champs remplis partent.
export const METADATA = "/settings/metadata";
export const METADATA_SERVICES = [
    { key: "igdb", icon: "bi-joystick", fields: [["client_id", false], ["client_secret", true]] },
    { key: "steamgriddb", icon: "bi-grid-3x3-gap", fields: [["api_key", true]] },
    { key: "libretroDatabase", icon: "bi-database" },
    { key: "libretroThumbnails", icon: "bi-images" },
    { key: "embedded", icon: "bi-file-earmark-binary" },
    { key: "servers", icon: "bi-arrow-left-right" },
];

// Ceux qui demandent un compte (des champs à remplir), et ceux qui servent tels quels.
export const CONFIGURED = METADATA_SERVICES.filter((service) => service.fields);
export const INCLUDED = METADATA_SERVICES.filter((service) => !service.fields);

// Les comptes que le service des comptes connaît (`{service, configured, fields, required}`, jamais une valeur) ; null
// quand la lecture échoue.
export const readMetadataServices = () => listMetadataServices().then((answer) => (answer.ok ? answer.items || [] : null));
export const accountState = (status, t) => (status ? t(status.configured ? "settings.configured" : "settings.notConfigured") : null);

// Route : /settings/metadata — les fournisseurs de métadonnées.
export default function Metadata() {
    const { t } = useTranslation();
    const { servers } = useData();
    const { open } = useSettingsNavigation();
    // La valeur d'une source sans compte : « Actif » ; celle des serveurs fédérés, leur nombre.
    const activeOf = (service) => (service.key !== "servers" ? t("settings.metadataActive") : counted(t, "servers", (servers || []).length));
    const [statuses, setStatuses] = useState(undefined);

    useEffect(() => {
        let cancelled = false;

        readMetadataServices().then((items) => {
            if (cancelled) {
                return;
            }

            if (items === null) {
                toast(t("settings.metadataLoadFailed"));
            }
            setStatuses(items);
        });

        return () => {
            cancelled = true;
        };
    }, [t]);

    return (
        <SettingsPage title={t("settings.items.metadata.label")} guides={[["library", "if-something-is-missing"]]}>
            {/* DEUX SECTIONS (le porteur, 2026-11-25 : « scinder en 2 sections : "services to configure" et "services
                included" ») : ceux qui demandent un compte, qui s'ouvrent et se règlent ; ceux qui servent tels quels, pour
                information — chacune sa couleur. */}
            {[[CONFIGURED, "metadataToConfigure", "modify"], [INCLUDED, "metadataIncluded", "info"]].map(([services, key, sense]) => (
                <Section key={key} title={t(`settings.${key}`)} help={t(`settings.${key}Help`)} sense={sense}>
                    <Listing items={services} loading={statuses === undefined}
                        placeholderRow={(index) => <PlaceholderRow chevron={Boolean(services[index]?.fields)} />}
                        render={(service) => {
                            const label = t(`settings.metadata.${service.key}.label`);
                            const help = t(`settings.metadata.${service.key}.summary`);
                            const id = `md-${service.key}`;
                            const status = (statuses || []).find((entry) => entry.service === service.key) || null;

                            return service.fields ? (
                                <NavRow key={id} id={id} icon={service.icon} label={label} help={help} value={accountState(status, t)} valueTone={status?.configured ? "success" : "danger"}
                                    onOpen={() => open(`${METADATA}/${service.key}`, id, { status })} />
                            ) : <InfoRow key={id} icon={service.icon} label={label} help={help} value={activeOf(service)} pill={service.key === "servers" ? "info" : "success"} />;
                        }} />
                </Section>
            ))}
            <MetadataStorage />
        </SettingsPage>
    );
}

// « ÉCRIRE LES MÉTADONNÉES SUR LE DISQUE DU JEU » (le porteur, 2026-11-14 : « Sur téléphone, quand on rajoute un disque
// NTFS, le disque devient read-only […] rajouter un switch qui demande si on veut écrire les metadata sur le même
// disque où le jeu se trouve. Par défaut c'est checked. ») — un interrupteur qui s'applique aussitôt, gardé par le
// service des comptes avec les comptes des services ; le kernel le suit en dix secondes au plus. Coupé, les métadonnées
// d'un jeu d'un disque se lisent et s'écrivent au dossier du serveur, où un jeu déjà connu retrouve les siennes. Tant
// qu'il se lit, grisé.
export function MetadataStorage() {
    const { t } = useTranslation();
    const [onGameDisk, setOnGameDisk] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchMetadataSettings().then((answer) => !cancelled && setOnGameDisk(answer.ok ? answer.item?.write_on_game_disk !== false : true));

        return () => {
            cancelled = true;
        };
    }, []);

    // SON BOUTON (le porteur, 2026-11-26 : « les sections formulaire n'ont pas de FormButton ») : la réponse en fenêtre
    // (SaveSection). Tant qu'il se lit, grisé.
    return (
        <SaveSection id="md-storage" title={t("settings.metadataStorageTitle")} help={t("settings.metadataStorageHelp")} saved={{ onGameDisk: onGameDisk !== false }}
            disabled={onGameDisk === undefined} onSave={async (values) => {
                const answer = await saveMetadataSettings({ write_on_game_disk: values.onGameDisk });

                if (answer.ok) {
                    setOnGameDisk(values.onGameDisk);
                }

                return answer;
            }}>
            {(values, set) => (
                <SwitchRow id="md-on-game-disk" label={t("settings.metadataOnGameDisk")} help={t("settings.metadataOnGameDiskHelp")} checked={values.onGameDisk}
                    disabled={onGameDisk === undefined} onChange={(on) => set({ onGameDisk: on })} />
            )}
        </SaveSection>
    );
}

// LES CLÉS D'UN SERVICE : un champ par clé, « Enregistrer » dans l'encadré. Rien de tapé, rien de changé
// (CODE_CONVENTIONS.md, `isReplaced`) : un champ vide garde sa valeur. Une clé exigée qui n'est ni enregistrée ni tapée
// garde le bouton au repos — le service des comptes refuserait (422).
export function MetadataKeysForm({ service, status, onSaved }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const blank = () => Object.fromEntries(service.fields.map(([name]) => [name, ""]));
    const [values, setValues] = useState(blank);
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues);
    const stored = status?.fields || [];
    const typed = Object.fromEntries(Object.entries(values).map(([name, value]) => [name, (value || "").trim()]).filter(([, value]) => value));
    const isReplaced = Object.keys(typed).length > 0;
    const missing = (status?.required || []).filter((name) => !stored.includes(name) && !typed[name]);

    const save = async () => {
        setPending(true);
        const answer = await saveMetadataService(service.key, typed);
        setPending(false);

        if (answer.ok) {
            setValues(blank());
            onSaved(answer.items);
            feedback.success(t("settings.metadataSaved"));
        } else {
            feedback.failed(t("settings.metadataSaveFailed"));
        }
    };

    return (
        <Form props={{ onSubmit: (event) => {
            event.preventDefault();

            if (isReplaced && !missing.length && !pending) {
                save();
            }
        } }}>
            <Section title={t("settings.metadataKeys")} help={t(`settings.metadata.${service.key}.help`)} sense="modify">
                <Rows>
                    {service.fields.map(([name, secret]) => (
                        <TextRow key={name} id={name} context={form} kind={secret ? "password" : "text"} label={t(`settings.metadata.${service.key}.${name}`)} help={t(`settings.metadata.${service.key}.${name}Help`)}
                            props={{ autoComplete: secret ? "new-password" : "off", placeholder: stored.includes(name) ? t("settings.stored") : "", "data-key": `md-${name}` }} />
                    ))}
                    <SubmitRow>
                        <FormButton id="md-save" label={t("settings.save")} isReplaced={isReplaced} pending={pending} disabled={missing.length > 0} />
                    </SubmitRow>
                </Rows>
            </Section>
        </Form>
    );
}
