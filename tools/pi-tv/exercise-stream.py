"""Private owner control channel for an explicitly selected generated test session."""
import argparse
import json
from pathlib import Path
import subprocess
p=argparse.ArgumentParser();p.add_argument('session');a=p.parse_args()
if not __import__('re').fullmatch('[a-f0-9]{32}',a.session):p.error('Invalid session')
root=Path(__file__).resolve().parents[2]
c=json.loads(subprocess.check_output(['docker','inspect','gameflex-credential']))[0]
e=dict(x.split('=',1) for x in c['Config']['Env'] if '=' in x)
payload={'username':e.get('GAMEFLEX_ROOT_USERNAME','root'),'password':e.get('GAMEFLEX_ROOT_PASSWORD',''),'sessionId':a.session}
process=subprocess.run(['docker','run','--rm','-i','--name','gameflex-stream-picture-check','--label','org.gameflex.project='+str(root),'--network','gameflex-local','--cpus','.5','--memory','192m','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','-v',str(root)+':/app:ro','node:20-bookworm-slim','node','/app/tools/pi-tv/exercise-stream.cjs'],input=json.dumps(payload),text=True)
raise SystemExit(process.returncode)
