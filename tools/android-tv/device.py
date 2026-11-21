#!/usr/bin/env python3
"""Install/launch GameFlex only, after explicit TV-free confirmation."""
import argparse,hashlib,json,subprocess,time,shlex
from pathlib import Path
from tv_device import TV
ROOT=Path(__file__).resolve().parents[2];OUT=ROOT/'output/android-tv';PKG='org.gameflex.tv';TOOL=Path('/home/config/LoutreTV/.toolchain');SIGNER=TOOL/'android-sdk/build-tools/35.0.0/lib/apksigner.jar';JAVA=TOOL/'jdk/bin/java'
def certificate(path):
 raw=subprocess.check_output([str(JAVA),'-jar',str(SIGNER),'verify','--print-certs',str(path)],text=True)
 return next(s.split(':',1)[1].strip() for s in raw.splitlines() if 'certificate SHA-256 digest' in s)
def main():
 p=argparse.ArgumentParser();p.add_argument('action',choices=['install','launch']);p.add_argument('--tv-free',action='store_true');a=p.parse_args()
 if not a.tv_free:raise SystemExit('TV availability confirmation required before installation or launch.')
 info=json.loads((OUT/'apk.json').read_text());apk=Path(info['apk']);assert hashlib.sha256(apk.read_bytes()).hexdigest()==info['sha256']
 with TV() as tv:
  installed='package:'+PKG in tv.shell('pm list packages '+PKG).splitlines()
  if a.action=='launch':
   if not installed:raise RuntimeError('GameFlex not installed')
   print(tv.shell('am start -W -n org.gameflex.tv/.MainActivity'));return
  backup=ROOT/'.local/android-tv/rollback'/str(int(time.time()));backup.mkdir(parents=True)
  baseline={'identity':tv.identity,'home':tv.shell('cmd package resolve-activity --brief --user 0 -a android.intent.action.MAIN -c android.intent.category.HOME'),'enabled':tv.shell('pm list packages -e --user 0').splitlines(),'disabled':tv.shell('pm list packages -d --user 0').splitlines(),'previouslyInstalled':installed}
  if installed:
   paths=tv.shell('pm path '+PKG).splitlines()
   for i,path in enumerate(paths):
    local=backup/('previous-'+str(i)+'.apk');tv.device.pull(path.removeprefix('package:'),str(local),transport_timeout_s=30,read_timeout_s=120)
    if certificate(local)!=info['certificateSha256']:raise RuntimeError('SIGNATURE_MISMATCH: existing GameFlex preserved')
   version=tv.shell('dumpsys package '+PKG)
   import re
   current=int(re.search(r'versionCode=(\d+)',version)[1]);baseline['previousVersionCode']=current
   if current>info['versionCode']:raise RuntimeError('Existing GameFlex is newer; preserved')
  (backup/'baseline.json').write_text(json.dumps(baseline,indent=2))
  remote='/data/local/tmp/gameflex-'+info['sha256'][:16]+'-'+str(int(time.time()))+'.apk'
  tv.device.push(str(apk),remote,transport_timeout_s=30,read_timeout_s=300)
  if tv.shell('sha256sum '+remote,timeout=60).split()[0]!=info['sha256']:raise RuntimeError('Transfer incomplete; no package installation attempted')
  result=tv.shell('pm install '+('-r ' if installed else '')+remote,timeout=180)
  if 'Success' not in result:raise RuntimeError('Package installation refused: '+result)
  tv.shell('rm '+remote)
  final=tv.shell('dumpsys package '+PKG);assert 'versionName='+info['versionName'] in final
  launcher=tv.shell('cmd package query-activities --brief --user 0 -a android.intent.action.MAIN -c android.intent.category.LEANBACK_LAUNCHER '+PKG)
  assert PKG in launcher
  report={'installed':True,'package':PKG,'version':info['versionName'],'sha256':info['sha256'],'identity':tv.identity,'launcherEntry':launcher,'rollback':str(backup),'previouslyInstalled':installed}
  (OUT/'installation.json').write_text(json.dumps(report,indent=2));print(json.dumps(report))
if __name__=='__main__':
 try:main()
 except Exception as e:print('Stopped:',str(e));raise SystemExit(1)
