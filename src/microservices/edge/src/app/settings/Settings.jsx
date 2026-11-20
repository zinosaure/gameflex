import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { can, DataProvider, useAuth, useData } from "../libs/data.jsx";
import { returnToAppHome, setRemoteTarget } from "../libs/api.js";
import { adminThere, REMOTE_PAGES, RemoteAdminContext } from "../libs/remoteAdmin.js";
import { COFFEE_URL, EXTERNAL, openExternal, REPOSITORY_URL } from "../libs/links.js";
import { toast } from "../shell/Shell.jsx";
import { ProfileRoutes } from "./account/Profile.jsx";
import { LogoutPage } from "./account/Logout.jsx";
import { ApplicationPage } from "./application/Application.jsx";
import { DownloadPage } from "./about/Download.jsx";
import { LicencePage } from "./about/Licence.jsx";
import { ProjectPage } from "./about/Project.jsx";
import { CacheRoutes } from "./server/Cache.jsx";
import { MetadataRoutes } from "./server/Metadata.jsx";
import { RescanPage } from "./server/Rescan.jsx";
import { LogsRoutes } from "./server/Logs.jsx";
import { MembersRoutes } from "./people/Members.jsx";
import { ResourcesRoutes } from "./people/Resources.jsx";
import { FederationRoutes } from "./people/Federation.jsx";
import { ConsolesRoutes } from "./server/consoles/Consoles.jsx";
import Credits from "./about/Credits.jsx";
import { GamepadsRoutes } from "./application/Gamepads.jsx";
import { InvitePage } from "./application/Invite.jsx";
import { RemotePlayRoutes } from "./server/RemotePlay.jsx";
import { CatalogRoutes } from "./catalog/Games.jsx";
import SettingsPage, { useSettingsNavigation } from "./SettingsPage.jsx";
import NavRow, { AccountName, AdminMark, atUsername, Avatar, Note, QuickTiles, Rows, Section } from "../ui/Bricks.jsx";

// LES RÉGLAGES (le porteur, 2026-11-12 : « l'interface des settings »), en direction C (validée le 2026-11-13) :
// Mon compte (la carte du compte, seule : elle ouvre Profil), les tuiles de ce que TOUT compte voit (Manettes,
// Application, Catalogue, Inviter), puis des groupes en lignes — Serveur et Personnes (un administrateur seulement,
// l'insigne vert à côté de leur titre), À propos — et, en bas, « Session » : l'accueil de l'application et la
// déconnexion. Chaque entrée
// n'apparaît que là où l'hôte la déclare (`can`, les capacités de la webapp).
//
// MOT DE PASSE, JETON ET MES SAUVEGARDES SONT DANS PROFIL (le porteur, 2026-11-13 : « mot de passe, jeton et mes
// sauvegardes vont DANS profil. Là c'est en dehors. ») : des pages enfants, /settings/profile/password, …/token,
// …/saves (account/Profile.jsx, ProfileRoutes). Leurs anciennes adresses ne restent pas.
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

// La version de cette construction (le fichier VERSION ; webpack.config.js), dans le pied de la liste.
const VERSION = process.env.GAMEFLEX_VERSION;

// `url` : une entrée qui n'ouvre pas de page des Réglages mais une adresse d'un autre site (`opens="link"`).
const item = (key, icon, show = true, tone = "slate", url = null) => (show ? { key, icon, tone, url } : null);

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
        {
            key: "tiles", tiles: true, items: [
                item("gamepads", "bi-controller", true, "violet"),
                item("application", "bi-sliders", true, "indigo"),
                item("catalogGames", "bi-disc", true, "teal"),
                item("invite", "bi-qr-code", true, "green"),
            ],
        },
        {
            key: "server", admin: true, items: [
                item("consoles", "bi-joystick", true, "indigo"),
                item("remotePlay", "bi-playstation", true, "sky"),
                item("metadata", "bi-images", has("metadataServices"), "indigo"),
                item("cache", "bi-archive", true, "slate"),
                item("logs", "bi-journal-text", true, "slate"),
                item("rescan", "bi-arrow-repeat", true, "slate"),
            ],
        },
        {
            key: "people", admin: true, items: [
                // Les sauvegardes d'un compte sont sur la page de son membre (le porteur, 2026-11-14 : « rajoute aussi un
                // lien pour voir les sauvegardes de l'utilisateur (ce qui fait qu'on enlève "user's saves" dans
                // settings) »).
                item("members", "bi-person-badge", has("accounts"), "green"),
                item("resources", "bi-device-hdd", true, "green"),
                item("federation", "bi-arrow-left-right", has("serverAdmin"), "green"),
            ],
        },
        {
            key: "about", items: [
                // LE PROJET SUR GITHUB (le porteur, 2026-11-14, chantier 53, P13 : « dans settings > about rajoute "GameFlex"
                // avec un lien vers le GitHub : Readme, Licence, Fédération, etc... vers chaque doc et aussi le repos ») : le
                // dépôt public et chacun de ses documents (about/Project.jsx, ProjectPage).
                item("gameflex", "bi-github", true, "slate"),
                // LA LICENCE, CACHÉE POUR LE MOMENT (le porteur, 2026-11-14, chantier 53, P13 : « pour le moment cache le
                // about "Licence" ») : sa page reste (LicencePage), son entrée revient en décommentant la ligne.
                // item("licence", "bi-patch-check", isAdmin, "slate"),
                item("credits", "bi-c-circle", true, "slate"),
                item("download", "bi-android2", has("appDownload"), "slate"),
                // BUY ME A COFFEE (le porteur, 2026-11-14, chantier 53, P13 : « Si tu peux rajouter buymeacoffee dans la
                // section about ce serait bien aussi ») : sa page, hors de l'application (libs/links.js, COFFEE_URL).
                item("coffee", "bi-cup-hot-fill", true, "amber", COFFEE_URL),
            ],
        },
        // SE DÉCONNECTER N'EST PAS EN DANGER (le porteur, 2026-11-14 : « ne pas mettre sign out en couleur danger ») :
        // la ligne a la couleur de « Cet appareil », sur sa page ; seuls « les autres appareils » et « tous les
        // appareils », là-bas, restent rouges.
        // REVENIR À L'ACCUEIL DE L'APPLICATION EST DANS « SESSION » (le porteur, 2026-11-16 : « Dans settings mobile, rajouter
        // retourner à l'accueil dans section session ») : à côté de Se déconnecter, plus dans « À propos » — l'application
        // seule le déclare (`appHome`, `appHomeLink`).
        {
            key: "logout", items: [
                item("appHome", "bi-house", has("appHome") || has("appHomeLink"), "slate"),
                item("logout", "bi-box-arrow-right", has("accounts"), "indigo"),
            ],
        },
    ].filter((group) => !group.admin || isAdmin)
        .map((group) => ({ ...group, items: group.items.filter(Boolean) }))
        .filter((group) => group.items.length),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [capabilities, isAdmin]);
}

// L'adresse de chaque entrée, et ce qu'elle ouvre (`/*` : elle a ses propres pages dessous).
const PAGES = {
    profile: ["profile/*", ProfileRoutes],
    catalogGames: ["catalog/games/*", CatalogRoutes],
    application: ["application", ApplicationPage],
    gamepads: ["gamepads/*", GamepadsRoutes],
    invite: ["invite", InvitePage],
    remotePlay: ["remote-play/*", RemotePlayRoutes],
    consoles: ["consoles/*", ConsolesRoutes],
    cache: ["cache/*", CacheRoutes],
    logs: ["logs/*", LogsRoutes],
    members: ["members/*", MembersRoutes],
    resources: ["resources/*", ResourcesRoutes],
    federation: ["federation/*", FederationRoutes],
    metadata: ["metadata/*", MetadataRoutes],
    rescan: ["rescan", RescanPage],
    gameflex: ["gameflex", ProjectPage],
    licence: ["licence", LicencePage],
    credits: ["credits", Credits],
    download: ["download", DownloadPage],
    logout: ["logout", LogoutPage],
};

export const settingsPath = (key) => `/settings/${PAGES[key][0].replace("/*", "")}`;


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
            <SettingsPage title={name || t("remoteAdmin.title")} parent={home}>
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
                {REMOTE_PAGES.map(({ key }) => {
                    const [path, Page] = PAGES[key];

                    return <Route key={key} path={path} element={<Page />} />;
                })}
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
        <NavRow key={entry.key} id={`s-${entry.key}`} icon={entry.icon} tone={entry.tone} label={t(`settings.items.${entry.key}.label`)}
            help={entry.key === "logout" ? null : t(`settings.items.${entry.key}.help`)} opens={entry.url ? "link" : entry.key === "appHome" ? "action" : "page"}
            onOpen={() => (entry.url ? openExternal(entry.url) : entry.key === "appHome" ? goHome() : open(settingsPath(entry.key), `s-${entry.key}`))} />
    );

    // Son en-tête est celui de toute page des Réglages (PageHeader) : son Retour les quitte.
    return (
        <SettingsPage title={t("settings.title")} parent={null}>
            {card && (
                <Section title={t("settings.groups.account")}>
                    <Rows>
                        <NavRow id="s-profile" label={<AccountName name={name} admin={isAdmin} />} help={atUsername(user.username)}
                            lead={<Avatar name={name} />} onOpen={() => open(settingsPath("profile"), "s-profile")} />
                    </Rows>
                </Section>
            )}
            {groups.map((group) => (group.tiles ? (
                <QuickTiles key={group.key} items={group.items.map((entry) => ({
                    id: `s-${entry.key}`, icon: entry.icon, tone: entry.tone, label: t(`settings.items.${entry.key}.label`), value: t(`settings.items.${entry.key}.help`),
                    onClick: () => open(settingsPath(entry.key), `s-${entry.key}`),
                }))} />
            ) : (
                // L'INSIGNE DE L'ADMINISTRATION à côté du titre d'un groupe qu'un administrateur seul voit (AdminMark).
                <Section key={group.key} title={t(`settings.groups.${group.key}`)} badge={group.admin ? <AdminMark label={t("settings.adminOnly")} /> : null}>
                    <Rows>{group.items.map(row)}</Rows>
                </Section>
            )))}
            {/* LE PIED DES RÉGLAGES (le porteur, 2026-11-14, chantier 53, P13 : « Ah et dans le footer, tu mets gameflex, le
                lien GitHub et le lien Buymeacoffee ») : « GameFlex (version) · GitHub · Buy me a coffee », en petit, de la
                couleur secondaire ; deux liens de texte, hors de l'application, à la croix comme tout lien. LA VERSION (le
                porteur : « n'oublie pas d'afficher la version. GameFlex ({version}) ») : celle du fichier VERSION, que la
                construction lit (webpack.config.js, GAMEFLEX_VERSION). */}
            <Note className="text-center mt-4 mb-2">
                {VERSION ? `GameFlex (${VERSION})` : "GameFlex"} · <a href={REPOSITORY_URL} {...EXTERNAL} className="text-reset" data-nav data-key="s-footer-github">{t("settings.footerGithub")}</a>
                {" · "}<a href={COFFEE_URL} {...EXTERNAL} className="text-reset" data-nav data-key="s-footer-coffee">{t("settings.footerCoffee")}</a>
            </Note>
        </SettingsPage>
    );
}

// /settings/* (index.js) : la liste, puis une route par entrée que ce compte voit.
export default function Settings() {
    const groups = useSettingsGroups();
    const { capabilities } = useData();
    // Profil s'ouvre par la carte du compte, en tête de la liste. Les jeux vivent sous chaque console (le porteur,
    // 2026-11-13 ; chantier 53, P8) : Réglages › Consoles › une console › Jeux.
    const available = new Set([
        ...groups.flatMap((group) => group.items.map((entry) => entry.key)),
        ...(can(capabilities, "accounts") ? ["profile"] : []),
    ]);

    return (
        <Routes>
            <Route index element={<SettingsList groups={groups} />} />
            <Route path="remote/:serverId/*" element={<RemoteAdmin />} />
            {Object.entries(PAGES).map(([key, [path, Page]]) => (
                <Route key={key} path={path} element={available.has(key) ? <Page /> : <Navigate to="/settings" replace />} />
            ))}
            <Route path="*" element={<Navigate to="/settings" replace />} />
        </Routes>
    );
}
