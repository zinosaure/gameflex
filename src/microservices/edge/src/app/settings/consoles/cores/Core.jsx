// Route : /settings/consoles/<console>/cores/<nom> — un émulateur (RequiredPage).
import React from "react";
import { RequiredPage } from "../Required.jsx";

export default function Core() {
    return <RequiredPage key="cores" section="cores" />;
}
