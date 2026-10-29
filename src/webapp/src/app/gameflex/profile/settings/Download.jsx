import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
// Le MÊME formateur que partout ailleurs : celui d'ici disait « Mo » en
// toutes langues, et ne connaissait que les mégaoctets.
import { formatSize } from "./admin/api/library.js";
import * as bs from "react-bootstrap";

// Page de téléchargement de l'application Android.
//
// Elle existe pour qu'une installation ne demande ni câble, ni adb, ni
// ordinateur : on ouvre <le serveur>/download sur le téléphone, on
// télécharge, on installe. C'est le seul chemin praticable pour installer
// chez quelqu'un d'autre.
//
// L'APK est un fichier statique servi depuis public/downloads/ — déposé là
// par le build (src/mobileapp/docker-entrypoint.sh). La page ne le suppose
// pas présent : elle demande d'abord au serveur s'il existe, et dit
// clairement le contraire sinon. Une page de téléchargement qui offre un lien
// mort est pire que pas de page du tout.
const APK_URL = "/downloads/gameflex.apk";
// Écrit par le build (src/mobileapp/docker-entrypoint.sh) à côté de l'APK.
const MANIFEST_URL = "/downloads/gameflex.json";

export default function Download() {
    const { t } = useTranslation();
    // null = on cherche encore, false = absent, objet = présent.
    const [file, setFile] = useState(null);

    useEffect(() => {
        // Le manifeste d'abord, et LUI est demandé sans cache : c'est la
        // seule façon d'apprendre qu'une nouvelle version existe. Une URL
        // d'APK qui ne change jamais est gardée par le navigateur et par tout
        // intermédiaire — on retélécharge alors l'ancienne sans le voir
        // (signalé le 2026-08-26 : « j'ai pas reçu la dernière version »).
        fetch(`${MANIFEST_URL}?t=${Date.now()}`, { cache: "no-store" })
            .then((response) => (response.ok ? response.json() : Promise.reject()))
            .then((manifest) => setFile({
                size: manifest.size || 0,
                version: manifest.version,
                builtAt: manifest.builtAt,
                // La date de build DANS l'URL : elle change à chaque version,
                // donc le cache ne peut plus servir la précédente.
                url: `${APK_URL}?v=${encodeURIComponent(manifest.builtAt || "")}`,
            }))
            // Pas de manifeste — une APK déposée à la main, ou un build
            // d'avant ce mécanisme. On retombe sur les en-têtes HTTP plutôt
            // que de déclarer l'application absente.
            .catch(() => fetch(APK_URL, { method: "HEAD" })
                .then((response) => {
                    if (!response.ok) {
                        setFile(false);
                        return;
                    }
                    const updatedAt = response.headers.get("last-modified");
                    setFile({
                        size: Number(response.headers.get("content-length")) || 0,
                        builtAt: updatedAt,
                        url: `${APK_URL}?v=${encodeURIComponent(updatedAt || "")}`,
                    });
                })
                .catch(() => setFile(false)));
    }, []);

    return (
        <bs.Container className="py-5" style={{ maxWidth: "720px" }}>
            {file === null && (
                <div className="text-center py-4">
                    <bs.Spinner animation="border" role="status" />
                </div>
            )}

            {file === false && (
                <bs.Alert variant="warning">
                    <strong>{t("download.noBuildTitle")}</strong>
                    <div className="small mt-1">
                        {t("download.noBuildHelp")} <code>src/mobileapp</code> :
                        <code className="ms-1">docker compose run --rm build</code>.
                    </div>
                </bs.Alert>
            )}

            {file && (
                <>
                    <div className="d-grid mb-3">
                        {/* Un vrai lien, pas un bouton avec un handler : le
                            gestionnaire de téléchargement d'Android sait quoi
                            faire d'un href, et un lien fonctionne aussi quand
                            le JavaScript de la page a échoué. */}
                        <bs.Button as="a" href={file.url} download="gameflex.apk" variant="primary" size="lg">
                            <i className="bi bi-download me-2"></i>
                            {t("download.button")}
                        </bs.Button>
                    </div>
                    {/* La version et l'heure de build sont affichées pour
                        qu'on puisse VÉRIFIER qu'on a bien la dernière — c'est
                        la question qui a motivé tout ce mécanisme. */}
                    <p className="text-center text-muted small mb-0">
                        {file.version ? `v${file.version} · ` : ""}
                        {formatSize(file.size)}
                        {file.builtAt ? ` · ${new Date(file.builtAt).toLocaleString()}` : ""}
                    </p>
                </>
            )}

            <hr className="my-4" />

            <h5 className="fw-semibold">{t("download.installTitle")}</h5>
            <ol className="text-muted">
                <li>{t("download.installFromPhone")}</li>
                <li>{t("download.installAllow")}</li>
                <li>{t("download.installOpen")}</li>
            </ol>

            {/* CE QUE CETTE PAGE PROMETTAIT N'ÉTAIT PLUS VRAI (2026-09-09).
                Elle annonçait « un core NES embarqué », « une ROM à la fois,
                pas encore de bibliothèque » et des manettes physiques
                invisibles « par limite du navigateur embarqué ». Les trois
                sont faux depuis des semaines : plus aucun cœur n'est embarqué
                (licences), l'appareil a une vraie bibliothèque avec ses
                sources distantes, et la WebView charge le PWA depuis
                127.0.0.1 — un contexte sûr, donc l'API Gamepad y répond.

                Une page qui décrit l'application est une promesse ; elle se
                relit quand l'application change. */}
            <h5 className="fw-semibold mt-4">{t("download.whatTitle")}</h5>
            <p className="text-muted mb-2">{t("download.whatConnected")}</p>
            <p className="text-muted mb-0">{t("download.whatAlone")}</p>
        </bs.Container>
    );
}
