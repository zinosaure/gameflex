// Route : /settings/consoles/<console>/games/<source>/match?ref= — ce n'est pas le bon jeu (SheetPage, ../Games.jsx).
import React from "react";
import { GameMatch } from "../../server/consoles/GameMetadata.jsx";
import { SheetPage } from "../Games.jsx";

export default function Match() {
    return <SheetPage key="match" titleKey="gameAdmin.wrongGame" part={GameMatch} />;
}
