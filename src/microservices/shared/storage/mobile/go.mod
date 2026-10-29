module gameflex/storage/mobile

go 1.26.0

require gameflex/storage v0.0.0

require (
	gameflex/consoles v0.0.0 // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hirochachacha/go-smb2 v1.1.0 // indirect
	golang.org/x/crypto v0.0.0-20200728195943-123391ffb6de // indirect
	golang.org/x/mobile v0.0.0-20260821190718-4776eadac327 // indirect
	golang.org/x/mod v0.39.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
)

replace gameflex/storage => /shared/storage

replace gameflex/consoles => /shared/consoles

tool golang.org/x/mobile/cmd/gobind
