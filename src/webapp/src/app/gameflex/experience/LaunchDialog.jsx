import {useDialogFocus} from './dialog-focus.js';
import React, { useRef, useState } from 'react';
import {EmulatorRequirement} from './EmulatorAssistant.jsx';
import {useAuth} from '../profile/auth/AuthContext.jsx';
import { Icon } from './EmulatorUI.jsx';
export default function LaunchDialog({ game, mode, error, launching, onPlay, onClose, onEmulatorReady }) {
    const {token}=useAuth();
    const [coreReady,setCoreReady]=useState(null);
    const root = useRef(null), busy = useRef(launching), close = useRef(onClose);
    busy.current = launching; close.current = onClose;
    useDialogFocus(root);
    const onKey = e => {
        if (e.key === 'Escape') { e.stopPropagation(); if (!busy.current) close.current(); return; }
        if (!['Tab','ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key)) return;
        const buttons = [...root.current.querySelectorAll('button:not(:disabled)')];
        e.preventDefault(); e.stopPropagation();
        if (!buttons.length) return;
        const index = buttons.indexOf(document.activeElement), back = e.shiftKey || ['ArrowLeft','ArrowUp'].includes(e.key);
        buttons[(index + (back ? -1 : 1) + buttons.length) % buttons.length].focus();
    };
    return <div className="gx-dialog-backdrop"><section ref={root} onKeyDown={onKey} role="dialog" aria-modal="true" aria-labelledby="em-launch-title" className={`gx-launch-dialog gx-${mode}`}><small>GAMEFLEX / {game.consoleName}</small><h2 id="em-launch-title">{game.title}</h2>{error && <p role="alert">{error}</p>}<EmulatorRequirement consoleId={game.consoleId} token={token} onReady={onEmulatorReady} onStatus={setCoreReady}/><div className="em-dialog-actions"><button disabled={launching||coreReady===false} onClick={onPlay}><Icon name="play" size={17}/>{launching ? 'Lancement…' : 'Jouer'}</button><button disabled={launching} onClick={onClose}>Retour</button></div></section></div>;
}
