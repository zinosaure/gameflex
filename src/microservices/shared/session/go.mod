module gameflex/session

go 1.23

require (
	gameflex/auth v0.0.0
	github.com/gorilla/websocket v1.5.3
)

// Same absolute-path mechanism as every other shared module in this repo —
// the images copy /shared wholesale (see each runtime's Dockerfile).
replace gameflex/auth => /shared/auth
