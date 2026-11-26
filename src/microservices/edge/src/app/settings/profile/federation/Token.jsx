// Route : /settings/profile/federation/token — « Créer mon jeton » (Réglages › Profil › Fédération) : son détail, l'activer
// pour une durée, le lien et le jeton à copier, les alternatives (le QR, le code d'appairage), puis « Danger ».
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { fetchMyToken, regenerateMyToken, updateMyToken } from "../../../libs/api.js";
import { useAuth } from "../../../libs/data.jsx";
import { MyInvitation, useShareOrigin } from "../../../screens/access/Invitation.jsx";
import { toast } from "../../../shell/Shell.jsx";
import SettingsPage, { PROFILE, usePageText, useSettingsNavigation } from "../../SettingsPage.jsx";
import { CopyField } from "../../../bricks/CopyField.jsx";
import { DangerRow } from "../../../bricks/DangerRow.jsx";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Note } from "../../../bricks/Note.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { SelectRow } from "../../../bricks/SelectRow.jsx";
import { SubmitRow } from "../../../bricks/SubmitRow.jsx";
import { FormButton } from "../../../ui/Forms.jsx";
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import { counted, dateTime } from "../../../libs/format.js";

const TOKEN_DURATIONS = ["1d", "1w", "1m", "3m", "6m", "1y", "2y"];

// LE GUIDE D'UN JETON : il sert à jouer d'un autre serveur, sous ce compte (docs/federation.md).
export const TOKEN_GUIDES = [["federation", "play-on-a-friends-server"]];

// L'ÉTAT D'UN JETON : actif et pas échu.
const isLive = (token) => Boolean(token.enabled && token.expires_at && new Date(token.expires_at) > new Date());

// LE DÉTAIL D'UN JETON (le porteur, 2026-11-14 : « quand on extend un token et si on refresh la page, on ne sait plus la
// durée car le selectbox montre 1 mois par défaut ») : son état, SA VRAIE ÉCHÉANCE — celle que le serveur a enregistrée
// (`expires_at`), relue à chaque ouverture —, ses sessions et son dernier usage. Le choix d'une durée, lui, ne dit plus
// rien tant qu'on n'en a pas choisi une (OwnToken). `children` : ce qui suit ses lignes ; `help` : l'aide du groupe.
export function TokenDetails({ token, help = null, children = null }) {
    const { t, i18n } = useTranslation();
    const live = isLive(token);

    return (
        <Section title={t("details")} help={help || t("members.tokenDetailsHelp")} sense="info">
            <Rows>
                <InfoRow icon="bi-key" label={t("members.tokenState")}
                    value={t(!token.enabled ? "members.tokenDisabled" : live ? "members.tokenLive" : "members.tokenOver")} />
                {token.enabled && <InfoRow icon="bi-calendar-event" label={t(live ? "members.tokenExpiresAt" : "members.tokenExpiredAt")} value={token.expires_at ? dateTime(token.expires_at, i18n.language) : null} />}
                <InfoRow icon="bi-box-arrow-in-right" label={t("members.tokenSessions")} value={counted(t, "sessions", token.use_count || 0)} />
                <InfoRow icon="bi-clock-history" label={t("members.tokenLastUsed")} value={token.last_used_at ? dateTime(token.last_used_at, i18n.language) : t("members.tokenNever")} />
            </Rows>
            {children}
        </Section>
    );
}

// MON JETON (OwnToken ; le jeton d'un membre ne se gère plus depuis sa page, le porteur, 2026-11-20) : son détail (TokenDetails) ; l'activer (ou le prolonger) pour la durée choisie — AVANT le jeton
// (le porteur, 2026-11-18 : « mettre la section extend avant la section token ») ; actif, le lien et le jeton à copier — AUCUNE n'est choisie d'avance (« ••• » tant qu'on n'en a pas pris une), le bouton
// dans l'encadré de la durée ; `more` : les lignes de « Plus » (« Créer mon QR » dans Réglages › Jeton) ; puis
// « Danger » — le désactiver (le porteur, 2026-11-14 : « "disable my token" dans danger avec une confirmation »), le
// régénérer (l'ancien cesse de valoir). Avant « Danger », le guide : jouer chez un autre serveur avec ce jeton (TOKEN_GUIDES).
export function OwnToken({ member, onChanged, more = null }) {
    const { t } = useTranslation();
    const origin = useShareOrigin();
    const [state, setState] = useState(member.token || null);
    const [duration, setDuration] = useState("");
    const [pending, setPending] = useState(false);
    const feedback = useFeedback();
    const disabled = false;
    const busy = pending || disabled;

    if (!state) {
        return null;
    }

    // `viaForm` : le geste de son FormButton, qui répond par la fenêtre de Skeleton (useFeedback) ; les autres, un toast.
    const apply = async (call, done, viaForm = false) => {
        setPending(true);
        const answer = await call();
        setPending(false);

        if (!answer.ok) {
            (viaForm ? feedback.failed : toast)(answer.detail || t("settings.actionFailed"));

            return;
        }
        setState(answer.item);
        setDuration("");
        onChanged?.({ ...member, token: answer.item });
        (viaForm ? feedback.success : toast)(done);
    };

    const enable = state.enabled ? "extend" : "enable";

    return (
        <>
            <TokenDetails token={state} />
            <Section title={t(state.enabled ? "members.tokenExtend" : "members.tokenEnable")} help={t(state.enabled ? "members.tokenExtendHelp" : "members.tokenEnableHelp")} sense="modify">
                <Rows>
                    <SelectRow id="mt-duration" label={t("members.tokenDuration")} help={t("members.tokenDurationHelp")} value={duration} onChange={(value) => setDuration(value || "")} disabled={disabled}
                        options={[{ value: "", label: t("members.tokenDurationPick") }, ...TOKEN_DURATIONS.map((value) => ({ value, label: t(`members.tokenDurations.${value}`) }))]} />
                    <SubmitRow>
                        <FormButton id={`mt-${enable}`} type="button" label={t(state.enabled ? "members.tokenExtend" : "members.tokenEnable")} isReplaced={duration !== ""} pending={pending} disabled={disabled}
                            onClick={() => apply(() => updateMyToken({ enabled: true, duration }), t("members.tokenEnabledToast"), true)} />
                    </SubmitRow>
                </Rows>
            </Section>
            {isLive(state) && (
                <Section title={t("members.tokenValue")} help={t("members.tokenValueSectionHelp")} sense="play">
                    <Rows>
                        <CopyField id="mt-link" label={t("members.tokenLink")} help={t("members.tokenLinkHelp")} value={origin} />
                        <CopyField id="mt-token" label={t("members.tokenValue")} help={t("members.tokenValueHelp")} value={state.token || ""} />
                    </Rows>
                </Section>
            )}
            {more && (
                <Section title={t("settings.tokenAlternative")} help={t("settings.tokenAlternativeHelp")} sense="optional">
                    <Rows>{more}</Rows>
                </Section>
            )}
            <DangerSection>
                {state.enabled && (
                    <DangerRow id="mt-disable" icon="bi-toggle-off" label={t("members.tokenDisable")} help={t("members.tokenDisableHelp")} disabled={busy}
                        question={t("members.tokenDisableConfirm")}
                        onConfirm={() => apply(() => updateMyToken({ enabled: false }), t("members.tokenDisabledToast"))} />
                )}
                <DangerRow id="mt-regenerate" icon="bi-arrow-repeat" label={t("members.tokenRegenerate")} help={t("members.tokenRegenerateHelp")} disabled={busy}
                    question={t("members.tokenRegenerateConfirm")}
                    onConfirm={() => apply(() => regenerateMyToken(), t("members.tokenRegeneratedToast"))} />
            </DangerSection>
            <GuideSection guides={TOKEN_GUIDES} />
        </>
    );
}

// MON JETON — « Créer mon jeton », sous Réglages › Profil › Fédération (le porteur, 2026-11-16, 2.1 ; MyFederation.jsx)
// (chantier 20, Profile.jsx de la webapp) : chacun l'active lui-même pour une
// durée (2026-11-20) ; actif, il se copie avec le lien de ce serveur, à coller sur son propre serveur. Son
// détail d'abord — sa vraie échéance, lue sur le serveur (TokenDetails ; le porteur, 2026-11-14) —,
// puis le lien et le jeton, puis « Alternative » (le porteur, 2026-11-18 : le QR et le code d'appairage sont des
// alternatives au jeton) : « Utiliser un QR code à la place » (MyInvitation ; « "create my QR" doit
// passer dans more ») et « Code d'appairage » (chantier 60, Pairing.jsx), grisés tant que le jeton n'est pas actif. Chacun gère son propre jeton (OwnToken) : sa
// durée, puis « Danger » — le désactiver (« "disable my token" dans danger avec une confirmation »), le régénérer.
export default function Token() {
    const { t } = useTranslation();
    const page = usePageText("token");
    const origin = useShareOrigin();
    const { user } = useAuth();
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
                <NavRow id="tk-pairing" icon="bi-123" label={t("pairing.useInstead")} help={live ? t("pairing.myCodeRowHelp") : t("invitations.myQRNeedsToken")}
                    disabled={!live} onOpen={() => open(`${PROFILE}/federation/pairing`, "tk-pairing")} />
            </>
        );

        // Le temps de la lecture, un mot seul, sans titre : rien encore n'en mérite un.
        if (answer === undefined) {
            return <Section><Loading /></Section>;
        }

        if (!state) {
            return <Section title={t("settings.tokenValue")} sense="modify"><Note className="mt-0">{t("settings.tokenNone")}</Note></Section>;
        }

        // CHACUN GÈRE SON JETON (le porteur, 2026-11-20 : « Le membre l'active lui-même » ; « L'admin peut contrôler les
        // accès aux consoles. C'est pour cette raison que ça ne sert à rien l'accès au token ») : l'activer pour une
        // durée, le prolonger, le désactiver, le régénérer — administrateur ou non.
        return (
            <OwnToken member={{ id: user?.id, username: user?.username, display_name: user?.display_name, token: state }} more={qr}
                onChanged={(changed) => setAnswer((current) => ({ ...current, token: changed.token }))} />
        );
    };

    // L'invitation créée prend toute la page jusqu'à Terminé (MyInvitation).
    return (
        <SettingsPage title={page.title} parent={`${PROFILE}/federation`}>
            {answer !== undefined && state ? <MyInvitation live={live}>{content}</MyInvitation> : content(null)}
        </SettingsPage>
    );
}
