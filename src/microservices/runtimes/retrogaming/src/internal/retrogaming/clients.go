package retrogaming

import (
	"os"

	"gameflex/retrogaming/internal/storage_client"
)

// Storage paths for this session: the kernel resolves the section paths and
// passes them as env vars (storage_client.AdoptEnv) — those variables are its
// whole world. Set by RunChild.
//
// There was also a sessions-service client here, for registering the session
// and its players in the cross-platform registry. The sessions service does
// that itself now, from the spawn and its watchdog, so this process has no
// reason to talk back to it.
var storagePaths *storage_client.Client

// storageRootFromEnv is the root fallback — the section paths arrive as
// resolved absolute env vars, only the root itself can be missing when this
// binary is started by hand.
func storageRootFromEnv() string {
	if root := os.Getenv("GAMEFLEX_ROOT_PATH"); root != "" {
		return root
	}
	return "/gameflex"
}
