class ServerPolicyTest {
 public static void main(String[] args)throws Exception{
  String[] allowed={"http://192.168.1.63:2000","https://gameflex.skino.app","https://gameflex.skino.app/","http://10.0.0.1:2000"};
  for(String s:allowed)org.gameflex.tv.ServerPolicy.parse(s);
  String[] denied={"https://evil.example","http://gameflex.skino.app","file:///etc/passwd","http://192.168.1.63@evil.example","http://192.168.1.63:0","http://192.168.1.63:2000/evil","http://192.168.1.63:2000?token=bad","http://192.168.1.063:2000","http://127.0.0.1"};
  for(String s:denied){boolean rejected=false;try{org.gameflex.tv.ServerPolicy.parse(s);}catch(Exception e){rejected=true;}if(!rejected)throw new AssertionError("Invalid server accepted");}
  if(!org.gameflex.tv.ServerPolicy.sameOrigin(org.gameflex.tv.ServerPolicy.parse("https://gameflex.skino.app"),"https://gameflex.skino.app:443/streaming"))throw new AssertionError("Canonical origin");
  System.out.println("Server policy: 14 checks passed");
 }
}
