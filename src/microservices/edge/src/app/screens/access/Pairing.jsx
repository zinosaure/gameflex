import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { readToken } from "../../libs/api.js";
import { kernelReachable, kernelTarget, useKernelChannel, useKernelTopic } from "../../libs/kernelChannel.js";
import { useFederationHost } from "../../settings/people/federationHost.js";
import { toast } from "../../shell/Shell.jsx";
import NavRow, { EmptyRow, InfoRow, Note, Rows, Section, SwitchRow, useConfirm } from "../../ui/Bricks.jsx";

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
// aide qui suit le sujet, et « Prolonger de 5 minutes » une fois ouvert. L'allumer passe par la fenêtre de Skeleton ;
// l'éteindre ne demande rien, on n'y perd rien.
export function DiscoveryModeSection({ scope, pairing }) {
    const { t, i18n } = useTranslation();
    const confirm = useConfirm();
    const { api } = useFederationHost();
    const [pending, setPending] = useState(false);
    const mode = modeOf(pairing, scope);
    const closure = closureOf(pairing, scope);

    const open = async () => {
        setPending(true);
        const answer = await api.openPairing(scope);
        setPending(false);

        if (!answer.ok) {
            toast(answer.detail || t("settings.actionFailed"));
        }
    };
    const close = async () => {
        setPending(true);
        const answer = await api.closePairing(scope);
        setPending(false);

        if (!answer.ok && answer.status !== 204) {
            toast(answer.detail || t("settings.actionFailed"));
        }
    };

    let help = t(scope === "server" ? "pairing.serverModeHelp" : "pairing.accountModeHelp");

    if (mode) {
        help = t("pairing.openUntil", { time: timeOf(mode.open_until, i18n.language) });
    } else if (closure && closure.reason !== "owner") {
        help = t(closure.reason === "too_many_failures" ? "pairing.closedFailures" : "pairing.closedTimeout");
    }

    return (
        <Section title={t("pairing.modeTitle")}>
            <Rows>
                <SwitchRow id={`pr-${scope}-mode`} label={t("pairing.modeSwitch")} help={help} checked={Boolean(mode)} disabled={pending || !pairing}
                    onChange={(on) => {
                        if (!on) {
                            close();

                            return;
                        }
                        confirm({
                            title: t(scope === "server" ? "pairing.confirmServerTitle" : "pairing.confirmAccountTitle"),
                            message: t(scope === "server" ? "pairing.confirmServer" : "pairing.confirmAccount"),
                            confirmLabel: t("pairing.confirmOpen"),
                            onConfirm: open,
                        });
                    }} />
                {mode && <NavRow id={`pr-${scope}-extend`} icon="bi-clock-history" tone="indigo" label={t("pairing.extend")} help={t("pairing.extendHelp")}
                    pending={pending} opens="action" onOpen={open} />}
            </Rows>
        </Section>
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
    const host = useFederationHost();
    const origin = host.useOrigin();
    const serverName = host.useServerName();
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
            <Section title={t("pairing.code")}>
                <p className="display-4 font-monospace text-center mb-0" data-key="pr-code" aria-label={t("pairing.code")}>{groupedCode(created.code)}</p>
                <Note>{t("pairing.validFor", { left: leftText(t, Date.parse(created.expires_at) - now) })}</Note>
            </Section>
            <Section title={t("fed.serverTitle")}>
                <Rows>
                    <InfoRow icon="bi-hdd-network" tone="amber" label={serverName || t("fed.urlLabel")} value={origin} help={t("pairing.thisServerHelp")} />
                </Rows>
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="pr-cancel" icon="bi-x-circle" tone="slate" label={t("pairing.cancel")} help={t("pairing.cancelHelp")} opens="action" onOpen={onCancel} />
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
        <Section title={t("pairing.localTitle")} help={t("pairing.localHelp")}>
            <Rows>
                {(servers || []).map((server) => (
                    <NavRow key={`${server.instance}-${server.address}`} id={`pr-found-${server.address}-${server.instance}`} icon="bi-hdd-network" tone="amber"
                        label={server.name} help={counted[server.name] > 1 ? t("pairing.sameName", { address: server.address }) : server.link}
                        opens="action" onOpen={() => onChoose(server)} />
                ))}
                {servers && servers.length === 0 && <EmptyRow icon="bi-wifi-off" text={t("pairing.localEmpty")} />}
                {servers === null && <EmptyRow icon="bi-wifi" text={t("pairing.localSearching")} />}
                <NavRow id="pr-search" icon="bi-arrow-repeat" tone="indigo" label={t("pairing.searchAgain")} help={t("pairing.searchAgainHelp")} opens="action"
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
