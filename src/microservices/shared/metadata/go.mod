module gameflex/metadata

go 1.23

require (
	gameflex/consoles v0.0.0
	gameflex/storage v0.0.0
)

require github.com/google/uuid v1.6.0 // indirect

// Les images et la fiche d'un jeu (chantier 49), partagées depuis le chantier 57 : le kernel du serveur, et celui du
// téléphone à la P4. Même mécanisme de chemin absolu que les autres modules partagés — les images copient /shared en
// entier (voir les Dockerfile de chaque service).
replace gameflex/consoles => /shared/consoles

replace gameflex/storage => /shared/storage
