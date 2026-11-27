# Hardened image for the example service. Build from the repo root: make docker
# Base images are pinned by digest; Dependabot bumps them.
FROM cgr.dev/chainguard/go:latest@sha256:97b081e091e2c7df54b30f468969ea2e62d7d1a64fff89346facb564da979300 AS builder

WORKDIR /workdir

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY pkg ./pkg
COPY example ./example
ARG APP_VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=${APP_VERSION}" -o /app ./example

# --- Runtime: no shell, no package manager, only the static binary ---
FROM cgr.dev/chainguard/static:latest@sha256:599c3cd7597b84aa149c4282beecc8c6d4e1cb5c95dc8d982e70cd0a2a9147a2

COPY --from=builder /app /app

# Numeric so Kubernetes runAsNonRoot can verify it.
USER 65532:65532

ENV APP_ADDR=0.0.0.0:8080

EXPOSE 8080

# No shell or curl here to run a probe; probe /readyz from the orchestrator.
HEALTHCHECK NONE

ENTRYPOINT ["/app"]
CMD ["serve"]
