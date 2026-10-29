module gameflex/retrogaming

go 1.23

require (
	gameflex/consoles v0.0.0
	gameflex/gatewayclient v0.0.0
	gameflex/libretro v0.0.0
	gameflex/session v0.0.0
	gameflex/sessionstatus v0.0.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
)

require gameflex/auth v0.0.0 // indirect

// Shared console registry (see src/microservices/shared/consoles) — absolute path because
// the image build AND the hot-reload container both mount it at /shared.
replace gameflex/consoles => /shared/consoles

// chantier-4: the session authority and the JWT verification both live in
// /shared so the three runtimes share one implementation instead of each
// growing its own. Same absolute-path mechanism as the console registry.
replace gameflex/auth => /shared/auth

replace gameflex/session => /shared/session

// Shared internal gateway transport (see src/microservices/shared/gatewayclient) — Phase 2:
// the child pushes encoded frames here instead of holding its own pion code.
replace gameflex/gatewayclient => /shared/gatewayclient

// Le moteur libretro (pont et contexte graphique) — partagé avec le kernel du
// téléphone depuis le chantier 13 : un seul moteur pour Linux et Android.
replace gameflex/libretro => /shared/libretro

// Le GPU que la partie a reçu (son occupation, sa mémoire vidéo) se lit comme
// dans streamex — chantier 39.
replace gameflex/sessionstatus => /shared/sessionstatus
