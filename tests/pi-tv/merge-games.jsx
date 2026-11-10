import assert from 'node:assert/strict';
import {mergeGames,gameIdentity} from '../../src/webapp/src/app/gameflex/experience/merge-games.js';
const groups=[
 {provider:'Bibliothèque',items:[{title:'pac-man-collection',consoleId:'gba',reference:'local-ref'}]},
 {provider:'Planetemu',items:[{title:'Pac-Man Collection (Europe) (Virtual Console) Retirée',source:'https://www.planetemu.net/rom/gba/europe',image:'/cached-cover'},{title:'Pac-Man Collection (USA)',source:'https://www.planetemu.net/rom/gba/usa'}]},
 {provider:'Autre fournisseur de test',items:[{title:'Pac-Man Collection (Japan) (En)',source:'https://example.invalid/metadata/pacman'}]}
];
const merged=mergeGames(groups,'gba');assert.equal(merged.length,1);assert.equal(merged[0].reference,'local-ref');assert.equal(merged[0].image,'/cached-cover');assert.equal(merged[0].variants.length,4);assert.deepEqual(merged[0].sources,['Bibliothèque','Planetemu','Autre fournisseur de test']);assert.equal(merged[0].title,'Pac-Man Collection');
assert.notEqual(gameIdentity({title:'Game 1'},'nes'),gameIdentity({title:'Game 2'},'nes'));
assert.notEqual(gameIdentity({title:'Game (USA) (Beta)'},'nes'),gameIdentity({title:'Game (USA)'},'nes'));
assert.notEqual(gameIdentity({title:'Game'},'nes'),gameIdentity({title:'Game'},'gba'));
assert.notEqual(gameIdentity({title:'Game (Special Edition)'},'gba'),gameIdentity({title:'Game'},'gba'));
assert.equal(gameIdentity({title:'Game (Netherlands) (En,Nl)'},'gba'),gameIdentity({title:'Game (USA)'},'gba'));
const install=mergeGames([{provider:'Free',items:[{title:'Palette libre',downloadId:'palette-libre'}]},{provider:'Metadata',items:[{title:'Palette libre (World)',source:'https://example.invalid/title'}]}],'nes');assert.equal(install.length,1);assert.equal(install[0].downloadId,'palette-libre');
console.log('Fusion : sources réunies, priorité aux jeux installés, jaquettes conservées, éditions de région regroupées ; suites, consoles et prototypes distincts.');
const emeraldTitles=['Pokemon - Emerald Version (USA, Europe)','Pokemon - Version Emeraude (France)','Pokemon - Edicion Esmeralda (Spain)','Pokemon - Smaragd-Edition (Germany)','Pokemon - Versione Smeraldo (Italy)','Pocket Monsters - Emerald (Japan)','Pokemon: Emerald Version (UE)'];
const emerald=mergeGames([{provider:'Planetemu',items:emeraldTitles.map((title,i)=>({title,image:'/image-'+i,source:'https://www.planetemu.net/rom/gba/'+i}))}],'gba');
assert.equal(emerald.length,1);assert.equal(emerald[0].title,'Pokémon Émeraude');assert.equal(emerald[0].variants.length,7);assert.equal(emerald[0].image,'/image-1');assert.ok(emerald[0].variants.some(x=>x.language==='es'));assert.ok(emerald[0].variants.some(x=>x.language==='fr'));
assert.notEqual(gameIdentity({title:'Pokemon - Emerald Version (USA) (Beta)'},'gba'),gameIdentity({title:'Pokemon - Emerald Version (USA)'},'gba'));
assert.notEqual(gameIdentity({title:'Pokemon - Emerald Version - Mystery Event'},'gba'),gameIdentity({title:'Pokemon - Emerald Version'},'gba'));
assert.notEqual(gameIdentity({title:'Pokemon - FireRed Version'},'gba'),gameIdentity({title:'Pokemon - LeafGreen Version'},'gba'));
assert.notEqual(gameIdentity({title:'Pokemon - Ruby Version'},'gba'),gameIdentity({title:'Pokemon - Sapphire Version'},'gba'));
console.log('Pokémon GBA : noms traduits regroupés, langues et jaquettes conservées ; versions différentes et distributions distinctes.');

const pure=mergeGames([{provider:'Planetemu',items:[{title:'Pokemon - Emerald Version (USA)'}]},{provider:'ROMsPure',items:[{title:'Pokemon Emerald',downloadId:'romspure:gba:nintendo-game-boy-advance~pokemon-emerald',image:'/experience-api/v1/romspure-art/gba/nintendo-game-boy-advance~pokemon-emerald'}]}],'gba');
assert.equal(pure.length,1);assert.equal(pure[0].variants.length,2);assert.ok(pure[0].downloadId.startsWith('romspure:'));
console.log('ROMsPure : même jeu regroupé malgré l’absence du mot Version.');
