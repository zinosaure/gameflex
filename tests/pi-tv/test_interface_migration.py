import sys, unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
from server import validate_preferences

class InterfaceMigrationTests(unittest.TestCase):
 def test_round_trip_preserves_migration_and_library(self):
  for old in ('favorites','resume','collections','detail','archives','multiplayer'):
   p={'interface':'station','previousInterface':old,'interfaceDestination':old,'favorites':['nes:palette'],'history':['nes:palette'],'collections':[{'id':'test','name':'Test','games':['nes:palette']}]}
   self.assertEqual(validate_preferences(p),p)
   p['interfaceDestination']=None
   self.assertEqual(validate_preferences(p),p)
 def test_legacy_clients_still_accepted(self):
  for old in ('favorites','resume','collections','detail','archives','multiplayer'):
   self.assertEqual(validate_preferences({'interface':old})['interface'],old)
 def test_bad_destination_rejected(self):
  with self.assertRaises(ValueError):validate_preferences({'interface':'station','interfaceDestination':'untrusted'})

if __name__=='__main__':unittest.main()
