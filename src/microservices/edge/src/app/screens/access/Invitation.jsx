import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { createInvitation, readInvitationImage } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { toast } from "../../shell/Shell.jsx";
import { Ruleset } from "../../ui/Forms.jsx";
import { useFederationHost } from "../../settings/people/federationHost.js";
import NavRow, { CopyField, FileRow, Note, Rows, Section, TextRow } from "../../ui/Bricks.jsx";

// LES INVITATIONS (shared/Invitation.jsx de la webapp, chantier 33) : un lien, ou son QR, que
// l'autre serveur échange contre un jeton — le jeton, lui, ne voyage jamais. Valable 12 heures,
// une seule fois. En direction C (Bricks.jsx) : l'invitation créée et « Mon QR » sont chacun UN groupe
// (Section), son titre compris — la page qui les montre les pose tels quels, sans groupe autour.

const dateTime = (iso, language) => (iso ? new Date(iso).toLocaleString(language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "");

// L'adresse publique de ce serveur : celle que l'hôte déclare (la LAN du téléphone), sinon celle de la page.
export const useShareOrigin = () => {
    const { capabilities } = useData();

    return capabilities.shareOrigin || location.origin;
};

// Le lien d'une invitation de CE serveur.
export const invitationLink = (origin, code) => `${origin}/invite/${code}`;

// LE LIEN D'UN COMPTE (chantier 55) : la page /join/<code>, qu'une personne ouvre dans son navigateur — sur l'adresse
// publique de ce serveur.
export const useMemberLink = () => {
    const origin = useShareOrigin();

    return (code) => `${origin}/join/${code}`;
};

// UNE INVITATION CRÉÉE (05-jeton-mon-qr, 30-federation-invitation des modèles) : son groupe, titré — son QR (dessiné
// par le kernel), sa date limite ; « Le lien », à copier ; puis « Plus » : le QR se télécharge (pas dans
// l'application, qui a son propre partage), Partager ouvre le partage du système, Terminé la referme. Ces trois gestes
// n'agissent sur aucun champ : des LIGNES, pas des boutons (le porteur, 2026-11-13 : « Les boutons orphelins […] ne
// doivent pas être des boutons mais des liens »). `canShare` faux : pas de Partager — l'invitation de MON compte
// (MyInvitation ; le porteur, 2026-11-14 : « dans create my QR, il faut retirer le lien "share" c'est trop risqué »), ni
// celle d'un membre (le lien de son compte, chantier 55). `help` et `validity` : ce que dit l'invitation d'un membre à la
// place de celle d'un serveur — comment elle sert, et combien de temps elle vaut (une clé, qui reçoit `at`).
export function InvitationReveal({ link, expiresAt, title, onDone, canShare = true, help = null, validity = null }) {
    const { t, i18n } = useTranslation();
    const host = useFederationHost();
    const [image, setImage] = useState(null);
    const inApp = host.useInApp();

    useEffect(() => {
        let url = null;
        let cancelled = false;
        host.invitationQR(link).then((blob) => {
            if (!cancelled && blob) {
                url = URL.createObjectURL(blob);
                setImage({ url, blob });
            } else if (!cancelled) {
                setImage(false);
            }
        });

        return () => {
            cancelled = true;

            if (url) {
                URL.revokeObjectURL(url);
            }
        };
    }, [link]);

    const download = () => {
        const anchor = document.createElement("a");
        anchor.href = image.url;
        anchor.download = "gameflex-invitation.png";
        document.body.appendChild(anchor);
        anchor.click();
        anchor.remove();
    };

    const share = async () => {
        const file = image && new File([image.blob], "gameflex-invitation.png", { type: "image/png" });

        try {
            if (file && !inApp && navigator.canShare?.({ files: [file] })) {
                await navigator.share({ files: [file], text: link, title });
            } else if (navigator.share) {
                await navigator.share({ url: link, title, text: title });
            } else {
                toast(t("invitations.shareUnavailable"));
            }
        } catch {
            // Partage abandonné : rien à dire.
        }
    };

    return (
        <>
            <Section title={title}>
                <Note className="mt-0">{help || t("invitations.revealHelp")}</Note>
                <div className="text-center mt-3">
                    {image === false ? <Note className="mt-0">{t("invitations.qrUnavailable")}</Note> : image
                        ? <img src={image.url} alt={t("invitations.qrAlt")} className="rounded img-fluid bg-white p-2" width="240" height="240" />
                        : <span className="spinner-border" role="status"></span>}
                </div>
                {/* Sa validité, en ligne atténuée sous le code — plus une alerte (le porteur, 2026-11-16). */}
                <Note>{t(validity || "invitations.validUntil", { at: dateTime(expiresAt, i18n.language) })}</Note>
            </Section>
            <Section title={t("invitations.linkTitle")}>
                <Rows>
                    <CopyField id="iv-link" label={t("invitations.linkLabel")} help={t("invitations.linkHelp")} value={link} />
                </Rows>
            </Section>
            <Section title={t("more")}>
                <Rows>
                    {!inApp && <NavRow id="iv-download" icon="bi-download" tone={image ? "indigo" : "slate"} label={t("invitations.download")} disabled={!image} opens="action" onOpen={download} />}
                    {canShare && <NavRow id="iv-share" icon="bi-share" tone="indigo" label={t("invitations.share")} help={t("invitations.shareHelp")} opens="action" onOpen={share} />}
                    {onDone && <NavRow id="iv-done" icon="bi-check2" tone="slate" label={t("invitations.done")} help={t("invitations.doneHelp")} opens="action" onOpen={onDone} />}
                </Rows>
            </Section>
        </>
    );
}

// LE CHAMP D'UNE INVITATION REÇUE (Skeleton, comme shared/Invitation.jsx de la webapp) : son lien, ou l'image de
// son QR (lue par le kernel), qui le remplit — deux lignes de son groupe, le choix de l'image dans la sienne
// (FileRow, Form.File de Skeleton). Form.File range l'image dans `files` ; elle se lit aussitôt, et la liste se vide :
// c'est le lien qui compte, pas le fichier. Il sert aussi hors des réglages (l'engrenage d'un jeu,
// FederatedAccount.jsx) : FileRow y mène la manette jusqu'au choix de l'image, que Skeleton dessine seul.
// `children` : la dernière ligne du groupe — le bouton du formulaire qui l'envoie, dans son encadré (SubmitRow).
export function InvitationField({ id, value, onChange, label, help, children = null }) {
    const { t } = useTranslation();
    const [reading, setReading] = useState(false);
    const [picked, setPicked] = useState({ qr: [] });
    const picker = useRef(null);

    const read = async (file) => {
        setReading(true);
        const answer = await readInvitationImage(file);
        setReading(false);
        // LA MÊME IMAGE, UNE SECONDE FOIS : le champ de fichier garde son choix, et un choix identique ne
        // déclenche aucun changement (la leçon de la webapp, 2026-10-26).
        picker.current?.querySelectorAll("input[type=file]").forEach((input) => {
            input.value = "";
        });

        if (!answer.ok || !answer.data?.text) {
            toast(t(`invitations.errors.${answer.errorCode || "noQR"}`, { defaultValue: t("invitations.errors.noQR") }));

            return;
        }
        onChange(answer.data.text);
        toast(t("invitations.imageRead"));
    };

    const pickContext = {
        values: { qr: [] },
        setValues: () => {},
        files: picked,
        setFiles: (next) => {
            const file = next.qr?.[0];
            setPicked({ qr: [] });

            if (file) {
                read(file);
            }
        },
        rulesets: reading ? { qr: () => Ruleset.isDisabled(t("invitations.imageReading")) } : {},
    };

    return (
        <div ref={picker}>
            <Rows>
                <TextRow id={id} label={label} help={help} value={value} onChange={(next) => onChange(next || "")} props={{ type: "url", placeholder: "https://…/invite/…" }} />
                <FileRow id="qr" label={t("invitations.imageLabel")} help={reading ? t("invitations.imageReading") : t("invitations.imageHelp")} upload={{ context: pickContext }} maxFileSize={null} />
                {children}
            </Rows>
        </div>
    );
}

// MON QR (Profile.jsx de la webapp, MyInvitation ; 04-jeton des modèles) : plutôt que de recopier mon jeton,
// une invitation de MON compte, que l'autre serveur échange contre lui — seulement quand mon jeton est actif. Son geste
// est une ligne de « Plus » (le porteur, 2026-11-14 : « "create my QR" doit passer dans more »), grisée tant que le jeton
// n'est pas actif, la raison en sous-titre ; le tourniquet à son bord droit le temps de la création. `children` : une
// fonction qui reçoit cette ligne et rend la page (Réglages › Jeton la pose dans son « Plus »). L'invitation créée prend
// la page entière jusqu'à Terminé, comme dans 05-jeton-mon-qr : son groupe « Plus » est alors le seul de la page — sans
// Partager (« trop risqué »).
export function MyInvitation({ live, children }) {
    const { t } = useTranslation();
    const origin = useShareOrigin();
    const [invited, setInvited] = useState(null);
    const [pending, setPending] = useState(false);

    if (invited) {
        return <InvitationReveal link={invitationLink(origin, invited.code)} expiresAt={invited.expires_at} title={t("invitations.accountTitle")} canShare={false} onDone={() => setInvited(null)} />;
    }

    return children(
        <NavRow id="iv-my-qr" icon="bi-qr-code" tone={live ? "indigo" : "slate"} label={t("invitations.myQRCreate")} help={live ? t("invitations.accountHelp") : t("invitations.myQRNeedsToken")}
            pending={pending} disabled={!live} opens="action" onOpen={async () => {
                setPending(true);
                const answer = await createInvitation({ kind: "account" });
                setPending(false);

                if (answer.ok && answer.item) {
                    setInvited(answer.item);
                } else {
                    toast(answer.detail || t("settings.actionFailed"));
                }
            }} />,
    );
}
