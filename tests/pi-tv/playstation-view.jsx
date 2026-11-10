import React from 'react';
import {renderToStaticMarkup as render} from 'react-dom/server';
import assert from 'node:assert/strict';
import ExperienceView from '../../src/webapp/src/app/gameflex/experience/ExperienceView.jsx';
import {shortTitle} from '../../src/webapp/src/app/gameflex/experience/PlayStationHome.jsx';
import {archivedAppearances,interfaces,selectableInterfaces,migratePreference} from '../../src/webapp/src/app/gameflex/experience/interfaces.js';
const game={consoleId:'gba',consoleName:'Game Boy Advance',reference:'a',title:'Advance Wars (Europe) (En,Fr).gba',status:{status:'playable',emulators:{mgba:'playable'}},files:['a.gba']};
assert.equal(shortTitle(game),'Advance Wars');assert.equal(archivedAppearances.length,22);assert.deepEqual(selectableInterfaces.map(i=>i.id),['ps-home']);
for(const appearance of archivedAppearances){
 const before={interface:appearance.id,favorites:['gba:a'],history:['gba:a'],collections:[{id:'a',name:'Classiques',games:['gba:a']}]};
 const migrated=migratePreference(before);assert.equal(migrated.interface,'ps-home');assert.equal(migrated.archivedInterface,appearance.id);
 assert.deepEqual(migrated.favorites,before.favorites);assert.deepEqual(migratePreference(migrated),migrated);
}
for(const section of ['home','library','favorites','resume','collections','consoles','multiplayer','management','settings']){
 const html=render(<ExperienceView section={section} games={[game]} consoles={[{id:'gba',name:'GBA',maxPlayers:2}]} favorites={['gba:a']} history={['gba:a']} collections={[{id:'a',name:'Classiques',games:['gba:a']}]} settingsContent={<p>Réglages de test</p>}/>);
 assert(html.includes('data-interface="ps-home"'));assert(!html.includes('[object Object]'));assert(!html.includes('Changer d’interface'));
}
const pending=render(<ExperienceView loading games={[]}/>);assert(pending.includes('Chargement'));assert(!pending.includes('Votre bibliothèque prend vie ici'));
const home=render(<ExperienceView games={[game]}/>);assert(home.includes('Jouer'));assert(!home.includes('(Europe)'));
const recent=render(<ExperienceView games={[game]} history={['gba:a']}/>);assert(recent.includes('Relancer'));
console.log('Unified console: 22 idempotent archive migrations, nine sections, raw kernel status, loading, short titles and launch labels passed.');
