import React,{useState} from 'react';
import {createRoot} from 'react-dom/client';
import ExperienceView from '../../src/webapp/src/app/gameflex/experience/ExperienceView.jsx';
function Fixture(){const [letter,setLetter]=useState('');const games=['Zeta fixture','Alpha fixture','Golf fixture','12 fixture','Éclair fixture'].map((title,i)=>({title,consoleId:'gba',reference:'fixture-'+i}));return <ExperienceView mode="covers" section="consoles" selectedConsole={{id:'gba',name:'Game Boy Advance'}} games={games} planetLetter={letter} onPlanetLetter={setLetter}/>;}
createRoot(document.getElementById('root')).render(<Fixture/>);
