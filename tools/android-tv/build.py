#!/usr/bin/env python3
"""Build/sign the Java-only TV APK using the existing read-only neighbour SDK."""
from pathlib import Path
import hashlib,json,os,secrets,subprocess,zipfile,shutil,xml.etree.ElementTree as ET
ROOT=Path(__file__).resolve().parents[2];ANDROID=ROOT/'deploy/pi-tv/android';SRC=ANDROID/'app/src/main';WORK=ROOT/'.local/android-tv/build';SIGN=Path(os.environ.get('GAMEFLEX_TV_SIGNING_DIR',str(ROOT/'.local/android-tv/signing')));OUT=ROOT/'output/android-tv';TC=Path('/home/config/LoutreTV/.toolchain');JDK=TC/'jdk/bin';SDK=TC/'android-sdk';TOOLS=SDK/'build-tools/35.0.0';PLATFORM=SDK/'platforms/android-35/android.jar'
for directory in [WORK,SIGN,OUT]:directory.mkdir(parents=True,exist_ok=True)
SIGN.chmod(0o700);os.umask(0o077)
for name in ['classes','generated','dex','tmp']:(WORK/name).mkdir(exist_ok=True)
env={**os.environ,'JAVA_HOME':str(TC/'jdk'),'TMPDIR':str(WORK/'tmp')}
def run(*args):subprocess.run([str(a) for a in args],check=True,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
try:
 run(TC/'aapt2','compile','--dir',SRC/'res','-o',WORK/'resources.zip')
 manifest=ET.parse(SRC/'AndroidManifest.xml');manifest.getroot().set('package','org.gameflex.tv');manifest.write(WORK/'AndroidManifest.xml',encoding='utf-8',xml_declaration=True)
 run(TC/'aapt2','link','-o',WORK/'unsigned.apk','--manifest',WORK/'AndroidManifest.xml','-I',PLATFORM,'--java',WORK/'generated','-A',SRC/'assets','--min-sdk-version','26','--target-sdk-version','35','--version-code','4','--version-name','0.3.1',WORK/'resources.zip')
 sources=list((SRC/'java').rglob('*.java'))+list((WORK/'generated').rglob('*.java'))
 run(JDK/'javac','-source','8','-target','8','-bootclasspath',str(PLATFORM)+os.pathsep+str(TOOLS/'core-lambda-stubs.jar'),'-d',WORK/'classes',*sources)
 run(JDK/'jar','cf',WORK/'classes.jar','-C',WORK/'classes','.')
 run(JDK/'java','-XX:ActiveProcessorCount=2','-Xmx512m','-Djava.io.tmpdir='+str(WORK/'tmp'),'-cp',TOOLS/'lib/d8.jar','com.android.tools.r8.D8','--release','--min-api','26','--lib',PLATFORM,'--output',WORK/'dex',WORK/'classes.jar')
 with zipfile.ZipFile(WORK/'unsigned.apk','a') as archive:
  for file in (WORK/'dex').glob('*.dex'):archive.write(file,file.name)
 align=WORK/'zipalign';shutil.copyfile(TOOLS/'zipalign',align);align.chmod(0o700)
 run(TC/'qemu-root/usr/bin/qemu-x86_64','-L',TC/'x86-root','-E','LD_LIBRARY_PATH='+str(TOOLS/'lib64'),align,'-f','4',WORK/'unsigned.apk',WORK/'aligned.apk')
 password=SIGN/'store.pass';key=SIGN/'gameflex-tv.p12'
 if not password.exists():password.write_text(secrets.token_urlsafe(48));password.chmod(0o600)
 if not key.exists():
  run(JDK/'keytool','-genkeypair','-alias','gameflex-tv','-keyalg','RSA','-keysize','3072','-validity','7300','-dname','CN=GameFlex TV Personal, OU=GameFlex','-keystore',key,'-storetype','PKCS12','-storepass:file',password,'-keypass:file',password)
  key.chmod(0o600)
 apk=OUT/'GameFlex-TV-0.3.1.apk'
 run(JDK/'java','-jar',TOOLS/'lib/apksigner.jar','sign','--ks',key,'--ks-key-alias','gameflex-tv','--ks-pass','file:'+str(password),'--v1-signing-enabled','true','--v2-signing-enabled','true','--out',apk,WORK/'aligned.apk')
 cert=subprocess.check_output([str(JDK/'java'),'-jar',str(TOOLS/'lib/apksigner.jar'),'verify','--verbose','--print-certs',str(apk)],text=True)
 (OUT/'signature.txt').write_text(cert)
 info={'apk':str(apk),'package':'org.gameflex.tv','activity':'org.gameflex.tv.MainActivity','versionName':'0.3.1','versionCode':4,'minSdk':26,'targetSdk':35,'sha256':hashlib.sha256(apk.read_bytes()).hexdigest(),'bytes':apk.stat().st_size,'gitHead':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),'commitCreated':False,'certificateSha256':next(line.split(':',1)[1].strip() for line in cert.splitlines() if 'certificate SHA-256 digest' in line)}
 (OUT/'apk.json').write_text(json.dumps(info,indent=2));apk.chmod(0o644);(OUT/'apk.json').chmod(0o644);(OUT/'signature.txt').chmod(0o644)
 print(json.dumps(info))
except subprocess.CalledProcessError as error:
 # These build commands never contain a password/token value.
 print((error.stderr or b'Build failed').decode(errors='replace')[-10000:]);raise SystemExit(1)
