module gameflex/federation

go 1.25.0

require (
	gameflex/consoles v0.0.0
	gameflex/emuparams v0.0.0
	gameflex/lan v0.0.0
	gameflex/logfile v0.0.0
	gameflex/metadata v0.0.0
	gameflex/storage v0.0.0
	github.com/go-chi/chi/v5 v5.3.1
	github.com/hirochachacha/go-smb2 v1.1.0
	github.com/jlaffaye/ftp v0.2.4
	github.com/pkg/sftp v1.13.12
	golang.org/x/crypto v0.54.0
	modernc.org/sqlite v1.57.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/makiuchi-d/gozxing v0.1.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// Les modules partagés de GameFlex, par le même chemin absolu que les autres services : l'image copie
// src/microservices/shared/ dans /shared. Le service n'a besoin que du scan et du listing (gameflex/storage),
// pas d'un service qui tourne : il se lance seul (chantier 61, le porteur, 2026-11-18).
replace gameflex/storage => /shared/storage

replace gameflex/consoles => /shared/consoles

// La découverte en réseau local (chantier 60), pour le mode découverte du code d'appairage.
replace gameflex/lan => /shared/lan

// Les journaux sur le disque, ceux de GameFlex (le journal du jour, ses niveaux, les secrets masqués).
replace gameflex/logfile => /shared/logfile

// Les ressources internes (chantier 61) : les paramètres des émulateurs et les fiches des jeux, ceux de GameFlex.
replace gameflex/emuparams => /shared/emuparams

replace gameflex/metadata => /shared/metadata
