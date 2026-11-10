"""Start the Pi-native LAN-only adapter, reusing stable project secrets privately."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
root=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('deploy',root/'tools/pi-tv/deploy-local.py')
deploy=importlib.util.module_from_spec(spec);spec.loader.exec_module(deploy)
old=deploy.inspect('gameflex-gateway')
if old and old['Config'].get('Labels',{}).get('org.gameflex.project')!=str(root):raise RuntimeError('Unmanaged gateway')
wanted=json.loads(subprocess.check_output(['docker','image','inspect','gameflex-gateway:pi-native']))[0]['Id']
if old and old['Image']==wanted:
    if not old['State']['Running']:subprocess.run(['docker','start','gameflex-gateway'],check=True,stdout=subprocess.DEVNULL)
    print('Native gateway already available');raise SystemExit(0)
credential=deploy.inspect('gameflex-credential')
config=dict(v.split('=',1) for v in credential['Config']['Env'] if '=' in v)
env=os.environ.copy();env['GAMEFLEX_INTERNAL_SECRET']=config['GAMEFLEX_INTERNAL_SECRET'];env['GAMEFLEX_ICE_INTERFACE']='eth0';env['GOMAXPROCS']='2'
env['GAMEFLEX_STUN_URL']=os.environ.get('GAMEFLEX_STUN_URL','stun:stun.l.google.com:19302')
# A prepared image is required before replacing the previous adapter.
subprocess.run(['docker','image','inspect','gameflex-gateway:pi-native'],check=True,stdout=subprocess.DEVNULL)
if old:
    subprocess.run(['docker','stop','gameflex-gateway'],check=True,stdout=subprocess.DEVNULL)
    subprocess.run(['docker','rm','gameflex-gateway'],check=True,stdout=subprocess.DEVNULL)
deploy.run('gameflex-gateway',['--network','host','--user','1000:1000','--cpus','1','--memory','256m','--pids-limit','128','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','-e','GAMEFLEX_INTERNAL_SECRET','-e','GAMEFLEX_ICE_INTERFACE','-e','GAMEFLEX_STUN_URL','-e','GOMAXPROCS','gameflex-gateway:pi-native'],env)
