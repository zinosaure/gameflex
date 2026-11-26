// Route : /desktop/join — REJOINDRE UN SERVEUR, COMME L'APK (JoinPage) : son adresse, vérifiée avant de s'ouvrir
// (join.go), puis les serveurs rejoints. Rejoint, il s'ouvre dans la fenêtre, sous son compte de là-bas ; « Accueil de
// l'application » ramène ici. Ni lecteur de QR code : un PC n'en a pas — l'adresse se colle.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { EmptyRow } from "../../bricks/EmptyRow.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { Form, formContext, FormButton } from "../../ui/Forms.jsx";
import { DesktopPage, useDesktopState, useJoin } from "./shared.jsx";

export default function Join() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { state } = useDesktopState();
    const join = useJoin();
    const [values, setValues] = useState({ link: "" });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues);
    const joined = state.joined || [];

    return (
        <DesktopPage title={t("desktop.join")} guides={[["remoteAccess"]]}>
            <Form props={{ onSubmit: async (event) => {
                event.preventDefault();

                if (!values.link.trim() || pending) {
                    return;
                }
                setPending(true);
                await join(values.link.trim(), true);
                setPending(false);
            } }}>
                <Section title={t("desktop.joinSection")} help={t("desktop.joinSectionHelp")} sense="add">
                    <Rows>
                        <TextRow id="link" label={t("desktop.joinLink")} help={t("desktop.joinLinkHelp")} context={form}
                            props={{ inputMode: "url", autoComplete: "off", placeholder: "http://192.168.1.50:10200", "data-key": "dc-join-link" }} />
                        <SubmitRow>
                            <FormButton id="dc-join-submit" label={t("desktop.joinOpen")} isReplaced={Boolean(values.link.trim())} pending={pending} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
            <Section title={t("desktop.joined")} help={t("desktop.joinedHelp")} sense="play">
                <Rows>
                    {joined.length === 0 ? <EmptyRow icon="bi-hdd-network" text={t("desktop.joinedNone")} />
                        : joined.map((server) => (
                            <NavRow key={server.origin} id={`dc-joined-${server.origin}`} icon="bi-hdd-network" label={server.name} value={server.origin}
                                onOpen={() => navigate(`/desktop/join/${encodeURIComponent(server.origin)}`)} />
                        ))}
                </Rows>
            </Section>
        </DesktopPage>
    );
}
