import React from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { counted, fact } from "../../libs/format.js";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import { switchGroup } from "../../bricks/FilterGroups.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { hiddenOf, toggleHidden } from "../../globals/filters.js";
import { COMPONENTS, PAGES, TEMPLATES } from "./catalog.jsx";
import MockupsPage from "./Mockups.jsx";

// LES MODÈLES DES RÉGLAGES (la dev seulement, capacité `staging` ; catalog.jsx) : la liste des gabarits de page (T…), des
// composants (C…) et des pages des Réglages avec leur gabarit — puis la fiche de chaque code. Ces pages appliquent
// elles-mêmes les gabarits qu'elles montrent : T01 pour l'accueil, T02 pour chaque liste, T03 pour une fiche.
//
//   /settings/templates           les trois listes
//   /settings/templates/gallery   la galerie : l'exemple de chaque composant, à la suite
//   /settings/templates/c81       les maquettes du chantier 81 (Mockups.jsx), `?m=` en choisit une
//   /settings/templates/<code>    la fiche d'un gabarit ou d'un composant
const BASE = "/settings/templates";
const STATUS_TONE = { conforme: "green", "à vérifier": "amber", "à revoir": "rose" };
const STATUSES = ["conforme", "à vérifier", "à revoir"];
const byCode = (code) => TEMPLATES.find((entry) => entry.code === code) || COMPONENTS.find((entry) => entry.code === code) || null;
const matches = (needle, ...texts) => !needle || texts.join(" ").toLowerCase().includes(needle);

// La ligne d'une page des Réglages : son nom, son gabarit et son statut ; elle ouvre la fiche de son gabarit.
function PageRow({ page, onOpen }) {
    return (
        <NavRow key={page.path} id={`tpl-page-${page.path}`} icon="bi-file-earmark" tone={STATUS_TONE[page.status] || "slate"} label={page.name}
            help={[fact("Gabarit", page.template), fact("Statut", page.status), page.note || null]} onOpen={onOpen} />
    );
}

// Les pages d'un gabarit (ou toutes), cherchées par leur nom ou leur adresse, filtrées par statut et par gabarit.
function PagesListing({ pages, withTemplates = false }) {
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ pq: "", nostatus: "", notemplate: "", pagesPage: "1" });
    const needle = read("pq").trim().toLowerCase();
    const hiddenStatuses = hiddenOf(read("nostatus"));
    const hiddenTemplates = hiddenOf(read("notemplate"));
    const shown = pages.filter((page) => matches(needle, page.name, page.path) && !hiddenStatuses.includes(page.status) && !hiddenTemplates.includes(page.template));

    return (
        <Listing items={shown} search={read("pq")} onSearch={(pq) => update({ pq, pagesPage: null })} placeholder="Rechercher une page…" searchKey="tpl-pages-search"
            found={`${shown.length} pages`} empty="Aucune page." page={read("pagesPage")} onPage={(page) => update({ pagesPage: page })}
            filterCount={hiddenStatuses.length + hiddenTemplates.length} filters={[
                switchGroup({ key: "statuses", title: "Statut", id: "tpl-status", hidden: hiddenStatuses, onChange: (value, on) => update({ nostatus: toggleHidden(hiddenStatuses, value, on), pagesPage: null }),
                    choices: STATUSES.filter((status) => pages.some((page) => page.status === status)).map((status) => ({ value: status, label: status })) }),
                withTemplates && switchGroup({ key: "templates", title: "Gabarit", id: "tpl-template", hidden: hiddenTemplates, onChange: (value, on) => update({ notemplate: toggleHidden(hiddenTemplates, value, on), pagesPage: null }),
                    choices: TEMPLATES.map((entry) => ({ value: entry.code, label: `${entry.code} — ${entry.name}` })) }),
            ]}
            render={(page) => <PageRow key={page.path} page={page} onOpen={() => open(`${BASE}/${page.template}`, `tpl-page-${page.path}`)} />} />
    );
}

// /settings/templates : les gabarits, les composants, les pages.
function TemplatesPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ q: "", componentsPage: "1" });
    const needle = read("q").trim().toLowerCase();
    const components = COMPONENTS.filter((entry) => matches(needle, entry.code, entry.name, entry.role));
    const pagesOf = (code) => PAGES.filter((page) => page.template === code);

    return (
        <SettingsPage title={t("settings.items.templates.label")}>
            <Section title={t("templates.templatesTitle")} help={t("templates.templatesHelp")}>
                <Rows>
                    {TEMPLATES.map((entry) => (
                        <NavRow key={entry.code} id={`tpl-${entry.code}`} icon="bi-layout-text-window" tone="indigo" label={`${entry.code} — ${entry.name}`}
                            help={[fact("Pages", counted(t, "pages", pagesOf(entry.code).length)), fact("Conformes", counted(t, "pages", pagesOf(entry.code).filter((page) => page.status === "conforme").length))]}
                            onOpen={() => open(`${BASE}/${entry.code}`, `tpl-${entry.code}`)} />
                    ))}
                </Rows>
            </Section>
            <Section title={t("templates.componentsTitle")}>
                <Listing items={components} search={read("q")} onSearch={(q) => update({ q, componentsPage: null })} placeholder="Rechercher un composant…" searchKey="tpl-components-search"
                    found={`${components.length} composants`} empty="Aucun composant." page={read("componentsPage")} onPage={(page) => update({ componentsPage: page })}
                    render={(entry) => (
                        <NavRow key={entry.code} id={`tpl-${entry.code}`} icon="bi-puzzle" tone="teal" label={`${entry.code} — ${entry.name}`}
                            help={[fact("Fichier", entry.file), fact("Arguments", counted(t, "arguments", entry.args.length))]} onOpen={() => open(`${BASE}/${entry.code}`, `tpl-${entry.code}`)} />
                    )} />
            </Section>
            <Section title={t("templates.pagesTitle")} help={t("templates.pagesHelp")}>
                <PagesListing pages={PAGES} withTemplates />
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="tpl-gallery" icon="bi-grid-3x3-gap" tone="teal" label={t("templates.galleryTitle")} help={t("templates.galleryHelp")} onOpen={() => open(`${BASE}/gallery`, "tpl-gallery")} />
                    <NavRow id="tpl-mockups" icon="bi-easel" tone="teal" label={t("templates.mockupsTitle")} help={t("templates.mockupsHelp")} onOpen={() => open(`${BASE}/c81`, "tpl-mockups")} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}

// /settings/templates/gallery : LA GALERIE (le porteur, 2026-11-23 : « une page avec tous les exemples des composants
// seulement (avec le nom, id unique et lien vers la fiche) comme ça je pourrais plus facilement te dire ce qu'il faut faire
// et avec quoi ») — chaque composant sous son code et son nom, son exemple vivant, puis le lien vers sa fiche.
function GalleryPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const levels = { [BASE]: t("settings.items.templates.label") };

    return (
        <SettingsPage title={t("templates.galleryTitle")} parent={BASE} levels={levels}>
            {COMPONENTS.filter((entry) => entry.example).map((entry) => (
                // Le code en pastille à côté du nom (Section `badge`) : il se distingue des titres des exemples eux-mêmes.
                <Section key={entry.code} title={entry.name} badge={<span className="badge rounded-pill text-bg-primary">{entry.code}</span>}>
                    {entry.example()}
                    <Rows className="mt-3">
                        <NavRow id={`tpl-gallery-${entry.code}`} icon="bi-file-earmark-text" tone="slate" label={t("templates.openSheet", { code: entry.code })}
                            onOpen={() => open(`${BASE}/${entry.code}`, `tpl-gallery-${entry.code}`)} />
                    </Rows>
                </Section>
            ))}
        </SettingsPage>
    );
}

// /settings/templates/<code> : la fiche d'un gabarit (sa structure, son exemple, ses pages) ou d'un composant (ses
// arguments, son exemple, les gabarits qui l'utilisent).
function TemplateItemPage() {
    const { t } = useTranslation();
    const { code } = useParams();
    const { open } = useSettingsNavigation();
    const entry = byCode(code);
    const isTemplate = Boolean(entry?.structure);
    const levels = { [BASE]: t("settings.items.templates.label") };

    if (!entry) {
        return <SettingsPage title={code} parent={BASE} levels={levels}><NotFound /></SettingsPage>;
    }
    const usedBy = isTemplate ? [] : TEMPLATES.filter((template) => template.structure.some(([, parts]) => parts.includes(entry.code)));

    return (
        <SettingsPage title={`${entry.code} — ${entry.name}`} parent={BASE} levels={levels}>
            <Section title={t("details")}>
                <Rows>
                    <InfoRow icon="bi-upc" label="Code" value={entry.code} />
                    <InfoRow icon="bi-tag" label="Nom" value={entry.name} />
                    {entry.file && <InfoRow icon="bi-file-earmark-code" label="Fichier" value={entry.file} mono />}
                    <InfoRow icon="bi-info-circle" label="Rôle" value={entry.role} />
                </Rows>
            </Section>
            {isTemplate ? (
                <Section title={t("templates.structureTitle")} help={t("templates.structureHelp")}>
                    <Rows>
                        {entry.structure.map(([part, parts], index) => (
                            <InfoRow key={part} icon={`bi-${index + 1}-circle`} tone="indigo" label={part} value={parts} />
                        ))}
                    </Rows>
                </Section>
            ) : (
                <Section title={t("templates.argsTitle")}>
                    <Rows>
                        {entry.args.map(([name, type, fallback, role]) => (
                            <InfoRow key={name} icon="bi-braces" label={name} value={[fact("Type", type), fact("Défaut", fallback)]} help={role} noKey={false} />
                        ))}
                    </Rows>
                </Section>
            )}
            {entry.example && (
                <Section title={t("templates.exampleTitle")} help={t("templates.exampleHelp")}>
                    <div className="gf-template-example">{entry.example()}</div>
                </Section>
            )}
            {isTemplate ? (
                <Section title={t("templates.appliedTitle")}>
                    <PagesListing pages={PAGES.filter((page) => page.template === entry.code)} />
                </Section>
            ) : usedBy.length > 0 && (
                <Section title={t("templates.usedByTitle")}>
                    <Rows>
                        {usedBy.map((template) => (
                            <NavRow key={template.code} id={`tpl-used-${template.code}`} icon="bi-layout-text-window" tone="indigo" label={`${template.code} — ${template.name}`}
                                onOpen={() => open(`${BASE}/${template.code}`, `tpl-used-${template.code}`)} />
                        ))}
                    </Rows>
                </Section>
            )}
        </SettingsPage>
    );
}

export function TemplatesRoutes() {
    return (
        <Routes>
            <Route index element={<TemplatesPage />} />
            <Route path="gallery" element={<GalleryPage />} />
            <Route path="c81" element={<MockupsPage />} />
            <Route path=":code" element={<TemplateItemPage />} />
            <Route path="*" element={<Navigate to={BASE} replace />} />
        </Routes>
    );
}
