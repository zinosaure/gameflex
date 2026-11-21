import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import ExperienceView from '../../src/webapp/src/app/gameflex/experience/ExperienceView.jsx';

const consoles=[{id:'gba',name:'Game Boy Advance',year:2001}];
const games=[
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'one',title:'Aventure au titre volontairement très long sur deux lignes',image:'/missing'},
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'two',title:'Bravo installé'},
 {consoleId:'gba',consoleName:'Game Boy Advance',source:'catalogue',title:'Découverte catalogue'},
];
const html=renderToStaticMarkup(<ExperienceView mode="covers" games={games} consoles={consoles} section="home" onOpen={()=>{}} onConsole={()=>{}} onNavigate={()=>{}} onSettings={()=>{}}/>);
assert.ok(html.includes('gf-cover-tabs'));
assert.ok(html.includes('Tous'));
assert.ok(html.includes('Installés'));
assert.ok(html.includes('À découvrir'));
assert.ok(html.includes('gx-alphabet'));
assert.ok(html.includes('Aventure au titre volontairement très long'));
assert.ok(html.includes('em-cover-wall'));
const empty=renderToStaticMarkup(<ExperienceView mode="covers" games={[]} consoles={consoles} section="home" onOpen={()=>{}} onConsole={()=>{}} onNavigate={()=>{}} onSettings={()=>{}}/>);
assert.ok(empty.includes('Aucun jeu ici'));
console.log('Jaquettes : filtres, index A–Z, grille, titre long, image absente et vide valides.');
