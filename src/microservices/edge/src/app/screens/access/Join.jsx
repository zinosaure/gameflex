import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { checkJoin, joinWithLink } from "../../libs/api.js";
import { useAuth } from "../../libs/data.jsx";
import { focusElement, useScreenInput } from "../../libs/input.js";
import { passwordAccepted, refusalText } from "../../libs/password.js";
import { lastTabPath } from "../../libs/tabs.js";
import { useButton } from "../../libs/ui.js";
import { Form, formContext, FormButton, NewPassword, Ruleset } from "../../ui/Forms.jsx";
import MfaStep from "./MfaStep.jsx";

// LA PAGE DU LIEN D'UN COMPTE (chantier 55, `#694`) — /join/<code>, publique : le porteur, 2026-11-14 : « les nouveaux
// membres vont rejoindre avec un lien d'invitation où ils vont devoir mettre un mot de passe strong pour la première
// fois » ; « le reset password se fait par un administrateur et donc on lui donne un nouveau lien pour rejoindre et
// entrer un nouveau mot de passe ». Elle s'affiche avant la porte de connexion (index.js), connecté ou non.
//
// LA MISE EN PAGE EST CELLE DE LA CONNEXION (Login.jsx, `.login`) : « GameFlex », à qui est le lien (« Bienvenue, Léa »,
// ou « Nouveau mot de passe pour Léa »), son identifiant — un champ caché `autocomplete="username"` le porte, pour que le
// navigateur retienne la paire —, le mot de passe et la jauge de Skeleton (NewPassword : la règle du service des
// comptes), sa confirmation, puis le bouton (« ••• » tant que ce n'est pas valable). Le code part dans le CORPS des deux
// requêtes, jamais dans une adresse de l'API. Le lien accepté, la personne est connectée (`adopt`) et va au dernier
// onglet ; un navigateur connecté à un autre compte passe à celui-ci, et une note le dit avant.
//
// LES ÉTATS : la lecture du lien (le tourniquet) ; un lien mort (410 : servi, échu, remplacé), avec une ligne vers la
// connexion ; trop d'essais (429, et son attente) ; le serveur injoignable.

export default function Join({ code }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const button = useButton();
    const { user, status, adopt } = useAuth();
    const [link, setLink] = useState(undefined);
    const [refusal, setRefusal] = useState("");
    const [values, setValues] = useState({ password: "", confirm: "" });
    const [pending, setPending] = useState(false);
    // Le second temps (chantier 80) : la double authentification imposée, à activer avant d'entrer.
    const [challenge, setChallenge] = useState(null);
    const form = formContext(values, setValues, {
        confirm: (data) => (data.confirm && data.confirm !== data.password ? Ruleset.isInvalid(t("settings.passwordMismatch")) : null),
    });
    const ready = passwordAccepted(values.password) && values.confirm === values.password;

    // Pourquoi un lien ne s'ouvre pas, dit comme à la connexion. `Retry-After` compte une seconde de plus que l'attente
    // (respondLinkError) : 61 s, c'est une minute.
    const reason = (answer) => (answer.status === 0 ? t("login.unreachable")
        : answer.status === 429 ? t("join.throttled", { count: Math.max(1, Math.ceil((answer.retryAfter - 1) / 60)) })
            : answer.status === 410 ? t("join.dead") : refusalText(t, answer) || t("join.failed"));

    useEffect(() => {
        let cancelled = false;
        checkJoin(code).then((answer) => {
            if (!cancelled) {
                setLink(answer.ok && answer.item ? answer.item : { refused: reason(answer) });
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [code]);

    // À la manette : la lumière sur le premier champ, comme à la connexion ; ou sur la ligne vers la connexion.
    useEffect(() => {
        if (link) {
            focusElement(document.querySelector(link.refused ? '[data-key="join-login"]' : '[data-key="join-password"]'), true);
        }
    }, [link]);

    useScreenInput({});

    const submit = async (event) => {
        event.preventDefault();

        if (!ready || pending) {
            return;
        }
        setPending(true);
        setRefusal("");
        const answer = await joinWithLink(code, values.password);
        setPending(false);

        if (answer.ok && answer.item?.access_token) {
            adopt(answer.item);
            navigate(lastTabPath(), { replace: true });

            return;
        }
        // LA DOUBLE AUTHENTIFICATION IMPOSÉE (chantier 80) : le mot de passe est choisi, le lien est consommé ; il reste à
        // l'activer avant d'entrer (MfaStep.jsx). L'abandonner mène à la connexion, avec ce mot de passe.
        if (answer.ok && answer.item?.mfa) {
            setChallenge(answer.item);

            return;
        }

        if (answer.status === 410) {
            setLink({ refused: t("join.dead") });

            return;
        }
        setRefusal(reason(answer));
    };

    if (challenge) {
        return (
            <main className="login">
                <MfaStep challenge={challenge} onSession={(item) => {
                    adopt(item);
                    navigate(lastTabPath(), { replace: true });
                }} onCancel={() => navigate("/", { replace: true })} />
            </main>
        );
    }

    const name = link?.display_name || link?.username || "";
    const reset = link?.kind === "reset";

    return (
        <main className="login">
            <form className="d-flex flex-column gap-3" onSubmit={submit}>
                <h1 className="mb-0 fw-bold">GameFlex</h1>
                {link === undefined && (
                    <p className="text-body-secondary mb-0 d-flex align-items-center gap-2">
                        <span className="spinner-border spinner-border-sm" role="status"></span>{t("join.reading")}
                    </p>
                )}
                {link?.refused && (
                    <>
                        <p className="small text-danger mb-0" role="alert">{link.refused}</p>
                        <button type="button" className={button("main")} data-nav data-key="join-login" onClick={() => navigate("/", { replace: true })}>{t("join.toLogin")}</button>
                    </>
                )}
                {link && !link.refused && (
                    <>
                        <div>
                            <h2 className="h4 mb-1">{t(reset ? "join.resetTitle" : "join.welcome", { name })}</h2>
                            <p className="text-body-secondary mb-0">{t(reset ? "join.resetHelp" : "join.help")}</p>
                        </div>
                        <p className="mb-0">{t("join.username")} <strong>@{link.username}</strong></p>
                        {status === "in" && user && user.username !== link.username && (
                            <p className="small text-body-secondary mb-0">{t("join.signedInAs", { name: user.display_name || user.username, username: link.username })}</p>
                        )}
                        <input type="text" className="visually-hidden" name="username" autoComplete="username" value={link.username} readOnly tabIndex={-1} aria-hidden="true" />
                        <NewPassword name="password" context={form} props={{ "data-nav": true, "data-key": "join-password", autoComplete: "new-password", placeholder: t("join.password"), "aria-label": t("join.password") }} />
                        <Form.Password name="confirm" context={form} props={{ "data-nav": true, "data-key": "join-confirm", autoComplete: "new-password", placeholder: t("join.confirm"), "aria-label": t("join.confirm") }} />
                        {refusal && <p className="small text-danger mb-0" role="alert">{refusal}</p>}
                        <FormButton id="join-submit" label={t(reset ? "join.resetSubmit" : "join.submit")} isReplaced={Boolean(values.password || values.confirm)} pending={pending} disabled={!ready} />
                    </>
                )}
            </form>
        </main>
    );
}
