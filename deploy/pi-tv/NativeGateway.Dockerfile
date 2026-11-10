FROM golang:1.24-bookworm AS build
ENV GOMAXPROCS=2 CGO_ENABLED=0
WORKDIR /src
COPY tools/pi-tv/gateway/ ./
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod go mod download && go test -mod=readonly ./... && mkdir -p /licenses && find /go/pkg/mod -iname "license*" -type f -exec cp --parents {} /licenses/ \; && go build -mod=readonly -trimpath -ldflags='-s -w' -o /gateway .
FROM build AS probe-build
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod go build -mod=readonly -trimpath -o /probe ./cmd/probe
FROM scratch AS probe
COPY --from=probe-build /probe /probe
USER 1000:1000
ENTRYPOINT ["/probe"]
FROM scratch AS release
COPY --from=build /gateway /gateway
COPY --from=build /licenses /licenses
USER 1000:1000
ENTRYPOINT ["/gateway"]
