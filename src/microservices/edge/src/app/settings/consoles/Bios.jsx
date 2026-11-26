// Route : /settings/consoles/<console>/bios — ses BIOS (ItemsPage).
import React from "react";
import { ItemsPage } from "../Consoles.jsx";

export default function Bios() {
    return <ItemsPage key="bios" list="bios" />;
}
