// Production build using the committed configuration. Bound workers for the Pi host.
import path from 'path';
import {fileURLToPath} from 'url';
import configFactory from '../../src/webapp/src/webpack.config.js';
import webpack from '../../src/webapp/src/node_modules/webpack/lib/index.js';
import MinimizerPlugin from '../../src/webapp/src/node_modules/minimizer-webpack-plugin/dist/index.js';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
MinimizerPlugin.getAvailableNumberOfCores=()=>0;
const config=configFactory();config.mode='production';
config.context=path.join(root,'src/webapp/src');
config.output={...config.output,path:path.join(root,'.local/console-release/web')};
webpack(config,(error,stats)=>{
 if(error||stats.hasErrors()){console.error(error||stats.toString({all:false,errors:true}));process.exitCode=1;}
 else console.log(stats.toString({all:false,warnings:true,timings:true}));
});
