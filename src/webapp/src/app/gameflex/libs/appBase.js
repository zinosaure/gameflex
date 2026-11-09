// The LAN/root and the shared HTTPS route use the same build.
export function appBasePath() {
    if (typeof window === "undefined") return "";
    return /^\/gameflex(?:\/|$)/.test(window.location.pathname) ? "/gameflex" : "";
}
export function appPath(path = "/") { return `${appBasePath()}${path}`; }
export function appOrigin() {
    return appBasePath() ? `${window.location.origin}${appBasePath()}` : "";
}
