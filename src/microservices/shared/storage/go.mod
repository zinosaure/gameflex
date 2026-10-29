module gameflex/storage

go 1.23

require (
	gameflex/consoles v0.0.0
	github.com/google/uuid v1.6.0
	github.com/hirochachacha/go-smb2 v1.1.0
	github.com/makiuchi-d/gozxing v0.1.1
)

require (
	github.com/geoffgarside/ber v1.1.0 // indirect
	golang.org/x/crypto v0.0.0-20200728195943-123391ffb6de // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)

// Même mécanisme de chemin absolu que les autres modules partagés — les
// images copient /shared en entier (voir les Dockerfile de chaque runtime).
replace gameflex/consoles => /shared/consoles
