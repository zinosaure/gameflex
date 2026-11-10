import tempfile,unittest
from unittest.mock import patch
from covers import search,decode
class CoverSearchTests(unittest.TestCase):
 def test_installed_library_gets_lazy_attributed_covers(self):
  with tempfile.TemporaryDirectory() as folder,patch('covers.prefetch') as warm:
   item={'consoleId':'gba','reference':'abc','title':'Advance Wars (Europe) (En,Fr).gba'}
   value=search(folder,[item,item,{'consoleId':'unknown','reference':'a','title':'Test'}])
   self.assertEqual(len(value['items']),1)
   self.assertTrue(value['items'][0]['image'].startswith('/experience-api/v1/covers/gba/'))
   self.assertIn('Ayants droit',value['items'][0]['attribution'])
   self.assertEqual(decode('gba',value['items'][0]['image'].rsplit('/',1)[1]),'Advance Wars (Europe) (En,Fr)')
   warm.assert_called_once_with(folder,value['items'])
 def test_malformed_batch_is_rejected(self):
  for items in [None,{},[None],[{}]*2001]:
   with self.assertRaises(ValueError):search('/unused',items,False)
if __name__=='__main__':unittest.main()
