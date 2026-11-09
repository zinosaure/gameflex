import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import RetroLibrary from '../../src/webapp/src/app/gameflex/experience/RetroLibrary.jsx';

const consoleItem={id:'snes',name:'Super Nintendo',publisher:'Nintendo',year:1990};
const games=[
 {consoleId:'snes',consoleName:'Super Nintendo',reference:'long',title:'Un titre volontairement très long pour vérifier la ligne active',image:'/cover'},
 {consoleId:'snes',consoleName:'Super Nintendo',reference:'missing',title:'Jeu sans jaquette'},
 {consoleId:'snes',consoleName:'Super Nintendo',reference:'third',title:'Troisième jeu'},
];
const Art=({item})=><span data-art={item.image?'available':'missing'}>{item.title||item.name}</span>;
const html=renderToStaticMarkup(<RetroLibrary mode="crt" games={games} consoles={[consoleItem]} selectedConsole={consoleItem} Art={Art} section="home" onOpen={()=>{}} onHome={()=>{}}/>);
assert.ok(html.includes('ret-crt-screen'));
assert.ok(html.includes('Un titre volontairement très long'));
assert.ok(html.includes('data-art="missing"'));
assert.equal((html.match(/data-game=/g)||[]).length,3);
assert.ok(html.includes('aria-pressed="true"'));
assert.ok(html.includes('Lignes activées'));
const empty=renderToStaticMarkup(<RetroLibrary mode="crt" games={[]} consoles={[consoleItem]} selectedConsole={consoleItem} Art={Art} section="home" onOpen={()=>{}} onHome={()=>{}}/>);
assert.ok(empty.includes('Aucun jeu ne correspond.'));
console.log('Signal CRT : aperçu, liste réelle, lignes désactivables, titre long, image absente et vide valides.');
