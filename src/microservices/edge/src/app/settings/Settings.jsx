import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { can, DataProvider, useAuth, useData } from "../libs/data.jsx";
import { returnToAppHome, setRemoteTarget } from "../libs/api.js";
import { adminThere, REMOTE_PAGES, RemoteAdminContext } from "../libs/remoteAdmin.js";
import { COFFEE_URL, openExternal } from "../libs/links.js";
import { toast } from "../shell/Shell.jsx";
import SettingsPage, { useSettingsNavigation } from "./SettingsPage.jsx";
import { SETTINGS_ROUTES, settingsPath } from "../globals/routes.js";
import { AccountName } from "../bricks/AccountName.jsx";
import { AdminMark } from "../bricks/AdminMark.jsx";
import { Avatar } from "../bricks/Avatar.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Note } from "../bricks/Note.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";
import { atUsername } from "../globals/accounts.js";
import { CONSOLE_ICON } from "../globals/icons.js";

// LES RÉGLAGES (le porteur, 2026-11-12 : « l'interface des settings »), en direction C (validée le 2026-11-13) :
// Mon compte (la carte du compte, seule : elle ouvre Profil), les tuiles de ce que TOUT compte voit (Manettes,
// Application, Catalogue, Inviter), puis des groupes en lignes — Serveur et Personnes (un administrateur seulement,
// l'insigne vert à côté de leur titre), À propos — et, en bas, « Session » : l'accueil de l'application et la
// déconnexion. Chaque entrée
// n'apparaît que là où l'hôte la déclare (`can`, les capacités de la webapp).
//
// MOT DE PASSE, JETON ET MES SAUVEGARDES SONT DANS PROFIL (le porteur, 2026-11-13 : « mot de passe, jeton et mes
// sauvegardes vont DANS profil. Là c'est en dehors. ») : des pages enfants, /settings/profile/password, …/token,
// …/saves (globals/routes.js). Leurs anciennes adresses ne restent pas.
//
// CHAQUE ENTRÉE OUVRE SA ROUTE (le porteur, 2026-11-13 : « chaque lien doit ouvrir une ROUTE, plus un
// offcanvas ») : /settings/<entrée>, et ses propres pages dessous (une console, un membre, un journal…).
// Un lien direct s'ouvre, un rechargement garde la page ; une entrée que ce compte ne voit pas renvoie à la
// liste. La liste et chaque page tiennent dans la même boîte centrée (SettingsPage.jsx). À la manette : la
// croix parcourt la liste, A ouvre l'entrée, B remonte d'un niveau et rend la lumière à l'entrée d'où l'on
// vient ; B depuis la liste quitte les réglages.
//
// APPLICATION EST UNE SEULE PAGE (le porteur, 2026-11-13 : « regrouper les réglages dans une seule page ») :
// les sons, le plein écran et les notifications en tuiles, la langue et l'orientation dans « Apparence » (le porteur,
// 2026-11-14), l'autorisation des notifications.
// LES MANETTES ONT LEUR ENTRÉE (le porteur, 2026-11-13 : « Ce n'est pas un réglage de l'application ») :
// /settings/gamepads, avec ses correspondances, l'écoute d'un accord et l'éditeur de disposition.
// REMOTE PLAY A SON COIN ADMINISTRATEUR (/settings/remote-play) : les consoles enregistrées, en ajouter une.


// `url` : une entrée qui n'ouvre pas de page des Réglages mais une adresse d'un autre site (`opens="link"`).
// `disabled` : une ligne grisée, qui annonce une page pas encore faite.
// Une entrée : sa clé, son icône (la couleur est celle du sens de sa section), si ce compte la voit, son adresse dehors.
const item = (key, icon, show = true, url = null, disabled = false) => (show ? { key, icon, url, disabled } : null);

export function useSettingsGroups() {
    const { capabilities } = useData();
    const { isAdmin } = useAuth();
    const has = (name) => can(capabilities, name);

    return useMemo(() => [
        // En tuiles, en tête : ce que TOUT compte voit (le porteur, 2026-11-13 : « Dans les tuiles tu as mis des
        // sections qui concernent que l'admin. Un utilisateur normal ne verra pas ça. »). QUATRE TUILES (le porteur,
        // 2026-11-16 : « mets la Catalogue/jeux en tuiles et rajoute un 4eme tuile : Inviter » ; « Le tuile Inviter c'est
        // pour tout le monde et ça permet d'avoir seulement le lien du serveur ») : la première rangée pour CET appareil
        // (Manettes, Application, à leur place d'avant), la seconde pour CE serveur — le Catalogue, tous les jeux d'ici et
        // des serveurs fédérés (catalog/Games.jsx ; « Catalogue c'est tout membres/roles »), et Inviter, son adresse à
        // ouvrir sur un autre appareil (application/Invite.jsx) —, juste au-dessus de ce qu'un administrateur seul voit du
        // serveur. Le groupe « Le catalogue », qui n'avait que cette ligne, est parti avec elle.
        // LES TUILES DEVIENNENT DES SECTIONS (le porteur, 2026-11-25 : « On remplacera les tuiles par des sections dans
        // settings » ; « remplace le texte "cet appareil" par "réglages" ») : « Réglages » — ce qui ne vaut que pour cet
        // appareil —, puis « Ce serveur » — ses jeux, y inviter quelqu'un.
        {
            key: "device", sense: "modify", items: [
                item("gamepads", "bi-controller"),
                item("application", "bi-sliders"),
            ],
        },
        {
            key: "here", sense: "play", items: [
                item("catalogGames", "bi-disc"),
                item("invite", "bi-qr-code"),
            ],
        },
        // QUATRE GROUPES POUR L'ADMINISTRATEUR (le porteur, 2026-11-20 : « On remplace "serveur" et "personnes et partage"
        // par les sections suivantes ») : ce que le serveur sert (Contenu), ce qu'il partage et ce qu'il lit ailleurs
        // (Partage et Stockage), qui y entre (Personnes), et son entretien (Serveur). Une ligne grisée a la pastille
        // ardoise, comme les autres lignes désactivées des Réglages.
        {
            key: "content", admin: true, sense: "modify", items: [
                // Leurs icônes à elles (le porteur, 2026-11-25 : « change l'icône de consoles et Remote Play pour un autre »).
                item("consoles", CONSOLE_ICON),
                item("remotePlay", "bi-broadcast"),
                item("metadata", "bi-images", has("metadataServices")),
                // GOOGLE CAST (le porteur, 2026-11-24 : « ça devrait être dans l'administration directement et non plus
                // dans le launch.sh ») : l'identifiant du récepteur (Cast.jsx). Sur toutes les plateformes (le porteur,
                // 2026-11-26 : « le cast to tv c'est toutes les plateformes qui utilise ») : chaque hôte le garde
                // (`/api/v1/cast`), même vu d'une page qui ne sait pas encore caster elle-même (`castToTv`, #124).
                item("cast", "bi-cast"),
            ],
        },
        {
            key: "sharing", admin: true, sense: "modify", items: [
                item("federation", "bi-arrow-left-right", has("serverAdmin")),
                item("resources", "bi-device-hdd", true),
            ],
        },
        {
            key: "people", admin: true, sense: "modify", items: [
                // Les sauvegardes d'un compte sont sur la page de son membre (le porteur, 2026-11-14 : « rajoute aussi un
                // lien pour voir les sauvegardes de l'utilisateur (ce qui fait qu'on enlève "user's saves" dans
                // settings) »).
                item("members", "bi-person-badge", has("accounts")),
            ],
        },
        {
            key: "server", admin: true, sense: "modify", items: [
                // CE QUI A QUITTÉ LE COMPOSE (chantier 81) : son nom, les manettes, les parties par console, le miroir, le GPU.
                item("serverConfig", "bi-hdd-network", has("serverConfig")),
                item("logs", "bi-journal-text", true),
                item("cache", "bi-archive", true),
                item("rescan", "bi-arrow-repeat", true),
            ],
        },
        {
            key: "about", sense: "optional", items: [
                // LE PROJET SUR GITHUB (le porteur, 2026-11-14, chantier 53, P13 : « dans settings > about rajoute "GameFlex"
                // avec un lien vers le GitHub : Readme, Licence, Fédération, etc... vers chaque doc et aussi le repos ») : le
                // dépôt public et chacun de ses documents (about/Project.jsx, ProjectPage).
                item("gameflex", "bi-github", true),
                // LA LICENCE, CACHÉE POUR LE MOMENT (le porteur, 2026-11-14, chantier 53, P13 : « pour le moment cache le
                // about "Licence" ») : sa page reste (LicencePage), son entrée revient en décommentant la ligne.
                // item("licence", "bi-patch-check", isAdmin),
                item("credits", "bi-c-circle", true),
                item("download", "bi-android2", has("appDownload")),
            ],
        },
        // BUY ME A COFFEE (le porteur, 2026-11-14, chantier 53, P13), DANS SA SECTION « SOUTIEN » (le porteur, 2026-11-25 :
        // « Tu peux mettre "buy me a coffee" dans une section Support ? ») : sa page, hors de l'application (libs/links.js,
        // COFFEE_URL).
        {
            key: "support", sense: "optional", items: [
                item("coffee", "bi-cup-hot-fill", true, COFFEE_URL),
            ],
        },
        // SE DÉCONNECTER N'EST PAS EN DANGER (le porteur, 2026-11-14 : « ne pas mettre sign out en couleur danger ») :
        // la ligne a la couleur de « Cet appareil », sur sa page ; seuls « les autres appareils » et « tous les
        // appareils », là-bas, restent rouges.
        // REVENIR À L'ACCUEIL DE L'APPLICATION EST DANS « SESSION » (le porteur, 2026-11-16 : « Dans settings mobile, rajouter
        // retourner à l'accueil dans section session ») : à côté de Se déconnecter, plus dans « À propos » — l'application
        // seule le déclare (`appHome`, `appHomeLink`).
        {
            key: "logout", sense: "optional", items: [
                item("appHome", "bi-house", has("appHome") || has("appHomeLink")),
                item("logout", "bi-box-arrow-right", has("accounts")),
            ],
        },
    ].filter((group) => !group.admin || isAdmin)
        .map((group) => ({ ...group, items: group.items.filter(Boolean) }))
        .filter((group) => group.items.length),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [capabilities, isAdmin]);
}



// /settings/remote/<serveur>/* : LES MÊMES PAGES, CHEZ UN SERVEUR FÉDÉRÉ (chantier 73). La session de mon compte de
// là-bas (peerHost) ; administrateur là-bas seulement si je l'y ai autorisé (son jeton, « Administration à distance ») —
// sinon la page le dit et ne montre rien. Tant qu'elle est ouverte, chaque appel des Réglages part chez lui
// (setRemoteTarget) ; la quitter le rend à ce serveur.
function RemoteAdmin() {
    const { t } = useTranslation();
    const { serverId } = useParams();
    const { servers } = useData();
    const server = (servers || []).find((entry) => String(entry.id) === serverId);
    const [state, setState] = useState({ phase: "loading" });
    const home = `/settings/federation/servers/${encodeURIComponent(serverId)}`;
    const name = server ? server.hostname || server.name || server.label || server.link : "";

    useEffect(() => {
        if (!server) {
            return undefined;
        }
        let cancelled = false;
        adminThere(server).then((answer) => {
            if (cancelled) {
                return;
            }
            if (!answer.ok) {
                setState({ phase: answer.host ? "notAdmin" : "refused" });

                return;
            }
            setRemoteTarget(answer.host);
            setState({ phase: "ready" });
        });

        return () => {
            cancelled = true;
            setRemoteTarget(null);
        };
    }, [server?.id]);

    if (state.phase !== "ready") {
        return (
            <SettingsPage title={name || t("remoteAdmin.title")} parent={home} guides={[["install"], ["library", "1-add-a-console"]]}>
                <Section>
                    <Note className="mt-0">{state.phase === "loading" ? t("settings.loading") : t(state.phase === "notAdmin" ? "remoteAdmin.notAdmin" : "remoteAdmin.unreachable")}</Note>
                </Section>
            </SettingsPage>
        );
    }

    return (
        <RemoteAdminContext.Provider value={{ serverId, name, home }}>
            {/* SES DONNÉES, PAS LES NÔTRES : consoles, disques, serveurs rejoints, consoles Remote Play — lues chez lui. */}
            <DataProvider remote>
            <Routes>
                {SETTINGS_ROUTES.filter((route) => REMOTE_PAGES.some(({ key }) => key === route.entry)).map(({ path, page: Page }) => (
                    <Route key={path} path={path} element={<Page />} />
                ))}
                <Route path="*" element={<Navigate to={home} replace />} />
            </Routes>
            </DataProvider>
        </RemoteAdminContext.Provider>
    );
}

// LA LISTE, en direction C (validée par le porteur le 2026-11-13) : « Mon compte » — la carte du compte, seule (elle
// ouvre Profil, qui tient le mot de passe, le jeton et les sauvegardes) —, les tuiles de ce que tout compte voit, puis
// les groupes en lignes, et « Session » en bas. CHAQUE GROUPE A SON TITRE (le porteur, 2026-11-13 : « chaque section
// doit toujours avoir un titre si possible ») : la carte celui de « Mon compte », Se déconnecter « Session », comme
// dans les modèles (01-reglages). La carte : le nom, le badge d'un administrateur, « @identifiant » dessous, rien
// d'autre (le porteur, 2026-11-14 : « le sous-titre ne doit contenir que le username avec le @ devant. Pour les
// administrateurs on rajoute le shield dans son nom. »).
function SettingsList({ groups }) {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const { user, isAdmin } = useAuth();
    const { capabilities } = useData();
    const name = user?.display_name || user?.username || "";
    const card = can(capabilities, "accounts") && user;
    // « ACCUEIL DE L'APPLICATION » Y RAMÈNE AUSSITÔT (le porteur, 2026-11-16 : « Oui tu peux le faire », pour une page
    // qui ne tenait qu'une ligne) : un geste, sans page à ouvrir ; l'application qui ne répond pas le dit.
    const goHome = async () => {
        if (!(await returnToAppHome(capabilities))) {
            toast(t("settings.appHomeFailed"));
        }
    };
    const row = (entry) => (
        <NavRow key={entry.key} id={`s-${entry.key}`} icon={entry.icon} label={t(`settings.items.${entry.key}.label`)}
            help={entry.key === "logout" ? null : t(`settings.items.${entry.key}.help`)} opens={entry.url ? "link" : entry.key === "appHome" ? "action" : "page"}
            disabled={entry.disabled} onOpen={() => (entry.url ? openExternal(entry.url) : entry.key === "appHome" ? goHome() : open(settingsPath(entry.key), `s-${entry.key}`))} />
    );

    // Son en-tête est celui de toute page des Réglages (PageHeader) : son Retour les quitte.
    return (
        <SettingsPage title={t("settings.title")} parent={null} guides={[["install"], ["library", "1-add-a-console"]]}>
            {card && (
                <Section title={t("settings.groups.account")} help={t("settings.groupsHelp.account")} sense="info">
                    <Rows>
                        <NavRow id="s-profile" label={<AccountName name={name} admin={isAdmin} />} help={atUsername(user.username)}
                            lead={<Avatar name={name} />} onOpen={() => open(settingsPath("profile"), "s-profile")} />
                    </Rows>
                </Section>
            )}
            {groups.map((group) => (
                // L'INSIGNE DE L'ADMINISTRATION à côté du titre d'un groupe qu'un administrateur seul voit (AdminMark).
                <Section key={group.key} title={t(`settings.groups.${group.key}`)} help={t(`settings.groupsHelp.${group.key}`)} sense={group.sense}
                    badge={group.admin ? <AdminMark label={t("settings.adminOnly")} /> : null}>
                    <Rows>{group.items.map(row)}</Rows>
                </Section>
            ))}
        </SettingsPage>
    );
}

// /settings/* (index.js) : la liste, puis une route par entrée que ce compte voit.
export default function Settings() {
    const groups = useSettingsGroups();
    const { capabilities } = useData();
    // Profil s'ouvre par la carte du compte, en tête de la liste. Les jeux vivent sous chaque console (le porteur,
    // 2026-11-13 ; chantier 53, P8) : Réglages › Consoles › une console › Jeux.
    const { isAdmin } = useAuth();
    // LES MODÈLES, SUR LA STAGING SEULEMENT, SUR UNE ROUTE CACHÉE (le porteur, 2026-11-25 : « pour staging on le garde dans
    // une autre route cachée que pour le staging. Ça permettra d'avoir la référence ») : `/settings/templates`, sans entrée
    // dans la liste — la référence des composants et les maquettes du chantier 81. La capacité `staging`, un administrateur.
    const available = new Set([
        ...groups.flatMap((group) => group.items.map((entry) => entry.key)),
        ...(can(capabilities, "accounts") ? ["profile"] : []),
        ...(can(capabilities, "staging") && isAdmin ? ["templates"] : []),
    ]);

    return (
        <Routes>
            <Route index element={<SettingsList groups={groups} />} />
            <Route path="remote/:serverId/*" element={<RemoteAdmin />} />
            {SETTINGS_ROUTES.map(({ entry, path, page: Page, capability }) => (
                <Route key={path} path={path} element={available.has(entry) && (!capability || can(capabilities, capability)) ? <Page /> : <Navigate to="/settings" replace />} />
            ))}
            <Route path="*" element={<Navigate to="/settings" replace />} />
        </Routes>
    );
}
