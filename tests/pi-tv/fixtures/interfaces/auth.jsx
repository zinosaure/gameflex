// Isolated preview identity: never consult the public account or its credentials.
export const useAuth=()=>({user:{username:'preview',display_name:'Aperçu',role_level:0},token:'fixture-only'});
export const authHeaders=()=>({});
export const getStoredToken=()=>null;
