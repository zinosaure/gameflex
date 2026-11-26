// Route : /settings/profile/federation/servers/<id> — MON COMPTE FÉDÉRÉ CHEZ CE SERVEUR — sous quel compte j'y joue (et
// l'oublier), ou le formulaire où coller son invitation ou son jeton, ou pourquoi on n'y est pas ; puis le guide.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { useData } from "../../../../libs/data.jsx";
import { useFederationSession } from "../../../../libs/federation.js";
import { FederatedAccount, FederatedRefusal, FederatedTokenForm } from "../../../../screens/access/FederatedAccount.jsx";
import SettingsPage, { NotFound } from "../../../SettingsPage.jsx";
import { GuideSection } from "../../../../bricks/GuideSection.jsx";
import { Loading } from "../../../../bricks/Loading.jsx";
import { Section } from "../../../../bricks/Section.jsx";
import { FEDERATION_GUIDES, named, SERVERS } from "../../Federation.jsx";

export default function Server() {
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
        return <SettingsPage title={t("myFed.servers")} parent={SERVERS} levels={levels} guides={FEDERATION_GUIDES}>{reread ? <NotFound /> : <Section><Loading /></Section>}</SettingsPage>;
    }

    return (
        <SettingsPage title={server.name} parent={SERVERS} levels={levels}>
            {session ? <FederatedAccount server={server} account={session} />
                : loading || !code ? <Section title={t("gear.peerAccount")} sense="info"><Loading /></Section>
                    : code === "noFederatedAccount" || code === "accountRefused" ? <FederatedTokenForm server={server} refused={code === "accountRefused"} />
                        : <FederatedRefusal server={server} code={code} />}
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}
