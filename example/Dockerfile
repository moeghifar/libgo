# Hardened image for the example service. Build from the repo root: make docker
# Base images are pinned by digest; Dependabot bumps them.
FROM cgr.dev/chainguard/go:latest@sha256:d5eb532c64db43c555c9fbf644628aac2690dcbfd07ac8b2bae97a7c66d085fb AS builder

WORKDIR /workdir

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY pkg ./pkg
COPY example ./example
ARG APP_VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=${APP_VERSION}" -o /app ./example

# --- Runtime: no shell, no package manager, only the static binary ---
FROM cgr.dev/chainguard/static:latest@sha256:54b5f14edad6f2554774d499539e837588a501bd5bf22ac6cd97b1068c35d44d

COPY --from=builder /app /app

# Numeric so Kubernetes runAsNonRoot can verify it.
USER 65532:65532

ENV APP_ADDR=0.0.0.0:8080

EXPOSE 8080

# No shell or curl here to run a probe; probe /readyz from the orchestrator.
HEALTHCHECK NONE

ENTRYPOINT ["/app"]
CMD ["serve"]
