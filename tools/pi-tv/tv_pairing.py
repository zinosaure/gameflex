"""Short-lived Android TV association using the existing verified GameFlex JWT.
Only hashes of polling secrets are retained; JWTs stay in RAM until one delivery.
"""
import hashlib,hmac,re,secrets,threading,time
LOCK=threading.RLock();RECORDS={};RATE={};TTL=180
class PairError(ValueError):pass
def cleanup():
 now=time.time()
 for key in list(RECORDS):
  if RECORDS[key]['expiresAt']<=now:RECORDS.pop(key,None)
 for key in list(RATE):
  if now-RATE[key][0]>60:RATE.pop(key,None)
def limit(key,maximum):
 now=time.time();at,count=RATE.get(key,(now,0))
 if now-at>60:at,count=now,0
 if count>=maximum:raise PairError('Trop de tentatives. Réessayez dans une minute.')
 RATE[key]=(at,count+1)
def create(challenge,client):
 if not isinstance(challenge,str) or not re.fullmatch('[a-f0-9]{64}',challenge):raise PairError('Association invalide.')
 with LOCK:
  cleanup();limit('create:'+client,8)
  if len(RECORDS)>=16:raise PairError('Associations occupées. Réessayez.')
  code=''.join(secrets.choice('0123456789') for _ in range(6))
  while any(r['code']==code for r in RECORDS.values()):code=''.join(secrets.choice('0123456789') for _ in range(6))
  identifier=secrets.token_hex(16);expires=time.time()+TTL
  RECORDS[identifier]={'code':code,'challenge':challenge,'expiresAt':expires,'token':None}
  return {'id':identifier,'code':code,'expiresAt':expires}
def approve(code,token,user):
 if not isinstance(code,str) or not re.fullmatch('[0-9]{6}',code):raise PairError('Code invalide.')
 with LOCK:
  cleanup();limit('approve:'+user,6)
  record=next((r for r in RECORDS.values() if hmac.compare_digest(r['code'],code)),None)
  if not record or record['token']:raise PairError('Code expiré ou déjà utilisé.')
  record['token']=token
  return {'status':'approved'}
def poll(identifier,secret):
 if not isinstance(secret,str) or not re.fullmatch('[a-f0-9]{64}',secret):raise PairError('Association invalide.')
 with LOCK:
  cleanup();record=RECORDS.get(identifier)
  if not record:return {'status':'expired'}
  if not hmac.compare_digest(hashlib.sha256(secret.encode()).hexdigest(),record['challenge']):raise PairError('Association refusée.')
  if not record['token']:return {'status':'pending','expiresAt':record['expiresAt']}
  token=record['token'];RECORDS.pop(identifier,None)
  return {'status':'approved','accessToken':token}
PAGE='''<!doctype html><html lang="fr"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Associer GameFlex TV</title><style>body{background:#0b1326;color:#fff;font:18px system-ui;max-width:540px;margin:12vh auto;padding:24px}input,button{font:inherit;padding:18px;box-sizing:border-box;width:100%;margin:12px 0;border-radius:8px;border:1px solid #678;background:#111e38;color:white}button{background:#345eff}p{line-height:1.6;color:#bac6df}a{color:#8cb6ff}</style><h1>Associer GameFlex TV</h1><p>Sur le téléviseur devant vous, vérifiez le code à six chiffres. Cette action associe votre compte GameFlex à cette TV.</p><form><label for="code">Code affiché sur la TV</label><input id="code" inputmode="numeric" autocomplete="off" maxlength="6" pattern="[0-9]{6}" required><button>Autoriser cette TV</button></form><p id="result" role="status"></p><a href="/">Connexion à GameFlex</a><script>document.querySelector('form').onsubmit=async e=>{e.preventDefault();const t=localStorage.getItem('retrogaming-users-token');const r=document.querySelector('#result');if(!t){r.textContent='Connectez-vous à GameFlex avec le lien ci-dessous, puis revenez sur cette page.';return;}try{const v=await fetch('/experience-api/v1/tv/pairing/approve',{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+t},body:JSON.stringify({code:document.querySelector('#code').value.trim()})});const d=await v.json();r.textContent=v.ok?'TV autorisée. La bibliothèque va apparaître sur le téléviseur.':d.error||'Association refusée.';if(v.ok)document.querySelector('#code').value='';}catch{r.textContent='Serveur indisponible. Réessayez.';}};</script></html>'''
