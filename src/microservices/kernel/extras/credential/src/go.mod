module gameflex/credential

go 1.25.0

require (
	gameflex/auth v0.0.0-00010101000000-000000000000
	gameflex/consoles v0.0.0
	gameflex/storage v0.0.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/gorilla/websocket v1.5.3
	golang.org/x/crypto v0.32.0
	modernc.org/sqlite v1.57.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hirochachacha/go-smb2 v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// Le module partagé, au MÊME chemin absolu que dans les autres images
// (/shared). Le service d'identité en a besoin depuis qu'il sonde les
// partages : il détient déjà les identifiants, et faire faire la sonde
// ailleurs obligerait à les faire voyager.
replace gameflex/storage => /shared/storage

replace gameflex/consoles => /shared/consoles

// Les jetons signés en Ed25519 et le secret de chaque partie (chantier 34, P10).
replace gameflex/auth => /shared/auth
