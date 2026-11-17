import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { cancelPairingCode, createPairingCode, fetchMyToken } from "../../libs/api.js";
import { useAuth } from "../../libs/data.jsx";
import { MemberToken, TOKEN_GUIDES, TokenDetails } from "../people/Members.jsx";
import { MyInvitation, useShareOrigin } from "../../screens/access/Invitation.jsx";
import { DiscoveryModeSection, PairingReveal, usePairing } from "../../screens/access/Pairing.jsx";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { PROFILE, usePageText, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { CopyField, GuideSection, Note, Rows, Section } from "../../ui/Bricks.jsx";

// MON JETON — « Créer mon jeton », sous Réglages › Profil › Fédération (le porteur, 2026-11-16, 2.1 ; MyFederation.jsx)
// (chantier 20, Profile.jsx de la webapp) : un administrateur l'active pour une
// durée ; actif, il se copie avec le lien de ce serveur, à coller sur son propre serveur. Son
// détail d'abord — sa vraie échéance, lue sur le serveur (TokenDetails ; le porteur, 2026-11-14) —,
// puis le lien et le jeton, puis « Plus » : « Créer mon QR » (MyInvitation ; « "create my QR" doit
// passer dans more ») et « Code d'appairage » (chantier 60, MyPairingPage), grisés tant que le jeton n'est pas actif. Un administrateur gère son propre jeton comme celui d'un membre (MemberToken,
// Profile.jsx de la webapp) : sa durée, puis « Danger » — le désactiver (« "disable my token" dans
// danger avec une confirmation »), le régénérer. Un joueur n'a pas ces gestes : le serveur les
// réserve aux administrateurs.
export function TokenPage() {
    const { t } = useTranslation();
    const page = usePageText("token");
    const origin = useShareOrigin();
    const { isAdmin, user } = useAuth();
    const { open } = useSettingsNavigation();
    const [answer, setAnswer] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchMyToken().then((result) => {
            if (!cancelled) {
                setAnswer(result.ok ? result.item : null);
            }
        });

        return () => {
            cancelled = true;
        };
    }, []);

    const state = answer?.token;
    const live = Boolean(state?.enabled && state.expires_at && new Date(state.expires_at) > new Date());

    // `mine` : la ligne « Créer mon QR » (MyInvitation), pour « Plus », et celle du code d'appairage à côté.
    const content = (mine) => {
        const qr = (
            <>
                {mine}
                <NavRow id="tk-pairing" icon="bi-123" tone={live ? "indigo" : "slate"} label={t("pairing.code")} help={live ? t("pairing.myCodeRowHelp") : t("invitations.myQRNeedsToken")}
                    disabled={!live} onOpen={() => open(`${PROFILE}/federation/pairing`, "tk-pairing")} />
            </>
        );

        // Le temps de la lecture, un mot seul, sans titre : rien encore n'en mérite un.
        if (answer === undefined) {
            return <Section><Note className="mt-0">{t("settings.loading")}</Note></Section>;
        }

        if (!state) {
            return <Section title={t("settings.tokenValue")}><Note className="mt-0">{t("settings.tokenNone")}</Note></Section>;
        }

        if (isAdmin && user?.id) {
            return <MemberToken member={{ id: user.id, username: user.username, display_name: user.display_name, token: state }} more={qr}
                onChanged={(changed) => setAnswer((current) => ({ ...current, token: changed.token }))} />;
        }

        return (
            <>
                <TokenDetails token={state}>
                    {!live && <Note>{t("settings.tokenAskAdmin")}</Note>}
                </TokenDetails>
                {live && (
                    <Section title={t("settings.tokenValue")} help={t("settings.tokenHowTo")}>
                        <Rows>
                            <CopyField id="sc-token-link" label={t("settings.tokenLink")} help={t("settings.tokenLinkHelp")} value={origin} />
                            <CopyField id="sc-token-value" label={t("settings.tokenValue")} help={t("settings.tokenValueHelp")} value={state.token || ""} />
                        </Rows>
                    </Section>
                )}
                <Section title={t("more")}>
                    <Rows>{qr}</Rows>
                </Section>
                <GuideSection guides={TOKEN_GUIDES} />
            </>
        );
    };

    // L'invitation créée prend toute la page jusqu'à Terminé (MyInvitation).
    return (
        <SettingsPage title={page.title} parent={`${PROFILE}/federation`}>
            {answer !== undefined && state ? <MyInvitation live={live}>{content}</MyInvitation> : content(null)}
        </SettingsPage>
    );
}

// LE CODE D'APPAIRAGE DE MON COMPTE (chantier 60), sous Réglages › Profil › Fédération › Créer mon jeton › « Plus » : mon
// mode compte en tête, puis « Créer le code », grisé tant qu'il est fermé. Le code créé prend la page (PairingReveal) ;
// servi, la page revient à celle de mon jeton.
export function MyPairingPage() {
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
                    <Section title={t("pairing.code")}>
                        <Rows>
                            <NavRow id="tk-pairing-create" icon="bi-123" tone={open ? "indigo" : "slate"} label={t("pairing.create")}
                                help={open ? t("pairing.myCodeRowHelp") : t("pairing.createNeedsMode")} disabled={!open} pending={pending} opens="action" onOpen={create} />
                        </Rows>
                    </Section>
                    <GuideSection guides={TOKEN_GUIDES} />
                </>
            )}
        </SettingsPage>
    );
}
