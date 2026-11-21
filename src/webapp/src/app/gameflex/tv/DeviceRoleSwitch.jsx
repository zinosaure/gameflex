import React from 'react';
export default function DeviceRoleSwitch({role,onScreen,onController,disabled=false}){
 return <nav className="gf-device-roles" aria-label="Rôle de cet appareil"><button type="button" aria-pressed={role==='screen'} disabled={disabled} onClick={onScreen}>Écran</button><button type="button" aria-pressed={role==='controller'} disabled={disabled} onClick={onController}>Manette</button></nav>;
}
