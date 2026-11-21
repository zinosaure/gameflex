import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import RetroLibrary from '../../src/webapp/src/app/gameflex/experience/RetroLibrary.jsx';

const consoles=[
 {id:'ps1',name:'PlayStation',year:1994},
 {id:'gba',name:'Console portable au nom volontairement très long',year:2001},
 {id:'snes',name:'Super Nintendo',year:1990},
];
const games=[
 {consoleId:'gba',reference:'one',title:'Premier jeu'},
 {consoleId:'gba',reference:'two',title:'Deuxième jeu'},
 {consoleId:'ps1',reference:'three',title:'Jeu sans image'},
 {consoleId:'snes',reference:'four',title:'Quatrième jeu'},
];
const Art=({item})=><span data-art={item.image?'available':'missing'}>{item.title||item.name}</span>;
const html=renderToStaticMarkup(<RetroLibrary mode="orbit" games={games} consoles={consoles} Art={Art} section="home" onConsole={()=>{}}/>);
assert.ok(html.includes('ret-orbit-active'));
assert.ok(html.includes('ret-orbit-rail'));
assert.ok(html.includes('2001'));
assert.ok(html.includes('2 jeux'));
assert.ok(html.includes('Console portable au nom volontairement très long'));
assert.ok(html.includes('data-art="missing"'));
const gba=html.indexOf('data-system="gba"'),ps1=html.indexOf('data-system="ps1"'),snes=html.indexOf('data-system="snes"');
assert.ok(gba<ps1&&ps1<snes,'ordre orbital alphabétique stable');
const empty=renderToStaticMarkup(<RetroLibrary mode="orbit" games={[]} consoles={consoles} Art={Art} section="home" onConsole={()=>{}}/>);
assert.ok(empty.includes('Votre collection commence ici.'));
console.log('Orbitale : fiche active, ordre stable, rail, compteurs, titre long, repli photo et vide valides.');
