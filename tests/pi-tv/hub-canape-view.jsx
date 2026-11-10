import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import HubCanape from '../../src/webapp/src/app/gameflex/experience/HubCanape.jsx';

const games=[
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'saved',title:'Aventure au titre volontairement très long',saveState:{slot:1},image:'/cover'},
 {consoleId:'snes',consoleName:'Super Nintendo',reference:'plain',title:'Jeu sans image'},
];
const Art=({item})=><span data-art={item.image?'available':'missing'}>{item.title}</span>;
const html=renderToStaticMarkup(<HubCanape games={games} history={['gba:saved']} favorites={['gba:saved']} Art={Art} onOpen={()=>{}} onNavigate={()=>{}}/>);
assert.ok(html.includes('gf-hub-hero'));
assert.ok(html.includes('Reprendre'));
assert.ok(html.includes('Aventure au titre volontairement très long'));
assert.ok(html.includes('Épinglés'));
assert.ok(html.includes('data-art="missing"'));
const relaunch=renderToStaticMarkup(<HubCanape games={games} history={['snes:plain']} favorites={[]} Art={Art} onOpen={()=>{}} onNavigate={()=>{}}/>);
assert.ok(relaunch.includes('Relancer'));
const empty=renderToStaticMarkup(<HubCanape games={[]} history={[]} favorites={[]} Art={Art} onOpen={()=>{}} onNavigate={()=>{}}/>);
assert.ok(empty.includes('Votre bibliothèque est vide'));
console.log('Hub Canapé : héros récent, reprise réelle, épingles, titre long, image absente et vide valides.');
