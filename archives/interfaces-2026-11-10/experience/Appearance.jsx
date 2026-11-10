import React, {useRef,useEffect} from 'react';
import {useDirectionalFocus} from './ExperienceView.jsx';
import InterfaceMenu from './InterfaceMenu.jsx';
import SectionErrorBoundary from './SectionErrorBoundary.jsx';
import './catalog-game-dialog.css';
import './appearance.css';

export function AppearanceSurface({mode,children}) {
    useEffect(()=>{
        const body=document.body, className=`gf-theme-${mode}`, previous=body.getAttribute('data-gameflex-appearance');
        const existing=body.classList.contains(className);
        body.classList.add(className);body.setAttribute('data-gameflex-appearance',mode);
        return()=>{if(!existing)body.classList.remove(className);if(previous===null)body.removeAttribute('data-gameflex-appearance');else body.setAttribute('data-gameflex-appearance',previous);};
    },[mode]);
    return <div className={`gf-theme-${mode} gf-appearance`} data-appearance={mode}>{children}</div>;
}
export function HomeShell({mode,loading,error,games,onRetry,onNavigate,onSettings,onAppearance,children}) {
    const root=useRef(null);useDirectionalFocus(root);
    return <div ref={root} className="gf-home-shell">
        {<div className="gf-interface-tools"><InterfaceMenu onNavigate={onNavigate} onSettings={onSettings} onAppearance={onAppearance}/></div>}
        <SectionErrorBoundary resetKey={`${mode}:${loading}`} onRetry={onRetry}>
            {loading&&<p className="gf-load-status" role="status">Chargement de la bibliothèque…</p>}
            {error&&<div className="gf-load-status" role="alert">{error} <button onClick={onRetry}>Réessayer</button></div>}
            {(!loading||games.length>0)&&!(error&&!games.length)&&children}
        </SectionErrorBoundary>
    </div>;
}
