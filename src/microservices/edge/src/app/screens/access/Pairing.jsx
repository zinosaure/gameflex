import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { closePairing, openPairing, readToken } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { kernelReachable, kernelTarget, useKernelChannel, useKernelTopic } from "../../libs/kernelChannel.js";
import { useShareOrigin } from "./Invitation.jsx";
import { toast } from "../../shell/Shell.jsx";
import { EmptyRow } from "../../bricks/EmptyRow.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Note } from "../../bricks/Note.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SwitchRow } from "../../bricks/SwitchRow.jsx";
import { SaveSection } from "../../bricks/SaveSection.jsx";
import { fact } from "../../libs/format.js";

// LE CODE D'APPAIRAGE (chantier 60 ; le porteur, 2026-11-17 : « Comme ça à côté de la création du token on a "code
// appairage" et dans rejoins un serveur et s'authentifier on aura entrér le code d'appairage ») : six chiffres, affichés
// par un serveur et tapés dans l'autre, valables deux minutes, une fois. Ils ne s'acceptent que pendant un MODE
// DÉCOUVERTE de cinq minutes : le mode serveur (l'administrateur, le code qui crée le lien entre deux serveurs) ou le
// mode compte (chacun pour le sien, le code qui sert à s'authentifier).
//
// PAS DE SONDAGE (CODE_CONVENTIONS.md) : l'état des modes et des codes arrive par le sujet `pairing` du kernel ; la
// fenêtre d'un code se ferme quand le sujet dit qu'il a servi, expiré ou été annulé. Le compte à rebours affiché est une
// horloge : il ne décide de rien. Les serveurs du réseau local arrivent par le sujet `discovery`, auquel la page ne
// s'abonne que le temps où elle les montre ; « Chercher de nouveau » est un geste.

const timeOf = (iso, language) => (iso ? new Date(iso).toLocaleTimeString(language === "fr" ? "fr-FR" : "en-GB", { hour: "2-digit", minute: "2-digit" }) : "");

// Les six chiffres en deux groupes : « 482 913 ».
export const groupedCode = (code) => `${String(code).slice(0, 3)} ${String(code).slice(3)}`;

// Le canal de CE kernel, tant que le composant vit, ou null.
function useOwnChannel(enabled = true) {
    const token = readToken();

    return useKernelChannel(enabled && kernelReachable(token) ? kernelTarget(null, token) : null);
}

// LE SUJET `pairing` : `{server, account, codes, ended, closed}` (la part de ce compte), ou null le temps qu'il arrive.
export function usePairing() {
    const channel = useOwnChannel();
    const [state, setState] = useState(null);

    useKernelTopic(channel, "pairing", null, (data) => setState(data || null));

    return state;
}

// LE SUJET `discovery` : les serveurs du réseau local en mode découverte, tant que `enabled` ; `search` repose la
// question une fois.
export function useDiscovery(enabled) {
    const channel = useOwnChannel(enabled);
    const [servers, setServers] = useState(null);

    useEffect(() => {
        if (!enabled) {
            setServers(null);
        }
    }, [enabled]);
    useKernelTopic(channel, enabled ? "discovery" : null, null, (data) => setServers(data?.servers || []));

    return { servers, search: () => channel?.request("discovery.search") };
}

// Le mode d'une sorte dans le sujet : ouvert jusqu'à quand, ou la dernière fermeture.
const modeOf = (pairing, scope) => (scope === "server" ? pairing?.server : pairing?.account) || null;
const closureOf = (pairing, scope) => [...(pairing?.closed || [])].reverse().find((entry) => entry.scope === scope) || null;

// LA SECTION « MODE DÉCOUVERTE », une seule pour les deux modes (`scope` : "server" ou "account") : l'interrupteur, son
// aide qui suit le sujet, enregistré par le bouton de la section (le porteur, 2026-11-25 : « Faudra rajouter des boutons
// pour tous les formulaires qui n'en ont pas ») — le bouton tient lieu de la confirmation d'avant —, la réponse en
// fenêtre (SaveSection) ; puis « Prolonger de 5 minutes ». Le sujet le referme seul : l'interrupteur le suit.
// `children` : des lignes de plus, actives tant que le mode est ouvert — sur l'accueil de la fédération, les deux
// formulaires qui acceptent une connexion (le porteur, 2026-11-18 : « Comme ça on va directement sur les formulaires
// pour accepter une connexion »).
export function DiscoveryModeSection({ scope, pairing, children }) {
    const { t, i18n } = useTranslation();
    const [pending, setPending] = useState(false);
    const mode = modeOf(pairing, scope);
    const closure = closureOf(pairing, scope);

    const open = async () => {
        setPending(true);
        const answer = await openPairing(scope);
        setPending(false);

        if (!answer.ok) {
            toast(answer.detail || t("settings.actionFailed"));
        }
    };
    const saveMode = async ({ on }) => {
        const answer = on ? await openPairing(scope) : await closePairing(scope);

        return answer.status === 204 ? { ok: true } : answer;
    };

    let help = t(scope === "server" ? "pairing.serverModeHelp" : "pairing.accountModeHelp");

    if (mode) {
        help = t("pairing.openUntil", { time: timeOf(mode.open_until, i18n.language) });
    } else if (closure && closure.reason !== "owner") {
        help = t(closure.reason === "too_many_failures" ? "pairing.closedFailures" : "pairing.closedTimeout");
    }

    return (
        <SaveSection id={`pr-${scope}`} title={t("pairing.modeTitle")} help={t("pairing.modeHelp")} saved={{ on: Boolean(mode) }} disabled={!pairing}
            onSave={saveMode} success={({ on }) => t(on ? "pairing.opened" : "pairing.closed")}
            after={(
                <Rows className="mt-3">
                    {/* JAMAIS CACHÉES, DÉSACTIVÉES (le porteur, 2026-11-25 : « le discovery mode ne respecte pas la règle "pas de
                        hide/show". Il faut que les switches soient en enabled/disabled ») : fermé, ses lignes restent là, leur
                        raison à la place de leur aide. */}
                    <NavRow id={`pr-${scope}-extend`} icon="bi-clock-history" label={t("pairing.extend")} help={mode ? t("pairing.extendHelp") : t("pairing.needsMode")}
                        pending={pending} disabled={!mode} opens="action" onOpen={open} />
                    {mode ? children : React.Children.map(children, (child) => (React.isValidElement(child) ? React.cloneElement(child, { disabled: true, help: t("pairing.needsMode") }) : child))}
                </Rows>
            )}>
            {(values, set) => (
                <SwitchRow id={`pr-${scope}-mode`} label={t("pairing.modeSwitch")} help={help} checked={values.on} disabled={!pairing} onChange={(on) => set({ on })} />
            )}
        </SaveSection>
    );
}

// Ce qu'un code est devenu, dit à la fermeture de sa fenêtre.
const endedText = (t, ended) => {
    switch (ended?.reason) {
    case "used":
        return t("pairing.endedUsed", { name: ended.by || "—", address: ended.from || "—" });
    case "cancelled":
        return t("pairing.endedCancelled");
    case "too_many_failures":
        return t("pairing.endedFailures");
    default:
        return t("pairing.endedExpired");
    }
};

// Le temps qui reste, « 1 min 40 s ».
const leftText = (t, milliseconds) => {
    const seconds = Math.max(0, Math.ceil(milliseconds / 1000));

    return seconds >= 60
        ? t("pairing.leftMinutes", { minutes: Math.floor(seconds / 60), seconds: seconds % 60 })
        : t("pairing.leftSeconds", { seconds });
};

// LA FENÊTRE DU CODE (comme InvitationReveal) : ses six chiffres en deux groupes, le temps qui reste, ce serveur (son
// nom, celui que voient ses serveurs fédérés, et son adresse : l'autre côté vérifie qu'il vise le bon) ; puis
// « Plus » › « Annuler le code ». Elle se ferme d'elle-même quand le sujet dit que le code a servi, expiré ou été
// annulé — `onDone(texte)` dit pourquoi.
export function PairingReveal({ created, pairing, onCancel, onDone }) {
    const { t } = useTranslation();
    const origin = useShareOrigin();
    const serverName = useData().capabilities?.serverName || "";
    const [now, setNow] = useState(Date.now());
    const seen = useRef(false);
    const finished = useRef(false);

    // L'horloge affichée, et rien d'autre : seule l'échéance poussée par le serveur ferme la fenêtre.
    useEffect(() => {
        const clock = setInterval(() => setNow(Date.now()), 1000);

        return () => clearInterval(clock);
    }, []);

    useEffect(() => {
        if (!pairing || finished.current) {
            return;
        }
        const ended = (pairing.ended || []).find((entry) => entry.id === created.id);
        const alive = (pairing.codes || []).some((entry) => entry.id === created.id);
        seen.current = seen.current || alive;

        if (ended || (seen.current && !alive)) {
            finished.current = true;
            onDone(endedText(t, ended));
        }
    }, [pairing, created.id, onDone, t]);

    return (
        <>
            <Section title={t("pairing.code")} sense="modify">
                <p className="display-4 font-monospace text-center mb-0" data-key="pr-code" aria-label={t("pairing.code")}>{groupedCode(created.code)}</p>
                <Note>{t("pairing.validFor", { left: leftText(t, Date.parse(created.expires_at) - now) })}</Note>
            </Section>
            <Section title={t("fed.serverTitle")} sense="info">
                <Rows>
                    <InfoRow icon="bi-hdd-network" label={serverName || t("fed.urlLabel")} value={serverName ? fact(t("fed.urlLabel"), origin) : origin} help={t("pairing.thisServerHelp")} />
                </Rows>
            </Section>
            <Section title={t("more")} sense="modify">
                <Rows>
                    <NavRow id="pr-cancel" icon="bi-x-circle" label={t("pairing.cancel")} help={t("pairing.cancelHelp")} opens="action" onOpen={onCancel} />
                </Rows>
            </Section>
        </>
    );
}

// LES SERVEURS DU RÉSEAU LOCAL : une ligne chacun, son nom et son adresse ; la choisir remplit l'adresse. Un nom vu à
// deux adresses se signale (un autre appareil peut se faire passer pour un serveur). La liste vide garde sa boîte, et
// « Chercher de nouveau » repose la question.
export function LocalServersSection({ onChoose }) {
    const { t } = useTranslation();
    const { servers, search } = useDiscovery(true);
    const counted = {};

    (servers || []).forEach((server) => {
        counted[server.name] = (counted[server.name] || 0) + 1;
    });

    return (
        <Section title={t("pairing.localTitle")} help={t("pairing.localHelp")} sense="modify">
            <Rows>
                {(servers || []).map((server) => (
                    <NavRow key={`${server.instance}-${server.address}`} id={`pr-found-${server.address}-${server.instance}`} icon="bi-hdd-network"
                        label={server.name} help={counted[server.name] > 1 ? [fact(t("fed.urlLabel"), server.address), t("pairing.sameName")] : fact(t("fed.urlLabel"), server.link)}
                        opens="action" onOpen={() => onChoose(server)} />
                ))}
                {servers && servers.length === 0 && <EmptyRow icon="bi-wifi-off" text={t("pairing.localEmpty")} />}
                {servers === null && <EmptyRow icon="bi-wifi" text={t("pairing.localSearching")} />}
                <NavRow id="pr-search" icon="bi-arrow-repeat" label={t("pairing.searchAgain")} help={t("pairing.searchAgainHelp")} opens="action"
                    disabled={servers === null} onOpen={() => search()} />
            </Rows>
        </Section>
    );
}

// Le refus d'un code, par son code : ceux du code d'appairage d'abord, puis ceux des invitations.
export const pairingRefusal = (t, code, server = "") => t(`pairing.errors.${code}`, {
    server,
    defaultValue: t(`invitations.errors.${code || "serverUnreachable"}`, { server, defaultValue: t("fed.unreachable") }),
});
