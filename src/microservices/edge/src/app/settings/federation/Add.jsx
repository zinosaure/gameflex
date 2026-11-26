// Route : /settings/federation/add — rejoindre un serveur.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChoiceRow } from "../../bricks/ChoiceRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { addServerRegistry, checkFederationServer, redeemServerCode, redeemServerInvitation, rescanServer } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { InvitationField } from "../../screens/access/Invitation.jsx";
import { LocalServersSection, pairingRefusal } from "../../screens/access/Pairing.jsx";
import { Form, FormButton, formContext } from "../../ui/Forms.jsx";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../profile/Federation.jsx";
import { JOINED, useLevels } from "../Federation.jsx";

// REJOINDRE UN SERVEUR : son invitation (et le nom sous lequel il apparaîtra), son code d'appairage (chantier 60 : les
// serveurs du réseau local en mode découverte, et l'adresse, toujours tapable), ou son lien et son jeton — le choix de
// la méthode est un choix segmenté (ChoiceRow), plus des onglets.
export default function AddServer() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const levels = useLevels();
    const { refreshServers } = useData();
    const { back } = useSettingsNavigation();
    const [values, setValues] = useState({ mode: "invitation", invitation: "", name: "", url: "", token: "", address: "", code: "" });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues);
    const set = (patch) => setValues((previous) => ({ ...previous, ...patch }));

    // Le serveur désigné : l'invitation échangée chez lui, ou le lien et le jeton vérifiés.
    const serverOf = async () => {
        if (values.mode === "invitation") {
            const redeemed = await redeemServerInvitation(values.invitation);

            if (!redeemed.ok) {
                feedback.failed(t(`invitations.errors.${redeemed.errorCode || "serverUnreachable"}`, { server: "", defaultValue: redeemed.errorMessage || t("fed.unreachable") }));

                return null;
            }

            return { link: redeemed.data.link, serverToken: redeemed.data.token, seen: redeemed.data };
        }

        if (values.mode === "code") {
            const redeemed = await redeemServerCode(values.address, values.code);

            if (!redeemed.ok) {
                feedback.failed(pairingRefusal(t, redeemed.errorCode));

                return null;
            }

            return { link: redeemed.data.link, serverToken: redeemed.data.token, seen: redeemed.data };
        }
        const probe = await checkFederationServer(values.url.trim(), values.token.trim());

        if (!probe.ok) {
            feedback.failed(probe.errorMessage || t("fed.unreachable"));

            return null;
        }

        return { link: values.url.trim(), serverToken: values.token.trim(), seen: probe.data || {} };
    };

    const save = async () => {
        setPending(true);
        const found = await serverOf();

        if (!found) {
            setPending(false);

            return;
        }
        const answer = await addServerRegistry({ link: found.link, serverToken: found.serverToken, hostname: (values.name || "").trim() || found.seen.hostname || found.link, identity: found.seen.identity || "" });

        if (!answer.ok) {
            setPending(false);
            feedback.failed(answer.detail || t("settings.actionFailed"));

            return;
        }

        if (answer.item?.id) {
            await rescanServer(answer.item.id);
        }
        await refreshServers();
        setPending(false);
        feedback.success(t("fed.added"));
        back(JOINED);
    };

    const field = (name, label, help, placeholder) => (
        <TextRow id={name} context={form} label={label} help={help} props={{ placeholder, "data-key": `fd-${name}` }} />
    );
    const typed = { invitation: ["invitation", "name"], code: ["address", "code", "name"], token: ["name", "url", "token"] }[values.mode];
    const ready = values.mode === "invitation" ? (values.invitation || "").trim()
        : values.mode === "code" ? (values.address || "").trim() && /^\d{3} ?\d{3}$/.test((values.code || "").trim())
            : (values.url || "").trim() && (values.token || "").trim();
    // Rien de tapé dans le mode choisi, rien de changé (CODE_CONVENTIONS.md, `isReplaced`).
    const isReplaced = typed.some((name) => (values[name] || "").trim());

    // Le bouton du formulaire, dans l'encadré de son dernier groupe (le porteur, 2026-11-13 : « le bouton doit être
    // dans la section card avec les autres form controls »).
    const submit = (
        <SubmitRow>
            <FormButton id="fd-save" label={t("settings.save")} isReplaced={isReplaced} pending={pending} disabled={!ready} />
        </SubmitRow>
    );

    // La méthode, puis l'invitation (son champ, Invitation.jsx) et le nom ; ou le nom, le lien et le jeton.
    return (
        <SettingsPage title={t("fed.join")} parent={JOINED} levels={levels} guides={FEDERATION_GUIDES}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (ready && !pending) {
                    save();
                }
            } }}>
                <Section title={t("fed.joinMethodTitle")} help={t("fed.joinMethodHelp")} sense="modify">
                    <Rows>
                        <ChoiceRow id="fd-mode" label={t("fed.joinMethod")} help={t("fed.joinMethodHelp")} value={values.mode} onChange={(mode) => set({ mode })}
                            options={[["invitation", t("invitations.tab")], ["code", t("pairing.code")], ["token", t("invitations.tokenTab")]]} />
                    </Rows>
                </Section>
                {values.mode === "code" ? (
                    <>
                        {/* La liste du sujet `discovery`, le temps où cette option est choisie : la page ne s'y abonne qu'ici. */}
                        <LocalServersSection onChoose={(server) => set({ address: server.link, name: values.name || server.name })} />
                        <Section title={t("fed.serverTitle")} help={t("fed.serverCheckHelp")} sense="info">
                            <Rows>
                                {field("address", t("pairing.urlLabel"), t("pairing.urlHelp"), "http://192.168.1.10:10200")}
                                <TextRow id="code" context={form} label={t("pairing.codeLabel")} help={t("pairing.codeHelp")}
                                    props={{ placeholder: "000 000", "data-key": "fd-code", inputMode: "numeric", autoComplete: "one-time-code", maxLength: 7 }} />
                                {field("name", t("fed.nameLabel"), t("invitations.nameHelp"), t("fed.namePlaceholder"))}
                                {submit}
                            </Rows>
                        </Section>
                    </>
                ) : values.mode === "invitation" ? (
                    <>
                        <Section title={t("invitations.detailTitle")} help={t("fed.invitationDetailHelp")} sense="info">
                            <InvitationField id="fd-invitation" label={t("invitations.fieldLabel")} help={t("invitations.serverFieldHelp")} value={values.invitation} onChange={(invitation) => set({ invitation })} />
                        </Section>
                        <Section title={t("fed.serverTitle")} help={t("fed.serverCheckHelp")} sense="info">
                            <Rows>
                                {field("name", t("fed.nameLabel"), t("invitations.nameHelp"), t("fed.namePlaceholder"))}
                                {submit}
                            </Rows>
                        </Section>
                    </>
                ) : (
                    <Section title={t("fed.serverTitle")} help={t("fed.serverCheckHelp")} sense="info">
                        <Rows>
                            {field("name", t("fed.nameLabel"), t("fed.nameHelp"), t("fed.namePlaceholder"))}
                            {field("url", t("fed.urlLabel"), t("fed.gameflexUrlHelp"), "https://gameflex.com")}
                            {field("token", t("fed.tokenLabel"), t("fed.tokenHelp"), t("fed.tokenLabel"))}
                            {submit}
                        </Rows>
                    </Section>
                )}
            </Form>
        </SettingsPage>
    );
}
