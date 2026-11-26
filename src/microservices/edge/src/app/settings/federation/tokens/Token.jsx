// Route : /settings/federation/tokens/<id> — un accès délivré.
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { revokeCatalogToken, updateCatalogTokenRemoteAdmin, updateCatalogTokenScopes } from "../../../libs/api.js";
import SettingsPage, { useSettingsNavigation } from "../../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../../profile/Federation.jsx";
import { ACCESSES, at, IssuedTokenSections, loadTokens, savedConsoles, useLevels, usePassed, Waiting } from "../../Federation.jsx";

export default function Token() {
    const { t } = useTranslation();
    const { tokenId } = useParams();
    const { back, open } = useSettingsNavigation();
    const loaded = usePassed("token", tokenId, () => loadTokens());
    const passed = loaded && savedConsoles.has(String(loaded.id)) ? { ...loaded, consoles: savedConsoles.get(String(loaded.id)) } : loaded;
    const levels = useLevels();

    if (!passed) {
        return <Waiting item={passed} title={t("fed.accessesTitle")} parent={ACCESSES} />;
    }

    return (
        <SettingsPage title={passed.label} parent={ACCESSES} levels={levels} guides={FEDERATION_GUIDES}>
            <IssuedTokenSections token={passed} onScopes={(scopes) => updateCatalogTokenScopes(passed.id, scopes)}
                onConsoles={() => open(at(`tokens/${passed.id}/consoles`), `fd-consoles-${passed.id}`, { token: passed })}
                onRemoteAdmin={(allowed) => updateCatalogTokenRemoteAdmin(passed.id, allowed)}
                onRevoke={() => revokeCatalogToken(passed.id)} onRevoked={() => back(ACCESSES)} />
        </SettingsPage>
    );
}
