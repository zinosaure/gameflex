module gameflex/resources

go 1.23

require (
	gameflex/consoles v0.0.0
	gameflex/emuparams v0.0.0
	gameflex/storage v0.0.0
	github.com/hirochachacha/go-smb2 v1.1.0
	github.com/jlaffaye/ftp v0.2.0
	github.com/pkg/sftp v1.13.7
	golang.org/x/crypto v0.31.0
)

require (
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/kr/fs v0.1.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
)

// LES RESSOURCES EXTERNES (`#808`) : le kernel du serveur et celui du téléphone les partagent — un seul code pour les
// deux serveurs. Même mécanisme de chemin absolu que les autres modules partagés.
replace gameflex/consoles => /shared/consoles

replace gameflex/emuparams => /shared/emuparams

replace gameflex/storage => /shared/storage
