import React from 'react';
import {createRoot} from 'react-dom/client';
import {EmugenSources} from '../../src/webapp/src/app/gameflex/experience/EmugenDetails.jsx';
const game={title:'Démo libre',consoleId:'wii',emugenSlug:'free-demo-wii',emugenUrl:'https://www.emugen.net/jeux/free-demo-wii/'};
createRoot(document.getElementById('root')).render(<EmugenSources game={game} token="fixture-only"/>);
