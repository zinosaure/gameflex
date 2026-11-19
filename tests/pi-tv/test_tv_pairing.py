import hashlib,sys,time,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import tv_pairing as p
class PairingTests(unittest.TestCase):
 def setUp(self):p.RECORDS.clear();p.RATE.clear();self.secret='a'*64;self.challenge=hashlib.sha256(self.secret.encode()).hexdigest()
 def test_pending_approve_deliver_once(self):
  r=p.create(self.challenge,'fixture');self.assertEqual(p.poll(r['id'],self.secret)['status'],'pending')
  p.approve(r['code'],'fixture-token','alice');self.assertEqual(p.poll(r['id'],self.secret)['accessToken'],'fixture-token');self.assertEqual(p.poll(r['id'],self.secret)['status'],'expired')
 def test_wrong_poll_secret_and_expiry(self):
  r=p.create(self.challenge,'fixture')
  with self.assertRaises(p.PairError):p.poll(r['id'],'b'*64)
  p.RECORDS[r['id']]['expiresAt']=time.time()-1;self.assertEqual(p.poll(r['id'],self.secret)['status'],'expired')
 def test_cannot_reapprove_or_get_secret_in_creation_response(self):
  r=p.create(self.challenge,'fixture');self.assertNotIn('token',r);self.assertNotIn('challenge',r)
  p.approve(r['code'],'token','alice')
  with self.assertRaises(p.PairError):p.approve(r['code'],'another','bob')
 def test_invalid_input_and_rate_limit(self):
  for value in ['',None,'x'*64]:
   with self.assertRaises(p.PairError):p.create(value,'fixture')
  for _ in range(8):p.create(self.challenge,'fixture')
  with self.assertRaises(p.PairError):p.create(self.challenge,'fixture')
