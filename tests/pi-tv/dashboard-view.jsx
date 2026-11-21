import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import assert from 'node:assert/strict';
import ExperienceView from '../../src/webapp/src/app/gameflex/experience/ExperienceView.jsx';

const consoles=[{id:'gba',name:'Game Boy Advance',year:2001}];
const games=[
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'saved',title:'Aventure avec sauvegarde compatible au titre très long',saveState:{slot:1},saveCompatible:true,image:'/missing'},
 {consoleId:'gba',consoleName:'Game Boy Advance',reference:'recent',title:'Jeu joué sans sauvegarde'},
];
const base={mode:'dashboard',games,consoles,section:'home',favorites:['gba:saved'],collections:[],onOpen:()=>{},onConsole:()=>{},onNavigate:()=>{},onSettings:()=>{}};
const resume=renderToStaticMarkup(<ExperienceView {...base} history={['gba:saved']}/>);
assert.ok(resume.includes('em-dashboard-resume'));
assert.ok(resume.includes('Reprendre'));
assert.ok(resume.includes('Aventure avec sauvegarde compatible'));
assert.ok(!resume.includes('%'));
assert.ok(resume.includes('1 favori'));
assert.ok(resume.includes('1 système'));
assert.ok(resume.includes('Derniers jeux ajoutés'));
const relaunch=renderToStaticMarkup(<ExperienceView {...base} history={['gba:recent']}/>);
assert.ok(relaunch.includes('Relancer'));
assert.ok(!relaunch.includes('Reprendre'));
const empty=renderToStaticMarkup(<ExperienceView {...base} games={[]} history={[]}/>);
assert.ok(empty.includes('Votre bibliothèque est vide'));
console.log('Tableau de bord : Reprendre/Relancer, compteurs réels, ajouts récents, titre long, image absente et vide valides.');
