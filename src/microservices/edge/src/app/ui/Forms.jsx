import React, { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { strengthTests } from "../libs/password.js";
import { useButton } from "../libs/ui.js";
import { Form, Ruleset } from "../skeleton/libs/Form.jsx";
import { Table } from "../skeleton/libs/Table.jsx";
import { usePagination } from "../skeleton/libs/Pagination.jsx";

// LES FORMULAIRES, LES TABLEAUX ET LA PAGINATION DU LAUNCHER : CEUX DE SKELETON (le porteur, 2026-11-13 :
// « pour les forms, les tables et la pagination, utilise les libs de Skeleton. Seulement ces trois. Sinon
// chaque form a son propre style et on se perd. Tout doit rester pareil. »). Ses libs sont COPIÉES dans le
// launcher (app/skeleton/libs, chantier 53 P1 ; le porteur, 2026-11-13 : « Du coup faudra copier la lib depuis
// skeleton ») ; le launcher les importe d'ici seulement, avec ce qu'il y ajoute :
//
//   formContext — le contexte d'un formulaire, formé comme la webapp le forme (profile/settings/Password.jsx) ;
//   fieldContext — celui d'un champ seul qui applique ce qu'on choisit (un interrupteur, des cases) ;
//   FormButton — le bouton d'un formulaire, celui de la webapp, le même partout ;
//   NewPassword — un mot de passe qu'une personne choisit, avec la jauge de Skeleton et la règle (chantier 55) ;
//   PadReach — la manette jusqu'aux contrôles que Skeleton dessine lui-même ; useCalendarDays, jusqu'aux jours de ses
//   calendriers.
//
// Un champ va toujours dans un `Form.Group` qui porte son libellé ET son aide (CODE_CONVENTIONS.md). Un
// interrupteur est `Form.Switch` — l'interrupteur de Bootstrap : celui de Skeleton (SkeletonSwitch) demande
// la feuille de styles de Skeleton entière, qui changerait le reste du launcher.
export { Form, Ruleset, Table, usePagination };

export const formContext = (values, setValues, rulesets = {}) => ({
    values,
    setValues: (patch) => setValues((previous) => ({ ...previous, ...patch })),
    files: {},
    setFiles: () => {},
    rulesets,
});

// UN CHAMP SEUL QUI APPLIQUE CE QU'ON CHOISIT (un sélecteur, un interrupteur, des cases d'option) : son
// contexte d'un seul nom, sans formulaire autour — la valeur vient de l'appelant, le choix lui revient.
export const fieldContext = (name, value, apply, disabled = false) => ({
    values: { [name]: value },
    setValues: (next) => apply(next[name]),
    files: {},
    setFiles: () => {},
    rulesets: disabled ? { [name]: () => Ruleset.isDisabled(null) } : {},
});

// LE BOUTON D'UN FORMULAIRE, celui de la webapp (FormButton, gameflex/shared/templates/Essentials.jsx) — le porteur,
// 2026-11-14 : « il faut garder le système de isReplaced comme sur webapp. Si un formulaire n'est pas modifié ou qu'il
// est isDisabled ou not isValid alors le bouton affiche par défaut "..." ». Au repos — rien de changé (`isReplaced`
// faux), un champ invalide ou manquant (`disabled`), l'envoi en cours (`pending`) — il est grisé, en danger, et dit
// « ••• » ; sinon son libellé, dans la sorte du formulaire (`kind` : main, other, danger ; libs/ui.js, selon le lieu :
// primary dans les Réglages, success sur les écrans).
//
// `isReplaced` N'A PAS DE DÉFAUT : chaque formulaire compare ce qu'il montre à ce qui est enregistré
// (CODE_CONVENTIONS.md, « Bouton Enregistrer ») — un formulaire qui l'oublie garde son bouton au repos, il ne ment pas.
// Un formulaire qui ne remplace rien mais fait un geste (détecter, rechercher à nouveau, activer un jeton, quitter) n'a
// rien à comparer : il passe `isReplaced` tel quel.
//
// LA LIGNE AU-DESSUS (le porteur, 2026-11-14 : « Tu peux aussi rajouter un hr avec chaque bouton d'un formulaire
// (seulement). ») vit ICI, et nulle part ailleurs : aucun formulaire ne l'oublie, et rien d'autre n'en a — ni un bouton
// hors d'un formulaire, ni les lignes de « Plus » ou de « Danger ». Deux boutons du même formulaire, l'un sous l'autre,
// n'en ont qu'une, au-dessus du premier (launcher.css, `.gf-form-button`). Il se pose DANS l'encadré du groupe qu'il
// envoie (SubmitRow, Bricks.jsx), après son dernier contrôle.
export function FormButton({ id, label, isReplaced, pending = false, disabled = false, kind = "main", type = "submit", onClick = undefined }) {
    const button = useButton();
    const idle = pending || disabled || !isReplaced;

    return (
        <div className="gf-form-button d-grid border-top pt-3">
            <button type={type} className={button(idle ? "danger" : kind)} data-nav data-key={id} disabled={idle} onClick={onClick}>
                {idle ? "•••" : label}
            </button>
        </div>
    );
}

// UN MOT DE PASSE QU'UNE PERSONNE CHOISIT (chantier 55 ; le porteur, 2026-11-14 : « Normalement, skeleton.Password
// avec PasswordStrength gère ça. Regarde comment l'utiliser. ») : le champ de Skeleton, puis sa jauge
// (`Form.Password.StrengthEvaluation`) avec les tests de la règle (libs/password.js), dits dans la langue de la page — la
// même que le service des comptes. `withStrengthEvaluation` ne transmet pas `tests` à la jauge : elle se pose donc ici,
// sous le champ, comme Form.Password la poserait ; la lib reste telle quelle. La page du lien (Join.jsx) et « Mon mot de
// passe » (TextRow, `kind="new-password"`).
export function NewPassword({ name, context, props = {} }) {
    const { t } = useTranslation();

    return (
        <div>
            <Form.Password name={name} context={context} props={props} />
            <Form.Password.StrengthEvaluation name={`${name}_strength`} following={name} context={context} tests={strengthTests(t)} />
        </div>
    );
}

// LA MANETTE JUSQU'AUX CONTRÔLES DE SKELETON. La croix ne s'arrête que sur `data-nav` (libs/input.js) ;
// Skeleton passe nos `props` au champ de ses Text, Password, Select, Switch…, mais pas aux contrôles qu'il
// dessine seul — les boutons de sa pagination, son choix de fichier, les cases d'un groupe. Ils reçoivent
// `data-nav` ici, dès qu'ils paraissent ; un contrôle sorti exprès de la tabulation (tabindex -1) en reste
// dehors. Une page des réglages l'est tout entière (SettingsPage.jsx) ; ailleurs, on enveloppe le formulaire.
const REACHABLE = "button, a[href], input:not([type=hidden]), select, textarea";

export function usePadReach(ref) {
    useEffect(() => {
        const root = ref.current;

        if (!root) {
            return undefined;
        }
        const mark = () => root.querySelectorAll(REACHABLE).forEach((element) => {
            if (!element.hasAttribute("data-nav") && element.getAttribute("tabindex") !== "-1") {
                element.setAttribute("data-nav", "");
            }
        });
        mark();
        const observer = new MutationObserver(mark);
        observer.observe(root, { childList: true, subtree: true });

        return () => observer.disconnect();
    }, [ref]);
}

export function PadReach({ className = "", children }) {
    const ref = useRef(null);
    usePadReach(ref);

    return <div ref={ref} className={className}>{children}</div>;
}

// LES JOURS D'UN CALENDRIER DE SKELETON (Form.Date, Form.DateRange) JUSQU'À LA MANETTE : des `div` que Skeleton rend
// touchables (`cursor: pointer`), et que PadReach ne voit donc pas — marqués ici pour la croix (`data-nav`,
// `role="button"` : A les touche, libs/input.js), démarqués quand une borne les grise. `ref` : ce qui contient le
// calendrier, toujours là — un calendrier qui paraît plus tard (caché par un Ruleset) se marque à son arrivée. Les
// bornes du journal (settings/server/Logs.jsx) et la sortie d'un jeu (settings/server/consoles/GameMetadata.jsx).
export function useCalendarDays(ref) {
    useEffect(() => {
        const root = ref.current;

        if (!root) {
            return undefined;
        }
        const mark = () => root.querySelectorAll(".flex-column.gap-1 > div > div").forEach((cell) => {
            const pickable = cell.style.cursor === "pointer";

            if (pickable && !cell.hasAttribute("data-nav")) {
                cell.setAttribute("data-nav", "");
                cell.setAttribute("role", "button");
                cell.tabIndex = 0;
            } else if (!pickable && cell.hasAttribute("data-nav")) {
                cell.removeAttribute("data-nav");
                cell.removeAttribute("role");
                cell.removeAttribute("tabindex");
            }
        });
        mark();
        const observer = new MutationObserver(mark);
        observer.observe(root, { childList: true, subtree: true, attributes: true, attributeFilter: ["style"] });

        return () => observer.disconnect();
    }, [ref]);
}
