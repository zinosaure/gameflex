import React from "react";
import { useTranslation } from "react-i18next";
import { copyTextToClipboard, qrImage } from "../libs/share-link.js";
import { CopyField } from "../bricks/CopyField.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { useConfirm } from "../bricks/useConfirm.jsx";
import { useFeedback } from "../bricks/useFeedback.jsx";
import { FormButton, FormButtons } from "./Forms.jsx";

// LA DOUBLE AUTHENTIFICATION, SES DEUX MOMENTS À MONTRER (chantier 80, `#809`), les mêmes à la connexion
// (screens/access/MfaStep.jsx) et dans Mon compte (settings/profile/Mfa.jsx).

// L'ACTIVER : le code QR que l'application d'authentification scanne, et la clé en texte pour une application sur le
// même téléphone (qui ne peut pas scanner son propre écran). Le code QR se dessine dans la page (qrcode-generator) :
// le secret ne part jamais vers un service extérieur.
export function MfaEnrollment({ uri, secret }) {
    const { t } = useTranslation();

    return (
        <Rows>
            <div className="list-group-item gf-row">
                <div className="bg-white rounded-4 p-3 mx-auto gf-qr">
                    <img className="d-block w-100" src={qrImage(uri)} alt={t("mfa.qrAlt")} />
                </div>
            </div>
            <CopyField id="mfa-secret" label={t("mfa.secret")} help={t("mfa.secretHelp")} value={secret} />
        </Rows>
    );
}

// LES CODES DE SECOURS, MONTRÉS UNE SEULE FOIS : dix codes à usage unique, à garder hors du téléphone. Les copier tous
// d'un geste, le plus important (le porteur, 2026-11-24 : « Le plus important c'est de copier », 75/25) ; « Je les ai
// gardés » en rouge, confirmé dans la fenêtre de Skeleton : les codes ne se reverront plus. `onDone` : confirmé.
export function RecoveryCodes({ codes, onDone }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const copy = async () => ((await copyTextToClipboard(codes.join("\n"))) ? feedback.success(t("settings.copied")) : feedback.failed(t("settings.copyFailed")));
    const confirm = useConfirm();
    const kept = () => confirm({ title: t("mfa.codesKeptTitle"), message: t("mfa.codesKeptConfirm"), confirmLabel: t("mfa.codesKept"), onConfirm: onDone });

    return (
        <Rows>
            <div className="list-group-item gf-row">
                <div className="row row-cols-2 g-2 font-monospace text-center" data-mfa-recovery>
                    {codes.map((code) => <div key={code} className="col">{code}</div>)}
                </div>
            </div>
            <div className="list-group-item gf-row gf-row-submit">
                <FormButtons split="75/25">
                    <FormButton bare id="mfa-copy-codes" type="button" label={t("mfa.copyCodes")} isReplaced onClick={copy} />
                    <FormButton bare id="mfa-codes-kept" type="button" kind="danger" label={t("mfa.codesKept")} isReplaced onClick={kept} />
                </FormButtons>
            </div>
        </Rows>
    );
}
