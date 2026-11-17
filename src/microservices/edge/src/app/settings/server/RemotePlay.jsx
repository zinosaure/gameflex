import React, { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";
import { listConsoleBackgrounds, listDevices, resetConsoleBackgroundSlot, searchConsoleBackgrounds, setConsoleBackground } from "../../libs/api.js";
import { consoleBackgroundPath, forgetConsoleBackgrounds, useImage } from "../../libs/images.js";
import { can, useData } from "../../libs/data.jsx";
import { focusElement } from "../../libs/input.js";
import { deleteDevice, registerDevice } from "../../engine/api/remoteplay.js";
import { playStationService } from "../../engine/stream/remoteplay/service/playstation.service.js";
import { useDeviceStatus } from "../../engine/stream/remoteplay/hooks/use-device-status.js";
import { toast } from "../../shell/Shell.jsx";
import { forgetBand } from "../../screens/library/ConsoleVisual.jsx";
import { ImageSearch } from "./consoles/ImageSearch.jsx";
import { Form, formContext, FormButton, Ruleset } from "../../ui/Forms.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { ChoiceRow, DangerSection, GuideSection, IconAddon, InfoRow, Listing, Note, Rows, Section, SubmitRow, TextRow, Thumb } from "../../ui/Bricks.jsx";

// REMOTE PLAY (administration, « Le serveur ») — la page Remote Play de la webapp (RemotePlay.jsx), portée dans les
// Réglages en direction C (le porteur, 2026-11-13 : « Pour Remote Play tu peux prendre l'exemple dans webapp. C'est
// déjà fait mais ce n'était pas dans le settings auparavant. ») : les mêmes étapes, les mêmes champs, les mêmes appels.
//
//   /settings/remote-play         les consoles enregistrées sur CE serveur (DeviceList), une ligne chacune ;
//   /settings/remote-play/new     en enregistrer une (RegisterPanel) — le seul endroit d'où l'on enregistre une
//                                 console : l'écran Remote Play ne montre que les consoles enregistrées (chantier 53, P12) ;
//   /settings/remote-play/<id>    une console (DeviceDetailPanel) : ses détails, son image de fond, la dissocier ;
//   /settings/remote-play/<id>/background   son image de fond : la remplacer par une recherche, la remettre par défaut.
//
// SEULEMENT LES CONSOLES D'ICI : celles d'un serveur fédéré s'appairent et se règlent chez lui ; le sélecteur de
// source de la webapp (ici / un pair) n'a donc rien à choisir dans un coin d'administration. Jouer une console reste
// l'affaire de l'écran Remote Play (se connecter, l'allumer) ; ni « Jouer » ni « Regarder » ici.
//
// LES RÈGLES DES BOUTONS (le porteur, 2026-11-13) : chaque section a un titre ; Enregistrer est dans l'encadré de son
// (dernier) groupe ; un geste qui n'agit sur aucun groupe est une ligne — en enregistrer une : le groupe « Plus » ; la
// dissocier : la section « Danger ».
//
// LA PAGE D'UNE CONSOLE N'A NI LE FLUX NI LE RÉVEIL (le porteur, 2026-11-14 : « Supprime la section "stream settings"
// et "more" (on n'est pas supposé allumer la console depuis l'admin). Et remplace le texte "Delete this console" par
// "unlink this console" dans la section danger. ») :
//   - le réveil reste là où un joueur lance sa partie : l'écran Remote Play (Platforms.jsx, Allumer) ;
//   - LE FLUX N'EST PAS UNE AFFAIRE D'ADMINISTRATION : chaque utilisateur le choisit dans l'engrenage de la console, sur
//     l'écran Remote Play (Platforms.jsx), il est retenu dans son navigateur et part avec sa connexion (le porteur,
//     2026-11-16 : « C'est l'utilisateur qui choisit »). Le réglage au serveur, sur la ligne de la console, « pour tout
//     le monde », est supprimé avec sa route (`/playstation/device-settings/{id}`) et sa colonne (`config_json`).
//
// L'IMAGE DE FOND (le porteur, 2026-11-14, chantier 53, P12 : « celui-ci affiche un background qu'on choisit dans
// settings / remote play / la console. Ce serait le même mécanisme que pour les autres consoles sauf que là on choisit une
// seule grande image. ») : le premier emplacement du fond de sa console (`rp-ps5`…), rangé et servi comme ceux des autres
// consoles (metadata/console_backgrounds.go), cherché chez les fournisseurs de métadonnées (settings/server/consoles/ImageSearch.jsx) ; la
// « Danger » le remet par défaut. Il vaut pour toutes les consoles de ce type sur ce serveur : le fond est rangé par
// console, pas par appareil, et la page le dit.
//
// L'ÉTAT DES CONSOLES EN DIRECT, comme la webapp : la liste lue une fois (`my-devices`, qui sonde chaque console),
// puis le canal d'état du service (use-device-status.js, pas de sondage) là où l'hôte ouvre des sessions — c'est lui
// qui fait passer une console de « Veille » à « En ligne » quand on l'allume.
//
// LES LISTES LUES EN ASYNCHRONE se feuillettent (Listing ; le porteur, 2026-11-14 : « Rajouter des pagination pour les
// listes asynchrone ») : les consoles d'ici (leur page dans l'adresse, `?page=`), celles que la recherche sur le réseau
// a trouvées — et, le temps de leur lecture ou de la recherche, des lignes fantômes.

const ROOT = "/settings/remote-play";

// Seule la PlayStation a une implémentation (le service `/api/v1/playstation`) : ses deux types, ceux que le service
// sait appairer.
const HOST_TYPES = ["PS4", "PS5"];

// Le site qui donne l'identifiant de compte PSN (Base64) qu'attend l'appairage (RemotePlay.jsx, décidé le 2026-10-10).
const PSN_ACCOUNT_ID_URL = "https://psn.flipscreen.games/";

const STATUS_LOOK = { awake: ["bi-power", "green"], standby: ["bi-moon", "amber"], offline: ["bi-power", "slate"] };
const deviceName = (device) => device.hostName || device.hostId;

// LA LUMIÈRE NE TOMBE PAS : un bouton grisé le temps d'un appel la perd (la recherche, Enregistrer) ; elle
// revient au premier de ces contrôles qui l'accepte, si rien d'autre ne l'a prise entre-temps.
const restoreLight = (...keys) => requestAnimationFrame(() => {
    if (document.activeElement && document.activeElement !== document.body) {
        return;
    }
    focusElement(keys.map((key) => document.querySelector(`[data-key^="${CSS.escape(key)}"]`)).find((element) => element && !element.disabled));
});

// L'état d'une console : sa pastille et son libellé. Une partie en cours (la ligne le dit, le pool des parties le
// confirme) l'emporte sur « En ligne » : quelqu'un y joue.
function useStatus(device) {
    const { t } = useTranslation();
    const { sessions } = useData();
    const busy = Boolean(device?.hasActiveSession) && sessions.some((session) => session.id === device.id);

    if (!device) {
        return { busy: false, label: "", icon: "bi-playstation", tone: "slate" };
    }

    if (busy) {
        return { busy, label: t("remoteplay.device.sessionActive"), icon: "bi-people", tone: "amber" };
    }
    const [icon, tone] = STATUS_LOOK[device.status] || ["bi-question-circle", "slate"];

    return { busy, label: t(`remoteplay.status.${device.status}`, { defaultValue: device.status || "" }), icon, tone };
}

// LES CONSOLES D'ICI : `null` le temps de la lecture, `false` si elle a échoué, puis la liste — tenue à jour par le
// canal d'état du service.
function useDevices() {
    const { t } = useTranslation();
    const { capabilities } = useData();
    const [devices, setDevices] = useState(null);
    const onDevices = useCallback((updated) => setDevices(updated || []), []);

    useEffect(() => {
        let cancelled = false;
        listDevices().then((answer) => {
            if (cancelled) {
                return;
            }

            if (answer.ok) {
                setDevices(answer.data || []);
            } else {
                setDevices((current) => current || false);
                toast(answer.errorMessage || t("remoteplay.loadFailed"));
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    useDeviceStatus({ onDevicesUpdate: onDevices, onStatusUpdate: onDevices, enabled: can(capabilities, "spawnsSessions") });

    return devices;
}

// Une console de la liste : son nom, puis « type · adresse · état ».
function DeviceRow({ device, onOpen }) {
    const status = useStatus(device);

    return (
        <NavRow id={`rp-${device.id}`} icon="bi-playstation" tone="sky" label={deviceName(device)}
            help={[device.hostType, device.ipAddress, status.label].filter(Boolean).join(" · ")} onOpen={onOpen} />
    );
}

// Les consoles d'ici, puis « Plus » : en enregistrer une, la même ligne sous la liste et sous la liste vide (l'état
// vide n'a plus de bouton : son remède est une ligne).
function DevicesPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const devices = useDevices();
    const [read, update] = useQueryState({ page: "1" });

    return (
        <SettingsPage title={t("settings.items.remotePlay.label")}>
            {devices === false ? <Note>{t("remoteplay.loadFailed")}</Note> : (
                <>
                    <Section title={t("rp.devicesTitle")}>
                        <Listing items={devices || []} loading={devices === null} found={t("consoles.found", { count: (devices || []).length })} empty={t("remoteplay.noDevicesFound")}
                            page={read("page")} onPage={(page) => update({ page })}
                            render={(device) => <DeviceRow key={device.id} device={device} onOpen={() => open(`${ROOT}/${encodeURIComponent(device.id)}`, `rp-${device.id}`, { device })} />} />
                    </Section>
                    <Section title={t("more")}>
                        <Rows>
                            <NavRow id="rp-add" icon="bi-plus-lg" tone="indigo" label={t("remoteplay.addDevice")} help={t("remoteplay.addDeviceDesc")} onOpen={() => open(`${ROOT}/new`, "rp-add")} />
                        </Rows>
                    </Section>
                </>
            )}
            <GuideSection guides={[["remotePlay"]]} />
        </SettingsPage>
    );
}

// ENREGISTRER UNE CONSOLE (RegisterPanel) : son type, son adresse — ou la recherche sur le réseau, dont chaque console
// trouvée remplit l'adresse, le nom et le type —, son nom, l'identifiant du compte PSN (le lien qui le donne sous le
// titre de son groupe) et le code PIN que la console affiche ; puis l'appairage (`/playstation/bind`, son bouton dans
// l'encadré du dernier groupe), et la liste.
//
// LE TYPE EST UN CHAMP ICI : la webapp le tirait de la console ouverte dans son menu (`/remoteplay/rp-ps5`) ; ce coin
// d'administration n'en a pas, et le service appairerait sinon une PS5 par défaut.
function RegisterPage() {
    const { t } = useTranslation();
    const { back } = useSettingsNavigation();
    const { refreshDevices } = useData();
    const [values, setValues] = useState(() => ({ hostType: "PS5", ip: "", name: "", identifier: "", pin: "" }));
    const [pending, setPending] = useState(false);
    const [discovering, setDiscovering] = useState(false);
    const [found, setFound] = useState([]);
    const title = t("remoteplay.addDevice");


    // Les règles de RegisterPanel ; une erreur se montre une fois qu'on a écrit dans le champ (un champ vide n'est
    // pas faux, il manque : le bouton reste grisé), comme les autres formulaires des Réglages.
    const problems = {
        ip: values.ip ? "" : t("form.required"),
        name: values.name && values.name.length >= 2 ? "" : t("form.required"),
        identifier: values.identifier ? "" : t("form.required"),
        pin: !values.pin ? t("form.required") : /^\d{1,8}$/.test(String(values.pin)) ? "" : t("remoteplay.register.pinInvalid"),
    };
    const shownProblem = (name) => (data) => (data[name] && problems[name] ? Ruleset.isInvalid(problems[name]) : null);
    const form = formContext(values, setValues, { ip: shownProblem("ip"), name: shownProblem("name"), identifier: shownProblem("identifier"), pin: shownProblem("pin") });
    const invalid = Object.values(problems).some(Boolean);
    // Rien de tapé, rien de changé (CODE_CONVENTIONS.md, `isReplaced`) ; le type choisi seul ne suffit pas à enregistrer.
    const isReplaced = ["ip", "name", "identifier", "pin"].some((name) => values[name]);

    const discover = async () => {
        setDiscovering(true);
        setFound([]);
        let result;

        try {
            result = await playStationService.discoverDevices();
        } catch (error) {
            setDiscovering(false);
            restoreLight("rp-discover");
            toast(error?.message || t("remoteplay.register.discoverFailed"));

            return;
        }
        setDiscovering(false);

        if (!result.success) {
            restoreLight("rp-discover");
            toast(result.errorMessage || t("remoteplay.register.discoverFailed"));

            return;
        }
        setFound(result.data || []);
        restoreLight("rp-found-", "rp-discover");

        if (!(result.data || []).length) {
            toast(t("remoteplay.register.discoverEmpty"));
        }
    };

    // Une console trouvée remplit l'adresse, le nom (s'il est vide) et le type ; la liste se referme, la lumière
    // revient à la recherche (un bouton : le clavier d'un téléphone ne surgit pas).
    const choose = (device) => {
        const hostType = String(device.hostType || "").toUpperCase();
        setValues((previous) => ({ ...previous, ip: device.ip, name: previous.name || device.name || "", hostType: HOST_TYPES.includes(hostType) ? hostType : previous.hostType }));
        setFound([]);
        restoreLight("rp-discover");
    };

    const submit = async () => {
        setPending(true);

        try {
            const result = await registerDevice({ name: values.name, ip: values.ip, identifier: values.identifier, pin: values.pin, hostType: values.hostType });

            if (result.ok) {
                toast(t("remoteplay.register.success"));
                refreshDevices();
                back(ROOT);

                return;
            }
            toast(result.errorMessage || t("remoteplay.register.failure"));
        } catch (failure) {
            console.error("échec de l'enregistrement de l'appareil", failure);
            toast(t("remoteplay.register.failure"));
        }
        setPending(false);
        restoreLight("rp-register");
    };

    return (
        <SettingsPage title={title} parent={ROOT}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (!invalid && !pending) {
                    submit();
                }
            } }}>
                <Section title={t("rp.consoleTitle")}>
                    <Rows>
                        <ChoiceRow id="rp-type" label={t("remoteplay.device.type")} value={values.hostType} options={HOST_TYPES.map((type) => [type, type])}
                            onChange={(hostType) => setValues((previous) => ({ ...previous, hostType }))} />
                        <TextRow id="ip" label={t("remoteplay.register.ip")} help={t("remoteplay.register.ipHelp")} context={form} props={{ placeholder: t("remoteplay.register.ipPlaceholder") }}
                            addon={<IconAddon id="rp-discover" icon={discovering ? "bi-hourglass-split" : "bi-search"} label={t("remoteplay.register.discover")} disabled={discovering} onClick={discover} />} />
                    </Rows>
                </Section>
                {(found.length > 0 || discovering) && (
                    <Section title={t("remoteplay.register.discoverResultsLabel")}>
                        <Listing items={found} loading={discovering} render={(device) => (
                            <NavRow key={device.ip} id={`rp-found-${device.ip}`} icon="bi-playstation" tone="sky" label={device.name || device.ip}
                                help={[device.hostType, device.ip].filter(Boolean).join(" · ")} opens="action" onOpen={() => choose(device)} />
                        )} />
                    </Section>
                )}
                {/* L'IDENTIFIANT PSN S'OBTIENT AILLEURS (RemotePlay.jsx) : le lien du site qui le donne est dans l'aide de son
                    champ, DANS l'encadré — une action qui concerne un groupe y vit (le porteur, 2026-11-13) — et la manette
                    l'atteint. Sur le téléphone, l'adresse s'ouvre dans le navigateur (MainActivity). */}
                <Section title={t("rp.pairingTitle")}>
                    <Rows>
                        <TextRow id="name" label={t("remoteplay.register.name")} help={t("remoteplay.register.nameHelp")} context={form} props={{ placeholder: t("remoteplay.register.namePlaceholder") }} />
                        <TextRow id="identifier" label={t("remoteplay.register.identifierLabel.playstation")} context={form}
                            help={<>{t("remoteplay.register.identifierHint")} <a href={PSN_ACCOUNT_ID_URL} target="_blank" rel="noopener noreferrer" data-nav data-key="rp-psn">{t("remoteplay.register.getViaPsnLogin")}</a></>} />
                        <TextRow id="pin" kind="number" min={0} max={99999999} step={1} label={t("remoteplay.register.pin")} help={t("remoteplay.register.pinHelp")} context={form}
                            props={{ placeholder: t("remoteplay.register.pinPlaceholder") }} />
                        <SubmitRow>
                            <FormButton id="rp-register" label={t("remoteplay.register.submit")} isReplaced={isReplaced} pending={pending} disabled={invalid} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
            <GuideSection guides={[["remotePlay", "add-your-console"]]} />
        </SettingsPage>
    );
}

// LA CONSOLE DU REGISTRE QUI RANGE LE FOND D'UN APPAREIL : `rp-ps5` pour une PS5.
const backgroundConsole = (device) => `rp-${String(device?.hostType || "").toLowerCase()}`;

// LE PREMIER EMPLACEMENT DU FOND D'UNE CONSOLE REMOTE PLAY : `{name, server, serverName}` — son nom (« 01.jpg », "" s'il
// est libre) et le serveur fédéré qui le prête quand cette console n'a aucune image ici (`#740`), avec son nom —,
// `undefined` tant qu'on ne sait pas, `null` si la lecture a échoué. `epoch` : le relire après un geste.
function useRemoteBackground(device, epoch = 0) {
    const consoleId = backgroundConsole(device);
    const [background, setBackground] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        setBackground(undefined);
        listConsoleBackgrounds(consoleId).then((answer) => {
            if (!cancelled) {
                setBackground(answer.ok
                    ? { name: (answer.data?.slots || []).find((slot) => slot.slot === 1)?.name || "", server: answer.data?.server || null, serverName: answer.data?.serverName || "" }
                    : null);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [consoleId, epoch]);

    return background;
}

// L'image en grand (« L'image actuelle »), ou sa vignette en tête d'une ligne.
function RemoteBackgroundImage({ consoleId, name, server = null, width = 1280, thumb = false }) {
    const image = useImage(name ? consoleBackgroundPath(consoleId, name, width, server) : null);

    if (thumb) {
        return <Thumb url={image?.url || null} fit="cover" />;
    }

    return <div className="ratio ratio-16x9 rounded-4 overflow-hidden bg-body-secondary">{image ? <img className="object-fit-cover" src={image.url} alt="" /> : <span></span>}</div>;
}

// Le fond changé : ses images, et celle de l'écran Remote Play, se relisent.
const forgetRemoteBackground = (consoleId) => {
    forgetConsoleBackgrounds(consoleId);
    forgetBand(consoleId);
};

// UNE CONSOLE (DeviceDetailPanel) : ses détails, « Plus » — son image de fond —, puis la dissocier (« Danger ») —
// l'appairage oublié, il faudra la réenregistrer pour s'y connecter. Ni le flux ni le réveil (plus haut).
function DeviceView({ device }) {
    const { t } = useTranslation();
    const { back, open } = useSettingsNavigation();
    const background = useRemoteBackground(device);
    const { refreshDevices } = useData();
    const status = useStatus(device);

    const unlink = async () => {
        try {
            const result = await deleteDevice(device.id);

            if (result.ok) {
                refreshDevices();
                back(ROOT);

                return;
            }
            toast(result.errorMessage || t("rp.unlinkFailed"));
        } catch (failure) {
            console.error("échec de la dissociation de l'appareil", failure);
            toast(t("rp.unlinkFailed"));
        }
    };

    return (
        <SettingsPage title={deviceName(device)} parent={ROOT}>
            <Section title={t("remoteplay.device.details")} help={t("remoteplay.device.detailsHelp")}>
                <Rows>
                    <InfoRow icon={status.icon} tone={status.tone} label={t("remoteplay.device.status")} value={status.label} />
                    <InfoRow icon="bi-playstation" tone="sky" label={t("remoteplay.device.type")} value={device.hostType} />
                    <InfoRow icon="bi-router" tone="slate" label={t("remoteplay.device.ip")} value={device.ipAddress} />
                </Rows>
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="rp-background" lead={<RemoteBackgroundImage consoleId={backgroundConsole(device)} name={background?.name} server={background?.server} width={256} thumb />} label={t("rp.background")}
                        help={t("rp.backgroundHelp", { console: device.hostType })} onOpen={() => open(`${ROOT}/${encodeURIComponent(device.id)}/background`, "rp-background", { device })} />
                </Rows>
            </Section>
            <DangerSection id="rp-unlink" label={t("rp.unlink")}
                question={t("rp.unlinkConfirm", { name: deviceName(device) })} confirmLabel={t("rp.unlinkAction")} onConfirm={unlink} />
        </SettingsPage>
    );
}

// /settings/remote-play/<id>/background : L'IMAGE DE FOND D'UNE CONSOLE — « L'image actuelle », puis la recherche chez
// les fournisseurs (comme un emplacement de l'apparence d'une console : une image choisie, la fenêtre, puis le serveur la
// récupère dans le premier emplacement), puis « Danger » : la remettre par défaut (le remplissage par défaut la refait
// à la prochaine visite). Une image prêtée par un serveur fédéré dit sa provenance dans « Détails », comme la page d'un
// emplacement de l'apparence d'une console (Consoles.jsx, SlotPage ; #748).
function BackgroundView({ device }) {
    const { t } = useTranslation();
    const { back } = useSettingsNavigation();
    const consoleId = backgroundConsole(device);
    const [epoch, setEpoch] = useState(0);
    const background = useRemoteBackground(device, epoch);
    const name = background?.name;
    const parent = `${ROOT}/${encodeURIComponent(device.id)}`;

    const reset = async () => {
        const answer = await resetConsoleBackgroundSlot(consoleId, 1);

        if (!answer.ok) {
            toast(answer.errorMessage || t("settings.actionFailed"));

            return;
        }
        forgetRemoteBackground(consoleId);
        toast(t("rp.backgroundResetDone"));
        setEpoch((value) => value + 1);
    };

    return (
        <SettingsPage title={t("rp.background")} parent={parent} levels={{ [parent]: deviceName(device) }}>
            {name && (
                <Section title={t("appearance.current")} help={t("rp.backgroundHelp", { console: device.hostType })}>
                    <RemoteBackgroundImage consoleId={consoleId} name={name} server={background.server} />
                </Section>
            )}
            {name && background.server && (
                <Section title={t("details")}>
                    <Rows>
                        <InfoRow icon="bi-arrow-left-right" tone="green" label={t("game.origin")} value={background.serverName} />
                    </Rows>
                </Section>
            )}
            <Section title={t("appearance.searchTitle")} help={t("appearance.searchConsoleHelp")}>
                <ImageSearch fit="cover" search={(query) => searchConsoleBackgrounds(consoleId, query)}
                    confirmOf={(image) => ({
                        title: t("rp.backgroundConfirmTitle"),
                        message: t("appearance.chosenConfirm", { game: image.game, source: t(`images.services.${image.source}`) }),
                        confirmLabel: t("appearance.replaceAction"),
                    })}
                    onChoose={async (image) => {
                        const answer = await setConsoleBackground(consoleId, 1, image);

                        if (answer.ok) {
                            forgetRemoteBackground(consoleId);
                            toast(t("rp.backgroundSet"));
                            back(parent);
                        }

                        return answer;
                    }} />
            </Section>
            <DangerSection id="rp-background-reset" icon="bi-arrow-counterclockwise" label={t("rp.backgroundReset")} question={t("rp.backgroundResetConfirm")}
                confirmLabel={t("rp.backgroundResetAction")} onConfirm={reset} />
        </SettingsPage>
    );
}

// La page d'une console : celle que la liste a passée le temps de relire la liste (un lien direct, un rechargement),
// puis celle de la liste, tenue à jour. `View` : la console elle-même, ou son image de fond.
function DevicePage({ View = DeviceView }) {
    const { t } = useTranslation();
    const { deviceId } = useParams();
    const location = useLocation();
    const devices = useDevices();
    const passed = location.state?.device?.id === deviceId ? location.state.device : null;
    const device = Array.isArray(devices) ? devices.find((entry) => entry.id === deviceId) : passed;

    if (device) {
        return <View device={device} />;
    }

    return (
        <SettingsPage title={t("settings.items.remotePlay.label")} parent={ROOT}>
            {devices === null ? <Note>{t("settings.loading")}</Note> : devices === false ? <Note>{t("remoteplay.loadFailed")}</Note> : <NotFound />}
        </SettingsPage>
    );
}

export function RemotePlayRoutes() {
    return (
        <Routes>
            <Route index element={<DevicesPage />} />
            <Route path="new" element={<RegisterPage />} />
            <Route path=":deviceId" element={<DevicePage />} />
            <Route path=":deviceId/background" element={<DevicePage View={BackgroundView} />} />
            <Route path="*" element={<Navigate to={ROOT} replace />} />
        </Routes>
    );
}
