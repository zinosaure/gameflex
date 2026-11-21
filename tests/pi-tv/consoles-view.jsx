import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import {SystemBrowser} from '../../src/webapp/src/app/gameflex/experience/EmulatorUI.jsx';

const consoles=[
 {id:'gba',name:'Console portable au nom volontairement très long',year:2001,publisher:'Nintendo'},
 {id:'snes',name:'Super Nintendo',year:1990,publisher:'Nintendo'},
 {id:'ps1',name:'PlayStation',year:1994,publisher:'Sony'},
];
const games=[{consoleId:'gba',reference:'one',title:'Jeu'},{consoleId:'gba',reference:'two',title:'Jeu 2'},{consoleId:'snes',reference:'three',title:'Jeu 3'}];
const Art=({item})=><span data-art={item.image?'available':'missing'}>{item.name}</span>;
const html=renderToStaticMarkup(<SystemBrowser mode="consoles" consoles={consoles} games={games} Art={Art} onConsole={()=>{}}/>);
assert.ok(html.includes('em-system-prev'));
assert.ok(html.includes('em-system-next'));
assert.ok(html.includes('Console portable au nom volontairement très long'));
assert.ok(html.includes('2 jeux installés'));
assert.ok(html.includes('Explorer'));
assert.ok(html.includes('data-art="missing"'));
const empty=renderToStaticMarkup(<SystemBrowser mode="consoles" consoles={[]} games={[]} Art={Art} onConsole={()=>{}}/>);
assert.ok(empty.includes('Aucune console disponible'));
console.log('Consoles : rail, flèches, photo, compteurs, titre long, image absente et vide valides.');
