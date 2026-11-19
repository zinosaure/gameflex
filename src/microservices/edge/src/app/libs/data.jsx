import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import {
    fetchCapabilities, fetchConsoles, fetchMe, listConsoleCounts, listDevices, listDisks, listEmulatorParams, listFederatedDevices, listRoms,
    listHiddenServers, listServers, login as loginRequest, onAuthLost, readToken, setServerHidden, subscribeInvites, subscribeSessions, writeToken,
} from "./api.js";

// CE QUE LE BACKEND FOURNIT, chargé une fois pour toute la page : le compte,
// le registre des consoles, les disques internes, les serveurs fédérés, les
// appareils Remote Play et la liste vivante des parties. Rien d'inventé : une
// donnée absente reste absente (règle « ne montrer que ce que le backend fournit »).

const AuthContext = createContext(null);
const DataContext = createContext(null);

export const useAuth = () => useContext(AuthContext);

// CE QUE L'HÔTE DÉCLARE (`GET /api/v1/capabilities`) — et, pour ce qu'il ne dit pas, les
// valeurs par défaut de la webapp (libs/capabilities.js, WEB_DEFAULTS : « l'hôte déclare,
// l'interface obéit »). Seules celles que le launcher lit sont recopiées ici.
const CAPABILITY_DEFAULTS = {
    accounts: true,
    serverAdmin: true,
    appDownload: true,
    staging: false,
    appHome: false,
    appHomeLink: false,
    fullscreen: true,
    metadataServices: false,
    // CE QUI S'ÉCRIT DANS LES JOURNAUX se règle de la page (Réglages › Journaux) : le serveur le déclare ; le téléphone
    // non, son journal a son interrupteur sur la console d'accueil.
    logSettings: false,
    // LES SIGNALEMENTS S'ÉCRIVENT : l'écran d'erreur d'un lancement propose « Signaler l'erreur ». Le serveur le déclare
    // selon Réglages › Journaux ; le téléphone ne le déclare pas, les siens s'écrivent toujours.
    reports: true,
    issuesCatalogTokens: true,
    pushNotifications: false,
    onlyAdminsLaunch: false,
    nativeBrowse: false,
};

export const can = (capabilities, name) => Boolean(capabilities?.[name] ?? CAPABILITY_DEFAULTS[name]);

// CE QUE L'HÔTE REFUSE DE LANCER (chantier 56, T1 et T3 ; le porteur, 2026-11-14 : « Oui ») — deux capacités que le
// téléphone déclare et que la webapp lisait :
//   `onlyAdminsLaunch` — seul un administrateur lance ce qui tourne ici (le propriétaire ; c'est aussi le contrôle
//     parental : le parent lance pour l'enfant, mobilekernel.go). Ses consoles Remote Play, elles, sont à tout compte
//     valide (`#757`, Platforms.jsx) ;
//   `localEmulators` — les consoles récentes que l'appareil fait tourner lui-même, quand il ne fait pas naître de
//     conteneurs (`spawnsSessions` faux) : une autre n'y part pas, et la page ne propose pas un lancement qui échouerait.
// Rend la clé de la raison (`noLaunch.<clé>`), ou null. Un jeu d'un serveur fédéré part chez lui, sous notre compte de
// là-bas : rien ici ne le retient.
export function hostRefusal(console, rom, capabilities, isAdmin) {
    if (!console || rom?.peer) {
        return null;
    }

    if (can(capabilities, "onlyAdminsLaunch") && !isAdmin) {
        return "adminOnly";
    }
    const local = capabilities?.localEmulators;

    if (console.method === "streamex" && capabilities?.spawnsSessions === false && Array.isArray(local) && !local.includes(console.id)) {
        return "emulatorNotHere";
    }

    return null;
}
export const useData = () => useContext(DataContext);

// UN SERVEUR QUI NE LANCE RIEN — gameflex-federation (le porteur, 2026-11-18 : « les jeux de Gameflex-Federation ne
// doivent apparaître que dans catalogue/jeux ») : son listing le dit (`server.catalogOnly`). Ses jeux se téléchargent,
// depuis le catalogue ; aucune bibliothèque ne les montre.
export const catalogOnly = (server) => Boolean(server?.listing?.server?.catalogOnly);

export function AuthProvider({ children }) {
    const [user, setUser] = useState(null);
    const [status, setStatus] = useState(readToken() ? "loading" : "out");

    useEffect(() => {
        if (status !== "loading") {
            return undefined;
        }
        let cancelled = false;
        fetchMe().then((result) => {
            if (cancelled) {
                return;
            }

            if (result.ok && result.item) {
                setUser(result.item);
                setStatus("in");
            } else {
                writeToken(null);
                setStatus("out");
            }
        });

        return () => {
            cancelled = true;
        };
    }, [status]);

    useEffect(() => onAuthLost(() => {
        setUser(null);
        setStatus("out");
    }), []);

    // Une session ouverte (`{access_token, user}`) : par la connexion, ou par le lien d'un compte (chantier 55, Join.jsx).
    // Un navigateur connecté à un autre compte passe à celui-ci.
    const adopt = useCallback((item) => {
        writeToken(item.access_token);
        setUser(item.user || null);
        setStatus(item.user ? "in" : "loading");
    }, []);

    const login = useCallback(async (username, password) => {
        const result = await loginRequest(username, password);

        if (result.ok && result.item?.access_token) {
            adopt(result.item);
        }

        return result;
    }, [adopt]);

    // Relire mon compte (après un changement de nom, par exemple).
    const refreshMe = useCallback(async () => {
        const result = await fetchMe();

        if (result.ok && result.item) {
            setUser(result.item);
        }

        return result;
    }, []);

    // Se déconnecter n'efface que le jeton de CETTE page : la webapp garde le sien.
    const logout = useCallback(() => {
        writeToken(null);
        setUser(null);
        setStatus("out");
    }, []);

    const value = useMemo(() => ({ user, status, login, adopt, logout, refreshMe, isAdmin: Boolean(user) && user.role_level <= 1 }), [user, status, login, adopt, logout, refreshMe]);

    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

// Les droits d'un compte sur une console (libs/consoleGrants.js de la webapp, chantier 20).
export function allows(grants, console) {
    if (!Array.isArray(grants) || !console) {
        return false;
    }

    return grants.some((grant) => grant === "*" || grant === console.id || (grant === "retro" && console.method === "retrogaming"));
}

// Une partie est-elle la mienne ? Le nom sous lequel le kernel me connaît
// (`me.username`), sinon le mien d'ici (libs/SessionPool.js, isOwnSession).
export const isOwnSession = (session, username) => {
    const name = session?.me?.username || username || "";

    return Boolean(name) && session?.owner === name;
};

const normalize = (text) => String(text || "").normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();
export const matchesQuery = (rom, query) => !query || normalize(rom.title).includes(normalize(query));

// AFFICHER LES CONSOLES SANS JEU (Réglages › Application › Apparence) : oui par défaut,
// gardé dans CE navigateur, comme le thème et la langue de la webapp — le compte n'a pas
// de préférences d'affichage (PATCH /auth/me ne prend que le nom et le mot de passe).
const SHOW_EMPTY_KEY = "gameflex-edge-show-empty";

const readShowEmpty = () => {
    try {
        return localStorage.getItem(SHOW_EMPTY_KEY) !== "off";
    } catch {
        return true;
    }
};

// Le nombre de jeux par console, `{<id>: N}` ; null si le kernel n'a pas répondu (on ne sait pas).
const toCounts = (answer) => (answer.ok && Array.isArray(answer.data)
    ? Object.fromEntries(answer.data.map((entry) => [entry.id, entry.count]))
    : null);

// Les appareils Remote Play : ceux de cet hôte quand il en appaire (`pairsConsoles`, RemotePlay.jsx), puis ceux
// des serveurs fédérés.
const readDevices = (declared) => Promise.all([declared.pairsConsoles !== false ? listDevices() : Promise.resolve({ ok: true, data: [] }), listFederatedDevices()])
    .then(([own, federated]) => [...(own.ok ? own.data || [] : []), ...(federated.ok ? federated.data || [] : [])]);

export function DataProvider({ children, idle = false }) {
    const { user } = useAuth();
    const [consoles, setConsoles] = useState(null);
    const [capabilities, setCapabilities] = useState({});
    const [disks, setDisks] = useState([]);
    const [servers, setServers] = useState([]);
    // LA LISTE DES SERVEURS FÉDÉRÉS EST-ELLE LUE ? Une fiche demandée par son adresse attend ses sources (Library.jsx,
    // `awaited`) : une liste encore vide n'y dit pas « aucun serveur » (#748).
    const [serversRead, setServersRead] = useState(false);
    const [devices, setDevices] = useState([]);
    const [sessions, setSessions] = useState([]);
    const [invites, setInvites] = useState([]);
    const [paramsConsoles, setParamsConsoles] = useState(() => new Set());
    const [counts, setCounts] = useState(null);
    const [showEmpty, setShowEmptyState] = useState(readShowEmpty);
    // LES SERVEURS FÉDÉRÉS QUE CE COMPTE A ÉTEINTS (Réglages › Application ; le porteur, 2026-11-16 : « chaque utilisateur
    // choisit quel serveur fédéré il a envie de voir les jeux ») : leurs identifiants, lus au service des comptes
    // (hidden_servers.go) — le choix suit le compte d'un appareil à l'autre. Tous montrés par défaut, et tant qu'ils se
    // lisent ; un serveur ajouté plus tard aussi.
    const [hiddenServers, setHiddenServers] = useState(() => new Set());
    const [channelReady, setChannelReady] = useState(false);
    const lists = useRef(new Map());
    const disksPromise = useRef(Promise.resolve([]));

    useEffect(() => {
        if (idle) {
            return undefined;
        }
        let cancelled = false;
        // Les disques internes répondent lentement (leur liste se relit) : ils
        // ne retiennent pas l'écran, et les listes de jeux les attendent.
        disksPromise.current = listDisks().then((answer) => {
            const present = answer.ok ? (answer.data || []).filter((disk) => disk.present) : [];

            if (!cancelled) {
                setDisks(present);
            }

            return present;
        });
        listServers().then((answer) => {
            if (!cancelled) {
                setServers(answer.ok ? answer.data || [] : []);
                setServersRead(true);
            }
        });
        // Les nombres de jeux arrivent AVEC le registre : un écran de consoles qui cache
        // celles sans jeu ne se dessine pas d'abord avec, puis sans.
        Promise.all([fetchConsoles(), fetchCapabilities(), listConsoleCounts()]).then(([registry, declared, counted]) => {
            if (cancelled) {
                return;
            }
            setCounts(toCounts(counted));
            setConsoles(registry);
            setCapabilities(declared);
            // LE CANAL APRÈS UNE PREMIÈRE REQUÊTE : le relais de webpack
            // (http-proxy-middleware) n'écoute les poignées de main WebSocket
            // qu'après avoir servi une requête HTTP de ce service — ouvert
            // trop tôt, le canal échoue une première fois.
            setChannelReady(true);

            readDevices(declared).then((read) => {
                if (!cancelled) {
                    setDevices(read);
                }
            });
        });

        return () => {
            cancelled = true;
        };
    }, [idle]);

    useEffect(() => (idle || !channelReady ? undefined : subscribeSessions(setSessions)), [idle, channelReady]);

    // LES CONSOLES QUI ONT DES RÉGLAGES D'ÉMULATEUR (Réglages › Stockage ›
    // Paramètres des émulateurs, #347) : la liste n'est servie qu'aux
    // administrateurs — pour un autre compte, aucune, et l'engrenage reste grisé.
    useEffect(() => {
        if (idle || !user || user.role_level > 1) {
            return undefined;
        }
        let cancelled = false;
        listEmulatorParams().then((answer) => {
            if (!cancelled && answer.ok) {
                setParamsConsoles(new Set((answer.data?.emulators || []).map((emulator) => emulator.consoleId)));
            }
        });

        return () => {
            cancelled = true;
        };
    }, [idle, user]);
    useEffect(() => (idle ? undefined : subscribeInvites((invite) => setInvites((previous) => [...previous, invite]))), [idle]);
    const dismissInvite = useCallback((id) => setInvites((previous) => previous.filter((invite) => invite.id !== id)), []);

    // LES JEUX « ICI » D'UNE CONSOLE : la bibliothèque ET chaque disque interne
    // qui en a (chantier 23), réunis — une même référence n'apparaît qu'une
    // fois, la copie de la bibliothèque d'abord. Gardés pour la page : un
    // retour sur la console ne redemande rien.
    const loadHere = useCallback((consoleId) => {
        if (lists.current.has(consoleId)) {
            return lists.current.get(consoleId);
        }
        let sources = [null];
        const promise = disksPromise.current.then((present) => {
            sources = [null, ...present.filter((disk) => (disk.consoles?.[consoleId] || 0) > 0).map((disk) => disk.name)];

            return Promise.all(sources.map((disk) => listRoms(consoleId, { disk })));
        }).then((answers) => {
            const seen = new Set();
            const merged = [];
            const failed = answers.every((answer) => !answer.ok);

            answers.forEach((answer, index) => {
                for (const rom of (answer.ok ? answer.data || [] : [])) {
                    if (!seen.has(rom.reference)) {
                        seen.add(rom.reference);
                        merged.push(sources[index] && !rom.disk ? { ...rom, disk: sources[index] } : rom);
                    }
                }
            });
            if (failed) {
                lists.current.delete(consoleId);
            }

            return { ok: !failed, roms: merged };
        });
        lists.current.set(consoleId, promise);

        return promise;
    }, []);

    // UN JEU D'ICI A CHANGÉ (son statut, Réglages › … › un jeu › Informations ; settings/server/consoles/GameMetadata.jsx) : la liste
    // gardée de sa console le prend tout de suite — la fiche, la recherche de la bibliothèque, ses sous-titres et ses
    // filtres la lisent au retour (Library.jsx, loadHere). Rien ne se relit : la réponse du serveur porte déjà ce que la
    // liste aurait dit. Une console pas encore lue n'a rien à changer.
    const patchHere = useCallback((consoleId, reference, changes) => {
        const kept = lists.current.get(consoleId);

        if (kept) {
            lists.current.set(consoleId, kept.then((result) => ({
                ...result,
                roms: (result.roms || []).map((rom) => (rom.reference === reference ? { ...rom, ...changes } : rom)),
            })));
        }
    }, []);

    // LA LISTE GARDÉE D'UNE CONSOLE OUBLIÉE : un changement qui touche tous ses jeux à la fois (le statut de toute la
    // console, settings/server/consoles/Consoles.jsx) — la prochaine lecture la redemande au serveur.
    const forgetHere = useCallback((consoleId) => {
        lists.current.delete(consoleId);
    }, []);

    // Les serveurs éteints de ce compte, relus à chaque connexion (un autre compte, sur la même page, a les siens).
    useEffect(() => {
        if (idle || !user) {
            return undefined;
        }
        let cancelled = false;
        listHiddenServers().then((answer) => {
            if (!cancelled) {
                setHiddenServers(new Set(answer.ok ? (answer.items || []).map((item) => String(item.server_id)) : []));
            }
        });

        return () => {
            cancelled = true;
        };
    }, [idle, user?.id]);

    // LES SERVEURS FÉDÉRÉS DONT CE COMPTE VOIT LES JEUX : tous, moins ceux qu'il a éteints — la bibliothèque (Library.jsx)
    // et ses nombres (libraryCountOf) ne lisent que ceux-là ; les Réglages des consoles lisent toujours `servers`. MOINS
    // CEUX QUI NE LANCENT RIEN (gameflex-federation, `catalogOnly`) : leurs jeux ne sont que dans le catalogue.
    const shownServers = useMemo(() => servers.filter((server) => !hiddenServers.has(String(server.id)) && !catalogOnly(server)),
        [servers, hiddenServers]);

    // LE NOMBRE DE JEUX D'ICI d'une console (besoin B4) : `GET /retrogaming/consoles`, la
    // bibliothèque de cet hôte et ses disques internes — ce que comptent Réglages › Consoles
    // et la vitrine d'En direct. 0 pour une console absente de la réponse ; null si le kernel
    // n'a pas répondu. Relu à chaque écran de consoles (une requête légère) : un jeu importé
    // ou un nouveau parcours change le compte pendant qu'on navigue.
    const countOf = useCallback((consoleId) => (counts ? counts[consoleId] || 0 : null), [counts]);

    // LE NOMBRE DE JEUX QUE MONTRE LA BIBLIOTHÈQUE d'une console : ceux d'ici, plus ceux de
    // chaque serveur fédéré tels que la copie de son listing les compte (`listing.consoles`
    // de `GET /servers`) — la bibliothèque réunit toutes ses sources (Library.jsx). Le bouton
    // « Bibliothèque (N) » et le choix de cacher les consoles sans jeu le lisent. Ils lisaient
    // ceux d'ici seulement, et un téléphone sans jeu à lui voyait « Bibliothèque (0) » grisée
    // sur chaque console : aucun jeu de ses serveurs ne s'atteignait (le porteur, 2026-11-15).
    // Un serveur sans copie ne compte rien ; null si l'on ne sait pas ce qu'il y a ici. UN SERVEUR QUE CE COMPTE A ÉTEINT
    // (Réglages › Application, `shownServers`) ne compte pas : ses jeux ne sont pas dans la bibliothèque.
    const libraryCountOf = useCallback((consoleId) => {
        const here = countOf(consoleId);

        return here === null ? null : shownServers.reduce((total, server) => total + (server.listing?.consoles?.[consoleId] || 0), here);
    }, [countOf, shownServers]);

    const refreshCounts = useCallback(() => {
        listConsoleCounts().then((answer) => {
            const next = toCounts(answer);

            if (next) {
                setCounts(next);
            }
        });
    }, []);

    // Relire les appareils Remote Play (après en avoir enregistré ou supprimé un, Réglages › Remote Play) : l'écran
    // Remote Play les montre sans recharger la page.
    const refreshDevices = useCallback(() => readDevices(capabilities).then(setDevices), [capabilities]);
    // UNE CONSOLE COUPÉE OU RALLUMÉE POUR TOUT LE SERVEUR (chantier 66) : la page la grise ou la rend tout de suite.
    const markConsoleDisabled = useCallback((id, disabled) => setConsoles((current) => (current
        ? current.map((entry) => (entry.id === id ? { ...entry, disabled } : entry))
        : current)), []);

    // Relire les serveurs fédérés (après en avoir ajouté ou retiré un, Réglages › Fédération).
    const refreshServers = useCallback(() => listServers().then((answer) => {
        if (answer.ok) {
            setServers(answer.data || []);
        }

        return answer.ok ? answer.data || [] : [];
    }), []);

    const setShowEmpty = useCallback((on) => {
        setShowEmptyState(on);

        try {
            localStorage.setItem(SHOW_EMPTY_KEY, on ? "on" : "off");
        } catch {
            // Sans stockage, le choix ne vaut que pour cette page.
        }
    }, []);

    // MONTRER OU CACHER LES JEUX D'UN SERVEUR FÉDÉRÉ pour ce compte : tout de suite à l'écran, puis au service des
    // comptes ; refusé, le choix revient — la page dit pourquoi (la réponse est rendue).
    const setServerShown = useCallback(async (serverId, on) => {
        const id = String(serverId);
        const apply = (shown) => setHiddenServers((current) => {
            const next = new Set(current);

            if (shown) {
                next.delete(id);
            } else {
                next.add(id);
            }

            return next;
        });
        apply(on);
        const answer = await setServerHidden(serverId, !on);

        if (!answer.ok) {
            apply(!on);
        }

        return answer;
    }, []);

    const value = useMemo(() => ({
        consoles, capabilities, disks, servers, serversRead, refreshServers, devices, refreshDevices, sessions, invites, dismissInvite, loadHere, patchHere, forgetHere, countOf, libraryCountOf,
        refreshCounts, paramsConsoles, showEmpty, setShowEmpty, shownServers, hiddenServers, setServerShown, markConsoleDisabled,
        ownSessions: sessions.filter((session) => isOwnSession(session, user?.username)),
    }), [consoles, capabilities, disks, servers, serversRead, refreshServers, devices, refreshDevices, sessions, invites, dismissInvite, loadHere, patchHere, forgetHere, countOf, libraryCountOf,
        refreshCounts, paramsConsoles, showEmpty, setShowEmpty, shownServers, hiddenServers, setServerShown, markConsoleDisabled, user]);

    return <DataContext.Provider value={value}>{children}</DataContext.Provider>;
}

// LES ÉCRANS DE PLATEFORME, par famille — une famille par onglet (Streamex,
// Retrogaming, Remote Play ; libs/tabs.js). Chaque famille par ordre alphabétique, comme le
// menu de la webapp (le porteur, 2026-10-11). Remote Play : LES APPAREILS ENREGISTRÉS, seuls,
// des consoles accordées au compte (le porteur, 2026-11-14, chantier 53, P12 : « il faut retirer
// la Playstation 4 de remoteplay car j'avais dit qu'on affiche que les consoles enregistrées ») —
// un type sans appareil n'a plus d'écran ; une console s'enregistre dans Réglages › Remote Play.
// UN SERVEUR FÉDÉRÉ ÉTEINT PAR LE COMPTE (Réglages › Application › Serveurs fédérés) cache aussi ses consoles
// Remote Play (le porteur, 2026-11-16 : « le remoteplay affiche toujours les consoles des serveurs fédérés qu'on a
// décoché »), comme ses jeux ; les Réglages les montrent toujours.
export function usePlatforms() {
    const { consoles, devices, hiddenServers } = useData();
    const { user } = useAuth();

    return useMemo(() => {
        if (!consoles) {
            return null;
        }
        const byName = (a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
        const recent = consoles.filter((entry) => entry.method === "streamex").sort(byName);
        const retro = consoles.filter((entry) => entry.method === "retrogaming").sort(byName);
        const remote = [];

        for (const entry of consoles.filter((item) => item.method === "remoteplay" && allows(user?.consoles, item)).sort(byName)) {
            const hostType = entry.id.replace(/^rp-/, "").toUpperCase();
            const own = devices.filter((device) => String(device.hostType).toUpperCase() === hostType && !(device.serverId && hiddenServers.has(String(device.serverId))));

            own.forEach((device) => remote.push({ id: `device-${device.serverId ? device.serverId + "-" : ""}${device.id}`, kind: "device", family: "remote", console: entry, device, name: device.hostName || device.name || entry.name }));
        }

        return [
            ...recent.map((entry) => ({ id: entry.id, kind: "console", family: "recent", console: entry, name: entry.name })),
            ...retro.map((entry) => ({ id: entry.id, kind: "console", family: "retro", console: entry, name: entry.name })),
            ...remote,
        ];
    }, [consoles, devices, hiddenServers, user]);
}
