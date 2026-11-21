import json,re,sys,urllib.request
from pathlib import Path
from tv_device import TV
ROOT=Path(__file__).resolve().parents[2];OUT=ROOT/'.local/android-tv';OUT.mkdir(exist_ok=True)
try:
 with TV() as tv:
  report={'identity':tv.identity}
  for name,cmd in {'foreground':'dumpsys activity activities | /system/bin/grep mResumedActivity','power':'dumpsys power | /system/bin/grep mWakefulness','gameflex':'pm list packages --show-versioncode org.gameflex','webview':'dumpsys webviewupdate','moonlight':'pm list packages com.limelight','home':'cmd package resolve-activity --brief --user 0 -a android.intent.action.MAIN -c android.intent.category.HOME','display':'wm size','abis':'getprop ro.product.cpu.abilist'}.items():
   report[name]=tv.shell(cmd)
  media=tv.shell('dumpsys media_session');report['playingMediaSessions']=len(re.findall(r'state=PlaybackState \{state=3[, ]',media))
  report['bluetoothGameControllerPresent']=bool(re.search(r'(?i)(gamepad|joystick|xbox|dualshock|dualsense)',tv.shell('dumpsys input')))
  (OUT/'tv-audit.json').write_text(json.dumps(report,ensure_ascii=False,indent=2));print(json.dumps(report,ensure_ascii=False))
except Exception as e:
 print(json.dumps({'audit':'blocked','error':str(e) if str(e) in ('ADB_UNAUTHORIZED','IDENTITY_MISMATCH') else type(e).__name__}));sys.exit(1)
