import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import CatalogGameDialog from '../../src/webapp/src/app/gameflex/experience/CatalogGameDialog.jsx';
import {mergeGames} from '../../src/webapp/src/app/gameflex/experience/merge-games.js';
import {selectableInterfaces} from '../../src/webapp/src/app/gameflex/experience/interfaces.js';
const game=mergeGames([{provider:'Planetemu',items:[{title:'Pokemon - Emerald Version (USA)',source:'https://www.planetemu.net/rom/gba/en'},{title:'Pokemon - Version Emeraude (France)',source:'https://www.planetemu.net/rom/gba/fr'},{title:'Pokemon - Edicion Esmeralda (Spain)',source:'https://www.planetemu.net/rom/gba/es'}]}],'gba')[0];
const html=renderToStaticMarkup(<CatalogGameDialog game={game} onClose={()=>{}}/>);
assert.ok(html.includes('Pokémon Émeraude'));assert.ok(html.includes('Langue et édition'));assert.ok(html.includes('value="1" selected=""'));assert.ok(html.toLowerCase().includes('non installé'));assert.ok(html.includes('Français'));assert.ok(html.includes('Espagnol'));assert.ok(!html.includes('role="alert"'));assert.ok(!html.includes('>Télécharger</button>'));assert.ok(!html.includes('Jeu indisponible'));assert.ok(!html.includes('https://www.planetemu.net/rom/gba/fr'));assert.ok(!html.includes('Source de la fiche'));
const owned=mergeGames([{provider:'Bibliothèque',items:[{title:'Pokemon - Emerald Version (USA)',reference:'own-en',consoleId:'gba'}]},{provider:'Planetemu',items:[{title:'Pokemon - Version Emeraude (France)',source:'https://www.planetemu.net/rom/gba/fr'}]}],'gba')[0];
const own=renderToStaticMarkup(<CatalogGameDialog game={owned} onClose={()=>{}}/>);assert.ok(own.includes('>Jouer</button>'));assert.ok(own.includes('value="0" selected=""'));
for(const {id} of selectableInterfaces){const themed=renderToStaticMarkup(<CatalogGameDialog mode={id} game={owned} onClose={()=>{}}/>);assert.ok(themed.includes(`gf-theme-${id}`));assert.ok(themed.includes(`data-interface-theme="${id}"`));}
console.log('Fiche du jeu : français par défaut, versions conservées, édition installée prioritaire, aucune fausse indisponibilité ni installation commerciale annoncée.');
