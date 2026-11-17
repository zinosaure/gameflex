import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { useData } from "../../libs/data.jsx";
import { useFederationSession } from "../../libs/federation.js";
import { FederatedAccount, FederatedRefusal, FederatedTokenForm } from "../../screens/access/FederatedAccount.jsx";
import { MyPairingPage, TokenPage } from "./Token.jsx";
import SettingsPage, { NotFound, PROFILE, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { GuideSection, Listing, Note, Rows, Section } from "../../ui/Bricks.jsx";

// RÉGLAGES › PROFIL › FÉDÉRATION (le porteur, 2026-11-16, 2.1 : « dans profile, remplace token par fédération. Il y aura
// 2 liens dans celui-ci : créer mon token et s'authentifier chez les serveurs fédérés ») :
//
//   /settings/profile/federation                    les deux liens, et le guide ;
//   …/federation/token                              « Créer mon jeton » : la page du jeton d'avant (Token.jsx) ;
//   …/federation/pairing                            « Code d'appairage » : mon mode compte et mon code (chantier 60) ;
//   …/federation/servers                            « M'authentifier chez les serveurs fédérés » : les serveurs fédérés
//                                                   de ce GameFlex (`GET /servers`, ouvert à tout compte d'ici) ;
//   …/federation/servers/<id>                       un serveur : MON COMPTE FÉDÉRÉ chez lui — le formulaire où je colle
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

const FEDERATION = `${PROFILE}/federation`;
const SERVERS = `${FEDERATION}/servers`;

// La page de MON COMPTE FÉDÉRÉ chez un serveur : là, et nulle part ailleurs, il se colle ou s'oublie (l'engrenage d'un jeu
// fédéré y renvoie, GameSettings.jsx).
export const federatedAccountPath = (serverId) => `${SERVERS}/${encodeURIComponent(serverId)}`;

// Le guide de la fédération, au titre qui dit ce que sont un serveur et un compte fédérés, et ce qu'il faut pour jouer.
export const FEDERATION_GUIDES = [["federation", "federated-servers-and-federated-accounts"]];

// Un serveur fédéré, et le nom sous lequel il se montre (son nom, sinon son lien).
const named = (server) => ({ ...server, name: server.hostname || server.link });

// /settings/profile/federation : les deux liens.
function FederationPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();

    return (
        <SettingsPage title={t("settings.items.myFederation.label")} parent={PROFILE}>
            <Section title={t("myFed.title")} help={t("myFed.help")}>
                <Rows>
                    <NavRow id="mf-token" icon="bi-key" tone="amber" label={t("settings.items.token.label")} help={t("settings.items.token.help")}
                        onOpen={() => open(`${FEDERATION}/token`, "mf-token")} />
                    <NavRow id="mf-servers" icon="bi-person-check" tone="green" label={t("myFed.servers")} help={t("myFed.serversHelp")}
                        onOpen={() => open(SERVERS, "mf-servers")} />
                </Rows>
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}

// /settings/profile/federation/servers : les serveurs fédérés de ce GameFlex, une ligne chacun.
function ServersPage() {
    const { t } = useTranslation();
    const { servers } = useData();
    const { open } = useSettingsNavigation();
    const items = (servers || []).map(named);

    return (
        <SettingsPage title={t("myFed.servers")} parent={FEDERATION}>
            <Section title={t("myFed.serversTitle")} help={t("myFed.serversListHelp")}>
                <Listing items={items} empty={t("myFed.serversEmpty")}
                    render={(server) => (
                        <NavRow key={server.id} id={`mf-srv-${server.id}`} icon="bi-arrow-left-right" tone="green" label={server.name} help={server.link}
                            onOpen={() => open(`${SERVERS}/${server.id}`, `mf-srv-${server.id}`)} />
                    )} />
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}

// /settings/profile/federation/servers/<id> : MON COMPTE FÉDÉRÉ CHEZ CE SERVEUR — sous quel compte j'y joue (et
// l'oublier), ou le formulaire où coller son invitation ou son jeton, ou pourquoi on n'y est pas ; puis le guide.
function ServerAccountPage() {
    const { t } = useTranslation();
    const { serverId } = useParams();
    const { servers, refreshServers } = useData();
    const found = (servers || []).find((entry) => String(entry.id) === serverId);
    const server = found ? named(found) : null;
    const { session, code, loading } = useFederationSession(server ? server.id : null);
    const levels = { [SERVERS]: t("myFed.servers") };
    // OUVERTE PAR UN LIEN DIRECT, la liste des serveurs peut ne pas être lue encore : relue une fois avant de dire
    // « introuvable ».
    const [reread, setReread] = useState(Boolean(found));

    useEffect(() => {
        if (!found && !reread) {
            refreshServers().finally(() => setReread(true));
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [serverId]);

    if (!server) {
        return <SettingsPage title={t("myFed.servers")} parent={SERVERS} levels={levels}>{reread ? <NotFound /> : <Section><Note className="mt-0">{t("settings.loading")}</Note></Section>}</SettingsPage>;
    }

    return (
        <SettingsPage title={server.name} parent={SERVERS} levels={levels}>
            {session ? <FederatedAccount server={server} account={session} />
                : loading || !code ? <Section title={t("gear.peerAccount")}><Note className="mt-0">{t("settings.loading")}</Note></Section>
                    : code === "noFederatedAccount" || code === "accountRefused" ? <FederatedTokenForm server={server} refused={code === "accountRefused"} />
                        : <FederatedRefusal server={server} code={code} />}
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}

export function MyFederationRoutes() {
    return (
        <Routes>
            <Route index element={<FederationPage />} />
            <Route path="token" element={<TokenPage />} />
            <Route path="pairing" element={<MyPairingPage />} />
            <Route path="servers" element={<ServersPage />} />
            <Route path="servers/:serverId" element={<ServerAccountPage />} />
            <Route path="*" element={<Navigate to={FEDERATION} replace />} />
        </Routes>
    );
}
