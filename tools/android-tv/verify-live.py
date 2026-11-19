#!/usr/bin/env python3
"""Exercise the actual LAN API/player with the existing original CC0 NES demo.
Existing administrator credentials remain in memory and are passed on stdin.
The temporary browser uses host networking for direct UDP media; no host setting changes.
"""
import json,subprocess,urllib.request
from pathlib import Path
root=Path(__file__).resolve().parents[2];env=dict(x.split('=',1) for x in json.loads(subprocess.check_output(['docker','inspect','gameflex-credential']))[0]['Config']['Env'] if '=' in x)
req=urllib.request.Request('http://192.168.1.63:2000/api/v1/auth/login',data=json.dumps({'username':env.get('GAMEFLEX_ROOT_USERNAME','root'),'password':env.get('GAMEFLEX_ROOT_PASSWORD','')}).encode(),headers={'Content-Type':'application/json'})
with urllib.request.urlopen(req,timeout=20) as r:token=json.load(r)['item']['access_token']
result=subprocess.run(['docker','run','--rm','-i','--init','--network','host','--cpus','2','--memory','1024m','--pids-limit','256','--shm-size','128m','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--tmpfs','/tmp:rw,size=192m,mode=1777','--mount','type=bind,src='+str(root)+',dst=/app','-w','/app','gameflex-experience:tv-candidate','python','/app/tools/android-tv/verify-client.py'],input=json.dumps({'token':token}),text=True)
raise SystemExit(result.returncode)
