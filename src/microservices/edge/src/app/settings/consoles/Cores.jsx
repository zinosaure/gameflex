// Route : /settings/consoles/<console>/cores — ses émulateurs (ceux de ce serveur, les cœurs du rétro) (ItemsPage).
import React from "react";
import { ItemsPage } from "../Consoles.jsx";

export default function Cores() {
    return <ItemsPage key="cores" list="cores" />;
}
