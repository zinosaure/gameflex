import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import RetroLibrary from '../../src/webapp/src/app/gameflex/experience/RetroLibrary.jsx';

const consoles=[
 {id:'gba',name:'Console portable au nom volontairement très long',year:2001},
 {id:'snes',name:'Super Nintendo',year:1990},
 {id:'ps1',name:'PlayStation',year:1994},
];
const games=[
 {consoleId:'gba',reference:'one',title:'Jeu installé'},
 {consoleId:'snes',reference:'two',title:'Jeu installé'},
 {consoleId:'snes',reference:'three',title:'Deuxième jeu'},
 {consoleId:'ps1',reference:'four',title:'Jeu sans image'},
];
const Art=({item})=><span data-art={item.image?'available':'missing'}>{item.name}</span>;
const html=renderToStaticMarkup(<RetroLibrary mode="station" games={games} consoles={consoles} Art={Art} section="home" onConsole={()=>{}}/>);
assert.ok(html.includes('ret-horizon-title'));
assert.ok(html.includes('Game Boy Advance'));
assert.ok(!html.includes('La prochaine<br/>aventure'));
assert.ok(html.includes('Explorer les jeux'));
assert.ok(html.includes('aria-current="true"'));
assert.ok(html.includes('1 jeu'));
assert.ok(html.includes('2 jeux'));
assert.ok(html.includes('data-art="missing"'));
assert.equal((html.match(/ret-horizon-card/g)||[]).length,3);
const longName='Console portable au nom volontairement très long';
const longTitle=renderToStaticMarkup(<RetroLibrary mode="station" games={[{consoleId:'portable',reference:'long',title:'Jeu installé'}]} consoles={[{id:'portable',name:longName,year:2026}]} Art={Art} section="home" onConsole={()=>{}}/>);
assert.ok(longTitle.includes(longName));
const empty=renderToStaticMarkup(<RetroLibrary mode="station" games={[]} consoles={consoles} Art={Art} section="home" onConsole={()=>{}}/>);
assert.ok(empty.includes('Votre collection commence ici.'));
assert.ok(!empty.includes('0 jeu'));
console.log('Horizon : titre dynamique, rail réel, singulier/pluriel, image absente et bibliothèque vide valides.');
