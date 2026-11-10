import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import ExperienceView, { InterfacePicker } from '../../src/webapp/src/app/gameflex/experience/ExperienceView.jsx';
import {retroModes} from '../../src/webapp/src/app/gameflex/experience/RetroLibrary.jsx';
import { interfaces } from '../../src/webapp/src/app/gameflex/experience/interfaces.js';
import assert from 'node:assert/strict';
const games=[{consoleId:'nes',consoleName:'NES',reference:'free',title:'Palette libre',status:'Complete'}];
const consoles=[{id:'nes',name:'NES',maxPlayers:2}];
const standaloneModes=['hub-canape','ligne-cinema','mosaique-joy','roue-retro','terminal-poche','cartouches','carte-epoques','double-ecran'];
const results=[];
for(const {id,name} of interfaces.filter(x=>!retroModes.includes(x.id)&&!standaloneModes.includes(x.id))){
 const html=renderToStaticMarkup(<ExperienceView mode={id} games={games} consoles={consoles} favorites={['nes:free']} history={['nes:free']} collections={[{id:'local',name:'Homebrew',games:['nes:free']}]} onOpen={()=>{}} onMode={()=>{}} onFavorite={()=>{}}/>);
 assert.ok(html.includes(`gx-${id}`));
 assert.ok(html.includes('Accueil'));
 assert.ok(!html.includes('onMode'));
 assert.ok(!html.includes('undefined'));
 assert.ok(html.includes('Palette libre') || id==='consoles');
 const content=html.slice(html.indexOf('</header>')+9,html.lastIndexOf('</main>'));
 assert.ok(!results.some(x=>x.content===content),'Composition identique : '+id);
 results.push({id,content});
 const empty=renderToStaticMarkup(<ExperienceView mode={id} onOpen={()=>{}} onMode={()=>{}}/>);
 assert.ok(empty.includes('gx-app'));
}
const picker=renderToStaticMarkup(<InterfacePicker onChoose={()=>{}}/>);
assert.equal((picker.match(/class="gx-choice /g)||[]).length,17);
assert.ok(picker.includes('gx-choice gf-choice-orbit'));
assert.ok(!picker.includes('gx-choice gx-orbit'),'La carte Orbitale ne doit pas réutiliser la classe du décor absolu.');
for(const id of standaloneModes){const html=renderToStaticMarkup(<ExperienceView mode={id} games={games} consoles={consoles} favorites={['nes:free']} history={['nes:free']} onOpen={()=>{}} onConsole={()=>{}}/>);assert.ok(html.includes(`data-interface="${id}"`));assert.ok(html.includes('Palette libre')||id==='roue-retro');const empty=renderToStaticMarkup(<ExperienceView mode={id} games={[]} consoles={[]} onOpen={()=>{}} onConsole={()=>{}}/>);assert.ok(empty.includes('Votre bibliothèque est vide'));}
console.log('18 compositions distinctes rendues, 18 états vides rendus, sélecteur de 17 choix valide.');

const photoHtml=renderToStaticMarkup(<ExperienceView mode="consoles" consoles={[{id:'nes',name:'NES',image:'/experience-api/v1/media/'+'a'.repeat(64),attribution:'Evan-Amos · Public domain'}]} />);
assert.ok(photoHtml.includes('gx-console-art'));
assert.ok(photoHtml.includes('alt="Photo de NES"'));
assert.ok(photoHtml.includes('Chargement de la photo'));
console.log('Photos des consoles et crédits : rendu valide.');

for (const {id} of interfaces.filter(x=>!retroModes.includes(x.id))) {
 const html=renderToStaticMarkup(<ExperienceView mode={id} section="favorites" />);
 assert.ok(html.includes(`gx-app gx-${id}`));
 assert.ok(html.includes('Favoris'));
}
const consoleHtml=renderToStaticMarkup(<ExperienceView mode="consoles" selectedConsole={{id:'nes',name:'NES'}} games={games} planetLetter="E" planet={{items:[{title:'External title',source:'https://www.planetemu.net/rom/example/title'}]}} />);
assert.ok(consoleHtml.includes('gf-unified-catalogue'));assert.ok(consoleHtml.includes('Installés')); assert.ok(consoleHtml.includes('cartridge-shelf'));assert.ok(consoleHtml.includes('External title'));
console.log('Navigation indépendante des 10 apparences ; rayons local/externe distincts.');

const filteredM=renderToStaticMarkup(<ExperienceView mode="consoles" selectedConsole={{id:'nes',name:'NES'}} planetLetter="M" planet={{items:[{title:'Mario Bros.',source:'https://www.planetemu.net/rom/nes/mario'},{title:'Adian no Tsue',source:'https://www.planetemu.net/rom/fds/adian'}]}} />);
assert.ok(filteredM.includes('Mario Bros.'));assert.ok(!filteredM.includes('Adian no Tsue'));
const coverCard=renderToStaticMarkup(<ExperienceView mode="consoles" selectedConsole={{id:'nes',name:'NES'}} planetLetter="M" planet={{items:[{title:'Mario Bros.',source:'https://www.planetemu.net/rom/nes/mario',image:'/experience-api/v1/covers/nes/TWFyaW8'}]}} />);
assert.ok(coverCard.includes('src="/experience-api/v1/covers/nes/TWFyaW8"'));
console.log('Filtre alphabétique et jaquettes du cache : rendu valide.');

const layouts={minimal:['compact-list','<ol'],covers:['cover-wall','em-cover-wall'],consoles:['cartridge-shelf','em-cartridge-index'],favorites:['pinned-showcase','em-featured-pin'],resume:['xmb-browser','em-xmb-spine'],collections:['record-shelves','em-record-row'],detail:['cinema-browser','em-cinema-filmstrip'],archives:['file-browser','em-file-preview'],multiplayer:['arcade-select','em-arcade-index'],dashboard:['metro-grid','em-tile-wide']};
const structures=[];
for (const [mode,[layout,feature]] of Object.entries(layouts)) {
 const html=renderToStaticMarkup(<ExperienceView mode={mode} selectedConsole={{id:'nes',name:'NES',year:1983}} games={games} planetLetter="M" planet={{items:[{title:'Mario Bros.',source:'https://www.planetemu.net/rom/nes/mario'},{title:'Metroid',source:'https://www.planetemu.net/rom/nes/metroid'}]}} />);
 assert.ok(html.includes(`data-layout="${layout}"`));assert.ok(html.includes(feature));
 assert.ok(html.includes('gf-unified-catalogue')&&html.includes('Installés'));
 const dock=html.slice(html.indexOf('<nav class="em-dock"'),html.indexOf('</nav>',html.indexOf('<nav class="em-dock"')));
 assert.equal((dock.match(/<button/g)||[]).length,5);assert.equal((dock.match(/<svg/g)||[]).length,5);assert.ok(dock.includes('Plus de menus'));
 const body=html.slice(html.indexOf('class="em-console-library'));
 structures.push(body.replace(/class="[^"]*"/g,'').replace(/data-layout="[^"]*"/g,''));
}
assert.equal(new Set(structures).size,10,'Les pages console doivent avoir dix arbres DOM distincts');
assert.deepEqual(interfaces.filter(x=>!retroModes.includes(x.id)).map(x=>x.id),['minimal','covers','consoles','favorites','resume','collections','detail','archives','multiplayer','dashboard','hub-canape','ligne-cinema','mosaique-joy','roue-retro','terminal-poche','cartouches','carte-epoques','double-ecran']);
console.log('10 pages console structurellement différentes ; dock de 5 boutons SVG ; identifiants préservés.');

const paged=renderToStaticMarkup(<ExperienceView mode="covers" selectedConsole={{id:'gba',name:'GBA'}} planetLetter="P" planet={{items:Array.from({length:60},(_,i)=>({title:`Poster ${String(i).padStart(2,'0')}`,source:`https://www.planetemu.net/rom/gba/p${i}`}))}}/>);
assert.ok(paged.includes('Poster 23'));assert.ok(!paged.includes('Poster 24'));assert.ok(paged.includes('Afficher la suite'));
console.log('Catalogue : premier lot limité à 24 jaquettes.');

const install=renderToStaticMarkup(<ExperienceView mode="covers" selectedConsole={{id:'nes',name:'NES'}} planetLetter="P" downloadable={[{title:'Palette libre',downloadId:'palette-libre',license:'CC0-1.0'}]}/>);
assert.ok(install.includes('gf-unified-catalogue'));assert.ok(install.includes('Catalogue · CC0-1.0'));assert.ok(install.includes('<button'));
console.log('Contenu du catalogue : fiche visible, ajout personnel séparé.');

assert.ok(!consoleHtml.includes('href="https://www.freeroms.com/"'));assert.ok(!consoleHtml.includes('Sources de jeux'));
console.log('Fournisseurs masqués dans le catalogue ; crédits conservés dans les paramètres.');
const union=renderToStaticMarkup(<ExperienceView mode="covers" selectedConsole={{id:'gba',name:'GBA'}} planetLetter="P" games={[{title:'pac-man-collection',consoleId:'gba',reference:'local'}]} planet={{items:[{title:'Pac-Man Collection (Europe)',source:'https://www.planetemu.net/rom/gba/pac-man'},{title:'Pac-Man Collection (Japan)',source:'https://www.planetemu.net/rom/gba/pac-man-jp'}]}}/>);
assert.equal((union.match(/<strong>Pac-Man Collection<\/strong>/g)||[]).length,1);
assert.ok(!union.includes('Installé · Bibliothèque · Planetemu'));assert.ok(union.includes('Installé'));assert.ok(union.includes('3 éditions'));
console.log('Catalogue commun : une seule carte pour les copies locales et les variantes externes.');
const globalSearch=renderToStaticMarkup(<ExperienceView mode="covers" selectedConsole={{id:'gba',name:'GBA'}} planetLetter="A" initialQuery="Pokemon" planet={{query:'pokemon',items:[{title:'Pokemon - Emerald Version (USA)',provider:'Planetemu'},{title:'Pokemon - Version Emeraude (France)',provider:'Planetemu'},{title:'Advance Test',provider:'Planetemu'}]}}/>);
assert.ok(globalSearch.includes('<strong>Pokémon Émeraude</strong>'));assert.ok(!globalSearch.includes('Lettre du catalogue'));assert.ok(!globalSearch.includes('<strong>Advance Test</strong>'));assert.ok(globalSearch.includes('gf-global-search'));
console.log('Recherche « Pokemon » avec lettre A : Pokémon Émeraude visible une seule fois, alphabet suspendu.');
const transfer={id:'transfer-preview',gameId:'freeroms:gba:free-demo',title:'Démo libre',consoleId:'gba',image:'/experience-api/v1/media/'+'a'.repeat(64),status:'running',phase:'downloading',progress:42,bytesReceived:420,totalBytes:1000,createdAt:Date.now()/1000,updatedAt:Date.now()/1000};
const inLibrary=renderToStaticMarkup(<ExperienceView mode="covers" section="library" games={[]} downloadJobs={[transfer]}/>);assert.ok(inLibrary.includes('<strong>Démo libre</strong>'));assert.ok(inLibrary.includes('42 %'));assert.ok(inLibrary.includes('aria-valuenow="42"'));assert.ok(inLibrary.includes('disabled=""'));
const inShop=renderToStaticMarkup(<ExperienceView mode="covers" selectedConsole={{id:'gba',name:'GBA'}} planetLetter="D" downloadJobs={[transfer]} planet={{items:[{title:'Démo libre (Europe)',consoleId:'gba',downloadId:'freeroms:gba:free-demo',provider:'FreeROMs'}]}}/>);assert.equal((inShop.match(/<strong>Démo libre<\/strong>/g)||[]).length,1);assert.ok(inShop.includes('42 %'));
console.log('Shop et bibliothèque : carte unique présente pendant le transfert, progression 42 %, lancement bloqué avant installation.');

for(const mode of retroModes){
 const home=renderToStaticMarkup(<ExperienceView mode={mode} games={games} consoles={consoles}/>);
 assert(home.includes(`data-interface="${mode}"`));assert(home.includes('data-system="nes"'));assert(!home.includes('data-game="free"'));
 const library=renderToStaticMarkup(<ExperienceView mode={mode} games={games} consoles={consoles} selectedConsole={consoles[0]} planet={{items:[{title:'Not installed',source:'https://example.org'}]}}/>);
 assert(library.includes('data-game="free"'));assert(!library.includes('Not installed'));
}
console.log('5 nouveaux parcours console → bibliothèque installée valides.');

const {selectableInterfaces,movedInterfaces,migratePreference,legacyDestination,preferenceKey,interfacePackCapacity}=require('../../src/webapp/src/app/gameflex/experience/interfaces.js');
assert.deepEqual(selectableInterfaces.map(x=>x.id),['station','crt','shelf','orbit','arcade','minimal','covers','consoles','dashboard','hub-canape','ligne-cinema','mosaique-joy','roue-retro','terminal-poche','cartouches','carte-epoques','double-ecran']);
assert.equal(interfacePackCapacity,22);
const saved={favorites:['nes:free'],history:['nes:free'],collections:[{id:'local',name:'Homebrew',games:['nes:free']}]};
for(const id of Object.keys(movedInterfaces)){
 const old={interface:id,...saved},next=migratePreference(old);
 assert.equal(next.interface,'station');assert.equal(next.previousInterface,id);
 assert.deepEqual(migratePreference(next),next);assert.deepEqual(old,{interface:id,...saved});
 for(const field of ['favorites','history','collections'])assert.deepEqual(next[field],saved[field]);
 assert(legacyDestination(id,saved.history,games));
}
assert.equal(legacyDestination('archives').section,'management');
assert.equal(legacyDestination('detail',saved.history,games).game.reference,'free');
assert.equal(legacyDestination('detail',[],games).game,undefined);
assert.notEqual(preferenceKey('alice','/gameflex'),preferenceKey('bob','/gameflex'));
for(const {id} of selectableInterfaces){
 for(const section of ['favorites','resume','collections','management','archives','multiplayer','library']){
  const html=renderToStaticMarkup(<ExperienceView mode={id} section={section} games={games} consoles={consoles} {...saved}/>);
  assert(html.includes('Palette libre'),`${id}/${section}`);
 }
 const pending=renderToStaticMarkup(<ExperienceView mode={id} section="management" loading games={[]} consoles={consoles}/>);
 assert(pending.includes('Chargement'));assert(!pending.includes('0 fichiers'));assert(!pending.includes('Aucun jeu ici'));
 const emptyFavorites=renderToStaticMarkup(<ExperienceView mode={id} section="favorites" games={games} consoles={consoles}/>);
 assert(!emptyFavorites.includes('Palette libre'),'Empty favorites must not show the library');
}
const singular=renderToStaticMarkup(<ExperienceView mode="consoles" games={games} consoles={consoles}/>);
assert(singular.includes('1 jeu installé'));assert(!singular.includes('1 jeux'));
const pack=renderToStaticMarkup(<InterfacePicker options={[...selectableInterfaces,...Array.from({length:5},(_,i)=>({id:'pack-'+i,name:'Pack '+i}))]} onChoose={()=>{}}/>);
assert.equal((pack.match(/class="gx-choice /g)||[]).length,22);
console.log('Migration idempotente des six anciens choix ; dix-sept modes × sept destinations ; chargement, singulier et pack de 22 vérifiés.');
