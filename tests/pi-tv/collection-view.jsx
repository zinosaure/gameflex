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
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'one',title:'Premier jeu'},
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'two',title:'Deuxième jeu'},
 {consoleId:'snes',consoleName:'Super Nintendo',reference:'three',title:'Jeu sans image'},
 {consoleId:'ps1',consoleName:'PlayStation',reference:'four',title:'Jeu PlayStation'},
];
const Art=({item})=><span data-art={item.image?'available':'missing'}>{item.title||item.name}</span>;
const gallery=renderToStaticMarkup(<RetroLibrary mode="shelf" games={games} consoles={consoles} Art={Art} section="home" onConsole={()=>{}}/>);
assert.equal((gallery.match(/ret-museum-card/g)||[]).length,3);
assert.ok(gallery.includes('2001 · 2 jeux'));
assert.ok(gallery.includes('1990 · 1 jeu'));
assert.ok(gallery.includes('Console portable au nom volontairement très long'));
assert.ok(gallery.includes('data-art="missing"'));
const library=renderToStaticMarkup(<RetroLibrary mode="shelf" games={games} consoles={consoles} selectedConsole={consoles[0]} Art={Art} section="home" onOpen={()=>{}} onHome={()=>{}}/>);
assert.ok(library.includes('ret-shelf-library'));
assert.ok(library.includes('Premier jeu'));
const empty=renderToStaticMarkup(<RetroLibrary mode="shelf" games={[]} consoles={consoles} Art={Art} section="home" onConsole={()=>{}}/>);
assert.ok(empty.includes('Votre collection commence ici.'));
console.log('La collection : cartels réels, années, compteurs, titre long, repli photo, bibliothèque et vide valides.');
