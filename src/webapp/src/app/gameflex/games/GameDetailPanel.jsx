import React, { useEffect, useMemo, useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../skeleton/skeleton.js";
import { StreamSettings } from "../stream/shared/StreamSettings.jsx";
import {
    hasLocalLibrary,
    deviceBiosNames,
    isRemoteSource,
    sourceOf,
    deviceRemoveGame,
    rescanLocalLibrary,
    onLocalLibraryReloaded,
} from "../libs/localLibrary.js";
import { federatedServerNow, federatedServers, parseFederatedId } from "../libs/federatedHost.js";
import { federationSession, useFederationSession } from "../libs/federationSession.js";
import { allows } from "../libs/consoleGrants.js";
import { FederatedAccount, FederatedTokenForm } from "../shared/FederatedAccountPanel.jsx";
import { GameStatusDetails } from "./GameStatus.jsx";
import {
    NO_LAUNCH,
    launchModeFor,
    coreMissingOn,
    defaultLaunchTarget,
    missingRequiredBios,
    expectedBiosFor,
    biosIsPresent,
} from "../libs/launch-mode.js";
import { can, capabilities } from "../libs/capabilities.js";
import { useConsoles } from "../contexts/ConsolesContext.jsx";
import {
    listBios,
    listCores,
    listRoms,
    downloadTicket,
    handToBrowser,
    ticketDownloadUrl,
} from "../api/retrogaming.js";
import { useAuth } from "../profile/auth/AuthContext.jsx";
import { SectionPanel, OffCanvas } from "../shared/templates/Essentials.jsx";
import {
    loadStreamingSettings,
    saveStreamingSettings,
} from "../libs/streamingSettingsStorage.js";
import { GameSource } from "./GameSource.jsx";
import { formatSize } from "./helpers.js";

const Form = Skeleton.Form;

// ── Offcanvas: infos du jeu + bouton Play ──────────────────────────────────
export function GameDetailPanel({
    rom,
    consoleId,
    consoleMeta,
    t,
    context,
    onPlay,
}) {
    const { playsHere, user } = useAuth();
    // EN TÊTE, et pas au milieu du composant : les effets plus bas les lisent
    // dans leurs tableaux de dépendances, qui sont évalués PENDANT le rendu.
    // Déclarés après eux, `onDevice` tombait dans sa zone morte temporelle et
    // la fiche d'un jeu jetait « Cannot access 'onDevice' before
    // initialization » à chaque ouverture, sur le web comme sur l'appareil
    // (introduit le 2026-08-30 avec « déjà dans la bibliothèque »).
    const onDevice = hasLocalLibrary();
    // LE REGISTRE, pour savoir quels BIOS cette console RÉCLAME (#162).
    //
    // La page portait ses propres tables (`REQUIRED_BIOS`/`OPTIONAL_BIOS`) ;
    // elles vivent maintenant dans `registry.go` (#161) et voyagent par
    // `GET /api/v1/consoles`, que ce contexte interroge déjà. Deux tables
    // décrivant la même chose avaient d'ailleurs fini par diverger — sur la
    // Switch, et sur un `saturn_bios.bin` que le registre ne connaissait pas.
    const consoles = useConsoles();
    // UNE ENTRÉE, UNE COPIE, UNE SOURCE.
    //
    // La fiche ne choisit plus d'où part le jeu : c'est fait EN AMONT, par le
    // sélecteur de la liste. `rom.variants` — le repliement qui unifiait les
    // copies d'un même titre — n'existe plus, et le second sélecteur que
    // portait cette fiche est parti avec. Deux endroits posaient la même
    // question et le second pouvait contredire le premier : on filtrait la
    // liste sur « kDrive », puis la fiche lançait la copie locale, élue par un
    // tri que personne n'avait demandé.
    //
    // Le couple (source, identifiant) ne voyage pas à part : il est DANS
    // l'entrée (`kind`, `serverId`, `mountId`), et `sourceOf` le rend ici
    // comme dans la liste — une seule question, une seule réponse.

    // LA COPIE LOCALE, indépendamment de la source affichée.
    //
    // Elle répond à « ce jeu est-il déjà dans la bibliothèque d'ici ? », par
    // l'UUID (chantier 14). Pour un jeu d'ici, elle pilote le bouton
    // télécharger/retirer. Pour celui d'un serveur fédéré, elle ne change RIEN
    // à ce qui part : un jeu de la liste d'un ami ne se lance que chez lui (le
    // porteur, 2026-10-13) ; elle rend seulement juste le refus d'« Ici »
    // (`also-in-library` plutôt que « pas dans la bibliothèque »).
    // La prop date de l'ouverture du panneau et ne peut pas connaître une
    // copie posée depuis : on la cherche donc nous-mêmes, à chaque relecture
    // de la bibliothèque (signalé le 2026-09-02).
    const [localCopy, setLocalCopy] = useState(null);

    // L'ÉMULATEUR D'ABORD, LA SOURCE ENSUITE — et pas l'inverse.
    //
    // La fiche demandait « d'où vient le jeu ? » puis « avec quoi le
    // lancer ? ». C'est le bon ordre tant que tout se passe ici ; il devient
    // faux dès qu'un émulateur peut vivre ailleurs, parce que c'est LUI qui
    // décide des sources acceptables, et pas le contraire.
    //
    // LA RÈGLE, ET SON MOTIF : un émulateur fédéré n'accepte que les jeux de
    // SON serveur. Pas par pureté de modèle — parce qu'on ignore l'espace
    // libre de la machine d'un ami, et surtout parce que lui faire exécuter
    // un fichier venu d'une troisième source serait lui faire ouvrir la porte
    // à n'importe quoi. Un serveur fédéré prête son catalogue et ses
    // ressources, à quelqu'un qui a un compte chez lui : rien d'autre.
    //
    // Un émulateur d'ici, lui, ne joue que la bibliothèque d'ici : un jeu
    // d'ailleurs n'y descend plus pour être joué (chantier 14, 2026-10-12) —
    // on l'ajoute par Réglages › Jeux, ou on le joue chez son serveur.
    const [servers, setServers] = useState([]);

    useEffect(() => {
        let cancelled = false;
        // RELUE À L'OUVERTURE : ce que chaque pair nous ouvre (« Consoles »)
        // décide de Play, et peut avoir changé depuis le début de la page.
        federatedServers({ fresh: true }).then((found) => {
            if (!cancelled) {
                setServers(found || []);
            }
        });

        return () => {
            cancelled = true;
        };
    }, []);

    const cores = consoleMeta?.cores || [];
    // CET HÔTE-CI SAIT-IL LANCER CETTE CONSOLE ?
    //
    // Une console streamex demande un émulateur en conteneur : un téléphone
    // n'en a pas, et n'en aura pas. Le rétro, lui, tourne partout — un cœur
    // libretro se charge dans le processus.
    // Une console streamex demande un ÉMULATEUR — pas forcément un conteneur.
    // Un serveur en ouvre un ; un téléphone charge une bibliothèque et la
    // pilote (Eden pour la Switch). D'où deux réponses possibles, et non
    // plus la seule `spawnsSessions`, qui disait « non » à tout téléphone.
    //
    // `localEmulators` ne liste que ce qui est réellement INSTALLÉ : proposer
    // un lancement local qui échoue serait pire que de n'en pas proposer,
    // surtout à côté d'un serveur qui sait le faire.
    const canRunHere = consoleMeta?.method === "streamex"
        ? (can("spawnsSessions") || (capabilities().localEmulators || []).includes(consoleId))
        : true;
    // UNE CIBLE DE LANCEMENT = UN ÉMULATEUR, LÀ OÙ EST LE JEU.
    //
    // Un jeu d'ici ne se joue qu'ici, celui d'un ami que chez lui (chantier
    // 14) : `launchModeFor` refusait d'office toute autre cible. La fiche les
    // proposait pourtant toutes — les cœurs de chaque serveur connu, à côté de
    // ceux d'ici. Le porteur, 2026-10-22 : « ça ne sert à rien de lister les
    // bios, émulateurs des autres sources. Quel intérêt de montrer aux users
    // que le core existe chez d'autres ? ». Ne restent que les émulateurs de
    // l'endroit du jeu ; un jeu d'un partage monté garde ceux d'ici, dont le
    // refus dit qu'il faut d'abord l'ajouter.
    const origin = sourceOf(rom);
    const launchTargets = useMemo(() => {
        if (origin.source === "federation") {
            const server = servers.find((candidate) => String(candidate.id) === String(origin.id));

            return server ? cores.map((core) => ({ id: `srv${server.id}:${core}`, core, server })) : [];
        }

        return canRunHere ? cores.map((core) => ({ id: `here:${core}`, core, server: null })) : [];
        // `cores` est un tableau neuf à chaque rendu (`consoleMeta?.cores ||
        // []`) : dépendre de lui ne mémoïserait rien. On dépend de son
        // CONTENU, qui, lui, ne change qu'avec la console.
    }, [cores.join(","), canRunHere, servers, origin.source, origin.id]);

    // QUELS ÉMULATEURS SONT RÉELLEMENT LÀ.
    //
    // `launchTargets` se construit depuis `consoleMeta.cores`, c'est-à-dire
    // les cœurs DÉCLARÉS au registre — jamais ceux présents sur le disque. Le
    // sélecteur proposait donc des émulateurs absents, et le garde du bouton
    // lisait `const noCore = false`, une constante en dur : rien ne vérifiait
    // la présence d'un cœur (mesuré le 2026-09-17, #163).
    //
    // La route, elle, le dit depuis toujours : `/cores` rend les cœurs de la
    // console « installés ou non », chacun avec son `installed`.
    const [coreFiles, setCoreFiles] = useState([]);

    useEffect(() => {
        // LE TÉLÉPHONE AUSSI (2026-10-16) : il rend la même liste, à la même
        // forme (`serveConsoleCores`), et ses cœurs vivent dans
        // cores/<console>/ comme au serveur. On ne lui demandait rien — et
        // « Jouer » n'y était jamais grisé, puisqu'un cœur absent se
        // téléchargeait au lancement. Plus rien ne se télécharge seul : c'est
        // l'admin, depuis Réglages → Émulateurs.
        let cancelled = false;
        listCores(consoleId).then((result) => {
            if (!cancelled && result.ok) setCoreFiles(result.data || []);
        });

        return () => {
            cancelled = true;
        };
    }, [consoleId]);

    const [selectedTargetId, setSelectedTargetId] = useState(null);
    // LA CIBLE PAR DÉFAUT, tant qu'on n'en a pas choisi — le serveur du jeu
    // pour un jeu fédéré, ici pour un jeu de la bibliothèque. La règle et son
    // histoire : `defaultLaunchTarget`.
    const defaultTarget = defaultLaunchTarget(launchTargets, {
        gameServerId: rom.serverId,
        cores: coreFiles,
    });
    // La cible retenue, ou celle par défaut. Dérivé plutôt que stocké : la
    // liste change avec les serveurs connus, et un identifiant qui n'y est
    // plus laisserait le panneau sur un choix qui n'existe pas.
    const selectedTarget = launchTargets.find((target) => target.id === selectedTargetId)
        || defaultTarget;
    const targetServer = selectedTarget?.server || null;
    const runsElsewhere = Boolean(targetServer);

    // `selectedCore` reste ce qu'il était pour tout le reste du panneau (BIOS
    // manquants, appel de lancement) — il vient juste de la cible désormais.
    const selectedCore = selectedTarget?.core || null;

    // CE SERVEUR, DEMANDÉ CHEZ LUI À L'OUVERTURE (chantier 15) : s'il répond,
    // et s'il accepte encore notre lien, vérifiés à l'instant. La liste des
    // serveurs disait ce que la copie de son listing savait, et un accès retiré
    // entre-temps n'y paraissait pas : Play restait, et le lancement échouait
    // chez lui (relevé par le porteur le 2026-10-13).
    const [checked, setChecked] = useState(null);

    useEffect(() => {
        if (!targetServer) {
            setChecked(null);

            return undefined;
        }
        let cancelled = false;
        setChecked(null);
        federatedServerNow(targetServer.id).then((found) => {
            if (!cancelled) {
                setChecked({ id: targetServer.id, found });
            }
        });

        return () => {
            cancelled = true;
        };
    }, [targetServer?.id]);
    const federatedServer = targetServer
        ? { id: targetServer.id, link: targetServer.link, name: targetServer.hostname }
        : null;
    // QUI JOUE CHEZ LUI (chantier 20) : notre compte de chez lui, par le jeton
    // qu'on y a reçu et collé ici une fois — sa session, ou pourquoi il n'y en
    // a pas (pas de jeton, jeton refusé). Ses consoles disent ce qu'il y lance.
    const federatedPlay = useFederationSession(federatedServer?.id || null);
    // Les questions sont parties, les réponses pas encore : Play attend, sans
    // refus à dire.
    const checking = Boolean(targetServer) && (checked?.id !== targetServer.id || federatedPlay.isLoading);
    // BIOS choice — most consoles have nothing to pick (biosList stays
    // empty and the whole section below just doesn't render, see
    // golang/internal/retrogaming/bios.go's biosConsoleDir), so this is a
    // separate fetch rather than part of consoleMeta (which every console
    // page needs regardless). "" (the default/library-picked BIOS,
    // whatever that resolves to for this core) is always the first option
    // — selecting one of the real files is opt-in.
    const [biosList, setBiosList] = useState([]);
    const [selectedBios, setSelectedBios] = useState("");
    const [isLaunching, setIsLaunching] = useState(false);
    // Stream settings — passed to the launch and mapped by the kernel onto
    // the session's resolution, frame rate and bitrate. Same localStorage
    // persistence as the remote-play panel (streamingSettingsStorage), keyed
    // by console id.
    //
    // LE RÉTRO AUSSI, ET À LA MÊME RÈGLE (#332, décision du porteur le
    // 2026-10-11 : « il faut que ça fonctionne aussi comme les autres » —
    // jouer en ligne dépend de la connexion de chacun). Le panneau s'affichait
    // pour le rétro depuis #150, mais seules les consoles de bureau envoyaient
    // leurs réglages : une NES gardait sa géométrie d'époque quoi qu'on
    // choisisse. La condition est écrite UNE fois, pour l'affichage, la
    // sauvegarde et le lancement.
    const takesStreamSettings = consoleMeta?.method === "streamex"
        || consoleMeta?.method === "retrogaming";
    const [streamSettings, setStreamSettings] = useState(
        () =>
            loadStreamingSettings(consoleId) || {
                // 540p et non 480p : ce dernier a été retiré de l'échelle le
                // 2026-09-20 (voir StreamSettings.jsx). 540p est le mode
                // survivant le plus proche.
                //
                // 60 IMAGES PAR SECONDE, et non 30 (décision du porteur le
                // 2026-10-14) : les cœurs rétro tournent à 60, la cadence du
                // flux ne doit pas en jeter une sur deux par défaut. Un choix
                // déjà enregistré pour la console reste le sien.
                resolution: "540p",
                frameRate: "60",
                bitrate: "auto",
            },
    );
    const streamSettingsContext = {
        values: streamSettings,
        setValues: (v) => setStreamSettings(v),
        files: {},
        setFiles: () => { },
        rulesets: {},
    };

    useEffect(() => {
        if (hasLocalLibrary()) {
            // AUCUN sélecteur sur l'appareil, et c'est délibéré.
            //
            // Les BIOS du téléphone vivent tous dans un même dossier, sans
            // rangement par console : les proposer tous donnait une liste où
            // il fallait deviner lequel va avec quoi — un choix que le web ne
            // demande jamais, puisqu'il n'affiche la liste que pour les
            // consoles qui en ont vraiment.
            //
            // Un core libretro trouve son BIOS TOUT SEUL dans le dossier
            // système, par son nom. Il n'y a donc rien à choisir. Ce qu'il
            // faut, c'est le dire quand il manque — voir needsBios ci-dessous.
            setBiosList([]);

            return undefined;
        }

        let cancelled = false;
        listBios(consoleId).then((result) => {
            if (!cancelled && result.ok) setBiosList(result.data || []);
        });
        return () => {
            cancelled = true;
        };
    }, [consoleId]);

    // La question « ce cœur est-il là ? » vit dans `libs/launch-mode.js`
    // (`coreMissingOn`) : une règle écrite en condition dans le JSX ne
    // s'éprouve pas, et celle-ci ne peut PAS l'être à l'écran — tous les cœurs
    // déclarés sont installés sur cette machine.

    const [isDownloading, setIsDownloading] = useState(false);
    const [isRemoving, setIsRemoving] = useState(false);
    // Un jeu déjà chez soi n'a rien à retélécharger : sur un appareil, le
    // bouton se ferme au lieu de proposer une copie qui ne servira jamais.
    //
    // Réévalué à chaque changement de bibliothèque, pas seulement à
    // l'ouverture : le jeu peut ENTRER pendant que la fiche est ouverte —
    // c'est même le cas normal, puisque c'est cette fiche qui l'a demandé.
    // « Déjà chez moi » n'est plus un état à tenir à jour : c'est la copie
    // locale, ou son absence. Deux états pour une seule vérité finissent
    // toujours par ne plus dire la même chose.
    const alreadyLocal = Boolean(localCopy);
    // L'UUID DU JEU, sans le préfixe d'un serveur fédéré (`srv<id>:<uuid>`) :
    // c'est par lui, et plus par le titre, qu'une copie d'ici se reconnaît
    // (chantier 14). Un nom écrit autrement est un autre jeu.
    const gameId = parseFederatedId(rom.reference)?.id || rom.reference;

    useEffect(() => {
        let cancelled = false;
        // Même objet tant que rien n'a changé : sans cette garde, chaque
        // relecture rend une référence neuve et relance le rendu.
        const settle = (found) => {
            if (!cancelled) {
                setLocalCopy((current) => (
                    current?.reference === found?.reference ? current : found
                ));
            }
        };

        // SUR LE SERVEUR, un jeu d'ici est sa propre copie ; un jeu fédéré se
        // cherche dans la liste d'ici, une fois, à l'ouverture de la fiche.
        if (!onDevice) {
            if (!parseFederatedId(rom.reference)) {
                settle(sourceOf(rom).source === "local" ? rom : null);

                return () => {
                    cancelled = true;
                };
            }
            listRoms(consoleId)
                .then((answer) => settle(
                    (answer.ok ? answer.data || [] : []).find((game) => game.reference === gameId) || null,
                ))
                .catch(() => settle(null));

            return () => {
                cancelled = true;
            };
        }

        // SUR LE TÉLÉPHONE, LA MÊME LISTE QU'AU SERVEUR (chantier 15) : celle
        // de son listing, que le kernel refait avant d'annoncer qu'un jeu est
        // entré ou sorti — la fiche la relit alors. Elle ne lisait plus un
        // scan gardé par la page (`/api/v1/library/games`, supprimée). La
        // garde reste : un jeu d'un partage s'était reconnu LUI-MÊME comme
        // présent, et la fiche proposait de retirer ce qui n'y avait jamais
        // été (2026-09-02).
        const look = () => listRoms(consoleId)
            .then((answer) => settle((answer.ok ? answer.data || [] : []).find(
                (game) => !isRemoteSource(game.kind) && game.reference === gameId,
            ) || null))
            .catch(() => settle(null));

        look();

        // Le signal « RELUE », pas « changée » : entre les deux il y a une
        // lecture, et répondre trop tôt fait relire l'ancien cache. Le jeu
        // venait d'entrer dans la bibliothèque et la fiche n'en savait
        // rien — il fallait la fermer et la rouvrir (signalé le 2026-09-02).
        // Le menu des consoles avait eu exactement ce défaut.
        const unsubscribe = onLocalLibraryReloaded(look);

        return () => {
            cancelled = true;
            unsubscribe();
        };
    }, [onDevice, consoleId, gameId]);

    // LE NAVIGATEUR TÉLÉCHARGE, PLUS LA PAGE.
    //
    // Elle le faisait elle-même — `fetch` avec l'en-tête, puis un blob remis au
    // navigateur — parce qu'un lien ne peut pas porter d'`Authorization` et que
    // toute route `/api/v1/retrogaming` en exige un. Ça marchait, et ça perdait
    // tout ce que le navigateur sait faire : sa barre de progression, la
    // reprise après coupure, le dossier Téléchargements — et un jeu de
    // plusieurs gigaoctets passait d'abord ENTIER par la mémoire de l'onglet.
    //
    // On demande donc un TICKET, dont l'émission est authentifiée, et on ouvre
    // l'adresse qu'il déverrouille. Le ticket ne porte aucune identité, ne vaut
    // que pour CE jeu, expire en une minute et se consomme au premier usage :
    // voir `retro_download_ticket.go`, qui dit pourquoi c'est un ticket tenu
    // par le serveur et non une signature rejouable.
    //
    // UN VRAI LIEN `<a download>` (chantier 14, demandé par le porteur) : c'est
    // l'utilisateur qui décide, dans son navigateur, où le jeu atterrit. Le nom
    // vient du serveur (`Content-Disposition`), d'où un `download` vide. Un jeu
    // d'un serveur fédéré passe par notre kernel, qui le relaie : le jeton de
    // ce serveur ne sort pas d'ici.
    const handleDownload = async () => {
        setIsDownloading(true);
        try {
            const answer = await downloadTicket(consoleId, rom.reference);
            const ticket = answer?.data?.ticket;
            if (!answer?.ok || !ticket) {
                throw new Error("no ticket");
            }
            const url = ticketDownloadUrl(ticket);
            // SUR LE TÉLÉPHONE, SON NAVIGATEUR : la WebView ne télécharge
            // pas, et elle consommerait le ticket (`handToBrowser`).
            if (await handToBrowser(url)) {
                context.toast.open(t("retrogaming.detail.downloadInBrowser"), { variant: "success" });

                return;
            }
            const link = document.createElement("a");
            link.href = url;
            link.download = "";
            link.rel = "noopener";
            document.body.appendChild(link);
            link.click();
            link.remove();
        } catch {
            context.toast.open(t("retrogaming.detail.downloadFailed"), { variant: "error" });
        } finally {
            setIsDownloading(false);
        }
    };

    const targetFormContext = {
        values: { target: selectedTarget?.id || "" },
        setValues: (value) => setSelectedTargetId(value.target),
        files: {},
        setFiles: () => { },
        rulesets: {},
    };

    const biosFormContext = {
        values: { bios: selectedBios },
        setValues: (v) => setSelectedBios(v.bios),
        files: {},
        setFiles: () => { },
        rulesets: {},
    };

    // Les consoles dont le core REFUSE de démarrer sans BIOS.
    //
    // Une liste courte et assumée, pas une table de vérité : ce sont celles
    // où l'absence donne un écran noir sans un mot, ce qu'on passe ensuite une
    // soirée à diagnostiquer. Partout ailleurs le core se débrouille, et
    // avertir à tort serait pire que se taire.
    //
    // Le core trouve son BIOS tout seul dans le dossier système, par son nom :
    // il n'y a rien à CHOISIR, seulement à signaler quand il manque.
    //
    // Et « manque » veut dire : LE BIOS DE CETTE CONSOLE, pas un fichier
    // quelconque. Le test comptait le contenu du dossier — un gba_bios.bin
    // suffisait donc à déclarer une PlayStation prête, et pcsx_rearmed
    // partait sur son BIOS HLE approximatif : écran noir, aucun message.
    // LES BIOS PRÉSENTS, SELON L'HÔTE QUI RÉPOND.
    //
    // Une règle, deux inventaires : l'appareil range ses BIOS dans sa propre
    // bibliothèque (`deviceBiosNames`), le serveur les sert par
    // `GET /api/v1/retrogaming/{console}/bios` — c'est `biosList`, déjà
    // chargé plus haut pour le sélecteur. Sans cette distinction, la règle
    // lue depuis le module de l'appareil aurait déclaré TOUT manquant sur le
    // web, et bloqué les jeux PS1 et Saturn.
    const presentBios = onDevice ? deviceBiosNames(consoleId) : biosList;
    // Ce qui manque VRAIMENT : « au moins un suffit » (des révisions du même
    // BIOS), sémantique que le registre porte désormais. La PS1 n'est plus
    // dedans — ses dumps y sont FACULTATIFS depuis #144, le cœur se rabattant
    // sur son BIOS émulé : on les propose sans bloquer.
    const missingBios = missingRequiredBios(consoles, consoleId, presentBios);
    const biosMissing = missingBios.length > 0;

    // LES OPTIONS DE BIOS : L'ATTENDU, PAS SEULEMENT LE PRÉSENT (#163/#162).
    //
    // Le sélecteur se construisait sur `biosList`, c'est-à-dire sur `/bios`,
    // qui ne liste QUE ce que le disque porte (`ListBiosEntries`). Tout ce
    // qu'il rendait était donc présent par construction, et aucune option ne
    // pouvait s'afficher désactivée — alors que c'est exactement ce qui a été
    // demandé le 2026-09-17 : « l'option du sélecteur doit être en disabled
    // (pas le selectbox) ».
    //
    // On part donc du registre (requis PUIS facultatifs, dans cet ordre), et
    // on ajoute à la suite ce que le disque porte en plus — un dump que le
    // registre ne nomme pas reste choisissable, il n'y a aucune raison de le
    // cacher à qui l'a déposé.
    const biosOptions = useMemo(() => {
        const expected = expectedBiosFor(consoles, consoleId);
        const seen = new Set();
        const names = [];

        for (const name of [...expected.required, ...expected.optional]) {
            const key = String(name).toLowerCase();
            if (!seen.has(key)) {
                seen.add(key);
                names.push(name);
            }
        }
        for (const entry of biosList) {
            const name = typeof entry === "string" ? entry : entry.name;
            const key = String(name).toLowerCase();
            if (name && !seen.has(key)) {
                seen.add(key);
                names.push(name);
            }
        }

        // Le premier choix reste « celui que la bibliothèque décide » : c'est
        // le cœur qui balaie son dossier, et ne rien choisir est le cas
        // ordinaire.
        return [
            { value: "", label: t("retrogaming.detail.biosDefault") },
            ...names.map((name) => ({
                value: name,
                label: name,
                // ABSENT = DÉSACTIVÉ, et l'option reste VISIBLE : on lit ainsi
                // ce qui manque, au lieu de l'ignorer. Le sac d'attributs vient
                // de #160, précisément pour ça.
                attributes: { disabled: !biosIsPresent(presentBios, name) },
            })),
        ];
    }, [consoles, consoleId, biosList, presentBios, t]);
    // CE QUI VA SE PASSER, EN UN SEUL ENDROIT (#163).
    //
    // La décision vivait dispersée dans le `isDisabled` du bouton : un
    // empilement de six conditions qui savait dire « mort » sans dire
    // pourquoi. `launchModeFor` rend le couple (mode, refus), donc l'écran
    // peut enfin l'expliquer — et chaque entrée est maintenant dite
    // honnêtement, ce qui n'était pas le cas tant que `noCore` valait `false`
    // en dur et que les BIOS n'étaient vérifiés que sur l'appareil (#162).
    //
    // `sourceOnline` VIENT DE LA QUESTION POSÉE À L'OUVERTURE (chantier 15) :
    // le serveur visé a-t-il répondu, à l'instant (`GET /api/v1/servers/{id}`).
    // C'est le signal vivant qui manquait — `server-offline` ne se produisait
    // jamais ici, faute de lui. Une cible d'ici n'en dépend pas.
    // Les possibilités qu'un refus propose, une par ligne — rien quand il
    // n'en propose pas.
    const noLaunchSteps = (slug) => {
        const steps = t(`retrogaming.detail.noLaunchSteps.${slug}`, { returnObjects: true, defaultValue: [] });

        return Array.isArray(steps) ? steps : [];
    };
    const launchDecision = launchModeFor({
        source: sourceOf(rom).source,
        inLibrary: alreadyLocal,
        target: selectedTarget?.server ? "peer" : "here",
        sourceOnline: checking || !targetServer || checked?.found?.online !== false,
        // IL A RÉVOQUÉ NOTRE LIEN (chantier 20) : ses jeux ne se lisent plus.
        tokenRefused: !checking && checked?.found?.refused === true,
        // NOTRE COMPTE CHEZ LUI (chantier 20) : son jeton collé, accepté, et
        // ce qu'il y lance — les consoles de sa session. Chez un ami, ce sont
        // ses droits à lui qui comptent, pas ceux d'ici.
        signedIn: Boolean(federatedPlay.session),
        accountRefused: federatedPlay.code === "accountRefused",
        remoteLaunch: checking || allows(federatedPlay.session?.consoles, consoleMeta),
        // CE COMPTE-CI LANCE-T-IL CETTE CONSOLE ICI (chantier 20).
        consoleGranted: allows(user?.consoles, consoleMeta),
        coreAvailable: Boolean(selectedCore) && !coreMissingOn(selectedTarget, coreFiles),
        // Les BIOS lus ici sont les NÔTRES : chez un pair, c'est lui qui
        // juge, comme pour le cœur (`coreMissingOn`).
        biosAvailable: runsElsewhere || !biosMissing,
        playsHere,
    });
    const handleLaunch = async () => {
        // Each launch spawns a container and the sessions service enforces
        // one session per player (a new launch kills the previous one) —
        // six rapid clicks opened six tabs pointing at six containers,
        // five of which were then stopped, leaving dead tabs. Guard the
        // whole launch (including the await) so repeated clicks are no-ops.
        if (isLaunching) {
            return;
        }

        setIsLaunching(true);

        try {
            // PLUS RIEN NE DESCEND AVANT DE JOUER (chantier 14) : un jeu d'ici
            // part tel quel, un jeu fédéré part chez son serveur. Ce bouton
            // téléchargeait un jeu distant dans la bibliothèque du téléphone
            // avant de l'ouvrir ; `launchModeFor` refuse désormais un jeu qui
            // n'est pas dans la bibliothèque, ici.
            // Saved right before launching (same order as remote-play's Connect)
            // so THIS browser keeps its own preference for the next launch.
            // LA SESSION CHEZ L'AMI, au moment de jouer (chantier 20) : notre
            // kernel l'échange contre le jeton de notre compte chez lui.
            // Refusée, le lancement ne part pas, et la raison se dit.
            // `username` voyage avec le reste parce que la file de joueurs de
            // l'hôte est indexée par nom — le nôtre n'y désigne personne
            // (C9-P4).
            let remoteHost = null;
            if (runsElsewhere && federatedServer) {
                const opened = await federationSession(federatedServer.id);
                if (!opened.ok) {
                    context.toast.open(t(`common.errorCodes.${opened.code === "serverUnreachable" ? "peerUnreachable" : opened.code}`, {
                        server: federatedServer.name,
                        defaultValue: t("common.errorCodes.peerUnreachable", { server: federatedServer.name }),
                    }), { variant: "error" });

                    return;
                }
                remoteHost = {
                    serverId: federatedServer.id,
                    origin: federatedServer.link,
                    token: opened.session.token,
                    username: opened.session.username,
                };
            }
            if (takesStreamSettings) {
                saveStreamingSettings(consoleId, streamSettings);
            }
            await onPlay(
                rom,
                selectedCore,
                selectedBios,
                takesStreamSettings ? streamSettings : null,
                remoteHost,
            );
        } finally {
            setIsLaunching(false);
        }
    };

    const handleRemoveLocal = () => {
        context.modal.open(t("retrogaming.detail.removeConfirm"), {
            variant: "confirm",
            title: rom.title,
            confirmLabel: t("app.delete"),
            cancelLabel: t("app.cancel"),
            onConfirm: async () => {
                setIsRemoving(true);
                try {
                    const error = await deviceRemoveGame(rom.reference);
                    if (error) {
                        context.toast.open(t("retrogaming.detail.removeFailed"), { variant: "error" });

                        return;
                    }
                    // La relecture prévient la fiche elle-même (voir l'effet
                    // plus haut) : la copie locale disparaît, donc le bouton
                    // et le sélecteur suivent. Le faire aussi à la main
                    // donnait deux chemins pour la même vérité.
                    await rescanLocalLibrary();
                    context.toast.open(t("retrogaming.detail.removed"), { variant: "success" });
                } catch {
                    context.toast.open(t("retrogaming.detail.removeFailed"), { variant: "error" });
                } finally {
                    setIsRemoving(false);
                }
            },
        });
    };

    // `handleAddToLibrary` a été SUPPRIMÉE le 2026-09-18. Elle faisait ranger un
    // jeu distant dans la LUDOTHÈQUE par le kernel (route `/import`), depuis
    // l'écran d'un joueur — or la ludothèque est partagée et un rôle `user` n'a
    // rien à y écrire. Un téléchargement va chez la personne, jamais dans la
    // bibliothèque ; c'est un administrateur qui l'y ajoute, par Réglages ›
    // Jeux (chantier 14).

    // RETIRER écrit sur la machine qui tient le jeu : c'est au propriétaire du
    // téléphone seul (`playsHere`), pour une copie qui est sur le téléphone.
    // Tous les autres TÉLÉCHARGENT — un membre, un invité, le serveur : le
    // fichier va dans leur navigateur, rien ne s'écrit ici (chantier 14).
    // Pour un jeu d'ICI seulement : la liste d'un serveur fédéré ne fait que
    // télécharger, même quand le même fichier est aussi sur le téléphone.
    const removable = onDevice && alreadyLocal && playsHere && sourceOf(rom).source === "local";
    const downloadLabel = removable
        ? t("retrogaming.detail.remove")
        : isDownloading
            ? t("retrogaming.detail.downloading")
            : `${t("retrogaming.detail.downloadRom")} (${formatSize(rom.sizeBytes)})`;

    return (
        <OffCanvas
            icon="bi bi-controller"
            label={rom.title}
            helpText={consoleMeta?.publisher + " · " + consoleMeta?.name}
            callback={() => handleLaunch()}
            // LA DÉCISION D'ABORD, LE GESTE ENSUITE.
            //
            // `launchDecision` couvre tout ce qui relève du LANCEMENT — cœur,
            // BIOS, jeu absent, authentification chez le pair, et depuis le
            // chantier 14 un jeu qui n'est pas dans la bibliothèque d'ici.
            isDisabled={!launchDecision.canLaunch || !selectedTarget || checking}
            buttonLabel={t("games.play")}
            // The launch can take a while (spawn + boot the emulator + readiness poll) —
            // the button must LOOK busy during it, not just swallow extra clicks
            // silently (isLaunching guard) or the user re-clicks and relaunches.
            isPending={isLaunching}
        >
            <SectionPanel>
                {/* POURQUOI JOUER NE PART PAS — dit, et non deviné.
                    `launchModeFor` calcule six empêchements précis depuis
                    #163, et l'écran les jetait TOUS dans un bouton grisé :
                    aucune clé i18n ne les nommait, aucun composant ne les
                    lisait (mesuré le 2026-09-18). C'était exactement le défaut
                    que ce fichier de règles devait supprimer — « un `isDisabled`
                    qui savait dire que le bouton est mort sans jamais dire
                    pourquoi ».
                    La clé est indexée par le SLUG du refus, sans table de
                    correspondance à tenir : un refus ajouté sans sa clé
                    s'affiche en clé brute, donc se voit, au lieu de retomber
                    dans le silence d'où l'on vient.
                    Ce qu'on PEUT faire, quand le refus en propose, se lit en
                    liste (`noLaunchSteps.<slug>`, demandé par le porteur le
                    2026-10-13) — sous le refus, une possibilité par ligne.
                    Pendant que la fiche demande au serveur visé ce qu'il nous
                    ouvre, aucun refus : il serait dit avant d'être su. */}
                {!checking && launchDecision.noLaunch && (
                    <div className="alert alert-secondary py-2 px-3 small mb-3" role="status">
                        <i className="bi bi-info-circle me-2"></i>
                        {t(`retrogaming.detail.noLaunch.${launchDecision.noLaunch}`)}
                        {noLaunchSteps(launchDecision.noLaunch).length > 0 && (
                            <ul className="mb-0 mt-1 ps-4">
                                {noLaunchSteps(launchDecision.noLaunch).map((step) => <li key={step}>{step}</li>)}
                            </ul>
                        )}
                    </div>
                )}
                <Form.Group label={t("retrogaming.origin.title")} helpText={t("retrogaming.origin.titleHelp")}>
                    <GameSource rom={rom} t={t} />
                </Form.Group>
                {/* LE MOTEUR DU JEU (chantier 36), reconnu à ses fichiers au
                    scan — absent quand il ne l'est pas : rien à en dire. */}
                {rom.engine && (
                    <Form.Group label={t("retrogaming.engine.title")} helpText={t("retrogaming.engine.titleHelp")}>
                        <div className="d-flex align-items-center gap-2">
                            <i className="bi bi-cpu"></i>
                            <span>{t(`retrogaming.engine.names.${rom.engine}`)}</span>
                        </div>
                    </Form.Group>
                )}
                <Form.Group>
                    {/* UN SEUL BOUTON, qui fait l'inverse une fois le jeu là.
                        Il y en avait deux empilés : « déjà dans la
                        bibliothèque », grisé et muet, puis « retirer » —
                        c'est-à-dire un bouton qui occupe la place sans rien
                        offrir, au-dessus de celui qui agit. Un jeu est
                        dedans ou dehors ; le bouton dit lequel des deux
                        gestes reste possible (signalé le 2026-09-01).

                        UN TÉLÉCHARGEMENT VA CHEZ LA PERSONNE, jamais dans la
                        bibliothèque — « un user normal peut télécharger sans
                        problème mais ça ne va pas dans la bibliothèque »
                        (2026-09-18) ; un jeu y entre par Réglages › Jeux
                        (chantier 14). */}
                    <bs.Button
                        className="w-100 d-inline-flex align-items-center justify-content-center gap-2"
                        variant={removable ? "danger" : "secondary"}
                        onClick={removable ? handleRemoveLocal : handleDownload}
                        disabled={isDownloading || isRemoving}
                        title={removable
                            ? t("retrogaming.detail.removeHelp")
                            : t("retrogaming.detail.download")}>
                        {removable && <i className="bi bi-trash"></i>}
                        {downloadLabel}
                    </bs.Button>
                </Form.Group>
                <hr />
                {/* PAS D'AVERTISSEMENT DE BIOS MANQUANT ICI, C'EST VOULU.
                    Le joueur n'a rien à y faire : le refus se lit déjà sur
                    l'option de BIOS grisée et sur le bouton Jouer qui ne part
                    pas. Ce qui manque se gère à l'administration, qui le sait
                    déjà — et le bouton « récupérer depuis un serveur » qui
                    vivait ici mentait en plus (`found` n'y était jamais
                    déclaré : la réussite partait en ReferenceError et
                    s'affichait en échec). Retiré le 2026-09-18. */}
                {/* L'ÉMULATEUR, LÀ OÙ EST LE JEU : ceux d'ici pour un jeu
                    d'ici, ceux de l'ami pour le sien (`launchTargets`). */}
                {launchTargets.length === 0 && (
                    <div className="text-muted small mb-3">
                        {t("retrogaming.detail.noEmulator")}
                    </div>
                )}
                {/* TOUJOURS AFFICHÉ, MÊME À UNE SEULE OPTION — et les
                    indisponibles sont montrées désactivées plutôt que tues
                    (demandé le 2026-09-17). Lire le nom de l'émulateur qui va
                    servir fait partie de ce qu'on est venu voir ; et un cœur
                    absent qu'on n'affiche pas laisse la personne chercher
                    pourquoi le bouton ne part pas.
                    C'est l'OPTION qui est désactivée, jamais le sélecteur —
                    la brique existe depuis #160 (le sac d'attributs de
                    `Form.Select`).
                    LIMITE ASSUMÉE : on ne désactive que les cibles D'ICI.
                    L'inventaire d'un pair ne se lit pas depuis cette page
                    (`fetchRequiredCores` est une route d'administration), donc
                    ses options restent actives — le refus, s'il y en a un,
                    viendra de lui. */}
                {launchTargets.length > 0 && (
                    <Form.Group
                        label={t("retrogaming.detail.emulator")}
                        helpText={t("retrogaming.detail.emulatorHelp")}
                    >
                        <Form.Select
                            name="target"
                            context={targetFormContext}
                            options={launchTargets.map((target) => ({
                                attributes: { disabled: coreMissingOn(target, coreFiles) },
                                value: target.id,
                                // LE NOM DU CŒUR SEUL : l'endroit est celui
                                // du jeu, déjà dit par sa provenance, et il
                                // n'y en a plus qu'un (2026-10-22).
                                label: target.core,
                            }))}
                        />
                    </Form.Group>
                )}

                {/* PAS DE SÉLECTEUR DE SOURCE ICI : il est dans la LISTE.
                    La fiche en portait un second, alimenté par le repliement
                    des copies d'un même titre. Les deux pouvaient se
                    contredire — filtrer sur « kDrive » puis lancer la copie
                    locale — et la question ne se pose plus qu'une fois, en
                    amont. D'où vient ce jeu reste dit, plus haut, à côté du
                    bouton de téléchargement. Retiré le 2026-09-18. */}
                {/* Une console qui n'attend aucun BIOS et n'en porte aucun
                    n'affiche rien — la plupart sont dans ce cas, et un
                    sélecteur à une seule entrée « par défaut » ne dirait rien.
                    Dès qu'il y a quelque chose à montrer, il reste affiché
                    MÊME À UNE SEULE OPTION : on vient aussi y lire le nom de
                    ce qui va servir (demandé le 2026-09-17).
                    Une liste déroulante et non une grille de boutons : la PS1
                    porte à elle seule vingt-quatre dumps réels. */}
                {/* NOS BIOS, POUR UNE PARTIE D'ICI SEULEMENT. Chez un ami,
                    ce sont les siens qui servent : lister les nôtres n'y
                    voudrait rien dire (le porteur, 2026-10-22). */}
                {!runsElsewhere && biosOptions.length > 1 && (
                    <Form.Group
                        label={t("retrogaming.detail.bios")}
                        helpText={t("retrogaming.detail.biosHelp2")}
                        props={{ style: { marginBottom: 0 } }}>
                        <Form.Select
                            name="bios"
                            context={biosFormContext}
                            options={biosOptions}
                        />
                    </Form.Group>
                )}
            </SectionPanel>

            {/* LE STATUT DU JEU (chantier 30) : par émulateur et ses notes,
                tel que le serveur du jeu l'a établi — sans version. */}
            <GameStatusDetails status={rom.status} t={t} />

            {/* QUI JOUE CHEZ CELUI QUI VA LANCER LE JEU (C9-P5, chantier 20).
                Même panneau que pour une console fédérée, et volontairement :
                les deux posent la même question — sous quel nom je joue chez
                lui. Sans son jeton, le formulaire pour le coller ; avec, le
                nom sous lequel on y joue. Il n'apparaît que pour un jeu qui se
                joue ailleurs, et seulement s'il répond et accepte notre lien :
                coller un jeton n'y ferait rien sinon. */}
            {runsElsewhere && federatedServer && !checking
                && launchDecision.noLaunch !== NO_LAUNCH.serverOffline
                && launchDecision.noLaunch !== NO_LAUNCH.federationRefused && (federatedPlay.session
                ? <FederatedAccount server={federatedServer} account={federatedPlay.session} t={t} />
                : <FederatedTokenForm
                    server={federatedServer}
                    refused={federatedPlay.code === "accountRefused"}
                    t={t}
                    context={context} />)}

            {/* Les réglages de flux — voir `stream/shared/StreamSettings.jsx`.
                Ils voyagent avec la demande de lancement et deviennent les
                variables du conteneur. Le même panneau sert remote-play, à
                ceci près qu'une PS5 DÉCLARE ce qu'elle accepte, alors qu'un
                conteneur prend ce qu'on lui donne.
                Le rétro suit la même règle depuis #332 (voir
                `takesStreamSettings`) : la résolution y devient le canevas de
                l'encodeur, la cadence celle des images que le moteur rend, le
                débit celui de l'encodeur. Agrandir 256x240 se fait au plus
                proche voisin, sans ajouter de détail : le réglage sert d'abord
                à tenir la connexion. */}
            {takesStreamSettings && (
                <StreamSettings t={t} context={streamSettingsContext} toast={context.toast} />
            )}
        </OffCanvas >
    );
}

