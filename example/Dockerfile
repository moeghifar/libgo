# Hardened image for the example service. Build from the repo root: make docker
# Base images are pinned by digest; Dependabot bumps them.
FROM cgr.dev/chainguard/go:latest@sha256:8348380e90a63090904e1b124c387c61e68801e9d8f7e8a3784e83cd7d154be1 AS builder

WORKDIR /workdir

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY pkg ./pkg
COPY example ./example
ARG APP_VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=${APP_VERSION}" -o /app ./example

# --- Runtime: no shell, no package manager, only the static binary ---
FROM cgr.dev/chainguard/static:latest@sha256:8658e9eaeb2294fa0e49b0667f7d66b4e21fa7690b8d2a4c30cd1d013ceb9eb7

COPY --from=builder /app /app

# Numeric so Kubernetes runAsNonRoot can verify it.
USER 65532:65532

ENV APP_ADDR=0.0.0.0:8080

EXPOSE 8080

# No shell or curl here to run a probe; probe /readyz from the orchestrator.
HEALTHCHECK NONE

ENTRYPOINT ["/app"]
CMD ["serve"]
