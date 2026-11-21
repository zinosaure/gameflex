package org.gameflex.tv;
import java.net.URI;
public final class ServerPolicy {
 public static URI parse(String text) throws Exception {
  URI u=new URI(text.trim());String host=u.getHost();
  if(host==null||u.getRawUserInfo()!=null||u.getRawQuery()!=null||u.getRawFragment()!=null||!(u.getPath()==null||u.getPath().isEmpty()||u.getPath().equals("/")))throw new IllegalArgumentException();
  if(u.getPort()!=-1&&(u.getPort()<1||u.getPort()>65535))throw new IllegalArgumentException();
  boolean official="https".equals(u.getScheme())&&host.equalsIgnoreCase("gameflex.skino.app");
  boolean local=false;String[] parts=host.split("\\.");
  if(parts.length==4){int[] p=new int[4];try{for(int i=0;i<4;i++){p[i]=Integer.parseInt(parts[i]);if(p[i]<0||p[i]>255||!parts[i].equals(Integer.toString(p[i])))throw new Exception();}local=p[0]==10||(p[0]==192&&p[1]==168)||(p[0]==172&&p[1]>=16&&p[1]<=31);}catch(Exception ignored){local=false;}}
  if(!official&&!(local&&("http".equals(u.getScheme())||"https".equals(u.getScheme()))))throw new IllegalArgumentException();
  return new URI(u.getScheme(),null,host.toLowerCase(),u.getPort(),null,null,null);
 }
 public static boolean sameOrigin(URI origin,String url){try{URI u=new URI(url);return origin.getScheme().equals(u.getScheme())&&origin.getHost().equalsIgnoreCase(u.getHost())&&port(origin)==port(u);}catch(Exception e){return false;}}
 private static int port(URI u){return u.getPort()!=-1?u.getPort():"https".equals(u.getScheme())?443:80;}
}
