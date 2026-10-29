module gameflex/kernel

go 1.23

require (
	gameflex/auth v0.0.0-00010101000000-000000000000
	gameflex/consoles v0.0.0
	gameflex/emuparams v0.0.0-00010101000000-000000000000
	gameflex/facade v0.0.0-00010101000000-000000000000
	gameflex/logfile v0.0.0
	gameflex/sessionstatus v0.0.0-00010101000000-000000000000
	gameflex/storage v0.0.0
	github.com/docker/docker v27.3.1+incompatible
	github.com/docker/go-connections v0.5.0
	github.com/go-chi/chi/v5 v5.3.1
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/gorilla/websocket v1.5.3
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hirochachacha/go-smb2 v1.1.0 // indirect
	github.com/makiuchi-d/gozxing v0.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	golang.org/x/crypto v0.27.0 // indirect
	golang.org/x/net v0.29.0 // indirect
	golang.org/x/text v0.19.0 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)

require (
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	// Pinned so the docker v27 dependency tree stays Go-1.23-compatible —
	// unpinned, its otel deps resolve to versions requiring go >= 1.25.
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.44.0 // indirect
	go.opentelemetry.io/otel v1.21.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.21.0 // indirect
	go.opentelemetry.io/otel/metric v1.21.0 // indirect
	go.opentelemetry.io/otel/sdk v1.21.0 // indirect
	go.opentelemetry.io/otel/trace v1.21.0 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	gotest.tools/v3 v3.5.2 // indirect
)

// Shared console registry (see src/microservices/shared/consoles) — absolute path because
// the image build AND the hot-reload container both mount it at /shared.
replace gameflex/consoles => /shared/consoles

replace gameflex/sessionstatus => /shared/sessionstatus

replace gameflex/logfile => /shared/logfile

replace gameflex/storage => /shared/storage

// Le centre de messages, partagé avec le kernel du téléphone : la page est la
// même, le protocole doit l'être par construction.
replace gameflex/facade => /shared/facade

// Les paramètres des émulateurs, partagés avec le kernel du téléphone (#347) :
// « c'est une feature en commun ».
replace gameflex/emuparams => /shared/emuparams

// Les jetons signés en Ed25519 et le secret de chaque partie (chantier 34, P10).
replace gameflex/auth => /shared/auth
