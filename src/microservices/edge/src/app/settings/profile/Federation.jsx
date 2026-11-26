// Route : /settings/profile/federation — RÉGLAGES › PROFIL › FÉDÉRATION (le porteur, 2026-11-16, 2.1 : « dans profile, remplace token par fédération. Il y aura
// 2 liens dans celui-ci : créer mon token et s'authentifier chez les serveurs fédérés ») :
//
//   /settings/profile/federation                    les liens, et le guide (ce fichier) ;
//   …/federation/token                              « Créer mon jeton » (federation/Token.jsx) ;
//   …/federation/pairing                            « Code d'appairage » : mon mode compte et mon code (chantier 60,
//                                                   federation/Pairing.jsx) ;
//   …/federation/connections                        « Connexions avec mon jeton » (#781, federation/Connections.jsx ;
//                                                   une connexion, federation/connections/Connection.jsx) : les serveurs et appareils qui
//                                                   s'en servent — le nom qu'ils se donnent, l'adresse d'où ils sont
//                                                   venus, la première et la dernière fois ;
//   …/federation/servers                            « M'authentifier chez les serveurs fédérés » (federation/Servers.jsx) : les serveurs fédérés
//                                                   de ce GameFlex (`GET /servers`, ouvert à tout compte d'ici) ;
//   …/federation/servers/<id>                       un serveur (federation/servers/Server.jsx) : MON COMPTE FÉDÉRÉ chez lui — le formulaire où je colle
//                                                   l'invitation ou le jeton de mon compte de là-bas
//                                                   (FederatedAccount.jsx, le même que l'engrenage d'un jeu et les
//                                                   sauvegardes d'un serveur fédéré), et le compte sous lequel j'y joue.
//                                                   Aucune ligne par console : toutes demandent un compte (le porteur,
//                                                   2026-11-16 : « les consoles ne sont accessibles que par
//                                                   authentification »).
//
// CHACUN AUTHENTIFIE SON PROPRE COMPTE : le jeton collé est gardé avec mon compte d'ici, sur tous mes appareils, et
// l'administrateur n'y voit rien (`PUT /servers/{id}/account`, localOnly — un membre comme un administrateur). Ajouter
// ou retirer un SERVEUR fédéré reste à l'administrateur, dans Réglages › Fédération.
//
// LE VOCABULAIRE (3.1) : un SERVEUR FÉDÉRÉ est un autre GameFlex qu'un administrateur a ajouté ici, avec le jeton que
// l'autre lui a donné — ses jeux se voient et se téléchargent ; un COMPTE FÉDÉRÉ est mon compte chez lui, dont je colle le
// jeton une fois — valide, il permet de jouer sur ses consoles.

import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { fetchMyToken } from "../../libs/api.js";
import SettingsPage, { PROFILE, useSettingsNavigation } from "../SettingsPage.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";

export const FEDERATION = `${PROFILE}/federation`;
export const SERVERS = `${FEDERATION}/servers`;
export const CONNECTIONS = `${FEDERATION}/connections`;

// La page de MON COMPTE FÉDÉRÉ chez un serveur : là, et nulle part ailleurs, il se colle ou s'oublie (l'engrenage d'un jeu
// fédéré y renvoie, GameSettings.jsx).
export const federatedAccountPath = (serverId) => `${SERVERS}/${encodeURIComponent(serverId)}`;

// Le guide de la fédération, au titre qui dit ce que sont un serveur et un compte fédérés, et ce qu'il faut pour jouer.
export const FEDERATION_GUIDES = [["federation", "federated-servers-and-federated-accounts"]];

// Un serveur fédéré, et le nom sous lequel il se montre (son nom, sinon son lien).
export const named = (server) => ({ ...server, name: server.hostname || server.link });

// LES CONNEXIONS AVEC MON JETON, lues une fois par page (la liste, et la fiche d'une connexion relue après un
// rechargement) ; `reread` les relit (après « Révoquer »).
export function useConnections(reread = 0) {
    const [items, setItems] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchMyToken().then((result) => {
            if (!cancelled) {
                setItems(result.ok ? result.item?.token?.connections || [] : []);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [reread]);

    return items;
}

// Une connexion se nomme par ce qu'elle donne (son nom) et d'où elle vient (son adresse) : la clé de sa fiche.
export const connectionKey = (row) => `${row.name || ""}|${row.address || ""}`;
export const connectionIcon = (row) => (row.here ? "bi-house-check" : row.kind === "phone" ? "bi-phone" : row.kind === "server" ? "bi-hdd-network" : "bi-plug");

// LES LIENS : mon jeton, qui s'en sert, et mes comptes chez les serveurs fédérés.
export default function Federation() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();

    return (
        <SettingsPage title={t("settings.items.myFederation.label")} parent={PROFILE}>
            <Section title={t("myFed.title")} help={t("myFed.help")} sense="modify">
                <Rows>
                    <NavRow id="mf-token" icon="bi-key" label={t("settings.items.token.label")} help={t("settings.items.token.help")}
                        onOpen={() => open(`${FEDERATION}/token`, "mf-token")} />
                    <NavRow id="mf-connections" icon="bi-plug" label={t("myFed.connections")} help={t("myFed.connectionsHelp")}
                        onOpen={() => open(`${FEDERATION}/connections`, "mf-connections")} />
                    <NavRow id="mf-servers" icon="bi-person-check" label={t("myFed.servers")} help={t("myFed.serversHelp")}
                        onOpen={() => open(SERVERS, "mf-servers")} />
                </Rows>
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}
