"""Single authorized TV, pinned identity, no shared ADB server or key copying."""
import hashlib,json,sys,select,time,re
from pathlib import Path
LEGACY=Path('/home/config/tv-optimizer')
sys.path.insert(0,str(LEGACY/'scripts/runtime/python'))
from adb_shell.adb_device import AdbDevice
from adb_shell.auth.sign_pythonrsa import PythonRSASigner
from adb_shell.transport.tcp_transport import TcpTransport
TARGET='192.168.1.175';PORT=5555
class ReliableTransport(TcpTransport):
 def bulk_write(self,data,transport_timeout_s):
  view=memoryview(data);sent=0;deadline=time.monotonic()+(transport_timeout_s or 300)
  while sent<len(view):
   remaining=deadline-time.monotonic()
   if remaining<=0:raise TimeoutError('ADB transfer deadline')
   _,ready,_=select.select([],[self._connection],[],remaining)
   if not ready:raise TimeoutError('ADB transfer deadline')
   try:n=self._connection.send(view[sent:])
   except BlockingIOError:continue
   if n<=0:raise ConnectionError('ADB transport closed')
   sent+=n
  return sent
class TV:
 def __enter__(self):
  self.device=AdbDevice(ReliableTransport(TARGET,PORT),default_transport_timeout_s=20)
  if not self.device.connect(rsa_keys=[PythonRSASigner.FromRSAKeyPath(str(LEGACY/'scripts/runtime/tv-adbkey'))],auth_timeout_s=5):raise RuntimeError('ADB_UNAUTHORIZED')
  known=json.loads((LEGACY/'backups/device_identity.json').read_text())
  model=self.shell('getprop persist.sys.set_type');serial=self.shell('getprop ro.serialno')
  if model!=known['model'] or hashlib.sha256(serial.encode()).hexdigest()!=known['serial_sha256']:
   self.device.close();raise RuntimeError('IDENTITY_MISMATCH')
  self.identity={'model':model,'serialHashMatches':True,'target':TARGET+':'+str(PORT),'android':self.shell('getprop ro.build.version.release'),'firmware':self.shell('getprop ro.build.display.id')}
  # The reference also pins the Wi-Fi address. Compare in memory, never print it.
  mac=self.shell('cat /sys/class/net/wlan0/address')
  self.identity['wifiReadable']=bool(re.fullmatch(r'[0-9a-fA-F:]{17}',mac))
  self.identity['wifiMatches']=mac.lower()==known.get('wifi_mac','').lower() if self.identity['wifiReadable'] else None
  if self.identity['wifiReadable'] and known.get('wifi_mac') and not self.identity['wifiMatches']:
   self.device.close();raise RuntimeError('IDENTITY_MISMATCH')
  return self
 def shell(self,command,timeout=30):return self.device.shell(command,transport_timeout_s=20,read_timeout_s=timeout,timeout_s=timeout).strip()
 def __exit__(self,*args):self.device.close()
