// Route : /settings/consoles/<console>/bios/<nom> — un BIOS (RequiredPage).
import React from "react";
import { RequiredPage } from "../Required.jsx";

export default function BiosFile() {
    return <RequiredPage key="bios" section="bios" />;
}
