// La découverte des serveurs GameFlex en réseau local (chantier 60) : DNS-SD
// sur mDNS, partagée par la passerelle WebRTC du serveur et par le kernel du
// téléphone. Même contrainte que gameflex/playstation : compilable pour
// `GOOS=android GOARCH=arm64` sans cgo, d'où anet pour lister les interfaces
// (#753).
module gameflex/lan

go 1.23

require (
	github.com/wlynxg/anet v0.0.5
	golang.org/x/net v0.29.0
)

require golang.org/x/sys v0.25.0 // indirect
