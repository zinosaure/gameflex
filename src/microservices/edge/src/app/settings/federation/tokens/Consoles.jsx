// Route : /settings/federation/tokens/<id>/consoles — les consoles qu'un accès ouvre.
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { updateCatalogTokenConsoles } from "../../../libs/api.js";
import { ConsoleGrantsSections } from "../../Members.jsx";
import SettingsPage from "../../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../../profile/Federation.jsx";
import { ACCESSES, at, loadTokens, savedConsoles, useLevels, usePassed, Waiting } from "../../Federation.jsx";

// /settings/federation/tokens/<id>/consoles : CE QUE LES COMPTES VENUS PAR CET ACCÈS PEUVENT LANCER (chantier 66 ; le
// porteur, 2026-11-19 : « je peux soit couper des consoles à certains accès délivrés ») — la page des consoles d'un
// membre, les mêmes interrupteurs, un bouton par section. Un serveur de salon qui a besoin de son GPU coupe la PS5
// à un ami d'un geste, sans toucher à ses comptes.
export default function TokenConsoles() {
    const { t } = useTranslation();
    const { tokenId } = useParams();
    const token = usePassed("token", tokenId, () => loadTokens());
    const parent = at(`tokens/${tokenId}`);
    const levels = { ...useLevels(), [parent]: token ? token.label : "…" };

    if (!token) {
        return <Waiting item={token} title={t("members.consolesTitle")} parent={ACCESSES} />;
    }

    return (
        <SettingsPage title={t("members.consolesTitle")} parent={parent} levels={levels} guides={FEDERATION_GUIDES}>
            <ConsoleGrantsSections id="fdc" grants={savedConsoles.get(String(token.id)) || token.consoles || ["*"]} help={t("fed.consolesPageHelp")}
                save={async (grants) => {
                    const answer = await updateCatalogTokenConsoles(token.id, grants);

                    if (answer.ok) {
                        savedConsoles.set(String(token.id), answer.item?.consoles || grants);
                    }

                    return answer;
                }} />
        </SettingsPage>
    );
}
