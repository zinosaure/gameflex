// Route : /settings/members/<id>/consoles — ce qu'il peut lancer.
import React from "react";
import { useTranslation } from "react-i18next";
import { isAdministrator } from "../../globals/accounts.js";
import { updateUser } from "../../libs/api.js";
import { ConsoleGrantsSections, MemberSubPage } from "../Members.jsx";

// /settings/members/<id>/consoles : CE QU'IL PEUT LANCER (le porteur, 2026-11-14 : « All rétro, streamex et remoteplay
// doit être un lien dans more qui ouvre la page des "Console access" ») — tout le rétro, puis chaque console récente,
// puis chaque console Remote Play, un interrupteur chacune ; « * » quand tout est coché (formToGrants), ce que lit le
// kernel. Un administrateur a toutes les consoles : ses interrupteurs se montrent tous allumés, et grisés.
//
// UN BOUTON PAR SECTION (le porteur, 2026-11-25, à la place de l'enregistrement au changement du 2026-11-16) : chaque
// section enregistre ses consoles (tous les droits du compte, `PATCH /auth/users/{id}`), la réponse en fenêtre
// (ConsoleGrantsSections).
function MemberConsoles({ member, locked }) {
    const { t } = useTranslation();
    const admin = isAdministrator(member.role_level);

    return (
        <ConsoleGrantsSections id="mb" grants={member.consoles} all={admin} locked={locked}
            help={t(admin ? "members.consolesAdmin" : "members.consolesHelp")}
            save={(grants) => updateUser(member.id, { consoles: grants })} />
    );
}

export default function MemberConsolesPage() {
    const { t } = useTranslation();

    return <MemberSubPage title={t("members.consolesTitle")} render={(member, locked) => <MemberConsoles member={member} locked={locked} />} />;
}
