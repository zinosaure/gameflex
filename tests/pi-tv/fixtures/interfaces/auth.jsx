// Isolated preview identity: never consult the public account or its credentials.
export const useAuth=()=>({user:{username:'preview',display_name:'Aperçu',role_level:0},token:'fixture-only',isAdmin:true,isAuthenticated:true,logout:()=>{}});
export const authHeaders=()=>({});
export const getStoredToken=()=>null;

export const getStoredUsername=()=>"preview";
export const authApi=async()=>({ok:true,item:{},items:[]});
