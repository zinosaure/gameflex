// LES SAUVEGARDES D'UNE CONSOLE RÉCENTE, jeu par jeu (chantier 38).
//
// Chaque émulateur range un dossier par jeu, nommé par son identifiant
// (`CUSA07010`, `BLES00635`…) : c'est l'unité qu'on copie et qu'on restaure —
// « on remplace ou copie le jeu de la console seulement. Pas tous les jeux »
// (le porteur, 2026-10-29). La ligne du haut porte la console entière.
//
// La même forme que les emplacements du rétro (SaveStatesPanel) : on touche
// une ligne, ses gestes s'ouvrent dessous.
import React, { useEffect, useState } from "react";
import { ListElement, SectionPanel, AsyncSectionPanel } from "../../../shared/templates/Essentials.jsx";
import { listStreamexSaves } from "../../../api/retrogaming.js";
import { formatSize } from "../../../games/helpers.js";
import { SaveActions } from "./SaveActions.jsx";

// Le nom d'un jeu : son titre quand la sauvegarde le dit, sinon son
// identifiant ; la Xbox n'a que son disque.
export function streamexGameName(game, t) {
    return game.console === "xbox" ? t("profile.saveStates.xboxDisk") : (game.title || game.id);
}

export function StreamexSaves({ where, consoleId, consoleName, peers, selfName, t, context, onChanged = null }) {
    const [games, setGames] = useState(null);
    // « all », ou l'identifiant du jeu dont les gestes sont ouverts.
    const [selected, setSelected] = useState(null);

    const refresh = () => {
        listStreamexSaves(where.source, where.host).then((answer) => {
            setGames(answer.ok ? (answer.data?.games || []).filter((game) => game.console === consoleId) : []);
        });
    };
    useEffect(refresh, []);

    if (games === null || games.length === 0) {
        return <AsyncSectionPanel isLoading={games === null} isEmpty={true} emptyText={t("profile.saveStates.empty")} />;
    }

    const actions = (chosen, label) => (
        <div className="pt-2 mt-2 border-top">
            <SaveActions
                where={where}
                peers={peers}
                selfName={selfName}
                games={chosen}
                label={label}
                canDelete={where.kind === "copy"}
                t={t}
                context={context}
                onDone={() => {
                    refresh();
                    if (onChanged) {
                        onChanged();
                    }
                }} />
        </div>
    );

    return (
        <SectionPanel>
            {games.length > 1 ? (
                <ListElement
                    light={true}
                    icon="bi bi-collection"
                    label={t("profile.saveStates.wholeConsole", { name: consoleName })}
                    helpText={t("profile.saveStates.gameCount", { count: games.length })}
                    onClick={() => setSelected("all")}
                >
                    {selected === "all" ? actions(games, consoleName) : null}
                </ListElement>
            ) : null}
            {games.map((game) => (
                <ListElement
                    key={game.id}
                    icon="bi bi-shield-check"
                    label={streamexGameName(game, t)}
                    helpText={[
                        game.console === "xbox" ? t("profile.saveStates.xboxDiskHelp") : (game.title ? game.id : null),
                        new Date(game.updatedAt).toLocaleString(),
                        formatSize(game.sizeBytes),
                    ].filter(Boolean).join(" · ")}
                    onClick={() => setSelected(game.id)}
                >
                    {selected === game.id ? actions([game], streamexGameName(game, t)) : null}
                </ListElement>
            ))}
        </SectionPanel>
    );
}
