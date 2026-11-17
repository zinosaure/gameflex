module gameflex/libretro

go 1.23

require gameflex/consoles v0.0.0

// biosdir lit le registre des consoles. Même mécanisme de chemin absolu que
// les autres modules partagés : chaque image et le module du téléphone montent
// /shared.
replace gameflex/consoles => /shared/consoles
