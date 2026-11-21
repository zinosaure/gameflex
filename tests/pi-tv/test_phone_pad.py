import math,unittest
from phone_pad import Pairings,PadError
class PhonePadTests(unittest.TestCase):
 def setUp(self):self.now=10.;self.p=Pairings(lambda:self.now);self.tv=self.p.create('alice')
 def joined(self):return self.p.join(self.tv['code'],'Téléphone','fixture')
 def connected(self):
  phone=self.joined();host,hq=self.p.attach(self.tv['id'],'host',self.tv['hostToken'],'alice');pad,pq=self.p.attach(phone['id'],'phone',phone['phoneToken']);self.p.action(self.tv['id'],self.tv['hostToken'],'alice','approve');return host,hq,pad,pq
 def test_code_is_single_use_and_phone_has_no_account_token(self):
  phone=self.joined();self.assertEqual(set(phone),{'id','phoneToken'})
  with self.assertRaises(PadError):self.joined()
 def test_owner_and_capability_both_required(self):
  with self.assertRaises(PadError):self.p.action(self.tv['id'],self.tv['hostToken'],'bob','approve')
  with self.assertRaises(PadError):self.p.attach(self.tv['id'],'host','x'*43,'alice')
 def test_expired_code_refused(self):
  self.now+=121
  with self.assertRaises(PadError):self.joined()
 def test_no_input_before_tv_approval(self):
  phone=self.joined();self.p.attach(self.tv['id'],'host',self.tv['hostToken'],'alice');pad,q=self.p.attach(phone['id'],'phone',phone['phoneToken'])
  with self.assertRaises(PadError):self.p.message(phone['id'],'phone',pad,{'type':'pad'})
 def test_full_state_then_watchdog_releases(self):
  host,hq,pad,pq=self.connected();r=self.p.records[self.tv['id']]
  self.p.message(self.tv['id'],'phone',pad,{'type':'pad','buttons':[0,12],'axes':[.5,0,0,0],'seq':1,'epoch':r['epoch']})
  self.assertEqual(r['buttons'],[0,12]);self.now+=1.3;self.p.tick();self.assertEqual(r['buttons'],[]);self.assertEqual(r['axes'],[0,0,0,0])
 def test_old_sequence_and_previous_screen_are_ignored(self):
  host,hq,pad,pq=self.connected();r=self.p.records[self.tv['id']];epoch=r['epoch']
  self.p.message(self.tv['id'],'host',host,{'type':'context','screen':'game'})
  self.p.message(self.tv['id'],'phone',pad,{'type':'pad','buttons':[0],'axes':[0]*4,'seq':5,'epoch':epoch});self.assertEqual(r['buttons'],[])
  self.p.message(self.tv['id'],'phone',pad,{'type':'pad','buttons':[1],'axes':[0]*4,'seq':6,'epoch':r['epoch']})
  self.p.message(self.tv['id'],'phone',pad,{'type':'pad','buttons':[0],'axes':[0]*4,'seq':5,'epoch':r['epoch']});self.assertEqual(r['buttons'],[1])
 def test_duplicate_context_does_not_invalidate_next_button(self):
  host,hq,pad,pq=self.connected();r=self.p.records[self.tv['id']]
  self.p.message(self.tv['id'],'host',host,{'type':'context','screen':'locked'});epoch=r['epoch']
  self.p.message(self.tv['id'],'host',host,{'type':'context','screen':'locked'})
  self.assertEqual(r['epoch'],epoch)
  self.p.message(self.tv['id'],'phone',pad,{'type':'pad','buttons':[1],'axes':[0]*4,'seq':1,'epoch':epoch})
  self.assertEqual(r['buttons'],[1])
 def test_malformed_input_cannot_become_a_command(self):
  host,hq,pad,pq=self.connected();epoch=self.p.records[self.tv['id']]['epoch']
  for buttons,axes in [([99],[0]*4),([True],[0]*4),([],[math.nan,0,0,0]),([],[2,0,0,0])]:
   with self.assertRaises(PadError):self.p.message(self.tv['id'],'phone',pad,{'type':'pad','buttons':buttons,'axes':axes,'seq':1,'epoch':epoch})
  with self.assertRaises(PadError):self.p.message(self.tv['id'],'phone',pad,{'type':'context','screen':'game'})
 def test_disconnect_revocation_and_reconnect_are_neutral(self):
  host,hq,pad,pq=self.connected();r=self.p.records[self.tv['id']]
  self.p.message(self.tv['id'],'phone',pad,{'type':'pad','buttons':[0],'axes':[0]*4,'seq':1,'epoch':r['epoch']})
  self.p.detach(self.tv['id'],'phone',pad);self.assertEqual(r['buttons'],[])
  self.p.action(self.tv['id'],self.tv['hostToken'],'alice','revoke');self.assertNotIn(self.tv['id'],self.p.records)
 def test_join_rate_and_registry_size_are_bounded(self):
  for _ in range(20):
   with self.assertRaises(PadError):self.p.join('00000000','Phone','bad-client')
  with self.assertRaises(PadError):self.p.join('00000000','Phone','bad-client')
  self.p.create('alice')
  with self.assertRaises(PadError):self.p.create('alice')
if __name__=='__main__':unittest.main()
