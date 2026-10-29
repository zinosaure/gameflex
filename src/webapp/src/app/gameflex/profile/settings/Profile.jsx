import React, { useEffect, useState } from "react";
import Skeleton from "../../../skeleton/skeleton.js";
import { myAccountToken, updateMe } from "../api.js";
import {
    SectionPanel,
    FormButton,
} from "../../../gameflex/shared/templates/Essentials.jsx";
import { CopyableValue } from "../../shared/CopyableValue.jsx";
import { useConsoles } from "../../contexts/ConsolesContext.jsx";
import { describeGrants } from "../../libs/consoleGrants.js";
import { useAuth } from "../auth/AuthContext.jsx";
import { MemberToken } from "./admin/Members.jsx";
import { shareOrigin } from "../../libs/serverBase.js";
import { InvitationReveal } from "../../shared/Invitation.jsx";
import { createInvitation, invitationLink } from "../../api/invitations.js";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// Two independent forms/sections (Général / Mot de passe) — each with its
// own local state and its own Save, since they map to different backend
// concerns (display_name+username vs. current_password+new_password, see
// users/routes.py's update_me) and there's no reason changing your name
// should require also re-typing your password. Logout lives in the left
// menu now (Profil section), not here — see index.js.
export default function Profile({ context, user, token, t }) {
    const [values, setValues] = useState({
        username: user?.username || "",
        display_name: user?.display_name || "",
    });
    const [isPending, setPending] = useState(false);
    const contextForm = {
        values,
        setValues: (value) => setValues((prev) => ({ ...prev, ...value })),
        files: {},
        setFiles: () => { },
        rulesets: {
            display_name: (data) => {
                if (!data.display_name || data.display_name.length < 3) {
                    return Ruleset.isInvalid(t("profile.displayNameMinChars", { min: 3 }));
                }

                if (data.display_name.length > 25) {
                    return Ruleset.isInvalid(t("profile.displayNameMaxChars", { max: 25 }));
                }

                return null;
            },
        },
    };
    // "Nothing changed" is a plain comparison, not a ruleset: an untouched
    // field isn't INVALID, and marking it so put a red border and an error
    // tooltip on your own perfectly good name every time the page loaded.
    // Same approach as admin/TurnConfig.jsx and admin/Storage.jsx.
    const isReplaced = values.display_name !== (user?.display_name || "");

    return (
        <>
        <SectionPanel>
            <Form.Group
                label={t("profile.username")}
                helpText={t("profile.usernameHelp")}>
                <Form.Text
                    name="username"
                    context={contextForm}
                    props={{
                        placeholder: t("profile.username"),
                        autoComplete: "username",
                        disabled: true,
                    }} />
            </Form.Group>
            <Form.Group label={t("profile.displayName")} helpText={t("profile.displayNameHelp")}>
                <Form.Text
                    name="display_name"
                    context={contextForm}
                    props={{ placeholder: t("profile.displayName") }} />
            </Form.Group>
            <FormButton
                callback={async (e) => {
                    e.preventDefault();
                    setPending(true);

                    try {
                        const result = await updateMe(token, { display_name: values.display_name });

                        if (result.ok) {
                            context.toast.open(t("profile.saveSuccess"), { variant: "success" });
                        } else {
                            context.toast.open(result.exception?.detail || t("profile.saveFailed"), { variant: "danger" });
                        }
                    } catch (failure) {
                        console.error("échec de l'enregistrement du profil", failure);
                        context.toast.open(t("app.error"), { variant: "danger" });
                    } finally {
                        // Même raison que l'écran de connexion : sans ce
                        // `finally`, un appel qui jette laisse `isPending` à
                        // vrai, et le bouton d'enregistrement reste figé.
                        setPending(false);
                    }
                }}
                label={t("app.save")}
                isDisabled={isPending || !isReplaced || Ruleset.hasFeedbacks(contextForm)}
                isPending={isPending} />
        </SectionPanel>
        <MyToken context={context} user={user} token={token} t={t} />
        </>
    );
}

// yyyy-mm-dd hh:mm, comme la liste des membres.
const formatDateTime = (iso) => {
    if (!iso) return "—";
    const d = new Date(iso);
    const pad = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
};

/**
 * MON QR (chantier 33, le porteur : « Le QR code on peut le récupérer dans
 * settings profile ») — une invitation pour MON jeton : l'autre serveur
 * l'échange contre lui, et il ne passe jamais par une messagerie. À usage
 * unique, 12 heures ; seulement quand le jeton est actif, la règle du jeton.
 * En tête du bloc du jeton, et grisé tant qu'il n'est pas actif : tout en bas,
 * après les gestes de l'administrateur, le porteur ne le trouvait pas.
 */
function MyInvitation({ token, isLive, t, context }) {
    const [invited, setInvited] = useState(null);
    const [isPending, setPending] = useState(false);

    const create = async () => {
        setPending(true);
        try {
            const result = await createInvitation(token, { kind: "account" });
            if (!result.ok) {
                context.toast.open(result.exception?.detail || t("app.error"), { variant: "error" });

                return;
            }
            setInvited(result.item);
        } finally {
            setPending(false);
        }
    };

    if (invited) {
        return (
            <InvitationReveal
                link={invitationLink(invited.code)}
                expiresAt={invited.expires_at}
                title={t("invitations.accountTitle")}
                onDone={() => setInvited(null)}
                t={t}
                context={context} />
        );
    }

    return (
        <div className="mb-3">
            <p className="text-muted small">{t(isLive ? "invitations.accountHelp" : "invitations.myQRNeedsToken")}</p>
            <FormButton callback={create} label={t("invitations.myQR")} isPending={isPending} isDisabled={isPending || !isLive} />
        </div>
    );
}

/**
 * MON JETON (chantier 20) — « pour retrouver son token, le user devra aller
 * dans Settings › Profile, là-bas il pourra copier le token et lien si ce
 * dernier est enabled. Il aura aussi les infos comme le nombre de fois c'est
 * connecté, la dernière utilisation, les accès avec les consoles » (le
 * porteur). Le jeton et le lien se collent dans Réglages › Fédérations, sur
 * son propre serveur. Un administrateur d'ici l'active — et le sien, il le gère
 * ici, avec les gestes de la fiche d'un membre : « Membres » ne le montre pas.
 */
function MyToken({ context, user, token, t }) {
    const consoles = useConsoles();
    const { isAdmin } = useAuth();
    const [mine, setMine] = useState(null);

    useEffect(() => {
        let cancelled = false;
        myAccountToken(token).then((result) => {
            if (!cancelled && result.ok) {
                setMine(result.item);
            }
        }).catch((failure) => {
            // Pas de toast : un écran de réglages qui s'ouvre ne doit pas
            // crier pour un bloc qui n'est qu'informatif.
            console.error("échec du chargement de mon jeton", failure);
        });

        return () => {
            cancelled = true;
        };
    }, [token]);

    if (!mine?.token) {
        return null;
    }
    const consolesLine = (
        <div className="small">
            <span className="fw-semibold">{t("profile.tokenConsoles")}</span>
            <span className="text-muted"> {describeGrants(mine.consoles, consoles, t)}</span>
        </div>
    );
    const state = mine.token;
    const isLive = state.enabled && state.expires_at && new Date(state.expires_at) > new Date();
    if (isAdmin && user?.id) {
        return (
            <>
                <MemberToken
                    authToken={token}
                    member={{ id: user.id, display_name: user.display_name, token: mine.token }}
                    onChanged={(changed) => setMine((current) => ({ ...current, token: changed.token }))}
                    header={<MyInvitation token={token} isLive={isLive} t={t} context={context} />}
                    t={t}
                    context={context} />
                {consolesLine}
            </>
        );
    }

    const status = !state.enabled
        ? t("profile.tokenDisabled")
        : t(isLive ? "profile.tokenEnabledUntil" : "profile.tokenExpired", { at: formatDateTime(state.expires_at) });
    const usage = state.last_used_at
        ? t("admin.members.tokenUsage", { count: state.use_count, at: formatDateTime(state.last_used_at) })
        : t("admin.members.tokenNeverUsed");


    return (
        <SectionPanel label={t("profile.tokenTitle")} helpText={`${status} · ${usage}`}>
            <MyInvitation token={token} isLive={isLive} t={t} context={context} />
            {isLive
                ? (
                    <>
                        <p className="text-muted small">{t("profile.tokenHowTo")}</p>
                        <CopyableValue label={t("admin.members.tokenLink")} value={shareOrigin()} t={t} context={context} />
                        <CopyableValue label={t("admin.members.tokenValue")} value={state.token || ""} t={t} context={context} />
                    </>
                )
                : <p className="text-muted small">{t("profile.tokenAskAdmin")}</p>}
            {consolesLine}
        </SectionPanel>
    );
}