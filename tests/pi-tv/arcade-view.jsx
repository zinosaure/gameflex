import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import RetroLibrary from '../../src/webapp/src/app/gameflex/experience/RetroLibrary.jsx';

const consoles=[
 {id:'gba',name:'Game Boy Advance',year:2001,maxPlayers:4},
 {id:'snes',name:'Super Nintendo',year:1990,maxPlayers:2},
];
const games=[
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'one',title:'Un titre de jeu volontairement très long pour valider la borne',image:'/one.webp'},
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'two',title:'Jeu sans image'},
 {consoleId:'snes',consoleName:'Super Nintendo',reference:'three',title:'Troisième jeu'},
];
const Art=({item})=><span data-art={item.image?'available':'missing'}>{item.title||item.name}</span>;
const chooser=renderToStaticMarkup(<RetroLibrary mode="arcade" games={games} consoles={consoles} Art={Art} section="home" onConsole={()=>{}}/>);
assert.ok(chooser.includes('ret-arcade-systems'));
assert.ok(chooser.includes('Game Boy Advance'));
assert.ok(chooser.includes('2 jeux'));
assert.ok(chooser.includes('Choisir ce système'));

const library=renderToStaticMarkup(<RetroLibrary mode="arcade" games={games} consoles={consoles} selectedConsole={consoles[0]} Art={Art} section="home" onOpen={()=>{}} onHome={()=>{}}/>);
assert.ok(library.includes('ret-arcade-library'));
assert.equal((library.match(/ret-arcade-machine /g)||[]).length,2);
assert.ok(library.includes('Un titre de jeu volontairement très long'));
assert.ok(library.includes('Jusqu’à 4 joueurs'));
assert.ok(library.includes('Voir la fiche'));
assert.ok(library.includes('data-art="missing"'));

const empty=renderToStaticMarkup(<RetroLibrary mode="arcade" games={[]} consoles={consoles} Art={Art} section="home" onConsole={()=>{}}/>);
assert.ok(empty.includes('Votre collection commence ici.'));
assert.ok(!empty.includes('0 jeu'));
console.log('Borne Arcade : systèmes, rang de bornes, jeu actif, joueurs, titre long, image absente et vide valides.');
