import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";
import {
    fetchMetadataSettings, listMetadataServices, removeMetadataService, saveMetadataService, saveMetadataSettings,
} from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { toast } from "../../shell/Shell.jsx";
import { Form, formContext, FormButton } from "../../ui/Forms.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { DangerSection, InfoRow, Listing, Note, PlaceholderRow, Rows, Section, SubmitRow, SwitchRow, TextRow } from "../../ui/Bricks.jsx";

// LES PAGES D'ADMINISTRATION PORTÉES DE LA WEBAPP (profile/settings/admin) : le même
// comportement et les mêmes routes d'API, à la manière du launcher — une page des réglages
// chacune (SettingsPage.jsx), composée des briques de la direction C (Bricks.jsx) : ses groupes, ses
// lignes, ses champs, sa recherche et sa pagination ceux de Skeleton (Forms.jsx), la croix y circule, FR/EN.
// Un geste qui retire quelque chose est une ligne de « Danger », en bas de sa page, et se confirme dans la
// fenêtre de Skeleton, comme dans la webapp ; un geste qui n'appartient à aucun groupe est une ligne de « Plus »
// (le porteur, 2026-11-13 : « Les boutons orphelins […] ne doivent pas être des boutons mais des liens »).

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
const METADATA = "/settings/metadata";
export const METADATA_SERVICES = [
    { key: "igdb", icon: "bi-joystick", tone: "indigo", fields: [["client_id", false], ["client_secret", true]] },
    { key: "steamgriddb", icon: "bi-grid-3x3-gap", tone: "indigo", fields: [["api_key", true]] },
    { key: "libretroDatabase", icon: "bi-database", tone: "slate" },
    { key: "libretroThumbnails", icon: "bi-images", tone: "slate" },
    { key: "embedded", icon: "bi-file-earmark-binary", tone: "slate" },
    { key: "servers", icon: "bi-arrow-left-right", tone: "green" },
];

// Les comptes que le service des comptes connaît (`{service, configured, fields, required}`, jamais une valeur) ; null
// quand la lecture échoue.
const readMetadataServices = () => listMetadataServices().then((answer) => (answer.ok ? answer.items || [] : null));
const accountState = (status, t) => (status ? t(status.configured ? "settings.configured" : "settings.notConfigured") : null);

function MetadataListPage() {
    const { t } = useTranslation();
    const { servers } = useData();
    const { open } = useSettingsNavigation();
    // La valeur d'une source sans compte : « Actif » ; celle des serveurs fédérés, leur nombre.
    const activeOf = (service) => (service.key !== "servers" ? t("settings.metadataActive")
        : (servers || []).length ? t("settings.metadataServersCount", { count: servers.length }) : t("settings.metadataServersNone"));
    const [read, update] = useQueryState();
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
        <SettingsPage title={t("settings.items.metadata.label")}>
            <Section title={t("settings.metadataServices")} help={t("settings.metadataServicesHelp")}>
                <Listing items={METADATA_SERVICES} loading={statuses === undefined} page={read("page")} onPage={(page) => update({ page: page > 1 ? page : null })}
                    placeholderRow={(index) => <PlaceholderRow chevron={Boolean(METADATA_SERVICES[index]?.fields)} />}
                    render={(service) => {
                        const label = t(`settings.metadata.${service.key}.label`);
                        const help = t(`settings.metadata.${service.key}.summary`);
                        const key = `md-${service.key}`;
                        const status = (statuses || []).find((entry) => entry.service === service.key) || null;

                        return service.fields ? (
                            <NavRow key={key} id={key} icon={service.icon} tone={service.tone} label={label} help={help} value={accountState(status, t)}
                                onOpen={() => open(`${METADATA}/${service.key}`, key, { status })} />
                        ) : <InfoRow key={key} icon={service.icon} tone={service.tone} label={label} help={help} value={activeOf(service)} />;
                    }} />
            </Section>
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
function MetadataStorage() {
    const { t } = useTranslation();
    const [onGameDisk, setOnGameDisk] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchMetadataSettings().then((answer) => !cancelled && setOnGameDisk(answer.ok ? answer.item?.write_on_game_disk !== false : true));

        return () => {
            cancelled = true;
        };
    }, []);

    const change = async (on) => {
        const before = onGameDisk;
        setOnGameDisk(on);
        const answer = await saveMetadataSettings({ write_on_game_disk: on });

        if (!answer.ok) {
            setOnGameDisk(before);
            toast(answer.detail || t("settings.saveFailed"));
        }
    };

    return (
        <Section title={t("settings.metadataStorageTitle")}>
            <Rows>
                <SwitchRow id="md-on-game-disk" label={t("settings.metadataOnGameDisk")} help={t("settings.metadataOnGameDiskHelp")} checked={onGameDisk !== false}
                    disabled={onGameDisk === undefined} onChange={change} />
            </Rows>
        </Section>
    );
}

// LES CLÉS D'UN SERVICE : un champ par clé, « Enregistrer » dans l'encadré. Rien de tapé, rien de changé
// (CODE_CONVENTIONS.md, `isReplaced`) : un champ vide garde sa valeur. Une clé exigée qui n'est ni enregistrée ni tapée
// garde le bouton au repos — le service des comptes refuserait (422).
// `save` (service, champs) → {ok, items} : le service des comptes ici ; gameflex-federation passe le sien (chantier 61,
// P3 : ses propres comptes, dans sa base).
export function MetadataKeysForm({ service, status, onSaved, save: store = saveMetadataService }) {
    const { t } = useTranslation();
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
        const answer = await store(service.key, typed);
        setPending(false);

        if (answer.ok) {
            setValues(blank());
            onSaved(answer.items);
            toast(t("settings.metadataSaved"));
        } else {
            toast(t("settings.metadataSaveFailed"));
        }
    };

    return (
        <Form props={{ onSubmit: (event) => {
            event.preventDefault();

            if (isReplaced && !missing.length && !pending) {
                save();
            }
        } }}>
            <Section title={t("settings.metadataKeys")} help={t(`settings.metadata.${service.key}.help`)}>
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

// /settings/metadata/<service> : un service à compte. Son état vient de la liste qui l'a ouvert, sinon il se relit (un
// lien direct, un rechargement). Un service sans compte, ou inconnu, n'a pas de page.
function MetadataServicePage() {
    const { t } = useTranslation();
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
        <SettingsPage title={name} parent={METADATA}>
            {!service ? <NotFound /> : status === undefined ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : (
                <>
                    <MetadataKeysForm service={service} status={status} onSaved={saved} />
                    <DangerSection id="md-remove" label={t("settings.metadataRemove")} help={t(stored ? "settings.metadataRemoveHelp" : "settings.metadataNothingStored", { name })}
                        question={t("settings.metadataRemoveConfirm", { name })} disabled={!stored} onConfirm={async () => {
                            const answer = await removeMetadataService(service.key);

                            if (answer.ok) {
                                toast(t("settings.metadataRemoved", { name }));
                                back(METADATA);
                            } else {
                                toast(t("settings.actionFailed"));
                            }
                        }} />
                </>
            )}
        </SettingsPage>
    );
}

export function MetadataRoutes() {
    return (
        <Routes>
            <Route index element={<MetadataListPage />} />
            <Route path=":service" element={<MetadataServicePage />} />
            <Route path="*" element={<Navigate to={METADATA} replace />} />
        </Routes>
    );
}
