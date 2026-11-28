// Route : /settings/remote-play/new — ajouter une console.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChoiceRow } from "../../bricks/ChoiceRow.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { IconAddon } from "../../bricks/IconAddon.jsx";
import { Listing, useListSearch } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { discoverDevices, registerDevice } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { fact } from "../../libs/format.js";
import { toast } from "../../shell/Shell.jsx";
import { Form, FormButton, formContext, shownProblem as shownProblemOf } from "../../ui/Forms.jsx";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { HOST_TYPES, PSN_ACCOUNT_ID_URL, restoreLight, ROOT } from "../RemotePlay.jsx";

// ENREGISTRER UNE CONSOLE (RegisterPanel) : son type, son adresse — ou la recherche sur le réseau, dont chaque console
// trouvée remplit l'adresse, le nom et le type —, son nom, l'identifiant du compte PSN (le lien qui le donne sous le
// titre de son groupe) et le code PIN que la console affiche ; puis l'appairage (`/playstation/bind`, son bouton dans
// l'encadré du dernier groupe), et la liste.
//
// LE TYPE EST UN CHAMP ICI : la webapp le tirait de la console ouverte dans son menu (`/remoteplay/rp-ps5`) ; ce coin
// d'administration n'en a pas, et le service appairerait sinon une PS5 par défaut.
export default function NewDevice() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { back } = useSettingsNavigation();
    const { refreshDevices } = useData();
    const [values, setValues] = useState(() => ({ hostType: "PS5", ip: "", name: "", identifier: "", pin: "" }));
    const [pending, setPending] = useState(false);
    const [discovering, setDiscovering] = useState(false);
    const [found, setFound] = useState([]);
    const shown = useListSearch(found, (device) => [device.name, device.ip, device.hostType].join(" "));
    const title = t("remoteplay.addDevice");

    // Les règles de RegisterPanel ; une erreur se montre une fois qu'on a écrit dans le champ (un champ vide n'est
    // pas faux, il manque : le bouton reste grisé), comme les autres formulaires des Réglages.
    const problems = {
        ip: values.ip ? "" : t("form.required"),
        name: values.name && values.name.length >= 2 ? "" : t("form.required"),
        identifier: values.identifier ? "" : t("form.required"),
        pin: !values.pin ? t("form.required") : /^\d{1,8}$/.test(String(values.pin)) ? "" : t("remoteplay.register.pinInvalid"),
    };
    const shownProblem = shownProblemOf(problems);
    const form = formContext(values, setValues, { ip: shownProblem("ip"), name: shownProblem("name"), identifier: shownProblem("identifier"), pin: shownProblem("pin") });
    const invalid = Object.values(problems).some(Boolean);
    // Rien de tapé, rien de changé (CODE_CONVENTIONS.md, `isReplaced`) ; le type choisi seul ne suffit pas à enregistrer.
    const isReplaced = ["ip", "name", "identifier", "pin"].some((name) => values[name]);

    const discover = async () => {
        setDiscovering(true);
        setFound([]);
        const result = await discoverDevices();
        setDiscovering(false);

        if (!result.ok) {
            restoreLight("rp-discover");
            toast(result.errorMessage || t("remoteplay.register.discoverFailed"));

            return;
        }
        setFound(result.data || []);
        restoreLight("rp-found-", "rp-discover");

        if (!(result.data || []).length) {
            toast(t("remoteplay.register.discoverEmpty"));
        }
    };

    // Une console trouvée remplit l'adresse, le nom (s'il est vide) et le type ; la liste se referme, la lumière
    // revient à la recherche (un bouton : le clavier d'un téléphone ne surgit pas).
    const choose = (device) => {
        const hostType = String(device.hostType || "").toUpperCase();
        setValues((previous) => ({ ...previous, ip: device.ip, name: previous.name || device.name || "", hostType: HOST_TYPES.includes(hostType) ? hostType : previous.hostType }));
        setFound([]);
        restoreLight("rp-discover");
    };

    const submit = async () => {
        setPending(true);

        try {
            const result = await registerDevice({ name: values.name, ip: values.ip, identifier: values.identifier, pin: values.pin, hostType: values.hostType });

            if (result.ok) {
                feedback.success(t("remoteplay.register.success"));
                refreshDevices();
                back(ROOT);

                return;
            }
            feedback.failed(result.errorMessage || t("remoteplay.register.failure"));
        } catch (failure) {
            console.error("échec de l'enregistrement de l'appareil", failure);
            feedback.failed(t("remoteplay.register.failure"));
        }
        setPending(false);
        restoreLight("rp-register");
    };

    return (
        <SettingsPage title={title} parent={ROOT}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (!invalid && !pending) {
                    submit();
                }
            } }}>
                <Section title={t("rp.consoleTitle")} sense="info">
                    <Rows>
                        <ChoiceRow id="rp-type" label={t("remoteplay.device.type")} value={values.hostType} options={HOST_TYPES.map((type) => [type, type])}
                            onChange={(hostType) => setValues((previous) => ({ ...previous, hostType }))} />
                        <TextRow id="ip" label={t("remoteplay.register.ip")} help={t("remoteplay.register.ipHelp")} context={form} props={{ placeholder: t("remoteplay.register.ipPlaceholder") }}
                            addon={<IconAddon id="rp-discover" icon={discovering ? "bi-hourglass-split" : "bi-search"} label={t("remoteplay.register.discover")} disabled={discovering} onClick={discover} />} />
                    </Rows>
                </Section>
                {(found.length > 0 || discovering) && (
                    <Section title={t("remoteplay.register.discoverResultsLabel")} sense="modify">
                        <Listing items={shown.found} loading={discovering} search={shown.search} onSearch={shown.onSearch} placeholder={t("rp.searchDevices")}
                            searchKey="rp-found-search" render={(device) => (
                            <NavRow key={device.ip} id={`rp-found-${device.ip}`} icon="bi-playstation" label={device.name || device.ip}
                                help={[fact(t("remoteplay.device.type"), device.hostType), fact(t("remoteplay.device.ip"), device.ip)]} opens="action" onOpen={() => choose(device)} />
                        )} />
                    </Section>
                )}
                {/* L'IDENTIFIANT PSN S'OBTIENT AILLEURS (RemotePlay.jsx) : le lien du site qui le donne est dans l'aide de son
                    champ, DANS l'encadré — une action qui concerne un groupe y vit (le porteur, 2026-11-13) — et la manette
                    l'atteint. Sur le téléphone, l'adresse s'ouvre dans le navigateur (MainActivity). */}
                <Section title={t("rp.pairingTitle")} sense="modify">
                    <Rows>
                        <TextRow id="name" label={t("remoteplay.register.name")} help={t("remoteplay.register.nameHelp")} context={form} props={{ placeholder: t("remoteplay.register.namePlaceholder") }} />
                        <TextRow id="identifier" label={t("remoteplay.register.identifierLabel.playstation")} context={form}
                            help={<>{t("remoteplay.register.identifierHint")} <a href={PSN_ACCOUNT_ID_URL} target="_blank" rel="noopener noreferrer" data-nav data-key="rp-psn">{t("remoteplay.register.getViaPsnLogin")}</a></>} />
                        <TextRow id="pin" kind="number" min={0} max={99999999} step={1} label={t("remoteplay.register.pin")} help={t("remoteplay.register.pinHelp")} context={form}
                            props={{ placeholder: t("remoteplay.register.pinPlaceholder") }} />
                        <SubmitRow>
                            <FormButton id="rp-register" label={t("remoteplay.register.submit")} isReplaced={isReplaced} pending={pending} disabled={invalid} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
            <GuideSection guides={[["remotePlay", "add-your-console"]]} />
        </SettingsPage>
    );
}
