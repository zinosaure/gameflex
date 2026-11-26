// Route : /settings/members/new — un nouveau membre : le formulaire d'un membre, vide (MemberForm, Member.jsx).
import React from "react";
import { MemberForm } from "./Member.jsx";

export default function NewMember() {
    return <MemberForm member={null} />;
}
