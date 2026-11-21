import React,{useEffect,useRef,useState}from'react';
import'./grand-ecran-accessible.css';

const key=game=>`${game.consoleId}:${game.reference}`;

export default function GrandEcranAccessible({games=[],favorites=[],Art,onOpen,onNavigate=()=>{},onSettings,onFavorite}){
 const root=useRef(null),[active,setActive]=useState(()=>games[0]);
 useEffect(()=>{
  let saved='';
  try{saved=sessionStorage.getItem('gameflex:accessible-focus')||''}catch{}
  const game=games.find(item=>key(item)===saved)||games[0];
  if(game)setActive(game);
  setTimeout(()=>root.current?.querySelector(`[data-access-key="${game?key(game):''}"]`)?.focus({preventScroll:true}),100);
 },[games]);
 const choose=game=>{setActive(game);try{sessionStorage.setItem('gameflex:accessible-focus',key(game))}catch{}};
 const move=(event,game)=>{
  if(!['ArrowLeft','ArrowRight'].includes(event.key))return;
  event.preventDefault();
  const index=games.indexOf(game),next=games[(index+(event.key==='ArrowRight'?1:-1)+games.length)%games.length];
  const node=root.current?.querySelector(`[data-access-key="${key(next)}"]`);
  node?.focus();node?.scrollIntoView({inline:'center'});
 };
 if(!games.length)return <section className="gf-access gf-access-empty" data-interface="grand-ecran-accessible"><h1>Votre bibliothèque est vide</h1></section>;
 const current=active&&games.includes(active)?active:games[0];
 return <section ref={root} className="gf-access" data-interface="grand-ecran-accessible"><header><strong>GAME<span>FLEX</span></strong><b>22 GRAND ÉCRAN ACCESSIBLE</b><aside>Mode accessible : <mark>Activé</mark> · {games.length} jeux</aside></header><h1>Choisissez un jeu</h1><div className="gf-access-rail">{games.map(game=><button key={key(game)} data-access-key={key(game)} aria-pressed={game===current} aria-label={game.title} onFocus={()=>choose(game)} onMouseEnter={()=>choose(game)} onKeyDown={event=>move(event,game)} onClick={()=>onOpen(game)}><Art item={game} wide/><strong>{game.title}</strong></button>)}</div><footer><span><kbd>+</kbd> Naviguer</span><button onClick={()=>onOpen(current)}><kbd>A</kbd> Lancer</button><button onClick={()=>onNavigate('library')}><kbd>B</kbd> Retour</button><button onClick={()=>onFavorite?.(current)} aria-pressed={favorites.includes(key(current))}><kbd>Y</kbd> Favori</button><button onClick={onSettings}><kbd>☰</kbd> Options</button></footer></section>;
}
