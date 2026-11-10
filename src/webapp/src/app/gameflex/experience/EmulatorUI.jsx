import { appPath } from "../libs/appBase.js";
import React, { useEffect, useRef, useState } from 'react';

const paths = {
    home: <><path d="m3 10 9-7 9 7v10H3z"/><path d="M9 20v-7h6v7"/></>,
    consoles: <><rect x="3" y="7" width="18" height="11" rx="3"/><path d="M7 10v5m-2-2h4m7-2h.01m2 3h.01M7 4h10"/></>,
    library: <><path d="M4 4h5v16H4zm7 0h4v16h-4zm7 1 3 14-4 1-3-14"/></>,
    favorites: <path d="M20 5c-3-3-7-1-8 1-1-2-5-4-8-1-5 5 2 10 8 15 6-5 13-10 8-15Z"/>,
    add: <path d="M12 4v16M4 12h16"/>,
    refresh: <path d="M20 7a9 9 0 1 0 1 8M20 3v5h-5"/>,
    resume: <><path d="M4 8a9 9 0 1 1-1 7M4 3v5h5"/><path d="M12 7v5l3 2"/></>,
    collections: <><rect x="3" y="7" width="18" height="14" rx="1"/><path d="M6 3h12M4 5h16m-7 6h5m-5 4h5M7 11v6"/></>,
    archives: <><path d="M3 6h7l2 2h9v12H3zM3 6V3h7l2 2h8"/><path d="M7 12h10m-10 4h7"/></>,
    multiplayer: <><circle cx="8" cy="7" r="3"/><circle cx="17" cy="8" r="2.5"/><path d="M2 21v-3a6 6 0 0 1 12 0v3m1-7a5 5 0 0 1 7 4v3"/></>,
    settings: <><path d="m9 3 1-1h4l1 3 3 1 3-1 2 4-2 2v3l1 2-2 4-3-1-3 1-1 2H9l-1-3-3-1-2 1-2-4 2-2v-3L1 8l2-4 3 1z" transform="translate(1 1) scale(.9)"/><circle cx="12" cy="12" r="3"/></>,
    more: <><circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/></>,
    play: <path d="m8 4 13 8-13 8z"/>,
    arrow: <path d="m9 5 7 7-7 7"/>,
    back: <path d="m14 5-7 7 7 7M7 12h14"/>,
    close: <path d="m6 6 12 12M18 6 6 18"/>,
    search: <><circle cx="10" cy="10" r="6"/><path d="m15 15 6 6"/></>,
    external: <><path d="M14 3h7v7m0-7L10 14M10 3H3v18h18v-7"/></>,
    palette: <><path d="M21 11c0 6-5 3-6 5-1 1 2 3-1 5C3 23-2 7 7 3c7-3 14 1 14 8Z"/><path d="M7 8h.01M12 5h.01M17 8h.01M5 13h.01"/></>,
};
export function Icon({ name, size = 21 }) {
    return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">{paths[name] || paths.consoles}</svg>;
}
export const destinations = [['home','Accueil'],['consoles','Consoles'],['library','Mes jeux'],['favorites','Favoris'],['resume','Récents'],['collections','Collections'],['management','Gestion / Archives'],['multiplayer','Multijoueur']];
const shellNames = { minimal:'toolbar', covers:'gallery-rail', consoles:'station', favorites:'pinned-rail', resume:'xmb', collections:'ribbon', detail:'cinema', archives:'explorer', multiplayer:'cabinet', dashboard:'metro' };

export function EmulatorNavigation({ mode, active, onNavigate, onSettings, onAppearance, user }) {
    const [more, setMore] = useState(false);
    const moreRef = useRef(null), first = useRef(null);
    const go = id => { setMore(false); onNavigate(id); };
    useEffect(() => {
        if (!more) return undefined;
        const previous = document.activeElement; first.current?.focus();
        const onKey = e => {
            if (e.key === 'Escape') { setMore(false); return; }
            if (e.key !== 'Tab') return;
            const buttons = [...moreRef.current.querySelectorAll('button')];
            if (e.shiftKey && document.activeElement === buttons[0]) { e.preventDefault(); buttons.at(-1).focus(); }
            if (!e.shiftKey && document.activeElement === buttons.at(-1)) { e.preventDefault(); buttons[0].focus(); }
        };
        document.addEventListener('keydown', onKey);
        return () => { document.removeEventListener('keydown', onKey); previous?.focus(); };
    }, [more]);
    const button = ([id, label], compact = false) => <button key={id} title={label} aria-label={label} aria-current={active === id ? 'page' : undefined} onClick={() => go(id)}><Icon name={id}/><span className={compact ? 'em-nav-label' : ''}>{label}</span></button>;
    const brand = <button className="em-brand" onClick={() => go('home')} aria-label="GameFlex · Accueil"><img className="em-brand-icon" src={appPath("/images/icon-192.png")} alt="" width="36" height="36"/><b>Game<span>Flex</span></b></button>;
    return <>
        <aside className={`em-desktop-nav em-${shellNames[mode] || 'station'}`} aria-label="Navigation principale">
            {brand}<nav>{destinations.map(x => button(x))}</nav><div className="em-nav-tools"><button className="em-desktop-more" aria-label="Tous les menus" aria-expanded={more} onClick={() => setMore(x => !x)}><Icon name="more"/><span>Plus</span></button><button onClick={onAppearance || onSettings} title="Changer d’interface" aria-label="Changer d’interface"><Icon name="palette"/><span>Interface</span></button><button onClick={onSettings} title="Paramètres" aria-label="Paramètres"><Icon name="settings"/><span>Paramètres</span></button></div><small className="em-profile">{user}</small>
        </aside>
        <div className="em-mobile-masthead">{brand}<button className="em-mobile-settings" onClick={onSettings} aria-label="Paramètres"><Icon name="settings"/></button></div>
        <nav className="em-dock" aria-label="Navigation mobile">{destinations.slice(0,4).map(x => button(x,true))}<button aria-current={!destinations.slice(0,4).some(([id]) => id === active) ? "page" : undefined} title="Plus" aria-label="Plus de menus" aria-expanded={more} onClick={() => setMore(x => !x)}><Icon name="more"/><span className="em-nav-label">Plus</span></button></nav>
        {more && <div className="em-drawer-backdrop" onClick={() => setMore(false)}><section ref={moreRef} role="dialog" aria-modal="true" aria-label="Tous les menus" className="em-drawer" onClick={e => e.stopPropagation()}><header><b>GameFlex</b><button ref={first} onClick={() => setMore(false)} aria-label="Fermer les menus"><Icon name="close"/></button></header><nav>{destinations.map(x => button(x))}<button onClick={() => { setMore(false); (onAppearance || onSettings)(); }}><Icon name="palette"/><span>Interface</span></button><button onClick={() => { setMore(false); onSettings(); }}><Icon name="settings"/><span>Paramètres</span></button></nav></section></div>}
    </>;
}

export function LayoutMiniature({ mode }) {
    return <span className={`em-mini em-mini-${mode}`} aria-hidden="true"><i className="em-mini-nav"/><i className="em-mini-title"/><span className="em-mini-body">{Array.from({length:6},(_,i) => <i key={i}/>)}</span></span>;
}

function Activate({ item, onOpen, children, className, onFocus }) {
    return <button data-focus-key={identity(item)} disabled={item.pendingDownload&&!item.reference} className={className} onClick={() => onOpen(item)} onFocus={onFocus}>{children}</button>;
}
const identity = g => `${g.consoleId || ''}:${g.reference || g.source || g.title}`;
export function GameBrowser({ mode, items = [], onOpen, Art, onFavorite, favorites = [], collections = [], onAddToCollection, consoleName = '', label = 'Jeux' }) {
    const [selected, setSelected] = useState(()=>{try{return mode==='covers'?sessionStorage.getItem('gameflex:covers-focus'):null;}catch{return null;}});
    const item = items.find(g => identity(g) === selected) || items[0];
    if (!items.length) return <div className="em-no-games"><Icon name="library" size={30}/><span>Aucun jeu ici</span></div>;
    const select = g => { const value=identity(g); setSelected(value); if(mode==='covers')try{sessionStorage.setItem('gameflex:covers-focus',value);}catch{} };
    const isFav = g => favorites.includes(`${g.consoleId}:${g.reference}`);
    const favorite = g => onFavorite && g.reference ? <button className="em-favorite" aria-label={`Favori : ${g.title}`} aria-pressed={isFav(g)} onClick={() => onFavorite(g)}><Icon name="favorites" size={17}/></button> : null;
    const action = g => <div className="em-action-row"><Activate item={g} onOpen={onOpen} className="em-launch"><Icon name={g.source && !g.reference ? 'external' : 'play'} size={16}/>Voir la fiche</Activate>{favorite(g)}</div>;
    const collectionActions = g => g.reference && onAddToCollection && collections.length > 0 ? <div className="em-collection-actions">{collections.map(c => <button key={c.id} onClick={() => onAddToCollection(c.id,g)}>{c.games.includes(`${g.consoleId}:${g.reference}`) ? '✓' : '+'} {c.name}</button>)}</div> : null;
    const poster = (g, cls = '') => <div className="em-poster-cell" key={identity(g)}><Activate className={`em-poster ${cls}`} item={g} onOpen={onOpen} onFocus={()=>select(g)}><Art item={g}/><span><strong>{g.title}</strong>{g.consoleName && <small>{g.consoleName}</small>}{g.reference&&<small>Installé</small>}{g.variants?.length>1&&<small>{g.variants.length} éditions</small>}{g.downloadId && <small>{g.pendingDownload?'Import en cours':'Catalogue'}{g.license?` · ${g.license}`:''}</small>}</span></Activate>{favorite(g)}</div>;
    const row = (g,i) => <li key={identity(g)} className={item===g?'is-selected':''}><span className="em-row-number">{String(i+1).padStart(2,'0')}</span><Activate className="em-list-game" item={g} onOpen={onOpen} onFocus={()=>select(g)}><Art item={g}/><span className="em-minimal-title"><strong>{g.title}</strong><small>{g.consoleName || consoleName}</small></span><span className="em-minimal-console">{g.consoleName || consoleName || '—'}</span><span className="em-minimal-year">{g.year || '—'}</span><span className="em-minimal-status">{g.reference ? g.status || 'Installé' : g.status || 'Catalogue'}</span><Icon name={g.source && !g.reference ? "external" : "arrow"} size={16}/></Activate>{favorite(g)}</li>;
    switch (mode) {
    case 'minimal': return <ol className="em-compact-list" data-layout="compact-list">{items.map(row)}</ol>;
    case 'covers': return <div className="em-cover-wall" data-layout="cover-wall">{items.map(g => poster(g))}</div>;
    case 'consoles': return <div className="em-cartridge-shelf" data-layout="cartridge-shelf">{items.map((g,i) => <article key={identity(g)}><span className="em-cartridge-index">{String(i+1).padStart(2,'0')}</span>{poster(g)}</article>)}</div>;
    case 'favorites': return <div className="em-pinned" data-layout="pinned-showcase"><section className="em-featured-pin"><small>EN VITRINE</small><Art item={item}/><div><h3>{item.title}</h3>{action(item)}</div></section><div className="em-pin-wall">{items.map(g => <article key={identity(g)}>{poster(g)}</article>)}</div></div>;
    case 'resume': return <div className="em-xmb-browser" data-layout="xmb-browser"><div className="em-xmb-spine"><Icon name="consoles" size={34}/><span>{label}</span><i/></div><div className="em-xmb-items">{items.map(g => <button key={identity(g)} aria-pressed={item === g} onFocus={() => select(g)} onClick={() => select(g)}><Icon name={g.reference ? 'play' : 'library'}/><span>{g.title}</span></button>)}</div><aside className="em-xmb-preview"><Art item={item}/><h3>{item.title}</h3>{action(item)}{collectionActions(item)}</aside></div>;
    case 'collections': {
        const groups = new Map(); items.forEach(g => { const group = /\(Japan/.test(g.title) ? 'Éditions japonaises' : /\(USA/.test(g.title) ? 'Éditions américaines' : /\(Europe/.test(g.title) ? 'Éditions européennes' : g.consoleName || 'La collection'; groups.set(group,[...(groups.get(group)||[]),g]); });
        return <div className="em-record-shelves" data-layout="record-shelves">{[...groups].map(([name,gs]) => <section key={name}><h3><Icon name="collections" size={18}/>{name}<small>{gs.length}</small></h3><div className="em-record-row">{gs.map(g => poster(g))}</div></section>)}</div>;
    }
    case 'detail': return <div className="em-cinema-browser" data-layout="cinema-browser"><div className="em-cinema-backdrop" aria-hidden="true"><Art item={item}/></div><div className="em-cinema-focus"><Art item={item}/><div><small>{item.consoleName || consoleName || label}</small><h2>{item.title}</h2>{action(item)}{collectionActions(item)}</div></div><div className="em-cinema-filmstrip" aria-label="Sélection du jeu">{items.map(g => <button key={identity(g)} aria-pressed={item === g} aria-label={g.title} onFocus={() => select(g)} onClick={() => select(g)}><Art item={g}/></button>)}</div></div>;
    case 'archives': return <div className="em-file-browser" data-layout="file-browser"><div className="em-file-table"><div className="em-file-heading"><span>Nom</span><span>État</span></div>{items.map((g,i) => <button key={identity(g)} aria-pressed={item === g} onFocus={() => select(g)} onClick={() => select(g)}><span><Icon name="library" size={16}/>{g.title}</span><small>{g.source && !g.reference ? 'Catalogue' : g.status || 'Local'}</small><span className="em-file-id">{String(i+1).padStart(3,'0')}</span></button>)}</div><aside className="em-file-preview"><small>APERÇU</small><Art item={item}/><h3>{item.title}</h3>{action(item)}{collectionActions(item)}</aside></div>;
    case 'multiplayer': return <div className="em-arcade-grid" data-layout="arcade-select">{items.map((g,i) => <article key={identity(g)}><span className="em-arcade-index">{String(i+1).padStart(2,'0')}</span>{poster(g)}<div className="em-arcade-command"><span>{g.reference ? 'PRÊT À JOUER' : 'CATALOGUE'}</span><Icon name={g.reference ? 'play' : 'external'} size={15}/></div></article>)}</div>;
    default: return <div className="em-metro-grid" data-layout="metro-grid">{items.map((g,i) => <article className={i%7 === 0 ? 'em-tile-wide' : ''} key={identity(g)}>{poster(g)}<span className="em-tile-order">{String(i+1).padStart(2,'0')}</span></article>)}</div>;
    }
}

export function SystemBrowser({ mode, consoles = [], games = [], Art, onConsole }) {
    const [chosen,setChosen] = useState(null);
    const stripRef=useRef(null);
    const current = consoles.find(c => c.id === chosen) || consoles[0];
    const count = c => games.filter(g => g.consoleId === c.id).length;
    const move=(step,from=current)=>{const index=Math.max(0,consoles.findIndex(c=>c.id===from?.id)),next=consoles[(index+step+consoles.length)%consoles.length];if(!next)return;setChosen(next.id);requestAnimationFrame(()=>{const node=stripRef.current?.querySelector(`[data-system="${next.id}"]`);node?.focus();node?.scrollIntoView({block:'nearest',inline:'center'});});};
    const stripBody = <div ref={stripRef} className="em-system-strip">{consoles.map(c => <button data-system={c.id} key={c.id} aria-pressed={c === current} onClick={() => setChosen(c.id)} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); onConsole(c); } else if(e.key==='ArrowLeft'||e.key==='ArrowRight'){e.preventDefault();e.stopPropagation();move(e.key==='ArrowRight'?1:-1,c);} }} onFocus={() => setChosen(c.id)}>{mode==='consoles'&&<span className="em-system-thumb"><Art item={c} kind="console"/></span>}<span>{c.publisher || c.year}</span><strong>{c.name}</strong></button>)}</div>;
    const strip = mode==='consoles'?<div className="em-system-rail"><button className="em-system-prev" aria-label="Console précédente" onClick={()=>move(-1)}><Icon name="back"/></button>{stripBody}<button className="em-system-next" aria-label="Console suivante" onClick={()=>move(1)}><Icon name="arrow"/></button></div>:stripBody;
    if (!current) return <p>Aucune console disponible.</p>;
    if (mode === 'consoles' || mode === 'resume') return <div className={`em-station em-station-${mode}`} data-layout={mode === 'resume' ? 'system-xmb' : 'system-carousel'}>{strip}<section className="em-system-stage"><div className="em-system-wordmark" aria-hidden="true">{current.publisher || 'RETRO'}</div><div className="em-system-object"><Art item={current} kind="console" wide/></div><div className="em-system-description"><small>{current.year} / {current.publisher}</small><h2>{current.name}</h2><p>{count(current)} jeu{count(current) !== 1 ? 'x' : ''} installé{count(current) !== 1 ? 's' : ''}</p><button className="em-launch" onClick={() => onConsole(current)}>Explorer <Icon name="arrow" size={17}/></button></div></section><div className="em-station-controls"><span>← → Choisir un système</span><span>Entrée · Explorer</span></div></div>;
    if (mode === 'minimal' || mode === 'archives') return <div className="em-system-ledger" data-layout="system-ledger">{consoles.map((c,i) => <button key={c.id} onClick={() => onConsole(c)}><small>{String(i+1).padStart(2,'0')}</small><Art item={c} kind="console"/><strong>{c.name}</strong><span>{c.year}</span><Icon name="arrow" size={16}/></button>)}</div>;
    return <div className={`em-machine-grid em-machine-${mode}`} data-layout={`machine-${mode}`}>{consoles.map(c => <button key={c.id} onClick={() => onConsole(c)}><Art item={c} kind="console" wide/><div><small>{c.publisher || c.year}</small><strong>{c.name}</strong><span>{c.year}<Icon name="arrow" size={15}/></span></div></button>)}</div>;
}

export function ConsoleIdentity({ console: c, count, Art, mode }) {
    return <section className={`em-console-identity em-console-${mode}`}><div className="em-console-object"><Art item={c} kind="console" wide/></div><div><small>{c.publisher || 'SYSTÈME'} / {c.year}</small><h2>{c.name}</h2><span>{count == null ? 'Chargement…' : `${count} jeu${count === 1 ? '' : 'x'} installé${count === 1 ? '' : 's'}`}</span></div><span className="em-console-code" aria-hidden="true">{c.id.toUpperCase()}</span></section>;
}
