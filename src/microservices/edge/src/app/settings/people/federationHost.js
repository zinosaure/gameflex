import { createContext, useContext } from "react";
import * as api from "../../libs/api.js";
import { can, useData } from "../../libs/data.jsx";
import { usePairing } from "../../screens/access/Pairing.jsx";
import { useShareOrigin } from "../../screens/access/Invitation.jsx";
import SettingsPage from "../SettingsPage.jsx";

// L'HÔTE DES PAGES DE LA FÉDÉRATION (chantier 61, option B ; le porteur, 2026-11-18 : « prendre la même interface et
// fonctionnalités que dans /settings/federation pour le gameflex-federation. Ça évitera d'avoir 2 codes différents pour
// faire la même chose ») : les pages de Réglages › Fédération (accès délivrés, invitations, jeton, code d'appairage,
// mode découverte, page d'un jeton) lisent ici à qui elles parlent. GameFlex est l'hôte par défaut : sans fournisseur,
// rien ne change. gameflex-federation fournit le sien (federation/host.js) — son API d'administration traduite dans
// les formes d'edge, son mode découverte par son flux d'événements, son cadre de page — et ne rejoint aucun serveur
// (« on fait que du output et non du input par fédération »).
//
//   base          le chemin de l'accueil de la fédération ("/" pour le service)
//   Page          le cadre d'une page : {title, parent, levels, enter, children}
//   joins         les serveurs rejoints existent ici (GameFlex) ou non (le service)
//   offers        ce qu'un accès peut prêter en plus des jeux : `consoles` (ses consoles, à restreindre une à une, chantier 66),
//                 `providers` (les fournisseurs). gameflex-federation ne prête aucune console : sa page n'en montre rien.
//   useIssues     l'hôte émet-il des accès (un hook)
//   useServerName, useOrigin, usePairing, useJoinedCount, useInApp   son nom, son adresse publique, le sujet `pairing`,
//                 le nombre de serveurs rejoints, l'application ou non (des hooks)
//   invitationQR  le QR d'un lien : un Blob, ou null
//   api           ses appels, aux formes d'edge : {ok, status, item | items, detail}
//   useHomeTitle, HomeBefore, HomeAfter   ce que l'accueil du service ajoute (son nom, ses ressources, ses journaux)
export const GAMEFLEX_HOST = {
    base: "/settings/federation",
    Page: SettingsPage,
    joins: true,
    offers: { consoles: true, providers: true },
    useIssues: () => can(useData().capabilities, "issuesCatalogTokens"),
    useServerName: () => useData().capabilities?.serverName || "",
    useOrigin: () => useShareOrigin(),
    usePairing: () => usePairing(),
    useJoinedCount: () => (useData().servers || []).length,
    // Dans l'application (son propre partage) : la révélation d'une invitation n'y propose pas le partage d'un fichier.
    useInApp: () => {
        const { capabilities } = useData();

        return can(capabilities, "appHome") || can(capabilities, "appHomeLink");
    },
    // Le QR d'un lien, dessiné par le kernel : un Blob, ou null.
    invitationQR: (text) => api.invitationQR(text),
    api: {
        listTokens: api.listCatalogTokens,
        createToken: api.createCatalogToken,
        updateTokenScopes: api.updateCatalogTokenScopes,
        updateTokenConsoles: api.updateCatalogTokenConsoles,
        revokeToken: api.revokeCatalogToken,
        listInvitations: api.listInvitations,
        createInvitation: api.createInvitation,
        revokeInvitation: api.revokeInvitation,
        createPairingCode: api.createPairingCode,
        cancelPairingCode: api.cancelPairingCode,
        openPairing: api.openPairing,
        closePairing: api.closePairing,
    },
    useHomeTitle: () => null,
    HomeBefore: null,
    HomeAfter: null,
};

export const FederationHost = createContext(GAMEFLEX_HOST);

export const useFederationHost = () => useContext(FederationHost);

// Les chemins des pages, sous l'accueil de l'hôte.
export const federationPaths = (base) => {
    const at = (sub) => (base === "/" ? `/${sub}` : `${base}/${sub}`);

    return { FEDERATION: base, ACCESSES: at("access"), PENDING: at("pending"), JOINED: at("servers"), at };
};

export const useFederationPaths = () => federationPaths(useFederationHost().base);
