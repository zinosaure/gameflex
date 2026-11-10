import React, {useRef, useState} from 'react';
import {destinations, Icon} from './EmulatorUI.jsx';
import {useDialogFocus} from './dialog-focus.js';

function MenuDialog({onClose,onNavigate,onSettings,onAppearance}) {
    const root=useRef(null);useDialogFocus(root);
    const activate=action=>{onClose();action?.();};
    const key=e=>{
        if(e.key==='Escape'){e.preventDefault();e.stopPropagation();onClose();return;}
        if(!['Tab','ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key))return;
        e.preventDefault();e.stopPropagation();
        const nodes=[...root.current.querySelectorAll('button:not(:disabled)')],index=nodes.indexOf(document.activeElement);
        const back=e.shiftKey||['ArrowLeft','ArrowUp'].includes(e.key);
        nodes[(index+(back?-1:1)+nodes.length)%nodes.length]?.focus();
    };
    return <div className="gf-menu-backdrop" onClick={onClose}><section ref={root} role="dialog" aria-modal="true" aria-label="Menu principal" className="gf-interface-menu" onKeyDown={key} onClick={e=>e.stopPropagation()}><header><strong>Menu principal</strong><button onClick={onClose} aria-label="Fermer le menu">×</button></header><nav>{destinations.map(([id,label])=><button key={id} onClick={()=>activate(()=>onNavigate?.(id))}>{label}</button>)}<button onClick={()=>activate(onSettings)}>Paramètres</button><button onClick={()=>activate(onAppearance)}>Changer d’interface</button></nav></section></div>;
}
export default function InterfaceMenu({onNavigate,onSettings,onAppearance,label='Menu principal',className='',children}) {
    const [open,setOpen]=useState(false);
    return <><button className={className} data-focus-key="interface-menu" aria-haspopup="dialog" aria-expanded={open} onClick={()=>setOpen(true)}>{children||<><Icon name="more"/><span>{label}</span></>}</button>{open&&<MenuDialog onClose={()=>setOpen(false)} onNavigate={onNavigate} onSettings={onSettings} onAppearance={onAppearance}/>}</>;
}
