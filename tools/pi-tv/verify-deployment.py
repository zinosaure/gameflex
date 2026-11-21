import json,subprocess,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
c=json.loads(subprocess.check_output(['docker','inspect','gameflex-credential']))[0]
e=dict(x.split('=',1) for x in c['Config']['Env'] if '=' in x)
payload={'username':e.get('GAMEFLEX_ROOT_USERNAME','root'),'password':e.get('GAMEFLEX_ROOT_PASSWORD',''),'testLaunch':'--launch' in sys.argv,'keepLaunch':'--keep' in sys.argv,'testCatalogue':'--catalogue' in sys.argv,'testFilter':'--filter' in sys.argv}
if '--stop' in sys.argv:
    sid=sys.argv[sys.argv.index('--stop')+1]
    if not __import__('re').fullmatch('[a-f0-9]{32}',sid):raise SystemExit('Invalid test session')
    payload['stopExisting']=sid
p=subprocess.run(['docker','run','--rm','-i','--network','gameflex-local','--cpus','.5','--memory','192m','--mount','type=bind,src='+str(ROOT)+',dst=/app,readonly','node:20-bookworm-slim','node','/app/tools/pi-tv/verify-live.cjs'],input=json.dumps(payload),capture_output=True,text=True)
print(p.stdout.strip() or 'Verification unavailable')
if p.returncode:raise SystemExit(1)
