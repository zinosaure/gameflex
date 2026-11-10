import React,{useEffect,useMemo,useRef,useState}from'react';
import{Icon}from'./EmulatorUI.jsx';
import'./terminal-poche.css';

const key=game=>`${game.consoleId}:${game.reference}`;
const category=game=>game.genre||game.category||game.consoleName||'Autres';

export default function TerminalPoche({games=[],favorites=[],Art,onOpen,onNavigate=()=>{},onSettings,onFavorite}){
 const tabs=useMemo(()=>['Tous',...new Set(games.map(category).filter(Boolean))].slice(0,6),[games]),[filter,setFilter]=useState('Tous'),list=filter==='Tous'?games:games.filter(game=>category(game)===filter),[active,setActive]=useState(()=>games[0]),root=useRef(null);
 useEffect(()=>{let saved='';try{saved=sessionStorage.getItem('gameflex:pocket-focus')||'';}catch{}const game=games.find(item=>key(item)===saved)||games[0];if(game)setActive(game);setTimeout(()=>root.current?.querySelector(`[data-pocket-key="${game?key(game):''}"]`)?.focus({preventScroll:true}),100);},[]);
 useEffect(()=>{if(!list.includes(active)&&list[0])setActive(list[0]);},[filter]);
 const choose=game=>{setActive(game);try{sessionStorage.setItem('gameflex:pocket-focus',key(game));}catch{}};
 const move=(event,game)=>{if(!['ArrowUp','ArrowDown'].includes(event.key))return;event.preventDefault();const index=list.indexOf(game),next=list[(index+(event.key==='ArrowDown'?1:-1)+list.length)%list.length],node=root.current?.querySelector(`[data-pocket-key="${key(next)}"]`);node?.focus();node?.scrollIntoView({block:'nearest'});};
 if(!games.length)return <section className="gf-pocket gf-pocket-empty" data-interface="terminal-poche"><Icon name="library" size={40}/><h1>Votre bibliothèque est vide</h1><button onClick={()=>onNavigate('library')}>Ajouter un jeu</button></section>;
 const count=Number(active?.players||active?.maxPlayers||1),favorite=favorites.includes(key(active));
 return <section ref={root} className="gf-pocket" data-interface="terminal-poche"><header><strong>GAME<span>FLEX</span></strong><b>14 TERMINAL DE POCHE</b><nav><button onClick={()=>onNavigate('library')}><Icon name="library"/> Jeux</button><button onClick={onSettings}><Icon name="settings"/> Paramètres</button></nav></header><div className="gf-pocket-shell"><div className="gf-pocket-pad" aria-hidden="true"><i/><i/></div><main className="gf-pocket-lcd"><nav className="gf-pocket-tabs" aria-label="Catégories">{tabs.map(tab=><button key={tab} aria-pressed={filter===tab} onClick={()=>setFilter(tab)}>{tab}</button>)}</nav><div className="gf-pocket-content"><div className="gf-pocket-list">{list.map(game=><button key={key(game)} data-pocket-key={key(game)} aria-pressed={game===active} onFocus={()=>choose(game)} onMouseEnter={()=>choose(game)} onKeyDown={event=>move(event,game)} onClick={()=>onOpen(game)}><Icon name={favorites.includes(key(game))?'favorites':'library'}/><strong>{game.title}</strong></button>)}</div><article className="gf-pocket-preview"><Art item={active}/><h1>{active?.title}</h1><p>{category(active)} <i/> {count} joueur{count===1?'':'s'}</p></article></div><footer><button onClick={()=>onOpen(active)}><kbd>A</kbd> Jouer</button><button onClick={()=>onOpen(active)}><kbd>B</kbd> Détails</button><button aria-pressed={favorite} onClick={()=>onFavorite?.(active)}><Icon name="favorites"/> Favori</button></footer></main><div className="gf-pocket-actions" aria-hidden="true"><i>A</i><i>B</i></div></div></section>;
}
