import React, {useEffect,useRef,useState} from 'react';
import {useDialogFocus} from './dialog-focus.js';
import {apiUrl} from '../libs/serverBase.js';
import {consoleIcon} from './console-icons.js';
import {selectableInterfaces} from './interfaces.js';
import {Icon,LayoutMiniature} from './EmulatorUI.jsx';
const retroModes=[];
import {DownloadOverlay} from './DownloadProgress.jsx';
import PlayStationHome from './PlayStationHome.jsx';
import './experience.css';
import './emulator-layouts.css';
export function InterfacePicker({ onChoose, selected, onCancel, options = selectableInterfaces }) {
    const [query, setQuery] = useState('');
    const choices = options.filter(x => x.name.toLocaleLowerCase('fr').includes(query.toLocaleLowerCase('fr')));
    const ref = useRef(null); useDirectionalFocus(ref); useDialogFocus(ref);
    useEffect(() => { (ref.current?.querySelector('[aria-pressed="true"]') || ref.current?.querySelector('button'))?.focus(); }, [selected]);
    return <section ref={ref} className={onCancel ? "gx-picker gx-picker-overlay" : "gx-picker"} role="dialog" aria-modal="true" onKeyDown={e=>{if(e.key==='Escape'&&onCancel){e.preventDefault();onCancel();}}} aria-label="Choix de l’interface">
        <small>GAMEFLEX</small><h1>Choisir une interface</h1>
        <p>Modifiable dans les paramètres.</p>
        <label className="gx-search">Rechercher une interface<input type="search" value={query} onChange={e => setQuery(e.target.value)}/></label>
        <p role="status">{choices.length} interface{choices.length === 1 ? '' : 's'}</p>
        <div className="gx-choices">{choices.map((x, i) => <button key={x.id} className={`gx-choice ${retroModes.includes(x.id) ? `gf-choice-${x.id}` : `gx-${x.id}`}`} aria-pressed={selected === x.id} onClick={() => onChoose(x.id)}>
            <LayoutMiniature mode={x.id}/>
            <small>{String(i + 1).padStart(2, '0')}</small><strong>{x.name}</strong><span>{x.note}</span>
        </button>)}</div>{onCancel && <button onClick={onCancel}>Retour</button>}
    </section>;
}

// TV arrows operate on the actual rendered geometry, also on touch layouts.
export function useDirectionalFocus(ref) {
    useEffect(() => {
        const root = ref.current;
        if (!root) return undefined;
        const onKey = e => {
            if(e.defaultPrevented)return;
            if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key) || ['INPUT', 'SELECT', 'TEXTAREA'].includes(e.target.tagName)) return;
            const surface = root.querySelector('[aria-modal="true"]') || root;
            const all = [...surface.querySelectorAll('button:not(:disabled),a[href],input,select,summary')].filter(el => el.getClientRects().length);
            if (!all.length) return;
            const current = document.activeElement;
            if (!all.includes(current)) { all[0].focus(); e.preventDefault(); return; }
            const a = current.getBoundingClientRect(), ax = a.x + a.width / 2, ay = a.y + a.height / 2;
            const dx = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0;
            const dy = e.key === 'ArrowUp' ? -1 : e.key === 'ArrowDown' ? 1 : 0;
            const candidates = all.filter(x => x !== current).map(el => {
                const b = el.getBoundingClientRect(), x = b.x + b.width / 2 - ax, y = b.y + b.height / 2 - ay;
                return { el, ahead: x * dx + y * dy, score: Math.hypot(x, y) + Math.abs(x * dy - y * dx) * 2 };
            }).filter(x => x.ahead > 5).sort((a, b) => a.score - b.score);
            if (candidates[0]) { candidates[0].el.focus(); candidates[0].el.scrollIntoView({ block: 'nearest', inline: 'nearest' }); e.preventDefault(); }
        };
        root.addEventListener('keydown', onKey);
        return () => root.removeEventListener('keydown', onKey);
    }, [ref]);
}

export function Art({ item, wide = false, kind = 'game' }) {
    const icon=kind==='console'?consoleIcon(item.id):null;
    return icon?<ConsoleIconArt key={icon.file} item={item} icon={icon} wide={wide}/>:<CachedArt item={item} wide={wide} kind={kind}/>;
}

function ConsoleIconArt({item,icon,wide}) {
    const [failed,setFailed]=useState(false);
    if(failed)return <CachedArt item={item} wide={wide} kind="console"/>;
    return <div className={`gx-art gx-console-art gx-console-icon ${wide?'gx-wide':''}`}>
        <img src={icon.src} width={icon.width} height={icon.height} alt={`Illustration de ${item.name||icon.name}`} onError={()=>setFailed(true)} decoding="async"/>
    </div>;
}

function CachedArt({ item, wide = false, kind = 'game' }) {
    const identity=`${item.image||''}:${item.fallbackImage||''}`;
    const [fallbackFor,setFallbackFor]=useState(null),[failedFor,setFailedFor]=useState(null),[loadedSrc,setLoadedSrc]=useState('');
    const [visible,setVisible]=useState(()=>typeof IntersectionObserver==='undefined');
    const outer=useRef(null),photo=useRef(null);
    const fallback=fallbackFor===identity&&Boolean(item.fallbackImage)&&item.fallbackImage!==item.image;
    const url=fallback?item.fallbackImage:item.image;
    const allowed=typeof url==='string'&&(/^\/experience-api\/v1\/media\/[a-f0-9]{64}$/.test(url)||/^\/experience-api\/v1\/covers\/[a-z0-9_-]+\/[A-Za-z0-9_-]{1,800}$/.test(url)||/^\/experience-api\/v1\/emugen-art\/[A-Za-z0-9_-]{1,2500}$/.test(url)||/^\/experience-api\/v1\/(?:freeroms-art\/[a-z0-9_-]+\/[a-z0-9_-]{1,200}|romspure-art\/[a-z0-9_-]+\/[a-z0-9~-]{1,301})$/.test(url));
    const requestKey=`${identity}:${url}`;
    const failed=failedFor===requestKey;
    const loaded=loadedSrc===requestKey;
    const fail=()=>{if(!fallback&&item.fallbackImage&&item.fallbackImage!==item.image)setFallbackFor(identity);else setFailedFor(requestKey);};
    useEffect(()=>{
        if(visible||typeof IntersectionObserver==='undefined')return undefined;
        const observer=new IntersectionObserver(entries=>{if(entries.some(entry=>entry.isIntersecting)){setVisible(true);observer.disconnect();}},{rootMargin:'160px'});
        if(outer.current)observer.observe(outer.current);return()=>observer.disconnect();
    },[visible]);
    useEffect(()=>{
        if(!visible||!allowed||failed||loaded)return undefined;
        if(photo.current?.complete&&photo.current.naturalWidth>0){setLoadedSrc(requestKey);return undefined;}
        const timer=setTimeout(()=>{if(photo.current?.complete&&photo.current.naturalWidth>0)setLoadedSrc(requestKey);else fail();},8000);
        return()=>clearTimeout(timer);
    },[requestKey,visible,allowed,failed,loaded]);
    const safe=allowed&&!failed;
    const credit=fallback?(item.fallbackAttribution||'Image du cache local'):(item.attribution||'Image du cache local');
    return <div ref={outer} className={`gx-art ${wide?'gx-wide':''} ${kind==='console'?'gx-console-art':''} ${!safe?'em-art-missing':''}`} style={{'--seed':(item.title||item.name||'').length%7}}>
        {safe?<>{visible&&<img key={requestKey} ref={photo} src={apiUrl(url)} onLoad={()=>setLoadedSrc(requestKey)} onError={fail} decoding="async" loading="eager" fetchpriority="high" alt={kind==='console'?`Photo de ${item.name}`:''}/>} {!loaded&&visible&&<span className="em-cover-loading" role="status">{kind==='console'?'Chargement de la photo…':'Chargement de la jaquette…'}</span>}{loaded&&<span className="gx-credit">{credit}</span>}</>:<><Icon name={kind==='console'?'consoles':'library'} size={32}/><span className="gx-no-cover">{failed?'Image momentanément indisponible':kind==='console'?'Photo indisponible':'Jaquette indisponible'}</span></>}
        <DownloadOverlay job={item.download} pending={item.pendingDownload}/>
    </div>;
}

export default function ExperienceView(props){return <PlayStationHome {...props}/>;}
