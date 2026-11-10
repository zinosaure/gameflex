import React from 'react';
import {renderToStaticMarkup as render} from 'react-dom/server';
import assert from 'node:assert/strict';
import ExperienceView from '../../src/webapp/src/app/gameflex/experience/ExperienceView.jsx';
import CatalogGameDialog from '../../src/webapp/src/app/gameflex/experience/CatalogGameDialog.jsx';
import {selectableInterfaces,acceptRemoteChoice} from '../../src/webapp/src/app/gameflex/experience/interfaces.js';
import {gameStatusCode,gameStatusLabel,canResumeGame} from '../../src/webapp/src/app/gameflex/experience/game-presentation.js';
import {editionFields} from '../../src/webapp/src/app/gameflex/experience/edition-presentation.js';
const status={status:'playable',emulators:{mgba:'playable'}};
const games=[{consoleId:'gba',consoleName:'Game Boy Advance',reference:'a',title:'Advance Wars (Europe) (En,Fr).gba',status,files:['a.gba']}];
const consoles=[{id:'gba',name:'Game Boy Advance',maxPlayers:2}];
assert.equal(gameStatusCode(status),'playable');assert.equal(gameStatusLabel(status),'Jouable');
assert.equal(gameStatusCode({status:{bad:1}}),'');assert.equal(gameStatusLabel(null),'Non testé');
assert.equal(status.emulators.mgba,'playable','The original report must remain intact');
for(const {id} of selectableInterfaces){
 for(const section of ['library','management','archives']){
  const html=render(<ExperienceView mode={id} games={games} consoles={consoles} section={section}/>);
  assert(html.includes('Advance Wars'),`${id}/${section}`);
  assert(!html.includes('[object Object]'));
 }
 const pending=render(<ExperienceView mode={id} games={[]} consoles={consoles} loading section="home"/>);
 assert(pending.includes('Chargement'));assert(!pending.includes('Votre bibliothèque est vide'),id);
}
const sheet=render(<CatalogGameDialog game={games[0]} onClose={()=>{}}/>);
assert(sheet.includes('<h2 id="gf-catalogue-title">Advance Wars</h2>'));
assert(sheet.includes('Édition installée'));assert(!sheet.includes('<select'));
assert(sheet.includes('<dt>Région</dt><dd>Europe</dd>'));
assert(sheet.includes('Anglais / Français'));assert(!sheet.includes('(Europe)'));
assert.equal(editionFields({title:'Test (USA) (En) (Rev 1)'}).edition,'Rev 1');
assert.equal(canResumeGame({hasSave:true}),false);
assert.equal(canResumeGame({saveCompatible:false,saveState:{slot:1}}),false);
assert.equal(canResumeGame({saveCompatible:true}),true);
assert.equal(canResumeGame({saves:[{compatible:true}]}),true);
assert.equal(acceptRemoteChoice({interface:'crt'}, {id:'shelf',updatedAt:20},0,0),false);
assert.equal(acceptRemoteChoice({interface:'crt',interfaceUpdatedAt:21}, {id:'shelf',updatedAt:20},0,0),true);
assert.equal(acceptRemoteChoice({interface:'crt'},null,0,1),false);
const collections=render(<ExperienceView mode="shelf" section="collections" games={games} consoles={consoles} collections={[{id:'a',name:'Mes classiques',games:['gba:a']}]}/>);
assert(!collections.includes('<h2>Collections</h2>'),'One section title is sufficient');
console.log('UI regressions: kernel status objects, 22 modes × 3 sections, loading, short title, edition fields, compatible saves and stale preference guards passed.');
