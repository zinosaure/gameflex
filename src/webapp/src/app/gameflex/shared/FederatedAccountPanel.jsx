import React, { useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../skeleton/skeleton.js";
import { SectionPanel, FormButton } from "./templates/Essentials.jsx";
import { InvitationField } from "./Invitation.jsx";
import { forgetFederationAccount, pasteFederationToken } from "../libs/federationSession.js";

const Form = Skeleton.Form;

// ── Chez celui qui tient la chose ──────────────────────────────────────────
//
// Une PS5 de chez un ami, un jeu de sa bibliothèque : on les VOIT par le lien
// que l'administrateur a enregistré (un jeton de catalogue) ; on y JOUE sous
// SON compte de chez lui. Le formulaire vit dans le panneau de la chose qu'on
// veut lancer, et pas dans un écran de réglages : c'est au moment de vouloir
// jouer qu'on comprend pourquoi on nous le demande.
//
// UN JETON, PLUS UN MOT DE PASSE (chantier 20, le porteur : « au lieu de voir
// l'ancien formulaire login, c'est un formulaire de token […] on met le token
// valide et c'est bon »). Le jeton de son compte chez l'ami se recopie depuis
// Réglages › Profil, chez l'ami ; collé ici une fois, il se garde avec le
// compte d'ici, sur tous ses appareils.
//
// Partagé entre la console (RemotePlay), le jeu (GameDetailPanel) et les
// sauvegardes (Mes sauvegardes) : les trois posent la même question, et une
// seconde copie aurait divergé sur le seul point qui compte — ce qu'on dit
// quand il nous refuse.

/**
 * Pas encore de compte chez lui, ou son jeton ne vaut plus (`refused`) : le
 * coller — ou, depuis le chantier 33, coller son INVITATION (le lien, ou
 * l'image de son QR, créés dans Réglages › Profil chez l'ami) : notre kernel
 * l'échange chez lui contre le jeton, qui ne passe plus par une messagerie.
 */
export function FederatedTokenForm({ server, refused = false, t, context }) {
    const [values, setValues] = useState({ mode: "invitation", invitation: "", token: "" });
    const [isPending, setPending] = useState(false);
    const formContext = {
        values,
        setValues,
        files: {},
        setFiles: () => { },
        rulesets: {},
    };

    const pasted = (values.mode === "invitation" ? values.invitation : values.token).trim();

    const submit = async () => {
        setPending(true);
        try {
            const result = await pasteFederationToken(server.id, pasted);
            if (!result.ok) {
                // Deux échecs, deux gestes : un jeton refusé se recopie chez
                // l'ami, un serveur muet s'allume.
                context.toast.open(t(`common.errorCodes.${result.code === "serverUnreachable" ? "peerUnreachable" : result.code}`, {
                    server: server.name,
                    defaultValue: t("common.errorCodes.peerUnreachable", { server: server.name }),
                }), { variant: "error" });

                return;
            }
            setValues({ mode: values.mode, invitation: "", token: "" });
        } finally {
            setPending(false);
        }
    };

    return (
        <SectionPanel
            label={t("federation.tokenTitle", { server: server.name })}
            helpText={t(refused ? "federation.tokenRefusedHelp" : "federation.tokenHelp", { server: server.name })}>
            <bs.Tabs
                activeKey={values.mode}
                onSelect={(mode) => setValues({ ...values, mode })}
                id={`federated-token-${server.id}`}
                className="nav-tabs-boxed mb-3"
                justify>
                <bs.Tab eventKey="invitation" title={t("invitations.tab")}>
                    <InvitationField
                        formContext={formContext}
                        label={t("invitations.fieldLabel")}
                        helpText={t("invitations.accountFieldHelp", { server: server.name })}
                        t={t}
                        context={context} />
                </bs.Tab>
                <bs.Tab eventKey="token" title={t("federation.tokenLabel")}>
                    <Form.Group label={t("federation.tokenLabel")} helpText={t("federation.tokenWhere", { server: server.name })}>
                        <Form.Password
                            name="token"
                            context={formContext}
                            props={{ autoComplete: "off" }} />
                    </Form.Group>
                </bs.Tab>
            </bs.Tabs>
            <FormButton
                callback={submit}
                label={t(values.mode === "invitation" ? "invitations.use" : "federation.tokenUse")}
                isDisabled={isPending || pasted === ""}
                isPending={isPending}
                variant="primary" />
        </SectionPanel>
    );
}

/**
 * On y joue : sous quel compte — et on peut oublier le jeton. Une partie
 * lancée sur la machine de quelqu'un d'autre, sous un compte de chez lui, ne
 * doit pas pouvoir se confondre avec une partie d'ici.
 */
export function FederatedAccount({ server, account, t }) {
    return (
        <SectionPanel
            label={t("federation.playingAtTitle", { server: server.name })}
            helpText={t("federation.playingAtHelp", {
                name: account.displayName || account.username,
                server: server.name,
            })}>
            <FormButton
                callback={() => forgetFederationAccount(server.id)}
                label={t("federation.tokenForget")}
                variant="danger" />
        </SectionPanel>
    );
}

/** On n'y joue pas, et ce n'est pas notre jeton : pourquoi (`code`). */
export function FederatedRefusal({ server, code, t }) {
    return (
        <SectionPanel
            label={t("federation.refusedTitle", { server: server.name })}
            helpText={t(`common.errorCodes.${code === "serverUnreachable" ? "peerUnreachable" : code}`, {
                server: server.name,
                defaultValue: t("common.errorCodes.peerUnreachable", { server: server.name }),
            })} />
    );
}
