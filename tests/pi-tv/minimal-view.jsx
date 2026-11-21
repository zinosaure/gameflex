import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import ExperienceView from '../../src/webapp/src/app/gameflex/experience/ExperienceView.jsx';

const consoles=[
 {id:'gba',name:'Game Boy Advance',year:2001},
 {id:'snes',name:'Super Nintendo',year:1990},
];
const games=[
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'one',title:'Un titre volontairement très long pour la liste compacte',status:'Local',image:'/missing'},
 {consoleId:'snes',consoleName:'Super Nintendo',reference:'two',title:'Deuxième jeu'},
];
const html=renderToStaticMarkup(<ExperienceView mode="minimal" games={games} consoles={consoles} section="home" onOpen={()=>{}} onConsole={()=>{}} onNavigate={()=>{}} onSettings={()=>{}}/>);
assert.ok(html.includes('em-minimal-filters'));
assert.ok(html.includes('Filtrer par console'));
assert.ok(html.includes('Filtrer par état'));
assert.ok(html.includes('Sortie'));
assert.ok(html.includes('Statut'));
assert.ok(html.includes('2001'));
assert.ok(html.includes('Installé'));
assert.ok(html.includes('Un titre volontairement très long'));
assert.ok(html.includes('is-selected'));
const empty=renderToStaticMarkup(<ExperienceView mode="minimal" games={[]} consoles={consoles} section="home" onOpen={()=>{}} onConsole={()=>{}} onNavigate={()=>{}} onSettings={()=>{}}/>);
assert.ok(empty.includes('Aucun jeu ici'));
console.log('Minimal : recherche, filtres, colonnes, focus, année, statut, titre long et vide valides.');
