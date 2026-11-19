#!/usr/bin/env python3
"""Deploy the built Pi demonstration using Docker CLI, without installing Compose.
Reuses the existing identity service; secrets stay in process environments.
"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess

ROOT=Path(__file__).resolve().parents[2]
LABEL='org.gameflex.project='+str(ROOT)

def inspect(name):
    p=subprocess.run(['docker','inspect',name],capture_output=True,text=True)
    return json.loads(p.stdout)[0] if p.returncode==0 else None

def run(name,args,env=None):
    old=inspect(name)
    if old:
        if old['Config'].get('Labels',{}).get('org.gameflex.project')!=str(ROOT):
            raise RuntimeError('Un conteneur non géré occupe le nom '+name)
        if old['State']['Running']:
            print(name+': déjà en cours');return
        subprocess.run(['docker','start',name],check=True,stdout=subprocess.DEVNULL)
        print(name+': redémarré');return
    cmd=['docker','run','-d','--name',name,'--label',LABEL,'--restart','unless-stopped']+args
    p=subprocess.run(cmd,env=env,capture_output=True,text=True)
    if p.returncode: raise RuntimeError('Démarrage refusé pour '+name+' : '+p.stderr[:300])
    print(name+': démarré')

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bind',required=True,help='Adresse IPv4 LAN du Pi vérifiée depuis l’hôte')
    p.add_argument('--port',type=int,default=2000)
    p.add_argument('--demo',action='store_true',required=True,help='Déployer uniquement la démo originale ; aucun dossier personnel')
    p.add_argument('--recreate-interface',action='store_true',help='Remplacer uniquement les conteneurs web/API gérés, sans toucher aux données')
    p.add_argument('--interface-only',action='store_true',help='Servir les comptes et l’interface pendant le build du moteur')
    a=p.parse_args()
    import ipaddress
    address=ipaddress.IPv4Address(a.bind)
    if not address.is_private or address.is_loopback or address.is_unspecified: p.error('Adresse LAN privée requise')
    credential=inspect('gameflex-credential')
    if not credential or not credential['State']['Running']: p.error('Service gameflex-credential existant requis')
    cfg=dict(x.split('=',1) for x in credential['Config']['Env'] if '=' in x)
    if not cfg.get('GAMEFLEX_JWT_SECRET') or not cfg.get('GAMEFLEX_INTERNAL_SECRET'): p.error('Secrets stables requis dans le service credential')
    if a.recreate_interface:
        # Preflight everything before stopping a managed service.
        for image in ['gameflex-experience:pi-local','gameflex-web:pi-local']:
            if subprocess.run(['docker','image','inspect',image],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode:p.error('Image à construire : '+image)
        for name in ['gameflex-web','gameflex-experience']:
            current=inspect(name)
            if current and current['Config'].get('Labels',{}).get('org.gameflex.project')!=str(ROOT):p.error('Conteneur non géré : '+name)
        for pattern in ['downloads/*/*.json','metadata-jobs/*.json']:
            for path in (ROOT/'.local/pi-tv').glob(pattern):
                try:active=json.loads(path.read_text()).get('status') in ('queued','running')
                except (OSError,ValueError):active=False
                if active:p.error('Transfert ou rafraîchissement actif ; réessayez après sa fin.')
        for name in ['gameflex-web','gameflex-experience']:
            if inspect(name):
                subprocess.run(['docker','stop',name],check=True,stdout=subprocess.DEVNULL)
                subprocess.run(['docker','rm',name],check=True,stdout=subprocess.DEVNULL)
    if not a.interface_only:
        subprocess.run(['python3',str(ROOT/'tools/pi-tv/start-native-gateway.py')],check=True)
        subprocess.run(['python3',str(ROOT/'tools/pi-tv/start-relay.py')],check=True)
        for image in ['gameflex-kernel:pi-local','gameflex-retrogaming:pi-local','gameflex-web:pi-local']:
            if subprocess.run(['docker','image','inspect',image],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode: p.error('Image à construire : '+image)
        library=ROOT/'.local/demo-library'
        for d in ['roms/nes','cores/nes','cache/profiles','saves','logs']:(library/d).mkdir(parents=True,exist_ok=True)
        shutil.copy2(ROOT/'output/nes-demo/palette-libre.nes',library/'roms/nes/palette-libre.nes')
        shutil.copy2(ROOT/'.local/pi-tv/cores/quicknes_libretro.so',library/'cores/nes/quicknes_libretro.so')
        (library/'DEMONSTRATION.txt').write_text('Generated demonstration only; not the owner’s personal library.\nContent CC0; QuickNES core GPL-2.0 with LGPL components; not redistributed.\n')
        env=os.environ.copy()
        env.update({k:cfg[k] for k in ['GAMEFLEX_JWT_SECRET','GAMEFLEX_INTERNAL_SECRET']})
        env.update({'GAMEFLEX_PUBLIC_HOST':a.bind,'GAMEFLEX_STORAGE_PATH':str(library),'GAMEFLEX_PROFILES_HOST_DIR':str(library/'cache/profiles'),'GAMEFLEX_STACK_NAME':'gameflex-pi','GAMEFLEX_CREDENTIAL_URL':'http://gameflex-credential:8080','GAMEFLEX_KERNEL_URL':'http://127.0.0.1:10201','GAMEFLEX_GATEWAY_URL':'http://127.0.0.1:10202','GAMEFLEX_GATEWAY_CONTROL_URL':'http://gameflex-host-relay:10202','GAMEFLEX_INTERNAL_HOST':'gameflex-host-relay','GAMEFLEX_RETRO_IMAGE':'gameflex-retrogaming:pi-local','GAMEFLEX_REMOTEPLAY_IMAGE':'gameflex-remoteplay:not-installed','GAMEFLEX_MAX_PLAYERS':'2','GAMEFLEX_PORT_RANGE':'9001-9010','GAMEFLEX_STAGING':'false','GOMAXPROCS':'2'})
        args=['--network','gameflex-local','--cpus','1','--memory','384m','--pids-limit','128','--security-opt','no-new-privileges:true','-p','127.0.0.1:10201:8080','--mount','type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock','--mount','type=bind,src='+str(library)+',dst=/gameflex','--mount','type=bind,src='+str(library/'cache/profiles')+',dst=/profiles']
        for k in ['GAMEFLEX_JWT_SECRET','GAMEFLEX_INTERNAL_SECRET','GAMEFLEX_PUBLIC_HOST','GAMEFLEX_STORAGE_PATH','GAMEFLEX_PROFILES_HOST_DIR','GAMEFLEX_STACK_NAME','GAMEFLEX_CREDENTIAL_URL','GAMEFLEX_KERNEL_URL','GAMEFLEX_GATEWAY_URL','GAMEFLEX_GATEWAY_CONTROL_URL','GAMEFLEX_INTERNAL_HOST','GAMEFLEX_RETRO_IMAGE','GAMEFLEX_REMOTEPLAY_IMAGE','GAMEFLEX_MAX_PLAYERS','GAMEFLEX_PORT_RANGE','GAMEFLEX_STAGING','GOMAXPROCS']:args+=['-e',k]
        run('gameflex-kernel',args+['gameflex-kernel:pi-local'],env)
    data=ROOT/'.local/pi-tv';data.mkdir(parents=True,exist_ok=True)
    os.chown(data,1000,1000)
    run('gameflex-experience',['--init','--network','gameflex-local','--user','1000:1000','--cpus','1.5','--memory','1024m','--pids-limit','256','--shm-size','128m','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--tmpfs','/tmp:rw,size=192m,mode=1777','--mount','type=bind,src='+str(ROOT)+',dst=/app,readonly','--mount','type=bind,src='+str(data)+',dst=/data','--mount','type=bind,src='+str(ROOT/'.local/demo-library')+',dst=/library','gameflex-experience:pi-local','python3','/app/tools/pi-tv/server.py','--bind','0.0.0.0','--data','/data','--auth-base','http://gameflex-credential:8080','--library-host',str(ROOT/'.local/demo-library'),'--library-mount','/library','--default-roms'])
    run('gameflex-web',['--network','gameflex-local','--user','101:101','--cpus','.5','--memory','128m','--pids-limit','64','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--tmpfs','/tmp:rw,size=16m','--tmpfs','/var/cache/nginx:rw,mode=1777,size=32m','--tmpfs','/var/run:rw,mode=1777,size=1m','-p',a.bind+':'+str(a.port)+':8080','gameflex-web:pi-local'])
    print('Adresse locale : http://'+a.bind+':'+str(a.port))
if __name__=='__main__': main()
