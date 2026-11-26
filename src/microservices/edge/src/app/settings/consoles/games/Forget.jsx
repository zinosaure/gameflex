// Route : /settings/consoles/<console>/games/<source>/forget?ref= — oublier une source (SheetPage, ../Games.jsx).
import React from "react";
import { GameForget } from "../../server/consoles/GameMetadata.jsx";
import { SheetPage } from "../Games.jsx";

export default function Forget() {
    return <SheetPage key="forget" titleKey="gameAdmin.meta.forget" part={GameForget} />;
}
