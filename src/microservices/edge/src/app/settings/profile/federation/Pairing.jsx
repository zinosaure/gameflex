// Route : /settings/profile/federation/pairing — LE CODE D'APPAIRAGE DE MON COMPTE (chantier 60), sous Réglages › Profil ›
// Fédération › Créer mon jeton › « Alternative » : mon mode compte en tête, puis « Créer le code », désactivé tant qu'il est
// fermé. Le code créé prend la page (PairingReveal) ; servi, la page revient à celle de mon jeton.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { cancelPairingCode, createPairingCode } from "../../../libs/api.js";
import { DiscoveryModeSection, PairingReveal, usePairing } from "../../../screens/access/Pairing.jsx";
import { toast } from "../../../shell/Shell.jsx";
import SettingsPage, { PROFILE, useSettingsNavigation } from "../../SettingsPage.jsx";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { TOKEN_GUIDES } from "./Token.jsx";

export default function Pairing() {
    const { t } = useTranslation();
    const pairing = usePairing();
    const { back } = useSettingsNavigation();
    const [pending, setPending] = useState(false);
    const [created, setCreated] = useState(null);
    const open = Boolean(pairing?.account);
    const parent = `${PROFILE}/federation/token`;

    const create = async () => {
        setPending(true);
        const answer = await createPairingCode({ kind: "account" });
        setPending(false);

        if (!answer.ok || !answer.item) {
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }
        setCreated(answer.item);
    };
    const finish = (text) => {
        toast(text);
        setCreated(null);
        back(parent);
    };

    return (
        <SettingsPage title={t("pairing.code")} parent={parent} levels={{ [parent]: t("settings.items.token.label") }}>
            {created ? (
                <PairingReveal created={created} pairing={pairing} onDone={finish} onCancel={() => cancelPairingCode(created.id)} />
            ) : (
                <>
                    <DiscoveryModeSection scope="account" pairing={pairing} />
                    <Section title={t("pairing.code")} help={t("pairing.codeSectionHelp")} sense="add">
                        <Rows>
                            <NavRow id="tk-pairing-create" icon="bi-123" label={t("pairing.create")}
                                help={open ? t("pairing.myCodeRowHelp") : t("pairing.createNeedsMode")} disabled={!open} pending={pending} opens="action" onOpen={create} />
                        </Rows>
                    </Section>
                    <GuideSection guides={TOKEN_GUIDES} />
                </>
            )}
        </SettingsPage>
    );
}
