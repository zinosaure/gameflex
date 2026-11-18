// gameflex-federation — un serveur fédéré fait de partages Samba, WebDAV et kDrive (chantier 61).
//
// Il se lance SEUL (le porteur, 2026-11-18 : « il se peut que certaines personnes utilisent que ce service et rien
// d'autre de Gameflex ») : aucun autre service de GameFlex n'est requis, et tout ce qu'il garde est dans son volume.
//
// L'environnement :
//
//	FEDERATION_DATA            le volume de données (défaut /data) : la base, la clé de scellement, le catalogue
//	FEDERATION_PORT            le port d'écoute (défaut 8090)
//	FEDERATION_ADMIN_USERNAME  le compte de l'administrateur, celui de la page de connexion (le porteur,
//	FEDERATION_ADMIN_PASSWORD  2026-11-18 : « un compte passé en varenv dans le docker ») ; obligatoires
//	FEDERATION_DISKS           où les ressources internes sont montées (défaut /mnt) : chaque dossier rangé comme une
//	                           bibliothèque GameFlex (roms/, bios/, cores/, patches/, params/, metadata/) est un disque
//	FEDERATION_NAME            le nom du serveur au premier démarrage (défaut « GameFlex Federation ») ; ensuite,
//	                           l'administration le change
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"gameflex/federation/internal/api"
	"gameflex/federation/internal/catalog"
	"gameflex/federation/internal/store"
	"gameflex/logfile"
)

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}

	return fallback
}

func main() {
	dataDir := env("FEDERATION_DATA", "/data")
	// Le journal de GameFlex : la sortie standard, et le fichier du jour dans le volume (logs/<jour>/federation.log),
	// que l'administration lit (internal/api/logs.go).
	slog.SetDefault(slog.New(logfile.ServiceHandler(os.Stderr, dataDir, "federation")))

	st, err := store.Open(dataDir)
	if err != nil {
		slog.Error("base illisible", "dir", dataDir, "error", err)
		os.Exit(1)
	}
	defer st.Close()
	logfile.Apply(api.LoadLogSettings(st))
	if st.Setting(api.SettingServerName, "") == "" {
		_ = st.SetSetting(api.SettingServerName, env("FEDERATION_NAME", "GameFlex Federation"))
	}
	username, password := env("FEDERATION_ADMIN_USERNAME", ""), env("FEDERATION_ADMIN_PASSWORD", "")
	if username == "" || password == "" {
		slog.Error("FEDERATION_ADMIN_USERNAME et FEDERATION_ADMIN_PASSWORD sont obligatoires : le compte de l'administration")
		os.Exit(1)
	}

	name := func() string { return st.Setting(api.SettingServerName, "GameFlex Federation") }
	cat := catalog.New(st, dataDir, name)
	server := &api.Server{Store: st, Catalog: cat, AdminUsername: username, AdminPassword: password, Name: name,
		DataDir: dataDir, DisksDir: env("FEDERATION_DISKS", "/mnt")}
	cat.Disks = server.DiskResources

	// Une ressource jamais parcourue (un volume neuf, ou un catalogue perdu) l'est au démarrage ; les autres gardent leur
	// dernier parcours. Rien ne se relit tout seul ensuite : le geste « Relire » de l'administration.
	resources, err := st.Resources()
	if err != nil {
		slog.Error("ressources illisibles", "error", err)
		os.Exit(1)
	}
	states := cat.States()
	for _, r := range resources {
		if states[r.ID].ScannedAt.IsZero() {
			cat.Rescan(r)
		}
	}
	// Les ressources internes, elles, se relisent à chaque démarrage : une lecture locale est rapide, et un disque a pu
	// changer pendant que le service était arrêté. Un disque parti emporte ses jeux.
	present := map[int64]bool{}
	for _, disk := range server.DiskResources() {
		present[disk.ID] = true
		cat.Rescan(disk)
	}
	for id := range states {
		if id < 0 && !present[id] {
			cat.Forget(id, true)
		}
	}
	address := ":" + env("FEDERATION_PORT", "8090")
	slog.Info("gameflex-federation à l'écoute", "address", address, "resources", len(resources), "name", name())
	httpServer := &http.Server{Addr: address, Handler: server.Router(), ReadHeaderTimeout: 10 * time.Second}
	if err := httpServer.ListenAndServe(); err != nil {
		slog.Error("arrêt", "error", err)
		os.Exit(1)
	}
}
