import React, { useContext } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import Skeleton from "../../skeleton/skeleton.js";
import { useAuth } from "./auth/AuthContext.jsx";
import { GamepadProvider } from "../hooks/use-gamepad.jsx";
import {
    Page,
    SectionPanel,
    FormButton,
    ListElement,
    OffCanvas,
} from "../../gameflex/shared/templates/Essentials.jsx";
import Profile from "./settings/Profile.jsx";
import Password from "./settings/Password.jsx";
import Notifications from "./settings/Notifications.jsx";
import Gamepad from "./settings/Gamepad.jsx";
import Preferences from "./settings/Preferences.jsx";
import Download from "./settings/Download.jsx";

// library sections
import SavedStates from "./settings/library/SavedStates.jsx";

// admin-only sections (Members, Storage, DeviceStorage, TurnServer, Federation, Servers) are imported in Settings.jsx itself, not here, to avoid loading them for non-admins.
import Members from "./settings/admin/Members.jsx";
import TurnServer from "./settings/admin/TurnServer.jsx";
import ROMs from "./settings/admin/ROMs.jsx";
import StorageCores from "./settings/admin/storage/Cores.jsx";
import StorageBios from "./settings/admin/storage/Bios.jsx";
import StorageSaves from "./settings/admin/storage/Saves.jsx";
import StorageCache from "./settings/admin/storage/Cache.jsx";
import StorageLogs from "./settings/admin/storage/Logs.jsx";
import StorageParams from "./settings/admin/storage/Params.jsx";
import StoragePatches from "./settings/admin/storage/Patches.jsx";
import Federations from "./settings/admin/Federations.jsx";
import RemoteServers from "./settings/admin/RemoteServers.jsx";
import InternalDisks from "./settings/admin/InternalDisks.jsx";
import RescanAll from "./settings/admin/RescanAll.jsx";

import { can } from "../libs/capabilities.js";
import { apiUrl } from "../libs/serverBase.js";

// LA DERNIÈRE VERSION PUBLIÉE DE L'APPLICATION — l'APK de la Release GitHub,
// pour toute installation qui n'est pas un banc d'essai.
const RELEASE_URL = "https://github.com/zinosaure/gameflex/releases/latest";


// returnToAppHome — referme l'interface web de l'application et ramène sa
// console (POST /api/v1/device/home, DeviceWindowHost.onHome). Faux si
// l'application n'a pas répondu.
//
// Dans l'application d'un INVITÉ (`appHomeLink`, chantier 32), cet hôte n'a
// pas de console derrière la page : le lien réservé `gameflex://home`, que la
// WebView de l'invité intercepte, la ramène à son propre accueil.
async function returnToAppHome() {
    if (can("appHomeLink")) {
        window.location.href = "gameflex://home";

        return true;
    }
    try {
        const response = await fetch(apiUrl("/api/v1/device/home"), { method: "POST" });

        return response.ok;
    } catch {
        return false;
    }
}

export default function Settings() {

    const { t } = useTranslation();
    const { user, token, logout, isAdmin } = useAuth();
    const navigate = useNavigate();
    const context = useContext(Skeleton.context);

    // `profile.*` ET NON `settings.*` pour le titre et le texte d'aide de cette
    // page : les deux appels pointaient `settings.title` / `settings.description`,
    // qui n'existent dans AUCUNE des deux langues — la page affichait donc ses
    // clés brutes (trouvé par l'audit i18n des conventions, le 2026-09-14). Les
    // clés justes existaient déjà, l'appel avait dérivé d'espace de noms :
    // repointé plutôt que dupliqué, deux clés de plus auraient caché la dérive.
    //
    // Ce commentaire est ici, et non au milieu des attributs de `<Page>`, parce
    // qu'un `{/* … */}` n'est valide qu'en position d'ENFANT : Babel y attend un
    // attribut de diffusion et refuse le fichier entier (« Unexpected token,
    // expected "..." »). C'est exactement ce que j'ai cassé en l'écrivant.
    return (
        <Page
            label={t("profile.settingsMenu")}
            helpText={t("profile.settingsDescription")}
        >
            <SectionPanel
                label={t("settings.generalTitle")}
                helpText={t("settings.generalHelp")}>
                {/* Le téléphone a de VRAIS comptes depuis le 2026-09-04 :
                    chacun crée le sien, et le porteur est administrateur.
                    Ces deux écrans étaient masqués parce qu'il n'y en avait
                    qu'un seul, qui se connectait tout seul — ce n'est plus le
                    cas, et les cacher priverait l'administrateur du téléphone
                    de la gestion de ses membres. */}
                {can("accounts") && (
                    <>
                        <ListElement
                            icon="bi bi-person"
                            label={t("settings.profileTitle")}
                            helpText={t("settings.profileHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-person"
                                        label={t("settings.profileTitle")}
                                        helpText={t("settings.profileHelp")}>
                                        <Profile context={context} user={user} token={token} t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                        <ListElement
                            icon="bi bi-lock"
                            label={t("settings.passwordTitle")}
                            helpText={t("settings.passwordHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-lock"
                                        label={t("settings.passwordTitle")}
                                        helpText={t("settings.passwordHelp")}>
                                        <Password context={context} user={user} token={token} t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                    </>
                )}
                {/* Les notifications supposent un DESTINATAIRE : elles
                    partent d'un compte vers un autre (une invitation, une
                    demande de manette). Un appareil n'a pas d'annuaire, et
                    l'écran s'ouvrait sur des réglages sans effet. Et seul un
                    hôte qui sert les notifications push les déclare : voir
                    `pushNotifications` dans capabilities.js. */}
                {can("accounts") && can("pushNotifications") && (
                    <ListElement
                        icon="bi bi-bell"
                        label={t("settings.notificationsManage")}
                        helpText={t("settings.notificationsSection")}
                        onClick={() => {
                            context.offcanvas.open(
                                <OffCanvas
                                    icon="bi bi-bell"
                                    label={t("settings.notificationsTitle")}
                                    helpText={t("settings.notificationsHelp")}>
                                    <Notifications context={context} user={user} token={token} t={t} />
                                </OffCanvas>, {
                                size: 600,
                            });
                        }}
                    />
                )}
                <ListElement
                    icon="bi bi-gear"
                    label={t("settings.appTitle")}
                    helpText={t("settings.appHelp")}
                    onClick={() => {
                        context.offcanvas.open(
                            <OffCanvas
                                icon="bi bi-gear"
                                label={t("settings.appTitle")}
                                helpText={t("settings.appHelp")}>
                                <Preferences context={context} user={user} token={token} t={t} />
                            </OffCanvas>, {
                            size: 600,
                        });
                    }}
                />
            </SectionPanel>
            <SectionPanel
                label={t("profile.library.mineTitle")}
                helpText={t("profile.library.mineHelp")}>
                <ListElement
                    icon="bi bi-shield-check"
                    label={t("profile.library.saves")}
                    helpText={t("profile.library.savesHelp")}
                    onClick={() => {
                        context.offcanvas.open(
                            <OffCanvas
                                icon="bi bi-shield-check"
                                label={t("profile.library.saves")}
                                helpText={t("profile.library.savesHelp")}>
                                <SavedStates context={context} user={user} t={t} />
                            </OffCanvas>, {
                            size: 600,
                        });
                    }}
                />
            </SectionPanel>
            <SectionPanel
                label={t("settings.padsTitle")}
                helpText={t("settings.padsHelp")}>
                <ListElement
                    icon="bi bi-controller"
                    label={t("settings.padsSection")}
                    helpText={t("settings.padsSectionHelp")}
                    onClick={() => {
                        context.offcanvas.open(
                            <OffCanvas
                                icon="bi bi-controller"
                                label={t("settings.padsSection")}
                                helpText={t("settings.padsSectionHelp")}>
                                <GamepadProvider>
                                    <Gamepad context={context} user={user} token={token} t={t} />
                                </GamepadProvider>
                            </OffCanvas>, {
                            size: 600,
                        });
                    }}
                />
            </SectionPanel>
            {isAdmin && (
                <>
                    <SectionPanel
                        label={t("settings.adminTitle")}
                        helpText={t("settings.adminHelp")}>
                        {can("accounts") && (
                            <ListElement
                                icon="bi bi-people"
                                label={t("settings.membersTitle")}
                                helpText={t("settings.membersHelp")}
                                onClick={() => {
                                    context.offcanvas.open(
                                        <OffCanvas
                                            icon="bi bi-people"
                                            label={t("settings.membersTitle")}
                                            helpText={t("settings.membersHelp")}>
                                            <Members context={context} user={user} token={token} t={t} />
                                        </OffCanvas>, {
                                        size: 600,
                                    });
                                }}
                            />
                        )}
                        {/* AJOUTER des serveurs et des partages n'est pas un
                        geste d'administrateur de serveur : c'est ce qu'on
                        fait avec SES ressources, et un téléphone en a comme
                        une machine. L'entrée vivait pourtant dans le bloc
                        `serverAdmin`, donc invisible là où elle sert le plus
                        — alors que le commentaire d'à côté disait déjà le
                        contraire (signalé le 2026-09-01). */}
                        <ListElement
                            icon="bi bi-hdd-network"
                            label={t("settings.remoteTitle")}
                            helpText={t("settings.remoteHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-hdd-network"
                                        label={t("settings.remoteTitle")}
                                        helpText={t("settings.remoteHelp")}>
                                        <RemoteServers context={context} user={user} token={token} t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                        {/* Fédérer CE serveur (émettre des jetons) et régler son
                        TURN sont des gestes d'administrateur de serveur. Un
                        téléphone se fédère dans l'autre sens — il AJOUTE des
                        serveurs, ce que « Ressources distantes » fait juste
                        au-dessus — et sa signalisation passe par son propre
                        gateway. */}
                        {/* LES RESSOURCES INTERNES (chantier 23) : les autres
                        disques de jeux de CE serveur, déclarés dans son compose.
                        Un geste de serveur, donc réservé à son administrateur ;
                        un téléphone n'en a pas. */}
                        {can("serverAdmin") && (
                            <ListElement
                                icon="bi bi-device-hdd"
                                label={t("settings.internalTitle")}
                                helpText={t("settings.internalHelp")}
                                onClick={() => {
                                    context.offcanvas.open(
                                        <OffCanvas
                                            icon="bi bi-device-hdd"
                                            label={t("settings.internalTitle")}
                                            helpText={t("settings.internalHelp")}>
                                            <InternalDisks context={context} t={t} />
                                        </OffCanvas>, {
                                        size: 600,
                                    });
                                }}
                            />
                        )}
                        {can("serverAdmin") && (
                            <>
                                {/* UN SEUL ÉCRAN POUR LA FÉDÉRATION (2026-09-14).
                                Il y avait ici « Fédérer le serveur » — émettre un
                                jeton — et, juste au-dessus, « Ressources
                                distantes » en enregistrait un. Deux entrées pour
                                les deux bouts d'un même échange, chacune portant
                                un encadré dont le seul rôle était de désigner
                                l'autre. Réunies en onglets Serveurs/Jetons, sur la
                                forme de « Serveur TURN ». Le stockage distant
                                (WebDAV, kDrive) reste, lui, dans « Ressources
                                distantes » : c'est une autre API et un autre
                                sujet. */}
                                <ListElement
                                    icon="bi bi-arrow-left-right"
                                    label={t("settings.federationsTitle")}
                                    helpText={t("settings.federationsHelp")}
                                    onClick={() => {
                                        context.offcanvas.open(
                                            <OffCanvas
                                                icon="bi bi-arrow-left-right"
                                                label={t("settings.federationsTitle")}
                                                helpText={t("settings.federationsHelp")}>
                                                <Federations context={context} user={user} token={token} t={t} />
                                            </OffCanvas>, {
                                            size: 600,
                                        });
                                    }}
                                />
                                {/* `turnConfig` EN PLUS de `serverAdmin`, et il a
                                fallu l'ajouter le 2026-09-14 : le téléphone passe
                                `serverAdmin` à vrai ce jour-là (son porteur est
                                administrateur chez lui, et c'est cette garde qui
                                ouvre la fédération), mais il ne sert AUCUNE route
                                d'administration TURN. Sans cette seconde garde il
                                héritait d'une entrée qui ouvre un panneau et
                                affiche aussitôt « loadFailed ».
                                L'hôte s'exclut lui-même, par sa propre
                                déclaration : `"turnConfig": false`, avec la raison
                                écrite à côté dans mobilekernel.go. */}
                                {can("turnConfig") && (
                                    <ListElement
                                        icon="fa fa-tower-broadcast"
                                        label={t("settings.turnTitle")}
                                        helpText={t("settings.turnHelp")}
                                        onClick={() => {
                                            context.offcanvas.open(
                                                <OffCanvas
                                                    icon="fa fa-tower-broadcast"
                                                    label={t("settings.turnTitle")}
                                                    helpText={t("settings.turnHelp")}>
                                                    <TurnServer context={context} user={user} token={token} t={t} />
                                                </OffCanvas>, {
                                                size: 600,
                                            });
                                        }}
                                    />
                                )}
                            </>
                        )}
                    </SectionPanel>
                    {/* « RESCAN ALL » (chantier 15) : sa propre section, juste
                    avant « Stockage » — comme la déconnexion, un seul bouton,
                    et la date du listing en dessous. */}
                    <RescanAll t={t} />
                    {/* GESTION DU STOCKAGE — ce que la machine contient, et ce
                    qui lui manque.

                    Remplace « Ma bibliothèque », qui ne montrait que ce qui
                    était SUR LE DISQUE. C'était la moitié inutile de la
                    question : un core présent ne pose aucun problème, un core
                    absent se manifestait par un émulateur qui refuse de
                    démarrer sans dire ce qu'il cherchait. Cores et firmwares
                    partent maintenant du REGISTRE — ce qu'il FAUT avoir — et le
                    disque ne fait que répondre présent ou absent.

                    C'est aussi la partie qui diverge le plus entre les deux
                    hôtes : un serveur a une bibliothèque froide garnie à la
                    main et une racine chaude pour ce qu'il produit, un
                    téléphone n'a que du chaud. Les six entrées sont pourtant
                    les mêmes des deux côtés, et le kernel du téléphone sert les
                    mêmes routes — la différence est dans ce qu'elles trouvent,
                    pas dans ce que l'écran propose.

                    `isAdmin` : sur un serveur, le disque est celui de tout le
                    monde. Un appareil n'a qu'un porteur, et son compte root est
                    administrateur — la section y est donc visible. */}
                    <SectionPanel>
                        {/* OÙ VIT LA BIBLIOTHÈQUE D'UN TÉLÉPHONE se choisit dans
                        son application, sur l'écran de celui qui le tient (le
                        porteur, 2026-10-15) : l'entrée « Emplacement du
                        stockage » d'ici est partie, avec ses routes. */}
                        <ListElement
                            icon="bi bi-disc"
                            label={t("admin.storage.roms")}
                            helpText={t("admin.storage.romsHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-disc"
                                        label={t("admin.storage.roms")}
                                        helpText={t("admin.storage.romsHelp")}>
                                        <ROMs context={context} user={user} token={token} t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                        <ListElement
                            icon="bi bi-cpu"
                            label={t("admin.storage.cores")}
                            helpText={t("admin.storage.coresHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-cpu"
                                        label={t("admin.storage.cores")}
                                        helpText={t("admin.storage.coresHelp")}>
                                        <StorageCores t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                        <ListElement
                            icon="bi bi-fingerprint"
                            label={t("admin.storage.bios")}
                            helpText={t("admin.storage.biosHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-fingerprint"
                                        label={t("admin.storage.bios")}
                                        helpText={t("admin.storage.biosHelp")}>
                                        <StorageBios t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                        {/* LES CORRECTIFS (chantier 15) : patches/<console>/,
                            ceux d'ici et ceux d'un serveur fédéré. */}
                        <ListElement
                            icon="bi bi-bandaid"
                            label={t("admin.storage.patches")}
                            helpText={t("admin.storage.patchesHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-bandaid"
                                        label={t("admin.storage.patches")}
                                        helpText={t("admin.storage.patchesHelp")}>
                                        <StoragePatches t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                        <ListElement
                            icon="bi bi-shield-check"
                            label={t("admin.storage.saves")}
                            helpText={t("admin.storage.savesHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-shield-check"
                                        label={t("admin.storage.saves")}
                                        helpText={t("admin.storage.savesHelp")}>
                                        <StorageSaves t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                        {/* Le premier lancement d'un jeu, une fois pour tous
                            les comptes (#448) : vu et vidé entrée par entrée,
                            sur le téléphone comme sur le serveur. */}
                        <ListElement
                            icon="bi bi-hdd-stack-fill"
                            label={t("admin.storage.cache")}
                            helpText={t("admin.storage.cacheHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-hdd-stack-fill"
                                        label={t("admin.storage.cache")}
                                        helpText={t("admin.storage.cacheHelp")}>
                                        <StorageCache t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                        {/* Régler un émulateur sans reconstruire l'APK
                            (2026-10-12, #347) — sur le téléphone comme sur le
                            serveur : « c'est une feature en commun ». */}
                        <ListElement
                            icon="bi bi-sliders"
                            label={t("admin.storage.params")}
                            helpText={t("admin.storage.paramsHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-sliders"
                                        label={t("admin.storage.params")}
                                        helpText={t("admin.storage.paramsHelp")}>
                                        <StorageParams t={t} />
                                    </OffCanvas>, {
                                    size: 600,
                                });
                            }}
                        />
                        <ListElement
                            icon="bi bi-journal-text"
                            label={t("admin.storage.logs")}
                            helpText={t("admin.storage.logsHelp")}
                            onClick={() => {
                                context.offcanvas.open(
                                    <OffCanvas
                                        icon="bi bi-journal-text"
                                        label={t("admin.storage.logs")}
                                        helpText={t("admin.storage.logsHelp")}>
                                        <StorageLogs t={t} />
                                    </OffCanvas>, {
                                    size: 800,
                                });
                            }}
                        />
                    </SectionPanel>
                </>
            )}
            {/* Proposer de télécharger l'application DEPUIS l'application
                n'a pas de sens : `appDownload` le dit, plutôt que la page ne
                le devine (C8-P5).
                OÙ, ÇA DÉPEND DE L'INSTALLATION (le porteur, 2026-10-26) : un
                banc d'essai (`staging`) donne l'APK qu'il construit lui-même ;
                toute autre installation renvoie à la Release GitHub. */}
            {can("appDownload") && !can("staging") && (
                <SectionPanel
                    label={t("settings.downloadTitle")}
                    helpText={t("settings.downloadReleaseHelp")}>
                    <ListElement
                        icon="bi bi-android2"
                        label={t("settings.downloadTitle")}
                        helpText={t("settings.downloadReleaseHelp")}
                        onClick={() => window.open(RELEASE_URL, "_blank", "noopener,noreferrer")}
                    />
                </SectionPanel>
            )}
            {can("appDownload") && can("staging") && (
                <SectionPanel
                    label={t("settings.downloadTitle")}
                    helpText={t("settings.downloadHelp", { version: process.env.GAMEFLEX_VERSION })}>
                    <ListElement
                        icon="bi bi-android2"
                        label={t("settings.downloadTitle")}
                        helpText={t("settings.downloadHelp", { version: process.env.GAMEFLEX_VERSION })}
                        onClick={() => {
                            context.offcanvas.open(
                                <OffCanvas
                                    icon="bi bi-android2"
                                    label={t("settings.downloadTitle")}
                                    helpText={t("settings.downloadHelp", { version: process.env.GAMEFLEX_VERSION })}>
                                    <Download context={context} user={user} token={token} t={t} />
                                </OffCanvas>, {
                                size: 600,
                            });
                        }}
                    />
                </SectionPanel>
            )}
            {/* Se déconnecter de QUOI ? Le téléphone a un seul compte, qui se
                reconnecte tout seul au démarrage suivant : le bouton ne fait
                que provoquer un aller-retour par un écran de connexion que
                personne ne peut remplir. */}
            {/* REVENIR À L'ACCUEIL DE L'APPLICATION, sans se déconnecter (le
                porteur, 2026-10-19) : la WebView se referme et la console
                revient. Dans la seule WebView de l'application (`appHome`) —
                Chrome, sur le même téléphone, n'a pas de console derrière lui.
                Sans ce bouton, la console ne revenait qu'en remontant tout
                l'historique de la page à coups de Retour. */}
            {(can("appHome") || can("appHomeLink")) && (
                <SectionPanel
                    label={t("settings.appHomeTitle")}
                    helpText={t("settings.appHomeHelp")}>
                    <FormButton
                        callback={async () => {
                            if (!(await returnToAppHome())) {
                                context.toast.open(t("settings.appHomeFailed"), { variant: "error" });
                            }
                        }}
                        label={t("settings.appHomeButton")}
                    />
                </SectionPanel>
            )}
            {can("accounts") && (
                <SectionPanel
                    label={t("profile.logout")}
                    helpText={t("profile.logoutHelp")}>
                    <FormButton
                        variant="danger"
                        callback={() => {
                            logout();
                            navigate("/", { replace: true });
                        }}
                        label={t("profile.logout")}
                    />
                </SectionPanel>
            )}
        </Page>
    );
}
