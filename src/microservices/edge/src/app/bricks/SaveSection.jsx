import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Form, FormButton, formContext } from "../ui/Forms.jsx";
import { Listing, useListSearch } from "./Listing.jsx";
import { Rows } from "./Rows.jsx";
import { Section } from "./Section.jsx";
import { SubmitRow } from "./SubmitRow.jsx";
import { useFeedback } from "./useFeedback.jsx";

// UNE SECTION QUI S'ENREGISTRE PAR SON BOUTON (chantier 81 ; le porteur, 2026-11-25 : « Faudra rajouter des boutons pour
// tous les formulaires qui n'en ont pas » ; « Avec bouton ça permet de savoir si il y a un problème ou pas pendant la
// sauvegarde vu qu'il y aura des feedbacks ») — plus aucun réglage du serveur ne part au changement d'un interrupteur ou
// d'un sélecteur. Écrite UNE fois : ses champs (`children(values, set)`), puis son bouton, actif quand quelque chose a
// changé ; la réponse en fenêtre (useFeedback).
//
//   <SaveSection id="lg" title={…} help={…} saved={{ kernel: true }} onSave={(values) => api(values)}>
//       {(values, set) => <SwitchRow … checked={values.kernel} onChange={(kernel) => set({ kernel })} />}
//   </SaveSection>
//
// `saved` : ce qui est enregistré (un objet) ; quand il change (une relecture), les champs le suivent. `onSave(values)` →
// la réponse ({ok, detail | errorMessage}) ; `success` : le texte d'un enregistrement réussi (ou `(values) => texte`).
// `disabled` : rien à enregistrer (une page grisée). `after` : ce qui suit le bouton dans la section (une note).
// `rules` : les règles des champs texte ({champ: (values) => Ruleset…}, comme formContext) ; `children` reçoit alors le
// contexte du formulaire en troisième (`(values, set, form)`), et une règle enfreinte garde le bouton éteint.
//
// `list` : DES LIGNES QUI PEUVENT ÊTRE NOMBREUSES (un interrupteur par serveur fédéré ; le porteur, 2026-11-25 :
// « Pagination et recherche, oui ») — `{ items, textOf, placeholder, render: (item, values, set) => ligne }` : un
// listing qui se cherche et se feuillette (Listing), sous les champs de `children` s'il y en a, puis le bouton.
export function SaveSection({ id, title, help = null, sense = "modify", saved, onSave, success = null, disabled = false, after = null, rules = {}, list = null, children = null }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const [base, setBase] = useState(saved);
    const [values, setValues] = useState(saved);
    const [pending, setPending] = useState(false);
    const key = JSON.stringify(saved);

    useEffect(() => {
        setBase(saved);
        setValues(saved);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [key]);

    const broken = Object.values(rules).some((rule) => Boolean(rule(values)));
    const changed = !disabled && !broken && JSON.stringify(values) !== JSON.stringify(base);
    const set = (patch) => setValues((current) => ({ ...current, ...patch }));
    const form = formContext(values, setValues, rules);
    const listed = useListSearch(list?.items || [], list?.textOf || (() => ""));

    const submit = async (event) => {
        event.preventDefault();

        if (!changed || pending) {
            return;
        }
        setPending(true);
        const sent = values;
        const answer = await onSave(sent);
        setPending(false);

        if (answer?.ok) {
            setBase(sent);
            feedback.success((typeof success === "function" ? success(sent) : success) || t("settings.saved"));
        } else {
            feedback.failed(answer?.detail || answer?.errorMessage || t("settings.saveFailed"));
        }
    };

    const button = (
        <SubmitRow>
            <FormButton id={`${id}-save`} label={t("settings.save")} isReplaced={changed} pending={pending} />
        </SubmitRow>
    );

    return (
        <Form props={{ onSubmit: submit }}>
            <Section title={title} help={help} sense={sense}>
                {list ? (
                    <>
                        {children && <Rows className="mb-3">{children(values, set, form)}</Rows>}
                        <Listing items={listed.found} search={listed.search} onSearch={listed.onSearch} placeholder={list.placeholder} searchKey={`${id}-search`}
                            render={(item) => list.render(item, values, set)} />
                        <Rows className="mt-3">{button}</Rows>
                    </>
                ) : (
                    <Rows>
                        {children(values, set, form)}
                        {button}
                    </Rows>
                )}
                {after}
            </Section>
        </Form>
    );
}
