import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { forgetFederationAccount, pasteFederationToken, typeFederationCode } from "../../libs/federation.js";
import { InvitationField } from "./Invitation.jsx";
import { pairingRefusal } from "./Pairing.jsx";
import { Form, formContext, FormButton, PadReach } from "../../ui/Forms.jsx";
import { ChoiceRow } from "../../bricks/ChoiceRow.jsx";
import { DangerRow } from "../../bricks/DangerRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";

// NOTRE COMPTE CHEZ UN SERVEUR FÉDÉRÉ (shared/FederatedAccountPanel.jsx de la webapp) : le coller
// une fois (son invitation — son lien, ou l'image de son QR — ou son jeton), le voir, l'oublier ; ou
// pourquoi on n'y est pas. En direction C (bricks/) : chacun est UN groupe (Section), son titre compris —
// la page ou le panneau qui le montre le pose tel quel, sans groupe autour.

// Le message d'un refus, avec le nom du serveur ; un code inconnu : « ne répond pas ».
export const refusalText = (t, code, server) => t(`federation.errors.${code === "serverUnreachable" ? "peerUnreachable" : code}`, {
    server: server.name,
    defaultValue: t("federation.errors.peerUnreachable", { server: server.name }),
});

// LE FORMULAIRE (Skeleton) : l'invitation, le code d'appairage (chantier 60) ou le jeton, une fois — le même choix
// segmenté que « Rejoindre un serveur » (ChoiceRow ; la même méthode se choisit d'une seule manière, accepté par le
// porteur le 2026-11-17) ; son bouton dans l'encadré du groupe, après son dernier champ (SubmitRow). Avec le code, il
// n'y a que son champ : le serveur est connu, rien n'est à découvrir. Il sert hors des réglages (l'engrenage d'un jeu) :
// PadReach y mène la manette jusqu'aux contrôles que Skeleton dessine seul.
export function FederatedTokenForm({ server, refused = false }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const [mode, setMode] = useState("invitation");
    const [values, setValues] = useState({ invitation: "", code: "", token: "" });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues);
    const pasted = (values[mode] || "").trim();

    const submit = async () => {
        setPending(true);
        const result = mode === "code" ? await typeFederationCode(server.id, pasted) : await pasteFederationToken(server.id, pasted);
        setPending(false);

        if (result.ok) {
            setValues({ invitation: "", code: "", token: "" });
        } else if (mode === "code") {
            feedback.failed(result.code === "pairingClosed" ? t("pairing.errors.accountPairingClosed", { server: server.name }) : pairingRefusal(t, result.code, server.name));
        } else {
            feedback.failed(refusalText(t, result.code, server));
        }
    };

    return (
        <PadReach>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (pasted && !pending) {
                    submit();
                }
            } }}>
                <Section title={t("federation.tokenTitle", { server: server.name })} help={t(refused ? "federation.tokenRefusedHelp" : "federation.tokenHelp", { server: server.name })} sense="modify">
                    <Rows>
                        <ChoiceRow id="fa-mode" label={t("federation.methodLabel")} help={t("federation.methodHelp", { server: server.name })} value={mode} onChange={setMode}
                            options={[["invitation", t("invitations.tab")], ["code", t("pairing.code")], ["token", t("federation.tokenLabel")]]} />
                    </Rows>
                    {mode === "invitation" ? (
                        <InvitationField id="fa-field" label={t("federation.invitationLabel")} help={t("federation.invitationHelp", { server: server.name })}
                            value={values.invitation} onChange={(invitation) => setValues((previous) => ({ ...previous, invitation }))}>
                            <SubmitRow>
                                <FormButton id="fa-use" label={t("federation.invitationUse")} isReplaced={Boolean(pasted)} pending={pending} />
                            </SubmitRow>
                        </InvitationField>
                    ) : mode === "code" ? (
                        <Rows>
                            <TextRow id="code" label={t("pairing.codeLabel")} help={t("pairing.accountCodeHelp", { server: server.name })} context={form}
                                props={{ "data-key": "fa-field", inputMode: "numeric", autoComplete: "one-time-code", maxLength: 7, placeholder: "000 000" }} />
                            <SubmitRow>
                                <FormButton id="fa-use" label={t("pairing.codeUse")} isReplaced={/^\d{3} ?\d{3}$/.test(pasted)} pending={pending} />
                            </SubmitRow>
                        </Rows>
                    ) : (
                        <Rows>
                            <TextRow id="token" kind="password" label={t("federation.tokenLabel")} help={t("federation.tokenWhere", { server: server.name })} context={form} props={{ "data-key": "fa-field" }} />
                            <SubmitRow>
                                <FormButton id="fa-use" label={t("federation.tokenUse")} isReplaced={Boolean(pasted)} pending={pending} />
                            </SubmitRow>
                        </Rows>
                    )}
                </Section>
            </Form>
        </PadReach>
    );
}

// On y est : sous quel compte — et on peut oublier le jeton. Il faudrait le recoller : c'est un geste qui détruit,
// une ligne rouge qui ouvre la fenêtre de confirmation de Skeleton (DangerRow ; le porteur, 2026-11-13). Elle reste
// DANS ce groupe, pas dans une section « Danger » en bas de la page : le groupe se pose au milieu de pages qui ne
// sont pas les siennes (l'engrenage d'un jeu, les sauvegardes d'un serveur fédéré, ses listes dessous), et la ligne
// n'a de sens qu'à côté du compte qu'elle oublie.
export function FederatedAccount({ server, account }) {
    const { t } = useTranslation();

    return (
        <Section title={t("federation.playingAtTitle", { server: server.name })} help={t("federation.playingAtHelp", { name: account.displayName || account.username, server: server.name })} sense="info">
            <Rows>
                <DangerRow id="fa-forget" icon="bi-key" label={t("federation.tokenForget")} question={t("federation.tokenForgetConfirm", { server: server.name })}
                    onConfirm={() => forgetFederationAccount(server.id)} />
            </Rows>
        </Section>
    );
}

// On n'y est pas, et ce n'est pas une affaire de jeton : pourquoi — l'aide du groupe, plus une alerte (le porteur,
// 2026-11-16 : « retirer les alerts »).
export function FederatedRefusal({ server, code }) {
    const { t } = useTranslation();

    return <Section title={t("federation.refusedTitle", { server: server.name })} help={refusalText(t, code, server)} />;
}
