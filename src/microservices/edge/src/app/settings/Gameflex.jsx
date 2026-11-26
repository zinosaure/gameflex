import React from "react";
import { useTranslation } from "react-i18next";
import { GUIDES, openExternal, REPOSITORY_URL, repositoryFile } from "../libs/links.js";
// Le fichier LICENSE du dépôt, en texte (webpack.config.js : asset/source ; le compose le monte à /src/LICENSE).
import LICENSE from "../../LICENSE";
import SettingsPage, { usePageText } from "./SettingsPage.jsx";
import { BlockRow } from "../bricks/BlockRow.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";

// LE PROJET SUR GITHUB (le porteur, 2026-11-14, chantier 53, P13 : « dans settings > about rajoute "GameFlex" avec un
// lien vers le GitHub : Readme, Licence, Fédération, etc... vers chaque doc et aussi le repos ») : le dépôt PUBLIC
// (libs/links.js, REPOSITORY_URL — jamais le dépôt privé) et chacun de ses documents, tels que le dépôt les a le
// 2026-11-14 (relevés par l'API de GitHub) ; ni AGENTS.md ni CLAUDE.md, qui ne s'adressent pas aux personnes. Chaque
// ligne ouvre son adresse dans un autre onglet (`opens="link"`). Les documents sont en anglais : l'aide du premier
// groupe le dit. `path` : un fichier du dépôt ; `url` : une autre page du dépôt.
const PROJECT_GROUPS = [
    {
        key: "project", tone: "indigo", rows: [
            { key: "repository", icon: "bi-github", url: REPOSITORY_URL },
            { key: "readme", icon: "bi-book", path: "README.md" },
            { key: "changelog", icon: "bi-clock-history", path: "CHANGELOG.md" },
            { key: "releases", icon: "bi-tags", url: `${REPOSITORY_URL}/releases` },
        ],
    },
    // Les guides : ceux de libs/links.js (GUIDES), que les sections « Guide » des pages citent aussi.
    { key: "guides", tone: "teal", rows: Object.entries(GUIDES).map(([key, guide]) => ({ key, ...guide })) },
    {
        key: "licences", tone: "slate", rows: [
            { key: "licence", icon: "bi-patch-check", path: "LICENSE" },
            { key: "binaryLicence", icon: "bi-file-earmark-binary", path: "LICENSE-BINARIES.md" },
            { key: "thirdPartyNotices", icon: "bi-people", path: "THIRD-PARTY-NOTICES.md" },
            { key: "thirdPartyLicences", icon: "bi-file-earmark-text", path: "THIRD-PARTY-LICENSES.txt" },
            { key: "disclaimer", icon: "bi-exclamation-circle", path: "DISCLAIMER.md" },
        ],
    },
    {
        key: "contribute", tone: "green", rows: [
            { key: "contributing", icon: "bi-heart", path: "CONTRIBUTING.md" },
            { key: "security", icon: "bi-shield-lock", path: "SECURITY.md" },
        ],
    },
];

// LA LICENCE DE GAMEFLEX, EN ENTIER, DANS L'APPLICATION (le porteur, 2026-11-14, chantier 53, P13 : « Oui mais il faut au
// moins avec le texte licence de gameflex dans l'application ») : le fichier LICENSE du dépôt lui-même — monté en
// lecture seule à /src/LICENSE (le compose) et inscrit dans le bundle par webpack (asset/source) : jamais une copie dans
// les traductions ou dans le code, et l'application la montre sans réseau. Ses paragraphes gardés ; les retours à la
// ligne du fichier, coupé à 80 colonnes, se fondent dans le paragraphe : sinon chaque ligne se casse en deux sur un
// téléphone. Un texte, sans commande : la croix le fait défiler (libs/input.js, `scrollStep`).
const LICENSE_PARAGRAPHS = LICENSE.trim().split(/\n\s*\n/).map((paragraph) => paragraph.replace(/\s*\n\s*/g, " "));

export function ProjectPage() {
    const { t } = useTranslation();
    const page = usePageText("gameflex");

    return (
        <SettingsPage title={page.title}>
            {PROJECT_GROUPS.map((group) => (
                <Section key={group.key} title={t(`project.${group.key}Title`)} help={group.key === "project" ? t("project.help") : null} sense="optional">
                    <Rows>
                        {group.rows.map((row) => (
                            <NavRow key={row.key} id={`pj-${row.key}`} icon={row.icon} label={t(`project.${row.key}`)} help={t(`project.${row.key}Help`)}
                                opens="link" onOpen={() => openExternal(row.url || repositoryFile(row.path))} />
                        ))}
                    </Rows>
                </Section>
            ))}
            <Section title={t("project.licenceTextTitle")} help={t("project.licenceTextHelp")} sense="info">
                <Rows>
                    <BlockRow>
                        <div className="text-break">
                            {LICENSE_PARAGRAPHS.map((paragraph, index) => <p key={index} className={index === LICENSE_PARAGRAPHS.length - 1 ? "mb-0" : "mb-3"}>{paragraph}</p>)}
                        </div>
                    </BlockRow>
                </Rows>
            </Section>
        </SettingsPage>
    );
}
