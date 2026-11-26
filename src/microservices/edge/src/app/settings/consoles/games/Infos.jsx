// Route : /settings/consoles/<console>/games/<source>/infos?ref= — corriger ses informations (SheetPage, ../Games.jsx).
import React from "react";
import { GameInfoForm } from "../../server/consoles/GameMetadata.jsx";
import { SheetPage } from "../Games.jsx";

export default function Infos() {
    return <SheetPage key="infos" titleKey="gameAdmin.fixInfos" part={GameInfoForm} />;
}
