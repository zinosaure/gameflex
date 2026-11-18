module gameflex/storage

go 1.23

require (
	gameflex/consoles v0.0.0
	github.com/google/uuid v1.6.0
	github.com/makiuchi-d/gozxing v0.1.1
)

require (
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)

// Même mécanisme de chemin absolu que les autres modules partagés — les
// images copient /shared en entier (voir les Dockerfile de chaque runtime).
replace gameflex/consoles => /shared/consoles
